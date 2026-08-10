package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerAppliesResourceLimits(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	server := newHTTPServer("127.0.0.1:9090", handler)

	if server.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, want %q", server.Addr, "127.0.0.1:9090")
	}
	server.Handler.ServeHTTP(nil, nil)
	if !called {
		t.Error("configured Handler was not preserved")
	}
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %s, want 5s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %s, want 10s", server.ReadTimeout)
	}
	if server.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %s, want 15s", server.WriteTimeout)
	}
	if server.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %s, want 60s", server.IdleTimeout)
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, 1<<20)
	}
}
