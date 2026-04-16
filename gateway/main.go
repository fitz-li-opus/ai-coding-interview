package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

var routes = map[string]string{
	"/api/users":    "http://user-service:8080",
	"/api/orders":   "http://order-service:8080",
	"/api/products": "http://product-service:8080",
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	for prefix, target := range routes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			targetURL, _ := url.Parse(target)
			httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)
			return
		}
	}
	http.Error(w, "no route matched", http.StatusNotFound)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy", "service": "api-gateway"}`))
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", proxyHandler)
	log.Println("api-gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
