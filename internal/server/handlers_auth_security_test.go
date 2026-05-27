package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleRegister_rejectsShortPassword(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	form := url.Values{}
	form.Set("username", "shortpwuser")
	form.Set("password", "abc")
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleRegister(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=weak") {
		t.Fatalf("expected weak password redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestHandleLogin_rejectsPlaintextPassword(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	plain := "legacy-plain-pass"
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin) VALUES ('legacyuser', ?, 1, 0)`,
		plain,
	); err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	form.Set("username", "legacyuser")
	form.Set("password", plain)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleLogin(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=1") {
		t.Fatalf("expected login failure redirect, got %q", rec.Header().Get("Location"))
	}
}
