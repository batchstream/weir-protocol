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
	if file.Path() != "api/weir/v1/store.proto" || file.Package() != "weir.v1" || file.Services().Len() != 1 {
		t.Fatal("public package or service contract changed", file.Path(), file.Package(), file.Services().Len())
	}
	options := file.Options().(*descriptorpb.FileOptions)
	if options.GetGoPackage() != "github.com/batchstream/weir-protocol/api/weir/v1;weirv1" {
		t.Fatal("public Go package does not belong to protocol module", options.GetGoPackage())
	}
	service := file.Services().Get(0)
	if service.FullName() != "weir.v1.StoreService" || service.Methods().Len() != 4 {
		t.Fatal("unexpected public service", service.FullName(), service.Methods().Len())
	}
	resolve := service.Methods().Get(0)
	read := service.Methods().Get(1)
	mutate := service.Methods().Get(2)
	execute := service.Methods().Get(3)
	if resolve.Name() != "ResolveStore" || resolve.Input().FullName() != "weir.v1.ResolveStoreRequest" || resolve.Output().FullName() != "weir.v1.ResolveStoreResponse" || resolve.IsStreamingClient() || resolve.IsStreamingServer() {
		t.Fatal("ResolveStore RPC contract changed", resolve)
	}
	if execute.Name() != "Execute" || execute.Input().FullName() != "weir.v1.ExecuteRequest" || execute.Output().FullName() != "weir.v1.ExecuteResponse" || execute.IsStreamingClient() || !execute.IsStreamingServer() {
		t.Fatal("Execute RPC contract changed", execute)
	}
	if StoreService_ResolveStore_FullMethodName != "/weir.v1.StoreService/ResolveStore" || StoreService_Execute_FullMethodName != "/weir.v1.StoreService/Execute" {
		t.Fatal("generated public method paths changed")
	}
	request := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}}
	assertFields(t, resolve.Input(), request)
	response := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "endpoints", kind: protoreflect.StringKind, repeated: true}, {name: "cache_ttl_ms", kind: protoreflect.Uint64Kind}}
	assertFields(t, resolve.Output(), response)
	executeRequest := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "command", kind: protoreflect.MessageKind, message: "weir.v1.Command"}}
	assertFields(t, execute.Input(), executeRequest)
	executeResponse := []fieldContract{{name: "event", kind: protoreflect.MessageKind, message: "weir.v1.Event"}}
	assertFields(t, execute.Output(), executeResponse)
	for _, method := range []protoreflect.MethodDescriptor{read, mutate} {
		if method.IsStreamingClient() || method.IsStreamingServer() {
			t.Fatal("record RPC must be unary", method)
		}
	}
	if read.Name() != "Read" || read.Input().FullName() != "weir.v1.ReadBatchRequest" || read.Output().FullName() != "weir.v1.ReadBatchResponse" || mutate.Name() != "Mutate" || mutate.Input().FullName() != "weir.v1.MutateBatchRequest" || mutate.Output().FullName() != "weir.v1.MutateBatchResponse" {
		t.Fatal("record RPC types changed")
	}
	readRequest := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "requests", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.ReadRequest"}}
	readResponse := []fieldContract{{name: "results", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.ReadResult"}}
	mutationRequest := []fieldContract{{name: "store_name", kind: protoreflect.StringKind}, {name: "requests", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.MutateRequest"}}
	mutationResponse := []fieldContract{{name: "results", kind: protoreflect.MessageKind, repeated: true, message: "weir.v1.MutationResult"}}
	assertFields(t, read.Input(), readRequest)
	assertFields(t, read.Output(), readResponse)
	assertFields(t, mutate.Input(), mutationRequest)
	assertFields(t, mutate.Output(), mutationResponse)
	if resolve.Output().Fields().ByName("group") != nil || resolve.Output().Fields().ByName("replica_group") != nil {
		t.Fatal("peer replica group leaked into public response")
	}
	for _, name := range []protoreflect.Name{"Weir", "Directory"} {
		if file.Services().ByName(name) != nil {
			t.Fatal("retired service remains public", name)
		}
	}
	for _, name := range []protoreflect.Name{"Call", "NativeCall", "NodeAnnouncement", "NodeAdvertisement", "SyncDirectoryRequest", "SyncDirectoryResponse", "ExchangeRequest", "ExchangeResponse", "ResolveRequest", "ResolveResponse", "RouteRequest", "RouteResponse"} {
		if file.Messages().ByName(name) != nil {
			t.Fatal("internal or retired message remains public", name)
		}
	}
}

func TestPublicCommandContract(t *testing.T) {
	file := File_api_weir_v1_store_proto
	command := file.Messages().ByName("Command")
	native := file.Messages().ByName("NativeRequest")
	if command == nil || native == nil {
		t.Fatal("business command messages missing")
	}
	fields := []fieldContract{
		{name: "version", kind: protoreflect.Uint32Kind, number: 1},
		{name: "scan", kind: protoreflect.MessageKind, number: 12, message: "weir.v1.ScanRequest"},
		{name: "native", kind: protoreflect.MessageKind, number: 13, message: "weir.v1.NativeRequest"},
	}
	assertFields(t, command, fields)
	if command.Oneofs().Len() != 1 || command.Oneofs().Get(0).Name() != "operation" || command.Oneofs().Get(0).Fields().Len() != 2 {
		t.Fatal("command operation union changed")
	}
	nativeFields := []fieldContract{{name: "open", kind: protoreflect.MessageKind, message: "weir.v1.NativeOpen"}, {name: "body", kind: protoreflect.BytesKind}}
	assertFields(t, native, nativeFields)
}

func TestPublicCommandWireEncoding(t *testing.T) {
	scan := &ScanRequest{Resource: "x"}
	scanOperation := &Command_Scan{Scan: scan}
	command := &Command{Version: 1, Operation: scanOperation}
	payload, err := proto.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := []byte{0x08, 0x01, 0x62, 0x03, 0x0a, 0x01, 'x'}
	if !bytes.Equal(payload, wantPayload) {
		t.Fatal("command wire changed", payload)
	}
	request := &ExecuteRequest{StoreName: "records", Command: command}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	wantRequest := []byte{0x0a, 0x07, 'r', 'e', 'c', 'o', 'r', 'd', 's', 0x12, 0x07, 0x08, 0x01, 0x62, 0x03, 0x0a, 0x01, 'x'}
	if !bytes.Equal(encoded, wantRequest) {
		t.Fatal("typed execution wire changed", encoded)
	}
	open := &NativeOpen{Resource: "x"}
	native := &NativeRequest{Open: open, Body: []byte{1}}
	nativeOperation := &Command_Native{Native: native}
	command.Operation = nativeOperation
	encoded, err = proto.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	wantNative := []byte{0x08, 0x01, 0x6a, 0x08, 0x0a, 0x03, 0x0a, 0x01, 'x', 0x12, 0x01, 0x01}
	if !bytes.Equal(encoded, wantNative) {
		t.Fatal("native request field numbers or encoding changed", encoded)
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
