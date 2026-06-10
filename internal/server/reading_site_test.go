package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestIsProbeURLSafe_rejectsUnsafeURLs(t *testing.T) {
	t.Parallel()
	unsafe := []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"https://localhost/",
		"ftp://example.com/",
	}
	for _, raw := range unsafe {
		if isProbeURLSafe(raw) {
			t.Fatalf("isProbeURLSafe(%q) = true, want false", raw)
		}
	}
}

func TestProbeURL_rejectsUnsafeURL(t *testing.T) {
	t.Parallel()
	status, code, detail := ProbeURL(context.Background(), "http://127.0.0.1/")
	if status != ProbeStatusDown || code != 0 || detail != "unsafe URL" {
		t.Fatalf("ProbeURL unsafe: status=%q code=%d detail=%q", status, code, detail)
	}
}

func TestProbeHTTPClient_checkRedirect_blocksPrivateIP(t *testing.T) {
	t.Parallel()
	check := safeCheckRedirect(isProbeURLSafe, 3)
	req := &http.Request{URL: mustParseURL(t, "http://127.0.0.1/")}
	err := check(req, []*http.Request{{URL: mustParseURL(t, "https://example.com/")}})
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected ErrUseLastResponse, got %v", err)
	}
}

func TestNewProbeHTTPClient_blocksDialToPrivateIP(t *testing.T) {
	t.Parallel()
	// Listener on loopback; safe client must refuse to connect even if URL passed initial checks.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()

	client := newProbeHTTPClient(3 * time.Second)
	req, err := http.NewRequest(http.MethodHead, "http://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected dial error to private IP")
	}
}
