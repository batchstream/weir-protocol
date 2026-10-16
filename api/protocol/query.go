package protocol

import pb "github.com/batchstream/weir-protocol/api/weir/v1"

const DefaultQueryPageSize = 100

// QueryPageSize resolves the default without imposing an application row limit.
func QueryPageSize(request *pb.QueryRequest) uint64 {
	if request.GetPageSize() == 0 {
		return DefaultQueryPageSize
	}
	return uint64(request.PageSize)
}

// QueryOffset computes the independent page's zero-based offset. Both wire
// fields are uint32, so their product is always representable as uint64.
// Adapters must reject offsets outside their backend's supported range.
func QueryOffset(request *pb.QueryRequest) uint64 {
	if request.GetPage() <= 1 {
		return 0
	}
	return uint64(request.Page-1) * QueryPageSize(request)
}

// ValidateQuery checks the shared query envelope. Adapters validate supported
// read-only native commands, backend options, and execution capacity.
func ValidateQuery(request *pb.QueryRequest) *pb.Failure {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing Query request or unknown fields")
	}
	return validateQuery(request)
}

func validateQuery(request *pb.QueryRequest) *pb.Failure {
	if failure := validateNative(request.Native); failure != nil {
		return failure
	}
	fields := make(map[string]struct{}, len(request.Sort))
	for _, field := range request.Sort {
		if field == nil || !validProjectionField(field.Field) {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Query sort field")
		}
		if _, exists := fields[field.Field]; exists {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "Query sort fields repeat")
		}
		fields[field.Field] = struct{}{}
	}
	return validateProjection(request.Projection)
}

// ValidateCount checks the shared count envelope without interpreting the
// adapter-owned query source or choosing an estimation strategy.
func ValidateCount(request *pb.CountRequest) *pb.Failure {
	if request == nil || hasUnknown(request.ProtoReflect()) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing Count request or unknown fields")
	}
	return validateCount(request)
}

func validateCount(request *pb.CountRequest) *pb.Failure {
	return validateNative(request.Native)
}
