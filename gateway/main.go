package main

import (
	"log"

	"ai-coding-interview-gateway/internal/bootstrap"
)

func main() {
	if err := bootstrap.Run(); err != nil {
		log.Fatalf("gateway boot failed: %v", err)
	}
}
