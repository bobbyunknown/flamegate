package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
)

func TestService_LoadCache(t *testing.T) {
	t.Run("EmptyCachePath", func(t *testing.T) {
		svc := catalog.New(catalog.Config{
			CachePath: "",
		})
		err := svc.LoadCache()
		require.NoError(t, err)
		assert.Empty(t, svc.ListAllModels())
	})

	t.Run("MissingFile", func(t *testing.T) {
		tempDir := t.TempDir()
		missingPath := filepath.Join(tempDir, "nonexistent-catalog.json")

		svc := catalog.New(catalog.Config{
			CachePath: missingPath,
		})
		err := svc.LoadCache()
		require.NoError(t, err)
		assert.Empty(t, svc.ListAllModels())
	})

	t.Run("ExistingValidFile", func(t *testing.T) {
		tempDir := t.TempDir()
		cacheFile := filepath.Join(tempDir, "catalog.json")
		err := os.WriteFile(cacheFile, []byte(sampleCatalogJSON), 0644)
		require.NoError(t, err)

		svc := catalog.New(catalog.Config{
			CachePath: cacheFile,
		})
		err = svc.LoadCache()
		require.NoError(t, err)

		spec, ok := svc.FindModel("google", "gemini-2.5-flash")
		require.True(t, ok)
		assert.Equal(t, "gemini-2.5-flash", spec.ID)
		assert.Equal(t, "google", spec.Provider)
		assert.NotEmpty(t, svc.ListAllModels())
	})

	t.Run("CorruptedFile", func(t *testing.T) {
		tempDir := t.TempDir()
		cacheFile := filepath.Join(tempDir, "corrupted.json")
		err := os.WriteFile(cacheFile, []byte("{not valid json content"), 0644)
		require.NoError(t, err)

		svc := catalog.New(catalog.Config{
			CachePath: cacheFile,
		})
		err = svc.LoadCache()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse catalog cache")
	})
}

func TestService_SyncNow(t *testing.T) {
	t.Run("200_OK_With_ETag_And_Atomic_Write", func(t *testing.T) {
		tempDir := t.TempDir()
		cacheFile := filepath.Join(tempDir, "nested", "cache", "models.json")
		etagHeader := `W/"etag-test-12345"`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			w.Header().Set("ETag", etagHeader)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleCatalogJSON))
		}))
		defer server.Close()

		svc := catalog.New(catalog.Config{
			URL:       server.URL,
			CachePath: cacheFile,
		})
		svc.SetHTTPClient(server.Client())

		ctx := context.Background()
		err := svc.SyncNow(ctx)
		require.NoError(t, err)

		// Check internal state
		assert.Equal(t, etagHeader, svc.ETag())
		assert.False(t, svc.LastSync().IsZero())

		spec, ok := svc.FindModel("openai", "gpt-4o")
		require.True(t, ok)
		assert.Equal(t, "gpt-4o", spec.ID)

		// Check file written on disk
		diskBytes, err := os.ReadFile(cacheFile)
		require.NoError(t, err)
		assert.Equal(t, sampleCatalogJSON, string(diskBytes))
	})

	t.Run("304_Not_Modified", func(t *testing.T) {
		etagHeader := `W/"etag-fixed"`
		var requestsCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestsCount, 1)
			if r.Header.Get("If-None-Match") == etagHeader {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etagHeader)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleCatalogJSON))
		}))
		defer server.Close()

		svc := catalog.New(catalog.Config{
			URL: server.URL,
		})
		svc.SetHTTPClient(server.Client())

		// First sync: 200 OK
		err := svc.SyncNow(context.Background())
		require.NoError(t, err)
		assert.Equal(t, etagHeader, svc.ETag())
		firstSyncTime := svc.LastSync()

		time.Sleep(10 * time.Millisecond)

		// Second sync: 304 Not Modified
		err = svc.SyncNow(context.Background())
		require.NoError(t, err)
		assert.Equal(t, etagHeader, svc.ETag())
		assert.True(t, svc.LastSync().After(firstSyncTime))
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestsCount))

		// Models should still be loaded
		spec, ok := svc.FindModel("anthropic", "claude-3-7-sonnet")
		require.True(t, ok)
		assert.Equal(t, "claude-3-7-sonnet", spec.ID)
	})

	t.Run("500_Internal_Server_Error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal database error"))
		}))
		defer server.Close()

		svc := catalog.New(catalog.Config{
			URL: server.URL,
		})
		svc.SetHTTPClient(server.Client())

		err := svc.SyncNow(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
		assert.Contains(t, err.Error(), "internal database error")
	})

	t.Run("Invalid_JSON_Response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{broken-json-data"))
		}))
		defer server.Close()

		svc := catalog.New(catalog.Config{
			URL: server.URL,
		})
		svc.SetHTTPClient(server.Client())

		err := svc.SyncNow(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load catalog response")
	})

	t.Run("Context_Canceled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleCatalogJSON))
		}))
		defer server.Close()

		svc := catalog.New(catalog.Config{
			URL: server.URL,
		})
		svc.SetHTTPClient(server.Client())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := svc.SyncNow(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

func TestService_Start_Cancellation(t *testing.T) {
	svc := catalog.New(catalog.Config{
		SyncInterval: 10 * time.Minute,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		svc.Start(ctx)
		close(done)
	}()

	// Cancel context to ensure background loop exits
	cancel()

	select {
	case <-done:
		// Success, loop terminated cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("expected Start() to terminate upon context cancellation")
	}
}

func TestWriteAtomic(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "a", "b", "c", "catalog.json")

	data1 := []byte(`{"version": 1}`)
	err := os.WriteFile(filepath.Join(tempDir, "dummy"), data1, 0644)
	require.NoError(t, err)

	svc := catalog.New(catalog.Config{
		CachePath: targetFile,
	})
	_ = svc

	// Verify sync writes atomically even when parent dir does not exist
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleCatalogJSON))
	}))
	defer server.Close()

	svcWithCache := catalog.New(catalog.Config{
		URL:       server.URL,
		CachePath: targetFile,
	})
	svcWithCache.SetHTTPClient(server.Client())

	err = svcWithCache.SyncNow(context.Background())
	require.NoError(t, err)

	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, sampleCatalogJSON, string(content))
}
