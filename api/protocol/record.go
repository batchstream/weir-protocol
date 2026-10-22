package protocol

import (
	"fmt"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
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
	visibility := request.GetOptions().GetVisibility()
	if visibility != pb.MutationVisibility_APPLICATION_ACKNOWLEDGED && visibility != pb.MutationVisibility_QUERY_VISIBLE {
		return fmt.Errorf("Mutate requires a declared visibility option")
	}
	if failure := validateMutationFields(request); failure != nil {
		return fmt.Errorf("Mutate: %s", failure.Message)
	}
	return nil
}

// ValidateReadResult checks the required result variant and known fields.
func ValidateReadResult(result *pb.ReadResult) error {
	if result == nil || hasUnknown(result.ProtoReflect()) {
		return fmt.Errorf("missing Read result")
	}
	valid := false
	switch value := result.Result.(type) {
	case *pb.ReadResult_Document:
		if value == nil {
			break
		}
		valid = validDocument(value.Document)
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

// ValidateMutationResult checks the outcome, its failure code and known fields.
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
