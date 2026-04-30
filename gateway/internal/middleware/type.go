package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

type Middlewares struct {
	Logger Middleware
}
