package server

import (
	"sync"
	"testing"
	"time"

	"bookstorage/internal/catalog"
)

func TestEnrichBdCoversMissing(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}

	_, err := db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, source, external_id, user_id, updated_at)
		 VALUES
		 ('Has Cover', 1, 'Terminé', 'Album', 'openlibrary', '/works/OL1W', 1, CURRENT_TIMESTAMP),
		 ('Needs Cover', 2, 'Terminé', 'Album', 'bdgest', '55', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE bd_works SET image_path = 'https://cdn.test/existing.jpg' WHERE title = 'Has Cover'`)

	orig := resolveBdCoverURL
	origPace := bdCoverEnrichPace
	bdCoverEnrichPace = 0
	defer func() {
		resolveBdCoverURL = orig
		bdCoverEnrichPace = origPace
	}()
	var mu sync.Mutex
	calls := 0
	resolveBdCoverURL = func(source, externalID, title string) (bdCoverResolve, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if title == "Needs Cover" {
			return bdCoverResolve{URL: "https://covers.openlibrary.org/b/id/99-L.jpg"}, nil
		}
		return bdCoverResolve{}, nil
	}

	app.enrichBdCoversMissing(1)

	var img string
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM bd_works WHERE title = 'Needs Cover'`).Scan(&img)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://covers.openlibrary.org/b/id/99-L.jpg" {
		t.Fatalf("expected enriched cover, got %q", img)
	}
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM bd_works WHERE title = 'Has Cover'`).Scan(&img)
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

func TestEnrichBdCoversMissing_RetriesRateLimit(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, source, external_id, user_id, updated_at)
		 VALUES ('Rate Limited', 1, 'Terminé', 'Album', 'openlibrary', '/works/OL9W', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	orig := resolveBdCoverURL
	origSleep := bdCoverEnrichRateLimitSleep
	origBackoff := bdCoverEnrichInitialBackoff
	bdCoverEnrichRateLimitSleep = func(time.Duration) {}
	bdCoverEnrichInitialBackoff = time.Millisecond
	defer func() {
		resolveBdCoverURL = orig
		bdCoverEnrichRateLimitSleep = origSleep
		bdCoverEnrichInitialBackoff = origBackoff
	}()

	attempts := 0
	resolveBdCoverURL = func(source, externalID, title string) (bdCoverResolve, error) {
		attempts++
		if attempts == 1 {
			return bdCoverResolve{}, catalog.ErrOpenLibraryRateLimit
		}
		return bdCoverResolve{URL: "https://covers.openlibrary.org/b/id/1-L.jpg"}, nil
	}

	var id int
	_ = db.QueryRow(`SELECT id FROM bd_works WHERE title = 'Rate Limited'`).Scan(&id)
	app.enrichOneBdCover(1, bdCoverPending{
		id: id, title: "Rate Limited", source: "openlibrary", externalID: "/works/OL9W",
	})

	var img string
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM bd_works WHERE title = 'Rate Limited'`).Scan(&img)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://covers.openlibrary.org/b/id/1-L.jpg" {
		t.Fatalf("expected cover after retry, got %q (attempts=%d)", img, attempts)
	}
}

func TestScheduleBdCoverEnrichment(t *testing.T) {
	db, s := openTestDB(t)
	app := &App{Settings: s, DB: db}
	_, err := db.Exec(
		`INSERT INTO bd_works (title, tome, status, bd_type, user_id, updated_at)
		 VALUES ('Async BD', 1, 'Terminé', 'Album', 1, CURRENT_TIMESTAMP)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	orig := resolveBdCoverURL
	origPace := bdCoverEnrichPace
	bdCoverEnrichPace = 0
	defer func() {
		app.bdCoverJobs.waitIdle(3 * time.Second)
		resolveBdCoverURL = orig
		bdCoverEnrichPace = origPace
	}()
	resolveBdCoverURL = func(source, externalID, title string) (bdCoverResolve, error) {
		return bdCoverResolve{URL: "https://cdn.test/async-bd.jpg"}, nil
	}

	app.scheduleBdCoverEnrichment(1)
	app.bdCoverJobs.waitIdle(3 * time.Second)

	var img string
	err = db.QueryRow(`SELECT COALESCE(image_path, '') FROM bd_works WHERE title = 'Async BD'`).Scan(&img)
	if err != nil {
		t.Fatal(err)
	}
	if img != "https://cdn.test/async-bd.jpg" {
		t.Fatalf("expected async enrich, got %q", img)
	}
}
