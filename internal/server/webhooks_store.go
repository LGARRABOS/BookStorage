package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	webhookEventWorkUpdated        = "work.updated"
	webhookEventWorkDeleted        = "work.deleted"
	webhookEventWorkChapterChanged = "work.chapter_changed"
	webhookEventPing               = "ping"
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
