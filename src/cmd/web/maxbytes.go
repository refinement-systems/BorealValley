package main

import "net/http"

// MaxBytesBody wraps the request body of unsafe-method requests with
// http.MaxBytesReader so that no handler can be forced to consume an
// unbounded amount of memory.
func MaxBytesBody(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
