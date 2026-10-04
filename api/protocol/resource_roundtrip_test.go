package protocol

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestResourceSegmentEncoding(t *testing.T) {
	tests := []struct {
		input   string
		encoded string
	}{
		{input: ""},
		{input: "AZaz09-._~:", encoded: "AZaz09-._~:"},
		{input: "a/b", encoded: "a%2Fb"},
		{input: "a%2Fb", encoded: "a%252Fb"},
		{input: " +@&=$?#", encoded: "%20%2B%40%26%3D%24%3F%23"},
		{input: "中", encoded: "%E4%B8%AD"},
		{input: "\x00\xff", encoded: "%00%FF"},
	}
	for _, test := range tests {
		if actual := EncodeSegment(test.input); actual != test.encoded {
			t.Fatalf("encode %q: got %q, want %q", test.input, actual, test.encoded)
		}
	}
}

func FuzzResourceSegmentRoundTrip(f *testing.F) {
	for _, input := range []string{"data", "s:key", "a/b", "%2F", "中", " +@&=$?#", "", ".", "..", "\x00", "\xff", "\u0085"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > MaxURI {
			t.Skip()
		}
		encoded := EncodeSegment(input)
		resource := encoded
		expected := input != "" && input != "." && input != ".." && utf8.ValidString(input) && len(resource) <= MaxURI
		for _, value := range input {
			expected = expected && !unicode.IsControl(value)
		}
		parts, err := ParseRelativeResource(resource)
		if (err == nil) != expected {
			t.Fatalf("input %q expected valid=%v: %v", input, expected, err)
		}
		if expected && (len(parts) != 1 || parts[0] != input) {
			t.Fatalf("segment changed: parts %q", parts)
		}
		if strings.Contains(encoded, "/") {
			t.Fatal("encoded segment contains a path separator")
		}
	})
}
