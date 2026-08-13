package transform

import (
	"bytes"
	"fmt"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

// The Responses API streams a rich, typed event sequence rather than uniform
// chunks:
//
//	response.created → response.in_progress →
//	response.output_item.added (message|reasoning|function_call) →
//	response.output_text.delta* / response.function_call_arguments.delta* /
//	response.reasoning_summary_text.delta* →
//	response.output_item.done → response.completed
//
// ParseStreamLine maps each event payload to canonical chunks (for routing a
// Responses provider like Codex back to a client). RenderStreamChunk produces
// the corresponding event sequence for a client that speaks the Responses
// dialect.

// respStreamEvent is one Responses SSE data payload.
type respStreamEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	ItemID   string          `json:"item_id"`
	Item     *respStreamItem `json:"item"`
	Response *struct {
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type respStreamItem struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
}

// ParseStreamLine converts one Responses SSE event payload into canonical chunks.
func (OpenAIResponsesCodec) ParseStreamLine(line []byte, _ string) ([]core.StreamChunk, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
		return nil, nil
	}

	var ev respStreamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, fmt.Errorf("openai-responses: parse stream event: %w", err)
	}

	switch ev.Type {
	case "response.output_text.delta":
		if ev.Delta == "" {
			return nil, nil
		}
		return []core.StreamChunk{{Type: core.ChunkText, Delta: ev.Delta}}, nil

	case "response.reasoning_summary_text.delta":
		if ev.Delta == "" {
			return nil, nil
		}
		return []core.StreamChunk{{Type: core.ChunkThinking, Delta: ev.Delta}}, nil

	case "response.output_item.added":
		if ev.Item != nil && (ev.Item.Type == "function_call" || ev.Item.Type == "custom_tool_call") {
			return []core.StreamChunk{{
				Type: core.ChunkToolCall,
				ToolCall: &core.ToolCall{
					ID:        ev.Item.CallID,
					Name:      ev.Item.Name,
					Arguments: json.RawMessage("{}"),
				},
			}}, nil
		}
		return nil, nil

	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		if ev.Delta == "" {
			return nil, nil
		}
		return []core.StreamChunk{{
			Type:     core.ChunkToolCall,
			ToolCall: &core.ToolCall{Arguments: json.RawMessage(ev.Delta)},
		}}, nil

	case "response.completed":
		var chunks []core.StreamChunk
		chunks = append(chunks, core.StreamChunk{Type: core.ChunkFinish, FinishReason: core.FinishStop})
		if ev.Response != nil && ev.Response.Usage != nil {
			u := ev.Response.Usage
			chunks = append(chunks, core.StreamChunk{
				Type: core.ChunkUsage,
				Usage: &core.Usage{
					PromptTokens:     u.InputTokens,
					CompletionTokens: u.OutputTokens,
					TotalTokens:      u.InputTokens + u.OutputTokens,
					CachedTokens:     u.InputTokensDetails.CachedTokens,
				},
			})
		}
		return chunks, nil

	case "error", "response.failed":
		msg := ""
		if ev.Error != nil {
			msg = ev.Error.Message
		} else if ev.Response != nil && ev.Response.Error != nil {
			msg = ev.Response.Error.Message
		}
		return []core.StreamChunk{{
			Type: core.ChunkError,
			Err:  &core.ProviderError{Kind: core.ErrUpstream, Message: msg},
		}}, nil

	default:
		// response.created, in_progress, output_item.done, content_part.*,
		// output_text.done, reasoning_summary_*: nothing canonical to emit.
		return nil, nil
	}
}

// respStreamState tracks per-stream rendering bookkeeping for the Responses
// event sequence, stashed in StreamState.Custom.
type respStreamState struct {
	seq          int
	started      bool
	responseID   string
	msgAdded     bool
	msgPartAdded bool
	msgText      string
	toolAdded    map[int]bool
	toolCallID   map[int]string
	toolName     map[int]string
	toolArgs     map[int]string
	completed    bool
}

func respState(state *StreamState) *respStreamState {
	if state.Custom == nil {
		state.Custom = map[string]any{}
	}
	if s, ok := state.Custom["resp"].(*respStreamState); ok {
		return s
	}
	s := &respStreamState{
		responseID: "resp_" + firstNonEmpty(state.MessageID, "stream"),
		toolAdded:  map[int]bool{},
		toolCallID: map[int]string{},
		toolName:   map[int]string{},
		toolArgs:   map[int]string{},
	}
	state.Custom["resp"] = s
	return s
}

