package protocol

import (
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestStoreNames(t *testing.T) {
	for _, name := range []string{"data", "mongo-1", strings.Repeat("a", 128)} {
		if !ValidStoreName(name) {
			t.Errorf("valid Store name rejected: %q", name)
		}
	}
	for _, name := range []string{"", "Data", "1data", "a--b", "data/path", "data?filter", "data:7447", "a-", "weir://data"} {
		if ValidStoreName(name) {
			t.Errorf("invalid Store name accepted: %q", name)
		}
	}
}

func TestCanonicalRelativeResource(t *testing.T) {
	valid := []string{"db/c/s:a%2Fb", "a/s:%E4%B8%AD", "a/i:-3", "records"}
	for _, resource := range valid {
		if _, err := ParseRelativeResource(resource); err != nil {
			t.Errorf("%s: %v", resource, err)
		}
	}
	invalid := []string{"", "/records", "records/", "weir://mongo/db/c/s:a", "a//b", "a/..", "a/%61", "a/%2f", "a/%FF", "a/%00", "x?q=a", "x#f"}
	for _, resource := range invalid {
		if _, err := ParseRelativeResource(resource); err == nil {
			t.Errorf("accepted %q", resource)
		}
	}
}

func TestLuaRejectsPrecompiledBytecode(t *testing.T) {
	program := &pb.LuaTransform{Source: []byte("\x1bLua")}
	programForm := &pb.Transform_Lua{Lua: program}
	transform := &pb.Transform{Form: programForm}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation := &pb.MutateRequest{Resource: "db/c/s:a", Action: action}
	if failure := validateMutationFields(mutation); failure == nil || failure.Code != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(failure)
	}
}

func FuzzRelativeResource(f *testing.F) {
	f.Add("db/c/s:a%2Fb")
	f.Fuzz(func(t *testing.T, resource string) { _, _ = ParseRelativeResource(resource) })
}

func TestExpressionWireBoundaryIsOpaque(t *testing.T) {
	doc := &pb.Document{ContentType: "application/unknown", Data: []byte("not BSON or JSON")}
	form := &pb.Transform_BackendExpression{BackendExpression: doc}
	transform := &pb.Transform{Form: form}
	action := &pb.MutateRequest_AtomicTransform{AtomicTransform: transform}
	mutation := &pb.MutateRequest{Resource: "db/c/s:a", Action: action}
	if f := validateMutationFields(mutation); f != nil {
		t.Fatal("Core interpreted opaque expression", f)
	}
	doc.Data = nil
	if f := validateMutationFields(mutation); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(f)
	}
	transform.Form = nil
	if f := validateMutationFields(mutation); f.GetCode() != pb.FailureCode_INVALID_ARGUMENT {
		t.Fatal(f)
	}
}

func TestExecuteTypedEnvelopeAndUnknownFields(t *testing.T) {
	body := &pb.Document{ContentType: "application/opaque"}
	native := &pb.NativeRequest{Resource: "records", Request: body}
	variant := &pb.Command_Native{Native: native}
	command := &pb.Command{Operation: variant}
	request := &pb.ExecuteRequest{StoreName: "records", Index: 1, Command: command}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*pb.ExecuteRequest){
		func(r *pb.ExecuteRequest) { r.StoreName = "bad/store" },
		func(r *pb.ExecuteRequest) { r.Index = 0 },
		func(r *pb.ExecuteRequest) { r.Index = 2 },
		func(r *pb.ExecuteRequest) { r.Command = nil },
		func(r *pb.ExecuteRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
		func(r *pb.ExecuteRequest) { r.Command.GetNative().Resource = "weir://records/data" },
		func(r *pb.ExecuteRequest) { r.Command.GetNative().Resource = "/data" },
		func(r *pb.ExecuteRequest) { r.Command.GetNative().Resource = "data/%6B" },
		func(r *pb.ExecuteRequest) { r.Command.GetNative().ProtoReflect().SetUnknown([]byte{0x38, 1}) },
	} {
		copied := proto.Clone(request).(*pb.ExecuteRequest)
		mutate(copied)
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatal("accepted invalid request", copied)
		}
	}
	document := &pb.Document{ContentType: "application/octet-stream", Data: []byte("metadata")}
	head := &pb.NativeHead{Metadata: document}
	value := &pb.Event_Head{Head: head}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	response.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1})
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("unknown response field accepted")
	}
}

