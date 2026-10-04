package protocol

import (
	"bytes"
	"strings"
	"testing"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

func TestScanContinuationBindingAndBounds(t *testing.T) {
	filter := &pb.Document{ContentType: "application/json", Data: []byte(`{"match_all":{}}`)}
	request := &pb.ScanRequest{Resource: "records", Filter: filter, PageSize: 2}
	fingerprint := ScanFingerprint(request, "example", "example:ordered")
	state := []byte(`{"pit":"native","after":3}`)
	token, err := EncodeScanToken("example:ordered", fingerprint, state)
	if err != nil {
		t.Fatal(err)
	}
	request.PageSize = 3
	request.ContinuationToken = token
	if ScanFingerprint(request, "example", "example:ordered") != fingerprint {
		t.Fatal("changing page size changed traversal identity")
	}
	decoded, err := DecodeScanToken(token, "example:ordered", fingerprint)
	if err != nil || !bytes.Equal(decoded, state) {
		t.Fatal("continuation roundtrip", err)
	}
	for _, resource := range []string{"other", "records/s:other"} {
		request.Resource = resource
		if _, err := DecodeScanToken(token, "example:ordered", ScanFingerprint(request, "example", "example:ordered")); err == nil {
			t.Fatal("accepted different traversal", resource)
		}
	}
	request.Resource = "records"
	otherStore := ScanFingerprint(request, "other", "example:ordered")
	if _, err := DecodeScanToken(token, "example:ordered", otherStore); err == nil {
		t.Fatal("accepted continuation from another Store")
	}
	if _, err := DecodeScanToken(token, "example:other", fingerprint); err == nil {
		t.Fatal("accepted another backend dialect")
	}
	corrupt := bytes.Replace(token, []byte(`"checksum":"`), []byte(`"checksum":"x`), 1)
	for _, invalid := range [][]byte{nil, append(token, ' '), corrupt, []byte(strings.Repeat("x", MaxScanToken+1)), []byte(`{"state":"broken"}`)} {
		if _, err := DecodeScanToken(invalid, "example:ordered", fingerprint); err == nil {
			t.Fatal("accepted malformed continuation")
		}
	}
	if _, err := EncodeScanToken("example:ordered", fingerprint, make([]byte, MaxScanState+1)); err == nil {
		t.Fatal("encoded excessive native state")
	}
}

func TestScanEndRequiresOneSuccessCheckpoint(t *testing.T) {
	failure := Fail(pb.FailureCode_UNAVAILABLE, "fixture")
	cases := []struct {
		end   *pb.ScanEnd
		valid bool
	}{
		{end: &pb.ScanEnd{Exhausted: true}, valid: true},
		{end: &pb.ScanEnd{NextContinuationToken: []byte("token")}, valid: true},
		{end: &pb.ScanEnd{Failure: failure}, valid: true},
		{end: &pb.ScanEnd{}, valid: false},
		{end: &pb.ScanEnd{Exhausted: true, NextContinuationToken: []byte("token")}, valid: false},
		{end: &pb.ScanEnd{Failure: failure, Exhausted: true}, valid: false},
		{end: &pb.ScanEnd{Failure: failure, NextContinuationToken: []byte("token")}, valid: false},
	}
	for _, test := range cases {
		variant := &pb.Event_ScanEnd{ScanEnd: test.end}
		event := &pb.Event{Value: variant}
		if (ValidateEvent(event) == nil) != test.valid {
			t.Fatal("invalid ScanEnd evidence accepted", test.end)
		}
	}
}
