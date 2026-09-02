package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

func newAdminTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	return db
}

func TestProviderAccountMetadataSpecialProviders(t *testing.T) {
	custom, ok := connectors.SpecByID("custom-openai")
	require.True(t, ok)
	_, err := providerAccountMetadata(custom, providerMetadataInput{})
	require.Error(t, err)
	meta, err := providerAccountMetadata(custom, providerMetadataInput{BaseURL: "https://llm.example.com/v1"})
	require.NoError(t, err)
	require.Equal(t, "https://llm.example.com/v1", meta["base_url"])
}

func TestParseRefreshFlag(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on", " True "} {
		if !parseRefreshFlag(v) {
			t.Fatalf("parseRefreshFlag(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "refresh"} {
		if parseRefreshFlag(v) {
			t.Fatalf("parseRefreshFlag(%q) = true, want false", v)
		}
	}
}

func TestHumaProviderModels_EmptySliceNotNull(t *testing.T) {
	// Test that an unknown/empty provider models call returns empty slice not nil
	h := &Handler{}
	connectors.RegisterExtensionSpec(connectors.ProviderSpec{
		ID:          "test-ext",
		DisplayName: "Test Ext",
	})
	defer connectors.UnregisterExtensionSpec("test-ext")

	out, err := h.HumaProviderModels(context.Background(), &ProviderModelsInput{
		ID: "test-ext",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Body.Models)
	require.Equal(t, 0, len(out.Body.Models))
}

func TestSyncExtensionModels_Success(t *testing.T) {
	db := newAdminTestDB(t)
	ctx := context.Background()

	// Register extension in DB and spec catalog
	connectors.RegisterExtensionSpec(connectors.ProviderSpec{
		ID:          "cline",
		DisplayName: "Cline",
	})
	defer connectors.UnregisterExtensionSpec("cline")

	ext := schema.Extension{
		ID:           "cline",
		TenantID:     "default",
		Slug:         "cline",
		Name:         "Cline",
		Version:      "0.1.0",
		State:        "ACTIVE",
		Capabilities: `["chat","models"]`,
		Entrypoints:  `{"chat":"invoke","models":"list_models"}`,
	}
	require.NoError(t, db.Extensions().Create(ctx, ext))

	// Setup WASM Engine
	wasmBytes, err := os.ReadFile("../../../../../flamegate-ext/cline/dist/cline.wasm")
	if err != nil {
		t.Skip("skipping wasm test: cline.wasm not found")
	}

	wasmEngine := wasm.NewEngine(config.WASMConfig{
		MaxMemoryMB:    16,
		MaxInst:        4,
		DefaultTimeout: 10 * time.Second,
	}, nil, db.Accounts(), nil)
	defer wasmEngine.Close()

	require.NoError(t, wasmEngine.Compile(ctx, "cline", wasmBytes, wasm.ExtensionConfig{
		Slug:        "cline",
		Timeout:     10 * time.Second,
		Entrypoints: map[string]string{"chat": "invoke", "models": "list_models"},
	}))

	h := &Handler{
		db:         db,
		wasmEngine: wasmEngine,
		log:        logrus.New(),
	}

	n, err := h.syncExtensionModels(ctx, ext)
	require.NoError(t, err)
	require.Greater(t, n, 0)
	t.Logf("Successfully synced %d models into database", n)

	// Verify they are in extension_models table
	ems, err := db.ExtensionModels().ListByExtension(ctx, "cline")
	require.NoError(t, err)
	require.Equal(t, n, len(ems))
	t.Logf("First model in DB: %+v", ems[0])

	// Verify HumaProviderModels returns them
	out, err := h.HumaProviderModels(ctx, &ProviderModelsInput{
		ID: "cline",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, n, len(out.Body.Models))
}

type fakeTestConnector struct{}

func (f *fakeTestConnector) ID() string { return "cline" }

func (f *fakeTestConnector) Dialect() core.Dialect { return core.DialectOpenAI }

func (f *fakeTestConnector) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	return &core.ChatResponse{
		ID:    "test-resp",
		Model: req.Model,
		Message: core.Message{
			Role: "assistant",
			Content: []core.ContentPart{
				{Type: "text", Text: "Hello from test!"},
			},
		},
		Usage: core.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
	}, nil
}

func (f *fakeTestConnector) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 2)
	ch <- core.StreamChunk{Type: core.ChunkText, Delta: "Hello "}
	ch <- core.StreamChunk{Type: core.ChunkText, Delta: "from stream!"}
	close(ch)
	return ch, nil
}

