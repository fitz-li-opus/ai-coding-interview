package main

import (
	"sync/atomic"
)

// LoadBalancer distributes requests across backends using round-robin.
type LoadBalancer struct {
	backends map[string][]string // service -> backend URLs
	counters map[string]*uint64  // service -> atomic counter
}

// NewLoadBalancer creates a new LoadBalancer.
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		backends: make(map[string][]string),
		counters: make(map[string]*uint64),
	}
}

// AddBackends registers backend URLs for a service.
func (lb *LoadBalancer) AddBackends(service string, urls []string) {
	lb.backends[service] = urls
	var counter uint64
	lb.counters[service] = &counter
}

// Next returns the next backend URL for a service using round-robin.
// Returns empty string if service is not found.
func (lb *LoadBalancer) Next(service string) string {
	backends, ok := lb.backends[service]
	if !ok || len(backends) == 0 {
		return ""
	}

	counter := lb.counters[service]
	index := atomic.AddUint64(counter, 1) - 1
	return backends[index%uint64(len(backends))]
}
