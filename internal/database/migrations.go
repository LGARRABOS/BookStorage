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
	// OutsideTx: run Up on db (not sql.Tx). Required when Up contains DDL that breaks FK
	// while rebuilding a referenced table — SQLite ignores PRAGMA foreign_keys inside BEGIN.
	OutsideTx bool
}

// LatestSchemaMigrationVersion is the highest numbered migration (SQLite and Postgres logical version).
const LatestSchemaMigrationVersion = 26

// ApplyMigrations runs dialect-specific migration bookkeeping.
func ApplyMigrations(c *Conn) error {
	if c == nil {
		return fmt.Errorf("nil connection")
	}
	if c.B == BackendPostgres {
		return applyPostgresMigrationMarkers(c)
	}
	return applySQLiteMigrations(c.Std())
}

func applyPostgresMigrationMarkers(c *Conn) error {
	for v := 1; v <= LatestSchemaMigrationVersion; v++ {
		if _, err := c.Exec(`INSERT INTO schema_migrations (version) VALUES (?) ON CONFLICT (version) DO NOTHING`, v); err != nil {
			return fmt.Errorf("postgres migration marker %d: %w", v, err)
		}
	}
	return nil
}

// applySQLiteMigrations runs pending numbered migrations in a transaction each.
// Migrations with OutsideTx run their Up script on the raw connection (PRAGMA foreign_keys
// is ignored inside BEGIN, which would break migrations that DROP users while child tables exist).
func applySQLiteMigrations(db *sql.DB) error {
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

		if m.OutsideTx {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("migration %d: pragma foreign_keys=OFF: %w", m.Version, err)
			}
			if m.Up != "" {
				if _, err := db.Exec(m.Up); err != nil {
					_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
					return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
				}
			}
			tx, err := db.Begin()
			if err != nil {
				_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
				return fmt.Errorf("begin migration %d (record): %w", m.Version, err)
			}
			if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
				_ = tx.Rollback()
				_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
				return fmt.Errorf("record migration %d: %w", m.Version, err)
			}
			if err := tx.Commit(); err != nil {
				_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
				return fmt.Errorf("commit migration %d: %w", m.Version, err)
			}
			if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
				return fmt.Errorf("migration %d: pragma foreign_keys=ON: %w", m.Version, err)
			}
			continue
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
