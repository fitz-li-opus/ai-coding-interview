package health

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gateway/internal/lb"
)

// Active periodically probes upstream health endpoints and flips the
// instance health flag based on consecutive successes/failures.
type Active struct {
	Client             *http.Client
	Path               string
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
	Logger             *slog.Logger

	mu        sync.Mutex
	successes map[*lb.Instance]int
	failures  map[*lb.Instance]int

	stop chan struct{}
	wg   sync.WaitGroup
}

// NewActive builds an Active prober with sane fallbacks for zero values.
func NewActive(path string, interval, timeout time.Duration, healthy, unhealthy int, logger *slog.Logger) *Active {
	if path == "" {
		path = "/health"
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if healthy <= 0 {
		healthy = 2
	}
	if unhealthy <= 0 {
		unhealthy = 3
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Active{
		Client:             &http.Client{Timeout: timeout},
		Path:               path,
		Interval:           interval,
		Timeout:            timeout,
		HealthyThreshold:   healthy,
		UnhealthyThreshold: unhealthy,
		Logger:             logger,
		successes:          map[*lb.Instance]int{},
		failures:           map[*lb.Instance]int{},
		stop:               make(chan struct{}),
	}
}

// Start launches one probe loop per instance.
func (a *Active) Start(instances []*lb.Instance) {
	for _, inst := range instances {
		a.wg.Add(1)
		go func(inst *lb.Instance) {
			defer a.wg.Done()
			ticker := time.NewTicker(a.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-a.stop:
					return
				case <-ticker.C:
					a.ProbeOnce(inst)
				}
			}
		}(inst)
	}
}

// Stop terminates all probe loops and waits for them to finish.
func (a *Active) Stop() {
	select {
	case <-a.stop:
		return
	default:
		close(a.stop)
	}
	a.wg.Wait()
}

// ProbeOnce performs a single health probe against inst.
func (a *Active) ProbeOnce(inst *lb.Instance) {
	target := *inst.URL
	target.Path = a.Path
	target.RawQuery = ""

	resp, err := a.Client.Get(target.String())
	if err != nil {
		a.recordFailure(inst, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		a.recordSuccess(inst)
		return
	}
	a.recordFailure(inst, nil)
}

func (a *Active) recordSuccess(inst *lb.Instance) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[inst] = 0
	a.successes[inst]++
	if a.successes[inst] >= a.HealthyThreshold && !inst.Healthy() {
		inst.MarkHealthy()
		a.Logger.Info("upstream marked healthy", "upstream", inst.String())
	}
}

func (a *Active) recordFailure(inst *lb.Instance, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.successes[inst] = 0
	a.failures[inst]++
	if a.failures[inst] >= a.UnhealthyThreshold && inst.Healthy() {
		inst.MarkUnhealthy()
		attrs := []any{"upstream", inst.String()}
		if err != nil {
			attrs = append(attrs, "error", err.Error())
		}
		a.Logger.Warn("upstream marked unhealthy", attrs...)
	}
}

// Passive implements lightweight outlier ejection: an instance is removed
// from rotation for a cooldown window after enough consecutive failures.
// Once the cooldown elapses the instance gets a half-open probe; a success
// restores it, a failure immediately re-ejects it.
type Passive struct {
	Threshold int
	Cooldown  time.Duration

	mu           sync.Mutex
	failures     map[*lb.Instance]int
	ejectedUntil map[*lb.Instance]time.Time
}

// NewPassive builds a Passive ejector with fallbacks for zero values.
func NewPassive(threshold int, cooldown time.Duration) *Passive {
	if threshold <= 0 {
		threshold = 3
	}
	if cooldown <= 0 {
		cooldown = 5 * time.Second
	}
	return &Passive{
		Threshold:    threshold,
		Cooldown:     cooldown,
		failures:     map[*lb.Instance]int{},
		ejectedUntil: map[*lb.Instance]time.Time{},
	}
}

// Allow reports whether inst may receive traffic at time now.
func (p *Passive) Allow(inst *lb.Instance, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.ejectedUntil[inst]
	if !ok {
		return true
	}
	return !now.Before(until)
}

// Report records the outcome of a request to inst.
func (p *Passive) Report(inst *lb.Instance, err error, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.failures[inst] = 0
		delete(p.ejectedUntil, inst)
		return
	}
	p.failures[inst]++
	if p.failures[inst] >= p.Threshold {
		p.ejectedUntil[inst] = now.Add(p.Cooldown)
	}
}

// Failures returns the current consecutive failure count for inst.
func (p *Passive) Failures(inst *lb.Instance) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failures[inst]
}

// Ejected reports whether inst is currently ejected at time now.
func (p *Passive) Ejected(inst *lb.Instance, now time.Time) bool {
	return !p.Allow(inst, now)
}
