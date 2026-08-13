package transform

import (
	"bytes"
	"fmt"

	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// AnthropicCodec handles the Anthropic Messages wire format (/v1/messages).
type AnthropicCodec struct{}

func (AnthropicCodec) Dialect() core.Dialect { return core.DialectAnthropic }

// ---- wire types -------------------------------------------------------------

type antRequest struct {
	Model      string          `json:"model"`
	System     json.RawMessage `json:"system,omitempty"`
	Messages   []antMessage    `json:"messages"`
	Tools      []antTool       `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens"`
	Stream     bool            `json:"stream,omitempty"`
	Temp       *float64        `json:"temperature,omitempty"`
	TopP       *float64        `json:"top_p,omitempty"`
	Stop       []string        `json:"stop_sequences,omitempty"`
}

type antMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type antBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *antImageSource `json:"source,omitempty"`
}

type antImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type antTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ---- request parsing --------------------------------------------------------

func (AnthropicCodec) ParseRequest(body []byte) (*core.ChatRequest, error) {
	var raw antRequest
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse request: %w", err)
	}

	maxTokens := raw.MaxTokens
	req := &core.ChatRequest{
		Model:       raw.Model,
		System:      decodeAntSystem(raw.System),
		Temperature: raw.Temp,
		TopP:        raw.TopP,
		Stop:        raw.Stop,
		Stream:      raw.Stream,
	}
	if maxTokens > 0 {
		req.MaxTokens = &maxTokens
	}

	for _, t := range raw.Tools {
		req.Tools = append(req.Tools, core.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	req.ToolChoice = claudeToolChoiceToOpenAI(raw.ToolChoice)

	for _, m := range raw.Messages {
		req.Messages = append(req.Messages, parseAntMessage(m))
	}
	return req, nil
}

func decodeAntSystem(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []antBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var out string
	for _, b := range blocks {
		if b.Type == "text" {
			out += b.Text
		}
	}
	return out
}

func parseAntMessage(m antMessage) core.Message {
	msg := core.Message{Role: mapAntRole(m.Role)}

	// Content may be a plain string or an array of blocks.
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: s})
		return msg
	}

	var blocks []antBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return msg
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: b.Text})
		case "thinking":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: b.Text})
		case "tool_use":
			msg.Content = append(msg.Content, core.ContentPart{
				Type:     core.PartToolCall,
				ToolCall: &core.ToolCall{ID: b.ID, Name: b.Name, Arguments: b.Input},
			})
		case "tool_result":
			msg.Content = append(msg.Content, core.ContentPart{
				Type: core.PartToolResult,
				ToolResult: &core.ToolResult{
					CallID:  b.ToolUseID,
					Content: decodeAntToolResultContent(b.Content),
					IsError: b.IsError,
				},
			})
		case "image":
			if b.Source != nil {
				if b.Source.Type == "url" && b.Source.URL != "" {
					msg.Content = append(msg.Content, core.ContentPart{
						Type:  core.PartImage,
						Media: &core.MediaPayload{URL: b.Source.URL},
					})
				} else {
					msg.Content = append(msg.Content, core.ContentPart{
						Type:  core.PartImage,
						Media: &core.MediaPayload{MIMEType: b.Source.MediaType, Data: b.Source.Data},
					})
				}
			}
		}
	}
	return msg
}

func decodeAntToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []antBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return string(raw)
	}
	var out string
	for _, b := range blocks {
		if b.Type == "text" {
			out += b.Text
		}
	}
	return out
}

func mapAntRole(role string) core.Role {
	switch role {
	case "assistant":
		return core.RoleAssistant
	default:
		return core.RoleUser
	}
}

// ---- request rendering ------------------------------------------------------

