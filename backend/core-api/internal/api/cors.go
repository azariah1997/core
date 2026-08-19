package api

import "net/http"

// corsMiddleware exists so browser-based callers - the control room's
// API console (docs/control), and eventually apps/admin - can call this
// API directly from a different origin. Every route in this repo
// authenticates via a Bearer token (Authorization header), never
// cookies, so reflecting the request Origin back is safe from CSRF the
// way it would not be for a cookie-authenticated API: a page on another
// origin can't make the browser attach a token it doesn't already have.
// A production deployment fronting this with a fixed set of known admin/
// product origins would still want to replace the reflected origin
// below with an explicit allowlist - not needed for this platform's
// local-dev-and-single-admin-portal scope today.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
