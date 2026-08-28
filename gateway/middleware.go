package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const headerRequestID = "X-Request-ID"

// reqInfo travels with the request so the access log can report what the
// router decided, and error responses can quote the original inbound path.
type reqInfo struct {
	id       string
	path     string
	route    string
	upstream string
}

type reqInfoKey struct{}

func infoFrom(ctx context.Context) *reqInfo {
	info, _ := ctx.Value(reqInfoKey{}).(*reqInfo)
	return info
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// withRequestInfo reuses an inbound X-Request-ID when the caller already has a
// trace, otherwise mints one. It is echoed to the client and to the upstream.
func withRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(headerRequestID, id)
		info := &reqInfo{id: id, path: r.URL.Path}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), reqInfoKey{}, info)))
	})
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		// Deferred so a panic recovered further in is still logged.
		defer func() {
			info := infoFrom(r.Context())
			if info == nil {
				info = &reqInfo{path: r.URL.Path}
			}
			logger.Info("request",
				"request_id", info.id,
				"method", r.Method,
				"path", info.path,
				"route", info.route,
				"upstream", info.upstream,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", float64(time.Since(start).Microseconds())/1000,
				"client_ip", clientIP(r),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// net/http uses ErrAbortHandler to abort a response on purpose
			// (ReverseProxy does this); let it through untouched.
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			logger.Error("panic in handler", "err", rec, "path", r.URL.Path)
			writeError(w, r, http.StatusInternalServerError, "internal gateway error", "")
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code and body size for the access log.
// Unwrap lets http.ResponseController reach the real writer, which is what
// ReverseProxy uses for flushing streamed responses and protocol upgrades.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.status, s.wrote = code, true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type errorBody struct {
	Error     string `json:"error"`
	Status    int    `json:"status"`
	Path      string `json:"path"`
	Upstream  string `json:"upstream,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// writeError keeps every gateway-generated failure in one JSON shape, so a
// client can tell a gateway error from a backend error.
func writeError(w http.ResponseWriter, r *http.Request, status int, msg, upstream string) {
	body := errorBody{Error: msg, Status: status, Path: r.URL.Path, Upstream: upstream}
	if info := infoFrom(r.Context()); info != nil {
		body.RequestID = info.id
		body.Path = info.path
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
