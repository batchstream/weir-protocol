package protocol

import (
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestStreamingResponsesRejectNestedUnknownFields(t *testing.T) {
	metadata := &pb.Document{ContentType: "application/opaque", Data: []byte("metadata")}
	head := &pb.NativeHead{Metadata: metadata, BodyContentType: "application/opaque"}
	headValue := &pb.Event_Head{Head: head}
	nativeFailure := Fail(pb.FailureCode_UNAVAILABLE, "native failure")
	nativeEnd := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE, Failure: nativeFailure}
	nativeValue := &pb.Event_NativeEnd{NativeEnd: nativeEnd}
	chunkValue := &pb.Event_Chunk{Chunk: []byte("body")}
	events := []*pb.Event{{Value: headValue}, {Value: nativeValue}, {Value: chunkValue}}
	for _, event := range events {
		response := &pb.ExecuteResponse{Index: 1, Event: event}
		if err := ValidateExecuteResponse(response); err != nil {
			t.Fatal("invalid fixture", err)
		}
		messages := []proto.Message{response, event}
		switch value := event.Value.(type) {
		case *pb.Event_Head:
			messages = append(messages, value.Head, value.Head.Metadata)
		case *pb.Event_NativeEnd:
			messages = append(messages, value.NativeEnd, value.NativeEnd.Failure)
		}
		for _, message := range messages {
			message.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
			if err := ValidateExecuteResponse(response); err == nil {
				t.Fatal("unknown streaming response field accepted", message)
			}
			if message != response {
				if err := ValidateEvent(event); err == nil {
					t.Fatal("unknown streaming event field accepted", message)
				}
			}
			message.ProtoReflect().SetUnknown(nil)
		}
	}
}

func TestOnlyDeclaredFailureCodesAreAccepted(t *testing.T) {
	for _, code := range []pb.FailureCode{pb.FailureCode_FAILURE_CODE_UNSPECIFIED, pb.FailureCode(-1), pb.FailureCode(13), pb.FailureCode(99), pb.FailureCode(2147483647)} {
		failure := Fail(code, "invalid classification")
		read := ReadFailure(failure)
		if err := ValidateReadResult(read); err == nil {
			t.Fatal("undeclared Read failure accepted", code)
		}
		for _, outcome := range []pb.MutationOutcome{pb.MutationOutcome_APPLIED, pb.MutationOutcome_UNKNOWN} {
			mutation := Mutation(outcome, failure)
			if err := ValidateMutationResult(mutation); err == nil {
				t.Fatal("undeclared mutation failure accepted", code)
			}
		}
		native := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE, Failure: failure}
		nativeValue := &pb.Event_NativeEnd{NativeEnd: native}
		event := &pb.Event{Value: nativeValue}
		if err := ValidateEvent(event); err == nil {
			t.Fatal("undeclared Native failure accepted", code)
		}
	}
	for code := pb.FailureCode_INVALID_ARGUMENT; code <= pb.FailureCode_INTERNAL; code++ {
		failure := Fail(code, "known acknowledgement failure")
		mutation := Mutation(pb.MutationOutcome_APPLIED, failure)
		response := mutationResponse(1, mutation)
		raw, err := proto.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.ExecuteResponse{}
		if err := proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		if err := ValidateExecuteResponse(decoded); err != nil {
			t.Fatal("known application evidence rejected", err)
		}
		result := decoded.Event.GetMutationResult()
		if result.Outcome != pb.MutationOutcome_APPLIED || result.Failure.Code != code {
			t.Fatal("known application evidence changed", result)
		}
	}
}

func TestUnknownResponseVariantsAreInvalid(t *testing.T) {
	raw := protowire.AppendTag(nil, 99, protowire.BytesType)
	raw = protowire.AppendBytes(raw, nil)
	event := &pb.Event{}
	if err := proto.Unmarshal(raw, event); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("unrecognized event variant accepted")
	}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("unrecognized response event accepted")
	}
	read := &pb.ReadResult{}
	if err := proto.Unmarshal(raw, read); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadResult(read); err == nil {
		t.Fatal("unrecognized Read result accepted")
	}
	response = readResponse(1, read)
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("unrecognized nested Read result accepted")
	}
	failure := Fail(pb.FailureCode_UNAVAILABLE, "failure")
	mutation := Mutation(pb.MutationOutcome(99), failure)
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("unrecognized application evidence accepted")
	}
	native := &pb.NativeEnd{Completion: pb.NativeCompletion(99), Failure: failure}
	value := &pb.Event_NativeEnd{NativeEnd: native}
	event.Value = value
	event.ProtoReflect().SetUnknown(nil)
	if err := ValidateEvent(event); err == nil {
		t.Fatal("unrecognized transport evidence accepted")
	}
}
