package http

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type HTTPBackend struct {
	name   string
	target *url.URL
	proxy  *httputil.ReverseProxy
}

func NewHTTPBackend(name, target string, timeout time.Duration) (*HTTPBackend, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse target %q: %w", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	transport := &http.Transport{
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
	}
	proxy.Transport = transport

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[%s] upstream error path=%s err=%v", name, r.URL.Path, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return &HTTPBackend{name: name, target: u, proxy: proxy}, nil
}

func (b *HTTPBackend) Name() string { return b.name }

func (b *HTTPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.proxy.ServeHTTP(w, r)
}
