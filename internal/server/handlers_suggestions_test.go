package server

import (
	"bytes"
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bookstorage/internal/catalog"
)

func TestHandleSuggestions_lanesAndAdultFlag(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(
		`{{ define "suggestions" }}lanes={{ range .SuggestionLanes }}{{ . }};{{ end }}adult={{ .HasAdultWorks }}{{ end }}`,
	))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}
	session := mustCreateSession(t, app, 1)

	render := func() string {
		req := httptest.NewRequest(http.MethodGet, pathMangaSuggestions, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session})
		rec := httptest.NewRecorder()
		app.HandleSuggestions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := render(); body != "lanes=sfw;adult;adult=false" {
		t.Fatalf("sfw-only library: body=%q", body)
	}

	if _, err := db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type, is_adult, updated_at)
		 VALUES ('Adult', 1, 1, 'En cours', 'Webtoon', 1, CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatal(err)
	}
	if body := render(); body != "lanes=sfw;adult;adult=true" {
		t.Fatalf("adult library: body=%q", body)
	}
}

func TestSuggestionsTemplate_separatesAudiences(t *testing.T) {
	fm := template.FuncMap{
		"t":               func(tr map[string]string, key string) string { return key },
		"jsstr":           func(s string) template.JS { return `""` },
		"toJSON":          func(v any) template.JS { return `null` },
		"hasPrefix":       func(s, p string) bool { return false },
		"work_image_url":  func(s string) string { return s },
		"translateStatus": func(s string, tr map[string]string) string { return s },
		"fmtProbeTime":    func(s sql.NullString) string { return "" },
	}
	stubs := `{{define "site_head_icons"}}{{end}}{{define "site_brand_dashboard"}}{{end}}` +
		`{{define "nav_more_menu"}}{{end}}{{define "nav_settings_dropdown"}}{{end}}` +
		`{{define "mobile_shell_head"}}{{end}}{{define "mobile_topbar"}}{{end}}` +
		`{{define "mobile_settings_sheet"}}{{end}}{{define "mobile_install_banner"}}{{end}}` +
		`{{define "mobile_bottom_nav"}}{{end}}{{define "mobile_shell_scripts"}}{{end}}`
	root := template.New("root").Funcs(fm)
	if _, err := root.Parse(stubs); err != nil {
		t.Fatal(err)
	}
	tpl, err := root.ParseFiles(
		filepath.Join("..", "..", "templates", "suggestions.gohtml"),
		filepath.Join("..", "..", "templates", "shared", "recommendations.gohtml"),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	data := map[string]any{
		"Lang":            "fr",
		"T":               map[string]string{},
		"CSPNonce":        "test",
		"IsMobileView":    false,
		"Genres":          catalog.AnilistGenres(),
		"ReadingTypes":    catalogBrowseReadingTypes,
		"CatalogSource":   "anilist",
		"SuggestionLanes": suggestionLanes,
		"HasAdultWorks":   false,
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "suggestions", data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body := buf.String()

	for _, want := range []string{
		`id="sugg-panel-sfw"`,
		`id="sugg-panel-adult"`,
		`id="sugg-grid-sfw"`,
		`id="sugg-grid-adult"`,
		`id="reco-section"`,
		`id="reco-section-adult"`,
		`name="sugg-orient-adult"`,
		"suggestions.adult.no_library",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in rendered page", want)
		}
	}
	// The adult panel must start collapsed and never expose its 18+ filters to
	// the all-ages lane.
	if !strings.Contains(body, `data-sugg-panel="adult" hidden`) {
		t.Fatal("adult panel should be hidden by default")
	}
	if strings.Contains(body, `name="sugg-orient-sfw"`) {
		t.Fatal("18+ orientation filters leaked into the all-ages lane")
	}

	data["HasAdultWorks"] = true
	buf.Reset()
	if err := tpl.ExecuteTemplate(&buf, "suggestions", data); err != nil {
		t.Fatalf("execute with adult library: %v", err)
	}
	if strings.Contains(buf.String(), "suggestions.adult.no_library") {
		t.Fatal("no_library hint should disappear once the library has 18+ works")
	}
}
