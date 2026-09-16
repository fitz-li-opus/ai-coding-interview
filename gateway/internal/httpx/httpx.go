package httpx

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

type ctxKey int

const requestIDKey ctxKey = iota

// WithRequestID stores the request id on the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom returns the request id stored on the context, if any.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// Principal is the authenticated caller identity resolved from an API key.
type Principal struct {
	ID   string
	Plan string
}

type principalKeyType int

const principalKey principalKeyType = iota

// WithPrincipal attaches the authenticated caller to the context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom returns the authenticated caller, if any.
func PrincipalFrom(ctx context.Context) *Principal {
	if v, ok := ctx.Value(principalKey).(*Principal); ok {
		return v
	}
	return nil
}

// RouteInfo is a mutable holder the dispatcher fills in once a route is
// resolved, so outer middleware can observe the route after the fact.
type RouteInfo struct {
	ID string
}

type routeInfoKeyType int

const routeInfoKey routeInfoKeyType = iota

// WithRouteInfo attaches a RouteInfo holder to the context.
func WithRouteInfo(ctx context.Context) (context.Context, *RouteInfo) {
	info := &RouteInfo{}
	return context.WithValue(ctx, routeInfoKey, info), info
}

// RouteInfoFrom returns the RouteInfo holder stored on the context, if any.
func RouteInfoFrom(ctx context.Context) *RouteInfo {
	if v, ok := ctx.Value(routeInfoKey).(*RouteInfo); ok {
		return v
	}
	return nil
}

// Error is the JSON error envelope returned by the gateway.
type Error struct {
	Status    int    `json:"status"`
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a JSON error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, Error{Status: status, Error: msg})
}

// ClientIP returns the best-effort client IP, honouring X-Forwarded-For.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// StatusRecorder wraps a ResponseWriter to remember the status code.
type StatusRecorder struct {
	http.ResponseWriter
	Status int
	Wrote  bool
}

// NewStatusRecorder returns a StatusRecorder defaulting to 200.
func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder {
	return &StatusRecorder{ResponseWriter: w, Status: http.StatusOK}
}

func (s *StatusRecorder) WriteHeader(code int) {
	if !s.Wrote {
		s.Status = code
		s.Wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *StatusRecorder) Write(b []byte) (int, error) {
	if !s.Wrote {
		s.Status = http.StatusOK
		s.Wrote = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush implements http.Flusher when the underlying writer supports it.
func (s *StatusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
