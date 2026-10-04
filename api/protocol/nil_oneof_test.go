package protocol

import (
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

func TestTypedNilOneofsRejectWithoutPanicking(t *testing.T) {
	mutations := []*pb.MutateRequest{
		{Resource: "records/s:key", Action: (*pb.MutateRequest_Put)(nil)},
		{Resource: "records/s:key", Action: (*pb.MutateRequest_Create)(nil)},
		{Resource: "records/s:key", Action: (*pb.MutateRequest_Replace)(nil)},
		{Resource: "records/s:key", Action: (*pb.MutateRequest_Delete)(nil)},
		{Resource: "records/s:key", Action: (*pb.MutateRequest_AtomicTransform)(nil)},
	}
	transforms := []*pb.Transform{
		{Form: (*pb.Transform_Lua)(nil)},
		{Form: (*pb.Transform_BackendExpression)(nil)},
	}
	for _, transform := range transforms {
		action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
		mutation := &pb.MutateRequest{Resource: "records/s:key", Action: action}
		mutations = append(mutations, mutation)
	}
	for _, mutation := range mutations {
		if err := ValidateMutationRequest(mutation); err == nil {
			t.Fatal("typed nil mutation accepted")
		}
		request := mutationExecution("records", 1, mutation)
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("typed nil mutation envelope accepted")
		}
	}
	events := []*pb.Event{
		{Value: (*pb.Event_ReadResult)(nil)},
		{Value: (*pb.Event_MutationResult)(nil)},
		{Value: (*pb.Event_Document)(nil)},
		{Value: (*pb.Event_Head)(nil)},
		{Value: (*pb.Event_Chunk)(nil)},
		{Value: (*pb.Event_ScanEnd)(nil)},
		{Value: (*pb.Event_NativeEnd)(nil)},
	}
	reads := []*pb.ReadResult{
		{Result: (*pb.ReadResult_Document)(nil)},
		{Result: (*pb.ReadResult_Missing)(nil)},
		{Result: (*pb.ReadResult_Failure)(nil)},
	}
	for _, result := range reads {
		response := readResponse(1, result)
		events = append(events, response.Event)
	}
	for _, event := range events {
		if err := ValidateEvent(event); err == nil {
			t.Fatal("typed nil event accepted")
		}
		response := &pb.ExecuteResponse{Index: 1, Event: event}
		if err := ValidateExecuteResponse(response); err == nil {
			t.Fatal("typed nil event envelope accepted")
		}
	}
	commands := []*pb.Command{
		{Operation: (*pb.Command_Read)(nil)},
		{Operation: (*pb.Command_Mutate)(nil)},
		{Operation: (*pb.Command_Scan)(nil)},
		{Operation: (*pb.Command_Native)(nil)},
	}
	for _, command := range commands {
		if err := ValidateCommand(command); err == nil {
			t.Fatal("typed nil command accepted")
		}
		request := &pb.ExecuteRequest{StoreName: "records", Index: 1, Command: command}
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("typed nil command envelope accepted")
		}
	}
}

func TestNativeRequiresRequestDocument(t *testing.T) {
	request := &pb.NativeRequest{Resource: "records"}
	if failure := ValidateNative(request); failure == nil {
		t.Fatal("missing Native request document accepted")
	}
	operation := &pb.Command_Native{Native: request}
	command := &pb.Command{Operation: operation}
	if err := ValidateCommand(command); err == nil {
		t.Fatal("missing Native command document accepted")
	}
}
