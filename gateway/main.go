package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type config struct {
	ListenAddr         string
	UserServiceURL     string
	OrderServiceURL    string
	ProductServiceURL  string
	MaxBodyBytes       int64
	UpstreamTimeout    time.Duration
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
	ReadyCheckTimeout  time.Duration
	ReadyCheckParallel int
}

type gateway struct {
	cfg             config
	userProxy       *httputil.ReverseProxy
	orderProxy      *httputil.ReverseProxy
	productProxy    *httputil.ReverseProxy
	readyHTTPClient *http.Client
}

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

func main() {
	cfg := loadConfig()

	gw, err := newGateway(cfg)
	if err != nil {
		log.Fatalf("gateway init error: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", gw.healthz)
	mux.HandleFunc("/readyz", gw.readyz)

	mux.Handle("/api/users/", gw.userProxy)
	mux.Handle("/api/users", gw.userProxy)
	mux.Handle("/api/orders/", gw.orderProxy)
	mux.Handle("/api/orders", gw.orderProxy)
	mux.Handle("/api/products/", gw.productProxy)
	mux.Handle("/api/products", gw.productProxy)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":      "not_found",
			"message":    "no route matched",
			"path":       r.URL.Path,
			"request_id": requestIDFromContext(r.Context()),
		})
	})

	handler := chain(
		mux,
		withRecover,
		withRequestID,
		withMaxBodyBytes(cfg.MaxBodyBytes),
		withUpstreamTimeout(cfg.UpstreamTimeout),
		withAccessLog,
	)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("gateway listening on %s", cfg.ListenAddr)
		errCh <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("shutdown signal: %s", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func loadConfig() config {
	cfg := config{
		ListenAddr:         getenv("GATEWAY_LISTEN_ADDR", ":8080"),
		UserServiceURL:     getenv("USER_SERVICE_URL", "http://localhost:8081"),
		OrderServiceURL:    getenv("ORDER_SERVICE_URL", "http://localhost:8082"),
		ProductServiceURL:  getenv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
		MaxBodyBytes:       getenvInt64("MAX_BODY_BYTES", 10<<20),
		UpstreamTimeout:    getenvDuration("UPSTREAM_TIMEOUT", 30*time.Second),
		ReadHeaderTimeout:  getenvDuration("READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:        getenvDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:       getenvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:        getenvDuration("IDLE_TIMEOUT", 90*time.Second),
		ShutdownTimeout:    getenvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		ReadyCheckTimeout:  getenvDuration("READY_CHECK_TIMEOUT", 2*time.Second),
		ReadyCheckParallel: int(getenvInt64("READY_CHECK_PARALLEL", 3)),
	}
	if cfg.ReadyCheckParallel <= 0 {
		cfg.ReadyCheckParallel = 3
	}
	return cfg
}

func newGateway(cfg config) (*gateway, error) {
	userURL, err := url.Parse(cfg.UserServiceURL)
	if err != nil {
		return nil, err
	}
	orderURL, err := url.Parse(cfg.OrderServiceURL)
	if err != nil {
		return nil, err
	}
	productURL, err := url.Parse(cfg.ProductServiceURL)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
	}

	gw := &gateway{
		cfg: cfg,
		readyHTTPClient: &http.Client{
			Timeout: cfg.ReadyCheckTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 10 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          50,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: cfg.ReadyCheckTimeout,
			},
		},
	}

	gw.userProxy = newRouteProxy(userURL, "/api/users", transport)
	gw.orderProxy = newRouteProxy(orderURL, "/api/orders", transport)
	gw.productProxy = newRouteProxy(productURL, "/api/products", transport)

	return gw, nil
}

