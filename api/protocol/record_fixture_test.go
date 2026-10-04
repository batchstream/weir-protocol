package protocol

import pb "github.com/batchstream/weir-protocol/api/weir/v1"

func readExecution(store string, index uint64, read *pb.ReadRequest) *pb.ExecuteRequest {
	operation := &pb.Command_Read{Read: read}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: store, Index: index, Command: command}
	return request
}

func mutationExecution(store string, index uint64, mutation *pb.MutateRequest) *pb.ExecuteRequest {
	operation := &pb.Command_Mutate{Mutate: mutation}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: store, Index: index, Command: command}
	return request
}

func readResponse(index uint64, result *pb.ReadResult) *pb.ExecuteResponse {
	value := &pb.Event_ReadResult{ReadResult: result}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: index, Event: event}
	return response
}

func mutationResponse(index uint64, result *pb.MutationResult) *pb.ExecuteResponse {
	value := &pb.Event_MutationResult{MutationResult: result}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: index, Event: event}
	return response
}
