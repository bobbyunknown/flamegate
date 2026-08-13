package connectors

import (
	"context"
	"encoding/json"
	"strconv"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// This file implements the non-chat capability interfaces for the
// OpenAI-compatible connector: image generation (/images/generations),
// speech-to-text (/audio/transcriptions), and text-to-speech (/audio/speech).
// These follow the OpenAI wire format, so they serve OpenAI itself plus any
// provider that mirrors those endpoints (Groq STT, etc.).

// ---- Image generation -------------------------------------------------------

type oaiImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type oaiImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
}

// GenerateImage produces images via the OpenAI images endpoint.
func (c *OpenAICompatible) GenerateImage(ctx context.Context, req *core.ImageRequest, creds core.Credentials) (*core.ImageResponse, error) {
	n := req.N
	if n <= 0 {
		n = 1
	}
	body, err := json.Marshal(oaiImageRequest{
		Model: req.Model, Prompt: req.Prompt, N: n, Size: req.Size,
		Quality: req.Quality, Style: req.Style, ResponseFormat: req.ResponseFormat,
	})
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := joinURL(c.baseURL(creds), "images/generations")
	respBody, err := doJSON(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	var raw oaiImageResponse
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: "parse image: " + err.Error(), Cause: err}
	}
	out := &core.ImageResponse{Model: req.Model, Created: raw.Created}
	for _, d := range raw.Data {
		out.Data = append(out.Data, core.ImageData{URL: d.URL, B64JSON: d.B64JSON, RevisedPrompt: d.RevisedPrompt})
	}
	return out, nil
}

// ---- Speech-to-text ---------------------------------------------------------

type oaiTranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
}

// Transcribe converts audio to text via the OpenAI transcriptions endpoint.
func (c *OpenAICompatible) Transcribe(ctx context.Context, req *core.TranscriptionRequest, creds core.Credentials) (*core.TranscriptionResponse, error) {
	filename := req.Filename
	if filename == "" {
		filename = "audio.mp3"
	}
	fields := []multipartField{
		{Name: "model", Value: req.Model},
		{Name: "language", Value: req.Language},
		{Name: "prompt", Value: req.Prompt},
		{Name: "response_format", Value: req.ResponseFormat},
	}
	if req.Temperature != nil {
		fields = append(fields, multipartField{Name: "temperature", Value: strconv.FormatFloat(*req.Temperature, 'f', -1, 64)})
	}

	url := joinURL(c.baseURL(creds), "audio/transcriptions")
	respBody, err := doMultipart(ctx, c.id, req.Model, url, "file", filename, req.Audio, fields, c.headers(creds))
	if err != nil {
		return nil, err
	}

	// response_format=text returns a bare string, not JSON.
	if req.ResponseFormat == "text" {
		return &core.TranscriptionResponse{Text: string(respBody)}, nil
	}
	var raw oaiTranscriptionResponse
	if err := json.Unmarshal(respBody, &raw); err != nil {
		// Fall back to treating the body as plain text.
		return &core.TranscriptionResponse{Text: string(respBody)}, nil
	}
	return &core.TranscriptionResponse{Text: raw.Text, Language: raw.Language, Duration: raw.Duration}, nil
}

// ---- Text-to-speech ---------------------------------------------------------

type oaiSpeechRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Speed          *float64 `json:"speed,omitempty"`
}

// Synthesize converts text to speech via the OpenAI speech endpoint, returning
// raw audio bytes.
func (c *OpenAICompatible) Synthesize(ctx context.Context, req *core.SpeechRequest, creds core.Credentials) (*core.SpeechResponse, error) {
	voice := req.Voice
	if voice == "" {
		voice = "alloy"
	}
	body, err := json.Marshal(oaiSpeechRequest{
		Model: req.Model, Input: req.Input, Voice: voice,
		ResponseFormat: req.ResponseFormat, Speed: req.Speed,
	})
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := joinURL(c.baseURL(creds), "audio/speech")
	raw, err := doRaw(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}
	ct := raw.ContentType
	if ct == "" {
		ct = "audio/mpeg"
	}
	return &core.SpeechResponse{Audio: raw.Body, ContentType: ct}, nil
}
