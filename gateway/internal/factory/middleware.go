package factory

import "gateway/internal/middleware"

func NewMiddleware() *middleware.Middlewares {
	return &middleware.Middlewares{
		Logger: middleware.Logger(),
	}
}
