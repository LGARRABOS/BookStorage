package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestHandleHome_RedirectsToPreferredSection(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	if _, err := db.Exec(`UPDATE users SET home_section = 'library' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleHome(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != pathLibraryHome {
		t.Fatalf("Location=%q want %q", loc, pathLibraryHome)
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
