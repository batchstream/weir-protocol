// Package protocol provides shared client and server validation of public wire
// envelopes without interpreting document fields.
package protocol

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	MaxDocument             = 2 << 20
	MaxCommandBytes         = 9 << 20
	MaxExecuteRequestBytes  = MaxCommandBytes + 128
	MaxExecuteResponseBytes = MaxEvent + 64
	MaxEvent                = MaxDocument + (8 << 10)
	MaxBatchRequestBytes    = 32 << 20
	MaxBatchResponseBytes   = 32 << 20
	MaxResourceBytes        = 4096
	MaxExpression           = 16 << 10
	MaxSelector             = 16 << 10
)

var storePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9]|-[a-z0-9]){0,62}$`)
var mediaPattern = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)

// ParseRelativeResource validates a canonical Store-relative path and returns its
// decoded segments. Store identity is supplied separately by the request envelope.
func ParseRelativeResource(resource string) ([]string, error) {
	if resource == "" || len(resource) > MaxResourceBytes {
		return nil, fmt.Errorf("missing or oversized relative resource")
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
	if len(message) > 1024 {
		message = message[:1024]
		for !utf8.ValidString(message) && len(message) > 0 {
			message = message[:len(message)-1]
		}
	}
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

func validateReadFields(read *pb.ReadRequest) *pb.Failure {
	if read.ReadMediaType != "" && !validMedia(read.ReadMediaType) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid media type")
	}
	if read.AdapterOptions != nil && !validDocument(read.AdapterOptions, MaxDocument) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
	}
	return nil
}

func validateMutationFields(mutation *pb.MutateRequest) *pb.Failure {
	if mutation.AdapterOptions != nil && !validDocument(mutation.AdapterOptions, MaxDocument) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
	}
	var document *pb.Document
	switch action := mutation.Action.(type) {
	case *pb.MutateRequest_Put:
		document = action.Put
	case *pb.MutateRequest_Create:
		document = action.Create
	case *pb.MutateRequest_Replace:
		document = action.Replace
	case *pb.MutateRequest_Delete:
		if action.Delete == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing delete")
		}
		return nil
	case *pb.MutateRequest_AtomicTransform:
		if action.AtomicTransform == nil || action.AtomicTransform.Form == nil {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing transform")
		}
		switch transform := action.AtomicTransform.Form.(type) {
		case *pb.Transform_Program:
			program := transform.Program
			if program == nil ||
				program.Runtime == "" ||
				len(program.Source) == 0 ||
				len(program.Source) > MaxExpression ||
				!utf8.Valid(program.Source) ||
				strings.IndexByte(string(program.Source), 0) >= 0 ||
				strings.HasPrefix(string(program.Source), "\x1bLua") {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or oversized program transform")
			}
			if program.Runtime != "lua.v1" {
				return Fail(pb.FailureCode_UNSUPPORTED, "program runtime is unsupported")
			}
			if program.Input != nil && !validDocument(program.Input, MaxDocument) {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
			}
			return nil
		case *pb.Transform_BackendExpression:
			if transform.BackendExpression == nil || len(transform.BackendExpression.Data) == 0 || len(transform.BackendExpression.Data) > MaxExpression {
				return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or oversized backend expression")
			}
			document = transform.BackendExpression
		default:
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "unknown transform")
		}
	default:
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or unknown action")
	}
	if !validDocument(document, MaxDocument) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid document envelope")
	}
	return nil
}

func validMedia(s string) bool { return len(s) <= 127 && mediaPattern.MatchString(s) }

func ValidateScan(req *pb.ScanRequest) *pb.Failure {
	if req == nil || proto.Size(req) > MaxExecuteRequestBytes {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or oversized Scan")
	}
	if !validRelativeResource(req.Resource) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Scan relative resource")
	}
	if req.ReadMediaType != "" && !validMedia(req.ReadMediaType) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Scan media type")
	}
	if req.PageSize > MaxScanPageSize || len(req.ContinuationToken) > MaxScanToken {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "Scan page size or continuation exceeds bound")
	}
	if d := req.Selector; d != nil && (!validMedia(d.MediaType) || len(d.Data) > MaxSelector) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid or oversized selector")
	}
	return nil
}

const NativeChunk = 64 << 10
const NativeDescriptor = 64 << 10

func ValidateNative(open *pb.NativeOpen) *pb.Failure {
	if open == nil || proto.Size(open) > NativeDescriptor+MaxResourceBytes+256 || open.Descriptor_ == nil || len(open.Descriptor_.Data) > NativeDescriptor {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native Open bounds")
	}
	if !validRelativeResource(open.Resource) {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native relative resource")
	}
	return nil
}

func NativeFailure(started bool, failure *pb.Failure) *pb.NativeEnd {
	completion := pb.NativeCompletion_NATIVE_NOT_STARTED
	if started {
		completion = pb.NativeCompletion_RESPONSE_INCOMPLETE
	}
	end := &pb.NativeEnd{Completion: completion, Failure: failure}
	return end
}

// ValidateExecuteRequest validates one typed Scan or Native request.
func ValidateExecuteRequest(req *pb.ExecuteRequest) error {
	if req == nil || !ValidStoreName(req.StoreName) || proto.Size(req) > MaxExecuteRequestBytes || hasUnknown(req.ProtoReflect()) {
		return fmt.Errorf("invalid Execute request envelope")
	}
	return ValidateCommand(req.Command)
}

func ValidateExecuteResponse(response *pb.ExecuteResponse) error {
	if response == nil || proto.Size(response) > MaxExecuteResponseBytes || hasUnknown(response.ProtoReflect()) {
		return fmt.Errorf("invalid Execute response envelope")
	}
	return ValidateEvent(response.Event)
}

// ValidateCommand requires one Scan or Native request with a canonical relative path.
func ValidateCommand(command *pb.Command) error {
	if command == nil || command.Operation == nil || proto.Size(command) > MaxCommandBytes || hasUnknown(command.ProtoReflect()) {
		return fmt.Errorf("invalid command fields or byte bound")
	}
	var failure *pb.Failure
	switch operation := command.Operation.(type) {
	case *pb.Command_Scan:
		failure = ValidateScan(operation.Scan)
	case *pb.Command_Native:
		if operation.Native == nil {
			return fmt.Errorf("missing Native request")
		}
		failure = ValidateNative(operation.Native.Open)
	default:
		return fmt.Errorf("missing command operation")
	}
	if failure != nil {
		return fmt.Errorf("%s", failure.Message)
	}
	return nil
}

func validRelativeResource(resource string) bool {
	if resource == "" || len(resource) > MaxResourceBytes {
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

// Unknown fields are rejected throughout the typed payload so invalid or
// mismatched request and response schemas fail before their values are consumed.
func hasUnknown(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return true
	}
	unknown := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsList() && field.Message() != nil {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if hasUnknown(list.Get(i).Message()) {
					unknown = true
					return false
				}
			}
		} else if field.IsMap() && field.MapValue().Message() != nil {
			value.Map().Range(func(_ protoreflect.MapKey, entry protoreflect.Value) bool {
				unknown = hasUnknown(entry.Message())
				return !unknown
			})
		} else if field.Message() != nil {
			unknown = hasUnknown(value.Message())
		}
		return !unknown
	})
	return unknown
}

func ValidateEvent(event *pb.Event) error {
	if event == nil || event.Value == nil || hasUnknown(event.ProtoReflect()) || proto.Size(event) > MaxEvent {
		return fmt.Errorf("invalid event fields or byte bound")
	}
	valid := false
	switch value := event.Value.(type) {
	case *pb.Event_Document:
		valid = validDocument(value.Document, MaxDocument)
	case *pb.Event_Head:
		head := value.Head
		valid = head != nil && (head.BodyMediaType == "" || validMedia(head.BodyMediaType)) && (head.Metadata == nil || validDocument(head.Metadata, NativeDescriptor))
	case *pb.Event_Chunk:
		valid = len(value.Chunk) > 0 && len(value.Chunk) <= NativeChunk
	case *pb.Event_ScanEnd:
		end := value.ScanEnd
		if end != nil {
			valid = validFailure(end.Failure) && len(end.NextContinuationToken) <= MaxScanToken
			if end.Failure == nil {
				valid = valid && end.Exhausted != (len(end.NextContinuationToken) != 0)
			} else {
				valid = valid && !end.Exhausted && len(end.NextContinuationToken) == 0
			}
		}
	case *pb.Event_NativeEnd:
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
	return failure == nil || failure.Code >= pb.FailureCode_INVALID_ARGUMENT && failure.Code <= pb.FailureCode_INTERNAL && len(failure.Message) <= 1024 && utf8.ValidString(failure.Message)
}
func validDocument(document *pb.Document, limit int) bool {
	return document != nil && validMedia(document.MediaType) && len(document.Data) <= limit
}
