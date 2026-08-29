package wasm

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHotReloader_FileHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wasm")
	require.NoError(t, os.WriteFile(path, []byte("test content"), 0644))

	hash1, err := fileHash(path)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)

	// Same content → same hash.
	hash2, err := fileHash(path)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Different content → different hash.
	require.NoError(t, os.WriteFile(path, []byte("different content"), 0644))
	hash3, err := fileHash(path)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestHotReloader_RecordRemoveHash(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	mod := NewModule(e, "test-ext", 4)
	modules := map[string]*Module{"test-ext": mod}

	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, "/nonexistent", modules, time.Second, log)

	hr.RecordHash("test-ext", "abc123")
	assert.Equal(t, "abc123", hr.hashes["test-ext"])

	hr.RemoveHash("test-ext")
	_, ok := hr.hashes["test-ext"]
	assert.False(t, ok)
}

func TestHotReloader_SkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0755))

	// Write a .wasm file.
	wasmPath := filepath.Join(extDir, "test-ext.wasm")
	require.NoError(t, os.WriteFile(wasmPath, compileMinimalWASM(t), 0644))

	e := newEngineForTest(t)
	defer e.Close()

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateDisabled) // DISABLED

	modules := map[string]*Module{"test-ext": mod}
	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, dir, modules, time.Second, log)

	// Pre-populate hash.
	hash, _ := fileHash(wasmPath)
	hr.RecordHash("test-ext", hash)

	// Change the file.
	require.NoError(t, os.WriteFile(wasmPath, []byte("changed"), 0644))

	// Scan should skip DISABLED.
	hr.scan(context.Background())

	// Hash should NOT be updated (extension was skipped).
	assert.Equal(t, hash, hr.hashes["test-ext"])
}

func TestHotReloader_SkipsUpdating(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0755))

	wasmPath := filepath.Join(extDir, "test-ext.wasm")
	require.NoError(t, os.WriteFile(wasmPath, compileMinimalWASM(t), 0644))

	e := newEngineForTest(t)
	defer e.Close()

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateUpdating) // UPDATING

	modules := map[string]*Module{"test-ext": mod}
	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, dir, modules, time.Second, log)

	hash, _ := fileHash(wasmPath)
	hr.RecordHash("test-ext", hash)

	require.NoError(t, os.WriteFile(wasmPath, []byte("changed"), 0644))

	hr.scan(context.Background())

	assert.Equal(t, hash, hr.hashes["test-ext"])
}

func TestHotReloader_ReloadOnHashChange(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0755))

	wasmPath := filepath.Join(extDir, "test-ext.wasm")
	require.NoError(t, os.WriteFile(wasmPath, compileMinimalWASM(t), 0644))

	e := newEngineForTest(t)
	defer e.Close()

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)
	modules := map[string]*Module{"test-ext": mod}
	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, dir, modules, time.Second, log)

	hash, _ := fileHash(wasmPath)
	hr.RecordHash("test-ext", hash)

	// Write new valid WASM.
	require.NoError(t, os.WriteFile(wasmPath, compileMinimalWASM(t), 0644))

	hr.scan(context.Background())

	// Hash should be updated (reload triggered).
	newHash, _ := fileHash(wasmPath)
	assert.Equal(t, newHash, hr.hashes["test-ext"])
	assert.Equal(t, StateActive, mod.State())
}

func TestHotReloader_NoReloadWhenHashUnchanged(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0755))

	wasmPath := filepath.Join(extDir, "test-ext.wasm")
	require.NoError(t, os.WriteFile(wasmPath, compileMinimalWASM(t), 0644))

	e := newEngineForTest(t)
	defer e.Close()

	mod := NewModule(e, "test-ext", 4)
	modules := map[string]*Module{"test-ext": mod}
	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, dir, modules, time.Second, log)

	hash, _ := fileHash(wasmPath)
	hr.RecordHash("test-ext", hash)

	// Scan without changing file — no reload.
	hr.scan(context.Background())

	assert.Equal(t, hash, hr.hashes["test-ext"])
}

func TestHotReloader_RunStopsOnContext(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	log := logrus.WithField("test", true)
	hr := NewHotReloader(e, "/tmp", nil, 10*time.Millisecond, log)

	ctx, cancel := context.WithCancel(context.Background())

	var stopped atomic.Bool
	go func() {
		hr.Run(ctx)
		stopped.Store(true)
	}()

	cancel()
	time.Sleep(50 * time.Millisecond)
	assert.True(t, stopped.Load())
}
