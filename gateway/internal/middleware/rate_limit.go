package middleware

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	visitorTTL      = 10 * time.Minute
	cleanupInterval = time.Minute
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type LimiterStore struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	now      func() time.Time
}

func NewLimiterStore() *LimiterStore {
	return &LimiterStore{
		visitors: make(map[string]*visitor),
		now:      time.Now,
	}
}

func (s *LimiterStore) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanup()
			}
		}
	}()
}

func (s *LimiterStore) Allow(routeID, clientIP string, requestsPerSecond float64, burst int) bool {
	if requestsPerSecond == 0 && burst == 0 {
		return true
	}
	key := routeID + "\x00" + clientIP
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.visitors[key]
	if !exists {
		entry = &visitor{limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), burst)}
		s.visitors[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter.AllowN(now, 1)
}

func (s *LimiterStore) cleanup() {
	cutoff := s.now().Add(-visitorTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.visitors {
		if entry.lastSeen.Before(cutoff) {
			delete(s.visitors, key)
		}
	}
}
