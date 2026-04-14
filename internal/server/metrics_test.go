package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bookstorage/internal/config"
)

func TestMetricsRequestAuthorized_loopbackNoToken(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: ""}}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	if !a.metricsRequestAuthorized(req) {
		t.Fatal("expected loopback without token to be allowed")
	}
}

func TestMetricsRequestAuthorized_ipv6LoopbackNoToken(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: ""}}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "[::1]:5555"
	if !a.metricsRequestAuthorized(req) {
		t.Fatal("expected ::1 without token to be allowed")
	}
}

func TestMetricsRequestAuthorized_nonLoopbackNoToken(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: ""}}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "192.0.2.1:5555"
	if a.metricsRequestAuthorized(req) {
		t.Fatal("expected non-loopback without token to be denied")
	}
}

func TestMetricsRequestAuthorized_bearerToken(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: "test-secret-token"}}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "192.0.2.1:5555"
	req.Header.Set("Authorization", "Bearer test-secret-token")
	if !a.metricsRequestAuthorized(req) {
		t.Fatal("expected valid bearer to be allowed from any IP")
	}
}

func TestMetricsRequestAuthorized_queryToken(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: "abc"}}
	req := httptest.NewRequest(http.MethodGet, "/metrics?token=abc", nil)
	req.RemoteAddr = "192.0.2.1:5555"
	if !a.metricsRequestAuthorized(req) {
		t.Fatal("expected valid query token to be allowed")
	}
}

func TestMetricsRequestAuthorized_wrongBearer(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: "good"}}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Authorization", "Bearer bad")
	if a.metricsRequestAuthorized(req) {
		t.Fatal("expected wrong bearer to be denied even from loopback when token is set")
	}
}

func TestHandleMetrics_GET(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: "", Port: 5000}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:1"
	a.HandleMetrics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if body == "" || body[0] == '<' {
		t.Fatal("expected non-empty prometheus text body")
	}
}

func TestHandleMetrics_methodNotAllowed(t *testing.T) {
	a := &App{Settings: &config.Settings{MetricsToken: ""}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:1"
	a.HandleMetrics(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}
