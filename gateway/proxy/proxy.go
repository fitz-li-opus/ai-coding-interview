package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ReverseProxy wraps httputil.ReverseProxy with prefix stripping.
type ReverseProxy struct {
	target      *url.URL
	proxy       *httputil.ReverseProxy
	prefix      string
	stripPrefix bool
}

func New(upstream string, prefix string, stripPrefix bool) (*ReverseProxy, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}

	rp := &ReverseProxy{
		target:      target,
		prefix:      prefix,
		stripPrefix: stripPrefix,
	}

	rp.proxy = &httputil.ReverseProxy{
		Rewrite: rp.rewrite,
	}

	return rp, nil
}

func (rp *ReverseProxy) rewrite(pr *httputil.ProxyRequest) {
	pr.SetURL(rp.target)
	pr.SetXForwarded()

	if rp.stripPrefix {
		path := strings.TrimPrefix(pr.Out.URL.Path, rp.prefix)
		if path == "" {
			path = "/"
		}
		pr.Out.URL.Path = path
	}
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.proxy.ServeHTTP(w, r)
}
