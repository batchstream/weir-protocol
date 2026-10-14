package protocol

import (
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

func TestMutationOptionsRejectUnknownFieldsAndVisibility(t *testing.T) {
	empty := &pb.Empty{}
	action := &pb.MutateRequest_Delete{Delete: empty}
	for _, visibility := range []pb.MutationVisibility{-1, 2, 2147483647} {
		options := &pb.MutationOptions{Visibility: visibility}
		mutation := &pb.MutateRequest{Resource: "records/s:key", Action: action, Options: options}
		if err := ValidateMutationRequest(mutation); err == nil {
			t.Fatal("undeclared visibility accepted", visibility)
		}
		request := mutationExecution("records", 1, mutation)
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("undeclared visibility accepted in Execute", visibility)
		}
	}
	for _, visibility := range []pb.MutationVisibility{pb.MutationVisibility_APPLICATION_ACKNOWLEDGED, pb.MutationVisibility_SEARCH_VISIBLE} {
		options := &pb.MutationOptions{Visibility: visibility}
		options.ProtoReflect().SetUnknown([]byte{0x10, 1})
		mutation := &pb.MutateRequest{Resource: "records/s:key", Action: action, Options: options}
		if err := ValidateMutationRequest(mutation); err == nil {
			t.Fatal("unknown option field accepted", visibility)
		}
		request := mutationExecution("records", 1, mutation)
		if err := ValidateExecuteRequest(request); err == nil {
			t.Fatal("unknown option field accepted in Execute", visibility)
		}
	}
}
