package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleBdDashboard_AdultFilterAndRender(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(`
{{ define "bd_dashboard" }}{{ if .ActiveSeries }}SERIES={{ .ActiveSeries }}
{{ range .Works }}{{ .Title }}:{{ .Tome }}
{{ end }}{{ else }}{{ range .Series }}{{ .Name }}:{{ .AlbumCount }}
{{ end }}{{ end }}Adult={{ .AdultFilter }}{{ end }}
{{ define "mobile_bd_dashboard" }}{{ if .ActiveSeries }}{{ range .Works }}{{ .Title }}
{{ end }}{{ else }}{{ range .Series }}{{ .Name }}
{{ end }}{{ end }}{{ end }}
`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}

	_, err := db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, is_adult, user_id, updated_at)
		 VALUES ('Tintin', 1, 'En cours', 'Album', 0, 1, CURRENT_TIMESTAMP),
		        ('Adult BD', 2, 'En cours', 'Album', 1, 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/bd/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleBdDashboard(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Tintin") || strings.Contains(body, "Adult BD") {
		t.Fatalf("default should hide adult, body=%s", body)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/bd/dashboard?adult=only", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec2 := httptest.NewRecorder()
	app.HandleBdDashboard(rec2, req2)
	body2 := rec2.Body.String()
	if strings.Contains(body2, "Tintin") {
		t.Fatalf("expected non-adult hidden when adult=only, body=%s", body2)
	}
	if !strings.Contains(body2, "Adult BD") {
		t.Fatalf("expected adult visible when adult=only, body=%s", body2)
	}

	_, err = db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, is_adult, user_id, updated_at)
		 VALUES ('Aldébaran — La catastrophe', 1, 'Terminé', 'Album', 0, 1, CURRENT_TIMESTAMP),
		        ('Aldébaran — La blonde', 2, 'Terminé', 'Album', 0, 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/bd/dashboard?series=Ald%C3%A9baran", nil)
	req3.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec3 := httptest.NewRecorder()
	app.HandleBdDashboard(rec3, req3)
	body3 := rec3.Body.String()
	if !strings.Contains(body3, "SERIES=Aldébaran") || !strings.Contains(body3, "La blonde") {
		t.Fatalf("series drill-down body=%s", body3)
	}
}

func TestImportExportBdCSVRoundTrip(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	report := ImportReport{}
	tot := 5
	app.importOneBdWork(1, 1, exportBdWork{
		Title:      "Spirou",
		Tome:       2,
		TotalTomes: &tot,
		Status:     "En cours",
		BdType:     "Série",
		Rating:     4,
		Source:     "manual",
	}, DuplicateUpdate, &report)
	if report.Imported != 1 {
		t.Fatalf("import report=%+v", report)
	}

	var title string
	var tome, rating int
	err := db.QueryRow(`SELECT title, tome, rating FROM bd_works WHERE user_id = 1`).Scan(&title, &tome, &rating)
	if err != nil || title != "Spirou" || tome != 2 || rating != 4 {
		t.Fatalf("row title=%s tome=%d rating=%d err=%v", title, tome, rating, err)
	}
}
