package translate

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bookstorage/internal/config"
	"bookstorage/internal/database"

	_ "github.com/mattn/go-sqlite3"
)

func testDB(t *testing.T) *database.Conn {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE translation_cache (
		source_hash TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		translated_text TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (source_hash, target_lang)
	);`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return database.NewSQLiteConn(db)
}

func TestCachedToFrench_Caches(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/translate" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"translatedText": "Bonjour le monde"})
	}))
	defer srv.Close()

	db := testDB(t)
	s := &config.Settings{TranslateURL: srv.URL}

	out1, ok1, err := CachedToFrench(db, s, "Hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !ok1 || out1 != "Bonjour le monde" {
		t.Fatalf("first: ok=%v out=%q", ok1, out1)
	}
	if calls != 1 {
		t.Fatalf("expected 1 API call, got %d", calls)
	}

	out2, ok2, err := CachedToFrench(db, s, "Hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !ok2 || out2 != "Bonjour le monde" {
		t.Fatalf("second: ok=%v out=%q", ok2, out2)
	}
	if calls != 1 {
		t.Fatalf("expected cache hit (still 1 call), got %d", calls)
	}
}

func TestCachedToFrench_NoURL(t *testing.T) {
	db := testDB(t)
	out, ok, err := CachedToFrench(db, &config.Settings{}, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	if ok || out != "Hello" {
		t.Fatalf("got ok=%v out=%q", ok, out)
	}
}
