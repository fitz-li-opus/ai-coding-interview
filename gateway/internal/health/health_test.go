package health

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gateway/internal/lb"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustInstance(t *testing.T, raw string) *lb.Instance {
	t.Helper()
	inst, err := lb.NewInstance(raw, 1)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	return inst
}

func TestNewActiveFallbacks(t *testing.T) {
	a := NewActive("", 0, 0, 0, 0, nil)
	if a.Path != "/health" || a.Interval != 5*time.Second || a.Timeout != 2*time.Second {
		t.Fatalf("fallbacks wrong: %+v", a)
	}
	if a.HealthyThreshold != 2 || a.UnhealthyThreshold != 3 {
		t.Fatalf("threshold fallbacks wrong")
	}
	if a.Logger == nil || a.Client == nil {
		t.Fatal("logger/client should be set")
	}
}

func TestActiveProbeSuccessAndFailure(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	a := NewActive("/health", time.Second, time.Second, 2, 3, testLogger())

	good := mustInstance(t, okSrv.URL)
	good.MarkUnhealthy()
	a.ProbeOnce(good)
	if good.Healthy() {
		t.Fatal("should stay unhealthy after one success")
	}
	a.ProbeOnce(good)
	if !good.Healthy() {
		t.Fatal("should become healthy after threshold")
	}
	a.ProbeOnce(good) // already healthy path

	bad := mustInstance(t, badSrv.URL)
	a.ProbeOnce(bad)
	a.ProbeOnce(bad)
	if !bad.Healthy() {
		t.Fatal("should still be healthy after two failures")
	}
	a.ProbeOnce(bad)
	if bad.Healthy() {
		t.Fatal("should become unhealthy after threshold")
	}
	a.ProbeOnce(bad) // already unhealthy path
}

func TestActiveProbeTransportError(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	inst := mustInstance(t, dead.URL)
	a := NewActive("/health", time.Second, time.Second, 1, 2, testLogger())
	a.ProbeOnce(inst)
	a.ProbeOnce(inst)
	if inst.Healthy() {
		t.Fatal("expected unhealthy after transport errors")
	}
}

func TestActiveStartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inst := mustInstance(t, srv.URL)
	a := NewActive("/health", 10*time.Millisecond, time.Second, 1, 1, testLogger())
	a.Start([]*lb.Instance{inst})
	time.Sleep(30 * time.Millisecond)
	a.Stop()
	a.Stop()
}

func TestNewPassiveFallbacks(t *testing.T) {
	p := NewPassive(0, 0)
	if p.Threshold != 3 || p.Cooldown != 5*time.Second {
		t.Fatalf("fallbacks wrong: %+v", p)
	}
}

func TestPassiveEjectionAndCooldown(t *testing.T) {
	p := NewPassive(2, time.Minute)
	inst := mustInstance(t, "http://a")
	now := time.Now()

	if !p.Allow(inst, now) {
		t.Fatal("new instance should be allowed")
	}
	p.Report(inst, errors.New("boom"), now)
	if !p.Allow(inst, now) {
		t.Fatal("should still be allowed below threshold")
	}
	if p.Failures(inst) != 1 {
		t.Fatalf("failures got %d", p.Failures(inst))
	}
	p.Report(inst, errors.New("boom"), now)
	if p.Allow(inst, now) {
		t.Fatal("should be ejected at threshold")
	}
	if !p.Ejected(inst, now) {
		t.Fatal("Ejected should be true")
	}
	if !p.Allow(inst, now.Add(2*time.Minute)) {
		t.Fatal("should be allowed after cooldown")
	}
}

func TestPassiveSuccessResets(t *testing.T) {
	p := NewPassive(2, time.Minute)
	inst := mustInstance(t, "http://a")
	now := time.Now()

	p.Report(inst, errors.New("boom"), now)
	if p.Failures(inst) != 1 {
		t.Fatalf("failures got %d", p.Failures(inst))
	}
	p.Report(inst, nil, now)
	if p.Failures(inst) != 0 {
		t.Fatalf("failures should reset, got %d", p.Failures(inst))
	}

	p.Report(inst, errors.New("boom"), now)
	p.Report(inst, errors.New("boom"), now)
	if !p.Ejected(inst, now) {
		t.Fatal("expected ejected")
	}
	p.Report(inst, nil, now)
	if p.Ejected(inst, now) {
		t.Fatal("success should clear ejection")
	}
}
