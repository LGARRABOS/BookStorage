package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
