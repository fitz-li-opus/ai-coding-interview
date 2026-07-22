package main

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache(time.Minute)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	cache.Set("/api/users", 200, headers, []byte(`{"users":[]}`))

	entry, found := cache.Get("/api/users")
	if !found {
		t.Fatal("Expected cache hit")
	}
	if entry.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", entry.StatusCode)
	}
	if string(entry.Body) != `{"users":[]}` {
		t.Errorf("Body = %q, want %q", entry.Body, `{"users":[]}`)
	}
	if entry.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", entry.Headers.Get("Content-Type"))
	}
}

func TestCacheMiss(t *testing.T) {
	cache := NewCache(time.Minute)

	_, found := cache.Get("/api/unknown")
	if found {
		t.Error("Expected cache miss for unknown key")
	}
}

func TestCacheExpiry(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)

	cache.Set("/api/users", 200, http.Header{}, []byte("data"))

	// Should be found immediately
	_, found := cache.Get("/api/users")
	if !found {
		t.Error("Expected cache hit before expiry")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, found = cache.Get("/api/users")
	if found {
		t.Error("Expected cache miss after expiry")
	}
}
