package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ai-coding-interview/gateway/internal/config"
)

func TestRoundRobinAndHealthRecovery(t *testing.T) {
	var secondHealthy atomic.Bool
	first := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" && !secondHealthy.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	manager, err := New([]config.UpstreamConfig{{
		ID:        "users",
		Endpoints: []string{first.URL, second.URL},
		Health: config.HealthConfig{
			Path:     "/health",
			Interval: 10 * time.Millisecond,
			Timeout:  time.Second,
		},
	}}, http.DefaultTransport)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)

	firstPick, ok := manager.Pick("users")
	if !ok || firstPick.URL.String() != first.URL {
		t.Fatalf("initial Pick() = %v, %v", firstPick, ok)
	}

	secondHealthy.Store(true)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Status([]string{"users"})[0].Healthy == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Status([]string{"users"})[0].Healthy != 2 {
		t.Fatal("second endpoint did not become healthy")
	}

	pickedOne, _ := manager.Pick("users")
	pickedTwo, _ := manager.Pick("users")
	if pickedOne.URL.String() == pickedTwo.URL.String() {
		t.Fatalf("round robin selected %q twice", pickedOne.URL.String())
	}
}
