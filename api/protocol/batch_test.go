package protocol

import (
	"math"
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestRecordFrameValidatesEveryRequestWithoutChangingResources(t *testing.T) {
	read := &pb.ReadRequest{Resource: "data/s:key"}
	reads := readFrame("records", 1, []*pb.ReadRequest{read, read})
	if err := ValidateExecuteRequest(reads); err != nil || read.Resource != "data/s:key" {
		t.Fatal("valid input changed or rejected", err)
	}
	batch := reads.Command.GetRead()
	invalid := &pb.ReadRequest{Resource: "/data/s:key"}
	batch.Requests = append(batch.Requests, invalid)
	if err := ValidateExecuteRequest(reads); err == nil {
		t.Fatal("late invalid resource accepted")
	}
	batch.Requests = []*pb.ReadRequest{read, nil}
	if err := ValidateExecuteRequest(reads); err == nil {
		t.Fatal("nil request accepted")
	}
	batch.Requests = []*pb.ReadRequest{read}
	for position := range 4 {
		copied := proto.Clone(reads).(*pb.ExecuteRequest)
		messages := []proto.Message{copied, copied.Command, copied.Command.GetRead(), copied.Command.GetRead().Requests[0]}
		messages[position].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatalf("unknown input fields accepted at level %d", position)
		}
	}
	if err := ValidateReadRequest(read); err != nil {
		t.Fatal("individual read rejected", err)
	}
	read.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if err := ValidateReadRequest(read); err == nil {
		t.Fatal("individual unknown fields accepted")
	}
	document := &pb.Document{MediaType: "application/json", Data: []byte(`{}`)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	mutations := mutationFrame("records", 1, []*pb.MutateRequest{mutation, mutation})
	if err := ValidateExecuteRequest(mutations); err != nil || mutation.Resource != "data/s:key" {
		t.Fatal("valid mutation changed or rejected", err)
	}
	if err := ValidateMutationRequest(mutation); err != nil {
		t.Fatal("individual mutation rejected", err)
	}
	missing := &pb.MutateRequest{Resource: "data/s:key"}
	mutations.Command.GetMutate().Requests = append(mutations.Command.GetMutate().Requests, missing)
	if err := ValidateExecuteRequest(mutations); err == nil {
		t.Fatal("late missing action accepted")
	}
}

func TestRecordFrameByteBoundIncludesAllCommandEnvelopes(t *testing.T) {
	document := &pb.Document{MediaType: "application/octet-stream", Data: make([]byte, MaxRecordFrameBytes/3)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	request := mutationFrame("records", 1, []*pb.MutateRequest{mutation, mutation, mutation})
	if proto.Size(request.Command) <= MaxRecordFrameBytes {
		t.Fatal("fixture does not include envelope overhead")
	}
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("document bytes fill limit but wrapper bytes were ignored")
	}
	request.Command.GetMutate().Requests = request.Command.GetMutate().Requests[:2]
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("bounded frame rejected", err)
	}
}

func TestMaximalSingleMutationFitsOneRecordFrame(t *testing.T) {
	media := strings.Repeat("a", 63) + "/" + strings.Repeat("b", 63)
	document := &pb.Document{MediaType: media, Data: make([]byte, MaxDocument)}
	options := proto.Clone(document).(*pb.Document)
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: strings.Repeat("x", MaxResourceBytes), AdapterOptions: options, Action: action}
	request := mutationFrame(strings.Repeat("a", 63), 1, []*pb.MutateRequest{mutation})
	if err := ValidateMutationRequest(mutation); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximal single mutation cannot fit a frame", err)
	}
	program := &pb.ProgramTransform{Runtime: "lua.v1", Source: []byte(strings.Repeat(" ", MaxExpression)), Input: document}
	form := &pb.Transform_Program{Program: program}
	transform := &pb.Transform{Form: form}
	transformAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation.Action = transformAction
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximal Lua mutation cannot fit a frame", err)
	}
}

func TestRecordFrameCountAndIndexesAreBoundedWithoutCallState(t *testing.T) {
	read := &pb.ReadRequest{Resource: "data/s:key"}
	requests := make([]*pb.ReadRequest, MaxRecordFrameItems)
	for index := range requests {
		requests[index] = read
	}
	request := readFrame("records", 1, requests)
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("bounded frame rejected", err)
	}
	request.Command.GetRead().Requests = append(requests, read)
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("excessive frame metadata count accepted")
	}
	request.Command.GetRead().Requests = requests
	for index := range 65 {
		request.Index = uint64(index*MaxRecordFrameItems + 1)
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal("valid frame rejected by a logical-call bound", err)
		}
	}
	for _, index := range []uint64{0, math.MaxUint64, math.MaxUint64 - MaxRecordFrameItems + 1} {
		request.Index = index
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("zero or overflowing index accepted", index)
		}
	}
	request.Index = math.MaxUint64 - MaxRecordFrameItems
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("last representable next-frame ordinal rejected", err)
	}
}

func TestRecordFramesHaveNoLogicalByteBudget(t *testing.T) {
	document := &pb.Document{MediaType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	request := mutationFrame("records", 1, []*pb.MutateRequest{mutation, mutation})
	totalBytes := 0
	for range 16 {
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal("independent bounded frame rejected", err)
		}
		totalBytes += proto.Size(request)
		request.Index += 2
	}
	if totalBytes <= 64<<20 {
		t.Fatal("fixture did not span a large logical call")
	}
}

func TestRecordResponsesRejectMalformedApplicationEvidence(t *testing.T) {
	missing := Missing()
	response := readResponse(1, missing)
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	missing.GetMissing().ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("nested unknown result field accepted")
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
