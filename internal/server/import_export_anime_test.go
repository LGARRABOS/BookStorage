package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMalScoreToStars(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0}, {1, 1}, {5, 5}, {6, 3}, {7, 4}, {8, 4}, {9, 5}, {10, 5}, {11, 5},
	}
	for _, tc := range cases {
		if got := malScoreToStars(tc.in); got != tc.want {
			t.Fatalf("malScoreToStars(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseMALAnimeXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<myanimelist>
  <anime>
    <series_animedb_id>53802</series_animedb_id>
    <series_title><![CDATA[2.5-jigen no Ririsa]]></series_title>
    <series_type>TV</series_type>
    <series_episodes>24</series_episodes>
    <my_watched_episodes>24</my_watched_episodes>
    <my_start_date>2024-10-25</my_start_date>
    <my_finish_date>2024-12-14</my_finish_date>
    <my_score>8</my_score>
    <my_status>Completed</my_status>
    <my_comments><![CDATA[nice]]></my_comments>
    <my_tags><![CDATA[]]></my_tags>
  </anime>
  <anime>
    <series_animedb_id>60510</series_animedb_id>
    <series_title><![CDATA[Plan Entry]]></series_title>
    <series_type>Movie</series_type>
    <series_episodes>0</series_episodes>
    <my_watched_episodes>0</my_watched_episodes>
    <my_start_date>0000-00-00</my_start_date>
    <my_finish_date>0000-00-00</my_finish_date>
    <my_score>0</my_score>
    <my_status>Plan to Watch</my_status>
    <my_comments><![CDATA[]]></my_comments>
    <my_tags><![CDATA[]]></my_tags>
  </anime>
</myanimelist>`
	rows, ok := parseMALAnimeXML([]byte(xmlData))
	if !ok || len(rows) != 2 {
		t.Fatalf("parse ok=%v len=%d", ok, len(rows))
	}
	if rows[0].Title != "2.5-jigen no Ririsa" || rows[0].Status != "Terminé" || rows[0].Episode != 24 {
		t.Fatalf("row0 unexpected: %+v", rows[0])
	}
	if rows[0].Rating != 4 || rows[0].ExternalID != "53802" {
		t.Fatalf("row0 score/id: rating=%d ext=%s", rows[0].Rating, rows[0].ExternalID)
	}
	if rows[0].TotalEpisodes == nil || *rows[0].TotalEpisodes != 24 {
		t.Fatalf("row0 total episodes: %v", rows[0].TotalEpisodes)
	}
	if rows[0].StartedAt != "2024-10-25" || rows[0].FinishedAt != "2024-12-14" {
		t.Fatalf("row0 dates: %s / %s", rows[0].StartedAt, rows[0].FinishedAt)
	}
	if !strings.Contains(rows[0].Link, "/anime/53802") {
		t.Fatalf("row0 link: %s", rows[0].Link)
	}
	if rows[1].Status != "À voir" || rows[1].AnimeType != "Film" || rows[1].StartedAt != "" {
		t.Fatalf("row1 unexpected: %+v", rows[1])
	}
}

func TestImportFromXML_MALAnime(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<myanimelist>
  <anime>
    <series_animedb_id>21</series_animedb_id>
    <series_title><![CDATA[One Piece]]></series_title>
    <series_type>TV</series_type>
    <series_episodes>1000</series_episodes>
    <my_watched_episodes>1100</my_watched_episodes>
    <my_start_date>2020-01-01</my_start_date>
    <my_finish_date>0000-00-00</my_finish_date>
    <my_score>9</my_score>
    <my_status>Watching</my_status>
    <my_comments><![CDATA[]]></my_comments>
    <my_tags><![CDATA[]]></my_tags>
  </anime>
</myanimelist>`

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "animelist.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, xmlData)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/anime/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var gotTitle, gotStatus, gotSource, gotExt string
	var gotEpisode, gotRating int
	var total sql.NullInt64
	err = db.QueryRow(`SELECT title, status, episode, rating, total_episodes, source, external_id FROM anime_works WHERE user_id = 1 LIMIT 1`).
		Scan(&gotTitle, &gotStatus, &gotEpisode, &gotRating, &total, &gotSource, &gotExt)
	if err != nil {
		t.Fatal(err)
	}
	if gotTitle != "One Piece" || gotStatus != "En cours" || gotEpisode != 1100 || gotRating != 5 {
		t.Fatalf("MAL XML import unexpected: %s %s ep=%d rating=%d", gotTitle, gotStatus, gotEpisode, gotRating)
	}
	if !total.Valid || total.Int64 != 1000 || gotSource != "mal" || gotExt != "21" {
		t.Fatalf("meta unexpected: total=%v source=%s ext=%s", total, gotSource, gotExt)
	}
}

func TestImportFromCSV_MALAnime(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "mal-anime.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "series_title;series_type;my_status;my_watched_episodes;my_score;series_episodes\nFrieren;TV;watching;12;5;28\n")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/anime/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var gotTitle, gotStatus, gotType string
	var gotEpisode, gotRating int
	var total sql.NullInt64
	err = db.QueryRow(`SELECT title, status, anime_type, episode, rating, total_episodes FROM anime_works WHERE user_id = 1 LIMIT 1`).
		Scan(&gotTitle, &gotStatus, &gotType, &gotEpisode, &gotRating, &total)
	if err != nil {
		t.Fatal(err)
	}
	if gotTitle != "Frieren" || gotStatus != "En cours" || gotType != "TV" || gotEpisode != 12 || gotRating != 5 {
		t.Fatalf("anime MAL inattendu: %s %s %s %d %d", gotTitle, gotStatus, gotType, gotEpisode, gotRating)
	}
	if !total.Valid || total.Int64 != 28 {
		t.Fatalf("total_episodes attendu 28, got %v", total)
	}
}

func TestImportFromJSON_AniListAnimeOnly(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	payload := `{
		"lists": [{
			"entries": [
				{
					"status": "CURRENT",
					"progress": 10,
					"score": 4,
					"media": {
						"id": 21,
						"type": "ANIME",
						"title": {"english":"One Piece"},
						"format": "TV",
						"episodes": 1000,
						"isAdult": false,
						"coverImage": {"large":"https://cdn.test/op.jpg"}
					}
				},
				{
					"status": "CURRENT",
					"progress": 5,
					"score": 3,
					"media": {
						"id": 30001,
						"type": "MANGA",
						"title": {"romaji":"Berserk"},
						"format": "MANGA",
						"isAdult": false,
						"coverImage": {"large":""}
					}
				}
			]
		}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/anime/import?duplicate_mode=skip", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM anime_works WHERE user_id = 1`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 anime (manga filtered out), got %d", count)
	}
	var title string
	var ep int
	var link sql.NullString
	err := db.QueryRow(`SELECT title, episode, link FROM anime_works WHERE user_id = 1`).Scan(&title, &ep, &link)
	if err != nil {
		t.Fatal(err)
	}
	if title != "One Piece" || ep != 10 {
		t.Fatalf("unexpected anime: %s ep=%d", title, ep)
	}
	if !link.Valid || !strings.Contains(link.String, "/anime/21") {
		t.Fatalf("anilist anime link missing: %v", link.String)
	}
	var mangaCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = 1`).Scan(&mangaCount)
	if mangaCount != 0 {
		t.Fatalf("manga should not be imported into works via anime import, got %d", mangaCount)
	}
}

func TestHandleAnimeExport_JSONRoundTrip(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, total_episodes, status, anime_type, rating, user_id, source, updated_at)
		 VALUES ('Export Me', 3, 12, 'En cours', 'TV', 4, 1, 'manual', CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/anime/export?format=json", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeExport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var payload struct {
		AnimeWorks []exportAnimeWork `json:"anime_works"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.AnimeWorks) != 1 || payload.AnimeWorks[0].Title != "Export Me" || payload.AnimeWorks[0].Episode != 3 {
		t.Fatalf("export inattendu: %+v", payload.AnimeWorks)
	}
}

