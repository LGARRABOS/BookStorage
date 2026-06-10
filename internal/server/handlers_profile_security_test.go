package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHandleProfile_passwordChangeRevokesOtherSessions(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("OldPass!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password, validated, is_admin, email) VALUES (10, 'pwuser', ?, 1, 0, 'pw@example.com')`,
		hashed,
	); err != nil {
		t.Fatal(err)
	}

	oldSession := mustCreateSession(t, app, 10)
	otherSession := mustCreateSession(t, app, 10)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("username", "pwuser")
	_ = mw.WriteField("email", "pw@example.com")
	_ = mw.WriteField("current_password", "OldPass!99")
	_ = mw.WriteField("new_password", "NewPass!88")
	_ = mw.WriteField("confirm_password", "NewPass!88")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/profile", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: oldSession})
	rec := httptest.NewRecorder()
	app.HandleProfile(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var newCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			newCookie = c
			break
		}
	}
	if newCookie == nil {
		t.Fatal("expected new session cookie after password change")
	}

	reqOld := httptest.NewRequest(http.MethodGet, "/profile", nil)
	reqOld.AddCookie(&http.Cookie{Name: "session", Value: oldSession})
	if _, _, ok := app.currentSession(reqOld); ok {
		t.Fatal("old session should be revoked")
	}
	reqOther := httptest.NewRequest(http.MethodGet, "/profile", nil)
	reqOther.AddCookie(&http.Cookie{Name: "session", Value: otherSession})
	if _, _, ok := app.currentSession(reqOther); ok {
		t.Fatal("other session should be revoked")
	}
	reqNew := httptest.NewRequest(http.MethodGet, "/profile", nil)
	reqNew.AddCookie(newCookie)
	if uid, _, ok := app.currentSession(reqNew); !ok || uid != 10 {
		t.Fatalf("new session invalid uid=%d ok=%v", uid, ok)
	}
}

func TestHandleDeleteProfile_blocksAPITokenAuth(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	hashed, err := hashPassword("DeleteMe!99")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password, validated, is_admin, email) VALUES (11, 'deluser', ?, 1, 0, 'del@example.com')`,
		hashed,
	); err != nil {
		t.Fatal(err)
	}

	raw, _, err := app.createAPIToken(11, "del-token", []string{ScopeWorksRead})
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("confirm_delete", "SUPPRIMER")
	form.Set("current_password", "DeleteMe!99")

	req := httptest.NewRequest(http.MethodPost, "/profile/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()

	handler := app.WithAPITokenContext(http.HandlerFunc(app.HandleDeleteProfile))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/profile" {
		t.Fatalf("expected redirect to profile, got %q", loc)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 11`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("user should not be deleted via API token, count=%d", count)
	}
}
