package protocol

import (
	"fmt"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

// Batch bounds count encoded envelopes, including every request/result wrapper.
// There is no independent request-count limit.
func ValidateReadBatchRequest(request *pb.ReadBatchRequest) error {
	if request == nil || !ValidStoreName(request.StoreName) || len(request.Requests) == 0 || proto.Size(request) > MaxBatchRequestBytes || hasUnknown(request.ProtoReflect()) {
		return fmt.Errorf("Read requires a valid Store and nonempty requests within the batch byte bound")
	}
	for index, item := range request.Requests {
		if item == nil || !validRelativeResource(item.Resource) {
			return fmt.Errorf("Read request %d requires a canonical relative resource", index)
		}
		read := &pb.ReadRequest{Resource: "weir://" + request.StoreName + "/" + item.Resource, ReadMediaType: item.ReadMediaType, AdapterOptions: item.AdapterOptions}
		variant := &pb.Operation_Read{Read: read}
		operation := &pb.Operation{Operation: variant}
		if failure := Validate(operation, request.StoreName); failure != nil {
			return fmt.Errorf("Read request %d: %s", index, failure.Message)
		}
	}
	return nil
}

func ValidateMutateBatchRequest(request *pb.MutateBatchRequest) error {
	if request == nil || !ValidStoreName(request.StoreName) || len(request.Requests) == 0 || proto.Size(request) > MaxBatchRequestBytes || hasUnknown(request.ProtoReflect()) {
		return fmt.Errorf("Mutate requires a valid Store and nonempty requests within the batch byte bound")
	}
	for index, item := range request.Requests {
		if item == nil || !validRelativeResource(item.Resource) {
			return fmt.Errorf("Mutate request %d requires a canonical relative resource", index)
		}
		mutation := &pb.MutateRequest{Resource: "weir://" + request.StoreName + "/" + item.Resource, AdapterOptions: item.AdapterOptions, Action: item.Action}
		variant := &pb.Operation_Mutate{Mutate: mutation}
		operation := &pb.Operation{Operation: variant}
		if failure := Validate(operation, request.StoreName); failure != nil {
			return fmt.Errorf("Mutate request %d: %s", index, failure.Message)
		}
	}
	return nil
}

func ValidateReadBatchResponse(response *pb.ReadBatchResponse, count int) error {
	if response == nil || len(response.Results) != count || proto.Size(response) > MaxBatchResponseBytes || hasUnknown(response.ProtoReflect()) {
		return fmt.Errorf("invalid Read response count, fields or byte bound")
	}
	for index, result := range response.Results {
		if err := ValidateReadResult(result); err != nil {
			return fmt.Errorf("Read result %d: %w", index, err)
		}
	}
	return nil
}

func ValidateMutateBatchResponse(response *pb.MutateBatchResponse, count int) error {
	if response == nil || len(response.Results) != count || proto.Size(response) > MaxBatchResponseBytes || hasUnknown(response.ProtoReflect()) {
		return fmt.Errorf("invalid Mutate response count, fields or byte bound")
	}
	for index, result := range response.Results {
		if err := ValidateMutationResult(result); err != nil {
			return fmt.Errorf("Mutate result %d: %w", index, err)
		}
	}
	return nil
}

func ValidateReadResult(result *pb.ReadResult) error {
	if result == nil || hasUnknown(result.ProtoReflect()) {
		return fmt.Errorf("missing Read result or unknown fields")
	}
	valid := false
	switch value := result.Result.(type) {
	case *pb.ReadResult_Document:
		valid = validDocument(value.Document, MaxDocument)
	case *pb.ReadResult_Missing:
		valid = value.Missing != nil
	case *pb.ReadResult_Failure:
		valid = value.Failure != nil && validFailure(value.Failure)
	}
	if !valid {
		return fmt.Errorf("invalid Read result value")
	}
	return nil
}

func ValidateMutationResult(result *pb.MutationResult) error {
	if result == nil || hasUnknown(result.ProtoReflect()) || result.Outcome < pb.MutationOutcome_NOT_STARTED || result.Outcome > pb.MutationOutcome_UNKNOWN || !validFailure(result.Failure) {
		return fmt.Errorf("invalid mutation outcome or failure")
	}
	// Application evidence can coexist with a subsequent acknowledgement failure.
	if result.Outcome != pb.MutationOutcome_APPLIED && result.Failure == nil {
		return fmt.Errorf("unapplied mutation requires a failure")
	}
	return nil
}
