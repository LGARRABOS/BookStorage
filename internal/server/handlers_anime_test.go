package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"bookstorage/internal/config"
)

func TestHandleAnimeDashboard_AdultFilterAndRender(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(`
{{ define "anime_dashboard" }}{{ range .Works }}{{ .Title }}:{{ .Episode }}
{{ end }}{{ end }}
{{ define "mobile_anime_dashboard" }}{{ range .Works }}{{ .Title }}:{{ .Episode }}
{{ end }}{{ end }}
`))
	app := &App{
		Settings:        s,
		SiteConfig:      config.DefaultSiteConfig(),
		DB:              db,
		TemplatesWeb:    tpl,
		TemplatesMobile: tpl,
	}

	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, is_adult, user_id, updated_at)
		 VALUES
		 ('Safe Anime', 5, 'En cours', 'TV', 0, 1, CURRENT_TIMESTAMP),
		 ('Adult Anime', 3, 'En cours', 'TV', 1, 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/anime/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Safe Anime:5") {
		t.Fatalf("expected non-adult anime visible, body=%s", body)
	}
	if strings.Contains(body, "Adult Anime") {
		t.Fatalf("expected adult anime hidden by default, body=%s", body)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/anime/dashboard?adult=only", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec2 := httptest.NewRecorder()
	app.HandleAnimeDashboard(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status %d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if strings.Contains(body2, "Safe Anime") {
		t.Fatalf("expected non-adult anime hidden when adult=only, body=%s", body2)
	}
	if !strings.Contains(body2, "Adult Anime:3") {
		t.Fatalf("expected adult anime visible when adult=only, body=%s", body2)
	}
}

func TestHandleAnimeIncrementDecrement(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	res, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, user_id, updated_at)
		 VALUES ('Counter', 0, 'En cours', 'TV', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)
	path := "/api/anime/increment/" + idStr

	doPost := func(handler http.HandlerFunc, url string) {
		req := httptest.NewRequest(http.MethodPost, url, nil)
		req.SetPathValue("id", idStr)
		req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d for %s", rec.Code, url)
		}
	}

	doPost(app.HandleAnimeIncrement, path)
	doPost(app.HandleAnimeIncrement, path)
	var ep int
	_ = db.QueryRow(`SELECT episode FROM anime_works WHERE id = ?`, id).Scan(&ep)
	if ep != 2 {
		t.Fatalf("expected episode=2 after 2 increments, got %d", ep)
	}

	doPost(app.HandleAnimeDecrement, "/api/anime/decrement/"+idStr)
	_ = db.QueryRow(`SELECT episode FROM anime_works WHERE id = ?`, id).Scan(&ep)
	if ep != 1 {
		t.Fatalf("expected episode=1 after decrement, got %d", ep)
	}
}
