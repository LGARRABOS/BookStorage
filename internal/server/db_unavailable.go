package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"bookstorage/internal/database"
)

const dbProbeTTL = 2 * time.Second

// dbAvailabilityProbe caches PingContext results to avoid hammering the DB on every request.
type dbAvailabilityProbe struct {
	mu      sync.Mutex
	checked time.Time
	ok      bool
}

func (p *dbAvailabilityProbe) check(db *database.Conn) bool {
	if db == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.checked.IsZero() && time.Since(p.checked) < dbProbeTTL {
		return p.ok
	}
	p.checked = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := db.PingContext(ctx)
	p.ok = err == nil
	if err != nil {
		log.Printf("database unavailable: %v", err)
	}
	return p.ok
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
