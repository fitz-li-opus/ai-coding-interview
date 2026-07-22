package main

import (
	"testing"
)

func TestLoadBalancerRoundRobin(t *testing.T) {
	lb := NewLoadBalancer()
	lb.AddBackends("user-service", []string{
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
	})

	// Round-robin should cycle through backends
	results := make([]string, 6)
	for i := 0; i < 6; i++ {
		results[i] = lb.Next("user-service")
	}

	expected := []string{
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
	}

	for i, want := range expected {
		if results[i] != want {
			t.Errorf("Next() call %d = %q, want %q", i, results[i], want)
		}
	}
}

func TestLoadBalancerSingleBackend(t *testing.T) {
	lb := NewLoadBalancer()
	lb.AddBackends("order-service", []string{"http://localhost:8082"})

	for i := 0; i < 3; i++ {
		got := lb.Next("order-service")
		if got != "http://localhost:8082" {
			t.Errorf("Next() = %q, want http://localhost:8082", got)
		}
	}
}

func TestLoadBalancerUnknownService(t *testing.T) {
	lb := NewLoadBalancer()
	got := lb.Next("unknown-service")
	if got != "" {
		t.Errorf("Next(unknown) = %q, want empty string", got)
	}
}
