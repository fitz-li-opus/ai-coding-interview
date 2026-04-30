package http

import "time"

func NewOrderServiceClient(target string, timeout time.Duration) (*HTTPBackend, error) {
	return NewHTTPBackend("order-service", target, timeout)
}
