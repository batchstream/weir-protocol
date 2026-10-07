package protocol

import (
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestStreamingResponsesAcceptNestedAdditiveFields(t *testing.T) {
	metadata := &pb.Document{ContentType: "application/opaque", Data: []byte("metadata")}
	head := &pb.NativeHead{Metadata: metadata, BodyContentType: "application/opaque"}
	headValue := &pb.Event_Head{Head: head}
	scanFailure := Fail(pb.FailureCode(99), "future scan failure")
	scanEnd := &pb.ScanEnd{Failure: scanFailure}
	scanValue := &pb.Event_ScanEnd{ScanEnd: scanEnd}
	nativeFailure := Fail(pb.FailureCode(99), "future native failure")
	nativeEnd := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE, Failure: nativeFailure}
	nativeValue := &pb.Event_NativeEnd{NativeEnd: nativeEnd}
	chunkValue := &pb.Event_Chunk{Chunk: []byte("body")}
	events := []*pb.Event{{Value: headValue}, {Value: scanValue}, {Value: nativeValue}, {Value: chunkValue}}
	for _, message := range []proto.Message{metadata, head, scanFailure, scanEnd, nativeFailure, nativeEnd} {
		message.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	}
	for _, event := range events {
		event.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		response := &pb.ExecuteResponse{Index: 1, Event: event}
		response.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		if err := ValidateEvent(event); err != nil {
			t.Fatal("additive streaming event fields rejected", err)
		}
		if err := ValidateExecuteResponse(response); err != nil {
			t.Fatal("additive streaming response fields rejected", err)
		}
	}
}

func TestFutureFailureCodesPreserveApplicationEvidence(t *testing.T) {
	for _, code := range []pb.FailureCode{pb.FailureCode(99), pb.FailureCode(2147483647)} {
		failure := Fail(code, "future classification")
		failure.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
		read := ReadFailure(failure)
		if err := ValidateReadResult(read); err != nil || read.GetFailure().Code != code {
			t.Fatal("future Read failure changed or rejected", err)
		}
		for _, outcome := range []pb.MutationOutcome{pb.MutationOutcome_APPLIED, pb.MutationOutcome_UNKNOWN} {
			mutation := Mutation(outcome, failure)
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
				t.Fatal("future mutation failure rejected", err)
			}
			result := decoded.Event.GetMutationResult()
			if result.Outcome != outcome || result.Failure.Code != code {
				t.Fatal("application evidence or opaque failure classification changed", result)
			}
		}
	}
	for _, code := range []pb.FailureCode{pb.FailureCode_FAILURE_CODE_UNSPECIFIED, pb.FailureCode(-1)} {
		failure := Fail(code, "invalid classification")
		mutation := Mutation(pb.MutationOutcome_APPLIED, failure)
		if err := ValidateMutationResult(mutation); err == nil {
			t.Fatal("nonpositive failure code accepted", code)
		}
	}
}

func TestUnknownResponseVariantsRemainInvalid(t *testing.T) {
	// An unrecognized oneof variant is stored as an unknown field by protobuf.
	// Ancillary-field tolerance must not turn it into known completion evidence.
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
	failure := Fail(pb.FailureCode(99), "future failure")
	mutation := Mutation(pb.MutationOutcome(99), failure)
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("unrecognized application evidence accepted")
	}
	native := &pb.NativeEnd{Completion: pb.NativeCompletion(99), Failure: failure}
	value := &pb.Event_NativeEnd{NativeEnd: native}
	event.Value = value
	if err := ValidateEvent(event); err == nil {
		t.Fatal("unrecognized transport evidence accepted")
	}
}

func TestAdditiveResponseFieldsStillCountTowardsByteBounds(t *testing.T) {
	unknown := protowire.AppendTag(nil, 99, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, make([]byte, MaxEvent+1))
	read := Missing()
	read.ProtoReflect().SetUnknown(unknown)
	if err := ValidateReadResult(read); err == nil {
		t.Fatal("oversized additive Read fields accepted")
	}
	mutation := Mutation(pb.MutationOutcome_APPLIED, nil)
	mutation.ProtoReflect().SetUnknown(unknown)
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("oversized additive mutation fields accepted")
	}
	document := &pb.Document{ContentType: "application/opaque"}
	value := &pb.Event_Document{Document: document}
	event := &pb.Event{Value: value}
	event.ProtoReflect().SetUnknown(unknown)
	if err := ValidateEvent(event); err == nil {
		t.Fatal("oversized additive event fields accepted")
	}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("oversized additive response fields accepted")
	}
}
