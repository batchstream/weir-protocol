package protocol

import (
	"math"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func querySource() *pb.NativeRequest {
	document := &pb.Document{ContentType: "application/vnd.example.query", Data: []byte{0xff, 0, 1}}
	request := &pb.NativeRequest{Resource: "records", Request: document}
	return request
}

func queryExecution(query *pb.QueryRequest) *pb.ExecuteRequest {
	operation := &pb.Command_Query{Query: query}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "example", Index: 1, Command: command}
	return request
}

func countExecution(count *pb.CountRequest) *pb.ExecuteRequest {
	operation := &pb.Command_Count{Count: count}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "example", Index: 1, Command: command}
	return request
}

func TestQueryPageDefaultsAndLargeOffsets(t *testing.T) {
	tests := []struct {
		page   uint32
		size   uint32
		want   uint64
		offset uint64
	}{
		{want: 100},
		{page: 1, size: 25, want: 25},
		{page: 2, want: 100, offset: 100},
		{page: 3, size: 17, want: 17, offset: 34},
		{page: math.MaxUint32, size: math.MaxUint32, want: math.MaxUint32, offset: uint64(math.MaxUint32-1) * math.MaxUint32},
	}
	for _, test := range tests {
		request := &pb.QueryRequest{Page: test.page, PageSize: test.size}
		if QueryPageSize(request) != test.want || QueryOffset(request) != test.offset {
			t.Fatal("incorrect query page arithmetic", test, QueryPageSize(request), QueryOffset(request))
		}
	}
	if QueryPageSize(nil) != 100 || QueryOffset(nil) != 0 {
		t.Fatal("nil query must use defaults")
	}
}

func TestQueryCountKeepNativeFormatsOpaqueAndUseSingleIndex(t *testing.T) {
	native := querySource()
	query := &pb.QueryRequest{Native: native, Page: math.MaxUint32, PageSize: math.MaxUint32}
	count := &pb.CountRequest{Native: native}
	if failure := ValidateQuery(query); failure != nil {
		t.Fatal("opaque query or unbounded page rejected", failure)
	}
	if failure := ValidateCount(count); failure != nil {
		t.Fatal("opaque count rejected", failure)
	}
	for _, request := range []*pb.ExecuteRequest{queryExecution(query), countExecution(count)} {
		if err := ValidateExecuteRequest(request); err != nil {
			t.Fatal("valid query/count request rejected", err)
		}
		for _, index := range []uint64{0, 2, math.MaxUint64} {
			request.Index = index
			if err := ValidateExecuteRequest(request); err == nil {
				t.Fatal("query/count accepted non-singleton index", index)
			}
		}
	}
	// A backend root can be selected by the native command, as with Native.
	native.Resource = ""
	native.Request.Data = nil
	if ValidateQuery(query) != nil || ValidateCount(count) != nil {
		t.Fatal("public envelope interpreted native target or bytes")
	}
}

func TestQueryValidatesSortProjectionAndAllNestedFields(t *testing.T) {
	native := querySource()
	first := &pb.SortField{Field: "profile.name"}
	second := &pb.SortField{Field: "id", Descending: true}
	projection := &pb.Projection{Mode: pb.ProjectionMode_INCLUDE, Fields: []string{"profile.name"}}
	request := &pb.QueryRequest{Native: native, Sort: []*pb.SortField{first, second}, Projection: projection}
	if failure := ValidateQuery(request); failure != nil {
		t.Fatal("valid query options rejected", failure)
	}
	mutations := []func(*pb.QueryRequest){
		func(r *pb.QueryRequest) { r.Native = nil },
		func(r *pb.QueryRequest) { r.Native.Request = nil },
		func(r *pb.QueryRequest) { r.Native.Resource = "/records" },
		func(r *pb.QueryRequest) { r.Native.Request.ContentType = "application/json; charset=utf-8" },
		func(r *pb.QueryRequest) { r.Sort[0] = nil },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "" },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "nested..name" },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "name*" },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "name\x00" },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "\xff" },
		func(r *pb.QueryRequest) { r.Sort[0].Field = "id" },
		func(r *pb.QueryRequest) { r.Projection.Mode = pb.ProjectionMode_PROJECTION_MODE_UNSPECIFIED },
		func(r *pb.QueryRequest) { r.Projection.Fields = nil },
		func(r *pb.QueryRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.QueryRequest) { r.Native.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.QueryRequest) { r.Native.Request.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.QueryRequest) { r.Sort[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.QueryRequest) { r.Projection.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
	}
	for index, mutate := range mutations {
		copied := proto.Clone(request).(*pb.QueryRequest)
		mutate(copied)
		if failure := ValidateQuery(copied); failure == nil {
			t.Fatal("invalid query accepted", index)
		}
		if err := ValidateExecuteRequest(queryExecution(copied)); err == nil {
			t.Fatal("invalid query envelope accepted", index)
		}
	}
	if ValidateQuery(nil) == nil {
		t.Fatal("nil query accepted")
	}
}

