package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

var errBlockedIP = errors.New("blocked IP address")

func safeTransportDialContext(dialTimeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: dialTimeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("safehttp: expected resolved IP, got %q", host)
		}
		if !isPublicIP(ip) {
			return nil, errBlockedIP
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
}

type safeHTTPClientConfig struct {
	timeout       time.Duration
	dialTimeout   time.Duration
	maxRedirects  int
	urlSafe       func(string) bool
	tlsMinVersion uint16
}

func safeCheckRedirect(urlSafe func(string) bool, maxRedirects int) func(req *http.Request, via []*http.Request) error {
	if maxRedirects <= 0 {
		maxRedirects = 3
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return http.ErrUseLastResponse
		}
		if urlSafe != nil && !urlSafe(req.URL.String()) {
			return http.ErrUseLastResponse
		}
		return nil
	}
}

func newSafeHTTPClient(cfg safeHTTPClientConfig) *http.Client {
	transport := &http.Transport{
		DialContext: safeTransportDialContext(cfg.dialTimeout),
	}
	if cfg.tlsMinVersion != 0 {
		transport.TLSClientConfig = &tls.Config{MinVersion: cfg.tlsMinVersion}
	}
	return &http.Client{
		Timeout:       cfg.timeout,
		Transport:     transport,
		CheckRedirect: safeCheckRedirect(cfg.urlSafe, cfg.maxRedirects),
	}
}

func newWebhookHTTPClient(timeout time.Duration) *http.Client {
	return newSafeHTTPClient(safeHTTPClientConfig{
		timeout:      timeout,
		dialTimeout:  5 * time.Second,
		maxRedirects: 3,
		urlSafe:      isWebhookURLSafe,
	})
}

func newProbeHTTPClient(timeout time.Duration) *http.Client {
	return newSafeHTTPClient(safeHTTPClientConfig{
		timeout:       timeout,
		dialTimeout:   5 * time.Second,
		maxRedirects:  3,
		urlSafe:       isProbeURLSafe,
		tlsMinVersion: tls.VersionTLS12,
	})
}

// isProbeURLSafe mirrors webhook SSRF checks: scheme, blocked hostnames, and public IPs only.
func isProbeURLSafe(rawURL string) bool {
	return isWebhookURLSafe(rawURL)
}
