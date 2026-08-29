package handlers

import (
	"net/http"

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
