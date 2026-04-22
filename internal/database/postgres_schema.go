package database

import (
	"fmt"
	"strings"
)

// postgresSchemaStatements creates the final BookStorage schema on PostgreSQL
// (equivalent to SQLite after all numbered migrations).
var postgresSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		password TEXT,
		validated INTEGER NOT NULL DEFAULT 0,
		is_admin INTEGER NOT NULL DEFAULT 0,
		is_superadmin INTEGER NOT NULL DEFAULT 0,
		display_name TEXT,
		email TEXT,
		bio TEXT,
		avatar_path TEXT,
		is_public INTEGER NOT NULL DEFAULT 1,
		google_sub TEXT UNIQUE,
		google_email TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS catalog (
		id BIGSERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		reading_type TEXT NOT NULL,
		image_url TEXT,
		source TEXT NOT NULL DEFAULT 'manual',
		external_id TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS works (
		id BIGSERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		chapter INTEGER NOT NULL DEFAULT 0,
		link TEXT,
		status TEXT,
		image_path TEXT,
		reading_type TEXT,
		user_id BIGINT NOT NULL REFERENCES users(id),
		rating INTEGER DEFAULT 0,
		notes TEXT,
		updated_at TIMESTAMPTZ,
		is_adult INTEGER NOT NULL DEFAULT 0,
		catalog_id BIGINT REFERENCES catalog(id),
		anilist_enrich_opt_out INTEGER NOT NULL DEFAULT 0,
		parent_work_id BIGINT REFERENCES works(id),
		series_sort INTEGER NOT NULL DEFAULT 0,
		notify_new_chapters INTEGER NOT NULL DEFAULT 1
	)`,
	`CREATE TABLE IF NOT EXISTS dismissed_recommendations (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		source TEXT NOT NULL,
		external_id TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, source, external_id)
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		token_hash TEXT NOT NULL UNIQUE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMPTZ NOT NULL,
		ip TEXT,
		user_agent TEXT,
		revoked_at TIMESTAMPTZ
	)`,
	`CREATE TABLE IF NOT EXISTS translation_cache (
		source_hash TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		translated_text TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (source_hash, target_lang)
	)`,
	`CREATE TABLE IF NOT EXISTS csv_import_sessions (
		id TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		raw_csv TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_states (
		state_hash TEXT PRIMARY KEY,
		purpose TEXT NOT NULL,
		user_id BIGINT,
		next TEXT,
		expires_at_unix BIGINT NOT NULL,
		code_verifier TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_dismissed_recommendations_user_source
		ON dismissed_recommendations(user_id, source)`,
	`CREATE INDEX IF NOT EXISTS idx_works_user_id ON works(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_works_user_status ON works(user_id, status)`,
	`CREATE INDEX IF NOT EXISTS idx_works_user_type ON works(user_id, reading_type)`,
	`CREATE INDEX IF NOT EXISTS idx_works_user_updated_at ON works(user_id, updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_works_user_title ON works(user_id, title)`,
	`CREATE INDEX IF NOT EXISTS idx_works_catalog_id ON works(catalog_id)`,
	`CREATE INDEX IF NOT EXISTS idx_catalog_source_external_id ON catalog(source, external_id)`,
	`CREATE INDEX IF NOT EXISTS idx_users_validated_public ON users(validated, is_public)`,
	`CREATE INDEX IF NOT EXISTS idx_works_parent_work_id ON works(parent_work_id)`,
	`CREATE INDEX IF NOT EXISTS idx_csv_import_sessions_user ON csv_import_sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at_unix)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user_revoked ON sessions(user_id, revoked_at)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
}

var postgresFTSStatements = []string{
	`ALTER TABLE works ADD COLUMN IF NOT EXISTS works_fts_document tsvector
		GENERATED ALWAYS AS (
			to_tsvector('simple',
				coalesce(title, '') || ' ' || coalesce(notes, '') || ' ' || coalesce(link, '')
			)
		) STORED`,
	`CREATE INDEX IF NOT EXISTS works_fts_gin ON works USING gin (works_fts_document)`,
}

func ensurePostgresSchema(c *Conn) error {
	if c == nil || c.B != BackendPostgres {
		return fmt.Errorf("ensurePostgresSchema: not a postgres connection")
	}
	for _, stmt := range postgresSchemaStatements {
		if _, err := c.Exec(stmt); err != nil {
			return fmt.Errorf("postgres schema: %w", err)
		}
	}
	return ensurePostgresExtraColumns(c)
}

var postgresProfileColumns = map[string]string{
	"display_name": "TEXT",
	"email":        "TEXT",
	"bio":          "TEXT",
	"avatar_path":  "TEXT",
	"is_public":    "INTEGER DEFAULT 1",
}

var postgresWorkColumns = map[string]string{
	"reading_type":           "TEXT",
	"rating":                 "INTEGER DEFAULT 0",
	"notes":                  "TEXT",
	"updated_at":             "TIMESTAMPTZ",
	"is_adult":               "INTEGER DEFAULT 0",
	"catalog_id":             "BIGINT REFERENCES catalog(id)",
	"anilist_enrich_opt_out": "INTEGER DEFAULT 0",
	"parent_work_id":         "BIGINT REFERENCES works(id)",
	"series_sort":            "INTEGER DEFAULT 0",
	"notify_new_chapters":    "INTEGER DEFAULT 1",
}

func ensurePostgresExtraColumns(c *Conn) error {
	if err := ensureColumnsPostgres(c, "users", postgresProfileColumns); err != nil {
		return err
	}
	return ensureColumnsPostgres(c, "works", postgresWorkColumns)
}

func ensureColumnsPostgres(c *Conn, table string, cols map[string]string) error {
	rows, err := c.Query(
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = ?`,
		table,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	existing := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		existing[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name, colType := range cols {
		if !existing[strings.ToLower(name)] {
			q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quotePGIdent(table), quotePGIdent(name), colType)
			if _, err := c.Exec(q); err != nil {
				return err
			}
		}
	}
	return nil
}

func quotePGIdent(name string) string {
	if name == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func ensurePostgresFullText(c *Conn) error {
	if c == nil || c.B != BackendPostgres {
		return nil
	}
	for _, stmt := range postgresFTSStatements {
		if _, err := c.Exec(stmt); err != nil {
			return fmt.Errorf("postgres fts: %w", err)
		}
	}
	return nil
}
