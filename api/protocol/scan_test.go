package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestScanFingerprintStableVectors(t *testing.T) {
	plain := &pb.ScanRequest{Resource: "records"}
	filter := &pb.Document{ContentType: "application/json", Data: []byte(`{"term":{"state":"ready"}}`)}
	projection := &pb.Projection{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"name", "profile.age"}}
	full := &pb.ScanRequest{Resource: "index", Filter: filter, Projection: projection}
	binaryFilter := &pb.Document{ContentType: "application/bson", Data: []byte{0, 0xff, 0}}
	binaryProjection := &pb.Projection{Mode: pb.ProjectionMode_EXCLUDE, Fields: []string{"私人.字段"}}
	binaryRequest := &pb.ScanRequest{Resource: "db/c", Filter: binaryFilter, Projection: binaryProjection}
	emptyFilter := &pb.Document{}
	filterPresence := &pb.ScanRequest{Resource: "records", Filter: emptyFilter}
	emptyProjection := &pb.Projection{}
	projectionPresence := &pb.ScanRequest{Resource: "records", Projection: emptyProjection}
	vectors := []struct {
		name    string
		request *pb.ScanRequest
		backend string
		want    string
	}{
		{name: "plain", request: plain, backend: "mongodb:v1", want: "c35158171bf056c084bdce780b1c40be76ff491d40cb5459609d37e4c0b6bb62"},
		{name: "full", request: full, backend: "search:elasticsearch:v1", want: "dba9e81832457f5bbcb00ca4970fae3cb77e2120b6e18d9cd61b393d95ecd541"},
		{name: "binary", request: binaryRequest, backend: "mongodb:v1", want: "663f30fc00407a80ff9648c6346f38426f05fc9172654eb95cd63f5fb96a942f"},
		{name: "filter presence", request: filterPresence, backend: "mongodb:v1", want: "dd1cc3cf99d81bbeda9c88a7633cc9a557bf95cf314e7f2d79ab030198c627da"},
		{name: "projection presence", request: projectionPresence, backend: "mongodb:v1", want: "a98115b67c3760686abc4b319beff10f6eaeb135eb5728c6d98e3a489ca5f50d"},
	}
	for _, vector := range vectors {
		t.Run(vector.name, func(t *testing.T) {
			if got := ScanFingerprint(vector.request, "example", vector.backend); got != vector.want {
				t.Fatalf("traversal fingerprint changed: %s, want %s", got, vector.want)
			}
			copied := proto.Clone(vector.request).(*pb.ScanRequest)
			copied.PageSize = 256
			copied.ContinuationToken = []byte("opaque")
			copied.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
			if got := ScanFingerprint(copied, "example", vector.backend); got != vector.want {
				t.Fatal("identity depends on page size, token or protobuf unknown fields", got)
			}
		})
	}
	full.Projection.Fields[0], full.Projection.Fields[1] = full.Projection.Fields[1], full.Projection.Fields[0]
	if got := ScanFingerprint(full, "example", "search:elasticsearch:v1"); got == vectors[1].want {
		t.Fatal("projection field order was not preserved")
	}
	leftFilter := &pb.Document{ContentType: "a", Data: []byte("bc")}
	rightFilter := &pb.Document{ContentType: "ab", Data: []byte("c")}
	left := &pb.ScanRequest{Resource: "records", Filter: leftFilter}
	right := &pb.ScanRequest{Resource: "records", Filter: rightFilter}
	if ScanFingerprint(left, "example", "mongodb:v1") == ScanFingerprint(right, "example", "mongodb:v1") {
		t.Fatal("field boundaries are ambiguous")
	}
}

func TestScanTokenVersionAndFixedEncoding(t *testing.T) {
	const fingerprint = "c35158171bf056c084bdce780b1c40be76ff491d40cb5459609d37e4c0b6bb62"
	const want = `{"version":1,"backend":"mongodb:v1","fingerprint":"c35158171bf056c084bdce780b1c40be76ff491d40cb5459609d37e4c0b6bb62","state":"c3RhdGU=","checksum":"cdf4a2ea4e2af20471dca9e5495ab414ba014f817be83076088e40ec27dbecff"}`
	token, err := EncodeScanToken("mongodb:v1", fingerprint, []byte("state"))
	if err != nil || string(token) != want {
		t.Fatal("token format changed", string(token), err)
	}
	state, err := DecodeScanToken([]byte(want), "mongodb:v1", fingerprint)
	if err != nil || string(state) != "state" {
		t.Fatal("fixed token not readable", err)
	}
	for _, version := range []uint32{0, 2, 4294967295} {
		var value scanContinuation
		if err := json.Unmarshal(token, &value); err != nil {
			t.Fatal(err)
		}
		value.Version = version
		invalid, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeScanToken(invalid, "mongodb:v1", fingerprint); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatal("unsupported version not rejected before checksum or checkpoint consumption", version, err)
		}
	}
	missingVersion := bytes.Replace(token, []byte(`"version":1,`), nil, 1)
	tamperedState := bytes.Replace(token, []byte(`"state":"c3RhdGU="`), []byte(`"state":"e3RhdGU="`), 1)
	for _, invalid := range [][]byte{missingVersion, tamperedState} {
		if _, err := DecodeScanToken(invalid, "mongodb:v1", fingerprint); err == nil {
			t.Fatal("unversioned or tampered token accepted")
		}
	}
}

