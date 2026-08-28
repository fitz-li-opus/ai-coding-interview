package main

import (
	"net/http/httputil"
	"net/url"
	"sync/atomic"
)

// Upstream is one backend instance behind a route.
type Upstream struct {
	URL     *url.URL
	proxy   *httputil.ReverseProxy
	healthy atomic.Bool
}

func (u *Upstream) Healthy() bool { return u.healthy.Load() }

// setHealthy stores the new state and reports whether it actually changed,
// so the health checker only logs on transitions instead of every tick.
func (u *Upstream) setHealthy(v bool) bool { return u.healthy.Swap(v) != v }

// Route maps one path prefix onto a pool of interchangeable upstreams.
type Route struct {
	Prefix      string
	StripPrefix bool
	Upstreams   []*Upstream

	next atomic.Uint64 // round-robin cursor
}

// Pick returns the next healthy upstream in round-robin order, or nil when the
// whole pool is down. Advancing the cursor once per call keeps the distribution
// even; the scan then skips dead instances.
func (rt *Route) Pick() *Upstream {
	n := uint64(len(rt.Upstreams))
	if n == 0 {
		return nil
	}
	start := rt.next.Add(1) - 1
	for i := uint64(0); i < n; i++ {
		if u := rt.Upstreams[(start+i)%n]; u.Healthy() {
			return u
		}
	}
	return nil
}
