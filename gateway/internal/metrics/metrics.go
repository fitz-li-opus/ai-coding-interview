package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type requestKey struct {
	Route  string
	Method string
	Code   int
}

// Metrics is a small, dependency-free metrics registry that renders the
// Prometheus text exposition format.
type Metrics struct {
	mu sync.Mutex

	requests    map[requestKey]uint64
	durationSum map[string]float64
	durationCnt map[string]uint64
	retries     map[string]uint64
	upstreamUp  map[string]map[string]bool
}

// New returns an empty registry.
func New() *Metrics {
	return &Metrics{
		requests:    map[requestKey]uint64{},
		durationSum: map[string]float64{},
		durationCnt: map[string]uint64{},
		retries:     map[string]uint64{},
		upstreamUp:  map[string]map[string]bool{},
	}
}

// ObserveRequest records one proxied request.
func (m *Metrics) ObserveRequest(route, method string, code int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey{Route: route, Method: method, Code: code}]++
	m.durationSum[route] += d.Seconds()
	m.durationCnt[route]++
}

// IncRetry records a retry attempt for a route.
func (m *Metrics) IncRetry(route string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retries[route]++
}

// SetUpstreamHealth records the health gauge for a route/upstream pair.
func (m *Metrics) SetUpstreamHealth(route, upstream string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upstreamUp[route] == nil {
		m.upstreamUp[route] = map[string]bool{}
	}
	m.upstreamUp[route][upstream] = healthy
}

func quote(s string) string { return strconv.Quote(s) }

// Render writes the registry in Prometheus text format.
func (m *Metrics) Render(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Fprintln(w, "# HELP gateway_requests_total Total requests proxied.")
	fmt.Fprintln(w, "# TYPE gateway_requests_total counter")
	keys := make([]requestKey, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Route != keys[j].Route {
			return keys[i].Route < keys[j].Route
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Code < keys[j].Code
	})
	for _, k := range keys {
		fmt.Fprintf(w, "gateway_requests_total{route=%s,method=%s,code=%d} %d\n",
			quote(k.Route), quote(k.Method), k.Code, m.requests[k])
	}

	fmt.Fprintln(w, "# HELP gateway_request_duration_seconds_sum Total time spent proxying.")
	fmt.Fprintln(w, "# TYPE gateway_request_duration_seconds_sum counter")
	routes := make([]string, 0, len(m.durationSum))
	for r := range m.durationSum {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	for _, r := range routes {
		fmt.Fprintf(w, "gateway_request_duration_seconds_sum{route=%s} %g\n", quote(r), m.durationSum[r])
		fmt.Fprintf(w, "gateway_request_duration_seconds_count{route=%s} %d\n", quote(r), m.durationCnt[r])
	}

	fmt.Fprintln(w, "# HELP gateway_upstream_retries_total Retry attempts per route.")
	fmt.Fprintln(w, "# TYPE gateway_upstream_retries_total counter")
	retryRoutes := make([]string, 0, len(m.retries))
	for r := range m.retries {
		retryRoutes = append(retryRoutes, r)
	}
	sort.Strings(retryRoutes)
	for _, r := range retryRoutes {
		fmt.Fprintf(w, "gateway_upstream_retries_total{route=%s} %d\n", quote(r), m.retries[r])
	}

	fmt.Fprintln(w, "# HELP gateway_upstream_healthy Whether an upstream is in rotation.")
	fmt.Fprintln(w, "# TYPE gateway_upstream_healthy gauge")
	for _, route := range sortedKeys(m.upstreamUp) {
		insts := make([]string, 0, len(m.upstreamUp[route]))
		for inst := range m.upstreamUp[route] {
			insts = append(insts, inst)
		}
		sort.Strings(insts)
		for _, inst := range insts {
			v := 0
			if m.upstreamUp[route][inst] {
				v = 1
			}
			fmt.Fprintf(w, "gateway_upstream_healthy{route=%s,upstream=%s} %d\n", quote(route), quote(inst), v)
		}
	}
}

func sortedKeys(m map[string]map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// String renders the registry as a string.
func (m *Metrics) String() string {
	var b strings.Builder
	m.Render(&b)
	return b.String()
}
