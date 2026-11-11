package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const upstreamTimeout = 10 * time.Second

func main() {
	routes := map[string]string{
		"/api/users":    getenv("USER_SERVICE_URL", "http://localhost:8081"),
		"/api/orders":   getenv("ORDER_SERVICE_URL", "http://localhost:8082"),
		"/api/products": getenv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
	}
	handler, err := newGateway(routes, upstreamTimeout)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: ":8080", Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Println("gateway listening on :8080")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	stop() // from here a second Ctrl+C kills the process instead of waiting for the shutdown
	log.Println("gateway shutting down")
	// Let in-flight requests finish, but never hang forever.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("shutdown:", err)
	}
}

// newGateway forwards each route prefix to its upstream with the prefix stripped
// (/api/users/1?x=1 -> <upstream>/1?x=1). Anything else gets a JSON 404.
func newGateway(routes map[string]string, timeout time.Duration) (http.Handler, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 100 // default is 2: under load most upstream connections would be reopened per request

	mux := http.NewServeMux()
	for prefix, target := range routes {
		u, err := url.Parse(target)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("route %s: invalid upstream URL %q", prefix, target)
		}
		h := http.StripPrefix(prefix, newProxy(u, transport, timeout))
		// Register both forms so /api/users isn't 301-redirected to /api/users/, and /api/usersX matches neither.
		mux.Handle(prefix, h)
		mux.Handle(prefix+"/", h)
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "gateway"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no route for " + r.URL.Path})
	})
	return withRequestLog(mux), nil
}

func newProxy(target *url.URL, transport http.RoundTripper, timeout time.Duration) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status, msg := http.StatusBadGateway, "upstream unavailable"
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				status, msg = http.StatusGatewayTimeout, "upstream timed out"
			}
			log.Printf("id=%q upstream=%s error=%q", r.Header.Get("X-Request-ID"), target.Host, err)
			writeJSON(w, status, map[string]string{"error": msg})
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bounds the whole upstream exchange, including a body that stalls after the headers were sent.
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withRequestLog gives every request an X-Request-ID (kept if the client sent one),
// passes it upstream and back to the client, and writes one access-log line per request.
func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
			r = r.Clone(r.Context()) // handlers must not modify the incoming request
			r.Header.Set("X-Request-ID", id)
		}
		w.Header().Set("X-Request-ID", id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			p := recover() // ReverseProxy aborts a response cut off mid-body with panic(http.ErrAbortHandler)
			log.Printf("id=%q method=%s path=%s status=%d aborted=%t duration=%s", id, r.Method, r.URL.RequestURI(), rec.status, p != nil, time.Since(start))
			if p != nil {
				panic(p) // hand it back to net/http, which closes the connection
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController (used by ReverseProxy to flush streams) reach the real writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func newRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
