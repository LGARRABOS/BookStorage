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

const (
	rateBucketEvictAfter = time.Hour
	rateBucketEvictEvery = 5 * time.Minute
)

type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*rateBucket
	lastEvict time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]*rateBucket{}}
}

func (rl *rateLimiter) evictStaleLocked(now time.Time) {
	if !rl.lastEvict.IsZero() && now.Sub(rl.lastEvict) < rateBucketEvictEvery {
		return
	}
	rl.lastEvict = now
	for k, b := range rl.buckets {
		if now.Sub(b.last) > rateBucketEvictAfter {
			delete(rl.buckets, k)
		}
	}
}

func (rl *rateLimiter) allow(key string, capacity, refillPerSec float64) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.evictStaleLocked(now)

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

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
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

func isSameOriginRequest(r *http.Request, publicOrigin string) bool {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return false
	}
	publicOrigin = strings.TrimSpace(publicOrigin)
	if publicOrigin != "" {
		po, err := url.Parse(publicOrigin)
		if err != nil || strings.TrimSpace(po.Host) == "" {
			return false
		}
		if !strings.EqualFold(po.Host, host) {
			return false
		}
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
	case path == "/login", path == "/register", path == "/forgot-password", path == "/reset-password":
		return "auth", 8, 0.5, true
	case path == "/api/works/bulk",
		strings.HasPrefix(path, "/api/works"),
		strings.HasPrefix(path, "/api/increment/"),
		strings.HasPrefix(path, "/api/decrement/"),
		strings.HasPrefix(path, "/api/set-chapter/"),
		strings.HasPrefix(path, "/api/delete/"),
		path == "/profile/delete",
		path == "/profile/google/unlink",
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

func shouldRateLimitOAuthGET(path, method string) (key string, capacity, refillPerSec float64, ok bool) {
	if method != http.MethodGet {
		return "", 0, 0, false
	}
	switch path {
	case "/auth/google", "/auth/google/link":
		return "auth_oauth", 20, 0.4, true
	default:
		return "", 0, 0, false
	}
}

// WithRequestPolicies applies lightweight CSRF and rate limiting checks.
func (a *App) WithRequestPolicies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trustProxy := a.Settings != nil && a.Settings.TrustProxy
		if key, cap, refill, ok := shouldRateLimitOAuthGET(r.URL.Path, r.Method); ok {
			limiterKey := key + ":" + clientIP(r, trustProxy)
			if !globalRateLimiter.allow(limiterKey, cap, refill) {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
		}
		if key, cap, refill, ok := shouldRateLimit(r.URL.Path); ok && isMutatingMethod(r.Method) {
			limiterKey := key + ":" + clientIP(r, trustProxy)
			if !globalRateLimiter.allow(limiterKey, cap, refill) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					a.apiWriteError(w, http.StatusTooManyRequests, "rate_limited")
				} else {
					http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				}
				return
			}
		}

		publicOrigin := ""
		if a.Settings != nil {
			publicOrigin = a.Settings.PublicOrigin
		}
		if isMutatingMethod(r.Method) && !strings.HasPrefix(r.URL.Path, "/auth/webauthn/") && !isSameOriginRequest(r, publicOrigin) {
			if strings.HasPrefix(r.URL.Path, "/api/") && a.hasValidAPIToken(r) {
				next.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				a.apiWriteError(w, http.StatusForbidden, "csrf_blocked")
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
