package transform

import (
	"bytes"
	"fmt"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

// Gemini streams partial generateContent responses: each SSE data line is a
// full GenerateContentResponse fragment carrying incremental candidate parts
// and (on the final line) usageMetadata + finishReason. ParseStreamLine maps
// each fragment to canonical chunks; RenderStreamChunk produces the same wire
// shape for a client that speaks Gemini.

// gemStreamChunk is one SSE "data:" payload from streamGenerateContent.
type gemStreamChunk struct {
	Candidates []struct {
		Content      gemContent `json:"content"`
		FinishReason string     `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

// ParseStreamLine converts one Gemini SSE data payload into canonical chunks.
func (GeminiCodec) ParseStreamLine(line []byte, _ string) ([]core.StreamChunk, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}

	var raw gemStreamChunk
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("gemini: parse stream chunk: %w", err)
	}

	var chunks []core.StreamChunk
	if len(raw.Candidates) > 0 {
		cand := raw.Candidates[0]
		for _, p := range cand.Content.Parts {
			switch {
			case p.FunctionCall != nil:
				chunks = append(chunks, core.StreamChunk{
					Type: core.ChunkToolCall,
					ToolCall: &core.ToolCall{
						ID:        geminiCallID(p.FunctionCall.Name),
						Name:      p.FunctionCall.Name,
						Arguments: p.FunctionCall.Args,
					},
				})
			case p.Text != "":
				chunks = append(chunks, core.StreamChunk{Type: core.ChunkText, Delta: p.Text})
			}
		}
		if cand.FinishReason != "" {
			chunks = append(chunks, core.StreamChunk{
				Type:         core.ChunkFinish,
				FinishReason: mapGemFinish(cand.FinishReason),
			})
		}
	}

	if raw.UsageMetadata != nil {
		chunks = append(chunks, core.StreamChunk{
			Type: core.ChunkUsage,
			Usage: &core.Usage{
				PromptTokens:     raw.UsageMetadata.PromptTokenCount,
				CompletionTokens: raw.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      raw.UsageMetadata.TotalTokenCount,
				CachedTokens:     raw.UsageMetadata.CachedContentTokenCount,
			},
		})
	}
	return chunks, nil
}

// RenderStreamChunk encodes a canonical chunk as a Gemini SSE event. Gemini
// streams each fragment as a standalone GenerateContentResponse, so text,
// tool-call, finish, and usage chunks each become one "data:" line.
func (GeminiCodec) RenderStreamChunk(chunk core.StreamChunk, _ *StreamState) ([][]byte, error) {
	switch chunk.Type {
	case core.ChunkText:
		return [][]byte{gemEvent(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": chunk.Delta}},
				},
				"index": 0,
			}},
		})}, nil

	case core.ChunkToolCall:
		if chunk.ToolCall == nil {
			return nil, nil
		}
		args := chunk.ToolCall.Arguments
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		return [][]byte{gemEvent(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{{
						"functionCall": map[string]any{
							"name": chunk.ToolCall.Name,
							"args": args,
						},
					}},
				},
				"index": 0,
			}},
		})}, nil

	case core.ChunkFinish:
		return [][]byte{gemEvent(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []any{}},
				"finishReason": renderGemFinish(chunk.FinishReason),
				"index":        0,
			}},
		})}, nil

	case core.ChunkUsage:
		if chunk.Usage == nil {
			return nil, nil
		}
		return [][]byte{gemEvent(map[string]any{
			"candidates": []any{},
			"usageMetadata": map[string]int{
				"promptTokenCount":     chunk.Usage.PromptTokens,
				"candidatesTokenCount": chunk.Usage.CompletionTokens,
				"totalTokenCount":      chunk.Usage.TotalTokens,
			},
		})}, nil

	default:
		return nil, nil
	}
}

// RenderStreamDone has no terminal sentinel for Gemini SSE; the connection
// simply closes after the final fragment.
func (GeminiCodec) RenderStreamDone(_ *StreamState) [][]byte { return nil }

// gemEvent formats a Gemini SSE event: "data: <json>\n\n".
func gemEvent(payload map[string]any) []byte {
	b, _ := json.Marshal(payload)
	return append([]byte("data: "), append(b, '\n', '\n')...)
}
