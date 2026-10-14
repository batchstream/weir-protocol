package protocol

import (
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

func validateProjection(projection *pb.Projection) *pb.Failure {
	if projection == nil {
		return nil
	}
	if projection.Mode != pb.ProjectionMode_INCLUDE && projection.Mode != pb.ProjectionMode_EXCLUDE || len(projection.Fields) == 0 {
		return Fail(pb.FailureCode_INVALID_ARGUMENT, "Projection requires one mode and nonempty fields")
	}
	fields := make(map[string]struct{}, len(projection.Fields))
	for _, field := range projection.Fields {
		if !validProjectionField(field) {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "invalid Projection field path")
		}
		if _, exists := fields[field]; exists {
			return Fail(pb.FailureCode_INVALID_ARGUMENT, "Projection fields repeat")
		}
		fields[field] = struct{}{}
	}
	for _, field := range projection.Fields {
		for index := range field {
			if field[index] == '.' {
				if _, exists := fields[field[:index]]; exists {
					return Fail(pb.FailureCode_INVALID_ARGUMENT, "Projection field paths overlap")
				}
			}
		}
	}
	return nil
}

func validProjectionField(field string) bool {
	if field == "" || !utf8.ValidString(field) || strings.ContainsAny(field, "*?") {
		return false
	}
	for _, segment := range strings.Split(field, ".") {
		if segment == "" {
			return false
		}
		for _, value := range segment {
			if unicode.IsControl(value) {
				return false
			}
		}
	}
	return true
}
