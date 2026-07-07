package api

import (
	"net/http"
	"slices"
)

// withCORS answers cross-origin requests from the listed web origins: the
// static site is served from a different origin than the API. An empty list
// disables CORS entirely (same-origin deployments need none). Credentials are
// never allowed; bindings are protected by their signature, not by origin.
func withCORS(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		return next
	}
	wildcard := slices.Contains(allowed, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		switch {
		case wildcard:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case origin != "" && slices.Contains(allowed, origin):
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		// Preflight: answer before the method-scoped mux would 405 it.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
