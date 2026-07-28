package server

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestBdDashboardTemplateRendersSeries(t *testing.T) {
	fm := template.FuncMap{
		"work_image_url": func(s string) string { return s },
		"bd_album_title": bdAlbumTitle,
		"bd_dash_url":    bdDashboardURL,
		"url_for":        func(name string, args ...string) string { return "/" },
		"seq": func(n int) []int {
			out := make([]int, n)
			for i := range out {
				out[i] = i + 1
			}
			return out
		},
		"le":              func(a, b int) bool { return a <= b },
		"ge":              func(a, b int) bool { return a >= b },
		"divf":            func(a, b int) float64 { return 0 },
		"mulf":            func(a, b float64) float64 { return 0 },
		"t":               func(tr map[string]string, key string) string { return key },
		"jsstr":           func(s string) template.JS { return `""` },
		"translateStatus": func(s string, tr map[string]string) string { return s },
		"hasPrefix":       func(s, p string) bool { return false },
		"upper":           func(s string) string { return s },
		"join":            func(a []string, sep string) string { return "" },
		"int":             func(v int64) int { return int(v) },
		"fmtDateDisplay":  func(n nullFlexTime) string { return "" },
		"fmtDateInput":    func(n nullFlexTime) string { return "" },
		"fmtProbeTime":    func(s sql.NullString) string { return "" },
		"toJSON":          func(v any) template.JS { return `null` },
	}

	files := []string{
		filepath.Join("..", "..", "templates", "bd_dashboard.gohtml"),
		filepath.Join("..", "..", "templates", "shared", "bd_nav.gohtml"),
	}
	stubs := `{{define "site_head_icons"}}{{end}}{{define "site_brand_dashboard"}}{{end}}{{define "nav_settings_dropdown"}}{{end}}`
	root := template.New("root").Funcs(fm)
	if _, err := root.Parse(stubs); err != nil {
		t.Fatal(err)
	}
	tpl, err := root.ParseFiles(files...)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	works := make([]bdWorkRow, 0, 50)
	for i := 1; i <= 40; i++ {
		works = append(works, bdWorkRow{
			ID: i, Title: fmt.Sprintf("Aldébaran — Livre %d", i), Tome: i,
			Status:    sql.NullString{String: "Terminé", Valid: true},
			BdType:    sql.NullString{String: "Album", Valid: true},
			ImagePath: sql.NullString{String: "https://example.com/c.jpg", Valid: true},
		})
	}
	for i := 1; i <= 10; i++ {
		works = append(works, bdWorkRow{
			ID: 100 + i, Title: fmt.Sprintf("Thorgal — T%d", i), Tome: i,
			Status: sql.NullString{String: "Terminé", Valid: true},
		})
	}
	series := groupBdWorksBySeries(works)
	sortBdSeriesCards(series, "title")

	data := map[string]any{
		"Works":        []bdWorkRow(nil),
		"Series":       series,
		"ActiveSeries": "",
		"SortBy":       "title",
		"AdultFilter":  "",
		"SearchQuery":  "",
		"Lang":         "fr",
		"T":            map[string]string{},
		"CSPNonce":     "test",
		"BdTypes":      bdTypes,
		"BdStatuses":   bdStatuses,
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "bd_dashboard", data); err != nil {
		t.Fatalf("execute series list: %v", err)
	}
	if buf.Len() < 100 || !bytes.Contains(buf.Bytes(), []byte("Aldébaran")) {
		t.Fatalf("short/unexpected body len=%d", buf.Len())
	}

	buf.Reset()
	data["ActiveSeries"] = "Aldébaran"
	data["Works"] = series[0].Albums
	data["Series"] = []bdSeriesCard(nil)
	if err := tpl.ExecuteTemplate(&buf, "bd_dashboard", data); err != nil {
		t.Fatalf("execute series detail: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Livre")) {
		t.Fatalf("detail missing albums, body=%s", buf.String()[:min(200, buf.Len())])
	}
}

func TestHandleBdDashboard_RendersHTMLNotEmpty(t *testing.T) {
	db, s := openTestDB(t)
	// Use real templates from repo when available via NewApp-like load is heavy;
	// at least ensure handler + stub template produce non-empty output.
	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"bd_album_title":  bdAlbumTitle,
		"bd_dash_url":     bdDashboardURL,
		"t":               func(tr map[string]string, key string) string { return key },
		"work_image_url":  func(s string) string { return s },
		"jsstr":           func(s string) template.JS { return `""` },
		"seq":             func(n int) []int { return []int{1, 2, 3, 4, 5} },
		"ge":              func(a, b int) bool { return a >= b },
		"translateStatus": func(s string, tr map[string]string) string { return s },
	}).Parse(`
{{ define "bd_dashboard" }}<!DOCTYPE html><html><body>
{{ if .ActiveSeries }}DETAIL:{{ .ActiveSeries }}:{{ len .Works }}
{{ else }}{{ range .Series }}SERIES:{{ .Name }}:{{ .AlbumCount }};{{ end }}{{ end }}
</body></html>{{ end }}
{{ define "mobile_bd_dashboard" }}{{ template "bd_dashboard" . }}{{ end }}
`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}
	_, err := db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, user_id, updated_at) VALUES
		 ('Aldébaran — A', 1, 'Terminé', 'Album', 1, CURRENT_TIMESTAMP),
		 ('Aldébaran — B', 2, 'Terminé', 'Album', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/bd/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleBdDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" || !bytes.Contains(rec.Body.Bytes(), []byte("SERIES:Aldébaran:2")) {
		t.Fatalf("body=%q", body)
	}
}
