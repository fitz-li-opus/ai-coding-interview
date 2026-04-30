package http

import "time"

func NewUserServiceClient(target string, timeout time.Duration) (*HTTPBackend, error) {
	return NewHTTPBackend("user-service", target, timeout)
}
