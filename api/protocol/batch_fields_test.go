package protocol

import (
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

func TestBatchReadFieldPolicy(t *testing.T) {
	validOptions := &pb.Document{MediaType: "application/opaque", Data: []byte("uninterpreted")}
	invalidOptions := &pb.Document{MediaType: "INVALID", Data: []byte("uninterpreted")}
	oversizedOptions := &pb.Document{MediaType: "application/opaque", Data: make([]byte, MaxDocument+1)}
	tests := []struct {
		name  string
		read  *pb.ReadRequest
		valid bool
	}{
		{name: "default", read: &pb.ReadRequest{Resource: "data/s:key"}, valid: true},
		{name: "opaque options", read: &pb.ReadRequest{Resource: "data/s:key", ReadMediaType: "application/opaque", AdapterOptions: validOptions}, valid: true},
		{name: "invalid read media", read: &pb.ReadRequest{Resource: "data/s:key", ReadMediaType: "INVALID"}},
		{name: "invalid options media", read: &pb.ReadRequest{Resource: "data/s:key", AdapterOptions: invalidOptions}},
		{name: "oversized options", read: &pb.ReadRequest{Resource: "data/s:key", AdapterOptions: oversizedOptions}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &pb.ReadBatchRequest{StoreName: "records", Requests: []*pb.ReadRequest{test.read}}
			if err := ValidateReadBatchRequest(request); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}

func TestBatchMutationFieldPolicy(t *testing.T) {
	document := &pb.Document{MediaType: "application/opaque", Data: []byte("uninterpreted")}
	invalidDocument := &pb.Document{MediaType: "INVALID"}
	oversizedDocument := &pb.Document{MediaType: "application/opaque", Data: make([]byte, MaxDocument+1)}
	empty := &pb.Empty{}
	put := &pb.MutateRequest_Put{Put: document}
	create := &pb.MutateRequest_Create{Create: document}
	replace := &pb.MutateRequest_Replace{Replace: document}
	deleteAction := &pb.MutateRequest_Delete{Delete: empty}
	nilPut := &pb.MutateRequest_Put{}
	nilDelete := &pb.MutateRequest_Delete{}
	invalidPut := &pb.MutateRequest_Put{Put: invalidDocument}
	oversizedPut := &pb.MutateRequest_Put{Put: oversizedDocument}
	program := &pb.ProgramTransform{Runtime: "lua.v1", Source: []byte("return weir.keep()"), Input: document}
	programForm := &pb.Transform_Program{Program: program}
	programTransform := &pb.Transform{Form: programForm}
	programAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: programTransform}
	expression := &pb.Transform_BackendExpression{BackendExpression: document}
	expressionTransform := &pb.Transform{Form: expression}
	expressionAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: expressionTransform}
	nilTransform := &pb.MutateRequest_AtomicTransform{}
	missingForm := &pb.Transform{}
	missingFormAction := &pb.MutateRequest_AtomicTransform{AtomicTransform: missingForm}
	valid := []*pb.MutateRequest{
		{Resource: "data/s:key", Action: put},
		{Resource: "data/s:key", Action: create},
		{Resource: "data/s:key", Action: replace},
		{Resource: "data/s:key", Action: deleteAction},
		{Resource: "data/s:key", Action: programAction},
		{Resource: "data/s:key", Action: expressionAction},
	}
	for _, mutation := range valid {
		request := &pb.MutateBatchRequest{StoreName: "records", Requests: []*pb.MutateRequest{mutation}}
		if err := ValidateMutateBatchRequest(request); err != nil {
			t.Fatalf("valid opaque mutation rejected: %v", err)
		}
	}
	invalid := []*pb.MutateRequest{
		{Resource: "data/s:key"},
		{Resource: "data/s:key", Action: nilPut},
		{Resource: "data/s:key", Action: nilDelete},
		{Resource: "data/s:key", Action: invalidPut},
		{Resource: "data/s:key", Action: oversizedPut},
		{Resource: "data/s:key", Action: nilTransform},
		{Resource: "data/s:key", Action: missingFormAction},
		{Resource: "data/s:key", Action: put, AdapterOptions: invalidDocument},
		{Resource: "data/s:key", Action: put, AdapterOptions: oversizedDocument},
	}
	for _, mutation := range invalid {
		request := &pb.MutateBatchRequest{StoreName: "records", Requests: []*pb.MutateRequest{valid[0], mutation}}
		if err := ValidateMutateBatchRequest(request); err == nil {
			t.Fatal("invalid later mutation accepted", mutation)
		}
	}
	for _, source := range [][]byte{nil, []byte(strings.Repeat("x", MaxExpression+1)), {0xff}, {'x', 0}, []byte("\x1bLua")} {
		program.Source = source
		request := &pb.MutateBatchRequest{StoreName: "records", Requests: []*pb.MutateRequest{valid[4]}}
		if err := ValidateMutateBatchRequest(request); err == nil {
			t.Fatal("invalid program source accepted")
		}
	}
	program.Source = []byte(strings.Repeat("x", MaxExpression))
	request := &pb.MutateBatchRequest{StoreName: "records", Requests: []*pb.MutateRequest{valid[4]}}
	if err := ValidateMutateBatchRequest(request); err != nil {
		t.Fatal("program at the byte bound rejected", err)
	}
	program.Runtime = "lua.v2"
	if err := ValidateMutateBatchRequest(request); err == nil {
		t.Fatal("unsupported runtime accepted")
	}
	program.Runtime = "lua.v1"
	program.Input = invalidDocument
	if err := ValidateMutateBatchRequest(request); err == nil {
		t.Fatal("invalid program input accepted")
	}
	for _, data := range [][]byte{nil, make([]byte, MaxExpression+1)} {
		document.Data = data
		request.Requests = []*pb.MutateRequest{valid[5]}
		if err := ValidateMutateBatchRequest(request); err == nil {
			t.Fatal("empty or oversized expression accepted")
		}
	}
}

func TestBatchResponseRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	document := &pb.Document{MediaType: "application/opaque"}
	read := ReadDocument(document)
	reads := &pb.ReadBatchResponse{Results: []*pb.ReadResult{read, Missing(), ReadFailure(Fail(pb.FailureCode_UNAVAILABLE, "unavailable"))}}
	for position := range 6 {
		copied := proto.Clone(reads).(*pb.ReadBatchResponse)
		messages := []proto.Message{copied, copied.Results[0], copied.Results[0].GetDocument(), copied.Results[1].GetMissing(), copied.Results[2], copied.Results[2].GetFailure()}
		if err := ValidateReadBatchResponse(copied, 3); err != nil {
			t.Fatal("fixture must be valid before mutation", err)
		}
		messages[position].ProtoReflect().SetUnknown([]byte{0x78, 1})
		if err := ValidateReadBatchResponse(copied, 3); err == nil {
			t.Fatalf("unknown Read fields accepted at level %d", position)
		}
	}
	failure := Fail(pb.FailureCode_UNAVAILABLE, "unavailable")
	mutation := Mutation(pb.MutationOutcome_NOT_APPLIED, failure)
	mutations := &pb.MutateBatchResponse{Results: []*pb.MutationResult{mutation}}
	for position := range 3 {
		copied := proto.Clone(mutations).(*pb.MutateBatchResponse)
		messages := []proto.Message{copied, copied.Results[0], copied.Results[0].Failure}
		if err := ValidateMutateBatchResponse(copied, 1); err != nil {
			t.Fatal("fixture must be valid before mutation", err)
		}
		messages[position].ProtoReflect().SetUnknown([]byte{0x78, 1})
		if err := ValidateMutateBatchResponse(copied, 1); err == nil {
			t.Fatalf("unknown Mutate fields accepted at level %d", position)
		}
	}
}

