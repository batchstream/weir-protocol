package protocol

import pb "github.com/batchstream/weir-protocol/api/weir/v1"

const MaxNativeRequestBytes = 8 << 20
const MaxNativeMetadataBytes = 64 << 10

// ValidateNative checks a bounded request envelope without interpreting the
// adapter-owned content type or payload bytes.
func ValidateNative(request *pb.NativeRequest) *pb.Failure {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing Native request or unknown fields")
	}
	return validateNative(request)
}

func validateNative(request *pb.NativeRequest) *pb.Failure {
	if request == nil || !validRelativeResource(request.Resource) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native relative resource")
	}
	if !validDocument(request.Request, MaxNativeRequestBytes) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "Native requires a bounded request document")
	}
	return nil
}