func TestImportAnimeDuplicateSkip_FillsMissingCover(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, user_id, source, updated_at)
		 VALUES ('Coverless', 1, 'En cours', 'TV', 1, 'manual', CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	report := ImportReport{}
	app.importOneAnimeWork(1, 1, exportAnimeWork{
		Title:      "Coverless",
		Episode:    5,
		Status:     "En cours",
		AnimeType:  "TV",
		ImagePath:  "https://cdn.test/cover.jpg",
		Source:     "anilist",
		ExternalID: "99",
	}, DuplicateSkip, &report)

	if report.Updated != 1 || report.SkippedDuplicate != 0 {
		t.Fatalf("expected cover update, report=%+v", report)
	}
	var img, source, ext string
	var ep int
	err = db.QueryRow(`SELECT COALESCE(image_path,''), episode, source, COALESCE(external_id,'') FROM anime_works WHERE title='Coverless'`).
		Scan(&img, &ep, &source, &ext)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://cdn.test/cover.jpg" {
		t.Fatalf("cover not filled: %q", img)
	}
	if ep != 1 {
		t.Fatalf("skip mode must not overwrite episode, got %d", ep)
	}
	if source != "anilist" || ext != "99" {
		t.Fatalf("expected source/id merge, got %s/%s", source, ext)
	}
}

