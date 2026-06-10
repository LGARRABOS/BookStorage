package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bookstorage/internal/config"
)

func TestWorkImageURL_rejectsUnsafeExternalSchemes(t *testing.T) {
	s := &config.Settings{
		UploadURLPath:        "images",
		ProfileUploadURLPath: "avatars",
	}

	cases := []struct {
		in   string
		want string
	}{
		{"https://cdn.example/cover.jpg", "https://cdn.example/cover.jpg"},
		{"http://cdn.example/cover.jpg", "http://cdn.example/cover.jpg"},
		{"data:image/png;base64,abc", ""},
		{"//cdn.example/cover.jpg", ""},
		{"javascript:alert(1)", ""},
		{"images/foo.jpg", "/static/images/foo.jpg"},
		{"/static/images/foo.jpg", "/static/images/foo.jpg"},
	}
	for _, tc := range cases {
		got := workImageURL(s, tc.in)
		if got != tc.want {
			t.Errorf("workImageURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRequireLogin_setsNoStoreOnAuthenticatedHTML(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	token := mustCreateSession(t, app, 1)

	handler := app.RequireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control %q, want no-store", cc)
	}
}

func TestRequireLogin_noStoreNotSetOnAPI(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	token := mustCreateSession(t, app, 1)

	handler := app.RequireLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/works", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "" {
		t.Fatalf("Cache-Control %q, want empty on API", cc)
	}
}
