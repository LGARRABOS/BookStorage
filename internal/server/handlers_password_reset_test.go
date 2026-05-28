package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/mail"
)

func enableMailSettings(s *config.Settings) {
	s.PublicOrigin = "https://books.example.com"
	s.MailjetAPIKeyPublic = "pub"
	s.MailjetAPIKeyPrivate = "priv"
	s.MailFrom = "BookStorage <noreply@books.example.com>"
}

func TestHandleForgotPassword_notConfigured(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	req := httptest.NewRequest(http.MethodGet, "/forgot-password", nil)
	rec := httptest.NewRecorder()
	app.HandleForgotPassword(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleForgotPassword_fullFlow(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email) VALUES ('resetuser', ?, 1, 0, ?)`,
		hashed, "reset@example.com",
	); err != nil {
		t.Fatal(err)
	}

	var capturedToken string
	var capturedMsg mail.Message
	mail.SetSendHook(func(_ context.Context, msg mail.Message) error {
		capturedMsg = msg
		if msg.To != "reset@example.com" {
			t.Fatalf("to: %q", msg.To)
		}
		const prefix = "https://books.example.com/reset-password?token="
		for _, line := range strings.Split(msg.TextBody, "\n") {
			if strings.HasPrefix(line, prefix) {
				capturedToken = strings.TrimPrefix(line, prefix)
				break
			}
		}
		if capturedToken == "" {
			for _, part := range []string{msg.HTMLBody, msg.TextBody} {
				if i := strings.Index(part, prefix); i >= 0 {
					rest := part[i+len(prefix):]
					if j := strings.IndexAny(rest, `"' <>`); j >= 0 {
						capturedToken = rest[:j]
					} else {
						capturedToken = rest
					}
					break
				}
			}
		}
		return nil
	})
	t.Cleanup(func() { mail.SetSendHook(nil) })

	form := url.Values{}
	form.Set("email", "reset@example.com")
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleForgotPassword(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "sent=1") {
		t.Fatalf("redirect %q status %d", rec.Header().Get("Location"), rec.Code)
	}
	if capturedToken == "" {
		t.Fatal("expected reset token in email")
	}
	if strings.TrimSpace(capturedMsg.TextBody) == "" || strings.TrimSpace(capturedMsg.HTMLBody) == "" {
		t.Fatalf("email bodies must not be empty: text=%q html=%q", capturedMsg.TextBody, capturedMsg.HTMLBody)
	}
	if !strings.Contains(capturedMsg.HTMLBody, "Bonjour,") && !strings.Contains(capturedMsg.HTMLBody, "Hello,") {
		t.Fatalf("html missing greeting: %q", capturedMsg.HTMLBody)
	}
	var tokenHash string
	if err := db.QueryRow(`SELECT token_hash FROM password_reset_tokens`).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if tokenHash != hashSessionToken(capturedToken) {
		t.Fatalf("token hash mismatch")
	}

	oldSession := mustCreateSession(t, app, 2)

	resetForm := url.Values{}
	resetForm.Set("token", capturedToken)
	resetForm.Set("new_password", "NewPass!88")
	resetForm.Set("confirm_password", "NewPass!88")
	resetReq := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(resetForm.Encode()))
	resetReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resetRec := httptest.NewRecorder()
	app.HandleResetPassword(resetRec, resetReq)
	if resetRec.Code != http.StatusFound || !strings.Contains(resetRec.Header().Get("Location"), "/login?reset=1") {
		t.Fatalf("reset redirect %q status %d", resetRec.Header().Get("Location"), resetRec.Code)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/", nil)
	sessionReq.AddCookie(&http.Cookie{Name: "session", Value: oldSession})
	if uid, _, ok := app.currentSession(sessionReq); ok || uid != 0 {
		t.Fatalf("expected session revoked, uid=%d ok=%v", uid, ok)
	}

	loginForm := url.Values{}
	loginForm.Set("username", "resetuser")
	loginForm.Set("password", "NewPass!88")
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	app.HandleLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusFound || strings.Contains(loginRec.Header().Get("Location"), "error") {
		t.Fatalf("login failed: %q", loginRec.Header().Get("Location"))
	}
}

func TestHandleForgotPassword_emailCooldown(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email) VALUES ('cooldownuser', ?, 1, 0, ?)`,
		hashed, "cooldown@example.com",
	); err != nil {
		t.Fatal(err)
	}

	var sent int
	mail.SetSendHook(func(context.Context, mail.Message) error {
		sent++
		return nil
	})
	t.Cleanup(func() { mail.SetSendHook(nil) })

	postForgot := func() {
		t.Helper()
		form := url.Values{}
		form.Set("email", "cooldown@example.com")
		req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		app.HandleForgotPassword(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("status %d", rec.Code)
		}
	}

	postForgot()
	if sent != 1 {
		t.Fatalf("first request: sent=%d want 1", sent)
	}

	postForgot()
	if sent != 1 {
		t.Fatalf("second request within cooldown: sent=%d want 1", sent)
	}

	recent := time.Now().UTC().Add(-passwordResetEmailCooldown - time.Minute)
	if _, err := db.Exec(
		`UPDATE password_reset_tokens SET created_at = ? WHERE user_id = 2`,
		recent,
	); err != nil {
		t.Fatal(err)
	}

	postForgot()
	if sent != 2 {
		t.Fatalf("after cooldown: sent=%d want 2", sent)
	}
}

func TestHandleForgotPassword_unknownEmailSameResponse(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := &App{Settings: s, DB: db}
	mail.SetSendHook(func(context.Context, mail.Message) error { return nil })
	t.Cleanup(func() { mail.SetSendHook(nil) })

	form := url.Values{}
	form.Set("email", "nobody@example.com")
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleForgotPassword(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "sent=1") {
		t.Fatalf("redirect %q", rec.Header().Get("Location"))
	}
}
