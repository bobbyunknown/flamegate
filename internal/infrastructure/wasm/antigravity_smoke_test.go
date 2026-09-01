package wasm

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func TestAntigravityExtensionSmoke(t *testing.T) {
	wasmBytes, err := os.ReadFile("../../../flamegate-ext/antigravity/dist/antigravity.wasm")
	if err != nil {
		t.Skip("skipping wasm test: antigravity.wasm not found")
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var chunks [][]byte
	var lastURL string
	var lastBody string
	var lastHdrs string

	hostBuilder := r.NewHostModuleBuilder("env")
	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, urlPtr, urlLen, bodyPtr, bodyLen, hdrsPtr, hdrsLen uint32,
	) uint32 {
		u, _ := mod.Memory().Read(urlPtr, urlLen)
		lastURL = string(u)
		b, _ := mod.Memory().Read(bodyPtr, bodyLen)
		lastBody = string(b)
		h, _ := mod.Memory().Read(hdrsPtr, hdrsLen)
		lastHdrs = string(h)
		_ = lastHdrs

		// Mock token exchange / refresh
		if lastURL == "https://oauth2.googleapis.com/token" {
			return writeStubJSON(mod, `{"access_token":"ya29.test_token","refresh_token":"1//test_refresh","expires_in":3599}`)
		}

		// Mock loadCodeAssist
		if lastURL == "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist" {
			return writeStubJSON(mod, `{"cloudaicompanionProject":"test-project-123","currentTier":{"id":"GOOGLE_ONE_AI"}}`)
		}

		// Mock fetchAvailableModels
		if lastURL == "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels" {
			return writeStubJSON(mod, `{"models":{"gemini-2.5-pro":{"displayName":"Gemini 2.5 Pro"}}}`)
		}

		// Mock non-streaming generation
		if lastURL == "https://daily-cloudcode-pa.googleapis.com/v1internal:generateContent" {
			return writeStubJSON(mod, `{"candidates":[{"content":{"parts":[{"text":"Hello from Antigravity!"}]}}]}`)
		}

		// Mock streaming generation
		if lastURL == "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse" {
			sse := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello \"}]}}]}\n\n" +
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"from Antigravity stream!\"}]}}]}\n\n" +
				"data: [DONE]\n\n"
			return writeStubJSON(mod, sse)
		}

		return 0
	}).Export("http_post")

	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, urlPtr, urlLen, hdrsPtr, hdrsLen uint32,
	) uint32 {
		u, _ := mod.Memory().Read(urlPtr, urlLen)
		if string(u) == "https://www.googleapis.com/oauth2/v2/userinfo" {
			return writeStubJSON(mod, `{"email":"antigravity.tester@gmail.com","name":"Antigravity Tester"}`)
		}
		return 0
	}).Export("http_get")

	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, keyPtr, keyLen uint32,
	) uint32 {
		return writeStubJSON(mod, `{"access_token":"ya29.test_token","project_id":"test-project-123"}`)
	}).Export("get_credentials")

	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, chunkPtr, chunkLen uint32,
	) {
		b, ok := mod.Memory().Read(uint32(chunkPtr), uint32(chunkLen))
		require.True(t, ok)
		chunks = append(chunks, append([]byte(nil), b...))
	}).Export("emit_chunk")

	hostMod, err := hostBuilder.Instantiate(ctx)
	require.NoError(t, err)
	defer hostMod.Close(ctx)

	compiled, err := r.CompileModule(ctx, wasmBytes)
	require.NoError(t, err)
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer mod.Close(ctx)

	// 1. Test list_models
	lmFn := mod.ExportedFunction("list_models")
	lmRes, err := lmFn.Call(ctx)
	require.NoError(t, err)
	lmRaw := readStubJSON(t, mod, uint32(lmRes[0]))
	var models []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(lmRaw, &models))
	require.True(t, len(models) >= 4)
	require.NotEmpty(t, models[0].ID)
	t.Logf("list_models returned %d models", len(models))

	// 2. Test oauth_authorize
	authReq := map[string]interface{}{
		"capability":   "oauth_authorize",
		"state":        "test-state-123",
		"redirect_uri": "http://localhost:20180/api/oauth/antigravity/callback",
	}
	authReqJSON, _ := json.Marshal(authReq)
	authRespPtr := callInvoke(t, ctx, mod, authReqJSON)
	authResp := readStubJSON(t, mod, authRespPtr)
	var authData struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(authResp, &authData))
	require.Contains(t, authData.URL, "https://accounts.google.com/o/oauth2/v2/auth")
	require.Equal(t, "test-state-123", authData.State)
	t.Logf("oauth_authorize URL: %s", authData.URL)

	// 3. Test oauth_exchange
	exchReq := map[string]interface{}{
		"capability":   "oauth_exchange",
		"code":         "test-auth-code",
		"redirect_uri": "http://localhost:20180/api/oauth/antigravity/callback",
	}
	exchReqJSON, _ := json.Marshal(exchReq)
	exchRespPtr := callInvoke(t, ctx, mod, exchReqJSON)
	exchResp := readStubJSON(t, mod, exchRespPtr)
	var exchData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountName  string `json:"account_name"`
		ProjectID    string `json:"project_id"`
	}
	require.NoError(t, json.Unmarshal(exchResp, &exchData))
	require.Equal(t, "ya29.test_token", exchData.AccessToken)
	require.Equal(t, "1//test_refresh", exchData.RefreshToken)
	require.Equal(t, "antigravity.tester@gmail.com", exchData.AccountName)
	require.Equal(t, "aicode-consumers", exchData.ProjectID)
	t.Logf("oauth_exchange result: %+v", exchData)

	// 4. Test oauth_refresh
	refReq := map[string]interface{}{
		"capability":    "oauth_refresh",
		"refresh_token": "1//test_refresh",
	}
	refReqJSON, _ := json.Marshal(refReq)
	refRespPtr := callInvoke(t, ctx, mod, refReqJSON)
	refResp := readStubJSON(t, mod, refRespPtr)
	var refData struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(refResp, &refData))
	require.Equal(t, "ya29.test_token", refData.AccessToken)

	// 5. Test invoke non-streaming
	chatReq := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":[{"type":"text","text":"Hello!"}]}],"stream":false}`
	chatRespPtr := callInvoke(t, ctx, mod, []byte(chatReq))
	chatResp := readStubJSON(t, mod, chatRespPtr)
	var gResp struct {
		Content      string `json:"content"`
		FinishReason string `json:"finish_reason"`
		Error        string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(chatResp, &gResp))
	require.Empty(t, gResp.Error)
	require.Equal(t, "Hello from Antigravity!", gResp.Content)
	require.Equal(t, "stop", gResp.FinishReason)
	require.Contains(t, lastBody, `"project":"aicode-consumers"`)
	require.Contains(t, lastBody, `"model":"gemini-2.5-flash"`)
	t.Logf("Non-streaming response: %+v", gResp)

	// 6. Test invoke streaming
	chunks = nil
	streamReq := `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":[{"type":"text","text":"Hello stream!"}]}],"stream":true}`
	_ = callInvoke(t, ctx, mod, []byte(streamReq))
	require.NotEmpty(t, chunks)
	t.Logf("Streaming emitted %d chunks", len(chunks))
	var combinedText string
	for _, chunkBytes := range chunks {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(chunkBytes, &chunk); err == nil && len(chunk.Choices) > 0 {
			combinedText += chunk.Choices[0].Delta.Content
		}
	}
	require.Equal(t, "Hello from Antigravity stream!", combinedText)
	t.Logf("Combined stream text: %s", combinedText)
}
