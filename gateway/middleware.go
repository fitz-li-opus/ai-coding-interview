package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

const headerRequestID = "X-Request-Id"

type requestIDKey struct{}

// Middleware uses the stdlib-idiomatic signature so that anything from the
// wider net/http ecosystem plugs in without an adapter.
type Middleware func(http.Handler) http.Handler

// Chain wraps h so that mws[0] ends up outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// AccessLog assigns a request id, propagates it both upstream and back to the
// client, and emits one line per request.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = newRequestID()
			// Set on the inbound header so the proxy carries it upstream.
			r.Header.Set(headerRequestID, id)
		}
		w.Header().Set(headerRequestID, id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)

		slog.Info("request",
			"request_id", id,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration", time.Since(start).Round(time.Microsecond),
		)
	})
}

// Recover sits *inside* AccessLog, not outside it. That way a panic still
// produces exactly one access log line — with status 500 — instead of
// vanishing between the two middlewares.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v) // net/http's own signal; swallowing it breaks the server
			}
			slog.Error("panic recovered",
				"request_id", requestIDFrom(r.Context()),
				"path", r.URL.Path,
				"panic", v,
				"stack", string(debug.Stack()),
			)
			if rec, ok := w.(interface{ Wrote() bool }); ok && rec.Wrote() {
				return // the response already started; nothing safe to write
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", "gateway panic")
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code for the access log. Unwrap keeps
// http.ResponseController working, which ReverseProxy relies on to flush
// streamed responses.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Wrote() bool { return s.wrote }

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
