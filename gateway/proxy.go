package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
)

func newRouteProxy(route RouteSpec, logger *log.Logger) *httputil.ReverseProxy {
	target := *route.Target

	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			originalHost := request.Host
			request.Header.Del("Forwarded")
			request.Header.Del("X-Forwarded-For")
			request.Header.Del("X-Forwarded-Host")
			request.Header.Del("X-Forwarded-Proto")

			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			request.URL.Path = joinProxyPath(target.Path, request.URL.Path)
			request.URL.RawPath = ""
			request.Host = target.Host
			request.Header.Set("X-Forwarded-Host", originalHost)
			if request.TLS == nil {
				request.Header.Set("X-Forwarded-Proto", "http")
			} else {
				request.Header.Set("X-Forwarded-Proto", "https")
			}
		},
		ErrorLog: logger,
	}

	proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		logger.Printf("proxy error: service=%s error=%v", route.Name, err)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			writeJSONError(writer, http.StatusGatewayTimeout, "gateway_timeout", "upstream service timed out")
			return
		}
		if strings.Contains(err.Error(), "request body too large") {
			writeJSONError(writer, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit")
			return
		}
		writeJSONError(writer, http.StatusBadGateway, "bad_gateway", "upstream service is unavailable")
	}

	return proxy
}

func joinProxyPath(basePath, requestPath string) string {
	if basePath == "" || basePath == "/" {
		if requestPath == "" {
			return "/"
		}
		return requestPath
	}
	if requestPath == "" || requestPath == "/" {
		return basePath
	}
	if strings.HasSuffix(basePath, "/") && strings.HasPrefix(requestPath, "/") {
		return basePath + requestPath[1:]
	}
	if !strings.HasSuffix(basePath, "/") && !strings.HasPrefix(requestPath, "/") {
		return basePath + "/" + requestPath
	}
	return basePath + requestPath
}
