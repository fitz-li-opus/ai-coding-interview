package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Logger interface {
	Printf(format string, v ...any)
}

type Middleware func(http.Handler) http.Handler

type ctxKey string

const requestIDContextKey ctxKey = "request_id"

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.status = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *responseRecorder) Write(p []byte) (int, error) {
	if rr.status == 0 {
		rr.status = http.StatusOK
	}
	n, err := rr.ResponseWriter.Write(p)
	rr.bytes += int64(n)
	return n, err
}

func chain(h http.Handler, m ...Middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDContextKey).(string)
	return v
}

func withRequestID(header string) Middleware {
	if header == "" {
		header = "X-Request-Id"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get(header)
			if rid == "" {
				rid = newRequestID()
			}
			w.Header().Set(header, rid)
			ctx := context.WithValue(r.Context(), requestIDContextKey, rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func withRecover(logger Logger) Middleware {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Printf("panic recovered: %v", rec)
					writeJSON(w, http.StatusInternalServerError, map[string]any{
						"error":      "internal_error",
						"message":    "panic recovered",
						"request_id": requestIDFromContext(r.Context()),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func withMaxBodyBytes(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		if max <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > max {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
					"error":      "payload_too_large",
					"message":    "request body too large",
					"request_id": requestIDFromContext(r.Context()),
				})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

func withUpstreamTimeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		if timeout <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func withAccessLog(logger Logger) Middleware {
	if logger == nil {
		logger = log.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rr := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(rr, r)
			logger.Printf(
				"request method=%s path=%s status=%d bytes=%d dur=%s rid=%s",
				r.Method,
				r.URL.Path,
				rr.status,
				rr.bytes,
				time.Since(start),
				requestIDFromContext(r.Context()),
			)
		})
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
