// Package protocol provides shared client and server validation of public wire
// envelopes without interpreting document fields.
package protocol

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// MaxTransportMessageBytes is the largest message length representable on this Go platform.
// Backend protocol limits and actual process capacity govern business payloads.
const MaxTransportMessageBytes = int(^uint(0) >> 1)

var storePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9]|-[a-z0-9])*$`)
var mediaPattern = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)

// ValidStoreName reports whether name is a well-formed Store name: a lowercase
// letter followed by lowercase letters, digits and single internal hyphens.
func ValidStoreName(name string) bool {
	return storePattern.MatchString(name)
}

// ParseRelativeResource validates a canonical Store-relative path and returns its
// decoded segments. Store identity is supplied separately by the request envelope.
func ParseRelativeResource(resource string) ([]string, error) {
	if resource == "" {
		return nil, fmt.Errorf("missing or invalid relative resource")
	}
	pieces := strings.Split(resource, "/")
	segments := make([]string, len(pieces))
	for index, piece := range pieces {
		segment, err := decodeResourceSegment(piece)
		if err != nil {
			return nil, err
		}
		segments[index] = segment
	}
	return segments, nil
}

// EncodeSegment percent-encodes one path component using canonical uppercase hex.
func EncodeSegment(s string) string {
	escapes := 0
	for i := 0; i < len(s); i++ {
		if !unescapedSegmentByte(s[i]) {
			escapes++
		}
	}
	if escapes == 0 {
		return s
	}
	const hex = "0123456789ABCDEF"
	var builder strings.Builder
	builder.Grow(len(s) + 2*escapes)
	for i := 0; i < len(s); i++ {
		value := s[i]
		if unescapedSegmentByte(value) {
			builder.WriteByte(value)
		} else {
			builder.WriteByte('%')
			builder.WriteByte(hex[value>>4])
			builder.WriteByte(hex[value&15])
		}
	}
	return builder.String()
}

func unescapedSegmentByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '.' || value == '_' || value == '~' || value == ':'
}

func Fail(code pb.FailureCode, message string) *pb.Failure {
	f := &pb.Failure{Code: code, Message: message}
	return f
}

func ContextFailure(ctx context.Context) *pb.Failure {
	if ctx.Err() == context.DeadlineExceeded {
		return Fail(pb.FailureCode_DEADLINE_EXCEEDED, "deadline exceeded")
	}
	return Fail(pb.FailureCode_CANCELLED, "cancelled")
}

func Mutation(outcome pb.MutationOutcome, failure *pb.Failure) *pb.MutationResult {
	r := &pb.MutationResult{Outcome: outcome, Failure: failure}
	return r
}

func ReadFailure(f *pb.Failure) *pb.ReadResult {
	v := &pb.ReadResult_Failure{Failure: f}
	r := &pb.ReadResult{Result: v}
	return r
}

func ReadDocument(d *pb.Document) *pb.ReadResult {
	v := &pb.ReadResult_Document{Document: d}
	r := &pb.ReadResult{Result: v}
	return r
}

func Missing() *pb.ReadResult {
	e := &pb.Empty{}
	v := &pb.ReadResult_Missing{Missing: e}
	r := &pb.ReadResult{Result: v}
	return r
}

func validateMutationFields(mutation *pb.MutateRequest) *pb.Failure {
	var document *pb.Document
	switch action := mutation.Action.(type) {
	case *pb.MutateRequest_Put:
		if action == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing action")
		}
		document = action.Put
	case *pb.MutateRequest_Create:
		if action == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing action")
		}
		document = action.Create
	case *pb.MutateRequest_Replace:
		if action == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing action")
		}
		document = action.Replace
	case *pb.MutateRequest_Delete:
		if action == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing action")
		}
		if action.Delete == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing delete")
		}
		return nil
	case *pb.MutateRequest_AtomicTransform:
		if action == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing action")
		}
		if action.AtomicTransform == nil || action.AtomicTransform.Form == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing transform")
		}
		switch transform := action.AtomicTransform.Form.(type) {
		case *pb.Transform_Lua:
			if transform == nil {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing transform")
			}
			lua := transform.Lua
			if lua == nil ||
				len(lua.Source) == 0 ||
				!utf8.Valid(lua.Source) ||
				strings.IndexByte(string(lua.Source), 0) >= 0 ||
				strings.HasPrefix(string(lua.Source), "\x1bLua") {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or invalid Lua transform")
			}
			if lua.Input != nil && !validDocument(lua.Input) {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
			}
			return nil
		case *pb.Transform_BackendExpression:
			if transform == nil {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing transform")
			}
			if transform.BackendExpression == nil || len(transform.BackendExpression.Data) == 0 {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or invalid backend expression")
			}
			document = transform.BackendExpression
		default:
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "unknown transform")
		}
	default:
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or unknown action")
	}
	if !validDocument(document) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
	}
	return nil
}

func validMedia(s string) bool { return mediaPattern.MatchString(s) }

const NativeChunk = 64 << 10

func NativeFailure(started bool, failure *pb.Failure) *pb.NativeEnd {
	completion := pb.NativeCompletion_NATIVE_NOT_STARTED
	if started {
		completion = pb.NativeCompletion_RESPONSE_INCOMPLETE
	}
	end := &pb.NativeEnd{Completion: completion, Failure: failure}
	return end
}

// ValidateExecuteRequest validates one operation without retaining stream
// state. Callers enforce a fixed Store/kind and consecutive indexes across requests.
func ValidateExecuteRequest(req *pb.ExecuteRequest) error {
	if req == nil || !ValidStoreName(req.StoreName) || req.Index == 0 {
		return fmt.Errorf("invalid Execute request envelope")
	}
	if err := validateCommand(req.Command); err != nil {
		return err
	}
	if hasUnknown(req.ProtoReflect()) {
		return fmt.Errorf("invalid Execute request fields")
	}
	switch req.Command.Operation.(type) {
	case *pb.Command_Read, *pb.Command_Mutate:
		if req.Index == math.MaxUint64 {
			return fmt.Errorf("record index overflows the next ordinal")
		}
	default:
		if req.Index != 1 || req.TimeoutMicros != 0 {
			return fmt.Errorf("Native requires index 1 and no record timeout")
		}
	}
	return nil
}

// RecordTimeout converts a request's timeout_micros, saturating at the
// longest representable duration. Zero means no record deadline.
func RecordTimeout(micros uint64) time.Duration {
	if micros > uint64(math.MaxInt64/int64(time.Microsecond)) {
		return math.MaxInt64
	}
	return time.Duration(micros) * time.Microsecond
}

// TimeoutMicros rounds a remaining caller budget up to whole microseconds so a
// record never receives less time than its caller allows. An exhausted budget
// maps to one microsecond, never to the zero that disables the deadline.
func TimeoutMicros(remaining time.Duration) uint64 {
	if remaining <= time.Microsecond {
		return 1
	}
	micros := uint64(remaining / time.Microsecond)
	if remaining%time.Microsecond != 0 {
		micros++
	}
	return micros
}

// ValidateExecuteResponse validates one indexed event and rejects unknown fields
// recursively. Stream consumers enforce the expected event kind, exactly one
// result per submitted index, and terminal RPC evidence. Missing or
// unrecognized event variants remain invalid.
func ValidateExecuteResponse(response *pb.ExecuteResponse) error {
	if response == nil || response.Index == 0 || response.Index == math.MaxUint64 || hasUnknown(response.ProtoReflect()) {
		return fmt.Errorf("invalid Execute response envelope")
	}
	if err := ValidateEvent(response.Event); err != nil {
		return err
	}
	switch response.Event.Value.(type) {
	case *pb.Event_ReadResult, *pb.Event_MutationResult:
		return nil
	default:
		if response.Index != 1 {
			return fmt.Errorf("Native events require index 1")
		}
		return nil
	}
}

// ValidateCommand validates one typed operation and its fields.
func ValidateCommand(command *pb.Command) error {
	if err := validateCommand(command); err != nil {
		return err
	}
	if hasUnknown(command.ProtoReflect()) {
		return fmt.Errorf("unknown command fields")
	}
	return nil
}

func validateCommand(command *pb.Command) error {
	if command == nil || command.Operation == nil {
		return fmt.Errorf("invalid command fields")
	}
	switch operation := command.Operation.(type) {
	case *pb.Command_Read:
		if operation == nil {
			return fmt.Errorf("missing Read request")
		}
		return validateReadRequest(operation.Read)
	case *pb.Command_Mutate:
		if operation == nil {
			return fmt.Errorf("missing Mutate request")
		}
		return validateMutationRequest(operation.Mutate)
	case *pb.Command_Native:
		if operation == nil || operation.Native == nil {
			return fmt.Errorf("missing Native request")
		}
		if failure := validateNative(operation.Native); failure != nil {
			return fmt.Errorf("%s", failure.Message)
		}
		return nil
	default:
		return fmt.Errorf("missing command operation")
	}
}

func validRelativeResource(resource string) bool {
	if resource == "" {
		return false
	}
	for {
		piece, remaining, more := strings.Cut(resource, "/")
		if _, err := decodeResourceSegment(piece); err != nil {
			return false
		}
		if !more {
			return true
		}
		resource = remaining
	}
}

func decodeResourceSegment(raw string) (string, error) {
	segment, err := url.PathUnescape(raw)
	if err != nil || segment == "" || segment == "." || segment == ".." || !utf8.ValidString(segment) {
		return "", fmt.Errorf("invalid segment")
	}
	for _, value := range segment {
		if unicode.IsControl(value) {
			return "", fmt.Errorf("control character")
		}
	}
	if EncodeSegment(segment) != raw {
		return "", fmt.Errorf("noncanonical segment")
	}
	return segment, nil
}

// Unknown wire fields are rejected recursively so unknown semantics cannot be
// silently executed or accepted as completion evidence.
func hasUnknown(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return true
	}
	// Walk populated message fields by descriptor; a Range callback would
	// allocate on every validated message.
	fields := message.Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if field.Message() == nil || !message.Has(field) {
			continue
		}
		value := message.Get(field)
		if !field.IsList() {
			if hasUnknown(value.Message()) {
				return true
			}
			continue
		}
		list := value.List()
		for j := range list.Len() {
			if hasUnknown(list.Get(j).Message()) {
				return true
			}
		}
	}
	return false
}

// ValidateEvent checks known event semantics and fields.
func ValidateEvent(event *pb.Event) error {
	if event == nil || event.Value == nil || hasUnknown(event.ProtoReflect()) {
		return fmt.Errorf("invalid event fields")
	}
	valid := false
	switch value := event.Value.(type) {
	case *pb.Event_ReadResult:
		if value == nil {
			break
		}
		return ValidateReadResult(value.ReadResult)
	case *pb.Event_MutationResult:
		if value == nil {
			break
		}
		return ValidateMutationResult(value.MutationResult)
	case *pb.Event_Head:
		if value == nil {
			break
		}
		head := value.Head
		valid = head != nil && (head.BodyContentType == "" || validMedia(head.BodyContentType)) && (head.Metadata == nil || validDocument(head.Metadata))
	case *pb.Event_Chunk:
		if value == nil {
			break
		}
		valid = len(value.Chunk) > 0
	case *pb.Event_NativeEnd:
		if value == nil {
			break
		}
		end := value.NativeEnd
		if end != nil {
			valid = end.Completion >= pb.NativeCompletion_NATIVE_NOT_STARTED && end.Completion <= pb.NativeCompletion_RESPONSE_INCOMPLETE && validFailure(end.Failure)
			if end.Completion == pb.NativeCompletion_RESPONSE_COMPLETE {
				valid = valid && end.Failure == nil
			} else {
				valid = valid && end.Failure != nil
			}
		}
	}
	if !valid {
		return fmt.Errorf("invalid event value")
	}
	return nil
}

func validFailure(failure *pb.Failure) bool {
	return failure == nil || failure.Code >= pb.FailureCode_INVALID_ARGUMENT && failure.Code <= pb.FailureCode_INTERNAL && utf8.ValidString(failure.Message)
}
func validDocument(document *pb.Document) bool {
	return document != nil && validMedia(document.ContentType)
}
