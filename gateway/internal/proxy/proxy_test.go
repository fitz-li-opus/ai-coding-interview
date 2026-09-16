package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/health"
	"gateway/internal/lb"
	"gateway/internal/metrics"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func timeout() config.Timeout {
	return config.Timeout{
		Connect:        config.Duration(time.Second),
		ResponseHeader: config.Duration(time.Second),
		Idle:           config.Duration(time.Second),
	}
}

func mustInstance(t *testing.T, raw string) *lb.Instance {
	t.Helper()
	inst, err := lb.NewInstance(raw, 1)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	return inst
}

func newHandler(t *testing.T, instances []*lb.Instance, passive *health.Passive, rt config.Retry, to config.Timeout) *RouteHandler {
	t.Helper()
	if passive == nil {
		passive = health.NewPassive(100, time.Minute)
	}
	return New(
		&config.Route{ID: "test"},
		instances,
		&lb.RoundRobin{},
		passive,
		to,
		rt,
		metrics.New(),
		testLogger(),
	)
}

func TestServeHTTPSuccess(t *testing.T) {
	var gotXFF string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()

	h := newHandler(t, []*lb.Instance{mustInstance(t, up.URL)}, nil, config.Retry{}, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/users", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code got %d", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body got %q", rec.Body.String())
	}
	if gotXFF == "" {
		t.Fatal("X-Forwarded-For not set")
	}
}

func TestServeHTTPRetriesOnStatus(t *testing.T) {
	badHits := 0
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badHits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()

	rt := config.Retry{Max: 1, Statuses: []int{502}}
	h := newHandler(t, []*lb.Instance{mustInstance(t, bad.URL), mustInstance(t, good.URL)}, nil, rt, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code got %d", rec.Code)
	}
	if badHits != 1 {
		t.Fatalf("bad hits got %d", badHits)
	}
}

func TestServeHTTPRetriesOnConnectionError(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()
	goodHits := 0
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goodHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()

	rt := config.Retry{Max: 1, Statuses: []int{502}}
	h := newHandler(t, []*lb.Instance{mustInstance(t, dead.URL), mustInstance(t, good.URL)}, nil, rt, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK || goodHits != 1 {
		t.Fatalf("code=%d goodHits=%d", rec.Code, goodHits)
	}
}

func TestServeHTTPRetriableStatusExhausted(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	rt := config.Retry{Max: 1, Statuses: []int{502}}
	h := newHandler(t, []*lb.Instance{mustInstance(t, bad.URL)}, nil, rt, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "retriable status") {
		t.Fatalf("body got %s", rec.Body.String())
	}
}

func TestServeHTTPFinalRetriablePassesThrough(t *testing.T) {
	bad1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad1.Close()
	bad2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream-two")
	}))
	defer bad2.Close()

	rt := config.Retry{Max: 1, Statuses: []int{502}}
	h := newHandler(t, []*lb.Instance{mustInstance(t, bad1.URL), mustInstance(t, bad2.URL)}, nil, rt, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "upstream-two") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeHTTPNoHealthyUpstream(t *testing.T) {
	inst := mustInstance(t, "http://127.0.0.1:1")
	inst.MarkUnhealthy()
	h := newHandler(t, []*lb.Instance{inst}, nil, config.Retry{}, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no healthy upstream") {
		t.Fatalf("body got %s", rec.Body.String())
	}
}

func TestServeHTTPNoInstances(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{}, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code got %d", rec.Code)
	}
}

func TestServeHTTPPassiveEjection(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	inst := mustInstance(t, up.URL)
	passive := health.NewPassive(1, time.Minute)
	passive.Report(inst, errors.New("boom"), time.Now())

	h := newHandler(t, []*lb.Instance{inst}, passive, config.Retry{}, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code got %d", rec.Code)
	}
}

func TestServeHTTPTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	to := timeout()
	to.ResponseHeader = config.Duration(30 * time.Millisecond)
	h := newHandler(t, []*lb.Instance{mustInstance(t, up.URL)}, nil, config.Retry{}, to)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeHTTPZeroResponseTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	to := timeout()
	to.ResponseHeader = 0
	h := newHandler(t, []*lb.Instance{mustInstance(t, up.URL)}, nil, config.Retry{}, to)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code got %d", rec.Code)
	}
}

