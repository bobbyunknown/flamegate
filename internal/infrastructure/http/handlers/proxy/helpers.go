package proxy

import (
	"net/http"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

// WriteJSON writes a JSON response with the given status. Uses Sonic-backed
// fastjson.Marshal for JIT-compiled serialization.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	data, err := fastjson.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"internal error","type":"server_error"}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeJSON is a package-internal alias for WriteJSON.
func writeJSON(w http.ResponseWriter, status int, v any) { WriteJSON(w, status, v) }

// WriteError writes a JSON error response for raw Chi handlers (streaming/SSE).
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

// tenantOf returns the tenant id for an authenticated key.
func tenantOf(key schema.APIKey) string {
	if key.TenantID != "" {
		return key.TenantID
	}
	return schema.DefaultTenantID
}
