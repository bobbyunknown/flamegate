package wasm

// Temporary smoke test for the cline extension ABI — deleted after verification.
// Loads flamegate-ext/cline/dist/cline.wasm, stubs the env host functions,
// calls invoke (non-stream) + list_models, asserts canonical responses.

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func TestClineExtensionSmoke(t *testing.T) {
	wasm, err := os.ReadFile("../../../flamegate-ext/cline/dist/cline.wasm")
	require.NoError(t, err)

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	// Stub host module "env" with the 3 imports the extension needs.
	var chunks [][]byte
	var lastBody string
	var lastHdrs string
	hostBuilder := r.NewHostModuleBuilder("env")
	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, urlPtr, urlLen, bodyPtr, bodyLen, hdrsPtr, hdrsLen uint32,
	) uint32 {
		b, _ := mod.Memory().Read(bodyPtr, bodyLen)
		lastBody = string(b)
		h, _ := mod.Memory().Read(hdrsPtr, hdrsLen)
		lastHdrs = string(h)
		// Return a canned OpenAI SSE stream (Cline is streaming-only).
		sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"},\"index\":0}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"index\":0}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"index\":0,\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n\n"
		return writeStubJSON(mod, sse)
	}).Export("http_post")
	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, urlPtr, urlLen, hdrsPtr, hdrsLen uint32,
	) uint32 {
		// Mock http_get returning 0 to exercise the static fallback catalog
		return 0
	}).Export("http_get")
	hostBuilder.NewFunctionBuilder().WithFunc(func(
		ctx context.Context, mod api.Module, keyPtr, keyLen uint32,
	) uint32 {
		// api_key carrying a workos:-prefixed OAuth token.
		return writeStubJSON(mod, `{"api_key":"workos:test-token","base_url":""}`)
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

	compiled, err := r.CompileModule(ctx, wasm)
	require.NoError(t, err)
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)
	defer mod.Close(ctx)

	// --- invoke with FlameGate domain ContentPart array format ---
	reqContentParts := `{"model":"z-ai/glm-5.3-flash","messages":[{"role":"user","content":[{"type":"text","text":"Hello! Please reply with exactly the word PONG."}]}],"stream":false}`
	respPtr := callInvoke(t, ctx, mod, []byte(reqContentParts))
	resp := readStubJSON(t, mod, respPtr)
	var gResp struct {
		Content      string `json:"content"`
		FinishReason string `json:"finish_reason"`
		Error        string `json:"error"`
		Code         string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(resp, &gResp))
	require.Empty(t, gResp.Error, "unexpected guest error: %s", string(resp))
	require.Equal(t, "Hello", gResp.Content)
	require.Equal(t, "stop", gResp.FinishReason)
	t.Logf("Upstream Body Sent: %s", lastBody)
	t.Logf("Upstream Headers Sent: %s", lastHdrs)
	require.Contains(t, lastBody, `"content":"Hello! Please reply with exactly the word PONG."`)

	// --- list_models ---
	lmFn := mod.ExportedFunction("list_models")
	lmRes, err := lmFn.Call(ctx)
	require.NoError(t, err)
	lmPtr := uint32(lmRes[0])
	lmRaw := readStubJSON(t, mod, lmPtr)
	var models []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(lmRaw, &models))
	require.True(t, len(models) > 5, "expected model catalog, got %d", len(models))
	require.Equal(t, "moonshotai/kimi-k3", models[0].ID)

	// --- oauth_authorize ---
	authReq := map[string]interface{}{"capability": "oauth_authorize", "state": "test-state", "redirect_uri": "http://localhost:20180/api/oauth/cline/callback"}
	authReqJSON, _ := json.Marshal(authReq)
	authRespPtr := callInvoke(t, ctx, mod, authReqJSON)
	authResp := readStubJSON(t, mod, authRespPtr)
	var authData struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(authResp, &authData))
	require.NotEmpty(t, authData.URL)
	require.Equal(t, "test-state", authData.State)
	t.Logf("oauth_authorize URL: %s", authData.URL)

	// --- oauth_exchange with embedded base64 token ---
	embeddedJSON := `{"accessToken":"workos:test_access_token","refreshToken":"test_refresh_token","email":"sugengtd7@gmail.com","expiresAt":"2026-09-01T00:00:00Z"}`
	encodedCode := "code=" + base64.StdEncoding.EncodeToString([]byte(embeddedJSON))
	exchReq := map[string]interface{}{"capability": "oauth_exchange", "code": encodedCode, "redirect_uri": "http://localhost:20180/api/oauth/cline/callback"}
	exchReqJSON, _ := json.Marshal(exchReq)
	exchRespPtr := callInvoke(t, ctx, mod, exchReqJSON)
	exchResp := readStubJSON(t, mod, exchRespPtr)
	var tokData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Email        string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(exchResp, &tokData))
	require.Equal(t, "workos:test_access_token", tokData.AccessToken)
	require.Equal(t, "test_refresh_token", tokData.RefreshToken)
	require.Equal(t, "sugengtd7@gmail.com", tokData.Email)
	t.Logf("oauth_exchange decoded token successfully: %+v", tokData)
}

// writeStubJSON writes [4-byte LE len][data] via guest alloc and returns ptr.
func writeStubJSON(mod api.Module, s string) uint32 {
	allocFn := mod.ExportedFunction("alloc")
	data := []byte(s)
	total := len(data) + 4
	res, err := allocFn.Call(context.Background(), uint64(total))
	if err != nil || res[0] == 0 {
		return 0
	}
	ptr := uint32(res[0])
	mem := mod.Memory()
	var lb [4]byte
	binary.LittleEndian.PutUint32(lb[:], uint32(len(data)))
	if !mem.Write(ptr, lb[:]) || !mem.Write(ptr+4, data) {
		return 0
	}
	return ptr
}

// readStubJSON reads [4-byte LE len][data] at ptr.
func readStubJSON(t *testing.T, mod api.Module, ptr uint32) []byte {
	t.Helper()
	lb, ok := mod.Memory().Read(ptr, 4)
	require.True(t, ok)
	n := binary.LittleEndian.Uint32(lb)
	b, ok := mod.Memory().Read(ptr+4, n)
	require.True(t, ok)
	return append([]byte(nil), b...)
}

// callInvoke writes [len][json], calls invoke(ptr, len), returns resp ptr.
func callInvoke(t *testing.T, ctx context.Context, mod api.Module, reqJSON []byte) uint32 {
	t.Helper()
	allocFn := mod.ExportedFunction("alloc")
	total := len(reqJSON) + 4
	res, err := allocFn.Call(ctx, uint64(total))
	require.NoError(t, err)
	ptr := uint32(res[0])
	require.NotZero(t, ptr)
	mem := mod.Memory()
	var lb [4]byte
	binary.LittleEndian.PutUint32(lb[:], uint32(len(reqJSON)))
	require.True(t, mem.Write(ptr, lb[:]))
	require.True(t, mem.Write(ptr+4, reqJSON))
	invokeFn := mod.ExportedFunction("invoke")
	invokeRes, err := invokeFn.Call(ctx, uint64(ptr), uint64(len(reqJSON)))
	require.NoError(t, err)
	return uint32(invokeRes[0])
}
