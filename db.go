package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
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

var profileColumns = map[string]string{
	"display_name": "TEXT",
	"email":        "TEXT",
	"bio":          "TEXT",
	"avatar_path":  "TEXT",
	"is_public":    "INTEGER DEFAULT 1",
}

var workColumns = map[string]string{
	"reading_type": "TEXT",
}

func openDB(settings *Settings) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", settings.Database)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureColumns(db *sql.DB, table string, cols map[string]string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

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

func ensureSuperAdmin(db *sql.DB, s *Settings) error {
	var exists int
	if err := db.QueryRow("SELECT 1 FROM users WHERE is_superadmin = 1 LIMIT 1").Scan(&exists); err == nil {
		return nil
	}

	// NOTE: pour rester simple ici, on stocke le mot de passe en clair.
	// Pour un usage réel, remplacer par un hash sécurisé (bcrypt, argon2, ...).
	_, err := db.Exec(
		`INSERT INTO users (username, password, validated, is_admin, is_superadmin)
         VALUES (?, ?, 1, 1, 1)`,
		s.SuperadminUsername,
		s.SuperadminPassword,
	)
	return err
}

func ensureSchema(db *sql.DB, s *Settings) error {
	if _, err := db.Exec(createUsersTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createWorksTableSQL); err != nil {
		return err
	}
	if err := ensureColumns(db, "users", profileColumns); err != nil {
		return err
	}
	if err := ensureColumns(db, "works", workColumns); err != nil {
		return err
	}
	if err := ensureSuperAdmin(db, s); err != nil {
		return err
	}
	return nil
}

