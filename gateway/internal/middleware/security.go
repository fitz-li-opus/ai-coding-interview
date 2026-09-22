package middleware

import (
	"bytes"
	"io"
	"net/http"
)

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func BodyLimit(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ContentLength > maxBytes {
			WriteError(writer, request, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit")
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
		if err != nil {
			WriteError(writer, request, http.StatusBadRequest, "request_read_error", "request body could not be read")
			return
		}
		if err := request.Body.Close(); err != nil {
			panic(err)
		}
		if int64(len(body)) > maxBytes {
			WriteError(writer, request, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit")
			return
		}
		request.ContentLength = int64(len(body))
		if len(body) == 0 {
			request.Body = http.NoBody
			request.GetBody = func() (io.ReadCloser, error) {
				return http.NoBody, nil
			}
		} else {
			request.Body = io.NopCloser(bytes.NewReader(body))
			request.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
		}
		next.ServeHTTP(writer, request)
	})
}
