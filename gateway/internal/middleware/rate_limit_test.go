package middleware

import (
	"testing"
	"time"
)

func TestLimiterStoreScopesByRouteAndClient(t *testing.T) {
	store := NewLimiterStore()
	now := time.Now()
	store.now = func() time.Time { return now }
	if !store.Allow("users", "192.0.2.1", 1, 1) {
		t.Fatal("first request was rejected")
	}
	if store.Allow("users", "192.0.2.1", 1, 1) {
		t.Fatal("second request was allowed")
	}
	if !store.Allow("orders", "192.0.2.1", 1, 1) {
		t.Fatal("different route was rejected")
	}
	if !store.Allow("users", "192.0.2.2", 1, 1) {
		t.Fatal("different client was rejected")
	}

	now = now.Add(visitorTTL + time.Second)
	store.cleanup()
	if len(store.visitors) != 0 {
		t.Fatalf("visitors after cleanup = %d", len(store.visitors))
	}
}
