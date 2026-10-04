package protocol

import (
	"fmt"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

// Measure public unary batch validation without I/O or database work.
func BenchmarkReadBatchProtocolPath(b *testing.B) {
	request, response := benchmarkReadBatch()
	b.ReportAllocs()
	for b.Loop() {
		if err := ValidateReadBatchRequest(request); err != nil {
			b.Fatal(err)
		}
		if err := ValidateReadBatchResponse(response, len(request.Requests)); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkReadBatch() (*pb.ReadBatchRequest, *pb.ReadBatchResponse) {
	request := &pb.ReadBatchRequest{StoreName: "mongo"}
	response := &pb.ReadBatchResponse{}
	for index := range 32 {
		resource := fmt.Sprintf("weirtest_012345678901234567890123/records/s:record-%08d", index)
		read := &pb.ReadRequest{Resource: resource}
		request.Requests = append(request.Requests, read)
		document := &pb.Document{MediaType: "application/bson", Data: make([]byte, 1114)}
		response.Results = append(response.Results, ReadDocument(document))

	}
	return request, response
}
