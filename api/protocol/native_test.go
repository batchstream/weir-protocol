package protocol

import (
	"strings"
	"testing"

	searchpb "github.com/batchstream/weir-protocol/api/weir/search/v1"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func nativeHTTPExecution(http *searchpb.HttpRequest) *pb.ExecuteRequest {
	value := &pb.NativeRequest_SearchHttp{SearchHttp: http}
	native := &pb.NativeRequest{Resource: "records", Request: value}
	operation := &pb.Command_Native{Native: native}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "search", Index: 1, Command: command}
	return request
}

func TestNativeTypedHTTPBoundsAndFields(t *testing.T) {
	header := &searchpb.Header{Name: "x-example", Values: []string{"value"}}
	http := &searchpb.HttpRequest{Method: "POST", Path: "/_search", BodyContentType: "application/json", Body: []byte(`{}`), Headers: []*searchpb.Header{header}}
	request := nativeHTTPExecution(http)
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("typed HTTP request rejected", err)
	}
	for _, mutate := range []func(*searchpb.HttpRequest){
		func(r *searchpb.HttpRequest) { r.Method = "" },
		func(r *searchpb.HttpRequest) { r.Method = "GET\r\nInjected:" },
		func(r *searchpb.HttpRequest) { r.Path = "_search" },
		func(r *searchpb.HttpRequest) { r.Path = "/\n" },
		func(r *searchpb.HttpRequest) { r.Query = string([]byte{0xff}) },
		func(r *searchpb.HttpRequest) { r.BodyContentType = "" },
		func(r *searchpb.HttpRequest) { r.BodyContentType = "application/json; charset=utf-8" },
		func(r *searchpb.HttpRequest) { r.Body = make([]byte, MaxNativeBodyBytes+1) },
		func(r *searchpb.HttpRequest) { r.Headers[0].Name = "X-Example" },
		func(r *searchpb.HttpRequest) { r.Headers[0].Values = []string{"injected\r\nvalue"} },
		func(r *searchpb.HttpRequest) {
			r.Headers[0].Values = []string{strings.Repeat("x", MaxNativeHTTPMetadataBytes)}
		},
		func(r *searchpb.HttpRequest) { r.Headers[0] = nil },
		func(r *searchpb.HttpRequest) { r.Headers[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
	} {
		copied := proto.Clone(http).(*searchpb.HttpRequest)
		mutate(copied)
		invalid := nativeHTTPExecution(copied)
		if err := ValidateExecuteRequest(invalid); err == nil {
			t.Fatal("invalid typed HTTP request accepted", copied)
		}
		if failure := ValidateNative(invalid.Command.GetNative()); failure == nil {
			t.Fatal("invalid Native request accepted directly", copied)
		}
	}
}

func TestNativeMaximumBodyAndMetadataFitEnvelope(t *testing.T) {
	contentType := strings.Repeat("a", 63) + "/" + strings.Repeat("b", 63)
	header := &searchpb.Header{Name: "x-example", Values: []string{strings.Repeat("x", 60<<10)}}
	http := &searchpb.HttpRequest{Method: "POST", Path: "/_search", Headers: []*searchpb.Header{header}, BodyContentType: contentType, Body: make([]byte, MaxNativeBodyBytes)}
	request := nativeHTTPExecution(http)
	request.Command.GetNative().Resource = strings.Repeat("x", MaxResourceBytes)
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("legal body and metadata rejected together", err)
	}
	body := &pb.NativeRequest_MongodbCommand{MongodbCommand: make([]byte, MaxNativeBodyBytes)}
	request.Command.GetNative().Request = body
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximum MongoDB command rejected", err)
	}
	body.MongodbCommand = nil
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("empty MongoDB command accepted")
	}
}

func TestNativeHTTPResponseRequiresValidStatusAndHeaders(t *testing.T) {
	for _, code := range []uint32{200, 404, 500} {
		http := &searchpb.HttpResponse{StatusCode: code}
		head := &pb.NativeHead{Http: http}
		value := &pb.Event_Head{Head: head}
		event := &pb.Event{Value: value}
		if err := ValidateEvent(event); err != nil {
			t.Fatal("normal or backend-error HTTP response rejected", err)
		}
	}
	for _, code := range []uint32{0, 99, 600} {
		http := &searchpb.HttpResponse{StatusCode: code}
		head := &pb.NativeHead{Http: http}
		value := &pb.Event_Head{Head: head}
		event := &pb.Event{Value: value}
		if err := ValidateEvent(event); err == nil {
			t.Fatal("invalid HTTP status accepted", code)
		}
	}
}
