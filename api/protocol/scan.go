package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
)

const DefaultScanPageSize = 128
const MaxScanPageSize = 256
const MaxScanToken = 32 << 10
const MaxScanState = 20 << 10

const scanTokenVersion = 1

func ScanPageSize(request *pb.ScanRequest) uint64 {
	if request.PageSize == 0 {
		return DefaultScanPageSize
	}
	return uint64(request.PageSize)
}

// Continuations carry bounded native backend state. The checksum detects
// corruption only; they provide no authentication or authorization.
type scanContinuation struct {
	Version     uint32 `json:"version"`
	Backend     string `json:"backend"`
	Fingerprint string `json:"fingerprint"`
	State       []byte `json:"state"`
	Checksum    string `json:"checksum"`
}

// ScanFingerprint binds traversal settings to the Store and backend profile.
// Named, length-prefixed fields define the identity independently of protobuf
// serialization. Presence, filter bytes, and projection field order are preserved.
// Page size and continuation tokens do not change the traversal identity.
func ScanFingerprint(request *pb.ScanRequest, store, backend string) string {
	raw := appendScanField(nil, "format", []byte("weir.scan.traversal.v1"))
	raw = appendScanField(raw, "store", []byte(store))
	raw = appendScanField(raw, "backend", []byte(backend))
	raw = appendScanField(raw, "resource", []byte(request.Resource))
	filter := request.Filter
	if filter == nil {
		raw = appendScanField(raw, "filter.present", []byte{0})
	} else {
		raw = appendScanField(raw, "filter.present", []byte{1})
		raw = appendScanField(raw, "filter.content_type", []byte(filter.ContentType))
		raw = appendScanField(raw, "filter.data", filter.Data)
	}
	projection := request.Projection
	if projection == nil {
		raw = appendScanField(raw, "projection.present", []byte{0})
	} else {
		raw = appendScanField(raw, "projection.present", []byte{1})
		raw = appendScanField(raw, "projection.mode", binary.BigEndian.AppendUint32(nil, uint32(projection.Mode)))
		raw = appendScanField(raw, "projection.count", binary.BigEndian.AppendUint32(nil, uint32(len(projection.Fields))))
		for _, field := range projection.Fields {
			raw = appendScanField(raw, "projection.field", []byte(field))
		}
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func appendScanField(raw []byte, name string, value []byte) []byte {
	raw = binary.BigEndian.AppendUint32(raw, uint32(len(name)))
	raw = append(raw, name...)
	raw = binary.BigEndian.AppendUint32(raw, uint32(len(value)))
	return append(raw, value...)
}

func EncodeScanToken(backend, fingerprint string, state []byte) ([]byte, error) {
	if len(state) == 0 || len(state) > MaxScanState {
		return nil, errors.New("Scan continuation state exceeds bound")
	}
	value := scanContinuation{Version: scanTokenVersion, Backend: backend, Fingerprint: fingerprint, State: state}
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
	if value.Version != scanTokenVersion {
		return nil, errors.New("unsupported Scan continuation version")
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