func TestCancelRequestCarriesOnlyItsStoreAndIndex(t *testing.T) {
	request := &pb.ExecuteRequest{StoreName: "records", CancelIndex: 3}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("cancel request rejected", err)
	}
	invalid := []func(*pb.ExecuteRequest){
		func(r *pb.ExecuteRequest) { r.StoreName = "" },
		func(r *pb.ExecuteRequest) { r.CancelIndex = math.MaxUint64 },
		func(r *pb.ExecuteRequest) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 1}) },
	}
	// Every other field, including one added later, must stay unset.
	fields := request.ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if field.Name() == "store_name" || field.Name() == "cancel_index" {
			continue
		}
		invalid = append(invalid, func(r *pb.ExecuteRequest) {
			message := r.ProtoReflect()
			switch field.Kind() {
			case protoreflect.MessageKind:
				message.Set(field, protoreflect.ValueOfMessage(message.NewField(field).Message()))
			case protoreflect.StringKind:
				message.Set(field, protoreflect.ValueOfString("x"))
			case protoreflect.Uint64Kind:
				message.Set(field, protoreflect.ValueOfUint64(1))
			default:
				t.Fatal("cancel test cannot populate field", field.FullName())
			}
		})
	}
	for _, mutate := range invalid {
		copied := proto.Clone(request).(*pb.ExecuteRequest)
		mutate(copied)
		if err := ValidateExecuteRequest(copied); err == nil {
			t.Fatal("accepted invalid cancel request", copied)
		}
	}
}

func TestNativeUsesOneIndexedFrame(t *testing.T) {
	body := &pb.Document{ContentType: "application/vnd.example.native", Data: []byte("command")}
	native := &pb.NativeRequest{Resource: "records", Request: body}
	operation := &pb.Command_Native{Native: native}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "records", Index: 1, Command: command}
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("Native request rejected", err)
	}
	request.Index = 2
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("Native record ordinal accepted")
	}
	request.Index, request.TimeoutMicros = 1, 1
	if err := ValidateExecuteRequest(request); err == nil {
		t.Fatal("Native record timeout accepted")
	}
	head := &pb.NativeHead{}
	value := &pb.Event_Head{Head: head}
	event := &pb.Event{Value: value}
	response := &pb.ExecuteResponse{Index: 1, Event: event}
	if err := ValidateExecuteResponse(response); err != nil {
		t.Fatal(err)
	}
	response.Index = 2
	if err := ValidateExecuteResponse(response); err == nil {
		t.Fatal("Native response record ordinal accepted")
	}
}

func TestEventValidationAndTypedEncoding(t *testing.T) {
	document := &pb.Document{ContentType: "application/octet-stream", Data: []byte("metadata")}
	head := &pb.NativeHead{Metadata: document}
	value := &pb.Event_Head{Head: head}
	event := &pb.Event{Value: value}
	encoded, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &pb.Event{}
	if err := proto.Unmarshal(encoded, decoded); err != nil || !proto.Equal(event, decoded) {
		t.Fatal("typed event changed", err)
	}
	if err := ValidateEvent(decoded); err != nil {
		t.Fatal(err)
	}
	message := strings.Repeat("中", 4)
	failure := Fail(pb.FailureCode_INTERNAL, message)
	if failure.Message != message || !utf8.ValidString(failure.Message) {
		t.Fatal("failure message changed")
	}
	badDocument := &pb.Document{ContentType: "invalid"}
	badHead := &pb.NativeHead{Metadata: badDocument}
	badDocumentValue := &pb.Event_Head{Head: badHead}
	badChunkValue := &pb.Event_Chunk{}
	badEnd := &pb.NativeEnd{Completion: pb.NativeCompletion(99)}
	badEndValue := &pb.Event_NativeEnd{NativeEnd: badEnd}
	incomplete := &pb.NativeEnd{Completion: pb.NativeCompletion_RESPONSE_INCOMPLETE}
	incompleteValue := &pb.Event_NativeEnd{NativeEnd: incomplete}
	invalid := []*pb.Event{
		{}, {Value: badDocumentValue}, {Value: badChunkValue},
		{Value: badEndValue}, {Value: incompleteValue},
	}
	for _, item := range invalid {
		if err := ValidateEvent(item); err == nil {
			t.Fatal("accepted invalid event")
		}
	}
}

