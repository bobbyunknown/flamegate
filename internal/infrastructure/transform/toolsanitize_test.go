package transform

import (
	"encoding/json"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestToolArgSanitizer_PassesThroughNonToolChunks(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk

	s.Process(core.StreamChunk{Type: core.ChunkText, Delta: "hello"}, func(c core.StreamChunk) {
		emitted = append(emitted, c)
	})

	if len(emitted) != 1 || emitted[0].Delta != "hello" {
		t.Errorf("non-tool chunk not passed through: %+v", emitted)
	}
}

func TestToolArgSanitizer_BuffersAndFlushes(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk

	// First chunk: tool call start with ID.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID:        "tc1",
			Name:      "Read",
			Arguments: json.RawMessage("{}"),
		},
	}, func(c core.StreamChunk) { emitted = append(emitted, c) })

	// Second chunk: argument delta.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			Arguments: json.RawMessage(`{"file_path":"/tmp/a.txt"}`),
		},
	}, func(c core.StreamChunk) { emitted = append(emitted, c) })

	// Nothing emitted yet (buffering).
	if len(emitted) != 0 {
		t.Errorf("expected 0 emitted during buffering, got %d", len(emitted))
	}

	// Flush.
	s.Flush(func(c core.StreamChunk) { emitted = append(emitted, c) })

	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted on flush, got %d", len(emitted))
	}
	if emitted[0].ToolCall.ID != "tc1" {
		t.Errorf("expected ID 'tc1', got %q", emitted[0].ToolCall.ID)
	}
	if emitted[0].ToolCall.Name != "Read" {
		t.Errorf("expected Name 'Read', got %q", emitted[0].ToolCall.Name)
	}
}

func TestToolArgSanitizer_SanitizesReadLimit(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":"100"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if args["limit"] != float64(100) {
		t.Errorf("expected limit 100 (float64), got %v (%T)", args["limit"], args["limit"])
	}
}

func TestToolArgSanitizer_ClampsReadLimit(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":5000}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if args["limit"] != float64(2000) {
		t.Errorf("expected limit clamped to 2000, got %v", args["limit"])
	}
}

func TestToolArgSanitizer_ClampsReadLimitMin(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":0}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if args["limit"] != float64(1) {
		t.Errorf("expected limit clamped to 1, got %v", args["limit"])
	}
}

func TestToolArgSanitizer_SanitizesReadOffset(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","offset":"-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if args["offset"] != float64(0) {
		t.Errorf("expected offset clamped to 0, got %v", args["offset"])
	}
}

func TestToolArgSanitizer_RemovesPagesForNonPdf(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","pages":"1-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if _, ok := args["pages"]; ok {
		t.Error("expected 'pages' to be removed for non-PDF file")
	}
}

func TestToolArgSanitizer_KeepsPagesForPdf(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/doc.pdf","pages":"1-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args) //nolint:errcheck // test helper

	if args["pages"] != "1-5" {
		t.Errorf("expected pages '1-5' for PDF, got %v", args["pages"])
	}
}

func TestToolArgSanitizer_FlushOnNewToolAtSameIndex(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk
	emit := func(c core.StreamChunk) { emitted = append(emitted, c) }

	// First tool call at index 0.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID: "tc1", Name: "Read",
			Arguments: json.RawMessage(`{"file_path":"/a"}`),
		},
	}, emit)

	// Second tool call at same index 0 — should flush first.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID: "tc2", Name: "Write",
			Arguments: json.RawMessage(`{"file_path":"/b","content":"hi"}`),
		},
	}, emit)

	if len(emitted) != 1 {
		t.Fatalf("expected 1 flushed tool call, got %d", len(emitted))
	}
	if emitted[0].ToolCall.ID != "tc1" {
		t.Errorf("expected first tool 'tc1', got %q", emitted[0].ToolCall.ID)
	}

	// Flush remaining.
	s.Flush(emit)
	if len(emitted) != 2 {
		t.Fatalf("expected 2 total, got %d", len(emitted))
	}
	if emitted[1].ToolCall.ID != "tc2" {
		t.Errorf("expected second tool 'tc2', got %q", emitted[1].ToolCall.ID)
	}
}

