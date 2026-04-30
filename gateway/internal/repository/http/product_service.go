package http

import "time"

func NewProductServiceClient(target string, timeout time.Duration) (*HTTPBackend, error) {
	return NewHTTPBackend("product-service", target, timeout)
}
