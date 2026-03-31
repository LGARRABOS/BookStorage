package server

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"html/template"
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
	"bookstorage/internal/recommend"
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

func mustCreateSession(t *testing.T, app *App, userID int) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	token, err := app.createSession(req, userID)
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	return token
}

func TestSessions_CurrentSessionAndExpiry(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	token := mustCreateSession(t, app, 1)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	uid, _, ok := app.currentSession(req)
	if !ok || uid != 1 {
		t.Fatalf("currentSession uid=%d ok=%v", uid, ok)
	}

	_, _ = db.Exec(
		`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Add(-time.Minute),
		hashSessionToken(token),
	)
	uid2, _, ok2 := app.currentSession(req)
	if ok2 || uid2 != 0 {
		t.Fatalf("expected expired invalid, got uid=%d ok=%v", uid2, ok2)
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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

func TestHandleDashboard_AdultFilter(t *testing.T) {
	db, s := openTestDB(t)
	// Handler renders templates; for this test we only need deterministic output,
	// so we provide a tiny in-memory template set to avoid filesystem dependency.
	tpl := template.Must(template.New("").Parse(`
{{ define "dashboard" }}{{ range .Works }}{{ .Title }}
{{ end }}{{ end }}
{{ define "mobile_dashboard" }}{{ range .Works }}{{ .Title }}
{{ end }}{{ end }}
`))
	app := &App{
		Settings:        s,
		SiteConfig:      config.DefaultSiteConfig(),
		DB:              db,
		TemplatesWeb:    tpl,
		TemplatesMobile: tpl,
	}

	_, err := db.Exec(
		`INSERT INTO works (title, chapter, user_id, status, reading_type, is_adult, updated_at)
		 VALUES
		 ('Safe', 1, 1, 'En cours', 'Manga', 0, CURRENT_TIMESTAMP),
		 ('Adult', 1, 1, 'En cours', 'Manga', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Default: adult content is hidden
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Safe") {
		t.Fatalf("expected non-adult work visible, body=%s", body)
	}
	if strings.Contains(body, "Adult") {
		t.Fatalf("expected adult work hidden by default, body=%s", body)
	}

	// adult=only: show only adult works
	req2 := httptest.NewRequest(http.MethodGet, "/dashboard?adult=only", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec2 := httptest.NewRecorder()
	app.HandleDashboard(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status %d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if strings.Contains(body2, "Safe") {
		t.Fatalf("expected non-adult work hidden when adult=only, body=%s", body2)
	}
	if !strings.Contains(body2, "Adult") {
		t.Fatalf("expected adult work visible when adult=only, body=%s", body2)
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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

func TestAPIWorksCRUDFlow(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	session := mustCreateSession(t, app, 1)

	createReq := httptest.NewRequest(http.MethodPost, "/api/works", strings.NewReader(`{
		"title":"Flow Title",
		"chapter":7,
		"status":"En cours",
		"reading_type":"Manga",
		"rating":4,
		"notes":"flow notes"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: "session", Value: session})
	createRec := httptest.NewRecorder()
	app.HandleAPIWorksCreate(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data apiWork `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == 0 {
		t.Fatalf("expected created id, got %+v", created.Data)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/works/"+strconv.Itoa(created.Data.ID), strings.NewReader(`{
		"chapter": 12,
		"status": "Terminé",
		"rating": 5
	}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(&http.Cookie{Name: "session", Value: session})
	updateReq.SetPathValue("id", strconv.Itoa(created.Data.ID))
	updateRec := httptest.NewRecorder()
	app.HandleAPIWorksUpdate(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/works/"+strconv.Itoa(created.Data.ID), nil)
	detailReq.AddCookie(&http.Cookie{Name: "session", Value: session})
	detailReq.SetPathValue("id", strconv.Itoa(created.Data.ID))
	detailRec := httptest.NewRecorder()
	app.HandleAPIWorksDetail(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail struct {
		Data apiWork `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Data.Chapter != 12 || detail.Data.Status != "Terminé" || detail.Data.Rating != 5 {
		t.Fatalf("detail inattendu: %+v", detail.Data)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/works/"+strconv.Itoa(created.Data.ID), nil)
	deleteReq.AddCookie(&http.Cookie{Name: "session", Value: session})
	deleteReq.SetPathValue("id", strconv.Itoa(created.Data.ID))
	deleteRec := httptest.NewRecorder()
	app.HandleAPIWorksDelete(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	detailAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/works/"+strconv.Itoa(created.Data.ID), nil)
	detailAfterDeleteReq.AddCookie(&http.Cookie{Name: "session", Value: session})
	detailAfterDeleteReq.SetPathValue("id", strconv.Itoa(created.Data.ID))
	detailAfterDeleteRec := httptest.NewRecorder()
	app.HandleAPIWorksDetail(detailAfterDeleteRec, detailAfterDeleteReq)
	if detailAfterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", detailAfterDeleteRec.Code)
	}
}

func TestHandleAPIStats_UserScopedAggregates(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(`INSERT INTO users (id, username, password, validated) VALUES (2, 'other-user', 'x', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO works (title, chapter, status, reading_type, rating, user_id, updated_at)
		 VALUES
		 ('A', 10, 'En cours', 'Manga', 4, 1, CURRENT_TIMESTAMP),
		 ('B', 5, 'Terminé', 'Roman', 2, 1, CURRENT_TIMESTAMP),
		 ('C', 99, 'En cours', 'Manga', 5, 2, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAPIStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	var payload struct {
		Data struct {
			TotalWorks    int     `json:"total_works"`
			TotalChapters int     `json:"total_chapters"`
			AvgRating     float64 `json:"avg_rating"`
			RatedCount    int     `json:"rated_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.TotalWorks != 2 || payload.Data.TotalChapters != 15 || payload.Data.RatedCount != 2 {
		t.Fatalf("stats inattendues: %+v", payload.Data)
	}
	if payload.Data.AvgRating < 2.99 || payload.Data.AvgRating > 3.01 {
		t.Fatalf("avg rating inattendue: %.2f", payload.Data.AvgRating)
	}
}

func TestHandleDismissRecommendation_InsertsRow(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	req := httptest.NewRequest(http.MethodPost, "/api/recommendations/dismiss", strings.NewReader(`{"source":"anilist","anilist_id":12345}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleDismissRecommendation(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM dismissed_recommendations WHERE user_id = 1 AND source = 'anilist' AND external_id = '12345'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestFilterDismissedSuggestions_RemovesMatchingIDs(t *testing.T) {
	res := &recommend.ForUserResult{
		Results: []recommend.Suggestion{
			{Source: "browse", AnilistID: 111, Title: "Keep"},
			{Source: "browse", AnilistID: 222, Title: "Remove"},
			{Source: "recommendation", AnilistID: 333, Title: "Keep2"},
		},
	}
	dismissed := map[string]struct{}{"222": {}}
	filterDismissedSuggestions(res, dismissed)
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res.Results))
	}
	if res.Results[0].AnilistID != 111 || res.Results[1].AnilistID != 333 {
		t.Fatalf("unexpected results: %+v", res.Results)
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
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

func TestMergeDuplicate_MergesAndDeletesFrom(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO works (id, title, chapter, link, status, reading_type, rating, notes, user_id, updated_at)
		 VALUES
		 (10, 'Same', 3, NULL, 'En cours', 'Manga', 2, 'into notes', 1, CURRENT_TIMESTAMP),
		 (11, 'Same', 7, 'https://x.test', NULL, 'Manga', 5, 'from notes', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tools/duplicates/merge", strings.NewReader("from_id=11&into_id=10"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()

	// Ensure CSRF policy behaves like prod chain.
	app.WithRequestPolicies(http.HandlerFunc(app.HandleMergeDuplicate)).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	var (
		ch     int
		link   sql.NullString
		rating int
		notes  sql.NullString
	)
	if err := db.QueryRow(`SELECT chapter, link, rating, notes FROM works WHERE id = 10 AND user_id = 1`).Scan(&ch, &link, &rating, &notes); err != nil {
		t.Fatal(err)
	}
	if ch != 7 || !link.Valid || link.String != "https://x.test" || rating != 5 || !notes.Valid {
		t.Fatalf("merged work unexpected: ch=%d link=%v rating=%d notes=%v", ch, link, rating, notes)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM works WHERE id = 11 AND user_id = 1`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected from work deleted, count=%d", count)
	}
}
