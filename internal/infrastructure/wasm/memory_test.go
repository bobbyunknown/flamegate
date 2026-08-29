package wasm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// buildAllocWASM builds a WASM module where alloc returns a fixed pointer
// (1024) so we can test write/read to guest memory. The alloc function
// reads its i32 parameter but ignores it — it always returns 1024.
func buildAllocWASM(t *testing.T) []byte {
	t.Helper()

	// Types: (i32)->(i32), (i32,i32)->(), (i32,i32)->(i32)
	typeSection := encodeSection(1, []byte{
		0x03,
		0x60, 0x01, 0x7f, 0x01, 0x7f, // type 0: (i32)->(i32)
		0x60, 0x02, 0x7f, 0x7f, 0x00, // type 1: (i32,i32)->()
		0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, // type 2: (i32,i32)->(i32)
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

	// alloc: drop param, push 1024, end
	allocBody := encodeFuncBody(nil, []byte{
		0x41, 0x80, 0x08, // i32.const 1024 (LEB128)
		0x0b, // end
	})
	deallocBody := encodeFuncBody(nil, []byte{0x0b})
	invokeBody := encodeFuncBody(nil, []byte{
		0x41, 0x00, // i32.const 0
		0x0b, // end
	})
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
	require.NoError(t, err, "allocWASM must compile")

	return binary
}

func TestWriteGuestJSON_Success(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	wasmBin := buildAllocWASM(t)
	require.NoError(t, e.Compile(context.Background(), "mem-ext", wasmBin, ExtensionConfig{
		Slug: "mem-ext",
	}))

	env := &InvokeEnv{Ctx: context.Background(), Slug: "mem-ext"}
	mod, err := e.Instantiate(context.Background(), "mem-ext", env)
	require.NoError(t, err)
	defer mod.Close(context.Background())

	type payload struct {
		Msg string `json:"msg"`
	}
	input := payload{Msg: "hello"}

	ptr, size, err := writeGuestJSON(context.Background(), mod, input)
	require.NoError(t, err)
	assert.Equal(t, uint32(1024), ptr)
	assert.Greater(t, size, uint32(0))

	// Read back
	var got payload
	require.NoError(t, readGuestJSON(mod, ptr, size, &got))
	assert.Equal(t, "hello", got.Msg)
}

func TestWriteGuestJSON_MemoryLimit(t *testing.T) {
	t.Skip("TODO: create null-alloc WASM fixture (compileMinimalWASM alloc returns 1024, not 0)")
}


func TestWriteGuestBytes_Success(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	wasmBin := buildAllocWASM(t)
	require.NoError(t, e.Compile(context.Background(), "mem-ext", wasmBin, ExtensionConfig{
		Slug: "mem-ext",
	}))

	env := &InvokeEnv{Ctx: context.Background(), Slug: "mem-ext"}
	mod, err := e.Instantiate(context.Background(), "mem-ext", env)
	require.NoError(t, err)
	defer mod.Close(context.Background())

	data := []byte("raw bytes test")
	ptr, size, err := writeGuestBytes(context.Background(), mod, data)
	require.NoError(t, err)
	assert.Equal(t, uint32(1024), ptr)
	assert.Equal(t, uint32(len(data)), size)

	got, err := readGuestBytes(mod, ptr, size)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestDeallocGuest_Success(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close()

	wasmBin := buildAllocWASM(t)
	require.NoError(t, e.Compile(context.Background(), "mem-ext", wasmBin, ExtensionConfig{
		Slug: "mem-ext",
	}))

	env := &InvokeEnv{Ctx: context.Background(), Slug: "mem-ext"}
	mod, err := e.Instantiate(context.Background(), "mem-ext", env)
	require.NoError(t, err)
	defer mod.Close(context.Background())

	ptr, size, err := writeGuestBytes(context.Background(), mod, []byte("dealloc test"))
	require.NoError(t, err)

	err = deallocGuest(context.Background(), mod, ptr, size)
	assert.NoError(t, err)
}
