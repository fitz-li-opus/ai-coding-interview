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
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Printf("gateway stopped with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	logger := log.New(os.Stdout, "gateway ", log.LstdFlags|log.LUTC)
	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	handler, err := NewHandler(cfg, logger)
	if err != nil {
		return err
	}
	server := newHTTPServer(cfg.Address, handler)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	serveError := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", cfg.Address)
		serveError <- server.ListenAndServe()
	}()

	select {
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case received := <-stop:
		logger.Printf("received %s; shutting down", received)
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return err
	}
	if err := <-serveError; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
