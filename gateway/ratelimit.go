package main

import (
	"sync"
	"time"
)

// RateLimiter limits requests per IP using a sliding window.
type RateLimiter struct {
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	mu       sync.Mutex
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request from the given IP is allowed.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get existing requests and filter out expired ones
	requests := rl.requests[ip]
	var valid []time.Time
	for _, t := range requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	// Check if under limit
	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}

	// Allow request and record it
	rl.requests[ip] = append(valid, now)
	return true
}
