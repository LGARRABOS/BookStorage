package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeStatus represents the availability state of a reading site.
type ProbeStatus string

const (
	ProbeStatusUp       ProbeStatus = "up"
	ProbeStatusDown     ProbeStatus = "down"
	ProbeStatusDegraded ProbeStatus = "degraded"
	ProbeStatusUnknown  ProbeStatus = "unknown"
)

type readingSite struct {
	ID              int
	UserID          int
	Name            string
	BaseURL         string
	LastProbeAt     sql.NullString
	ProbeStatus     string
	ProbeHTTPStatus sql.NullInt64
	ProbeDetail     sql.NullString
}

// MatchReadingSite finds the best reading_sites row for a given link URL.
// It normalizes hosts (lowercase, optional www strip) and picks the longest base_url path prefix match.
func (a *App) MatchReadingSite(userID int, link string) (siteID int64, ok bool) {
	if link == "" {
		return 0, false
	}
	parsed, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return 0, false
	}
	linkHost := normalizeHost(parsed.Host)
	linkPath := normalizePath(parsed.Path)

	rows, err := a.DB.Query(`SELECT id, base_url FROM reading_sites WHERE user_id = ?`, userID)
	if err != nil {
		return 0, false
	}
	defer func() { _ = rows.Close() }()

	var bestID int64
	bestPathLen := -1

	for rows.Next() {
		var id int64
		var baseURL string
		if err := rows.Scan(&id, &baseURL); err != nil {
			continue
		}
		siteURL, err := url.Parse(strings.TrimSpace(baseURL))
		if err != nil {
			continue
		}
		siteHost := normalizeHost(siteURL.Host)
		if !hostsMatch(linkHost, siteHost) {
			continue
		}
		sitePath := normalizePath(siteURL.Path)
		if strings.HasPrefix(linkPath, sitePath) && len(sitePath) > bestPathLen {
			bestID = id
			bestPathLen = len(sitePath)
		}
	}
	if bestPathLen < 0 {
		return 0, false
	}
	return bestID, true
}

func normalizeHost(h string) string {
	host := strings.ToLower(strings.TrimSpace(h))
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return host
}

func stripWWW(host string) string {
	if strings.HasPrefix(host, "www.") {
		return host[4:]
	}
	return host
}

func hostsMatch(a, b string) bool {
	if a == b {
		return true
	}
	return stripWWW(a) == stripWWW(b)
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// ProbeReadingSite performs a safe HTTP probe of the given base URL and returns the status.
func ProbeReadingSite(ctx context.Context, baseURL string) (status ProbeStatus, httpStatus int, detail string) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ProbeStatusDown, 0, "invalid URL scheme"
	}

	host := parsed.Hostname()
	if isPrivateOrLoopback(host) {
		return ProbeStatusDown, 0, "private/loopback address"
	}

	// Resolve DNS to check for private IPs
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return ProbeStatusDown, 0, "DNS resolution failed"
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
			return ProbeStatusDown, 0, "resolved to private IP"
		}
	}

	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			rHost := req.URL.Hostname()
			if isPrivateOrLoopback(rHost) {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
		},
	}

	const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, nil)
	if err != nil {
		return ProbeStatusDown, 0, "bad request"
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return ProbeStatusDegraded, 0, "timeout"
		}
		if isTLSError(err) {
			return ProbeStatusDegraded, 0, "TLS error"
		}
		return ProbeStatusDown, 0, "connection error"
	}
	defer func() { _ = resp.Body.Close() }()

	code := resp.StatusCode
	// HEAD returned 4xx (blocked or not allowed) — retry with GET
	if code == http.StatusMethodNotAllowed || (code >= 400 && code < 500) {
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
		req2.Header.Set("User-Agent", browserUA)
		req2.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			if isTimeoutError(err2) {
				return ProbeStatusDegraded, code, "timeout on GET fallback"
			}
			return ProbeStatusDegraded, code, "GET fallback failed"
		}
		defer func() { _ = resp2.Body.Close() }()
		code = resp2.StatusCode
	}

	switch {
	case code >= 200 && code < 400:
		return ProbeStatusUp, code, ""
	case code == 403:
		// 403 after GET usually means anti-bot protection (Cloudflare, etc.)
		// The server IS responding — site is up for real users with a browser.
		return ProbeStatusUp, code, ""
	case code >= 400 && code < 500:
		return ProbeStatusDegraded, code, "client error"
	default:
		return ProbeStatusDown, code, "server error"
	}
}

