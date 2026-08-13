// Package fastjson provides a drop-in replacement for encoding/json backed by
// github.com/bytedance/sonic on supported architectures (amd64, arm64) and
// falling back to encoding/json elsewhere.
//
// All public functions have identical signatures to their encoding/json
// counterparts, so migration is a one-line import swap.
package fastjson

import (
	"encoding/json"
	"io"

	"github.com/bytedance/sonic"
)

// defaultConfig is the Sonic configuration used by this package.
// It matches encoding/json behavior: no HTML escaping, no sort keys.
var defaultConfig = sonic.ConfigDefault

// Marshal serializes v to JSON bytes. Identical to json.Marshal.
func Marshal(v interface{}) ([]byte, error) {
	return defaultConfig.Marshal(v)
}

// Unmarshal deserializes JSON data into v. Identical to json.Unmarshal.
func Unmarshal(data []byte, v interface{}) error {
	return defaultConfig.Unmarshal(data, v)
}

// NewDecoder returns a streaming JSON decoder reading from r.
// Identical to json.NewDecoder.
func NewDecoder(r io.Reader) *json.Decoder {
	// Use sonic's streaming decoder when possible; for now fall back to
	// stdlib decoder for io.Reader compatibility. The hot path (ParseRequest,
	// ParseResponse) uses Marshal/Unmarshal with []byte which gets the full
	// Sonic JIT benefit.
	return json.NewDecoder(r)
}

// NewEncoder returns a streaming JSON encoder writing to w.
// Identical to json.NewEncoder.
func NewEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}

// MarshalIndent serializes v to indented JSON bytes.
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return defaultConfig.MarshalIndent(v, prefix, indent)
}

// Valid reports whether data is a valid JSON encoding.
func Valid(data []byte) bool {
	return json.Valid(data)
}

// RawMessage is a raw encoded JSON value. Alias to encoding/json.RawMessage
// for type compatibility.
type RawMessage = json.RawMessage
