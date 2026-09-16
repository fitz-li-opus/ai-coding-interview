package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"gateway/internal/httpx"
	"gateway/internal/metrics"
)

// Middleware wraps an http.Handler with additional behaviour.
type Middleware func(http.Handler) http.Handler

// Chain is an ordered list of middleware.
type Chain struct {
	mws []Middleware
}

// New builds a Chain.
func New(mws ...Middleware) Chain { return Chain{mws: mws} }

// Then applies the chain to h in order.
func (c Chain) Then(h http.Handler) http.Handler {
	for i := len(c.mws) - 1; i >= 0; i-- {
		h = c.mws[i](h)
	}
	return h
}

// Registry maps middleware names to constructors for config-driven chains.
type Registry struct {
	mu        sync.Mutex
	factories map[string]func() Middleware
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]func() Middleware{}}
}

// Register adds a named middleware factory.
func (r *Registry) Register(name string, factory func() Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Build resolves names into middleware, returning an error on unknown names.
func (r *Registry) Build(names []string) ([]Middleware, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Middleware, 0, len(names))
	for _, name := range names {
		factory, ok := r.factories[name]
		if !ok {
			return nil, fmt.Errorf("middleware: unknown name %q", name)
		}
		out = append(out, factory())
	}
	return out, nil
}

// Names returns the registered names sorted.
func (r *Registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.factories))
	for name := range r.factories {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// NewRequestID returns a random 128-bit hex request id.
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// RequestID ensures every request carries an X-Request-ID and exposes it on
// the request context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = NewRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(httpx.WithRequestID(r.Context(), id)))
	})
}

// Recovery converts a panic in a downstream handler into a 500 response.
func Recovery(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "error", fmt.Sprint(rec), "path", r.URL.Path)
					httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logging emits one structured access log line per request.
func Logging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := httpx.NewStatusRecorder(w)
			next.ServeHTTP(rec, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.Status,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", httpx.ClientIP(r),
				"request_id", httpx.RequestIDFrom(r.Context()),
			)
		})
	}
}

// Observe records request metrics using the resolved route from context.
func Observe(m *metrics.Metrics) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx, info := httpx.WithRouteInfo(r.Context())
			rec := httpx.NewStatusRecorder(w)
			next.ServeHTTP(rec, r.WithContext(ctx))
			m.ObserveRequest(info.ID, r.Method, rec.Status, time.Since(start))
		})
	}
}
