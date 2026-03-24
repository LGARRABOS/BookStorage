package server

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets standard security-related HTTP headers (and optional CSP / HSTS).
func (a *App) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Phase 1 CSP: inline scripts remain allowed until extracted to static files.
		csp := "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)
		if a.Settings.EnableHSTS {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func sessionSameSite(env string) http.SameSite {
	if strings.ToLower(strings.TrimSpace(env)) == "production" {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}
