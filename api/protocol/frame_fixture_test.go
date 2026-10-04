package protocol

import pb "github.com/batchstream/weir-protocol/api/weir/v1"

func readFrame(store string, index uint64, requests []*pb.ReadRequest) *pb.ExecuteRequest {
	batch := &pb.ReadBatch{Requests: requests}
	operation := &pb.Command_Read{Read: batch}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: store, Index: index, Command: command}
	return request
}

func mutationFrame(store string, index uint64, requests []*pb.MutateRequest) *pb.ExecuteRequest {
	batch := &pb.MutationBatch{Requests: requests}
	operation := &pb.Command_Mutate{Mutate: batch}
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