func TestServeHTTPNonRetriableError(t *testing.T) {
	inst := mustInstance(t, "ftp://127.0.0.1:1")
	rt := config.Retry{Max: 1, Statuses: []int{502}}
	h := newHandler(t, []*lb.Instance{inst}, nil, rt, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "upstream request failed") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeHTTPForwardsBody(t *testing.T) {
	var got string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	h := newHandler(t, []*lb.Instance{mustInstance(t, up.URL)}, nil, config.Retry{}, timeout())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", strings.NewReader("payload")))
	if got != "payload" {
		t.Fatalf("upstream body got %q", got)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestServeHTTPBodyReadError(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{}, timeout())
	req := httptest.NewRequest("POST", "/", nil)
	req.Body = io.NopCloser(errReader{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code got %d", rec.Code)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestIsRetriable(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{}, timeout())
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "sentinel", err: errRetriable, want: true},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "canceled", err: context.Canceled, want: true},
		{name: "eof", err: io.EOF, want: true},
		{name: "unexpected eof", err: io.ErrUnexpectedEOF, want: true},
		{name: "conn refused", err: syscall.ECONNREFUSED, want: true},
		{name: "reset wrapped", err: fmt.Errorf("wrapped: %w", syscall.ECONNRESET), want: true},
		{name: "pipe", err: syscall.EPIPE, want: true},
		{name: "net timeout", err: timeoutErr{}, want: true},
		{name: "generic", err: errors.New("x"), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.isRetriable(tc.err); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{}, timeout())
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: http.StatusServiceUnavailable},
		{name: "retriable", err: errRetriable, want: http.StatusBadGateway},
		{name: "deadline", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "timeout", err: timeoutErr{}, want: http.StatusGatewayTimeout},
		{name: "generic", err: errors.New("x"), want: http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.writeError(rec, httptest.NewRequest("GET", "/", nil), tc.err)
			if rec.Code != tc.want {
				t.Fatalf("code got %d want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestPickSkipsTriedAndUnavailable(t *testing.T) {
	a := mustInstance(t, "http://a")
	b := mustInstance(t, "http://b")
	c := mustInstance(t, "http://c")
	c.MarkUnhealthy()

	h := newHandler(t, []*lb.Instance{a, b, c}, nil, config.Retry{}, timeout())
	if got := h.pick(map[*lb.Instance]bool{a: true}, time.Now()); got != b {
		t.Fatalf("got %v want b", got)
	}
	if got := h.pick(map[*lb.Instance]bool{a: true, b: true}, time.Now()); got != nil {
		t.Fatalf("got %v want nil", got)
	}
}

func TestSleepBackoff(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{Backoff: 0}, timeout())
	h.sleepBackoff(0)

	h2 := newHandler(t, nil, nil, config.Retry{Backoff: config.Duration(time.Millisecond)}, timeout())
	h2.sleepBackoff(0)

	h3 := newHandler(t, nil, nil, config.Retry{Backoff: config.Duration(time.Millisecond), Jitter: true}, timeout())
	h3.sleepBackoff(0)
}

func TestModifyResponseWithoutState(t *testing.T) {
	h := newHandler(t, nil, nil, config.Retry{}, timeout())
	resp := &http.Response{StatusCode: http.StatusOK, Request: httptest.NewRequest("GET", "/", nil)}
	if err := h.modifyResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRetriableStatus(t *testing.T) {
	if !retriableStatus(502, []int{500, 502}) {
		t.Fatal("expected retriable")
	}
	if retriableStatus(200, []int{502}) {
		t.Fatal("expected non-retriable")
	}
	if retriableStatus(502, nil) {
		t.Fatal("expected non-retriable with empty list")
	}
}

func TestStateFromMissing(t *testing.T) {
	if stateFrom(context.Background()) != nil {
		t.Fatal("expected nil state")
	}
}
