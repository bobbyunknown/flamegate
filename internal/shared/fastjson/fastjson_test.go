package fastjson

import (
	"bytes"
	"strings"
	"testing"
)

type sample struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags,omitempty"`
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	in := sample{Name: "flamegate", Count: 42, Tags: []string{"a", "b"}}
	data, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out sample
	if err := Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || out.Count != in.Count || len(out.Tags) != 2 {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestMarshalNoHTMLEscape(t *testing.T) {
	// Matches encoding/json config: no HTML escaping.
	data, err := Marshal(map[string]string{"expr": "1 < 2 && 3 > 2"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `\u003c`) {
		t.Fatalf("output HTML-escaped unexpectedly: %s", data)
	}
}

func TestMarshalIndent(t *testing.T) {
	data, err := MarshalIndent(sample{Name: "x", Count: 1}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !strings.Contains(string(data), "\n  ") {
		t.Fatalf("MarshalIndent output not indented: %s", data)
	}
}

func TestValid(t *testing.T) {
	if !Valid([]byte(`{"a":1}`)) {
		t.Fatal("Valid returned false for valid JSON")
	}
	if Valid([]byte(`{"a":}`)) {
		t.Fatal("Valid returned true for invalid JSON")
	}
}

func TestNewEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(sample{Name: "stream", Count: 7}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	dec := NewDecoder(&buf)
	var out sample
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.Name != "stream" || out.Count != 7 {
		t.Fatalf("decoded %+v, want {stream 7}", out)
	}
}

func TestUnmarshalError(t *testing.T) {
	var out sample
	if err := Unmarshal([]byte(`{"count": "not-an-int"}`), &out); err == nil {
		t.Fatal("expected error unmarshaling wrong type")
	}
}

func TestRawMessage(t *testing.T) {
	// RawMessage should be usable as a deferred-parse field.
	type wrapper struct {
		Payload RawMessage `json:"payload"`
	}
	var w wrapper
	if err := Unmarshal([]byte(`{"payload":{"nested":true}}`), &w); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !strings.Contains(string(w.Payload), "nested") {
		t.Fatalf("RawMessage = %s, want raw nested object", w.Payload)
	}
}
