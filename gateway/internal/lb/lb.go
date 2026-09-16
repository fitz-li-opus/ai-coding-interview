package lb

import (
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"sync/atomic"
)

// Instance is a single upstream endpoint plus its runtime health state.
type Instance struct {
	URL    *url.URL
	Weight int

	healthy atomic.Bool
}

// NewInstance builds an instance and starts it in the healthy state.
func NewInstance(rawURL string, weight int) (*Instance, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("lb: parse url %q: %w", rawURL, err)
	}
	inst := &Instance{URL: u, Weight: weight}
	inst.healthy.Store(true)
	return inst, nil
}

func (i *Instance) String() string { return i.URL.String() }

func (i *Instance) Healthy() bool  { return i.healthy.Load() }
func (i *Instance) MarkHealthy()   { i.healthy.Store(true) }
func (i *Instance) MarkUnhealthy() { i.healthy.Store(false) }

// Balancer selects an upstream from the provided candidates.
type Balancer interface {
	Pick(candidates []*Instance) *Instance
}

// RoundRobin cycles through candidates.
type RoundRobin struct {
	n atomic.Uint64
}

func (r *RoundRobin) Pick(candidates []*Instance) *Instance {
	if len(candidates) == 0 {
		return nil
	}
	i := r.n.Add(1) - 1
	return candidates[i%uint64(len(candidates))]
}

// Weighted picks candidates proportionally to their weight.
type Weighted struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewWeighted creates a weighted balancer seeded by seed.
func NewWeighted(seed int64) *Weighted {
	return &Weighted{rng: rand.New(rand.NewSource(seed))}
}

func (w *Weighted) Pick(candidates []*Instance) *Instance {
	if len(candidates) == 0 {
		return nil
	}
	total := 0
	for _, c := range candidates {
		if c.Weight > 0 {
			total += c.Weight
		}
	}
	if total <= 0 {
		return candidates[0]
	}
	w.mu.Lock()
	roll := w.rng.Intn(total)
	w.mu.Unlock()
	for _, c := range candidates {
		if c.Weight <= 0 {
			continue
		}
		roll -= c.Weight
		if roll < 0 {
			return c
		}
	}
	return candidates[len(candidates)-1]
}

// New returns a balancer for the given strategy name.
func New(name string) (Balancer, error) {
	switch name {
	case "", "round_robin":
		return &RoundRobin{}, nil
	case "weighted":
		return NewWeighted(rand.Int63()), nil
	default:
		return nil, fmt.Errorf("lb: unknown strategy %q", name)
	}
}
