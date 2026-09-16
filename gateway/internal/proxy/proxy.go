package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"syscall"
	"time"

	"gateway/internal/config"
	"gateway/internal/health"
	"gateway/internal/httpx"
	"gateway/internal/lb"
	"gateway/internal/metrics"
)

const maxBodyBytes = 1 << 20

// errRetriable is an internal sentinel returned by ModifyResponse to ask the
// outer retry loop to try the next upstream.
var errRetriable = errors.New("retriable upstream status")

type stateKeyType int

const stateKey stateKeyType = iota

type attemptState struct {
	inst           *lb.Instance
	attempt        int
	max            int
	err            error
	finalRetriable bool
}

func stateFrom(ctx context.Context) *attemptState {
	if v, ok := ctx.Value(stateKey).(*attemptState); ok {
		return v
	}
	return nil
}

// RouteHandler proxies requests for a single route to one of its upstreams,
// applying load balancing, passive ejection and retries.
type RouteHandler struct {
	route     *config.Route
	instances []*lb.Instance
	balancer  lb.Balancer
	passive   *health.Passive
	retry     config.Retry
	timeout   config.Timeout
	metrics   *metrics.Metrics
	logger    *slog.Logger
	proxy     *httputil.ReverseProxy
}

// New builds a RouteHandler.
func New(
	route *config.Route,
	instances []*lb.Instance,
	balancer lb.Balancer,
	passive *health.Passive,
	timeout config.Timeout,
	retry config.Retry,
	m *metrics.Metrics,
	logger *slog.Logger,
) *RouteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &RouteHandler{
		route:     route,
		instances: instances,
		balancer:  balancer,
		passive:   passive,
		retry:     retry,
		timeout:   timeout,
		metrics:   m,
		logger:    logger,
	}
	h.proxy = h.buildProxy()
	return h
}

func (h *RouteHandler) buildProxy() *httputil.ReverseProxy {
	dialer := &net.Dialer{
		Timeout:   h.timeout.Connect.D(),
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       h.timeout.Idle.D(),
		ResponseHeaderTimeout: h.timeout.ResponseHeader.D(),
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			st := stateFrom(pr.In.Context())
			if st == nil || st.inst == nil {
				return
			}
			pr.SetURL(st.inst.URL)
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if st := stateFrom(r.Context()); st != nil {
				st.err = err
			}
		},
		ModifyResponse: h.modifyResponse,
	}
}

func (h *RouteHandler) modifyResponse(resp *http.Response) error {
	st := stateFrom(resp.Request.Context())
	if st == nil {
		return nil
	}
	if retriableStatus(resp.StatusCode, h.retry.Statuses) {
		if st.attempt < st.max {
			return errRetriable
		}
		st.finalRetriable = true
	}
	return nil
}

func retriableStatus(code int, statuses []int) bool {
	for _, s := range statuses {
		if code == s {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler.
func (h *RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	restoreBody(r, body)

	var lastErr error
	tried := map[*lb.Instance]bool{}

	for attempt := 0; attempt <= h.retry.Max; attempt++ {
		now := time.Now()
		inst := h.pick(tried, now)
		if inst == nil {
			break
		}
		tried[inst] = true

		st := &attemptState{inst: inst, attempt: attempt, max: h.retry.Max}
		ctx := context.WithValue(r.Context(), stateKey, st)
		var cancel context.CancelFunc
		if h.timeout.ResponseHeader.D() > 0 {
			ctx, cancel = context.WithTimeout(ctx, h.timeout.ResponseHeader.D())
		}

		req := r.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		if len(body) == 0 {
			req.Body = http.NoBody
		}

		h.proxy.ServeHTTP(w, req)
		if cancel != nil {
			cancel()
		}

		if st.err == nil {
			if st.finalRetriable {
				h.passive.Report(inst, errRetriable, time.Now())
			} else {
				h.passive.Report(inst, nil, time.Now())
			}
			h.metrics.SetUpstreamHealth(h.route.ID, inst.String(), inst.Healthy())
			return
		}

		lastErr = st.err
		h.passive.Report(inst, st.err, time.Now())
		h.metrics.SetUpstreamHealth(h.route.ID, inst.String(), false)
		h.logger.Warn("upstream attempt failed",
			"route", h.route.ID,
			"upstream", inst.String(),
			"attempt", attempt,
			"error", st.err.Error(),
		)

		if attempt == h.retry.Max || !h.isRetriable(st.err) {
			break
		}
		h.metrics.IncRetry(h.route.ID)
		h.sleepBackoff(attempt)
	}

	h.writeError(w, r, lastErr)
}

func (h *RouteHandler) pick(tried map[*lb.Instance]bool, now time.Time) *lb.Instance {
	candidates := make([]*lb.Instance, 0, len(h.instances))
	for _, inst := range h.instances {
		if tried[inst] {
			continue
		}
		if !inst.Healthy() {
			continue
		}
		if !h.passive.Allow(inst, now) {
			continue
		}
		candidates = append(candidates, inst)
	}
	return h.balancer.Pick(candidates)
}

func (h *RouteHandler) isRetriable(err error) bool {
	if errors.Is(err, errRetriable) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

func (h *RouteHandler) sleepBackoff(attempt int) {
	d := h.retry.Backoff.D()
	if d <= 0 {
		return
	}
	d *= time.Duration(attempt + 1)
	if h.retry.Jitter {
		d += time.Duration(rand.Int63n(int64(d/2) + 1))
	}
	time.Sleep(d)
}

func (h *RouteHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		httpx.WriteError(w, http.StatusServiceUnavailable, "no healthy upstream available")
	case errors.Is(err, errRetriable):
		httpx.WriteError(w, http.StatusBadGateway, "upstream returned a retriable status")
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		httpx.WriteError(w, http.StatusGatewayTimeout, "upstream timed out")
	default:
		httpx.WriteError(w, http.StatusBadGateway, "upstream request failed")
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
}

func restoreBody(r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}
