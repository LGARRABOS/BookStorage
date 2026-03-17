package database

import (
	"database/sql"
	"fmt"

	"bookstorage/internal/config"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const createUsersTableSQL = `
CREATE TABLE IF NOT EXISTS users (
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

const createPushSubscriptionsTableSQL = `
CREATE TABLE IF NOT EXISTS push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users (id)
);`

const createRemindersTableSQL = `
CREATE TABLE IF NOT EXISTS reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    work_id INTEGER NOT NULL,
    remind_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    sent INTEGER DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users (id),
    FOREIGN KEY (work_id) REFERENCES works (id)
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
}

// Open opens a database connection
func Open(settings *config.Settings) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", settings.Database)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureColumns(db *sql.DB, table string, cols map[string]string) error {
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

func ensureSuperAdmin(db *sql.DB, s *config.Settings) error {
	var exists int
	if err := db.QueryRow("SELECT 1 FROM users WHERE is_superadmin = 1 LIMIT 1").Scan(&exists); err == nil {
		return nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(s.SuperadminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
         VALUES (?, ?, 1, 1, 1)`,
		s.SuperadminUsername,
		string(hashedPassword),
	)
	return err
}

// EnsureSchema creates tables and ensures all columns exist
func EnsureSchema(db *sql.DB, s *config.Settings) error {
	if _, err := db.Exec(createUsersTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createCatalogTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createWorksTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createRemindersTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createPushSubscriptionsTableSQL); err != nil {
		return err
	}
	if err := ensureColumns(db, "users", profileColumns); err != nil {
		return err
	}
	if err := ensureColumns(db, "works", workColumns); err != nil {
		return err
	}
	// Migration légère : transformer les anciens types \"18+\" en flag adulte
	if _, err := db.Exec(`UPDATE works SET is_adult = 1, reading_type = 'Autre' WHERE reading_type = '18+' AND COALESCE(is_adult, 0) = 0`); err != nil {
		return err
	}
	if err := ensureSuperAdmin(db, s); err != nil {
		return err
	}
	return nil
}