func TestImportAnimeDuplicateSkip_KeepsExistingCover(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, image_path, user_id, source, updated_at)
		 VALUES ('HasCover', 1, 'En cours', 'TV', 'https://cdn.test/old.jpg', 1, 'manual', CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	report := ImportReport{}
	app.importOneAnimeWork(1, 1, exportAnimeWork{
		Title:     "HasCover",
		ImagePath: "https://cdn.test/new.jpg",
		Source:    "anilist",
	}, DuplicateSkip, &report)
	if report.SkippedDuplicate != 1 || report.Updated != 0 {
		t.Fatalf("expected pure skip, report=%+v", report)
	}
	var img string
	_ = db.QueryRow(`SELECT image_path FROM anime_works WHERE title='HasCover'`).Scan(&img)
	if img != "https://cdn.test/old.jpg" {
		t.Fatalf("existing cover must stay: %q", img)
	}
}

func TestImportAnimeDuplicateSkip_SchedulesEnrichment(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, source, external_id, user_id, updated_at)
		 VALUES ('NeedEnrich', 1, 'En cours', 'TV', 'mal', '21', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	orig := resolveAnimeCoverURL
	origPace := animeCoverEnrichPace
	animeCoverEnrichPace = 0
	defer func() {
		resolveAnimeCoverURL = orig
		animeCoverEnrichPace = origPace
	}()
	resolveAnimeCoverURL = func(source, externalID, title string) (string, error) {
		defer close(done)
		return "https://cdn.test/enriched.jpg", nil
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("duplicate_mode", "skip")
	part, err := w.CreateFormFile("import_file", "animelist.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, `<?xml version="1.0"?>
<myanimelist>
  <anime>
    <series_animedb_id>21</series_animedb_id>
    <series_title><![CDATA[NeedEnrich]]></series_title>
    <series_type>TV</series_type>
    <series_episodes>0</series_episodes>
    <my_watched_episodes>1</my_watched_episodes>
    <my_score>0</my_score>
    <my_status>Watching</my_status>
    <my_comments><![CDATA[]]></my_comments>
    <my_tags><![CDATA[]]></my_tags>
  </anime>
</myanimelist>`)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/anime/import", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "session", Value: mustCreateSession(t, app, 1)})
	rec := httptest.NewRecorder()
	app.HandleAnimeImport(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enrichment not scheduled on duplicate skip re-import")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var img string
		_ = db.QueryRow(`SELECT COALESCE(image_path,'') FROM anime_works WHERE title='NeedEnrich'`).Scan(&img)
		if img == "https://cdn.test/enriched.jpg" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cover not enriched after duplicate re-import")
}

