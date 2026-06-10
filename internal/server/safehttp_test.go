package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestSafeHTTP_isPublicIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.1.1", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			t.Parallel()
			if got := isPublicIP(net.ParseIP(tc.ip)); got != tc.want {
				t.Fatalf("isPublicIP(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestSafeHTTP_dialContext_blocksPrivateIP(t *testing.T) {
	t.Parallel()
	dial := safeTransportDialContext(2 * time.Second)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error dialing loopback")
	}
	if !errors.Is(err, errBlockedIP) {
		t.Fatalf("expected errBlockedIP, got %v", err)
	}
}

func TestSafeHTTP_dialContext_blocksPrivateRange(t *testing.T) {
	t.Parallel()
	dial := safeTransportDialContext(2 * time.Second)
	for _, addr := range []string{"10.0.0.5:443", "192.168.0.1:80", "169.254.0.1:80"} {
		_, err := dial(context.Background(), "tcp", addr)
		if !errors.Is(err, errBlockedIP) {
			t.Fatalf("addr %q: expected errBlockedIP, got %v", addr, err)
		}
	}
}

func TestSafeHTTP_checkRedirect_blocksUnsafeURL(t *testing.T) {
	t.Parallel()
	check := safeCheckRedirect(isWebhookURLSafe, 3)
	req := &http.Request{URL: mustParseURL(t, "http://192.168.0.1/")}
	err := check(req, []*http.Request{{URL: mustParseURL(t, "https://example.com/")}})
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected ErrUseLastResponse, got %v", err)
	}
}

func TestSafeHTTP_checkRedirect_allowsSafeURL(t *testing.T) {
	t.Parallel()
	check := safeCheckRedirect(isWebhookURLSafe, 3)
	req := &http.Request{URL: mustParseURL(t, "https://8.8.8.8/path")}
	if err := check(req, []*http.Request{{URL: mustParseURL(t, "https://example.com/")}}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestSafeHTTP_checkRedirect_maxRedirects(t *testing.T) {
	t.Parallel()
	check := safeCheckRedirect(isWebhookURLSafe, 2)
	req := &http.Request{URL: mustParseURL(t, "https://example.com/2")}
	via := []*http.Request{
		{URL: mustParseURL(t, "https://example.com/0")},
		{URL: mustParseURL(t, "https://example.com/1")},
	}
	if !errors.Is(check(req, via), http.ErrUseLastResponse) {
		t.Fatal("expected redirect cap")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
