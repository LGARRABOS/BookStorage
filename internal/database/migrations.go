package database

import (
	"database/sql"
	"errors"
	"fmt"
)

const createSchemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

type migration struct {
	Version int
	Name    string
	Up      string
}

var migrations = []migration{
	{Version: 1, Name: "baseline", Up: ""},
	{Version: 2, Name: "reading_type_18plus_to_adult", Up: `UPDATE works SET is_adult = 1, reading_type = 'Autre' WHERE reading_type = '18+' AND COALESCE(is_adult, 0) = 0`},
	{Version: 3, Name: "drop_reminders_and_push", Up: `DROP TABLE IF EXISTS reminders; DROP TABLE IF EXISTS push_subscriptions;`},
	{Version: 4, Name: "translation_cache", Up: `CREATE TABLE IF NOT EXISTS translation_cache (
    source_hash TEXT NOT NULL,
    target_lang TEXT NOT NULL,
    translated_text TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_hash, target_lang)
);`},
}

// ApplyMigrations runs pending numbered migrations in a transaction each.
func ApplyMigrations(db *sql.DB) error {
	if _, err := db.Exec(createSchemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var done int
		err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, m.Version).Scan(&done)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}
		if m.Up != "" {
			if _, err := tx.Exec(m.Up); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}
	return nil
}
