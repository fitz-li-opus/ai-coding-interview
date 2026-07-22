package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	// Configuration from environment
	port := getEnvInt("GATEWAY_PORT", 8080)
	rateLimit := getEnvInt("RATE_LIMIT", 100)
	cacheTTL := getEnvInt("CACHE_TTL", 60)

	// Initialize components
	router := NewRouter()
	router.AddRoute("/api/users", "user-service")
	router.AddRoute("/api/orders", "order-service")
	router.AddRoute("/api/products", "product-service")

	lb := NewLoadBalancer()
	lb.AddBackends("user-service", []string{"http://localhost:8081"})
	lb.AddBackends("order-service", []string{"http://localhost:8082"})
	lb.AddBackends("product-service", []string{"http://localhost:8083"})

	cache := NewCache(time.Duration(cacheTTL) * time.Second)
	limiter := NewRateLimiter(rateLimit, time.Minute)

	// Create handler
	handler := NewProxyHandler(router, lb, cache, limiter)

	// Create mux for routing
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"api-gateway"}`))
	})

	// All other requests go to proxy handler
	mux.Handle("/", handler)

	// Start server
	addr := ":" + strconv.Itoa(port)
	log.Printf("API Gateway listening on %s", addr)
	log.Printf("Rate limit: %d requests/minute, Cache TTL: %ds", rateLimit, cacheTTL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