func newRouteProxy(target *url.URL, stripPrefix string, transport http.RoundTripper) *httputil.ReverseProxy {
	rp := httputil.NewSingleHostReverseProxy(target)
	originalDirector := rp.Director

	rp.Transport = transport

	rp.Director = func(req *http.Request) {
		originalDirector(req)

		req.URL.Path = stripPathPrefix(req.URL.Path, stripPrefix)
		req.URL.RawPath = stripPathPrefix(req.URL.RawPath, stripPrefix)

		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}

		if rid := requestIDFromContext(req.Context()); rid != "" {
			req.Header.Set("X-Request-Id", rid)
		}
	}

	rp.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Server")
		if rid := requestIDFromContext(resp.Request.Context()); rid != "" {
			resp.Header.Set("X-Request-Id", rid)
		}
		return nil
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusGatewayTimeout
		}
		writeJSON(w, status, map[string]any{
			"error":      "upstream_error",
			"message":    err.Error(),
			"request_id": requestIDFromContext(r.Context()),
		})
	}

	return rp
}

func stripPathPrefix(path, prefix string) string {
	if path == "" {
		return ""
	}
	if path == prefix {
		return "/"
	}
	if strings.HasPrefix(path, prefix+"/") {
		rest := strings.TrimPrefix(path, prefix)
		if rest == "" {
			return "/"
		}
		return rest
	}
	return path
}

func (gw *gateway) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"service":    "api-gateway",
		"request_id": requestIDFromContext(r.Context()),
	})
}

func (gw *gateway) readyz(w http.ResponseWriter, r *http.Request) {
	type dep struct {
		name   string
		base   string
		health string
	}

	deps := []dep{
		{name: "user-service", base: gw.cfg.UserServiceURL, health: strings.TrimRight(gw.cfg.UserServiceURL, "/") + "/health"},
		{name: "order-service", base: gw.cfg.OrderServiceURL, health: strings.TrimRight(gw.cfg.OrderServiceURL, "/") + "/health"},
		{name: "product-service", base: gw.cfg.ProductServiceURL, health: strings.TrimRight(gw.cfg.ProductServiceURL, "/") + "/health"},
	}

	type res struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Status int    `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	ctx := r.Context()
	results := make([]res, len(deps))

	sem := make(chan struct{}, gw.cfg.ReadyCheckParallel)
	var wg sync.WaitGroup
	wg.Add(len(deps))

	for i := range deps {
		i := i
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()

			out := res{Name: deps[i].name, URL: deps[i].base}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, deps[i].health, nil)
			if err != nil {
				out.OK = false
				out.Error = err.Error()
				results[i] = out
				return
			}
			resp, err := gw.readyHTTPClient.Do(req)
			if err != nil {
				out.OK = false
				out.Error = err.Error()
				results[i] = out
				return
			}
			defer resp.Body.Close()

			out.Status = resp.StatusCode
			out.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
			results[i] = out
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	allOK := true
	for i := range results {
		if !results[i].OK {
			allOK = false
			break
		}
	}

	status := http.StatusOK
	if !allOK {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"status":     map[bool]string{true: "ready", false: "not_ready"}[allOK],
		"service":    "api-gateway",
		"deps":       results,
		"request_id": requestIDFromContext(ctx),
	})
}

type middleware func(http.Handler) http.Handler

func chain(h http.Handler, m ...middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

type ctxKey string

const requestIDKey ctxKey = "request_id"

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if rid == "" {
			rid = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
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

func withMaxBodyBytes(max int64) middleware {
	return func(next http.Handler) http.Handler {
		if max <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

func withUpstreamTimeout(timeout time.Duration) middleware {
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

func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &responseRecorder{ResponseWriter: w}

		next.ServeHTTP(rr, r)

		dur := time.Since(start)
		rid := requestIDFromContext(r.Context())
		clientIP := clientIPFromRequest(r)

		log.Printf("request method=%s path=%s status=%d bytes=%d dur=%s rid=%s ip=%s ua=%q",
			r.Method, r.URL.Path, rr.status, rr.bytes, dur.String(), rid, clientIP, r.UserAgent(),
		)
	})
}

func clientIPFromRequest(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var out int64
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return def
		}
		out = out*10 + int64(v[i]-'0')
	}
	return out
}

func getenvDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
