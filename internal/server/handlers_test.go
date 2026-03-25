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
	"strconv"
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

func TestHandleAPIWorksList_WithFiltersAndMeta(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at)
		 VALUES
		 ('Alpha', 3, NULL, 'En cours', 'Manga', 4, 'note alpha', 1, CURRENT_TIMESTAMP),
		 ('Bravo', 10, NULL, 'Terminé', 'Roman', 5, 'note bravo', 1, CURRENT_TIMESTAMP),
		 ('Charlie', 2, NULL, 'En cours', 'Manga', 2, 'note charlie', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/works?status=En%20cours&reading_type=Manga&search=char&page=1&limit=5&sort=title_asc", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleAPIWorksList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var payload struct {
		Data []apiWork `json:"data"`
		Meta struct {
			Page      int  `json:"page"`
			Limit     int  `json:"limit"`
			Total     int  `json:"total"`
			HasNext   bool `json:"has_next"`
			HasPrev   bool `json:"has_prev"`
			TotalPage int  `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Meta.Page != 1 || payload.Meta.Limit != 5 {
		t.Fatalf("meta pagination invalide: %+v", payload.Meta)
	}
	if payload.Meta.Total != 1 || payload.Meta.TotalPage != 1 || payload.Meta.HasNext || payload.Meta.HasPrev {
		t.Fatalf("meta total invalide: %+v", payload.Meta)
	}
	if len(payload.Data) != 1 || payload.Data[0].Title != "Charlie" {
		t.Fatalf("résultat inattendu: %+v", payload.Data)
	}
}

func TestImportFromJSON_AniListExport(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	anilistPayload := `{
		"lists": [{
			"entries": [{
				"status": "CURRENT",
				"progress": 17,
				"score": 4,
				"notes": "from anilist",
				"media": {
					"id": 12345,
					"title": {"romaji":"Dandadan"},
					"format": "MANGA",
					"isAdult": false,
					"coverImage": {"large":"https://cdn.test/cover.jpg"}
				}
			}]
		}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/import?duplicate_mode=skip", strings.NewReader(anilistPayload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var (
		title string
		ch    int
		link  sql.NullString
	)
	err := db.QueryRow(`SELECT title, chapter, link FROM works WHERE user_id = 1 LIMIT 1`).Scan(&title, &ch, &link)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Dandadan" || ch != 17 {
		t.Fatalf("work importée inattendue: title=%s chapter=%d", title, ch)
	}
	if !link.Valid || !strings.Contains(link.String, "/12345") {
		t.Fatalf("lien AniList manquant: %v", link.String)
	}
}

func TestImportFromCSV_MAL(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "mal.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "series_title;series_type;my_status;my_read_chapters;my_score\nBerserk;Manga;reading;42;5\n")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	app.HandleImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var gotTitle, gotStatus, gotType string
	var gotChapter, gotRating int
	err = db.QueryRow(`SELECT title, status, reading_type, chapter, rating FROM works WHERE user_id = 1 LIMIT 1`).
		Scan(&gotTitle, &gotStatus, &gotType, &gotChapter, &gotRating)
	if err != nil {
		t.Fatal(err)
	}
	if gotTitle != "Berserk" || gotStatus != "En cours" || gotType != "Manga" || gotChapter != 42 || gotRating != 5 {
		t.Fatalf("work MAL inattendue: %s %s %s %d %d", gotTitle, gotStatus, gotType, gotChapter, gotRating)
	}
}

func TestWithRequestPolicies_CSRFAndRateLimit(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := app.WithRequestPolicies(next)

	// CSRF: requête mutatrice avec session mais sans Origin/Referer doit être bloquée.
	req := httptest.NewRequest(http.MethodPost, "/api/works", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:9000"
	req.AddCookie(&http.Cookie{Name: "session", Value: app.signSession(1, time.Now().Add(time.Hour).Unix())})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("csrf attendu 403, obtenu %d", rec.Code)
	}

	// Rate-limit login: plusieurs tentatives depuis la même IP doivent finir en 429.
	got429 := false
	for i := 0; i < 20; i++ {
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=u&password=p"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://example.test")
		r.Host = "example.test"
		r.RemoteAddr = "10.0.0.1:" + strconv.Itoa(8000+i)
		wr := httptest.NewRecorder()
		handler.ServeHTTP(wr, r)
		if wr.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("rate limiting attendu mais non observé")
	}
}
