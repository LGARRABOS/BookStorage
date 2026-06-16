package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAPIReadingSitesList(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	if _, err := db.Exec(
		`INSERT INTO reading_sites (user_id, name, base_url, probe_status, probe_http_status) VALUES (1, 'ScanFR', 'https://scan.example', 'up', 200)`,
	); err != nil {
		t.Fatal(err)
	}

	raw, _, err := app.createAPIToken(1, "test", []string{ScopeWorksRead})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reading-sites", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	handler := app.WithAPITokenContext(http.HandlerFunc(app.HandleAPIReadingSitesList))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data []apiReadingSite `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("sites=%d want 1", len(payload.Data))
	}
	if payload.Data[0].Name != "ScanFR" || payload.Data[0].ProbeStatus != "up" {
		t.Fatalf("unexpected site: %+v", payload.Data[0])
	}
	if payload.Data[0].ProbeHTTPStatus == nil || *payload.Data[0].ProbeHTTPStatus != 200 {
		t.Fatalf("probe_http_status=%v want 200", payload.Data[0].ProbeHTTPStatus)
	}
}

func TestAPIWorksDetail_includesLinkStatus(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	session := mustCreateSession(t, app, 1)

	_, err := db.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, link_probe_status, updated_at)
		 VALUES ('Linked', 3, 'https://read.example/manga', 'En cours', 'Manga', 0, '', 1, 'up', datetime('now'))`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/works/1", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session})
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	app.HandleAPIWorksDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data apiWork `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.LinkStatus != "up" {
		t.Fatalf("link_status=%q want up", payload.Data.LinkStatus)
	}
}