func TestScanContinuationBindingAndBounds(t *testing.T) {
	filter := &pb.Document{ContentType: "application/json", Data: []byte(`{"match_all":{}}`)}
	request := &pb.ScanRequest{Resource: "records", Filter: filter, PageSize: 2}
	fingerprint := ScanFingerprint(request, "example", "example:ordered")
	state := []byte(`{"pit":"native","after":3}`)
	token, err := EncodeScanToken("example:ordered", fingerprint, state)
	if err != nil {
		t.Fatal(err)
	}
	request.PageSize = 3
	request.ContinuationToken = token
	if ScanFingerprint(request, "example", "example:ordered") != fingerprint {
		t.Fatal("changing page size changed traversal identity")
	}
	decoded, err := DecodeScanToken(token, "example:ordered", fingerprint)
	if err != nil || !bytes.Equal(decoded, state) {
		t.Fatal("continuation roundtrip", err)
	}
	for _, resource := range []string{"other", "records/s:other"} {
		request.Resource = resource
		if _, err := DecodeScanToken(token, "example:ordered", ScanFingerprint(request, "example", "example:ordered")); err == nil {
			t.Fatal("accepted different traversal", resource)
		}
	}
	request.Resource = "records"
	otherStore := ScanFingerprint(request, "other", "example:ordered")
	if _, err := DecodeScanToken(token, "example:ordered", otherStore); err == nil {
		t.Fatal("accepted continuation from another Store")
	}
	if _, err := DecodeScanToken(token, "example:other", fingerprint); err == nil {
		t.Fatal("accepted another backend dialect")
	}
	corrupt := bytes.Replace(token, []byte(`"checksum":"`), []byte(`"checksum":"x`), 1)
	for _, invalid := range [][]byte{nil, append(token, ' '), corrupt, []byte(strings.Repeat("x", MaxScanToken+1)), []byte(`{"state":"broken"}`)} {
		if _, err := DecodeScanToken(invalid, "example:ordered", fingerprint); err == nil {
			t.Fatal("accepted malformed continuation")
		}
	}
	if _, err := EncodeScanToken("example:ordered", fingerprint, make([]byte, MaxScanState+1)); err == nil {
		t.Fatal("encoded excessive native state")
	}
	if _, err := EncodeScanToken("example:ordered", fingerprint, nil); err == nil {
		t.Fatal("encoded missing native state")
	}
	maximumState := make([]byte, MaxScanState)
	maximumToken, err := EncodeScanToken("example:ordered", fingerprint, maximumState)
	if err != nil {
		t.Fatal("maximum native state rejected", err)
	}
	decoded, err = DecodeScanToken(maximumToken, "example:ordered", fingerprint)
	if err != nil || !bytes.Equal(decoded, maximumState) {
		t.Fatal("maximum native state not readable", err)
	}
}

func TestScanEndRequiresOneSuccessCheckpoint(t *testing.T) {
	failure := Fail(pb.FailureCode_UNAVAILABLE, "fixture")
	cases := []struct {
		end   *pb.ScanEnd
		valid bool
	}{
		{end: &pb.ScanEnd{Exhausted: true}, valid: true},
		{end: &pb.ScanEnd{NextContinuationToken: []byte("token")}, valid: true},
		{end: &pb.ScanEnd{Failure: failure}, valid: true},
		{end: &pb.ScanEnd{}, valid: false},
		{end: &pb.ScanEnd{Exhausted: true, NextContinuationToken: []byte("token")}, valid: false},
		{end: &pb.ScanEnd{Failure: failure, Exhausted: true}, valid: false},
		{end: &pb.ScanEnd{Failure: failure, NextContinuationToken: []byte("token")}, valid: false},
	}
	for _, test := range cases {
		variant := &pb.Event_ScanEnd{ScanEnd: test.end}
		event := &pb.Event{Value: variant}
		if (ValidateEvent(event) == nil) != test.valid {
			t.Fatal("invalid ScanEnd evidence accepted", test.end)
		}
	}
}
