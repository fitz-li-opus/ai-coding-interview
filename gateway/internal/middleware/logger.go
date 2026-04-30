package middleware

import (
	"log"
	"net/http"
	"time"

	"crypto/rand"
	"encoding/hex"
)

const requestIDHeader = "X-Request-Id"

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Logger() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get(requestIDHeader)
			if reqID == "" {
				reqID = newRequestID()
				r.Header.Set(requestIDHeader, reqID)
			}
			w.Header().Set(requestIDHeader, reqID)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(rec, r)

			log.Printf("req_id=%s method=%s path=%s status=%d dur=%s remote=%s",
				reqID, r.Method, r.URL.Path, rec.status, time.Since(start), r.RemoteAddr)
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
