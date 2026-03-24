package server

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
)

func testSettings(dir string) *config.Settings {
	return &config.Settings{
		Database:             filepath.Join(dir, "db.sqlite"),
		SecretKey:            "0123456789abcdef0123456789abcdef",
		Environment:          "development",
		SuperadminUsername:   "admin",
		SuperadminPassword:   "TestAdmin!99",
		DataDirectory:        dir,
		UploadFolder:         filepath.Join(dir, "img"),
		ProfileUploadFolder:  filepath.Join(dir, "av"),
		UploadURLPath:        "images",
		ProfileUploadURLPath: "avatars",
	}
}

func openTestDB(t *testing.T) (*sql.DB, *config.Settings) {
	t.Helper()
	dir := t.TempDir()
	s := testSettings(dir)
	db, err := database.Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	return db, s
}

func TestVerifySession(t *testing.T) {
	s := &config.Settings{
		SecretKey:   "0123456789abcdef0123456789abcdef",
		Environment: "development",
	}
	app := &App{Settings: s}
	tok := app.signSession(42, time.Now().Add(time.Hour).Unix())
	id, ok := app.verifySession(tok)
	if !ok || id != 42 {
		t.Fatalf("verifySession got id=%d ok=%v", id, ok)
	}
	tok2 := app.signSession(1, time.Now().Add(-time.Hour).Unix())
	_, ok2 := app.verifySession(tok2)
	if ok2 {
		t.Fatal("expected expired session")
	}
}

func TestHandleExportJSON(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at, is_adult)
		 VALUES ('Alpha', 3, 'https://x.test', 'En cours', 'Manga', 4, 'note', 1, CURRENT_TIMESTAMP, 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/export?format=json", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleExport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if int(payload["export_version"].(float64)) != ExportFormatVersion {
		t.Fatalf("export_version: %v", payload["export_version"])
	}
}

func TestHandleExportCSV(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type) VALUES ('Beta', 1, 1, 'En cours', 'Roman')`,
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleExport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
		t.Fatal("expected UTF-8 BOM")
	}
	r := csv.NewReader(bytes.NewReader(body[3:]))
	r.Comma = ';'
	row, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(row) < 10 || row[0] != "Title" {
		t.Fatalf("header: %v", row)
	}
}

func TestHandleImportCSV(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "t.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "Title;Chapter;Link;Status;Type;Rating;Notes\nImported;5;;En cours;Manga;0;\n")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "import_report=") {
		t.Fatalf("expected import_report in redirect: %s", loc)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM works WHERE title = 'Imported'`).Scan(&count)
	if count != 1 {
		t.Fatalf("works count: %d", count)
	}
}
