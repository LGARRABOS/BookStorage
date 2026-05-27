package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type cspNonceKey struct{}

func newCSPNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func cspNonceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	nonce, _ := ctx.Value(cspNonceKey{}).(string)
	return nonce
}

// SecurityHeaders sets standard security-related HTTP headers (and optional CSP / HSTS).
func (a *App) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		nonce, err := newCSPNonce()
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		ctx := context.WithValue(r.Context(), cspNonceKey{}, nonce)
		scriptSrc := fmt.Sprintf("'self' 'nonce-%s' https://cdn.jsdelivr.net", nonce)
		csp := "default-src 'self'; script-src " + scriptSrc + "; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)
		if a.Settings.EnableHSTS {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sessionSameSite(env string) http.SameSite {
	if strings.ToLower(strings.TrimSpace(env)) == "production" {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}
