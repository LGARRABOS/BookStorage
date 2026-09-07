package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bookstorage/internal/config"
)

func TestNormalizeHomeSectionAndPath(t *testing.T) {
	if got := normalizeHomeSection("LIBRARY"); got != homeSectionLibrary {
		t.Fatalf("normalize=%q", got)
	}
	if got := normalizeHomeSection("nope"); got != homeSectionHub {
		t.Fatalf("fallback=%q", got)
	}
	if got := pathForHomeSection(homeSectionLibrary); got != pathLibraryHome {
		t.Fatalf("path=%q", got)
	}
	if got := pathForHomeSection(homeSectionMangaPhys); got != pathMangaPhysDashboard {
		t.Fatalf("path=%q", got)
	}
}

func TestHandleHome_DoesNotTrapPreferredSection(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(`
{{ define "hub" }}HUB{{ end }}
{{ define "mobile_hub" }}HUB{{ end }}
{{ define "landing" }}LANDING{{ end }}
`))
	app := &App{
		Settings:        s,
		SiteConfig:      config.DefaultSiteConfig(),
		DB:              db,
		TemplatesWeb:    tpl,
		TemplatesMobile: tpl,
	}
	if _, err := db.Exec(`UPDATE users SET home_section = 'manga' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	session := mustCreateSession(t, app, 1)

	reqHome := httptest.NewRequest(http.MethodGet, "/", nil)
	reqHome.AddCookie(&http.Cookie{Name: "session", Value: session})
	recHome := httptest.NewRecorder()
	app.HandleHome(recHome, reqHome)
	if recHome.Code != http.StatusOK {
		t.Fatalf("GET / status=%d want 200", recHome.Code)
	}
	if loc := recHome.Header().Get("Location"); loc != "" {
		t.Fatalf("GET / redirected to %q", loc)
	}
	if body := recHome.Body.String(); !strings.Contains(body, "HUB") {
		t.Fatalf("GET / body=%q", body)
	}

	reqHub := httptest.NewRequest(http.MethodGet, "/hub", nil)
	reqHub.AddCookie(&http.Cookie{Name: "session", Value: session})
	recHub := httptest.NewRecorder()
	app.HandleHub(recHub, reqHub)
	if recHub.Code != http.StatusOK {
		t.Fatalf("GET /hub status=%d want 200", recHub.Code)
	}
	if loc := recHub.Header().Get("Location"); loc != "" {
		t.Fatalf("GET /hub redirected to %q", loc)
	}
}

func TestResolvePostLoginRedirectForUser_UsesHomeSection(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	if _, err := db.Exec(`UPDATE users SET home_section = 'bd' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if got := app.resolvePostLoginRedirectForUser(1, ""); got != pathBdDashboard {
		t.Fatalf("empty next → %q", got)
	}
	if got := app.resolvePostLoginRedirectForUser(1, pathMangaDashboard); got != pathBdDashboard {
		t.Fatalf("section home → %q", got)
	}
	if got := app.resolvePostLoginRedirectForUser(1, "/manga/edit/42"); got != "/manga/edit/42" {
		t.Fatalf("deep link → %q", got)
	}
}
