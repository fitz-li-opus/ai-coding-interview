package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDAcceptsValidValueAndReplacesInvalidValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid", input: "request-1.test_value", want: "request-1.test_value"},
		{name: "invalid character", input: "invalid request", want: "generated"},
		{name: "too long", input: strings.Repeat("a", 129), want: "generated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stateRequestID string
			handler := RequestID(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				stateRequestID = State(request).RequestID
				writer.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "https://localhost/", nil)
			request.Header.Set("X-Request-ID", test.input)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if test.want == "generated" {
				if stateRequestID == test.input || len(stateRequestID) != 32 {
					t.Fatalf("generated request id = %q", stateRequestID)
				}
			} else if stateRequestID != test.want {
				t.Fatalf("request id = %q, want %q", stateRequestID, test.want)
			}
			if response.Header().Get("X-Request-ID") != stateRequestID {
				t.Fatalf("response request id = %q", response.Header().Get("X-Request-ID"))
			}
		})
	}
}
