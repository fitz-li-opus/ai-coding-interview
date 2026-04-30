package repository

import "net/http"

type Backend interface {
	Name() string
	http.Handler
}
