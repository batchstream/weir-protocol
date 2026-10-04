package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/protobuf/proto"
)

const DefaultScanPageSize = 128
const MaxScanPageSize = 256
const MaxScanToken = 32 << 10
const MaxScanState = 20 << 10

func ScanPageSize(request *pb.ScanRequest) uint64 {
	if request.PageSize == 0 {
		return DefaultScanPageSize
	}
	return uint64(request.PageSize)
}

// Continuations carry bounded native backend state. The checksum detects
// corruption only; they provide no authentication or authorization.
type scanContinuation struct {
	Backend     string `json:"backend"`
	Fingerprint string `json:"fingerprint"`
	State       []byte `json:"state"`
	Checksum    string `json:"checksum"`
}

// ScanFingerprint binds traversal settings to the Store and backend profile.
// Page size and continuation tokens do not change the traversal identity.
func ScanFingerprint(request *pb.ScanRequest, store, backend string) string {
	copy := proto.Clone(request).(*pb.ScanRequest)
	copy.PageSize = 0
	copy.ContinuationToken = nil
	options := proto.MarshalOptions{Deterministic: true}
	raw, _ := options.Marshal(copy)
	sum := sha256.Sum256(append([]byte(store+"\x00"+backend+"\x00"), raw...))
	return hex.EncodeToString(sum[:])
}

func EncodeScanToken(backend, fingerprint string, state []byte) ([]byte, error) {
	if len(state) == 0 || len(state) > MaxScanState {
		return nil, errors.New("Scan continuation state exceeds bound")
	}
	value := scanContinuation{Backend: backend, Fingerprint: fingerprint, State: state}
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	value.Checksum = hex.EncodeToString(sum[:])
	raw, _ = json.Marshal(value)
	if len(raw) > MaxScanToken {
		return nil, errors.New("Scan continuation exceeds bound")
	}
	return raw, nil
}

func DecodeScanToken(token []byte, backend, fingerprint string) ([]byte, error) {
	if len(token) == 0 || len(token) > MaxScanToken {
		return nil, errors.New("invalid Scan continuation size")
	}
	decoder := json.NewDecoder(bytes.NewReader(token))
	decoder.DisallowUnknownFields()
	var value scanContinuation
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid Scan continuation")
	}
	canonical, _ := json.Marshal(value)
	if !bytes.Equal(canonical, token) || value.Backend != backend || value.Fingerprint != fingerprint || len(value.State) == 0 || len(value.State) > MaxScanState {
		return nil, errors.New("Scan continuation does not match request")
	}
	checksum := value.Checksum
	value.Checksum = ""
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	if checksum != hex.EncodeToString(sum[:]) {
		return nil, errors.New("invalid Scan continuation checksum")
	}
	return value.State, nil
}
