package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	webhookSignatureHeader = "X-BookStorage-Signature"
	webhookMaxAttempts     = 5
	webhookDeliveryTimeout = 15 * time.Second
	webhookWorkerInterval  = 10 * time.Second
)

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

func signWebhookPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
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

	client := newWebhookHTTPClient(webhookDeliveryTimeout)
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
