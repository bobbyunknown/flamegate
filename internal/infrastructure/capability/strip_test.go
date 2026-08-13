package capability

import (
	"strings"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestStripUnsupportedModalitiesNil(t *testing.T) {
	if StripUnsupportedModalities(nil, "custom-openai-test", "glm-5.2") {
		t.Error("should return false for nil request")
	}
}

func TestStripUnsupportedModalitiesNoMedia(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "hello"},
			}},
		},
	}
	if StripUnsupportedModalities(req, "custom-openai-test", "glm-5.2") {
		t.Error("should return false for text-only request")
	}
	if req.Messages[0].Content[0].Text != "hello" {
		t.Error("text should be unchanged")
	}
}

func TestStripUnsupportedModalitiesStripsImage(t *testing.T) {
	// glm-5.2 resolves via *glm-5* pattern: no vision capability.
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "describe this"},
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "base64data"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "custom-openai-test", "glm-5.2")
	if !stripped {
		t.Error("should return true when image is stripped")
	}
	if req.Messages[0].Content[1].Type != core.PartText {
		t.Errorf("image part should be replaced with text, got %v", req.Messages[0].Content[1].Type)
	}
	if !strings.Contains(req.Messages[0].Content[1].Text, "image") {
		t.Errorf("placeholder should mention 'image', got %q", req.Messages[0].Content[1].Text)
	}
	// Text part should be unchanged.
	if req.Messages[0].Content[0].Text != "describe this" {
		t.Error("text part should be unchanged")
	}
}

func TestStripUnsupportedModalitiesKeepsImageWhenVisionCap(t *testing.T) {
	// gpt-4o resolves via *gpt-4o* pattern: has vision capability.
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "base64data"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "openai", "gpt-4o")
	if stripped {
		t.Error("should not strip image when model has vision capability")
	}
	if req.Messages[0].Content[0].Type != core.PartImage {
		t.Error("image part should remain unchanged")
	}
}

func TestStripUnsupportedModalitiesStripsAudio(t *testing.T) {
	// Most models lack audio input; use an unknown model that falls to floor.
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartAudio, Media: &core.MediaPayload{MIMEType: "audio/wav", Data: "base64audio"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "custom-openai-test", "mymodel")
	if !stripped {
		t.Error("should strip audio when model lacks audio input capability")
	}
	if req.Messages[0].Content[0].Type != core.PartText {
		t.Errorf("audio part should be replaced with text, got %v", req.Messages[0].Content[0].Type)
	}
	if !strings.Contains(req.Messages[0].Content[0].Text, "audio") {
		t.Errorf("placeholder should mention 'audio', got %q", req.Messages[0].Content[0].Text)
	}
}

func TestStripUnsupportedModalitiesPreservesToolCalls(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "my_tool"}},
			}},
			{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tc1", Content: "result"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "custom-openai-test", "glm-5.2")
	if stripped {
		t.Error("should not strip tool calls or tool results")
	}
	if req.Messages[0].Content[0].Type != core.PartToolCall {
		t.Error("tool call should be preserved")
	}
	if req.Messages[1].Content[0].Type != core.PartToolResult {
		t.Error("tool result should be preserved")
	}
}

func TestStripUnsupportedModalitiesCurrentVsPreviousPlaceholder(t *testing.T) {
	// First message (history) image → neutral "previous" placeholder.
	// Last message (current turn) image → explanatory placeholder.
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "old"}},
			}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "ok"},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "new"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "custom-openai-test", "glm-5.2")
	if !stripped {
		t.Fatal("should strip both images")
	}
	if req.Messages[0].Content[0].Text != placeholderImagePrev {
		t.Errorf("history image should use previous placeholder, got %q", req.Messages[0].Content[0].Text)
	}
	if req.Messages[2].Content[0].Text != placeholderImageCurrent {
		t.Errorf("current image should use current placeholder, got %q", req.Messages[2].Content[0].Text)
	}
}

func TestStripUnsupportedModalitiesMultipleMessages(t *testing.T) {

	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "img1"}},
			}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "I see an image"},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "another question"},
				{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/jpeg", Data: "img2"}},
			}},
		},
	}
	stripped := StripUnsupportedModalities(req, "custom-openai-test", "glm-5.2")
	if !stripped {
		t.Error("should strip images across multiple messages")
	}
	// First message: image replaced.
	if req.Messages[0].Content[0].Type != core.PartText {
		t.Error("first image should be replaced")
	}
	// Third message: text preserved, image replaced.
	if req.Messages[2].Content[0].Text != "another question" {
		t.Error("text in third message should be unchanged")
	}
	if req.Messages[2].Content[1].Type != core.PartText {
		t.Error("second image should be replaced")
	}
}
