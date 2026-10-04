package protocol

import (
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func nativeExecution(document *pb.Document) *pb.ExecuteRequest {
	native := &pb.NativeRequest{Resource: "records", Request: document}
	operation := &pb.Command_Native{Native: native}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "example", Index: 1, Command: command}
	return request
}

func TestNativeAcceptsOpaqueStoreOwnedRequests(t *testing.T) {
	for _, document := range []*pb.Document{
		{ContentType: "application/vnd.example.command", Data: []byte{0xff, 0, 1}},
		{ContentType: "application/vnd.another.command"},
		{ContentType: "application/http", Data: []byte("adapter validates these bytes")},
	} {
		request := nativeExecution(document)
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal("opaque request rejected by public envelope", document, err)
		}
		if failure := ValidateNative(request.Command.GetNative()); failure != nil {
			t.Fatal("opaque request rejected directly", document, failure)
		}
	}
}

func TestNativeRequestBoundsAndUnknownFields(t *testing.T) {
	document := &pb.Document{ContentType: "application/vnd.example.command", Data: []byte{1}}
	for _, mutate := range []func(*pb.NativeRequest){
		func(r *pb.NativeRequest) { r.Request = nil },
		func(r *pb.NativeRequest) { r.Request.ContentType = "" },
		func(r *pb.NativeRequest) { r.Request.ContentType = "application/json; charset=utf-8" },
		func(r *pb.NativeRequest) { r.Request.Data = make([]byte, MaxNativeRequestBytes+1) },
		func(r *pb.NativeRequest) { r.Request.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.NativeRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
	} {
		request := nativeExecution(document)
		copied := proto.Clone(request).(*pb.ExecuteRequest)
		mutate(copied.Command.GetNative())
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatal("invalid Native envelope accepted", copied)
		}
		if failure := ValidateNative(copied.Command.GetNative()); failure == nil {
			t.Fatal("invalid Native request accepted directly", copied)
		}
	}
}

func TestNativeMaximumPayloadFitsEnvelope(t *testing.T) {
	contentType := "application/" + strings.Repeat("x", 115)
	document := &pb.Document{ContentType: contentType, Data: make([]byte, MaxNativeRequestBytes)}
	request := nativeExecution(document)
	request.Command.GetNative().Resource = strings.Repeat("x", MaxResourceBytes)
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("maximal Native request rejected", err)
	}
}

func TestNativeResponseMetadataIsOpaqueAndBounded(t *testing.T) {
	for _, metadata := range []*pb.Document{
		nil,
		{ContentType: "application/vnd.example.response"},
		{ContentType: "application/vnd.another.response", Data: []byte{0xff, 0}},
		{ContentType: "application/http", Data: make([]byte, MaxNativeMetadataBytes)},
	} {
		head := &pb.NativeHead{Metadata: metadata, BodyContentType: "application/octet-stream"}
		value := &pb.Event_Head{Head: head}
		event := &pb.Event{Value: value}
		if err := ValidateEvent(event); err != nil {
			t.Fatal("opaque response metadata rejected", metadata, err)
		}
	}
	for _, metadata := range []*pb.Document{
		{},
		{ContentType: "invalid"},
		{ContentType: "application/vnd.example.response", Data: make([]byte, MaxNativeMetadataBytes+1)},
	} {
		head := &pb.NativeHead{Metadata: metadata}
		value := &pb.Event_Head{Head: head}
		event := &pb.Event{Value: value}
		if err := ValidateEvent(event); err == nil {
			t.Fatal("invalid Native metadata accepted", metadata)
		}
	}
	metadata := &pb.Document{ContentType: "application/vnd.example.response"}
	metadata.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	head := &pb.NativeHead{Metadata: metadata}
	value := &pb.Event_Head{Head: head}
	event := &pb.Event{Value: value}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("unknown Native metadata fields accepted")
	}
}
