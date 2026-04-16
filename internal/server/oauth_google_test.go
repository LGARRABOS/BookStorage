package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
	"bookstorage/internal/oauthgoogle"

	"golang.org/x/oauth2"
)

func testSettingsWithGoogle(t *testing.T, dir string) *config.Settings {
	t.Helper()
	s := testSettings(dir)
	s.PublicOrigin = "http://127.0.0.1:9"
	s.GoogleClientID = "test-client-id"
	s.GoogleClientSecret = "test-client-secret"
	return s
}

func TestHandleGoogleOAuthCallback_badState(t *testing.T) {
	dir := t.TempDir()
	s := testSettingsWithGoogle(t, dir)
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	app := &App{Settings: s, DB: db}

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=abc&state=nope", nil)
	rec := httptest.NewRecorder()
	app.HandleGoogleOAuthCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "google_error=state") {
		t.Fatalf("unexpected redirect: %s", loc)
	}
}

func TestHandleGoogleOAuthCallback_newUserPendingValidation(t *testing.T) {
	oldEx := googleOAuthExchangeHook
	oldUI := oauthgoogle.TestUserInfoHook
	defer func() {
		googleOAuthExchangeHook = oldEx
		oauthgoogle.TestUserInfoHook = oldUI
	}()
	googleOAuthExchangeHook = func(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "mock-token"}, nil
	}
	oauthgoogle.TestUserInfoHook = func(ctx context.Context, accessToken string) (oauthgoogle.UserInfo, error) {
		return oauthgoogle.UserInfo{Sub: "sub-pending-1", Email: "newpending@example.com"}, nil
	}

	dir := t.TempDir()
	s := testSettingsWithGoogle(t, dir)
	s.RequireAccountValidation = true
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	app := &App{Settings: s, DB: db}

	statePlain := "state-plain-test-1"
	verifier := "verifier-test-1"
	if err := database.InsertOAuthState(db, statePlain, database.OAuthPurposeLogin, sql.NullInt64{}, "", verifier); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=abc&state="+statePlain, nil)
	rec := httptest.NewRecorder()
	app.HandleGoogleOAuthCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "pending=1") {
		t.Fatalf("expected pending redirect, got %s", loc)
	}
	var validated int
	if err := db.QueryRow(`SELECT validated FROM users WHERE google_sub = ?`, "sub-pending-1").Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if validated != 0 {
		t.Fatalf("validated=%d want 0", validated)
	}
}

func TestHandleGoogleOAuthLink_subTaken(t *testing.T) {
	oldEx := googleOAuthExchangeHook
	oldUI := oauthgoogle.TestUserInfoHook
	defer func() {
		googleOAuthExchangeHook = oldEx
		oauthgoogle.TestUserInfoHook = oldUI
	}()
	googleOAuthExchangeHook = func(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "mock-token"}, nil
	}
	oauthgoogle.TestUserInfoHook = func(ctx context.Context, accessToken string) (oauthgoogle.UserInfo, error) {
		return oauthgoogle.UserInfo{Sub: "shared-sub", Email: "x@y.z"}, nil
	}

	dir := t.TempDir()
	s := testSettingsWithGoogle(t, dir)
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	app := &App{Settings: s, DB: db}

	if _, err := db.Exec(`INSERT INTO users (id, username, password, validated, google_sub) VALUES (99, 'other', 'x', 1, 'shared-sub')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username, password, validated) VALUES (2, 'linker', 'y', 1)`); err != nil {
		t.Fatal(err)
	}

	statePlain := "state-link-1"
	verifier := "verifier-link-1"
	if err := database.InsertOAuthState(db, statePlain, database.OAuthPurposeLink, sql.NullInt64{Int64: 2, Valid: true}, "", verifier); err != nil {
		t.Fatal(err)
	}

	token := mustCreateSession(t, app, 2)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=abc&state="+statePlain, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	app.HandleGoogleOAuthCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "google_error=link_taken") {
		t.Fatalf("unexpected redirect: %s", loc)
	}
}

func TestHandleLogin_googleOnlyUser(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	if _, err := db.Exec(`INSERT INTO users (username, password, validated, google_sub) VALUES ('gonly', NULL, 1, 'g-sub-1')`); err != nil {
		t.Fatal(err)
	}
	form := "username=gonly&password=anything"
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.HandleLogin(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "google_error=use_google") {
		t.Fatalf("unexpected redirect: %s", loc)
	}
}

func TestHandleGoogleOAuthStart_notConfigured(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
	rec := httptest.NewRecorder()
	app.HandleGoogleOAuthStart(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleGoogleOAuthCallback_createsSessionWhenValidated(t *testing.T) {
	oldEx := googleOAuthExchangeHook
	oldUI := oauthgoogle.TestUserInfoHook
	defer func() {
		googleOAuthExchangeHook = oldEx
		oauthgoogle.TestUserInfoHook = oldUI
	}()
	googleOAuthExchangeHook = func(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "mock-token"}, nil
	}
	oauthgoogle.TestUserInfoHook = func(ctx context.Context, accessToken string) (oauthgoogle.UserInfo, error) {
		return oauthgoogle.UserInfo{Sub: "sub-ok-1", Email: "ok@example.com"}, nil
	}

	dir := t.TempDir()
	s := testSettingsWithGoogle(t, dir)
	s.RequireAccountValidation = false
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	app := &App{Settings: s, DB: db}

	statePlain := "state-plain-ok"
	verifier := "verifier-ok"
	if err := database.InsertOAuthState(db, statePlain, database.OAuthPurposeLogin, sql.NullInt64{}, "/dashboard", verifier); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=abc&state="+statePlain, nil)
	rec := httptest.NewRecorder()
	app.HandleGoogleOAuthCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var sess *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" && c.Value != "" {
			sess = c
			break
		}
	}
	if sess == nil {
		t.Fatal("expected session cookie")
	}
}
