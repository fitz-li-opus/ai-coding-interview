package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the gateway's own error envelope. It is shaped differently from
// the backends' responses on purpose, so a caller can tell which hop failed,
// and it echoes request_id so that failure can be grepped out of the log.
type errorBody struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody{
		Error:     code,
		Message:   message,
		RequestID: requestIDFrom(r.Context()),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("write response", "err", err)
	}
}
