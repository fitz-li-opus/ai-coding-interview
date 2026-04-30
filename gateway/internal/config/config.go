package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Route struct {
	Name       string
	PathPrefix string
	Target     string
}

type Config struct {
	Port            string
	ENV             string
	UpstreamTimeout time.Duration
	Routes          []Route
}

var AppConfig *Config

// Load resolves config from environment variables. In local development a
// .env file (gitignored) is loaded first as a convenience; in k8s the same
// vars are projected from a ConfigMap/Secret, so application code stays
// agnostic about the source.
func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from process environment only")
	}

	AppConfig = &Config{
		Port:            getEnv("PORT", "8080"),
		ENV:             getEnv("ENV", "local"),
		UpstreamTimeout: getEnvDuration("UPSTREAM_TIMEOUT", 10*time.Second),
		Routes: []Route{
			{
				Name:       "user-service",
				PathPrefix: getEnv("USER_SERVICE_PREFIX", "/api/users"),
				Target:     getEnv("USER_SERVICE_URL", "http://localhost:8081"),
			},
			{
				Name:       "order-service",
				PathPrefix: getEnv("ORDER_SERVICE_PREFIX", "/api/orders"),
				Target:     getEnv("ORDER_SERVICE_URL", "http://localhost:8082"),
			},
			{
				Name:       "product-service",
				PathPrefix: getEnv("PRODUCT_SERVICE_PREFIX", "/api/products"),
				Target:     getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
			},
		},
	}

	log.Printf("config loaded: ENV=%s Port=%s upstreams=%d timeout=%s",
		AppConfig.ENV, AppConfig.Port, len(AppConfig.Routes), AppConfig.UpstreamTimeout)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid duration for %s=%q, using default %s", key, v, fallback)
		return fallback
	}
	return d
}
