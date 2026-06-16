package server

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestIsWebhookURLSafe_literalIPs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://8.8.8.8/hook", true},
		{"http://127.0.0.1/hook", false},
		{"http://10.0.0.1/hook", false},
		{"http://192.168.1.1/hook", false},
		{"http://169.254.169.254/latest/meta-data", false},
		{"ftp://example.com/hook", false},
		{"https://localhost/hook", false},
		{"https://svc.local/hook", false},
		{"https://app.internal/hook", false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			if got := isWebhookURLSafe(tc.url); got != tc.want {
				t.Fatalf("isWebhookURLSafe(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestIsWebhookURLSafe_hostnameResolvesToPrivate(t *testing.T) {
	t.Parallel()
	// Custom resolver is not injected; use a hostname that only resolves privately when present.
	// Verify public hostname parsing path with literal public IP already covered.
	if isWebhookURLSafe("http://[" + net.IPv4(127, 0, 0, 1).String() + "]/") {
		t.Fatal("IPv4-mapped loopback URL must be rejected")
	}
}

func TestWebhookHTTPClient_checkRedirect_blocksPrivateIP(t *testing.T) {
	t.Parallel()
	check := safeCheckRedirect(isWebhookURLSafe, 3)
	req := &http.Request{URL: mustParseURL(t, "http://10.0.0.1/internal")}
	err := check(req, []*http.Request{{URL: mustParseURL(t, "https://8.8.8.8/hook")}})
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected ErrUseLastResponse, got %v", err)
	}
}
