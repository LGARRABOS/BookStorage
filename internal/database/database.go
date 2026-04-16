package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"bookstorage/internal/config"

	"golang.org/x/crypto/bcrypt"
)

const createUsersTableSQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password TEXT,
    validated INTEGER DEFAULT 0,
    is_admin INTEGER DEFAULT 0,
    is_superadmin INTEGER DEFAULT 0,
    display_name TEXT,
    email TEXT,
    bio TEXT,
    avatar_path TEXT,
    is_public INTEGER DEFAULT 1,
    google_sub TEXT UNIQUE,
    google_email TEXT
);`

const createWorksTableSQL = `
CREATE TABLE IF NOT EXISTS works (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    chapter INTEGER DEFAULT 0,
    link TEXT,
    status TEXT,
    image_path TEXT,
    reading_type TEXT,
    user_id INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id)
);`

const createCatalogTableSQL = `
CREATE TABLE IF NOT EXISTS catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    reading_type TEXT NOT NULL,
    image_url TEXT,
    source TEXT NOT NULL DEFAULT 'manual',
    external_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createDismissedRecommendationsTableSQL = `
CREATE TABLE IF NOT EXISTS dismissed_recommendations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, source, external_id),
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_dismissed_recommendations_user_source
    ON dismissed_recommendations(user_id, source);`

const createSessionsTableSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    ip TEXT,
    user_agent TEXT,
    revoked_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_revoked ON sessions(user_id, revoked_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`

var profileColumns = map[string]string{
	"display_name": "TEXT",
	"email":        "TEXT",
	"bio":          "TEXT",
	"avatar_path":  "TEXT",
	"is_public":    "INTEGER DEFAULT 1",
}

var workColumns = map[string]string{
	"reading_type": "TEXT",
	"rating":       "INTEGER DEFAULT 0",
	"notes":        "TEXT",
	"updated_at":   "DATETIME",
	"is_adult":     "INTEGER DEFAULT 0",
	"catalog_id":   "INTEGER REFERENCES catalog(id)",
	// 1 = exclue du lot / file enrichissement AniList (œuvre sans correspondance catalogue).
	"anilist_enrich_opt_out": "INTEGER DEFAULT 0",
}

// sqliteDataSourceName appends go-sqlite3 DSN options (WAL, busy wait, foreign keys).
func sqliteDataSourceName(dbPath string) string {
	p := filepath.ToSlash(dbPath)
	const opts = "_fk=1&_journal_mode=WAL&_busy_timeout=20000"
	if strings.HasPrefix(p, ":memory:") {
		if strings.Contains(p, "?") {
			return p + "&" + opts
		}
		return p + "?" + opts
	}
	sep := "?"
	if strings.Contains(p, "?") {
		sep = "&"
	}
	return p + sep + opts
}

func ensureColumnsSQLite(db *sql.DB, table string, cols map[string]string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	for name, colType := range cols {
		if !existing[name] {
			if _, err := db.Exec(
				fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, colType),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureSuperAdmin(c *Conn, s *config.Settings) error {
	var exists int
	if err := c.QueryRow("SELECT 1 FROM users WHERE is_superadmin = 1 LIMIT 1").Scan(&exists); err == nil {
		return nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(s.SuperadminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = c.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
         VALUES (?, ?, 1, 1, 1)`,
		s.SuperadminUsername,
		string(hashedPassword),
	)
	return err
}

// EnsureSchema creates tables and ensures all columns exist.
func EnsureSchema(c *Conn, s *config.Settings) error {
	if c == nil {
		return fmt.Errorf("nil db connection")
	}
	if c.B == BackendPostgres {
		if err := ensurePostgresSchema(c); err != nil {
			return err
		}
		if err := ApplyMigrations(c); err != nil {
			return err
		}
		if err := ensurePostgresFullText(c); err != nil {
			return err
		}
		return ensureSuperAdmin(c, s)
	}

	db := c.Std()
	if _, err := db.Exec(createUsersTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createCatalogTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createSessionsTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createDismissedRecommendationsTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createWorksTableSQL); err != nil {
		return err
	}
	if err := ensureColumnsSQLite(db, "users", profileColumns); err != nil {
		return err
	}
	if err := ensureColumnsSQLite(db, "works", workColumns); err != nil {
		return err
	}
	if err := ApplyMigrations(c); err != nil {
		return err
	}
	if err := ensureWorksFTS5(db); err != nil {
		return err
	}
	return ensureSuperAdmin(c, s)
}
