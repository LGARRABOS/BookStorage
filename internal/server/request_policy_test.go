package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bookstorage/internal/config"
)

func TestIsSameOriginRequest_publicOriginHostAllowList(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.Host = "evil.example"
	r.Header.Set("Origin", "https://evil.example")

	if isSameOriginRequest(r, "https://books.example.com") {
		t.Fatal("expected CSRF block when Host does not match PublicOrigin")
	}

	r.Host = "books.example.com"
	r.Header.Set("Origin", "https://books.example.com")
	if !isSameOriginRequest(r, "https://books.example.com") {
		t.Fatal("expected same-origin when Host and Origin match PublicOrigin")
	}
}

func TestWithRequestPolicies_blocksMismatchedPublicOriginHost(t *testing.T) {
	db, s := openTestDB(t)
	s.PublicOrigin = "https://books.example.com"
	app := &App{Settings: s, DB: db}
	handler := app.WithRequestPolicies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=u&password=p"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://books.example.com")
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRateLimiter_evictsStaleBuckets(t *testing.T) {
	rl := newRateLimiter()
	rl.mu.Lock()
	rl.buckets["stale:key"] = &rateBucket{tokens: 1, last: time.Now().Add(-2 * time.Hour)}
	rl.buckets["fresh:key"] = &rateBucket{tokens: 1, last: time.Now()}
	rl.mu.Unlock()

	if !rl.allow("fresh:key", 8, 0.5) {
		t.Fatal("expected fresh bucket to allow request")
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if _, ok := rl.buckets["stale:key"]; ok {
		t.Fatal("expected stale bucket to be evicted")
	}
	if _, ok := rl.buckets["fresh:key"]; !ok {
		t.Fatal("expected fresh bucket to remain")
	}
}

func TestSecurityHeaders_enablesHSTSForHTTPSPublicOrigin(t *testing.T) {
	app := &App{Settings: &config.Settings{
		Environment:  "development",
		PublicOrigin: "https://books.example.com",
	}}
	handler := app.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("expected HSTS header when PublicOrigin is https")
	}
}
