package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleRegister_duplicateUsernameGenericError(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("ValidPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, email) VALUES ('takenuser', ?, 1, 0, ?)`,
		hashed, "taken@example.com",
	); err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("username", "takenuser")
	form.Set("email", "other@example.com")
	form.Set("password", "ValidPass!99")
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleRegister(rec, req)
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "error=1") {
		t.Fatalf("expected generic error redirect, got %q", loc)
	}
	if strings.Contains(loc, "exists") {
		t.Fatalf("must not reveal username existence, got %q", loc)
	}
}

func TestHandleRegister_rejectsShortPassword(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	form := url.Values{}
	form.Set("username", "shortpwuser")
	form.Set("email", "user@example.com")
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

func TestHandleRegister_requiresValidEmail(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	form := url.Values{}
	form.Set("username", "mailuser")
	form.Set("email", "not-an-email")
	form.Set("password", "ValidPass!99")
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleRegister(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=email") {
		t.Fatalf("expected email error redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestHandleRegister_storesNormalizedEmail(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	form := url.Values{}
	form.Set("username", "mailuser2")
	form.Set("email", "  User@Example.COM ")
	form.Set("password", "ValidPass!99")
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleRegister(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	var email string
	if err := db.QueryRow(`SELECT email FROM users WHERE username = ?`, "mailuser2").Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "user@example.com" {
		t.Fatalf("email %q", email)
	}
}

func TestHandleLogin_locksAfterRepeatedFailures(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("GoodPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin) VALUES ('lockuser', ?, 1, 0)`,
		hashed,
	); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < loginMaxFailuresBeforeLock; i++ {
		form := url.Values{}
		form.Set("username", "lockuser")
		form.Set("password", "wrong")
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		app.HandleLogin(rec, req)
	}

	form := url.Values{}
	form.Set("username", "lockuser")
	form.Set("password", "GoodPass!99")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleLogin(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=1") {
		t.Fatalf("expected lockout redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestHandleLogin_unknownUserStillFails(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	form := url.Values{}
	form.Set("username", "nobody-here")
	form.Set("password", "SomePass!99")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleLogin(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=1") {
		t.Fatalf("expected login failure, got %q", rec.Header().Get("Location"))
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
