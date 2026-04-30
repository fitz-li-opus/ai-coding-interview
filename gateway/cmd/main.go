package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gateway/internal/config"
	"gateway/internal/factory"
	"gateway/internal/router"
)

func main() {
	config.Load()

	repos, err := factory.NewRepoFactory(config.AppConfig)
	if err != nil {
		log.Fatalf("init repos: %v", err)
	}

	services, err := factory.NewService(config.AppConfig, repos)
	if err != nil {
		log.Fatalf("init services: %v", err)
	}

	handlers := factory.NewHandler(services)
	middlewares := factory.NewMiddleware()

	mux := router.Setup(handlers, middlewares)

	srv := &http.Server{
		Addr:              ":" + config.AppConfig.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown: SIGINT/SIGTERM stops accepting new requests and waits
	// up to 10s for in-flight ones before exiting.
	go func() {
		log.Printf("gateway listening on :%s", config.AppConfig.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("gateway stopped")
}
