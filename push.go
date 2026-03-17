package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func (a *App) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":"method_not_allowed"}`))
		return
	}
	if a.Settings.VAPIDPublicKey == "" || a.Settings.VAPIDPrivateKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"push_not_configured"}`))
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}

	var sub struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || sub.Endpoint == "" || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_subscription"}`))
		return
	}

	_, err := a.DB.Exec(
		`DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`,
		userID, sub.Endpoint,
	)
	if err == nil {
		_, err = a.DB.Exec(
			`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES (?, ?, ?, ?)`,
			userID, sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth,
		)
	}
	if err != nil {
		log.Printf("[push] subscribe error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal_error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (a *App) handlePushVapidPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"vapid_public_key": a.Settings.VAPIDPublicKey}
	if a.Settings.VAPIDPublicKey == "" {
		out["vapid_public_key"] = nil
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (a *App) runReminderPushWorker() {
	if a.Settings.VAPIDPrivateKey == "" {
		return
	}
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.processDueReminders()
	}
}

func (a *App) processDueReminders() {
	rows, err := a.DB.Query(
		`SELECT r.id, r.user_id, r.work_id, w.title
         FROM reminders r JOIN works w ON r.work_id = w.id
         WHERE r.sent = 0 AND r.remind_at <= datetime('now')`,
	)
	if err != nil {
		log.Printf("[push] query reminders: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var remID, userID, workID int
		var title string
		if err := rows.Scan(&remID, &userID, &workID, &title); err != nil {
			continue
		}

		subRows, err := a.DB.Query(
			`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?`,
			userID,
		)
		if err != nil {
			continue
		}

		payload, _ := json.Marshal(map[string]string{
			"title": "BookStorage",
			"body":  "Rappel : " + title,
		})

		sent := false
		for subRows.Next() {
			var endpoint, p256dh, auth string
			if err := subRows.Scan(&endpoint, &p256dh, &auth); err != nil {
				continue
			}
			sub := &webpush.Subscription{
				Endpoint: endpoint,
				Keys: webpush.Keys{
					P256dh: p256dh,
					Auth:   auth,
				},
			}
			resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
				Subscriber:      "bookstorage@localhost",
				VAPIDPublicKey:  a.Settings.VAPIDPublicKey,
				VAPIDPrivateKey: a.Settings.VAPIDPrivateKey,
				TTL:             30,
			})
			if err != nil {
				log.Printf("[push] send error: %v", err)
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				sent = true
			}
			if resp.StatusCode == 410 || resp.StatusCode == 404 {
				_, _ = a.DB.Exec(`DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`, userID, endpoint)
			}
		}
		_ = subRows.Close()

		if sent {
			_, _ = a.DB.Exec(`UPDATE reminders SET sent = 1 WHERE id = ?`, remID)
		}
	}
}
