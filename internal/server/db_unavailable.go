package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"bookstorage/internal/database"
)

// dbAvailabilityProbe runs Ping before each HTTP request (no positive cache).
// Caching "DB up" would let mutating requests through until the next full page load after an outage.
type dbAvailabilityProbe struct{}

func (p *dbAvailabilityProbe) check(db *database.Conn) bool {
	if db == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := db.PingContext(ctx)
	if err != nil {
		log.Printf("database unavailable: %v", err)
	}
	return err == nil
}

// WithDatabaseUnavailable serves a maintenance-style page (503) when the database cannot be reached.
// /healthz, /metrics, and /static/* are excluded so probes and assets keep working.
func (a *App) WithDatabaseUnavailable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/healthz" || p == "/metrics" || strings.HasPrefix(p, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.dbProbe.check(a.DB) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "15")
			if strings.HasPrefix(p, "/api/") {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"service_unavailable","reason":"database"}`))
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			a.renderTemplate(w, r, "maintenance", a.baseData(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}
