package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestDurationUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "string", in: `"2s"`, want: 2 * time.Second},
		{name: "number", in: `1.5`, want: 1500 * time.Millisecond},
		{name: "null", in: `null`, want: 0},
		{name: "empty quoted", in: `""`, want: 0},
		{name: "invalid string", in: `"abc"`, wantErr: true},
		{name: "invalid type", in: `{}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var d Duration
			err := json.Unmarshal([]byte(tc.in), &d)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.D() != tc.want {
				t.Fatalf("got %v want %v", d.D(), tc.want)
			}
		})
	}
}

func TestDurationMarshal(t *testing.T) {
	b, err := json.Marshal(Duration(3 * time.Second))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `"3s"` {
		t.Fatalf("got %s want %q", b, "3s")
	}
}

func TestLoadAppliesDefaultsAndEnv(t *testing.T) {
	path := writeFile(t, `{
		"routes": [
			{"id": "user-svc", "match": {"prefix": "/u"}, "upstreams": [{"url": "http://a:1"}]}
		]
	}`)
	t.Setenv("GATEWAY_LISTEN", ":9999")
	t.Setenv("GATEWAY_UPSTREAM_USER_SVC", "http://x:1, http://y:2, ")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":9999" {
		t.Fatalf("listen got %q", cfg.Listen)
	}
	r := cfg.Routes[0]
	if r.LB != "round_robin" {
		t.Fatalf("lb got %q", r.LB)
	}
	if len(r.Upstreams) != 2 {
		t.Fatalf("upstreams got %d", len(r.Upstreams))
	}
	if cfg.Defaults.Timeout.ResponseHeader.D() != 10*time.Second {
		t.Fatalf("default response header timeout wrong: %v", cfg.Defaults.Timeout.ResponseHeader.D())
	}
}

func TestLoadDefaultsWithoutEnv(t *testing.T) {
	path := writeFile(t, `{
		"defaults": {"timeout": {"connect": "1s"}},
		"routes": [
			{"id": "users", "match": {"prefix": "/u"}, "upstreams": [{"url": "http://a:1", "weight": 0}]}
		]
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Fatalf("default listen got %q", cfg.Listen)
	}
	if cfg.Routes[0].Upstreams[0].Weight != 1 {
		t.Fatalf("default weight got %d", cfg.Routes[0].Upstreams[0].Weight)
	}
	if cfg.Defaults.Timeout.Connect.D() != time.Second {
		t.Fatalf("explicit connect not honoured")
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantSub string
	}{
		{
			name:    "bad json",
			content: `{`,
			wantSub: "parse config",
		},
		{
			name:    "no routes",
			content: `{"listen": ":1", "routes": []}`,
			wantSub: "no routes defined",
		},
		{
			name:    "empty id",
			content: `{"routes":[{"id":"","match":{"prefix":"/"},"upstreams":[{"url":"http://a"}]}]}`,
			wantSub: "empty id",
		},
		{
			name:    "duplicate id",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"upstreams":[{"url":"http://a"}]},{"id":"a","match":{"prefix":"/b"},"upstreams":[{"url":"http://b"}]}]}`,
			wantSub: "duplicate route id",
		},
		{
			name:    "empty prefix",
			content: `{"routes":[{"id":"a","match":{"prefix":""},"upstreams":[{"url":"http://a"}]}]}`,
			wantSub: "empty prefix",
		},
		{
			name:    "prefix no slash",
			content: `{"routes":[{"id":"a","match":{"prefix":"api"},"upstreams":[{"url":"http://a"}]}]}`,
			wantSub: "must start with /",
		},
		{
			name:    "no upstreams",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"upstreams":[]}]}`,
			wantSub: "no upstreams",
		},
		{
			name:    "invalid url",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"upstreams":[{"url":"http://"}]}]}`,
			wantSub: "invalid url",
		},
		{
			name:    "unsupported scheme",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"upstreams":[{"url":"ftp://host"}]}]}`,
			wantSub: "unsupported scheme",
		},
		{
			name:    "negative weight",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"upstreams":[{"url":"http://a","weight":-1}]}]}`,
			wantSub: "negative weight",
		},
		{
			name:    "unknown lb",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a"},"lb":"magic","upstreams":[{"url":"http://a"}]}]}`,
			wantSub: "unknown lb",
		},
		{
			name:    "empty method",
			content: `{"routes":[{"id":"a","match":{"prefix":"/a","methods":[""]},"upstreams":[{"url":"http://a"}]}]}`,
			wantSub: "empty method",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeFile(t, tc.content))
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateEmptyListen(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "listen address is empty") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateNoRoutes(t *testing.T) {
	c := &Config{Listen: ":1"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "no routes defined") {
		t.Fatalf("got %v", err)
	}
}

