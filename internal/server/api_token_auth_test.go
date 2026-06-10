package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveAPIToken_rejectsExpiredToken(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	raw, _, err := app.createAPIToken(1, "test", []string{ScopeWorksRead})
	if err != nil {
		t.Fatal(err)
	}
	hash := hashAPIToken(raw)
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := db.Exec(`UPDATE api_tokens SET expires_at = ? WHERE token_hash = ?`, past, hash); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/works", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	_, _, ok := app.resolveAPIToken(req)
	if ok {
		t.Fatal("expected expired API token to be rejected")
	}
}

func TestCreateAPIToken_setsDefaultExpiry(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, row, err := app.createAPIToken(1, "ttl-test", []string{ScopeWorksRead})
	if err != nil {
		t.Fatal(err)
	}
	if !row.ExpiresAt.Valid {
		t.Fatal("expected expires_at on new token")
	}
	wantMin := time.Now().UTC().Add(defaultAPITokenTTL - time.Minute)
	if row.ExpiresAt.Time.Before(wantMin) {
		t.Fatalf("expires_at %v too soon (want ~%v)", row.ExpiresAt.Time, defaultAPITokenTTL)
	}
}
