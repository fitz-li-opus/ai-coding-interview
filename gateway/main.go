package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("invalid gateway configuration: %v", err)
	}
	addr := fmt.Sprintf(":%s", cfg.Port)
	if err := http.ListenAndServe(addr, newServer(cfg)); err != nil {
		log.Fatalf("gateway failed: %v", err)
	}
}