func TestApplyDefaults(t *testing.T) {
	c := &Config{Routes: []Route{{ID: "a", Upstreams: []Upstream{{URL: "http://a", Weight: 0}}}}}
	applyDefaults(c)
	if c.Listen != ":8080" {
		t.Fatalf("listen got %q", c.Listen)
	}
	if c.Routes[0].LB != "round_robin" {
		t.Fatalf("lb got %q", c.Routes[0].LB)
	}
	if c.Routes[0].Upstreams[0].Weight != 1 {
		t.Fatalf("weight got %d", c.Routes[0].Upstreams[0].Weight)
	}
}

func TestApplyEnvSkipsEmpty(t *testing.T) {
	c := &Config{Routes: []Route{{ID: "a", Upstreams: []Upstream{{URL: "http://a", Weight: 1}}}}}
	t.Setenv("GATEWAY_UPSTREAM_A", "")
	applyEnv(c)
	if len(c.Routes[0].Upstreams) != 1 || c.Routes[0].Upstreams[0].URL != "http://a" {
		t.Fatalf("upstreams changed: %+v", c.Routes[0].Upstreams)
	}
}

func TestEnvKey(t *testing.T) {
	if got := envKey("user-svc.1"); got != "USER_SVC_1" {
		t.Fatalf("envKey got %q", got)
	}
}

func TestMergeHelpers(t *testing.T) {
	base := defaultConfig().Defaults

	var to Timeout
	mergeTimeout(&to, base.Timeout)
	if to != base.Timeout {
		t.Fatalf("mergeTimeout got %+v", to)
	}
	to2 := Timeout{Connect: Duration(time.Second)}
	mergeTimeout(&to2, base.Timeout)
	if to2.Connect.D() != time.Second || to2.ResponseHeader != base.Timeout.ResponseHeader {
		t.Fatalf("mergeTimeout partial got %+v", to2)
	}

	var rt Retry
	mergeRetry(&rt, base.Retry)
	if rt.Backoff != base.Retry.Backoff || len(rt.Statuses) != len(base.Retry.Statuses) {
		t.Fatalf("mergeRetry got %+v", rt)
	}
	rt2 := Retry{Backoff: Duration(time.Second), Statuses: []int{500}}
	mergeRetry(&rt2, base.Retry)
	if rt2.Backoff.D() != time.Second || len(rt2.Statuses) != 1 {
		t.Fatalf("mergeRetry override got %+v", rt2)
	}

	var h Health
	mergeHealth(&h, base.Health)
	if h != base.Health {
		t.Fatalf("mergeHealth got %+v", h)
	}
	h2 := Health{Path: "/ping"}
	mergeHealth(&h2, base.Health)
	if h2.Path != "/ping" || h2.Interval != base.Health.Interval {
		t.Fatalf("mergeHealth partial got %+v", h2)
	}
}

func TestAccessors(t *testing.T) {
	c := defaultConfig()

	if got := c.TimeoutFor(&Route{}); got != c.Defaults.Timeout {
		t.Fatalf("TimeoutFor nil got %+v", got)
	}
	rt := c.TimeoutFor(&Route{Timeout: &Timeout{Connect: Duration(time.Second)}})
	if rt.Connect.D() != time.Second || rt.ResponseHeader != c.Defaults.Timeout.ResponseHeader {
		t.Fatalf("TimeoutFor partial got %+v", rt)
	}

	if got := c.RetryFor(&Route{}); got.Backoff != c.Defaults.Retry.Backoff {
		t.Fatalf("RetryFor nil got %+v", got)
	}
	rr := c.RetryFor(&Route{Retry: &Retry{Max: 7}})
	if rr.Max != 7 || rr.Backoff != c.Defaults.Retry.Backoff {
		t.Fatalf("RetryFor partial got %+v", rr)
	}

	if got := c.HealthFor(&Route{}); got != c.Defaults.Health {
		t.Fatalf("HealthFor nil got %+v", got)
	}
	hh := c.HealthFor(&Route{Health: &Health{Path: "/ping"}})
	if hh.Path != "/ping" || hh.Interval != c.Defaults.Health.Interval {
		t.Fatalf("HealthFor partial got %+v", hh)
	}
}
