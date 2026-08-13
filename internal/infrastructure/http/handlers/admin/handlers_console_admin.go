package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bobbyunknown/flamegate/internal/shared/consolelog"
)

func (s *Handler) adminConsoleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial history.
	entries := s.consoleLog.Entries()
	initData, _ := json.Marshal(map[string]any{"type": "init", "logs": entries})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", initData)
	flusher.Flush()

	// Subscribe to new log lines via buffered channel.
	listener := consolelog.NewListener(256)
	s.consoleLog.Subscribe(listener)
	defer s.consoleLog.Unsubscribe(listener)

	// Keepalive ping every 25s.
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-listener.C:
			var data []byte
			if ev.Clear {
				data, _ = json.Marshal(map[string]any{"type": "clear"})
			} else {
				data, _ = json.Marshal(map[string]any{"type": "line", "log": ev.Entry})
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// adminConsoleClear clears the log buffer.
func (s *Handler) adminConsoleClear(w http.ResponseWriter, r *http.Request) {
	s.consoleLog.Clear()
	w.WriteHeader(http.StatusNoContent)
}

// ---- database export/import -------------------------------------------------
