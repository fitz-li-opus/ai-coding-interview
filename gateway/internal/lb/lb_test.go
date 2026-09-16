package lb

import "testing"

func mustInstance(t *testing.T, raw string, weight int) *Instance {
	t.Helper()
	inst, err := NewInstance(raw, weight)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	return inst
}

func TestNewInstance(t *testing.T) {
	inst := mustInstance(t, "http://localhost:8081", 1)
	if !inst.Healthy() {
		t.Fatal("new instance should be healthy")
	}
	if inst.String() != "http://localhost:8081" {
		t.Fatalf("string got %q", inst.String())
	}
	inst.MarkUnhealthy()
	if inst.Healthy() {
		t.Fatal("expected unhealthy")
	}
	inst.MarkHealthy()
	if !inst.Healthy() {
		t.Fatal("expected healthy")
	}
}

func TestNewInstanceInvalidURL(t *testing.T) {
	if _, err := NewInstance("://bad", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestRoundRobin(t *testing.T) {
	rr := &RoundRobin{}
	a := mustInstance(t, "http://a", 1)
	b := mustInstance(t, "http://b", 1)
	cands := []*Instance{a, b}

	if rr.Pick(nil) != nil {
		t.Fatal("expected nil for empty candidates")
	}
	got := []string{
		rr.Pick(cands).String(),
		rr.Pick(cands).String(),
		rr.Pick(cands).String(),
		rr.Pick(cands).String(),
	}
	want := []string{a.String(), b.String(), a.String(), b.String()}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestWeighted(t *testing.T) {
	w := NewWeighted(1)
	a := mustInstance(t, "http://a", 3)
	b := mustInstance(t, "http://b", 1)

	if w.Pick(nil) != nil {
		t.Fatal("expected nil for empty candidates")
	}

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		counts[w.Pick([]*Instance{a, b}).String()]++
	}
	if counts[a.String()] <= counts[b.String()] {
		t.Fatalf("expected a to win more: %v", counts)
	}
	if counts[a.String()]+counts[b.String()] != 400 {
		t.Fatalf("total mismatch: %v", counts)
	}
}

func TestWeightedZeroTotalFallsBackToFirst(t *testing.T) {
	w := NewWeighted(1)
	a := mustInstance(t, "http://a", 0)
	b := mustInstance(t, "http://b", 0)
	if got := w.Pick([]*Instance{a, b}); got != a {
		t.Fatalf("expected first, got %v", got)
	}
}

func TestWeightedSkipsNonPositive(t *testing.T) {
	w := NewWeighted(1)
	a := mustInstance(t, "http://a", 0)
	b := mustInstance(t, "http://b", 1)
	for i := 0; i < 10; i++ {
		if got := w.Pick([]*Instance{a, b}); got != b {
			t.Fatalf("expected b, got %v", got)
		}
	}
}

func TestNew(t *testing.T) {
	for _, name := range []string{"", "round_robin"} {
		bal, err := New(name)
		if err != nil {
			t.Fatalf("new %q: %v", name, err)
		}
		if _, ok := bal.(*RoundRobin); !ok {
			t.Fatalf("new %q got %T", name, bal)
		}
	}
	bal, err := New("weighted")
	if err != nil {
		t.Fatalf("new weighted: %v", err)
	}
	if _, ok := bal.(*Weighted); !ok {
		t.Fatalf("got %T", bal)
	}
	if _, err := New("magic"); err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}
