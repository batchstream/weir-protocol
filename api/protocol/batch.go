package protocol

import (
	"fmt"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

// ValidateReadRequest validates one record before it is placed in a frame.
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
	if failure := validateReadFields(request); failure != nil {
		return fmt.Errorf("Read: %s", failure.Message)
	}
	return nil
}

// ValidateMutationRequest validates one record before it is placed in a frame.
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

func ValidateReadResult(result *pb.ReadResult) error {
	if result == nil || hasUnknown(result.ProtoReflect()) {
		return fmt.Errorf("missing Read result or unknown fields")
	}
	return validateReadResult(result)
}

func validateReadResult(result *pb.ReadResult) error {
	if result == nil {
		return fmt.Errorf("missing Read result")
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
	if result == nil || hasUnknown(result.ProtoReflect()) {
		return fmt.Errorf("missing mutation result or unknown fields")
	}
	return validateMutationResult(result)
}

func validateMutationResult(result *pb.MutationResult) error {
	if result == nil || result.Outcome < pb.MutationOutcome_NOT_STARTED || result.Outcome > pb.MutationOutcome_UNKNOWN || !validFailure(result.Failure) {
		return fmt.Errorf("invalid mutation outcome or failure")
	}
	// Application evidence can coexist with a subsequent acknowledgement failure.
	if result.Outcome != pb.MutationOutcome_APPLIED && result.Failure == nil {
		return fmt.Errorf("unapplied mutation requires a failure")
	}
	return nil
}
