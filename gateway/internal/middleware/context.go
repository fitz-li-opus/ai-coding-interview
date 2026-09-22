package middleware

import (
	"context"
	"net"
	"net/http"
)

type stateKey struct{}

type RequestState struct {
	RequestID string
	Route     string
	Upstream  string
}

func withState(request *http.Request, state *RequestState) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), stateKey{}, state))
}

func State(request *http.Request) *RequestState {
	state, _ := request.Context().Value(stateKey{}).(*RequestState)
	return state
}

func ClientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}
