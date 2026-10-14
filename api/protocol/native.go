package protocol

import pb "github.com/batchstream/weir-protocol/api/weir/v1"

// ValidateNative checks a request envelope without interpreting the
// adapter-owned content type or payload bytes.
func ValidateNative(request *pb.NativeRequest) *pb.Failure {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing Native request or unknown fields")
	}
	return validateNative(request)
}

func validateNative(request *pb.NativeRequest) *pb.Failure {
	if request == nil || request.Resource != "" && !validRelativeResource(request.Resource) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native relative resource")
	}
	if !validDocument(request.Request) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "Native requires a request document")
	}
	return nil
}
