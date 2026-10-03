package protocol

import (
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestBatchValidatesAllRequestsWithoutChangingResources(t *testing.T) {
	read := &pb.ReadRequest{Resource: "data/s:key"}
	reads := &pb.ReadBatchRequest{StoreName: "records", Requests: []*pb.ReadRequest{read, read}}
	if err := ValidateReadBatchRequest(reads); err != nil || read.Resource != "data/s:key" {
		t.Fatal("valid input changed or rejected", err, read)
	}
	invalid := &pb.ReadRequest{Resource: "weir://other/data/s:key"}
	reads.Requests = append(reads.Requests, invalid)
	if err := ValidateReadBatchRequest(reads); err == nil {
		t.Fatal("late invalid resource accepted")
	}
	reads.Requests = []*pb.ReadRequest{read, nil}
	if err := ValidateReadBatchRequest(reads); err == nil {
		t.Fatal("nil request accepted")
	}
	reads.Requests = []*pb.ReadRequest{read}
	read.ProtoReflect().SetUnknown([]byte{0x20, 1})
	if err := ValidateReadBatchRequest(reads); err == nil {
		t.Fatal("nested unknown fields accepted")
	}
	document := &pb.Document{MediaType: "application/json", Data: []byte(`{}`)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	mutations := &pb.MutateBatchRequest{StoreName: "records", Requests: []*pb.MutateRequest{mutation, mutation}}
	if err := ValidateMutateBatchRequest(mutations); err != nil || mutation.Resource != "data/s:key" {
		t.Fatal("valid mutation changed or rejected", err)
	}
	missing := &pb.MutateRequest{Resource: "data/s:key"}
	mutations.Requests = append(mutations.Requests, missing)
	if err := ValidateMutateBatchRequest(mutations); err == nil {
		t.Fatal("late missing action accepted")
	}
}

func TestBatchByteBoundsIncludeProtobufEnvelopes(t *testing.T) {
	document := &pb.Document{MediaType: "application/octet-stream", Data: make([]byte, MaxDocument)}
	action := &pb.MutateRequest_Put{Put: document}
	mutation := &pb.MutateRequest{Resource: "data/s:key", Action: action}
	mutations := &pb.MutateBatchRequest{StoreName: "records"}
	response := &pb.ReadBatchResponse{}
	for range MaxBatchRequestBytes / MaxDocument {
		mutations.Requests = append(mutations.Requests, mutation)
		response.Results = append(response.Results, ReadDocument(document))
	}
	if proto.Size(mutations) <= MaxBatchRequestBytes || proto.Size(response) <= MaxBatchResponseBytes {
		t.Fatal("fixture does not include envelope overhead")
	}
	if err := ValidateMutateBatchRequest(mutations); err == nil {
		t.Fatal("document bytes fill limit but wrapper bytes were ignored")
	}
	if err := ValidateReadBatchResponse(response, len(response.Results)); err == nil {
		t.Fatal("result wrapper bytes were ignored")
	}
	mutations.Requests = mutations.Requests[:len(mutations.Requests)-1]
	response.Results = response.Results[:len(response.Results)-1]
	if err := ValidateMutateBatchRequest(mutations); err != nil {
		t.Fatal("bounded input rejected", err)
	}
	if err := ValidateReadBatchResponse(response, len(response.Results)); err != nil {
		t.Fatal("bounded response rejected", err)
	}
}

func TestBatchHasNoArbitraryRequestCountLimit(t *testing.T) {
	request := &pb.ReadBatchRequest{StoreName: "records"}
	for range 513 {
		read := &pb.ReadRequest{Resource: "data/s:key"}
		request.Requests = append(request.Requests, read)
	}
	if err := ValidateReadBatchRequest(request); err != nil {
		t.Fatal("byte-bounded batch rejected for item count", err)
	}
}

func TestBatchResponsesRejectMissingMalformedAndUnknownEvidence(t *testing.T) {
	missing := Missing()
	reads := &pb.ReadBatchResponse{Results: []*pb.ReadResult{missing}}
	if err := ValidateReadBatchResponse(reads, 1); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadBatchResponse(reads, 2); err == nil {
		t.Fatal("incomplete result vector accepted")
	}
	missing.GetMissing().ProtoReflect().SetUnknown([]byte{0x08, 1})
	if err := ValidateReadBatchResponse(reads, 1); err == nil {
		t.Fatal("nested unknown result field accepted")
	}
	reads.Results[0] = nil
	if err := ValidateReadBatchResponse(reads, 1); err == nil {
		t.Fatal("nil result accepted")
	}
	for _, outcome := range []pb.MutationOutcome{pb.MutationOutcome_NOT_STARTED, pb.MutationOutcome_NOT_APPLIED, pb.MutationOutcome_UNKNOWN, pb.MutationOutcome_MUTATION_OUTCOME_UNSPECIFIED, pb.MutationOutcome(99)} {
		mutation := &pb.MutationResult{Outcome: outcome}
		response := &pb.MutateBatchResponse{Results: []*pb.MutationResult{mutation}}
		if err := ValidateMutateBatchResponse(response, 1); err == nil {
			t.Fatal("invalid or unqualified outcome accepted", outcome)
		}
	}
	failure := Fail(pb.FailureCode_UNAVAILABLE, "acknowledgement failed")
	applied := Mutation(pb.MutationOutcome_APPLIED, failure)
	mutations := &pb.MutateBatchResponse{Results: []*pb.MutationResult{applied}}
	if err := ValidateMutateBatchResponse(mutations, 1); err != nil {
		t.Fatal("APPLIED with acknowledgement failure rejected", err)
	}
}
