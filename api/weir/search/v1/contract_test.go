package searchv1

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestPublicSearchDescriptorContract(t *testing.T) {
	file := File_api_weir_search_v1_http_proto
	if file.Path() != "api/weir/search/v1/http.proto" || file.Package() != "weir.search.v1" || file.Imports().Len() != 0 || file.Services().Len() != 0 || file.Messages().Len() != 3 {
		t.Fatal("public Search descriptor contract changed", file.Path(), file.Package())
	}
	options := file.Options().(*descriptorpb.FileOptions)
	if options.GetGoPackage() != "github.com/batchstream/weir-protocol/api/weir/search/v1;searchv1" {
		t.Fatal("Search Go package does not belong to protocol module", options.GetGoPackage())
	}
	cases := []struct {
		name   protoreflect.Name
		fields []protoreflect.Name
	}{
		{name: "Header", fields: []protoreflect.Name{"name", "values"}},
		{name: "Request", fields: []protoreflect.Name{"method", "path", "query", "headers"}},
		{name: "Response", fields: []protoreflect.Name{"status_code", "headers"}},
	}
	for _, expected := range cases {
		message := file.Messages().ByName(expected.name)
		if message == nil || message.Fields().Len() != len(expected.fields) {
			t.Fatal("Search message changed", expected.name)
		}
		for i, name := range expected.fields {
			field := message.Fields().Get(i)
			if field.Name() != name || field.Number() != protoreflect.FieldNumber(i+1) {
				t.Fatal("Search field changed", field)
			}
			switch name {
			case "headers":
				if !field.IsList() || field.Kind() != protoreflect.MessageKind || field.Message().FullName() != "weir.search.v1.Header" {
					t.Fatal("Search headers wire type changed", field)
				}
			case "status_code":
				if field.IsList() || field.Kind() != protoreflect.Uint32Kind {
					t.Fatal("Search status wire type changed", field)
				}
			default:
				if field.Kind() != protoreflect.StringKind || field.IsList() != (name == "values") {
					t.Fatal("Search string wire type changed", field)
				}
			}
		}
	}
}
