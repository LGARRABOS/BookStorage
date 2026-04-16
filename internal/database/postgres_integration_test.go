package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bookstorage/internal/config"
)

// TestPostgresEnsureSchema runs when BOOKSTORAGE_POSTGRES_URL is set (e.g. in CI with a Postgres service).
func TestPostgresEnsureSchema(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("BOOKSTORAGE_POSTGRES_URL"))
	if dsn == "" {
		t.Skip("set BOOKSTORAGE_POSTGRES_URL to run PostgreSQL integration tests")
	}
	dir := t.TempDir()
	s := &config.Settings{
		PostgresURL:          dsn,
		Database:             filepath.Join(dir, "unused.db"),
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
	db, err := Open(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_superadmin = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("superadmin rows: %d", n)
	}
	if !WorksFTSEnabled(db) {
		t.Fatal("expected postgres FTS column")
	}
}
