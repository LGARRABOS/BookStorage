package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"bookstorage/internal/config"

	"golang.org/x/crypto/bcrypt"
)

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
		if err := ensureCatalogFTS(c); err != nil {
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
	if err := ensureColumnsSQLite(db, "catalog", catalogColumns); err != nil {
		return err
	}
	if _, err := db.Exec(createUserCatalogBlocklistSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createSessionsTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createDismissedRecommendationsTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createReadingSitesTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createWorksTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createAnimeWorksTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createBdWorksTableSQL); err != nil {
		return err
	}
	if err := ensureColumnsSQLite(db, "bd_works", bdWorksColumns); err != nil {
		return err
	}
	if _, err := db.Exec(createMangaPhysWorksTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createLibraryFurnitureTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(createLibraryShelvesTableSQL); err != nil {
		return err
	}
	if err := ensureColumnsSQLite(db, "library_shelves", map[string]string{
		"books_per_case": "INTEGER NOT NULL DEFAULT 8",
	}); err != nil {
		return err
	}
	if _, err := db.Exec(createLibraryPlacementsTableSQL); err != nil {
		return err
	}
	// One-shot: drop legacy manga placements that pointed at virtual works (migration 26 marker).
	if _, err := db.Exec(createSchemaMigrationsTableSQL); err != nil {
		return err
	}
	var has26 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 26`).Scan(&has26); err != nil {
		return err
	}
	if has26 == 0 {
		if _, err := db.Exec(`DELETE FROM library_placements WHERE media_kind = 'manga'`); err != nil {
			return err
		}
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
	// Re-apply after migrations: e.g. migration 9 rebuilds users without later columns.
	if err := ensureColumnsSQLite(db, "users", profileColumns); err != nil {
		return err
	}
	if err := ensureWorksFTS5(db); err != nil {
		return err
	}
	if err := ensureCatalogFTS(c); err != nil {
		return err
	}
	return ensureSuperAdmin(c, s)
}
