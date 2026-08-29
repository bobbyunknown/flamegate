package wasm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/bobbyunknown/flamegate/internal/config"
	core "github.com/bobbyunknown/flamegate/internal/domain/provider"
)

func testEngineConfig() config.WASMConfig {
	return config.WASMConfig{
		MaxMemoryMB:       16,
		MaxInst:           16,
		DefaultTimeout:    60 * time.Second,
		HotReloadInterval: 10 * time.Second,
	}
}

// newEngineForTest creates an Engine with nil dependencies for unit tests.
func newEngineForTest(t *testing.T) *Engine {
	t.Helper()
	return NewEngine(testEngineConfig(), nil, nil, nil)
}

// compileMinimalWASM compiles a minimal valid WASM module that exports:
// alloc(i32)->i32, dealloc(i32,i32)->void, invoke(i32,i32)->i32, memory.
func compileMinimalWASM(t *testing.T) []byte {
	t.Helper()
	return validMinimalWASM(t)
}

// validMinimalWASM returns a verified-valid minimal WASM binary.
func validMinimalWASM(t *testing.T) []byte {
	t.Helper()

	typeSection := encodeSection(1, []byte{
		0x03,
		0x60, 0x01, 0x7f, 0x01, 0x7f,
		0x60, 0x02, 0x7f, 0x7f, 0x00,
		0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
	})

	funcSection := encodeSection(3, []byte{0x03, 0x00, 0x01, 0x02})
	memSection := encodeSection(5, []byte{0x01, 0x00, 0x01})

	exportSection := encodeSection(7, concatBytes(
		[]byte{0x04},
		encodeExport("alloc", 0x00, 0),
		encodeExport("dealloc", 0x00, 1),
		encodeExport("invoke", 0x00, 2),
		encodeExport("memory", 0x02, 0),
	))

	allocBody := encodeFuncBody(nil, []byte{0x41, 0x80, 0x08, 0x0b})
	deallocBody := encodeFuncBody(nil, []byte{0x0b})
	invokeBody := encodeFuncBody(nil, []byte{0x41, 0x00, 0x0b})
	codeSection := encodeSection(10, concatBytes(
		[]byte{0x03},
		allocBody, deallocBody, invokeBody,
	))

	binary := concatBytes(
		[]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
		typeSection, funcSection, memSection, exportSection, codeSection,
	)

	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	wasi_snapshot_preview1.MustInstantiate(context.Background(), r)
	_, err := r.CompileModule(context.Background(), binary)
	require.NoError(t, err, "generated WASM must compile")

	return binary
}

func encodeSection(id byte, content []byte) []byte {
	return concatBytes([]byte{id}, encodeUint32(uint32(len(content))), content)
}

func encodeExport(name string, kind byte, index uint32) []byte {
	nameBytes := []byte(name)
	return concatBytes(
		encodeUint32(uint32(len(nameBytes))),
		nameBytes,
		[]byte{kind},
		encodeUint32(index),
	)
}

func encodeFuncBody(locals []byte, code []byte) []byte {
	return concatBytes(
		encodeUint32(uint32(1+len(code))),
		[]byte{0x00},
		code,
	)
}

func encodeUint32(v uint32) []byte {
	var buf []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func concatBytes(slices ...[]byte) []byte {
	var total int
	for _, s := range slices {
		total += len(s)
	}
	out := make([]byte, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

func TestEngine_CompileValid(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	err := e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug:    "test-ext",
		Timeout: 60 * time.Second,
	})
	require.NoError(t, err)

	mod, ok := e.Get("test-ext")
	assert.True(t, ok)
	assert.NotNil(t, mod)
}

func TestEngine_CompileInvalid(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	err := e.Compile(context.Background(), "bad-ext", []byte{0x00, 0x01, 0x02}, ExtensionConfig{
		Slug: "bad-ext",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compile module bad-ext")
}

func TestEngine_GetMissing(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	mod, ok := e.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, mod)
}

func TestEngine_Unload(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	require.NoError(t, e.Unload("test-ext"))

	mod, ok := e.Get("test-ext")
	assert.False(t, ok)
	assert.Nil(t, mod)
}

func TestEngine_UnloadMissing(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	err := e.Unload("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEngine_HasExtensions(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	assert.False(t, e.HasExtensions())

	require.NoError(t, e.Compile(context.Background(), "ext1", compileMinimalWASM(t), ExtensionConfig{
		Slug: "ext1",
	}))
	assert.True(t, e.HasExtensions())
}

func TestEngine_Close(t *testing.T) {
	e := NewEngine(testEngineConfig(), nil, nil, nil)

	require.NoError(t, e.Compile(context.Background(), "ext1", compileMinimalWASM(t), ExtensionConfig{
		Slug: "ext1",
	}))

	require.NoError(t, e.Close())

	mod, ok := e.Get("ext1")
	assert.False(t, ok)
	assert.Nil(t, mod)
}

func TestEngine_GetConnector(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	conn, ok := e.GetConnector("test-ext")
	assert.True(t, ok)
	assert.NotNil(t, conn)
	assert.Equal(t, "test-ext", conn.ID())
}

func TestEngine_GetConnectorMissing(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	conn, ok := e.GetConnector("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, conn)
}

func TestEngine_Slugs(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	assert.Empty(t, e.Slugs())

	require.NoError(t, e.Compile(context.Background(), "ext1", compileMinimalWASM(t), ExtensionConfig{Slug: "ext1"}))
	require.NoError(t, e.Compile(context.Background(), "ext2", compileMinimalWASM(t), ExtensionConfig{Slug: "ext2"}))

	slugs := e.Slugs()
	assert.Len(t, slugs, 2)
	assert.ElementsMatch(t, []string{"ext1", "ext2"}, slugs)
}

func TestEngine_GetConfig(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	cfg := ExtensionConfig{Slug: "test-ext", Timeout: 30 * time.Second}
	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), cfg))

	got, ok := e.GetConfig("test-ext")
	assert.True(t, ok)
	assert.Equal(t, "test-ext", got.Slug)
	assert.Equal(t, 30*time.Second, got.Timeout)
}

func TestEngine_ListModels_Cline(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	wasmBytes, err := os.ReadFile("../../../flamegate-ext/cline/dist/cline.wasm")
	require.NoError(t, err)

	cfg := ExtensionConfig{
		Slug:        "cline",
		Timeout:     30 * time.Second,
		Entrypoints: map[string]string{"chat": "invoke", "models": "list_models"},
	}
	require.NoError(t, e.Compile(context.Background(), "cline", wasmBytes, cfg))

	cm, ok := e.modules["cline"]
	require.True(t, ok)
	t.Logf("Exported functions: %+v", cm.compiled.ExportedFunctions())
	t.Logf("hasExport 'list_models': %v", hasExport(cm.compiled, "list_models"))

	models, err := e.ListModels(context.Background(), "cline", core.Credentials{})
	require.NoError(t, err)
	t.Logf("models returned: %+v", models)
	require.NotEmpty(t, models)
}

// Verify wazero import is used.
var _ = fmt.Sprintf // ensure fmt import is used
