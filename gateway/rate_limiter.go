package main

import (
	"sync"
	"time"
)

type FixedWindowRateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	windowStart time.Time
	count       int
	now         func() time.Time
}

func NewFixedWindowRateLimiter(limit int, window time.Duration) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		limit:       limit,
		window:      window,
		windowStart: time.Now(),
		now:         time.Now,
	}
}

func (l *FixedWindowRateLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.windowStart) >= l.window {
		l.windowStart = now
		l.count = 0
	}

	if l.count >= l.limit {
		return false
	}

	l.count++
	return true
}