func (AnthropicCodec) RenderRequest(req *core.ChatRequest) ([]byte, error) {
	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}
	out := antRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    req.Stream,
		Temp:      req.Temperature,
		TopP:      req.TopP,
		Stop:      req.Stop,
	}
	// claude-opus-4 deprecated temperature and returns a 400 when it is present.
	if modelRejectsTemperature(req.Model) {
		out.Temp = nil
	}
	if req.System != "" {
		sys, _ := json.Marshal(req.System)
		out.System = sys
	}

	for _, t := range req.Tools {
		out.Tools = append(out.Tools, antTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	// Render tool_choice only when tools are declared; Anthropic rejects a
	// tool_choice with an empty tools array.
	if len(out.Tools) > 0 {
		if tc := openAIToolChoiceToClaude(req.ToolChoice); tc != nil {
			if raw, err := json.Marshal(tc); err == nil {
				out.ToolChoice = raw
			}
		}
	}

	// Anthropic requires alternating user/assistant roles and groups tool
	// results into user messages. We render each canonical message to a block
	// array; consecutive same-role messages are merged.
	for _, m := range req.Messages {
		blocks := renderAntBlocks(m)
		raw, _ := json.Marshal(blocks)
		role := "user"
		if m.Role == core.RoleAssistant {
			role = "assistant"
		}
		out.Messages = appendAntMessage(out.Messages, role, raw, blocks)
	}

	return json.Marshal(out)
}

func renderAntBlocks(m core.Message) []antBlock {
	var blocks []antBlock
	for _, p := range m.Content {
		switch p.Type {
		case core.PartText:
			blocks = append(blocks, antBlock{Type: "text", Text: p.Text})
		case core.PartThinking:
			blocks = append(blocks, antBlock{Type: "thinking", Text: p.Text})
		case core.PartToolCall:
			blocks = append(blocks, antBlock{
				Type:  "tool_use",
				ID:    p.ToolCall.ID,
				Name:  p.ToolCall.Name,
				Input: normalizeAntToolInputRaw(p.ToolCall.Arguments),
			})
		case core.PartToolResult:
			content, _ := json.Marshal(p.ToolResult.Content)
			blocks = append(blocks, antBlock{
				Type:      "tool_result",
				ToolUseID: p.ToolResult.CallID,
				Content:   content,
				IsError:   p.ToolResult.IsError,
			})
		case core.PartImage:
			if p.Media != nil {
				if p.Media.Data != "" {
					blocks = append(blocks, antBlock{
						Type:   "image",
						Source: &antImageSource{Type: "base64", MediaType: p.Media.MIMEType, Data: p.Media.Data},
					})
				} else if p.Media.URL != "" {
					blocks = append(blocks, antBlock{
						Type:   "image",
						Source: &antImageSource{Type: "url", URL: p.Media.URL},
					})
				}
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, antBlock{Type: "text", Text: ""})
	}
	return blocks
}

// appendAntMessage merges blocks into the previous message when roles match, as
// Anthropic forbids consecutive messages with the same role.
func appendAntMessage(msgs []antMessage, role string, raw json.RawMessage, blocks []antBlock) []antMessage {
	if n := len(msgs); n > 0 && msgs[n-1].Role == role {
		var prev []antBlock
		_ = json.Unmarshal(msgs[n-1].Content, &prev)
		prev = append(prev, blocks...)
		merged, _ := json.Marshal(prev)
		msgs[n-1].Content = merged
		return msgs
	}
	return append(msgs, antMessage{Role: role, Content: raw})
}

func normalizeAntToolInputRaw(raw json.RawMessage) json.RawMessage {
	if antToolInputIsObject(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}

func normalizeAntToolInputValue(raw json.RawMessage) any {
	raw = normalizeAntToolInputRaw(raw)
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return map[string]any{}
	}
	return input
}

func antToolInputIsObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	return raw[0] == '{'
}

// ---- response parsing -------------------------------------------------------

type antResponse struct {
	ID         string     `json:"id"`
	Model      string     `json:"model"`
	Content    []antBlock `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (AnthropicCodec) ParseResponse(body []byte, model string) (*core.ChatResponse, error) {
	var raw antResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	msg := core.Message{Role: core.RoleAssistant}
	for _, b := range raw.Content {
		switch b.Type {
		case "text":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: b.Text})
		case "thinking":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: b.Text})
		case "tool_use":
			msg.Content = append(msg.Content, core.ContentPart{
				Type:     core.PartToolCall,
				ToolCall: &core.ToolCall{ID: b.ID, Name: b.Name, Arguments: b.Input},
			})
		}
	}

	return &core.ChatResponse{
		ID:           raw.ID,
		Model:        firstNonEmpty(raw.Model, model),
		Message:      msg,
		FinishReason: mapAntStop(raw.StopReason),
		Usage: core.Usage{
			PromptTokens:     raw.Usage.InputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens:      raw.Usage.InputTokens + raw.Usage.OutputTokens,
			CachedTokens:     raw.Usage.CacheReadInputTokens,
			CacheWriteTokens: raw.Usage.CacheCreationInputTokens,
		},
	}, nil
}

func (AnthropicCodec) RenderResponse(resp *core.ChatResponse) ([]byte, error) {
	var content []map[string]any
	for _, p := range resp.Message.Content {
		switch p.Type {
		case core.PartText:
			content = append(content, map[string]any{"type": "text", "text": p.Text})
		case core.PartThinking:
			content = append(content, map[string]any{"type": "thinking", "thinking": p.Text})
		case core.PartToolCall:
			content = append(content, map[string]any{
				"type": "tool_use", "id": p.ToolCall.ID, "name": p.ToolCall.Name, "input": normalizeAntToolInputValue(p.ToolCall.Arguments),
			})
		}
	}
	out := map[string]any{
		"id":          firstNonEmpty(resp.ID, "msg_"+resp.Model),
		"type":        "message",
		"role":        "assistant",
		"model":       resp.Model,
		"content":     content,
		"stop_reason": renderAntStop(resp.FinishReason),
		"usage": map[string]int{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
		},
	}
	return json.Marshal(out)
}

func mapAntStop(r string) core.FinishReason {
	switch r {
	case "end_turn", "stop_sequence":
		return core.FinishStop
	case "max_tokens":
		return core.FinishLength
	case "tool_use":
		return core.FinishToolCalls
	default:
		return core.FinishStop
	}
}

func renderAntStop(r core.FinishReason) string {
	switch r {
	case core.FinishLength:
		return "max_tokens"
	case core.FinishToolCalls:
		return "tool_use"
	default:
		return "end_turn"
	}
}