type fakeConnectorSource struct {
	conn core.Connector
}

func (s *fakeConnectorSource) Get(id string) (core.Connector, error) {
	return s.conn, nil
}

func TestHumaTestModel_And_TestChat(t *testing.T) {
	db := newAdminTestDB(t)
	ctx := context.Background()

	// Create an account for cline
	acc := schema.Account{
		ID:       "acc-cline-1",
		TenantID: schema.DefaultTenantID,
		Provider: "cline",
		Label:    "Primary Cline",
	}
	require.NoError(t, db.Accounts().Create(ctx, acc))

	disp := dispatch.New(&fakeConnectorSource{conn: &fakeTestConnector{}}, db.Accounts(), nil)
	pipe := pipeline.New(pipeline.Deps{
		Dispatcher: disp,
		Logger:     logrus.New(),
	})

	h := &Handler{
		db:       db,
		pipeline: pipe,
		log:      logrus.New(),
	}

	// 1. Test Quick Probe (HumaTestModel)
	testInput := &TestModelInput{}
	testInput.Body.Provider = "cline"
	testInput.Body.Model = "z-ai/glm-5.3-flash"
	testOut, err := h.HumaTestModel(ctx, testInput)
	require.NoError(t, err)
	require.NotNil(t, testOut)
	require.Equal(t, "ok", testOut.Body.Status)
	require.Equal(t, "Hello from test!", testOut.Body.ResponseText)

	// 2. Test Playground Unary Chat (HumaTestChat)
	chatInput := &TestChatInput{}
	chatInput.Body.Provider = "cline"
	chatInput.Body.Model = "cline/z-ai/glm-5.3-flash"
	chatInput.Body.Messages = []TestChatMessageInput{
		{Role: "user", Content: "Halo"},
	}
	chatOut, err := h.HumaTestChat(ctx, chatInput)
	require.NoError(t, err)
	require.NotNil(t, chatOut)
	require.Equal(t, "ok", chatOut.Body.Status)
	require.Equal(t, "Hello from test!", chatOut.Body.ResponseText)
	require.Equal(t, 5, chatOut.Body.PromptTokens)
	require.Equal(t, 10, chatOut.Body.CompletionTokens)
}

func TestAdminModelChatStream(t *testing.T) {
	db := newAdminTestDB(t)
	ctx := context.Background()

	acc := schema.Account{
		ID:       "acc-cline-1",
		TenantID: schema.DefaultTenantID,
		Provider: "cline",
		Label:    "Primary Cline",
	}
	require.NoError(t, db.Accounts().Create(ctx, acc))

	disp := dispatch.New(&fakeConnectorSource{conn: &fakeTestConnector{}}, db.Accounts(), nil)
	pipe := pipeline.New(pipeline.Deps{
		Dispatcher: disp,
		Logger:     logrus.New(),
	})

	h := &Handler{
		db:       db,
		pipeline: pipe,
		log:      logrus.New(),
	}

	reqBody := `{"provider":"cline","model":"cline/z-ai/glm-5.3-flash","messages":[{"role":"user","content":"Halo"}]}`
	req, err := http.NewRequestWithContext(ctx, "POST", "/api/models/chat/stream", strings.NewReader(reqBody))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.adminModelChatStream(w, req)

	res := w.Result()
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	bodyStr := string(bodyBytes)

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, bodyStr, `event: delta`)
	require.Contains(t, bodyStr, `Hello`)
	require.Contains(t, bodyStr, `event: done`)
}
