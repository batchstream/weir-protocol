package protocol

import (
	"fmt"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

// This follows the shared validation calls made by the unary Read handler and
// the backend record preparation, excluding I/O and database work.
func BenchmarkReadBatchProtocolPath(b *testing.B) {
	request, response, operations := benchmarkReadBatch()
	b.ReportAllocs()
	for b.Loop() {
		if err := ValidateReadBatchRequest(request); err != nil {
			b.Fatal(err)
		}
		for _, operation := range operations {
			if failure := Validate(operation, request.StoreName); failure != nil {
				b.Fatal(failure)
			}
			if _, _, err := ParseResource(operation.GetRead().Resource); err != nil {
				b.Fatal(err)
			}
		}
		if err := ValidateReadBatchResponse(response, len(request.Requests)); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkReadBatch() (*pb.ReadBatchRequest, *pb.ReadBatchResponse, []*pb.Operation) {
	request := &pb.ReadBatchRequest{StoreName: "mongo"}
	response := &pb.ReadBatchResponse{}
	var operations []*pb.Operation
	for index := range 32 {
		resource := fmt.Sprintf("weirtest_012345678901234567890123/records/s:record-%08d", index)
		read := &pb.ReadRequest{Resource: resource}
		request.Requests = append(request.Requests, read)
		document := &pb.Document{MediaType: "application/bson", Data: make([]byte, 1114)}
		response.Results = append(response.Results, ReadDocument(document))
		fullRead := &pb.ReadRequest{Resource: "weir://mongo/" + resource}
		variant := &pb.Operation_Read{Read: fullRead}
		operation := &pb.Operation{Index: uint64(index + 1), Operation: variant}
		operations = append(operations, operation)
	}
	return request, response, operations
}
