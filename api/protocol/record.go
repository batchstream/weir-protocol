package protocol

import (
	"fmt"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

// ValidateReadRequest validates one record before it is sent.
func ValidateReadRequest(request *pb.ReadRequest) error {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return fmt.Errorf("missing Read request or unknown fields")
	}
	return validateReadRequest(request)
}

func validateReadRequest(request *pb.ReadRequest) error {
	if request == nil || !validRelativeResource(request.Resource) {
		return fmt.Errorf("Read requires a canonical relative resource")
	}
	return nil
}

// ValidateMutationRequest validates one record before it is sent.
func ValidateMutationRequest(request *pb.MutateRequest) error {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return fmt.Errorf("missing mutation request or unknown fields")
	}
	return validateMutationRequest(request)
}

func validateMutationRequest(request *pb.MutateRequest) error {
	if request == nil || !validRelativeResource(request.Resource) {
		return fmt.Errorf("Mutate requires a canonical relative resource")
	}
	if failure := validateMutationFields(request); failure != nil {
		return fmt.Errorf("Mutate: %s", failure.Message)
	}
	return nil
}

// ValidateReadResult checks the required result variant and byte bound, ignoring
// additive ancillary fields.
func ValidateReadResult(result *pb.ReadResult) error {
	if result == nil || proto.Size(result) > MaxEvent {
		return fmt.Errorf("missing or oversized Read result")
	}
	valid := false
	switch value := result.Result.(type) {
	case *pb.ReadResult_Document:
		if value == nil {
			break
		}
		valid = validDocument(value.Document, MaxDocument)
	case *pb.ReadResult_Missing:
		if value == nil {
			break
		}
		valid = value.Missing != nil
	case *pb.ReadResult_Failure:
		if value == nil {
			break
		}
		valid = value.Failure != nil && validFailure(value.Failure)
	}
	if !valid {
		return fmt.Errorf("invalid Read result value")
	}
	return nil
}

// ValidateMutationResult checks fixed application evidence, allowing future
// positive failure codes and additive ancillary fields within the byte bound.
func ValidateMutationResult(result *pb.MutationResult) error {
	if result == nil || proto.Size(result) > MaxEvent || result.Outcome < pb.MutationOutcome_NOT_STARTED || result.Outcome > pb.MutationOutcome_UNKNOWN || !validFailure(result.Failure) {
		return fmt.Errorf("invalid mutation outcome or failure")
	}
	// Application evidence can coexist with a subsequent acknowledgement failure.
	if result.Outcome != pb.MutationOutcome_APPLIED && result.Failure == nil {
		return fmt.Errorf("unapplied mutation requires a failure")
	}
	return nil
}
