package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bookstorage/internal/catalog"
)

func TestEnrichAnimeCoversMissing(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, source, external_id, user_id, updated_at)
		 VALUES
		 ('Has Cover', 1, 'En cours', 'TV', 'mal', '1', 1, CURRENT_TIMESTAMP),
		 ('Needs Cover', 2, 'En cours', 'TV', 'mal', '21', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE anime_works SET image_path = 'https://cdn.test/existing.jpg' WHERE title = 'Has Cover'`)

	orig := resolveAnimeCoverURL
	origPace := animeCoverEnrichPace
	animeCoverEnrichPace = 0
	defer func() {
		resolveAnimeCoverURL = orig
		animeCoverEnrichPace = origPace
	}()
	var mu sync.Mutex
	calls := 0
	resolveAnimeCoverURL = func(source, externalID, title string) (animeCoverResolve, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if title == "Needs Cover" && source == "mal" && externalID == "21" {
			return animeCoverResolve{URL: "https://cdn.test/one-piece.jpg"}, nil
		}
		return animeCoverResolve{}, nil
	}

	app.enrichAnimeCoversMissing(1)

	var img string
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM anime_works WHERE title = 'Needs Cover'`).Scan(&img)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://cdn.test/one-piece.jpg" {
		t.Fatalf("expected enriched cover, got %q", img)
	}
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM anime_works WHERE title = 'Has Cover'`).Scan(&img)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://cdn.test/existing.jpg" {
		t.Fatalf("existing cover should stay, got %q", img)
	}
	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 cover lookup (only missing), got %d", n)
	}
}

func TestEnrichAnimeCoversMissing_RetriesRateLimit(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	res, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, source, external_id, user_id, updated_at)
		 VALUES ('Rate Limited', 1, 'En cours', 'TV', 'mal', '21', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	workID, _ := res.LastInsertId()

	orig := resolveAnimeCoverURL
	origSleep := animeCoverEnrichRateLimitSleep
	origBackoff := animeCoverEnrichInitialBackoff
	animeCoverEnrichRateLimitSleep = func(time.Duration) {}
	animeCoverEnrichInitialBackoff = time.Millisecond
	defer func() {
		resolveAnimeCoverURL = orig
		animeCoverEnrichRateLimitSleep = origSleep
		animeCoverEnrichInitialBackoff = origBackoff
	}()

	var attempts atomic.Int32
	resolveAnimeCoverURL = func(source, externalID, title string) (animeCoverResolve, error) {
		if attempts.Add(1) == 1 {
			return animeCoverResolve{}, catalog.ErrAnilistRateLimit
		}
		return animeCoverResolve{URL: "https://cdn.test/after-retry.jpg"}, nil
	}

	app.enrichOneAnimeCover(1, animeCoverPending{
		id: int(workID), title: "Rate Limited", source: "mal", externalID: "21",
	})

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts (rate-limit then ok), got %d", attempts.Load())
	}
	var img string
	_ = db.QueryRow(`SELECT COALESCE(image_path,'') FROM anime_works WHERE id = ?`, workID).Scan(&img)
	if img != "https://cdn.test/after-retry.jpg" {
		t.Fatalf("got %q", img)
	}
}

func TestScheduleAnimeCoverEnrichment(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO anime_works (title, episode, status, anime_type, source, external_id, user_id, updated_at)
		 VALUES ('Async Cover', 1, 'En cours', 'TV', 'mal', '99', 1, CURRENT_TIMESTAMP)`,
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
	resolveAnimeCoverURL = func(source, externalID, title string) (animeCoverResolve, error) {
		defer close(done)
		return animeCoverResolve{URL: "https://cdn.test/async.jpg"}, nil
	}

	app.scheduleAnimeCoverEnrichment(1)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enrichment did not run")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var img string
		_ = db.QueryRow(`SELECT COALESCE(image_path, '') FROM anime_works WHERE title = 'Async Cover'`).Scan(&img)
		if img == "https://cdn.test/async.jpg" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("async cover not applied")
}

func TestParseMALAnimeCSVRecords_SetsExternalID(t *testing.T) {
	records := [][]string{
		{"series_animedb_id", "series_title", "series_type", "series_episodes", "my_watched_episodes", "my_score", "my_status"},
		{"21", "One Piece", "TV", "1000", "10", "9", "Watching"},
		{"", "No ID Show", "TV", "12", "1", "0", "Plan to Watch"},
	}
	headers := make([]string, len(records[0]))
	for i, h := range records[0] {
		headers[i] = normalizeHeader(h)
	}
	out := parseMALAnimeCSVRecords(records, headers)
	if len(out) != 2 {
		t.Fatalf("got %d rows", len(out))
	}
	if out[0].Source != "mal" || out[0].ExternalID != "21" {
		t.Fatalf("row0 source/id = %q/%q", out[0].Source, out[0].ExternalID)
	}
	if out[1].Source != "mal" || out[1].ExternalID != "" {
		t.Fatalf("row1 source/id = %q/%q", out[1].Source, out[1].ExternalID)
	}
}
