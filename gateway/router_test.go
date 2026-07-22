package main

import (
	"testing"
)

func TestRouterMatch(t *testing.T) {
	router := NewRouter()
	router.AddRoute("/api/users", "user-service")
	router.AddRoute("/api/orders", "order-service")
	router.AddRoute("/api/products", "product-service")

	tests := []struct {
		path        string
		wantService string
		wantFound   bool
	}{
		{"/api/users", "user-service", true},
		{"/api/users/123", "user-service", true},
		{"/api/users/profile/settings", "user-service", true},
		{"/api/orders", "order-service", true},
		{"/api/orders/456", "order-service", true},
		{"/api/products", "product-service", true},
		{"/api/unknown", "", false},
		{"/health", "", false},
		{"/", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			service, found := router.Match(tt.path)
			if found != tt.wantFound {
				t.Errorf("Match(%q) found = %v, want %v", tt.path, found, tt.wantFound)
			}
			if service != tt.wantService {
				t.Errorf("Match(%q) service = %q, want %q", tt.path, service, tt.wantService)
			}
		})
	}
}
