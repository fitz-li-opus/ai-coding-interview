package main

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if rl.Allow("192.168.1.1") {
		t.Error("Request 6 should be denied")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// Each IP has its own limit
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("IP1 request 3 should be denied")
	}

	// Different IP should have fresh limit
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 2 should be allowed")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	if !rl.Allow("192.168.1.1") {
		t.Error("Request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("Request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("Request 3 should be denied")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow("192.168.1.1") {
		t.Error("Request after window expiry should be allowed")
	}
}
