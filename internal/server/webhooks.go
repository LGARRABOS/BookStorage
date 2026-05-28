package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	webhookSignatureHeader         = "X-BookStorage-Signature"
	webhookEventWorkUpdated        = "work.updated"
	webhookEventWorkDeleted        = "work.deleted"
	webhookEventWorkChapterChanged = "work.chapter_changed"
	webhookEventPing               = "ping"

	webhookMaxAttempts     = 5
	webhookDeliveryTimeout = 15 * time.Second
	webhookWorkerInterval  = 10 * time.Second
)

var validWebhookEvents = map[string]bool{
	webhookEventWorkUpdated:        true,
	webhookEventWorkDeleted:        true,
	webhookEventWorkChapterChanged: true,
	webhookEventPing:               true,
}

type webhookEndpointRow struct {
	ID        int
	UserID    int
	URL       string
	Secret    string
	Events    []string
	Enabled   bool
	CreatedAt time.Time
}

func newWebhookSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func encodeWebhookEvents(events []string) string {
	b, _ := json.Marshal(normalizeWebhookEvents(events))
	return string(b)
}

func decodeWebhookEvents(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var events []string
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return nil
	}
	return normalizeWebhookEvents(events)
}

func normalizeWebhookEvents(events []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e == "" || !validWebhookEvents[e] || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

func webhookEndpointSubscribes(events []string, event string) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}

func isWebhookURLSafe(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") || strings.HasSuffix(lowerHost, ".internal") {
		return false
	}

	if ip := net.ParseIP(host); ip != nil {
		return isPublicIP(ip)
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return false
		}
	}
	return true
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0:
			return false
		case ip4[0] == 10:
			return false
		case ip4[0] == 127:
			return false
		case ip4[0] == 169 && ip4[1] == 254:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		case ip4[0] >= 224:
			return false
		}
	}
	return true
}

func signWebhookPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) listWebhookEndpoints(userID int) ([]webhookEndpointRow, error) {
	if userID <= 0 {
		return nil, nil
	}
	rows, err := a.DB.Query(
		`SELECT id, user_id, url, secret, events, enabled, created_at
		 FROM webhook_endpoints
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []webhookEndpointRow
	for rows.Next() {
		var row webhookEndpointRow
		var eventsRaw string
		var enabled int
		if err := rows.Scan(&row.ID, &row.UserID, &row.URL, &row.Secret, &eventsRaw, &enabled, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.Events = decodeWebhookEvents(eventsRaw)
		row.Enabled = enabled != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

func (a *App) createWebhookEndpoint(userID int, rawURL string, events []string) (webhookEndpointRow, error) {
	rawURL = strings.TrimSpace(rawURL)
	if userID <= 0 || rawURL == "" {
		return webhookEndpointRow{}, sql.ErrNoRows
	}
	if !isWebhookURLSafe(rawURL) {
		return webhookEndpointRow{}, fmt.Errorf("invalid webhook url")
	}
	events = normalizeWebhookEvents(events)
	if len(events) == 0 {
		return webhookEndpointRow{}, fmt.Errorf("no events selected")
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return webhookEndpointRow{}, err
	}
	now := time.Now().UTC()
	res, err := a.DB.Exec(
		`INSERT INTO webhook_endpoints (user_id, url, secret, events, enabled, created_at)
		 VALUES (?, ?, ?, ?, 1, ?)`,
		userID, rawURL, secret, encodeWebhookEvents(events), now,
	)
	if err != nil {
		return webhookEndpointRow{}, err
	}
	id, _ := res.LastInsertId()
	return webhookEndpointRow{
		ID:        int(id),
		UserID:    userID,
		URL:       rawURL,
		Secret:    secret,
		Events:    events,
		Enabled:   true,
		CreatedAt: now,
	}, nil
}

func (a *App) updateWebhookEndpoint(userID, endpointID int, rawURL string, events []string, enabled bool) error {
	if userID <= 0 || endpointID <= 0 {
		return sql.ErrNoRows
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL != "" && !isWebhookURLSafe(rawURL) {
		return fmt.Errorf("invalid webhook url")
	}
	events = normalizeWebhookEvents(events)
	if len(events) == 0 {
		return fmt.Errorf("no events selected")
	}
	enabledVal := 0
	if enabled {
		enabledVal = 1
	}
	var res sql.Result
	var err error
	if rawURL != "" {
		res, err = a.DB.Exec(
			`UPDATE webhook_endpoints SET url = ?, events = ?, enabled = ? WHERE id = ? AND user_id = ?`,
			rawURL, encodeWebhookEvents(events), enabledVal, endpointID, userID,
		)
	} else {
		res, err = a.DB.Exec(
			`UPDATE webhook_endpoints SET events = ?, enabled = ? WHERE id = ? AND user_id = ?`,
			encodeWebhookEvents(events), enabledVal, endpointID, userID,
		)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *App) deleteWebhookEndpoint(userID, endpointID int) error {
	if userID <= 0 || endpointID <= 0 {
		return sql.ErrNoRows
	}
	res, err := a.DB.Exec(`DELETE FROM webhook_endpoints WHERE id = ? AND user_id = ?`, endpointID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *App) EmitWebhookEvent(userID int, event string, payload map[string]any) {
	if userID <= 0 || !validWebhookEvents[event] || a.DB == nil {
		return
	}
	endpoints, err := a.listWebhookEndpoints(userID)
	if err != nil {
		return
	}
	body, err := json.Marshal(map[string]any{
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      payload,
	})
	if err != nil {
		return
	}
	payloadStr := string(body)
	now := time.Now().UTC()
	for _, ep := range endpoints {
		if !ep.Enabled || !webhookEndpointSubscribes(ep.Events, event) {
			continue
		}
		_, _ = a.DB.Exec(
			`INSERT INTO webhook_deliveries (endpoint_id, event, payload, status, attempts, next_retry_at, created_at)
			 VALUES (?, ?, ?, 'pending', 0, ?, ?)`,
			ep.ID, event, payloadStr, now, now,
		)
	}
}

func (a *App) deliverWebhook(ctx context.Context, deliveryID int) {
	var endpointID int
	var event, payload, secret, targetURL string
	err := a.DB.QueryRow(
		`SELECT d.endpoint_id, d.event, d.payload, e.secret, e.url
		 FROM webhook_deliveries d
		 JOIN webhook_endpoints e ON e.id = d.endpoint_id
		 WHERE d.id = ? AND d.status = 'pending' AND e.enabled = 1`,
		deliveryID,
	).Scan(&endpointID, &event, &payload, &secret, &targetURL)
	if err != nil {
		return
	}
	if !isWebhookURLSafe(targetURL) {
		_, _ = a.DB.Exec(`UPDATE webhook_deliveries SET status = 'failed', attempts = attempts + 1 WHERE id = ?`, deliveryID)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL, strings.NewReader(payload))
	if err != nil {
		a.scheduleWebhookRetry(deliveryID, 0)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "BookStorage-Webhooks/1.0")
	req.Header.Set("X-BookStorage-Event", event)
	req.Header.Set(webhookSignatureHeader, signWebhookPayload(secret, []byte(payload)))

	client := &http.Client{Timeout: webhookDeliveryTimeout}
	resp, err := client.Do(req)
	if err != nil {
		a.scheduleWebhookRetry(deliveryID, 0)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = a.DB.Exec(
			`UPDATE webhook_deliveries SET status = 'delivered', attempts = attempts + 1 WHERE id = ?`,
			deliveryID,
		)
		return
	}
	a.scheduleWebhookRetry(deliveryID, resp.StatusCode)
}

func webhookRetryDelay(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 2 * time.Minute
	case attempts == 3:
		return 4 * time.Minute
	default:
		return 8 * time.Minute
	}
}

func (a *App) scheduleWebhookRetry(deliveryID, httpStatus int) {
	var attempts int
	_ = a.DB.QueryRow(`SELECT attempts FROM webhook_deliveries WHERE id = ?`, deliveryID).Scan(&attempts)
	attempts++
	if attempts >= webhookMaxAttempts {
		_, _ = a.DB.Exec(
			`UPDATE webhook_deliveries SET status = 'failed', attempts = ? WHERE id = ?`,
			attempts, deliveryID,
		)
		log.Printf("[webhooks] delivery %d failed permanently (http=%d)", deliveryID, httpStatus)
		return
	}
	delay := webhookRetryDelay(attempts)
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	next := time.Now().UTC().Add(delay)
	_, _ = a.DB.Exec(
		`UPDATE webhook_deliveries SET status = 'pending', attempts = ?, next_retry_at = ? WHERE id = ?`,
		attempts, next, deliveryID,
	)
}

func (a *App) runWebhookWorkerCycle(ctx context.Context) {
	now := time.Now().UTC()
	rows, err := a.DB.Query(
		`SELECT id FROM webhook_deliveries
		 WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= ?)
		 ORDER BY created_at ASC
		 LIMIT 20`,
		now,
	)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		default:
			a.deliverWebhook(ctx, id)
		}
	}
}

func (a *App) StartWebhookWorker(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[webhooks] recovered from panic: %v — restarting in 30s", r)
				time.Sleep(30 * time.Second)
				a.StartWebhookWorker(ctx)
			}
		}()

		log.Printf("[webhooks] worker started — interval %v", webhookWorkerInterval)
		a.runWebhookWorkerCycle(ctx)

		ticker := time.NewTicker(webhookWorkerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[webhooks] worker stopped")
				return
			case <-ticker.C:
				a.runWebhookWorkerCycle(ctx)
			}
		}
	}()
}

func (a *App) HandleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	var events []string
	for _, ev := range []string{webhookEventWorkUpdated, webhookEventWorkDeleted, webhookEventWorkChapterChanged} {
		if r.FormValue("event_"+strings.ReplaceAll(ev, ".", "_")) == "1" {
			events = append(events, ev)
		}
	}
	row, err := a.createWebhookEndpoint(userID, r.FormValue("url"), events)
	if err != nil {
		http.Redirect(w, r, "/profile?webhook_error=1", http.StatusFound)
		return
	}
	a.renderProfilePage(w, r, userID, map[string]any{
		"NewWebhook":     row,
		"WebhookCreated": true,
	})
}

func (a *App) HandleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	endpointID, _ := strconv.Atoi(r.PathValue("id"))
	if endpointID <= 0 {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	var events []string
	for _, ev := range []string{webhookEventWorkUpdated, webhookEventWorkDeleted, webhookEventWorkChapterChanged} {
		if r.FormValue("event_"+strings.ReplaceAll(ev, ".", "_")) == "1" {
			events = append(events, ev)
		}
	}
	if len(events) == 0 {
		rows, err := a.listWebhookEndpoints(userID)
		if err == nil {
			for _, ep := range rows {
				if ep.ID == endpointID {
					events = ep.Events
					break
				}
			}
		}
	}
	enabled := r.FormValue("enabled") == "1"
	urlVal := strings.TrimSpace(r.FormValue("url"))
	if err := a.updateWebhookEndpoint(userID, endpointID, urlVal, events, enabled); err != nil {
		http.Redirect(w, r, "/profile?webhook_error=1", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?webhook_updated=1", http.StatusFound)
}

func (a *App) HandleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	endpointID, _ := strconv.Atoi(r.PathValue("id"))
	if endpointID <= 0 {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	_ = a.deleteWebhookEndpoint(userID, endpointID)
	http.Redirect(w, r, "/profile?webhook_deleted=1", http.StatusFound)
}

func (a *App) HandleTestWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	endpointID, _ := strconv.Atoi(r.PathValue("id"))
	if endpointID <= 0 {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	var exists int
	err := a.DB.QueryRow(
		`SELECT 1 FROM webhook_endpoints WHERE id = ? AND user_id = ?`,
		endpointID, userID,
	).Scan(&exists)
	if err != nil {
		http.Redirect(w, r, "/profile?webhook_error=1", http.StatusFound)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"event":     webhookEventPing,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data": map[string]any{
			"endpoint_id": endpointID,
			"message":     "BookStorage webhook test",
		},
	})
	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`INSERT INTO webhook_deliveries (endpoint_id, event, payload, status, attempts, next_retry_at, created_at)
		 VALUES (?, ?, ?, 'pending', 0, ?, ?)`,
		endpointID, webhookEventPing, string(body), now, now,
	)
	http.Redirect(w, r, "/profile?webhook_test=1", http.StatusFound)
}
