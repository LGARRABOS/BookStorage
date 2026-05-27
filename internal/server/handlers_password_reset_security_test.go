package server

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
	"bookstorage/internal/mail"
)

func resetPasswordRenderApp(t *testing.T, db *database.Conn, s *config.Settings) *App {
	t.Helper()
	tpl := template.Must(template.New("").Parse(`{{ define "reset_password" }}err={{ .ResetError }} form={{ .FormError }}{{ end }}`))
	return &App{
		Settings:        s,
		DB:              db,
		TemplatesWeb:    tpl,
		TemplatesMobile: tpl,
	}
}

func TestHandleResetPassword_rejectsExpiredToken(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := resetPasswordRenderApp(t, db, s)

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email) VALUES ('expuser', ?, 1, 0, ?)`,
		hashed, "exp@example.com",
	); err != nil {
		t.Fatal(err)
	}
	rawToken, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	if _, err := db.Exec(
		`INSERT INTO password_reset_tokens (token_hash, user_id, created_at, expires_at) VALUES (?, 2, ?, ?)`,
		hashSessionToken(rawToken), expired.Add(-time.Hour), expired,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/reset-password?token="+rawToken, nil)
	rec := httptest.NewRecorder()
	app.HandleResetPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "err=invalid") {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestHandleResetPassword_rejectsReusedToken(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := resetPasswordRenderApp(t, db, s)

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email) VALUES ('reuseuser', ?, 1, 0, ?)`,
		hashed, "reuse@example.com",
	); err != nil {
		t.Fatal(err)
	}
	rawToken, err := app.createPasswordResetToken(2)
	if err != nil {
		t.Fatal(err)
	}
	app.markPasswordResetTokenUsed(rawToken)

	form := url.Values{}
	form.Set("token", rawToken)
	form.Set("new_password", "NewPass!88")
	form.Set("confirm_password", "NewPass!88")
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleResetPassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "form=invalid") {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestHandleForgotPassword_skipsGoogleOnlyUser(t *testing.T) {
	db, s := openTestDB(t)
	enableMailSettings(s)
	app := &App{Settings: s, DB: db}

	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email, google_sub) VALUES ('googleuser', NULL, 1, 0, ?, 'sub-1')`,
		"google@example.com",
	); err != nil {
		t.Fatal(err)
	}

	var sent int
	mail.SetSendHook(func(context.Context, mail.Message) error {
		sent++
		return nil
	})
	t.Cleanup(func() { mail.SetSendHook(nil) })

	form := url.Values{}
	form.Set("email", "google@example.com")
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleForgotPassword(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if sent != 0 {
		t.Fatalf("expected no email, sent=%d", sent)
	}
}
