package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	configPath := flag.String("config", "routes.yaml", "path to YAML routing config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	handler, err := newHandler(cfg.Routes)
	if err != nil {
		log.Fatalf("build handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"healthy","service":"api-gateway"}`)
	})
	mux.Handle("/", handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("api-gateway listening on %s", addr)
	for _, r := range cfg.Routes {
		log.Printf("  %s → %s", r.Prefix, r.Target)
	}
	log.Fatal(srv.ListenAndServe())
}
