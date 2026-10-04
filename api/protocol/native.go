package protocol

import (
	"strings"
	"unicode/utf8"

	searchpb "github.com/batchstream/weir-protocol/api/weir/search/v1"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

const MaxNativeBodyBytes = 8 << 20
const MaxNativeHTTPMetadataBytes = 64 << 10

// ValidateNative validates an explicit backend request without interpreting BSON
// commands or the adapter's allowed HTTP operations.
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
	switch value := request.Request.(type) {
	case *pb.NativeRequest_MongodbCommand:
		if value == nil || len(value.MongodbCommand) == 0 || len(value.MongodbCommand) > MaxNativeBodyBytes {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "Native requires a bounded nonempty MongoDB command")
		}
	case *pb.NativeRequest_SearchHttp:
		if value == nil || !validHTTPRequest(value.SearchHttp) {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native HTTP request or byte bound")
		}
	default:
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "Native requires one backend request")
	}
	return nil
}

func validHTTPRequest(request *searchpb.HttpRequest) bool {
	if request == nil || len(request.Body) > MaxNativeBodyBytes || !validHTTPToken(request.Method) || !strings.HasPrefix(request.Path, "/") || len(request.Path) > MaxResourceBytes || len(request.Query) > MaxResourceBytes || !validHTTPText(request.Path) || !validHTTPText(request.Query) {
		return false
	}
	if request.BodyContentType != "" && !validMedia(request.BodyContentType) || len(request.Body) > 0 && request.BodyContentType == "" {
		return false
	}
	metadata := &searchpb.HttpRequest{Method: request.Method, Path: request.Path, Query: request.Query, Headers: request.Headers, BodyContentType: request.BodyContentType}
	return proto.Size(metadata) <= MaxNativeHTTPMetadataBytes && validHTTPHeaders(request.Headers)
}

func validHTTPResponse(response *searchpb.HttpResponse) bool {
	return response == nil || response.StatusCode >= 100 && response.StatusCode <= 599 && proto.Size(response) <= MaxNativeHTTPMetadataBytes && validHTTPHeaders(response.Headers)
}

func validHTTPHeaders(headers []*searchpb.Header) bool {
	for _, header := range headers {
		if header == nil || header.Name != strings.ToLower(header.Name) || !validHTTPToken(header.Name) {
			return false
		}
		for _, value := range header.Values {
			if !validHTTPText(value) {
				return false
			}
		}
	}
	return true
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func validHTTPText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 32 && character != '\t' || character == 127 {
			return false
		}
	}
	return true
}