func TestBatchResourcesRespectRelativePathBound(t *testing.T) {
	for _, store := range []string{"a", "target", strings.Repeat("a", 63)} {
		limit := MaxURI
		for _, extra := range []int{0, 1} {
			resource := strings.Repeat("x", limit+extra)
			read := &pb.ReadRequest{Resource: resource}
			request := &pb.ReadBatchRequest{StoreName: store, Requests: []*pb.ReadRequest{read}}
			if err := ValidateReadBatchRequest(request); (err == nil) != (extra == 0) {
				t.Fatalf("store length %d, resource length %d: %v", len(store), len(resource), err)
			}
		}
	}
}

func FuzzBatchReadResource(f *testing.F) {
	for _, resource := range []string{"data/s:key", "data/s:a%2Fb", "data/%E4%B8%AD", "data/%61", "data/%00", "/data", "data/", "weir://other/data"} {
		f.Add("records", resource)
	}
	f.Add(strings.Repeat("a", 63), strings.Repeat("x", MaxURI))
	f.Fuzz(func(t *testing.T, store, resource string) {
		segments, resourceError := ParseRelativeResource(resource)
		expected := ValidStoreName(store) && resourceError == nil && len(segments) != 0
		read := &pb.ReadRequest{Resource: resource}
		request := &pb.ReadBatchRequest{StoreName: store, Requests: []*pb.ReadRequest{read}}
		if err := ValidateReadBatchRequest(request); (err == nil) != expected {
			t.Fatalf("Store %q resource %q expected %v: %v", store, resource, expected, err)
		}
	})
}
