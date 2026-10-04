package protocol

import (
	"fmt"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

// Measure per-record request and response validation without I/O.
func BenchmarkReadProtocolPath(b *testing.B) {
	requests := make([]*pb.ExecuteRequest, 32)
	responses := make([]*pb.ExecuteResponse, len(requests))
	for index := range requests {
		resource := fmt.Sprintf("weirtest_012345678901234567890123/records/s:record-%08d", index)
		read := &pb.ReadRequest{Resource: resource}
		requests[index] = readExecution("mongo", uint64(index+1), read)
		document := &pb.Document{ContentType: "application/bson", Data: make([]byte, 1114)}
		responses[index] = readResponse(uint64(index+1), ReadDocument(document))
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, request := range requests {
			if err := ValidateExecuteRequest(request); err != nil {
				b.Fatal(err)
			}
		}
		for _, response := range responses {
			if err := ValidateExecuteResponse(response); err != nil {
				b.Fatal(err)
			}
		}
	}
}
