package extstore

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveBytes returns a URL serving the bytes with the given name.
func serveBytes(t *testing.T, name string, data []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, name) {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/" + name
}

// httptestClient returns a client wired to httptest servers.
func httptestClient() *http.Client { return httptest.NewServer(nil).Client() }

// httptestServer serves fixed path→body fixtures and returns the server. The
// caller must Close it.
func httptestServer(t *testing.T, fixtures map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}