func isPrivateOrLoopback(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "timeout")
}

func isTLSError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "tls") || strings.Contains(msg, "certificate") || strings.Contains(msg, "x509")
}

// ProbeAndUpdateSite runs the probe for a single site and updates the DB row.
func (a *App) ProbeAndUpdateSite(ctx context.Context, site readingSite) ProbeStatus {
	status, httpCode, detail := ProbeReadingSite(ctx, site.BaseURL)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var httpArg any
	if httpCode > 0 {
		httpArg = httpCode
	}
	var detailArg any
	if detail != "" {
		detailArg = detail
	}
	_, _ = a.DB.Exec(
		`UPDATE reading_sites SET last_probe_at = ?, probe_status = ?, probe_http_status = ?, probe_detail = ? WHERE id = ?`,
		now, string(status), httpArg, detailArg, site.ID,
	)
	return status
}

// ProbeAllUserSites probes all sites for a user, respecting TTL (skip if probed within minInterval).
func (a *App) ProbeAllUserSites(ctx context.Context, userID int, minInterval time.Duration) {
	sites := a.loadUserReadingSites(userID)
	now := time.Now().UTC()
	for _, s := range sites {
		if s.LastProbeAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", s.LastProbeAt.String); err == nil {
				if now.Sub(t) < minInterval {
					continue
				}
			}
		}
		a.ProbeAndUpdateSite(ctx, s)
	}
}

func (a *App) loadUserReadingSites(userID int) []readingSite {
	rows, err := a.DB.Query(
		`SELECT id, user_id, name, base_url, last_probe_at, COALESCE(probe_status, 'unknown'), probe_http_status, probe_detail FROM reading_sites WHERE user_id = ? ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var sites []readingSite
	for rows.Next() {
		var s readingSite
		if err := rows.Scan(&s.ID, &s.UserID, &s.Name, &s.BaseURL, &s.LastProbeAt, &s.ProbeStatus, &s.ProbeHTTPStatus, &s.ProbeDetail); err != nil {
			continue
		}
		sites = append(sites, s)
	}
	return sites
}

func (a *App) loadReadingSiteStatusMap(userID int) map[int]readingSite {
	sites := a.loadUserReadingSites(userID)
	m := make(map[int]readingSite, len(sites))
	for _, s := range sites {
		m[s.ID] = s
	}
	return m
}

// StartBackgroundProber launches a goroutine that probes all reading sites every interval.
// It stops when ctx is cancelled.
func (a *App) StartBackgroundProber(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.probeAllSites(ctx)
			}
		}
	}()
}

func (a *App) probeAllSites(ctx context.Context) {
	rows, err := a.DB.Query(`SELECT id, user_id, name, base_url, last_probe_at, COALESCE(probe_status, 'unknown'), probe_http_status, probe_detail FROM reading_sites`)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	var sites []readingSite
	for rows.Next() {
		var s readingSite
		if err := rows.Scan(&s.ID, &s.UserID, &s.Name, &s.BaseURL, &s.LastProbeAt, &s.ProbeStatus, &s.ProbeHTTPStatus, &s.ProbeDetail); err != nil {
			continue
		}
		sites = append(sites, s)
	}
	for _, s := range sites {
		select {
		case <-ctx.Done():
			return
		default:
		}
		a.ProbeAndUpdateSite(ctx, s)
	}
}
