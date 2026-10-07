package protocol

import (
	"math"
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestSingleRecordValidationAndUnknownFields(t *testing.T) {
	read := &pb.ReadRequest{Resource: "data/s:key"}
	request := readExecution("records", 1, read)
	if err := ValidateExecuteRequest(request); err != nil || read.Resource != "data/s:key" {
		t.Fatal("valid input changed or rejected", err)
	}
	for position := range 3 {
		copied := proto.Clone(request).(*pb.ExecuteRequest)
		messages := []proto.Message{copied, copied.Command, copied.Command.GetRead()}
		messages[position].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatalf("unknown input fields accepted at level %d", position)
		}
	}
	request.Command.GetRead().Resource = "/data/s:key"
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("invalid resource accepted")
	}
	operation := &pb.Command_Read{}
	request.Command.Operation = operation
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("nil read accepted")
	}
}

func TestMaximalSingleMutationAndProgramInput(t *testing.T) {
	contentType := strings.Repeat("a", 63) + "/" + strings.Repeat("b", 63)
	document := &pb.Document{ContentType: contentType, Data: make([]byte, MaxDocument)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: strings.Repeat("x", MaxResourceBytes), Action: action}
	request := mutationExecution(strings.Repeat("a", 63), 1, mutation)
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximal mutation rejected", err)
	}
	document.Data = make([]byte, MaxDocument+1)
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("oversized document accepted")
	}
	document.Data = make([]byte, MaxDocument)
	program := &pb.LuaTransform{Source: []byte(strings.Repeat(" ", MaxExpression)), Input: document}
	form := &pb.Transform_Lua{Lua: program}
	transform := &pb.Transform{Form: form}
	transformAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation.Action = transformAction
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximal Lua mutation rejected", err)
	}
}

func TestRecordIndexesHaveNoLogicalCallBudget(t *testing.T) {
	read := &pb.ReadRequest{Resource: "data/s:key"}
	request := readExecution("records", 1, read)
	for _, index := range []uint64{1, 65537, math.MaxUint64 - 1} {
		request.Index = index
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal("valid ordinal rejected", index, err)
		}
	}
	for _, index := range []uint64{0, math.MaxUint64} {
		request.Index = index
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("invalid ordinal accepted", index)
		}
	}
	document := &pb.Document{ContentType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	request = mutationExecution("records", 1, mutation)
	total := 0
	for range 33 {
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal(err)
		}
		total += proto.Size(request)
		request.Index++
	}
	if total <= 64<<20 {
		t.Fatal("fixture did not span a large call")
	}
}

func TestRecordResponsesRejectMalformedApplicationEvidence(t *testing.T) {
	missing := Missing()
	response := readResponse(1, missing)
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	missing.GetMissing().ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal("additive missing-result field rejected", err)
	}
	response.Event.GetReadResult().Result = nil
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("missing result accepted")
	}
	for _, outcome := range []pb.MutationOutcome{pb.MutationOutcome_NOT_STARTED, pb.MutationOutcome_NOT_APPLIED, pb.MutationOutcome_UNKNOWN, pb.MutationOutcome_MUTATION_OUTCOME_UNSPECIFIED, pb.MutationOutcome(99)} {
		mutation := &pb.MutationResult{Outcome: outcome}
		response = mutationResponse(1, mutation)
		if err := ValidateExecuteResponse(response); err == nil {
			t.Fatal("invalid or unqualified outcome accepted", outcome)
		}
	}
	failure := Fail(pb.FailureCode_UNAVAILABLE, "acknowledgement failed")
	applied := Mutation(pb.MutationOutcome_APPLIED, failure)
	response = mutationResponse(1, applied)
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal("APPLIED with acknowledgement failure rejected", err)
	}
	for _, index := range []uint64{0, math.MaxUint64} {
		response.Index = index
		if err := ValidateExecuteResponse(response); err == nil {
			t.Fatal("invalid response index accepted", index)
		}
	}
}
