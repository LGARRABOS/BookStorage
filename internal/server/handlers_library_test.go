package server

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestFormatPlacementCode(t *testing.T) {
	if got := formatPlacementCode("a", 5, 6); got != "A5-6" {
		t.Fatalf("formatPlacementCode = %q, want A5-6", got)
	}
	label, caseNum, pos, ok := parsePlacementCode("A5-6")
	if !ok || label != "A" || caseNum != 5 || pos != 6 {
		t.Fatalf("parsePlacementCode = %s %d %d ok=%v", label, caseNum, pos, ok)
	}
	if _, _, _, ok := parsePlacementCode("bad"); ok {
		t.Fatal("expected parse failure")
	}
}

func TestLibraryPlacementsAPI_CreateSearchUnique(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Parse(`{{ define "library_home" }}ok{{ end }}`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}

	_, err := db.Exec(`INSERT INTO manga_phys_works (title, tome, status, manga_type, user_id, updated_at) VALUES ('One Piece', 15, 'En cours', 'Manga', 1, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO bd_works (title, tome, status, bd_type, user_id, updated_at) VALUES ('Lucky Luke', 12, 'Terminé', 'Album', 1, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}

	res, err := db.Exec(`INSERT INTO library_furniture (user_id, name, room_label, sort_order) VALUES (1, 'Salon', 'Salon', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	fid, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO library_shelves (furniture_id, label, case_count, books_per_case, sort_order) VALUES (?, 'A', 10, 8, 0)`, fid)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := res.LastInsertId()

	body := map[string]any{
		"shelf_id": sid, "case_num": 5, "media_kind": "manga", "work_id": 1, "volume": 15,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/library/placements", bytes.NewReader(b))
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.HandleAPILibraryPlacementsCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	placement, _ := created["placement"].(map[string]any)
	if placement["code"] != "A5-1" {
		t.Fatalf("code=%v want A5-1", placement["code"])
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/library/placements", bytes.NewReader(b))
	req2.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	app.HandleAPILibraryPlacementsCreate(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("dup status=%d", rec2.Code)
	}

	bdBody, _ := json.Marshal(map[string]any{
		"shelf_id": sid, "case_num": 5, "media_kind": "bd", "work_id": 1,
	})
	req3 := httptest.NewRequest(http.MethodPost, "/api/library/placements", bytes.NewReader(bdBody))
	req3.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	req3.Header.Set("Content-Type", "application/json")
	rec3 := httptest.NewRecorder()
	app.HandleAPILibraryPlacementsCreate(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("bd create status=%d body=%s", rec3.Code, rec3.Body.String())
	}
	var bdCreated map[string]any
	_ = json.Unmarshal(rec3.Body.Bytes(), &bdCreated)
	bdPlacement, _ := bdCreated["placement"].(map[string]any)
	if bdPlacement["code"] != "A5-2" {
		t.Fatalf("bd code=%v want A5-2", bdPlacement["code"])
	}
	if int(bdPlacement["volume"].(float64)) != 12 {
		t.Fatalf("bd volume=%v want 12", bdPlacement["volume"])
	}

	req4 := httptest.NewRequest(http.MethodGet, "/api/library/search?q=Lucky", nil)
	req4.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec4 := httptest.NewRecorder()
	app.HandleAPILibrarySearch(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("search status=%d", rec4.Code)
	}
	if !strings.Contains(rec4.Body.String(), "Lucky Luke") || !strings.Contains(rec4.Body.String(), "A5-2") {
		t.Fatalf("search body=%s", rec4.Body.String())
	}

	id := int(placement["id"].(float64))
	moveBody, _ := json.Marshal(map[string]any{"move": "down"})
	req5 := httptest.NewRequest(http.MethodPost, "/api/library/placements/"+strconv.Itoa(id), bytes.NewReader(moveBody))
	req5.SetPathValue("id", strconv.Itoa(id))
	req5.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	req5.Header.Set("Content-Type", "application/json")
	rec5 := httptest.NewRecorder()
	app.HandleAPILibraryPlacementsPatch(rec5, req5)
	if rec5.Code != http.StatusOK {
		t.Fatalf("move status=%d body=%s", rec5.Code, rec5.Body.String())
	}

	req6 := httptest.NewRequest(http.MethodGet, "/api/library/furniture/"+strconv.Itoa(int(fid)), nil)
	req6.SetPathValue("id", strconv.Itoa(int(fid)))
	req6.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec6 := httptest.NewRecorder()
	app.HandleAPILibraryFurniture(rec6, req6)
	if rec6.Code != http.StatusOK {
		t.Fatalf("furniture api status=%d", rec6.Code)
	}
	if !strings.Contains(rec6.Body.String(), `"label":"A"`) {
		t.Fatalf("furniture body=%s", rec6.Body.String())
	}
}

func TestHandleLibraryHome_Render(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"t": func(m map[string]string, k string) string { return k },
	}).Parse(`{{ define "library_home" }}HOME={{ len .FurnitureList }}{{ end }}{{ define "mobile_library_home" }}{{ template "library_home" . }}{{ end }}{{ define "library_topbar" }}{{ end }}`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}
	_, _ = db.Exec(`INSERT INTO library_furniture (user_id, name, sort_order) VALUES (1, 'Bureau', 0)`)

	req := httptest.NewRequest(http.MethodGet, "/library/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleLibraryHome(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "HOME=1") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

// Regression: topbar must not read .Furniture.ID when home passes only FurnitureList
// (a slice has no ID field and used to 500 /library/).
func TestLibraryTopbar_HomeDataNoPanic(t *testing.T) {
	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"t": func(m map[string]string, k string) string { return k },
	}).Parse(`
		{{ define "library_topbar" }}
		{{ if .Furniture }}{{ with .Furniture }}{{ if gt .ID 0 }}VIEW{{ end }}{{ end }}{{ end }}
		LIST={{ len .FurnitureList }}
		{{ end }}
	`))
	var buf strings.Builder
	err := tpl.ExecuteTemplate(&buf, "library_topbar", map[string]any{
		"FurnitureList": []libraryFurnitureRow{{ID: 1, Name: "Bureau"}},
		"T":             map[string]string{},
		"CurrentPath":   "/library/",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "LIST=1") {
		t.Fatalf("body=%s", buf.String())
	}
	if strings.Contains(buf.String(), "VIEW") {
		t.Fatalf("view link should be hidden on home: %s", buf.String())
	}
}

func TestLibraryUnassignedListsMissing(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"t": func(m map[string]string, k string) string { return k },
	}).Parse(`{{ define "library_unassigned" }}M={{ len .Manga }}B={{ len .Bd }}{{ end }}{{ define "mobile_library_unassigned" }}{{ template "library_unassigned" . }}{{ end }}{{ define "library_topbar" }}{{ end }}`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}
	_, _ = db.Exec(`INSERT INTO manga_phys_works (title, tome, status, manga_type, user_id, updated_at) VALUES ('Unplaced', 1, 'À lire', 'Manga', 1, CURRENT_TIMESTAMP)`)
	_, _ = db.Exec(`INSERT INTO bd_works (title, tome, status, bd_type, user_id, updated_at) VALUES ('Unplaced BD', 1, 'À lire', 'Album', 1, CURRENT_TIMESTAMP)`)

	req := httptest.NewRequest(http.MethodGet, "/library/unassigned", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleLibraryUnassigned(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "M=1") || !strings.Contains(rec.Body.String(), "B=1") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
