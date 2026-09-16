package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDContext(t *testing.T) {
	if got := RequestIDFrom(context.Background()); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	ctx := WithRequestID(context.Background(), "abc")
	if got := RequestIDFrom(ctx); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestRouteInfoContext(t *testing.T) {
	if got := RouteInfoFrom(context.Background()); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	ctx, info := WithRouteInfo(context.Background())
	if RouteInfoFrom(ctx) != info {
		t.Fatal("route info not stored")
	}
	info.ID = "users"
	if RouteInfoFrom(ctx).ID != "users" {
		t.Fatal("holder not mutable")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"a": "b"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("code got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type got %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["a"] != "b" {
		t.Fatalf("body got %s err=%v", rec.Body.String(), err)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusNotFound, "nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code got %d", rec.Code)
	}
	var body Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Status != http.StatusNotFound || body.Error != "nope" {
		t.Fatalf("body got %+v", body)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{name: "xff first", xff: "1.2.3.4, 5.6.7.8", remoteAddr: "9.9.9.9:1234", want: "1.2.3.4"},
		{name: "xff blank first", xff: ", 5.6.7.8", remoteAddr: "9.9.9.9:1234", want: "9.9.9.9"},
		{name: "remote with port", remoteAddr: "9.9.9.9:1234", want: "9.9.9.9"},
		{name: "remote without port", remoteAddr: "9.9.9.9", want: "9.9.9.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := ClientIP(req); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

type bareWriter struct {
	header http.Header
	status int
}

func (b *bareWriter) Header() http.Header         { return b.header }
func (b *bareWriter) Write(p []byte) (int, error) { return len(p), nil }
func (b *bareWriter) WriteHeader(status int)      { b.status = status }

func TestStatusRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := NewStatusRecorder(rec)
	if sr.Status != http.StatusOK {
		t.Fatalf("default status got %d", sr.Status)
	}
	sr.WriteHeader(http.StatusTeapot)
	sr.WriteHeader(http.StatusBadGateway)
	if sr.Status != http.StatusTeapot {
		t.Fatalf("first status should win, got %d", sr.Status)
	}
	if _, err := sr.Write([]byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sr.Flush()

	rec2 := httptest.NewRecorder()
	sr2 := NewStatusRecorder(rec2)
	if _, err := sr2.Write([]byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sr2.WriteHeader(http.StatusBadGateway)
	if sr2.Status != http.StatusOK || !sr2.Wrote {
		t.Fatalf("after write status=%d wrote=%v", sr2.Status, sr2.Wrote)
	}

	bare := &bareWriter{header: http.Header{}}
	sr3 := NewStatusRecorder(bare)
	sr3.Flush()
	sr3.WriteHeader(http.StatusAccepted)
	if sr3.Status != http.StatusAccepted {
		t.Fatalf("status got %d", sr3.Status)
	}
}
