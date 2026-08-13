package wasm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// ErrMemoryLimit indicates the guest's alloc returned 0, meaning the
// WASM linear memory is exhausted.
var ErrMemoryLimit = fmt.Errorf("wasm: memory limit reached (alloc returned 0)")

// writeGuestJSON marshals v as JSON, calls guest alloc(len+4), writes a
// 4-byte little-endian length prefix followed by the JSON bytes, and returns
// (ptr, size, error). ptr points to the length prefix. The guest reads the
// first 4 bytes to know how many bytes follow.
// Returns ErrMemoryLimit if alloc returns 0.
func writeGuestJSON(ctx context.Context, mod api.Module, v any) (ptr uint32, size uint32, err error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, 0, fmt.Errorf("wasm: marshal json: %w", err)
	}

	allocFn := mod.ExportedFunction("alloc")
	if allocFn == nil {
		return 0, 0, fmt.Errorf("wasm: guest does not export alloc")
	}

	total := len(data) + 4
	results, err := allocFn.Call(ctx, uint64(total))
	if err != nil {
		return 0, 0, fmt.Errorf("wasm: call alloc(%d): %w", total, err)
	}

	ptr = uint32(results[0])
	if ptr == 0 {
		return 0, 0, ErrMemoryLimit
	}

	mem := mod.Memory()
	ln := uint32(len(data))
	lenBytes := []byte{byte(ln), byte(ln >> 8), byte(ln >> 16), byte(ln >> 24)}
	if !mem.Write(ptr, lenBytes) {
		return 0, 0, fmt.Errorf("wasm: write length prefix at offset %d failed", ptr)
	}
	if !mem.Write(ptr+4, data) {
		return 0, 0, fmt.Errorf("wasm: write %d bytes at offset %d failed", len(data), ptr+4)
	}

	return ptr, uint32(len(data)), nil
}

// writeGuestRawLenPrefix writes raw bytes to guest memory with 4-byte LE length prefix.
// This is the wire format guest reads via read_host_json: [4-byte LE len][data bytes].
func writeGuestRawLenPrefix(ctx context.Context, mod api.Module, data []byte) (uint32, error) {
	allocFn := mod.ExportedFunction("alloc")
	if allocFn == nil {
		return 0, fmt.Errorf("wasm: guest does not export alloc")
	}

	total := len(data) + 4
	results, err := allocFn.Call(ctx, uint64(total))
	if err != nil {
		return 0, fmt.Errorf("wasm: call alloc(%d): %w", total, err)
	}

	ptr := uint32(results[0])
	if ptr == 0 {
		return 0, ErrMemoryLimit
	}

	mem := mod.Memory()
	ln := uint32(len(data))
	lenBytes := []byte{byte(ln), byte(ln >> 8), byte(ln >> 16), byte(ln >> 24)}
	if !mem.Write(ptr, lenBytes) {
		return 0, fmt.Errorf("wasm: write length prefix at offset %d failed", ptr)
	}
	if !mem.Write(ptr+4, data) {
		return 0, fmt.Errorf("wasm: write %d bytes at offset %d failed", len(data), ptr+4)
	}
	return ptr, nil
}

// writeGuestBytes writes raw bytes to guest memory via alloc. Returns
// ErrMemoryLimit if alloc returns 0.
func writeGuestBytes(ctx context.Context, mod api.Module, data []byte) (ptr uint32, size uint32, err error) {
	allocFn := mod.ExportedFunction("alloc")
	if allocFn == nil {
		return 0, 0, fmt.Errorf("wasm: guest does not export alloc")
	}

	results, err := allocFn.Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, 0, fmt.Errorf("wasm: call alloc(%d): %w", len(data), err)
	}

	ptr = uint32(results[0])
	if ptr == 0 {
		return 0, 0, ErrMemoryLimit
	}

	mem := mod.Memory()
	if !mem.Write(ptr, data) {
		return 0, 0, fmt.Errorf("wasm: write %d bytes at offset %d failed", len(data), ptr)
	}

	return ptr, uint32(len(data)), nil
}

// readGuestJSON reads a 4-byte little-endian length prefix at ptr, then
// reads that many bytes starting at ptr+4 and unmarshals the JSON into dst.
func readGuestJSON(mod api.Module, ptr, _size uint32, dst any) error {
	mem := mod.Memory()
	prefix, ok := mem.Read(ptr, 4)
	if !ok {
		return fmt.Errorf("wasm: read length prefix at offset %d failed", ptr)
	}
	size := uint32(prefix[0]) | uint32(prefix[1])<<8 | uint32(prefix[2])<<16 | uint32(prefix[3])<<24
	data, ok := mem.Read(ptr+4, size)
	if !ok {
		return fmt.Errorf("wasm: read %d bytes at offset %d failed", size, ptr+4)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("wasm: unmarshal json from guest: %w", err)
	}
	return nil
}

// readGuestBytes reads raw bytes from guest linear memory at ptr.
func readGuestBytes(mod api.Module, ptr, size uint32) ([]byte, error) {
	mem := mod.Memory()
	data, ok := mem.Read(ptr, size)
	if !ok {
		return nil, fmt.Errorf("wasm: read %d bytes at offset %d failed", size, ptr)
	}
	return data, nil
}

// deallocGuest calls the guest's dealloc(ptr, size) to free memory.
func deallocGuest(ctx context.Context, mod api.Module, ptr, size uint32) error {
	deallocFn := mod.ExportedFunction("dealloc")
	if deallocFn == nil {
		return fmt.Errorf("wasm: guest does not export dealloc")
	}
	_, err := deallocFn.Call(ctx, uint64(ptr), uint64(size))
	if err != nil {
		return fmt.Errorf("wasm: call dealloc(%d, %d): %w", ptr, size, err)
	}
	return nil
}
