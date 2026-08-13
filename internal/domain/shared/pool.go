// Reusable buffer pools to reduce GC pressure in the hot path. Every request
// that touches the proxy (read body, scan SSE, write response) can grab a
// buffer from a pool instead of allocating fresh memory.
//
// The pools are sized to match common AI-proxy workloads:
//   - 32 KiB: typical SSE line or small JSON response
//   - 64 KiB: medium response body or request payload
//   - 256 KiB: large response body (rare, but happens with big tool outputs)
//
// Callers MUST reset buffers before use and MUST NOT retain references after
// returning them to the pool.
package shared

import (
	"bufio"
	"bytes"
	"sync"
)

// BufPool32K pools *bytes.Buffer with 32 KiB initial capacity. Suitable for
// SSE line rendering, small JSON payloads, and intermediate serialization.
var BufPool32K = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 32*1024)) },
}

// BufPool64K pools *bytes.Buffer with 64 KiB initial capacity. Suitable for
// request/response bodies and medium-sized upstream payloads.
var BufPool64K = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 64*1024)) },
}

// BufPool256K pools *bytes.Buffer with 256 KiB initial capacity. For large
// response bodies (tool outputs, big completions).
var BufPool256K = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 256*1024)) },
}

// SSEWriterPool pools *bufio.Writer sized for SSE event writing. Writers are
// reset to the target http.ResponseWriter before use.
var SSEWriterPool = sync.Pool{
	New: func() any { return bufio.NewWriterSize(nil, 16*1024) },
}

// GetBuf32K grabs a 32 KiB buffer from the pool. Caller must call PutBuf32K
// when done.
func GetBuf32K() *bytes.Buffer {
	buf := BufPool32K.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuf32K returns a 32 KiB buffer to the pool.
func PutBuf32K(b *bytes.Buffer) {
	if b.Cap() <= 64*1024 { // don't pool oversized buffers
		BufPool32K.Put(b)
	}
}

// GetBuf64K grabs a 64 KiB buffer from the pool.
func GetBuf64K() *bytes.Buffer {
	buf := BufPool64K.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuf64K returns a 64 KiB buffer to the pool.
func PutBuf64K(b *bytes.Buffer) {
	if b.Cap() <= 128*1024 { // don't pool oversized buffers
		BufPool64K.Put(b)
	}
}

// GetBuf256K grabs a 256 KiB buffer from the pool.
func GetBuf256K() *bytes.Buffer {
	buf := BufPool256K.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuf256K returns a 256 KiB buffer to the pool.
func PutBuf256K(b *bytes.Buffer) {
	if b.Cap() <= 512*1024 { // don't pool oversized buffers
		BufPool256K.Put(b)
	}
}
