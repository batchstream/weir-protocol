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
	MaxDocument           = 2 << 20
	MaxPayload            = 9 << 20
	MaxFrame              = MaxPayload + 128
	MaxResponse           = MaxEvent + 64
	MaxEvent              = MaxDocument + (8 << 10)
	MaxBatchRequestBytes  = 32 << 20
	MaxBatchResponseBytes = 32 << 20
	MaxURI                = 4096
	ResultOverhead        = 512
	EntryOverhead         = 512
	MaxExpression         = 16 << 10
	MaxSelector           = 16 << 10
)

var storePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9]|-[a-z0-9]){0,62}$`)
var mediaPattern = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)

func ParseResource(raw string) (string, []string, error) {
	if len(raw) > MaxURI || !strings.HasPrefix(raw, "weir://") {
		return "", nil, fmt.Errorf("invalid resource")
	}
	pieces := strings.Split(strings.TrimPrefix(raw, "weir://"), "/")
	if len(pieces[0]) > 63 || !storePattern.MatchString(pieces[0]) {
		return "", nil, fmt.Errorf("invalid store")
	}
	segments := make([]string, 0, len(pieces)-1)
	for _, piece := range pieces[1:] {
		segment, err := decodeResourceSegment(piece)
		if err != nil {
			return "", nil, err
		}
		segments = append(segments, segment)
	}
	return pieces[0], segments, nil
}

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

func ResultError(op *pb.Operation, outcome pb.MutationOutcome, f *pb.Failure) *pb.Result {
	r := &pb.Result{Index: op.GetIndex()}
	if op.GetRead() != nil {
		r.Result = &pb.Result_Read{Read: ReadFailure(f)}
	} else {
		r.Result = &pb.Result_Mutation{Mutation: Mutation(outcome, f)}
	}
	return r
}

func Validate(op *pb.Operation, store string) *pb.Failure {
	if op == nil || proto.Size(op) > MaxFrame {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or oversized operation")
	}
	var resource string
	var failure *pb.Failure
	if read := op.GetRead(); read != nil {
		resource = read.Resource
		failure = validateReadFields(read)
	} else if mutation := op.GetMutate(); mutation != nil {
		resource = mutation.Resource
		failure = validateMutationFields(mutation)
	} else {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing operation")
	}
	if failure != nil {
		return failure
	}
	name, segments, err := ParseResource(resource)
	if err != nil || len(segments) == 0 || name != store {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid or wrong-store resource")
	}
	return nil
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

func Resource(op *pb.Operation) string {
	if r := op.GetRead(); r != nil {
		return r.Resource
	}
	return op.GetMutate().GetResource()
}

func ValidateScan(req *pb.ScanRequest, store string) *pb.Failure {
	if req == nil || proto.Size(req) > MaxFrame {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "missing or oversized Scan")
	}
	name, segments, err := ParseResource(req.Resource)
	if err != nil || name != store || len(segments) == 0 {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid or wrong-store Scan resource")
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

func ValidateNative(open *pb.NativeOpen, store string) *pb.Failure {
	if open == nil || proto.Size(open) > NativeDescriptor+MaxURI+256 || open.Descriptor_ == nil || len(open.Descriptor_.Data) > NativeDescriptor {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native Open bounds")
	}
	name, _, err := ParseResource(open.Resource)
	if err != nil || name != store {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Native resource")
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
	if req == nil || !ValidStoreName(req.StoreName) || proto.Size(req) > MaxFrame || hasUnknown(req.ProtoReflect()) {
		return fmt.Errorf("invalid Execute request envelope")
	}
	return ValidateCommand(req.Command)
}

func ValidateExecuteResponse(response *pb.ExecuteResponse) error {
	if response == nil || proto.Size(response) > MaxResponse || hasUnknown(response.ProtoReflect()) {
		return fmt.Errorf("invalid Execute response envelope")
	}
	return ValidateEvent(response.Event)
}

// ValidateCommand requires a versioned Scan or Native with a canonical relative resource.
func ValidateCommand(command *pb.Command) error {
	if command == nil || command.Version != 1 || command.Operation == nil || proto.Size(command) > MaxPayload || hasUnknown(command.ProtoReflect()) {
		return fmt.Errorf("unsupported command version, fields or bounds")
	}
	var resource string
	switch operation := command.Operation.(type) {
	case *pb.Command_Scan:
		if operation.Scan != nil {
			resource = operation.Scan.Resource
		}
	case *pb.Command_Native:
		if operation.Native != nil && operation.Native.Open != nil {
			resource = operation.Native.Open.Resource
		}
	}
	if !validRelativeResource(resource) {
		return fmt.Errorf("command requires a canonical relative resource")
	}
	if scan := command.GetScan(); scan != nil {
		normalized := &pb.ScanRequest{Resource: "weir://target/" + scan.Resource, Selector: scan.Selector, ReadMediaType: scan.ReadMediaType, PageSize: scan.PageSize, ContinuationToken: scan.ContinuationToken}
		if failure := ValidateScan(normalized, "target"); failure != nil {
			return fmt.Errorf("%s", failure.Message)
		}
	} else if native := command.GetNative(); native != nil {
		open := native.Open
		normalized := &pb.NativeOpen{Resource: "weir://target/" + open.Resource, Descriptor_: open.Descriptor_, BodyMediaType: open.BodyMediaType}
		if failure := ValidateNative(normalized, "target"); failure != nil {
			return fmt.Errorf("%s", failure.Message)
		}
	}
	return nil
}

func validRelativeResource(resource string) bool {
	if resource == "" || len(resource) > MaxURI-len("weir://target/") {
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

// Unknown fields are rejected throughout the typed payload. Adding fields to
// version 1 therefore requires every client and executor to update together.
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
	if event == nil || event.Version != 1 || event.Value == nil || hasUnknown(event.ProtoReflect()) || proto.Size(event) > MaxEvent {
		return fmt.Errorf("invalid event version or bounds")
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
