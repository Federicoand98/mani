package server

import (
	"crypto/subtle"
	"net/http"
)

func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	want := []byte("Bearer " + token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
