package server

import (
	"sync"
	"testing"
	"time"
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
	defer func() { resolveAnimeCoverURL = orig }()
	var mu sync.Mutex
	calls := 0
	resolveAnimeCoverURL = func(source, externalID, title string) string {
		mu.Lock()
		calls++
		mu.Unlock()
		if title == "Needs Cover" && source == "mal" && externalID == "21" {
			return "https://cdn.test/one-piece.jpg"
		}
		return ""
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
	defer func() { resolveAnimeCoverURL = orig }()
	resolveAnimeCoverURL = func(source, externalID, title string) string {
		defer close(done)
		return "https://cdn.test/async.jpg"
	}

	app.scheduleAnimeCoverEnrichment(1)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enrichment did not run")
	}
	// allow UPDATE to commit
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
