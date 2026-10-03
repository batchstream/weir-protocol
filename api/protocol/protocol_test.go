package protocol

import (
	"strings"
	"testing"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestCanonicalURI(t *testing.T) {
	valid := []string{"weir://mongo", "weir://mongo/db/c/s:a%2Fb", "weir://a-b/a/s:%E4%B8%AD", "weir://m/a/i:-3"}
	for _, s := range valid {
		if _, _, err := ParseResource(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	invalid := []string{"weir://mongo/", "WEIR://mongo", "weir://mongo:42/x", "weir://m/a//b", "weir://m/a/..", "weir://m/a/%61", "weir://m/a/%2f", "weir://m/a/%FF", "weir://m/a/%00", "weir://a--b/x", "weir://m/x?q=a", "weir://m/x#f", "weir://" + strings.Repeat("a", 64)}
	for _, s := range invalid {
		if _, _, err := ParseResource(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
func TestValidationAndUnsupported(t *testing.T) {
	req := &pb.ReadRequest{Resource: "weir://other/db/c/s:a"}
	v := &pb.Operation_Read{Read: req}
	op := &pb.Operation{Operation: v}
	if f := Validate(op, "mongo"); f == nil || f.Code != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(f)
	}
	program := &pb.ProgramTransform{Runtime: "unqualified", Source: []byte("return weir.keep()")}
	programForm := &pb.Transform_Program{Program: program}
	transform := &pb.Transform{Form: programForm}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	m := &pb.MutateRequest{Resource: "weir://mongo/db/c/s:a", Action: action}
	mv := &pb.Operation_Mutate{Mutate: m}
	op.Operation = mv
	if f := Validate(op, "mongo"); f == nil || f.Code != pb.FailureCode_UNSUPPORTED {
		t.Fatal(f)
	}
}
func FuzzResource(f *testing.F) {
	f.Add("weir://mongo/db/c/s:a%2Fb")
	f.Fuzz(func(t *testing.T, s string) { _, _, _ = ParseResource(s) })
}

func TestExpressionWireBoundaryIsOpaque(t *testing.T) {
	doc := &pb.Document{MediaType: "application/unknown", Data: []byte("not BSON or JSON")}
	form := &pb.Transform_BackendExpression{BackendExpression: doc}
	transform := &pb.Transform{Form: form}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation := &pb.MutateRequest{Resource: "weir://mongo/db/c/s:a", Action: action}
	variant := &pb.Operation_Mutate{Mutate: mutation}
	op := &pb.Operation{Operation: variant}
	if f := Validate(op, "mongo"); f != nil {
		t.Fatal("Core interpreted opaque expression", f)
	}
	for _, raw := range [][]byte{nil, make([]byte, MaxExpression+1)} {
		doc.Data = raw
		if f := Validate(op, "mongo"); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
			t.Fatal(f)
		}
	}
	transform.Form = nil
	if f := Validate(op, "mongo"); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(f)
	}
}

func TestExecuteTypedEnvelopeAndUnknownFields(t *testing.T) {
	scan := &pb.ScanRequest{Resource: "records", PageSize: 1}
	variant := &pb.Command_Scan{Scan: scan}
	command := &pb.Command{Version: 1, Operation: variant}
	request := &pb.ExecuteRequest{StoreName: "records", Command: command}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*pb.ExecuteRequest){
		func(r *pb.ExecuteRequest) { r.StoreName = "bad/store" },
		func(r *pb.ExecuteRequest) { r.Command = nil },
		func(r *pb.ExecuteRequest) { r.ProtoReflect().SetUnknown([]byte{0x18, 1}) },
		func(r *pb.ExecuteRequest) { r.Command.Version = 2 },
		func(r *pb.ExecuteRequest) { r.Command.GetScan().Resource = "weir://records/data" },
		func(r *pb.ExecuteRequest) { r.Command.GetScan().Resource = "/data" },
		func(r *pb.ExecuteRequest) { r.Command.GetScan().Resource = "data/%6B" },
		func(r *pb.ExecuteRequest) { r.Command.GetScan().ProtoReflect().SetUnknown([]byte{0x38, 1}) },
	} {
		copied := proto.Clone(request).(*pb.ExecuteRequest)
		mutate(copied)
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatal("accepted invalid request", copied)
		}
	}
	document := &pb.Document{MediaType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	value := &pb.Event_Document{Document: document}
	event := &pb.Event{Version: 1, Value: value}
	response := &pb.ExecuteResponse{Event: event}
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	response.ProtoReflect().SetUnknown([]byte{0x10, 1})
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("unknown response accepted")
	}
}

func TestEventValidationAndTypedEncoding(t *testing.T) {
	document := &pb.Document{MediaType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	value := &pb.Event_Document{Document: document}
	event := &pb.Event{Version: 1, Value: value}
	encoded, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &pb.Event{}
	if err := proto.Unmarshal(encoded, decoded); err != nil || !proto.Equal(event, decoded) {
		t.Fatal("typed event changed", err)
	}
	if err := ValidateEvent(decoded); err != nil {
		t.Fatal(err)
	}
	failure := Fail(pb.FailureCode_INTERNAL, strings.Repeat("中", 600))
	if len(failure.Message) > 1024 || !utf8.ValidString(failure.Message) {
		t.Fatal("failure envelope not bounded UTF8")
	}
	badDocument := &pb.Document{MediaType: "invalid"}
	badDocumentValue := &pb.Event_Document{Document: badDocument}
	badChunkValue := &pb.Event_Chunk{Chunk: make([]byte, NativeChunk+1)}
	badEnd := &pb.NativeEnd{Completion: pb.NativeCompletion(99)}
	badEndValue := &pb.Event_NativeEnd{NativeEnd: badEnd}
	incomplete := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE}
	incompleteValue := &pb.Event_NativeEnd{NativeEnd: incomplete}
	invalid := []*pb.Event{
		{Version: 1}, {Version: 1, Value: badDocumentValue}, {Version: 1, Value: badChunkValue},
		{Version: 1, Value: badEndValue}, {Version: 1, Value: incompleteValue},
	}
	for _, item := range invalid {
		if err := ValidateEvent(item); err == nil {
			t.Fatal("accepted invalid event")
		}
	}
}

func TestAppliedMutationMayReportPostWriteFailure(t *testing.T) {
	failure := Fail(pb.FailureCode_UNAVAILABLE, "write applied but replica acknowledgement failed")
	mutation := Mutation(pb.MutationOutcome_APPLIED, failure)
	if err := ValidateMutationResult(mutation); err != nil {
		t.Fatal(err)
	}
	failure.Code = pb.FailureCode_FAILURE_CODE_UNSPECIFIED
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("invalid failure accepted")
	}
	mutation.Outcome, mutation.Failure = pb.MutationOutcome_NOT_STARTED, nil
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("unfinished result without failure accepted")
	}
}
