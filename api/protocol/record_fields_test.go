package protocol

import (
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestMutationFieldPolicy(t *testing.T) {
	document := &pb.Document{ContentType: "application/opaque", Data: []byte("uninterpreted")}
	invalidDocument := &pb.Document{ContentType: "INVALID"}
	oversizedDocument := &pb.Document{ContentType: "application/opaque", Data: make([]byte, MaxDocument+1)}
	empty := &pb.Empty{}
	put := &pb.MutateRequest_Put{Put: document}
	create := &pb.MutateRequest_Create{Create: document}
	replace := &pb.MutateRequest_Replace{Replace: document}
	deleteAction := &pb.MutateRequest_Delete{Delete: empty}
	nilPut := &pb.MutateRequest_Put{}
	nilDelete := &pb.MutateRequest_Delete{}
	invalidPut := &pb.MutateRequest_Put{Put: invalidDocument}
	oversizedPut := &pb.MutateRequest_Put{Put: oversizedDocument}
	program := &pb.LuaTransform{Source: []byte("return weir.keep()"), Input: document}
	programForm := &pb.Transform_Lua{Lua: program}
	programTransform := &pb.Transform{Form: programForm}
	programAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: programTransform}
	expression := &pb.Transform_BackendExpression{BackendExpression: document}
	expressionTransform := &pb.Transform{Form: expression}
	expressionAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: expressionTransform}
	nilTransform := &pb.MutateRequest_AtomicTransform{}
	missingForm := &pb.Transform{}
	missingFormAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: missingForm}
	valid := []*pb.MutateRequest{
		{Resource: "data/s:key", Action: put},
		{Resource: "data/s:key", Action: create},
		{Resource: "data/s:key", Action: replace},
		{Resource: "data/s:key", Action: deleteAction},
		{Resource: "data/s:key", Action: programAction},
		{Resource: "data/s:key", Action: expressionAction},
	}
	for _, mutation := range valid {
		request := mutationExecution("records", 1, mutation)
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatalf("valid opaque mutation rejected: %v", err)
		}
	}
	invalid := []*pb.MutateRequest{
		{Resource: "data/s:key"},
		{Resource: "data/s:key", Action: nilPut},
		{Resource: "data/s:key", Action: nilDelete},
		{Resource: "data/s:key", Action: invalidPut},
		{Resource: "data/s:key", Action: oversizedPut},
		{Resource: "data/s:key", Action: nilTransform},
		{Resource: "data/s:key", Action: missingFormAction},
	}
	for _, mutation := range invalid {
		request := mutationExecution("records", 1, mutation)
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("invalid mutation accepted", mutation)
		}
	}
	for _, source := range [][]byte{nil, []byte(strings.Repeat("x", MaxExpression+1)), {0xff}, {'x', 0}, []byte("\x1bLua")} {
		program.Source = source
		request := mutationExecution("records", 1, valid[4])
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("invalid program source accepted")
		}
	}
	program.Source = []byte(strings.Repeat("x", MaxExpression))
	request := mutationExecution("records", 1, valid[4])
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("program at the byte bound rejected", err)
	}
	program.Input = invalidDocument
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("invalid program input accepted")
	}
	for _, data := range [][]byte{nil, make([]byte, MaxExpression+1)} {
		document.Data = data
		operation := &pb.Command_Mutate{Mutate: valid[5]}
		request.Command.Operation = operation
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("empty or oversized expression accepted")
		}
	}
}

func TestRecordResponsesAcceptAdditiveFieldsAtEveryNestedLevel(t *testing.T) {
	document := &pb.Document{ContentType: "application/opaque", Data: []byte("uninterpreted")}
	read := ReadDocument(document)
	response := readResponse(1, read)
	for position := range 4 {
		copied := proto.Clone(response).(*pb.ExecuteResponse)
		messages := []proto.Message{copied, copied.Event, copied.Event.GetReadResult(), copied.Event.GetReadResult().GetDocument()}
		if err := ValidateExecuteResponse(copied); err != nil {
			t.Fatal("fixture must be valid before mutation", err)
		}
		messages[position].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		if err := ValidateExecuteResponse(copied); err != nil {
			t.Fatalf("additive Read field rejected at level %d: %v", position, err)
		}
		if err := ValidateReadResult(copied.Event.GetReadResult()); err != nil {
			t.Fatalf("additive direct Read field rejected at level %d: %v", position, err)
		}
	}
	failure := Fail(pb.FailureCode_UNAVAILABLE, "unavailable")
	mutation := Mutation(pb.MutationOutcome_NOT_APPLIED, failure)
	response = mutationResponse(1, mutation)
	for position := range 4 {
		copied := proto.Clone(response).(*pb.ExecuteResponse)
		messages := []proto.Message{copied, copied.Event, copied.Event.GetMutationResult(), copied.Event.GetMutationResult().Failure}
		if err := ValidateExecuteResponse(copied); err != nil {
			t.Fatal("fixture must be valid before mutation", err)
		}
		messages[position].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		if err := ValidateExecuteResponse(copied); err != nil {
			t.Fatalf("additive Mutate field rejected at level %d: %v", position, err)
		}
		if err := ValidateMutationResult(copied.Event.GetMutationResult()); err != nil {
			t.Fatalf("additive direct Mutate field rejected at level %d: %v", position, err)
		}
	}
}

func TestRecordResourcesRespectRelativePathBound(t *testing.T) {
	for _, store := range []string{"a", "target", strings.Repeat("a", 63)} {
		limit := MaxResourceBytes
		for _, extra := range []int{0, 1} {
			resource := strings.Repeat("x", limit+extra)
			read := &pb.ReadRequest{Resource: resource}
			request := readExecution(store, 1, read)
			if err := ValidateExecuteRequest(request); (err == nil) != (extra == 0) {
				t.Fatalf("store length %d, resource length %d: %v", len(store), len(resource), err)
			}
		}
	}
}

func FuzzReadResource(f *testing.F) {
	for _, resource := range []string{"data/s:key", "data/s:a%2Fb", "data/%E4%B8%AD", "data/%61", "data/%00", "/data", "data/", "weir://other/data"} {
		f.Add("records", resource)
	}
	f.Add(strings.Repeat("a", 63), strings.Repeat("x", MaxResourceBytes))
	f.Fuzz(func(t *testing.T, store, resource string) {
		segments, resourceError := ParseRelativeResource(resource)
		expected := ValidStoreName(store) && resourceError == nil && len(segments) != 0
		read := &pb.ReadRequest{Resource: resource}
		request := readExecution(store, 1, read)
		if err := ValidateExecuteRequest(request); (err == nil) != expected {
			t.Fatalf("Store %q resource %q expected %v: %v", store, resource, expected, err)
		}
	})
}
