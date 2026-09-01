package router

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/static"
)

// RegisterEmbeddedUIRoutes mounts the embedded frontend dashboard onto a Chi router.
func RegisterEmbeddedUIRoutes(r chi.Router) {
	dist, err := fs.Sub(static.Files, static.DistDir)
	if err != nil || !embeddedFileExists(dist, "index.html") {
		return
	}

	indexBytes, err := fs.ReadFile(dist, "index.html")
	if err != nil || len(indexBytes) == 0 {
		return
	}

	fileServer := http.FileServer(http.FS(dist))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if shouldSkipEmbeddedUI(path) {
			http.NotFound(w, req)
			return
		}

		if path != "" && embeddedFileExists(dist, path) {
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, req)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}

func shouldSkipEmbeddedUI(path string) bool {
	for _, prefix := range []string{"api", "v1", "oauth", "healthz", "metrics", "docs", "openapi", "swagger"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func embeddedFileExists(filesystem fs.FS, name string) bool {
	_, err := fs.Stat(filesystem, name)
	return err == nil
}
