package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// sharedTransport is deliberately one instance for the whole gateway.
// Connection pooling lives on the Transport, so building one per route — let
// alone one per request — would defeat keep-alive entirely.
var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   2 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   100, // the stdlib default of 2 is far too low here
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

func newRouteHandler(route *Route) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			if route.StripPrefix {
				stripPrefix(pr.Out.URL, route.Prefix)
			}
			pr.SetURL(route.Target)
			// Rewrite, unlike the legacy Director hook, adds no forwarding
			// headers of its own. SetXForwarded *replaces* any inbound
			// X-Forwarded-For: the gateway is the trust boundary, so a
			// client-supplied chain must not be believed.
			pr.SetXForwarded()
		},
		Transport:    sharedTransport,
		ErrorHandler: proxyErrorHandler,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The per-route deadline, not a server WriteTimeout, is what bounds an
		// upstream call. It surfaces in ErrorHandler as DeadlineExceeded.
		ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	})
}

// stripPrefix removes prefix from u's path, preserving the client's original
// percent-encoding of everything after it. Rewriting u.Path alone would decode
// an escaped separator into a real one.
func stripPrefix(u *url.URL, prefix string) {
	if prefix == "" {
		return
	}
	rest := strings.TrimPrefix(u.EscapedPath(), prefix)
	if rest == "" || rest[0] != '/' {
		rest = "/" + rest
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		decoded = rest
	}
	u.Path = decoded
	u.RawPath = rest
}

func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	id := requestIDFrom(r.Context())
	switch {
	case errors.Is(err, context.Canceled):
		// The client hung up. There is nobody left to answer, and writing
		// would only produce a "superfluous WriteHeader" warning.
		slog.Warn("client canceled", "request_id", id, "path", r.URL.Path)
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		slog.Warn("upstream timeout", "request_id", id, "path", r.URL.Path, "err", err)
		writeError(w, r, http.StatusGatewayTimeout, "gateway_timeout", "upstream did not respond in time")
	default:
		slog.Error("upstream unreachable", "request_id", id, "path", r.URL.Path, "err", err)
		writeError(w, r, http.StatusBadGateway, "bad_gateway", "upstream is unavailable")
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