func TestAppliedMutationMayReportPostWriteFailure(t *testing.T) {
	failure := Fail(pb.FailureCode_UNAVAILABLE, "write applied but replica acknowledgement failed")
	mutation := Mutation(pb.MutationOutcome_APPLIED, failure)
	if err := ValidateMutationResult(mutation); err != nil {
		t.Fatal(err)
	}
	failure.Code = pb.FailureCode_FAILURE_CODE_UNSPECIFIED
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("invalid failure accepted")
	}
	mutation.Outcome, mutation.Failure = pb.MutationOutcome_NOT_STARTED, nil
	if err := ValidateMutationResult(mutation); err == nil {
		t.Fatal("unfinished result without failure accepted")
	}
}

func TestSchemaHasNoMapFields(t *testing.T) {
	pending := []protoreflect.MessageDescriptors{pb.File_api_weir_v1_store_proto.Messages()}
	for len(pending) > 0 {
		messages := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for i := range messages.Len() {
			message := messages.Get(i)
			fields := message.Fields()
			for j := range fields.Len() {
				if fields.Get(j).IsMap() {
					t.Fatalf("%s is a map field; hasUnknown does not inspect map values", fields.Get(j).FullName())
				}
			}
			pending = append(pending, message.Messages())
		}
	}
}

func TestRecordTimeoutNeverShortensOrDisablesTheCallerBudget(t *testing.T) {
	read := &pb.ReadRequest{Resource: "records/a"}
	request := readExecution("records", 1, read)
	request.TimeoutMicros = math.MaxUint64
	if err := ValidateExecuteRequest(request); err != nil {
		t.Fatal("record timeout rejected", err)
	}
	cases := []struct {
		remaining time.Duration
		micros    uint64
	}{
		{remaining: -time.Second, micros: 1},
		{remaining: 0, micros: 1},
		{remaining: time.Nanosecond, micros: 1},
		{remaining: time.Microsecond + time.Nanosecond, micros: 2},
		{remaining: 3 * time.Second, micros: 3_000_000},
		{remaining: math.MaxInt64, micros: math.MaxInt64/1000 + 1},
	}
	for _, c := range cases {
		if got := TimeoutMicros(c.remaining); got != c.micros {
			t.Fatalf("TimeoutMicros(%v) = %d, want %d", c.remaining, got, c.micros)
		}
	}
	durations := []struct {
		micros    uint64
		remaining time.Duration
	}{
		{micros: 0, remaining: 0},
		{micros: 3_000_000, remaining: 3 * time.Second},
		{micros: math.MaxInt64 / 1000, remaining: math.MaxInt64 / 1000 * time.Microsecond},
		{micros: math.MaxInt64/1000 + 1, remaining: math.MaxInt64},
		{micros: math.MaxUint64, remaining: math.MaxInt64},
	}
	for _, c := range durations {
		if got := RecordTimeout(c.micros); got != c.remaining {
			t.Fatalf("RecordTimeout(%d) = %v, want %v", c.micros, got, c.remaining)
		}
	}
}