func TestCountRejectsMissingAndUnknownFields(t *testing.T) {
	native := querySource()
	request := &pb.CountRequest{Native: native}
	mutations := []func(*pb.CountRequest){
		func(r *pb.CountRequest) { r.Native = nil },
		func(r *pb.CountRequest) { r.Native.Request = nil },
		func(r *pb.CountRequest) { r.Native.Resource = "records/%61" },
		func(r *pb.CountRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.CountRequest) { r.Native.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.CountRequest) { r.Native.Request.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
	}
	for index, mutate := range mutations {
		copied := proto.Clone(request).(*pb.CountRequest)
		mutate(copied)
		if failure := ValidateCount(copied); failure == nil {
			t.Fatal("invalid count accepted", index)
		}
		if err := ValidateExecuteRequest(countExecution(copied)); err == nil {
			t.Fatal("invalid count envelope accepted", index)
		}
	}
	if ValidateCount(nil) == nil {
		t.Fatal("nil count accepted")
	}
}

func TestQueryCountCompletionEvidence(t *testing.T) {
	failure := Fail(pb.FailureCode_UNAVAILABLE, "backend unavailable")
	validEnds := []*pb.QueryEnd{
		{},
		{ReturnedCount: 3, HasMore: true},
		{ReturnedCount: 1, Failure: failure},
	}
	for _, end := range validEnds {
		value := &pb.Event_QueryEnd{QueryEnd: end}
		event := &pb.Event{Value: value}
		response := &pb.ExecuteResponse{Index: 1, Event: event}
		if err := ValidateExecuteResponse(response); err != nil {
			t.Fatal("valid query completion rejected", err)
		}
		response.Index = 2
		if ValidateExecuteResponse(response) == nil {
			t.Fatal("query completion accepted non-singleton index")
		}
	}
	validCounts := []*pb.CountResult{
		{},
		{Count: math.MaxUint64},
		{Count: 3, Estimated: true},
		{Failure: failure},
	}
	for _, result := range validCounts {
		value := &pb.Event_CountResult{CountResult: result}
		event := &pb.Event{Value: value}
		response := &pb.ExecuteResponse{Index: 1, Event: event}
		if err := ValidateExecuteResponse(response); err != nil {
			t.Fatal("valid count completion rejected", err)
		}
		response.Index = 2
		if ValidateExecuteResponse(response) == nil {
			t.Fatal("count completion accepted non-singleton index")
		}
	}
	invalidEnd := &pb.QueryEnd{Failure: failure, HasMore: true}
	endValue := &pb.Event_QueryEnd{QueryEnd: invalidEnd}
	failedCount := &pb.CountResult{Count: 1, Failure: failure}
	countValue := &pb.Event_CountResult{CountResult: failedCount}
	estimatedFailure := &pb.CountResult{Estimated: true, Failure: failure}
	estimatedValue := &pb.Event_CountResult{CountResult: estimatedFailure}
	emptyEnd := &pb.Event_QueryEnd{}
	emptyCount := &pb.Event_CountResult{}
	emptyMore := &pb.QueryEnd{HasMore: true}
	emptyMoreValue := &pb.Event_QueryEnd{QueryEnd: emptyMore}
	events := []*pb.Event{{Value: endValue}, {Value: countValue}, {Value: estimatedValue}, {Value: emptyEnd}, {Value: emptyCount}, {Value: emptyMoreValue}}
	for _, event := range events {
		if err := ValidateEvent(event); err == nil {
			t.Fatal("contradictory query/count completion accepted", event)
		}
	}
	end := &pb.QueryEnd{}
	unknownEndValue := &pb.Event_QueryEnd{QueryEnd: end}
	result := &pb.CountResult{}
	unknownCountValue := &pb.Event_CountResult{CountResult: result}
	end.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	result.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	unknownEvents := []*pb.Event{{Value: unknownEndValue}, {Value: unknownCountValue}}
	for _, event := range unknownEvents {
		if ValidateEvent(event) == nil {
			t.Fatal("unknown query/count completion field accepted")
		}
	}
}
