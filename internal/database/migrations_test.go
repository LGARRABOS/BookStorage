package database

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestApplyMigrations_v9OutsideTx_dropsUsersWithFK reproduces production failure when migration 9
// ran inside a transaction (PRAGMA foreign_keys=OFF is ignored there).
func TestApplyMigrations_v9OutsideTx_dropsUsersWithFK(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:mig9fk?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(createSchemaMigrationsTableSQL); err != nil {
		t.Fatal(err)
	}
	// Minimal pre-v9 schema: users (no google columns) + child referencing users.
	if _, err := db.Exec(`
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	password TEXT NOT NULL,
	validated INTEGER DEFAULT 0,
	is_admin INTEGER DEFAULT 0,
	is_superadmin INTEGER DEFAULT 0,
	display_name TEXT,
	email TEXT,
	bio TEXT,
	avatar_path TEXT,
	is_public INTEGER DEFAULT 1
);
CREATE TABLE works (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	user_id INTEGER NOT NULL,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
INSERT INTO users (username, password, validated, is_admin, is_superadmin)
	VALUES ('reader', 'hashed', 1, 0, 0);
INSERT INTO works (title, user_id) VALUES ('One work', 1);
`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 8; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			t.Fatal(err)
		}
	}

	if err := applySQLiteMigrations(db); err != nil {
		t.Fatalf("applySQLiteMigrations: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM oauth_states`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE google_sub IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("users count = %d, want 1", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("works still linked: count = %d", n)
	}
}
