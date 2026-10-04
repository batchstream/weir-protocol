package protocol

import (
	"strings"
	"testing"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestCanonicalRelativeResource(t *testing.T) {
	valid := []string{"db/c/s:a%2Fb", "a/s:%E4%B8%AD", "a/i:-3", "records"}
	for _, resource := range valid {
		if _, err := ParseRelativeResource(resource); err != nil {
			t.Errorf("%s: %v", resource, err)
		}
	}
	invalid := []string{"", "/records", "records/", "weir://mongo/db/c/s:a", "a//b", "a/..", "a/%61", "a/%2f", "a/%FF", "a/%00", "x?q=a", "x#f", strings.Repeat("a", MaxResourceBytes+1)}
	for _, resource := range invalid {
		if _, err := ParseRelativeResource(resource); err == nil {
			t.Errorf("accepted %q", resource)
		}
	}
}

func TestLuaRejectsPrecompiledBytecode(t *testing.T) {
	program := &pb.LuaTransform{Source: []byte("\x1bLua")}
	programForm := &pb.Transform_Lua{Lua: program}
	transform := &pb.Transform{Form: programForm}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation := &pb.MutateRequest{Resource: "db/c/s:a", Action: action}
	if failure := validateMutationFields(mutation); failure == nil || failure.Code != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(failure)
	}
}

func FuzzRelativeResource(f *testing.F) {
	f.Add("db/c/s:a%2Fb")
	f.Fuzz(func(t *testing.T, resource string) { _, _ = ParseRelativeResource(resource) })
}

func TestExpressionWireBoundaryIsOpaque(t *testing.T) {
	doc := &pb.Document{ContentType: "application/unknown", Data: []byte("not BSON or JSON")}
	form := &pb.Transform_BackendExpression{BackendExpression: doc}
	transform := &pb.Transform{Form: form}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation := &pb.MutateRequest{Resource: "db/c/s:a", Action: action}
	if f := validateMutationFields(mutation); f != nil {
		t.Fatal("Core interpreted opaque expression", f)
	}
	for _, raw := range [][]byte{nil, make([]byte, MaxExpression+1)} {
		doc.Data = raw
		if f := validateMutationFields(mutation); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
			t.Fatal(f)
		}
	}
	transform.Form = nil
	if f := validateMutationFields(mutation); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(f)
	}
}

func TestExecuteTypedEnvelopeAndUnknownFields(t *testing.T) {
	scan := &pb.ScanRequest{Resource: "records", PageSize: 1}
	variant := &pb.Command_Scan{Scan: scan}
	command := &pb.Command{Operation: variant}
	request := &pb.ExecuteRequest{StoreName: "records", Index: 1, Command: command}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*pb.ExecuteRequest){
		func(r *pb.ExecuteRequest) { r.StoreName = "bad/store" },
		func(r *pb.ExecuteRequest) { r.Index = 0 },
		func(r *pb.ExecuteRequest) { r.Index = 2 },
		func(r *pb.ExecuteRequest) { r.Command = nil },
		func(r *pb.ExecuteRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
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
	document := &pb.Document{ContentType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	value := &pb.Event_Document{Document: document}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	response.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("unknown response accepted")
	}
}

func TestNativeUsesOneBoundedIndexedFrame(t *testing.T) {
	body := &pb.Document{ContentType: "application/vnd.example.native", Data: make([]byte, MaxNativeRequestBytes)}
	native := &pb.NativeRequest{Resource: "records", Request: body}
	operation := &pb.Command_Native{Native: native}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "records", Index: 1, Command: command}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("bounded Native body rejected", err)
	}
	request.Index = 2
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("Native record ordinal accepted")
	}
	request.Index = 1
	body.Data = make([]byte, MaxNativeRequestBytes+1)
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("Native command envelope bytes ignored")
	}
	head := &pb.NativeHead{}
	value := &pb.Event_Head{Head: head}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	response.Index = 2
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("Native response record ordinal accepted")
	}
}

func TestEventValidationAndTypedEncoding(t *testing.T) {
	document := &pb.Document{ContentType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	value := &pb.Event_Document{Document: document}
	event := &pb.Event{Value: value}
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
	badDocument := &pb.Document{ContentType: "invalid"}
	badDocumentValue := &pb.Event_Document{Document: badDocument}
	badChunkValue := &pb.Event_Chunk{Chunk: make([]byte, NativeChunk+1)}
	badEnd := &pb.NativeEnd{Completion: pb.NativeCompletion(99)}
	badEndValue := &pb.Event_NativeEnd{NativeEnd: badEnd}
	incomplete := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE}
	incompleteValue := &pb.Event_NativeEnd{NativeEnd: incomplete}
	invalid := []*pb.Event{
		{}, {Value: badDocumentValue}, {Value: badChunkValue},
		{Value: badEndValue}, {Value: incompleteValue},
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
