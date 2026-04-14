package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type rateBucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]*rateBucket{}}
}

func (rl *rateLimiter) allow(key string, capacity, refillPerSec float64) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &rateBucket{tokens: capacity - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * refillPerSec
		if b.tokens > capacity {
			b.tokens = capacity
		}
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

var globalRateLimiter = newRateLimiter()

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isSameOriginRequest(r *http.Request) bool {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return false
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, host)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		u, err := url.Parse(referer)
		return err == nil && strings.EqualFold(u.Host, host)
	}
	return false
}

func shouldRateLimit(path string) (key string, capacity, refillPerSec float64, ok bool) {
	switch {
	case path == "/login", path == "/register":
		return "auth", 8, 0.5, true
	case strings.HasPrefix(path, "/api/works"),
		strings.HasPrefix(path, "/api/increment/"),
		strings.HasPrefix(path, "/api/decrement/"),
		strings.HasPrefix(path, "/api/set-chapter/"),
		strings.HasPrefix(path, "/api/delete/"),
		path == "/profile/delete",
		path == "/import",
		strings.HasPrefix(path, "/tools/csv-import"),
		strings.HasPrefix(path, "/users/"),
		strings.HasPrefix(path, "/admin/"),
		strings.HasPrefix(path, "/api/admin/"):
		return "write", 30, 2.0, true
	default:
		return "", 0, 0, false
	}
}

// WithRequestPolicies applies lightweight CSRF and rate limiting checks.
func (a *App) WithRequestPolicies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key, cap, refill, ok := shouldRateLimit(r.URL.Path); ok && isMutatingMethod(r.Method) {
			limiterKey := key + ":" + clientIP(r)
			if !globalRateLimiter.allow(limiterKey, cap, refill) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					a.apiWriteError(w, http.StatusTooManyRequests, "rate_limited")
				} else {
					http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				}
				return
			}
		}

		if isMutatingMethod(r.Method) {
			if _, err := r.Cookie("session"); err == nil && !isSameOriginRequest(r) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					a.apiWriteError(w, http.StatusForbidden, "csrf_blocked")
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
