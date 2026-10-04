package protocol

import (
	"fmt"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

// Measure frame validation and per-record response validation without I/O.
func BenchmarkReadFrameProtocolPath(b *testing.B) {
	requests := make([]*pb.ReadRequest, 32)
	responses := make([]*pb.ExecuteResponse, len(requests))
	for index := range requests {
		resource := fmt.Sprintf("weirtest_012345678901234567890123/records/s:record-%08d", index)
		requests[index] = &pb.ReadRequest{Resource: resource}
		document := &pb.Document{MediaType: "application/bson", Data: make([]byte, 1114)}
		responses[index] = readResponse(uint64(index+1), ReadDocument(document))
	}
	request := readFrame("mongo", 1, requests)
	b.ReportAllocs()
	for b.Loop() {
		if err := ValidateExecuteRequest(request); err != nil {
			b.Fatal(err)
		}
		for _, response := range responses {
			if err := ValidateExecuteResponse(response); err != nil {
				b.Fatal(err)
			}
		}
	}
}
