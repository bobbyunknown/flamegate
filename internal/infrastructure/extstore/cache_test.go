package extstore

import (
	"testing"
	"time"
)

func TestTTLCacheSetGet(t *testing.T) {
	c := NewTTLCache()
	c.Set("a", 1, time.Minute)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Fatalf("Get after Set: got (%v, %v), want (1, true)", v, ok)
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	c := NewTTLCache()
	c.Set("a", 1, 30*time.Millisecond)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("immediate read should hit")
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected expiry after TTL")
	}
}

func TestTTLCacheMissAndDelete(t *testing.T) {
	c := NewTTLCache()
	if _, ok := c.Get("missing"); ok {
		t.Fatal("missing key should miss")
	}
	c.Set("k", "v", time.Minute)
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("deleted key should miss")
	}
}

func TestTTLCacheZeroTTL(t *testing.T) {
	c := NewTTLCache()
	c.Set("a", 1, 0)
	if _, ok := c.Get("a"); ok {
		t.Fatal("zero TTL should not store")
	}
}