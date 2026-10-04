package weirv1

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type fieldContract struct {
	name     protoreflect.Name
	kind     protoreflect.Kind
	repeated bool
	number   protoreflect.FieldNumber
	message  protoreflect.FullName
}

func assertFields(t *testing.T, message protoreflect.MessageDescriptor, expected []fieldContract) {
	t.Helper()
	fields := message.Fields()
	if fields.Len() != len(expected) {
		t.Fatalf("%s has %d fields, want %d", message.FullName(), fields.Len(), len(expected))
	}
	for i, contract := range expected {
		field := fields.Get(i)
		number := contract.number
		if number == 0 {
			number = protoreflect.FieldNumber(i + 1)
		}
		if field.Name() != contract.name || field.Number() != number || field.Kind() != contract.kind || field.IsList() != contract.repeated {
			t.Fatalf("%s field %d changed: %s number=%d kind=%s list=%v", message.FullName(), i, field.Name(), field.Number(), field.Kind(), field.IsList())
		}
		if contract.message != "" && (field.Message() == nil || field.Message().FullName() != contract.message) {
			t.Fatalf("%s field %s has unexpected message type", message.FullName(), field.Name())
		}
	}
}

func TestPublicStoreContract(t *testing.T) {
	file := File_api_weir_v1_store_proto
	if file.Path() != "api/weir/v1/store.proto" || file.Package() != "weir.v1" {
		t.Fatal("public package contract changed", file.Path(), file.Package())
	}
	options := file.Options().(*descriptorpb.FileOptions)
	if options.GetGoPackage() != "github.com/batchstream/weir-protocol/api/weir/v1;weirv1" {
		t.Fatal("public Go package does not belong to protocol module", options.GetGoPackage())
	}
	service := file.Services().ByName("StoreService")
	if service == nil {
		t.Fatal("public Store service missing")
	}
	resolve := service.Methods().ByName("ResolveStore")
	execute := service.Methods().ByName("Execute")
	if resolve == nil || resolve.Input().FullName() != "weir.v1.ResolveStoreRequest" || resolve.Output().FullName() != "weir.v1.ResolveStoreResponse" || resolve.IsStreamingClient() || resolve.IsStreamingServer() {
		t.Fatal("ResolveStore unary contract changed", resolve)
	}
	if execute == nil || execute.Input().FullName() != "weir.v1.ExecuteRequest" || execute.Output().FullName() != "weir.v1.ExecuteResponse" || !execute.IsStreamingClient() || !execute.IsStreamingServer() {
		t.Fatal("Execute bidirectional contract changed", execute)
	}
	if StoreService_ResolveStore_FullMethodName != "/weir.v1.StoreService/ResolveStore" || StoreService_Execute_FullMethodName != "/weir.v1.StoreService/Execute" {
		t.Fatal("generated public method paths changed")
	}
	request := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}}
	assertFields(t, resolve.Input(), request)
	response := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "endpoints", kind: protoreflect.StringKind, repeated: true}, {name: "cache_ttl_ms", kind: protoreflect.Uint64Kind}}
	assertFields(t, resolve.Output(), response)
	executeRequest := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "index", kind: protoreflect.Uint64Kind}, {name: "command", kind: protoreflect.MessageKind, message: "weir.v1.Command"}}
	assertFields(t, execute.Input(), executeRequest)
	executeResponse := []fieldContract{{name: "index", kind: protoreflect.Uint64Kind}, {name: "event", kind: protoreflect.MessageKind, message: "weir.v1.Event"}}
	assertFields(t, execute.Output(), executeResponse)
}

func TestPublicCommandAndEventContract(t *testing.T) {
	messages := File_api_weir_v1_store_proto.Messages()
	command := messages.ByName("Command")
	fields := []fieldContract{
		{name: "read", kind: protoreflect.MessageKind, message: "weir.v1.ReadBatch"},
		{name: "mutate", kind: protoreflect.MessageKind, message: "weir.v1.MutationBatch"},
		{name: "scan", kind: protoreflect.MessageKind, message: "weir.v1.ScanRequest"},
		{name: "native", kind: protoreflect.MessageKind, message: "weir.v1.NativeRequest"},
	}
	assertFields(t, command, fields)
	if command.Oneofs().Len() != 1 || command.Oneofs().Get(0).Name() != "operation" {
		t.Fatal("command operation union changed")
	}
	reads := []fieldContract{{name: "requests", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.ReadRequest"}}
	mutations := []fieldContract{{name: "requests", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.MutateRequest"}}
	assertFields(t, messages.ByName("ReadBatch"), reads)
	assertFields(t, messages.ByName("MutationBatch"), mutations)
	events := []fieldContract{
		{name: "read_result", kind: protoreflect.MessageKind, message: "weir.v1.ReadResult"},
		{name: "mutation_result", kind: protoreflect.MessageKind, message: "weir.v1.MutationResult"},
		{name: "document", kind: protoreflect.MessageKind, message: "weir.v1.Document"},
		{name: "head", kind: protoreflect.MessageKind, message: "weir.v1.NativeHead"},
		{name: "chunk", kind: protoreflect.BytesKind},
		{name: "scan_end", kind: protoreflect.MessageKind, message: "weir.v1.ScanEnd"},
		{name: "native_end", kind: protoreflect.MessageKind, message: "weir.v1.NativeEnd"},
	}
	assertFields(t, messages.ByName("Event"), events)
}

func TestIndexedExecutionWireEncoding(t *testing.T) {
	scan := &ScanRequest{Resource: "x"}
	operation := &Command_Scan{Scan: scan}
	command := &Command{Operation: operation}
	request := &ExecuteRequest{StoreName: "records", Index: 1, Command: command}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest := []byte{0x0a, 0x07, 'r', 'e', 'c', 'o', 'r', 'd', 's', 0x10, 1, 0x1a, 0x05, 0x1a, 0x03, 0x0a, 0x01, 'x'}
	if !bytes.Equal(encoded, wantRequest) {
		t.Fatal("indexed execution wire changed", encoded)
	}
	empty := &Empty{}
	missing := &ReadResult_Missing{Missing: empty}
	result := &ReadResult{Result: missing}
	value := &Event_ReadResult{ReadResult: result}
	event := &Event{Value: value}
	response := &ExecuteResponse{Index: 3, Event: event}
	encoded, err = proto.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	wantResponse := []byte{0x08, 3, 0x12, 4, 0x0a, 2, 0x12, 0}
	if !bytes.Equal(encoded, wantResponse) {
		t.Fatal("indexed per-record result wire changed", encoded)
	}
}

func TestPublicDescriptorClosureExcludesPeers(t *testing.T) {
	pending := []protoreflect.FileDescriptor{File_api_weir_v1_store_proto}
	seen := make(map[string]bool)
	for len(pending) > 0 {
		file := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[file.Path()] {
			continue
		}
		seen[file.Path()] = true
		if strings.HasPrefix(string(file.Package()), "weir.peer") || strings.HasPrefix(file.Path(), "internal/") {
			t.Fatal("public descriptor imports internal peer protocol", file.Path(), file.Package())
		}
		imports := file.Imports()
		for i := 0; i < imports.Len(); i++ {
			pending = append(pending, imports.Get(i).FileDescriptor)
		}
	}
}
