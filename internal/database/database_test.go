package database

import (
	"path/filepath"
	"testing"

	"bookstorage/internal/config"
)

func TestEnsureSchemaAndMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s := &config.Settings{
		Database:             dbPath,
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
	defer func() { _ = db.Close() }()
	if err := EnsureSchema(db, s); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected migrations applied, got count %d", n)
	}
	var idxCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_works_user_id'`).Scan(&idxCount); err != nil {
		t.Fatal(err)
	}
	if idxCount != 1 {
		t.Fatalf("expected idx_works_user_id index, got %d", idxCount)
	}
}
