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
)

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
