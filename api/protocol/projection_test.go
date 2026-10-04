package protocol

import (
	"fmt"
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestScanProjectionValidatesUniformFieldPaths(t *testing.T) {
	valid := []*pb.Projection{
		nil,
		{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"name", "profile.age", "_id"}},
		{Mode: pb.ProjectionMode_EXCLUDE, Fields: []string{"private.token", "私人.字段"}},
		{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"a", "ab", "a-b"}},
		{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{strings.Repeat("x", MaxProjectionFieldBytes)}},
	}
	for _, projection := range valid {
		request := &pb.ScanRequest{Resource: "records", Projection: projection}
		if failure := ValidateScan(request); failure != nil {
			t.Fatal("valid projection rejected", projection, failure)
		}
	}
	invalid := []*pb.Projection{
		{},
		{Mode: pb.ProjectionMode(99), Fields: []string{"name"}},
		{Mode: pb.ProjectionMode_INCLUDE},
		{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"name", "name"}},
		{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"a", "a-b", "a.b"}},
		{Mode: pb.ProjectionMode_EXCLUDE, Fields: []string{"profile.age", "profile"}},
	}
	for _, field := range []string{"", ".name", "name.", "a..b", "$field", "a.$field", "a.*", "a?", "a\x00b", string([]byte{0xff}), strings.Repeat("x", MaxProjectionFieldBytes+1)} {
		projection := &pb.Projection{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{field}}
		invalid = append(invalid, projection)
	}
	for _, projection := range invalid {
		request := &pb.ScanRequest{Resource: "records", Projection: projection}
		if failure := ValidateScan(request); failure == nil || failure.Code != pb.FailureCode_INVALID_ARGUMENT {
			t.Fatal("invalid projection accepted", projection, failure)
		}
	}
}

func TestScanProjectionBoundsAndUnknownFields(t *testing.T) {
	fields := make([]string, MaxProjectionFields)
	for index := range fields {
		fields[index] = fmt.Sprintf("field%d", index)
	}
	projection := &pb.Projection{Mode: pb.ProjectionMode_INCLUDE, Fields: fields}
	request := &pb.ScanRequest{Resource: "records", Projection: projection}
	if failure := ValidateScan(request); failure != nil {
		t.Fatal("maximum field count rejected", failure)
	}
	projection.Fields = append(fields, "extra")
	if failure := ValidateScan(request); failure == nil {
		t.Fatal("excess field count accepted")
	}
	projection.Fields = make([]string, 17)
	for index := range projection.Fields {
		projection.Fields[index] = fmt.Sprintf("field%d", index) + strings.Repeat("x", 490)
	}
	if failure := ValidateScan(request); failure == nil {
		t.Fatal("excess projection metadata accepted")
	}
	projection.Fields = []string{"name"}
	projection.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if failure := ValidateScan(request); failure == nil {
		t.Fatal("unknown projection fields accepted")
	}
	operation := &pb.Command_Scan{Scan: request}
	command := &pb.Command{Operation: operation}
	if err := ValidateCommand(command); err == nil {
		t.Fatal("unknown nested projection accepted by command boundary")
	}
}

func TestScanProjectionAndFilterBindContinuation(t *testing.T) {
	filter := &pb.Document{ContentType: "application/json", Data: []byte(`{"term":{"state":"ready"}}`)}
	projection := &pb.Projection{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"name"}}
	request := &pb.ScanRequest{Resource: "records", Filter: filter, Projection: projection}
	fingerprint := ScanFingerprint(request, "search", "search:elasticsearch")
	state := []byte("opaque backend state")
	token, err := EncodeScanToken("search:elasticsearch", fingerprint, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*pb.ScanRequest){
		func(r *pb.ScanRequest) { r.Projection.Mode = pb.ProjectionMode_EXCLUDE },
		func(r *pb.ScanRequest) { r.Projection.Fields[0] = "other" },
		func(r *pb.ScanRequest) { r.Projection = nil },
		func(r *pb.ScanRequest) { r.Filter.Data = []byte(`{"match_all":{}}`) },
	} {
		copied := proto.Clone(request).(*pb.ScanRequest)
		mutate(copied)
		changed := ScanFingerprint(copied, "search", "search:elasticsearch")
		if _, err := DecodeScanToken(token, "search:elasticsearch", changed); err == nil {
			t.Fatal("continuation accepted different filter or projection")
		}
	}
}