// RenderStreamChunk emits Responses API event(s) for a canonical chunk.
func (OpenAIResponsesCodec) RenderStreamChunk(chunk core.StreamChunk, state *StreamState) ([][]byte, error) {
	s := respState(state)
	var events [][]byte
	emit := func(eventType string, data map[string]any) {
		s.seq++
		data["sequence_number"] = s.seq
		data["type"] = eventType
		events = append(events, respEvent(eventType, data))
	}

	ensureStarted := func() {
		if s.started {
			return
		}
		s.started = true
		emit("response.created", map[string]any{
			"response": map[string]any{
				"id": s.responseID, "object": "response", "status": "in_progress", "output": []any{},
			},
		})
		emit("response.in_progress", map[string]any{
			"response": map[string]any{"id": s.responseID, "object": "response", "status": "in_progress"},
		})
	}

	switch chunk.Type {
	case core.ChunkText:
		ensureStarted()
		msgID := "msg_" + s.responseID + "_0"
		if !s.msgAdded {
			s.msgAdded = true
			emit("response.output_item.added", map[string]any{
				"output_index": 0,
				"item":         map[string]any{"id": msgID, "type": "message", "role": "assistant", "content": []any{}},
			})
		}
		if !s.msgPartAdded {
			s.msgPartAdded = true
			emit("response.content_part.added", map[string]any{
				"item_id": msgID, "output_index": 0, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
			})
		}
		s.msgText += chunk.Delta
		emit("response.output_text.delta", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "delta": chunk.Delta,
		})

	case core.ChunkThinking:
		ensureStarted()
		rsID := "rs_" + s.responseID + "_0"
		if _, ok := state.Custom["resp_reasoning_added"]; !ok {
			state.Custom["resp_reasoning_added"] = true
			emit("response.output_item.added", map[string]any{
				"output_index": 0,
				"item":         map[string]any{"id": rsID, "type": "reasoning", "summary": []any{}},
			})
		}
		emit("response.reasoning_summary_text.delta", map[string]any{
			"item_id": rsID, "output_index": 0, "summary_index": 0, "delta": chunk.Delta,
		})

	case core.ChunkToolCall:
		if chunk.ToolCall == nil {
			return nil, nil
		}
		ensureStarted()
		idx := chunk.Index
		if chunk.ToolCall.Name != "" {
			s.toolName[idx] = chunk.ToolCall.Name
		}
		if chunk.ToolCall.ID != "" && !s.toolAdded[idx] {
			s.toolAdded[idx] = true
			s.toolCallID[idx] = chunk.ToolCall.ID
			emit("response.output_item.added", map[string]any{
				"output_index": idx,
				"item": map[string]any{
					"id": "fc_" + chunk.ToolCall.ID, "type": "function_call",
					"call_id": chunk.ToolCall.ID, "name": s.toolName[idx], "arguments": "",
				},
			})
		}
		if args := string(chunk.ToolCall.Arguments); args != "" && args != "{}" {
			callID := s.toolCallID[idx]
			s.toolArgs[idx] += args
			emit("response.function_call_arguments.delta", map[string]any{
				"item_id": "fc_" + callID, "output_index": idx, "delta": args,
			})
		}

	case core.ChunkFinish:
		// Close any open message/tool items, then complete.
		if s.msgAdded {
			msgID := "msg_" + s.responseID + "_0"
			emit("response.output_text.done", map[string]any{
				"item_id": msgID, "output_index": 0, "content_index": 0, "text": s.msgText,
			})
			emit("response.output_item.done", map[string]any{
				"output_index": 0,
				"item": map[string]any{
					"id": msgID, "type": "message", "role": "assistant",
					"content": []map[string]any{{"type": "output_text", "text": s.msgText, "annotations": []any{}}},
				},
			})
		}
		for idx, callID := range s.toolCallID {
			args := s.toolArgs[idx]
			if args == "" {
				args = "{}"
			}
			emit("response.function_call_arguments.done", map[string]any{
				"item_id": "fc_" + callID, "output_index": idx, "arguments": args,
			})
			emit("response.output_item.done", map[string]any{
				"output_index": idx,
				"item": map[string]any{
					"id": "fc_" + callID, "type": "function_call",
					"call_id": callID, "name": s.toolName[idx], "arguments": args,
				},
			})
		}

	case core.ChunkUsage:
		if chunk.Usage == nil {
			return nil, nil
		}
		ensureStarted()
		if !s.completed {
			s.completed = true
			emit("response.completed", map[string]any{
				"response": map[string]any{
					"id": s.responseID, "object": "response", "status": "completed",
					"usage": map[string]int{
						"input_tokens":  chunk.Usage.PromptTokens,
						"output_tokens": chunk.Usage.CompletionTokens,
						"total_tokens":  chunk.Usage.TotalTokens,
					},
				},
			})
		}

	default:
		return nil, nil
	}
	return events, nil
}

// RenderStreamDone emits response.completed if no usage chunk already did.
func (OpenAIResponsesCodec) RenderStreamDone(state *StreamState) [][]byte {
	s := respState(state)
	if s.completed || !s.started {
		return nil
	}
	s.completed = true
	s.seq++
	return [][]byte{respEvent("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": s.seq,
		"response": map[string]any{
			"id": s.responseID, "object": "response", "status": "completed",
		},
	})}
}

// respEvent formats a Responses API SSE event: "event: <name>\ndata: <json>\n\n".
func respEvent(name string, payload map[string]any) []byte {
	b, _ := json.Marshal(payload)
	out := make([]byte, 0, len(name)+len(b)+20)
	out = append(out, "event: "...)
	out = append(out, name...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, b...)
	out = append(out, '\n', '\n')
	return out
}
