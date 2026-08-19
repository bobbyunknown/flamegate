package extstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadOk(t *testing.T) {
	payload := strings.Repeat("a", 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	d := NewDownloader(srv.Client())
	path, err := d.FetchToTemp(context.Background(), srv.URL, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != payload {
		t.Fatalf("content mismatch: got %d bytes want %d", len(b), len(payload))
	}
}

func TestDownloadTooLarge(t *testing.T) {
	payload := strings.Repeat("b", 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	d := NewDownloader(srv.Client())
	path, err := d.FetchToTemp(context.Background(), srv.URL, 1024)
	if err == nil {
		os.Remove(path)
		t.Fatal("expected ErrDownloadTooLarge")
	}
	if !errors.Is(err, ErrDownloadTooLarge) {
		t.Fatalf("err = %v, want ErrDownloadTooLarge", err)
	}
	if path != "" {
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatal("oversized download left a temp file behind")
		}
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	d := NewDownloader(srv.Client())
	path, err := d.FetchToTemp(context.Background(), srv.URL, 4096)
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if path != "" {
		os.Remove(path)
	}
}