// TestToolArgSanitizer_KiroRepeatedIDDelta reproduces the Kiro streaming shape:
// the connector emits an opening chunk (ID + Name + empty args) followed by an
// argument chunk that REPEATS the same tool-use ID. The sanitizer must treat
// the second chunk as a continuation of the same call, not a brand-new one.
// Before the fix, the repeated ID prematurely flushed the empty buffer (a tool
// call with "{}" args) and accumulated the real arguments into a second,
// name-less buffer — which made Cline see "read_file" with no parameters.
func TestToolArgSanitizer_KiroRepeatedIDDelta(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk
	emit := func(c core.StreamChunk) { emitted = append(emitted, c) }

	// Opening chunk: ID + Name + empty args (Kiro toolUseEvent announce).
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID:        "tooluse_1",
			Name:      "read_file",
			Arguments: json.RawMessage(""),
		},
	}, emit)

	// Argument chunk: SAME ID repeated, carries the real input.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID:        "tooluse_1",
			Arguments: json.RawMessage(`{"path":"src/main.go"}`),
		},
	}, emit)

	// Nothing should be emitted while buffering — no premature flush.
	if len(emitted) != 0 {
		t.Fatalf("expected 0 emitted during buffering, got %d: %+v", len(emitted), emitted)
	}

	// Finish flushes exactly one consolidated tool call.
	s.Process(core.StreamChunk{Type: core.ChunkFinish, FinishReason: core.FinishToolCalls}, emit)

	var tools []core.StreamChunk
	for _, c := range emitted {
		if c.Type == core.ChunkToolCall {
			tools = append(tools, c)
		}
	}
	if len(tools) != 1 {
		t.Fatalf("expected exactly 1 tool call, got %d: %+v", len(tools), tools)
	}
	tc := tools[0].ToolCall
	if tc.Name != "read_file" {
		t.Errorf("expected Name 'read_file', got %q", tc.Name)
	}
	if tc.ID != "tooluse_1" {
		t.Errorf("expected ID 'tooluse_1', got %q", tc.ID)
	}
	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("arguments not valid JSON: %v (%s)", err, tc.Arguments)
	}
	if args["path"] != "src/main.go" {
		t.Errorf("expected path 'src/main.go', got %v (full args: %s)", args["path"], tc.Arguments)
	}
}

// TestToolArgSanitizer_KiroFragmentedArgs covers Kiro emitting the arguments as
// multiple fragments after the announce chunk, all repeating the same ID. They
// must accumulate into one complete JSON object.
func TestToolArgSanitizer_KiroFragmentedArgs(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk
	emit := func(c core.StreamChunk) { emitted = append(emitted, c) }

	s.Process(core.StreamChunk{
		Type: core.ChunkToolCall, Index: 0,
		ToolCall: &core.ToolCall{ID: "t1", Name: "read_file", Arguments: json.RawMessage("")},
	}, emit)
	for _, frag := range []string{`{"path":`, `"a/b/`, `c.go"}`} {
		s.Process(core.StreamChunk{
			Type: core.ChunkToolCall, Index: 0,
			ToolCall: &core.ToolCall{ID: "t1", Arguments: json.RawMessage(frag)},
		}, emit)
	}
	s.Flush(emit)

	if len(emitted) != 1 {
		t.Fatalf("expected 1 tool call, got %d: %+v", len(emitted), emitted)
	}
	var args map[string]any
	if err := json.Unmarshal(emitted[0].ToolCall.Arguments, &args); err != nil {
		t.Fatalf("reassembled args invalid: %v (%s)", err, emitted[0].ToolCall.Arguments)
	}
	if args["path"] != "a/b/c.go" {
		t.Errorf("expected path 'a/b/c.go', got %v", args["path"])
	}
}

func TestSanitizeToolArgs_InvalidJSON(t *testing.T) {

	// Should return original if unparseable.
	got := sanitizeToolArgs("Read", "not json")
	if got != "not json" {
		t.Errorf("expected passthrough for invalid JSON, got %q", got)
	}
}

func TestSanitizeToolArgs_PassthroughValidArgs(t *testing.T) {
	input := `{"file_path":"/tmp/a.txt","limit":50}`
	got := sanitizeToolArgs("Read", input)
	var args map[string]any
	json.Unmarshal([]byte(got), &args) //nolint:errcheck // test helper

	if args["limit"] != float64(50) {
		t.Errorf("valid limit should pass through, got %v", args["limit"])
	}
}
