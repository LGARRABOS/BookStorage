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
		synopsis TEXT,
		alt_titles TEXT,
		genres TEXT,
		tags TEXT,
		fetched_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS reading_sites (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		base_url TEXT NOT NULL,
		last_probe_at TIMESTAMPTZ,
		probe_status TEXT DEFAULT 'unknown',
		probe_http_status INTEGER,
		probe_detail TEXT
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
		notify_new_chapters INTEGER NOT NULL DEFAULT 1,
		reading_site_id BIGINT REFERENCES reading_sites(id),
		started_at TIMESTAMPTZ,
		last_chapter_at TIMESTAMPTZ,
		finished_at TIMESTAMPTZ,
		link_probe_status TEXT DEFAULT 'unknown',
		link_probe_at TIMESTAMPTZ,
		link_probe_http_status INTEGER,
		link_probe_detail TEXT
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
	`CREATE TABLE IF NOT EXISTS reading_activity_daily (
		user_id BIGINT NOT NULL REFERENCES users(id),
		day TEXT NOT NULL,
		chapter_increments INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (user_id, day)
	)`,
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		scopes TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at TIMESTAMPTZ,
		revoked_at TIMESTAMPTZ
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_endpoints (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		url TEXT NOT NULL,
		secret TEXT NOT NULL,
		events TEXT NOT NULL DEFAULT '[]',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_deliveries (
		id BIGSERIAL PRIMARY KEY,
		endpoint_id BIGINT NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
		event TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		next_retry_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS user_catalog_blocklist (
		user_id BIGINT NOT NULL REFERENCES users(id),
		label_type TEXT NOT NULL,
		label_name TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, label_type, label_name)
	)`,
	`CREATE TABLE IF NOT EXISTS admin_audit_log (
		id BIGSERIAL PRIMARY KEY,
		actor_user_id BIGINT NOT NULL REFERENCES users(id),
		action TEXT NOT NULL,
		target_type TEXT,
		target_id TEXT,
		detail_json TEXT,
		ip TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS webauthn_credentials (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		credential_id BYTEA NOT NULL UNIQUE,
		public_key BYTEA NOT NULL,
		sign_count INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at TIMESTAMPTZ
	)`,
	`CREATE TABLE IF NOT EXISTS password_reset_tokens (
		token_hash TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id),
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMPTZ NOT NULL,
		used_at TIMESTAMPTZ
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
	`CREATE INDEX IF NOT EXISTS idx_reading_sites_user_id ON reading_sites(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_csv_import_sessions_user ON csv_import_sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at_unix)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user_revoked ON sessions(user_id, revoked_at)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_user_id ON webhook_endpoints(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint_id ON webhook_deliveries(endpoint_id)`,
	`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status)`,
	`CREATE INDEX IF NOT EXISTS idx_user_catalog_blocklist_user_id ON user_catalog_blocklist(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_admin_audit_log_created_at ON admin_audit_log(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_admin_audit_log_actor ON admin_audit_log(actor_user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id ON webauthn_credentials(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at)`,
}

// postgresSchemaAfterExtraColumns runs after ALTER TABLE ... ADD COLUMN for works, so indexes
// on columns that may not exist on upgraded DBs must live here (not in postgresSchemaStatements).
var postgresSchemaAfterExtraColumns = []string{
	`CREATE INDEX IF NOT EXISTS idx_works_reading_site_id ON works(reading_site_id)`,
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
	if err := ensurePostgresExtraColumns(c); err != nil {
		return err
	}
	for _, stmt := range postgresSchemaAfterExtraColumns {
		if _, err := c.Exec(stmt); err != nil {
			return fmt.Errorf("postgres schema: %w", err)
		}
	}
	return nil
}

var postgresProfileColumns = map[string]string{
	"display_name": "TEXT",
	"email":        "TEXT",
	"bio":          "TEXT",
	"avatar_path":  "TEXT",
	"is_public":    "INTEGER DEFAULT 1",
}

var postgresCatalogColumns = map[string]string{
	"synopsis":   "TEXT",
	"alt_titles": "TEXT",
	"genres":     "TEXT",
	"tags":       "TEXT",
	"fetched_at": "TIMESTAMPTZ",
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
	"reading_site_id":        "BIGINT REFERENCES reading_sites(id)",
	"started_at":             "TIMESTAMPTZ",
	"last_chapter_at":        "TIMESTAMPTZ",
	"finished_at":            "TIMESTAMPTZ",
	"link_probe_status":      "TEXT DEFAULT 'unknown'",
	"link_probe_at":          "TIMESTAMPTZ",
	"link_probe_http_status": "INTEGER",
	"link_probe_detail":      "TEXT",
}

func ensurePostgresExtraColumns(c *Conn) error {
	if err := ensureColumnsPostgres(c, "users", postgresProfileColumns); err != nil {
		return err
	}
	if err := ensureColumnsPostgres(c, "catalog", postgresCatalogColumns); err != nil {
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
