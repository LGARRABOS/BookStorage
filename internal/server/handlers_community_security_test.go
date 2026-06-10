package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHandleUserDetail_hidesAdultWorksFromVisitors(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(`
{{ define "user_detail" }}{{ range .Works }}{{ .Title }}
{{ end }}{{ end }}
{{ define "mobile_user_detail" }}{{ range .Works }}{{ .Title }}
{{ end }}{{ end }}
`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_public) VALUES ('publiclib', 'x', 1, 1)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var ownerID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'publiclib'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type, is_adult, updated_at)
		 VALUES
		 ('SafeWork', 1, ?, 'En cours', 'Manga', 0, CURRENT_TIMESTAMP),
		 ('AdultWork', 1, ?, 'En cours', 'Manga', 1, CURRENT_TIMESTAMP)`,
		ownerID, ownerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	visitorToken := mustCreateSession(t, app, 1)

	req := httptest.NewRequest(http.MethodGet, "/users/"+strconv.Itoa(ownerID), nil)
	req.SetPathValue("id", strconv.Itoa(ownerID))
	req.AddCookie(&http.Cookie{Name: "session", Value: visitorToken})
	rec := httptest.NewRecorder()
	app.HandleUserDetail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("visitor status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SafeWork") {
		t.Fatalf("expected safe work visible to visitor, body=%q", body)
	}
	if strings.Contains(body, "AdultWork") {
		t.Fatalf("expected adult work hidden from visitor, body=%q", body)
	}

	ownerToken := mustCreateSession(t, app, ownerID)
	reqOwn := httptest.NewRequest(http.MethodGet, "/users/"+strconv.Itoa(ownerID), nil)
	reqOwn.SetPathValue("id", strconv.Itoa(ownerID))
	reqOwn.AddCookie(&http.Cookie{Name: "session", Value: ownerToken})
	recOwn := httptest.NewRecorder()
	app.HandleUserDetail(recOwn, reqOwn)
	if recOwn.Code != http.StatusOK {
		t.Fatalf("owner status %d", recOwn.Code)
	}
	bodyOwn := recOwn.Body.String()
	if !strings.Contains(bodyOwn, "SafeWork") || !strings.Contains(bodyOwn, "AdultWork") {
		t.Fatalf("owner should see all works, body=%q", bodyOwn)
	}
}

func TestHandleImportWork_blocksAdultWork(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_public) VALUES ('donor', 'x', 1, 1)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	var donorID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'donor'`).Scan(&donorID); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type, is_adult, updated_at)
		 VALUES ('AdultOnly', 1, ?, 'En cours', 'Manga', 1, CURRENT_TIMESTAMP)`,
		donorID,
	)
	if err != nil {
		t.Fatal(err)
	}
	var workID int
	if err := db.QueryRow(`SELECT id FROM works WHERE title = 'AdultOnly'`).Scan(&workID); err != nil {
		t.Fatal(err)
	}

	visitorToken := mustCreateSession(t, app, 1)
	req := httptest.NewRequest(http.MethodPost, "/users/"+strconv.Itoa(donorID)+"/import/"+strconv.Itoa(workID), nil)
	req.SetPathValue("user_id", strconv.Itoa(donorID))
	req.SetPathValue("work_id", strconv.Itoa(workID))
	req.AddCookie(&http.Cookie{Name: "session", Value: visitorToken})
	rec := httptest.NewRecorder()
	app.HandleImportWork(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 for adult work import", rec.Code)
	}
}
