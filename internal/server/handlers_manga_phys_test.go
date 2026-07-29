package server

import (
	"bytes"
	"encoding/json"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bookstorage/internal/database"
)

func TestMangaPhysCRUD_AddListDelete(t *testing.T) {
	db, s := openTestDB(t)
	tpl := template.Must(template.New("").Funcs(template.FuncMap{
		"t":                   func(m map[string]string, k string) string { return k },
		"translateStatus":     func(st string, m map[string]string) string { return st },
		"jsstr":               func(s string) template.JSStr { return template.JSStr(s) },
		"manga_phys_dash_url": mangaPhysDashboardURL,
		"bd_dash_url":         bdDashboardURL,
	}).Parse(`
		{{ define "manga_phys_dashboard" }}DASH={{ len .Series }}{{ end }}
		{{ define "mobile_manga_phys_dashboard" }}{{ template "manga_phys_dashboard" . }}{{ end }}
		{{ define "manga_phys_topbar" }}{{ end }}
		{{ define "add_manga_phys" }}ADD{{ end }}
		{{ define "mobile_add_manga_phys" }}{{ template "add_manga_phys" . }}{{ end }}
	`))
	app := &App{Settings: s, DB: db, TemplatesWeb: tpl, TemplatesMobile: tpl}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("title", "Naruto — Tome 1")
	_ = w.WriteField("tome", "1")
	_ = w.WriteField("status", "À lire")
	_ = w.WriteField("manga_type", "Manga")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, pathMangaPhysAddWork, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleMangaPhysAddWork(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("add status=%d body=%s", rec.Code, rec.Body.String())
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM manga_phys_works WHERE user_id = 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}

	reqD := httptest.NewRequest(http.MethodGet, pathMangaPhysDashboard, nil)
	reqD.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	recD := httptest.NewRecorder()
	app.HandleMangaPhysDashboard(recD, reqD)
	if recD.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", recD.Code, recD.Body.String())
	}
	if !bytes.Contains(recD.Body.Bytes(), []byte("DASH=1")) {
		t.Fatalf("dashboard body=%s", recD.Body.String())
	}

	var id int
	if err := db.QueryRow(`SELECT id FROM manga_phys_works WHERE user_id = 1 LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	reqDel := httptest.NewRequest(http.MethodPost, "/api/manga-phys/delete/"+strconv.Itoa(id), nil)
	reqDel.SetPathValue("id", strconv.Itoa(id))
	reqDel.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	recDel := httptest.NewRecorder()
	app.HandleMangaPhysDeleteAPI(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", recDel.Code, recDel.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recDel.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("delete payload=%v", payload)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM manga_phys_works WHERE user_id = 1`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("after delete count=%d err=%v", n, err)
	}
}

func TestMigration26_CleansOrphanMangaPlacementsOnce(t *testing.T) {
	db, s := openTestDB(t)
	res, err := db.Exec(`INSERT INTO library_furniture (user_id, name, room_label, sort_order) VALUES (1, 'Salon', 'Salon', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	fid, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO library_shelves (furniture_id, label, case_count, books_per_case, sort_order) VALUES (?, 'A', 5, 8, 0)`, fid)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := res.LastInsertId()

	// Re-run migration 26: drop marker, insert a legacy orphan placement, EnsureSchema again.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 26`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		`INSERT INTO library_placements (user_id, shelf_id, case_num, position, media_kind, work_id, volume)
		 VALUES (1, ?, 1, 1, 'manga', 999, 1)`, sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = db.QueryRow(`SELECT COUNT(*) FROM library_placements WHERE media_kind = 'manga'`).Scan(&after)
	if after != 0 {
		t.Fatalf("expected manga placements cleaned by migration 26, got %d", after)
	}

	// New physical placements must survive subsequent EnsureSchema calls.
	_, err = db.Exec(
		`INSERT INTO manga_phys_works (title, tome, status, manga_type, user_id, updated_at)
		 VALUES ('Keep Me', 1, 'À lire', 'Manga', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		`INSERT INTO library_placements (user_id, shelf_id, case_num, position, media_kind, work_id, volume)
		 VALUES (1, ?, 1, 1, 'manga', 1, 1)`, sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM library_placements WHERE media_kind = 'manga'`).Scan(&after)
	if after != 1 {
		t.Fatalf("expected physical manga placement kept, got %d", after)
	}
}
