package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Options struct {
	UserURL, OrderURL, ProductURL string

	MaxBodyBytes       int64
	UpstreamTimeout    time.Duration
	ReadyCheckTimeout  time.Duration
	ReadyCheckParallel int

	RequestIDHeader       string
	TrustForwardedHeaders bool
}

type deps struct {
	userProxy    *httputil.ReverseProxy
	orderProxy   *httputil.ReverseProxy
	productProxy *httputil.ReverseProxy
	readyClient  *http.Client
	opt          Options
}

func NewHandler(opt Options, logger Logger) (http.Handler, error) {
	parsed, err := parseUpstreamURLs(opt)
	if err != nil {
		return nil, err
	}

	transport := newTransport(opt.UpstreamTimeout)
	state := deps{
		userProxy:    newRouteProxy(parsed.users, "/api/users", transport, opt),
		orderProxy:   newRouteProxy(parsed.orders, "/api/orders", transport, opt),
		productProxy: newRouteProxy(parsed.products, "/api/products", transport, opt),
		readyClient: &http.Client{
			Timeout:   opt.ReadyCheckTimeout,
			Transport: newTransport(opt.ReadyCheckTimeout),
		},
		opt: opt,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/users/", state.userProxy)
	mux.Handle("/api/users", state.userProxy)
	mux.Handle("/api/orders/", state.orderProxy)
	mux.Handle("/api/orders", state.orderProxy)
	mux.Handle("/api/products/", state.productProxy)
	mux.Handle("/api/products", state.productProxy)
	mux.HandleFunc("/healthz", state.healthz)
	mux.HandleFunc("/readyz", state.readyz)
	mux.HandleFunc("/", notFound)

	return chain(
		mux,
		withRecover(logger),
		withRequestID(opt.RequestIDHeader),
		withMaxBodyBytes(opt.MaxBodyBytes),
		withUpstreamTimeout(opt.UpstreamTimeout),
		withAccessLog(logger),
	), nil
}

func parseUpstreamURLs(opt Options) (struct {
	users    *url.URL
	orders   *url.URL
	products *url.URL
}, error) {
	parsedUsers, err := url.Parse(opt.UserURL)
	if err != nil {
		return struct {
			users    *url.URL
			orders   *url.URL
			products *url.URL
		}{}, fmt.Errorf("parse user upstream: %w", err)
	}
	parsedOrders, err := url.Parse(opt.OrderURL)
	if err != nil {
		return struct {
			users    *url.URL
			orders   *url.URL
			products *url.URL
		}{}, fmt.Errorf("parse order upstream: %w", err)
	}
	parsedProducts, err := url.Parse(opt.ProductURL)
	if err != nil {
		return struct {
			users    *url.URL
			orders   *url.URL
			products *url.URL
		}{}, fmt.Errorf("parse product upstream: %w", err)
	}

	return struct {
		users    *url.URL
		orders   *url.URL
		products *url.URL
	}{
		users: parsedUsers, orders: parsedOrders, products: parsedProducts,
	}, nil
}

func newTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
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
		ResponseHeaderTimeout: timeout,
	}
}

func newRouteProxy(target *url.URL, stripPrefix string, transport http.RoundTripper, opt Options) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)

			r.Out.URL.Path = stripPathPrefix(r.In.URL.Path, stripPrefix)
			r.Out.URL.RawPath = stripPathPrefix(r.In.URL.RawPath, stripPrefix)

			if !opt.TrustForwardedHeaders {
				r.Out.Header.Del("X-Forwarded-For")
				r.Out.Header.Del("X-Forwarded-Host")
				r.Out.Header.Del("X-Forwarded-Proto")
			}
			r.SetXForwarded()

			if rid := requestIDFromContext(r.In.Context()); rid != "" {
				r.Out.Header.Set(opt.RequestIDHeader, rid)
			}
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Server")
			if rid := requestIDFromContext(resp.Request.Context()); rid != "" {
				resp.Header.Set(opt.RequestIDHeader, rid)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				status = http.StatusGatewayTimeout
			}
			writeJSON(w, status, map[string]any{
				"error":      "upstream_error",
				"message":    err.Error(),
				"request_id": requestIDFromContext(req.Context()),
			})
		},
	}
}

func stripPathPrefix(path, prefix string) string {
	if path == "" {
		return "/"
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

func (d deps) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"service":    "api-gateway",
		"request_id": requestIDFromContext(r.Context()),
	})
}

func (d deps) readyz(w http.ResponseWriter, r *http.Request) {
	type depInfo struct {
		name string
		base string
		url  string
	}
	items := []depInfo{
		{name: "user-service", base: d.opt.UserURL, url: strings.TrimRight(d.opt.UserURL, "/") + "/health"},
		{name: "order-service", base: d.opt.OrderURL, url: strings.TrimRight(d.opt.OrderURL, "/") + "/health"},
		{name: "product-service", base: d.opt.ProductURL, url: strings.TrimRight(d.opt.ProductURL, "/") + "/health"},
	}

	type checkResult struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Status int    `json:"status,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	resultsCh := make(chan checkResult, len(items))
	sem := make(chan struct{}, d.opt.ReadyCheckParallel)
	var wg sync.WaitGroup

	for _, item := range items {
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()

			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, item.url, nil)
			if err != nil {
				resultsCh <- checkResult{Name: item.name, URL: item.base, OK: false, Error: err.Error()}
				return
			}

			resp, err := d.readyClient.Do(req)
			if err != nil {
				resultsCh <- checkResult{Name: item.name, URL: item.base, OK: false, Error: err.Error()}
				return
			}
			defer resp.Body.Close()

			resultsCh <- checkResult{
				Name:   item.name,
				URL:    item.base,
				OK:     resp.StatusCode >= 200 && resp.StatusCode < 300,
				Status: resp.StatusCode,
			}
		}()
	}

	wg.Wait()
	close(resultsCh)

	results := make([]checkResult, 0, len(items))
	allOK := true
	for result := range resultsCh {
		results = append(results, result)
		if !result.OK {
			allOK = false
		}
	}

	status := http.StatusOK
	state := "ready"
	if !allOK {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	writeJSON(w, status, map[string]any{
		"status":     state,
		"service":    "api-gateway",
		"deps":       results,
		"request_id": requestIDFromContext(r.Context()),
	})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error":      "not_found",
		"message":    "no route matched",
		"path":       r.URL.Path,
		"request_id": requestIDFromContext(r.Context()),
	})
}
