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
	{Version: 5, Name: "indexes_for_hot_paths", Up: `
CREATE INDEX IF NOT EXISTS idx_works_user_id ON works(user_id);
CREATE INDEX IF NOT EXISTS idx_works_user_status ON works(user_id, status);
CREATE INDEX IF NOT EXISTS idx_works_user_type ON works(user_id, reading_type);
CREATE INDEX IF NOT EXISTS idx_works_user_updated_at ON works(user_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_works_user_title ON works(user_id, title);
CREATE INDEX IF NOT EXISTS idx_works_catalog_id ON works(catalog_id);
CREATE INDEX IF NOT EXISTS idx_catalog_source_external_id ON catalog(source, external_id);
CREATE INDEX IF NOT EXISTS idx_users_validated_public ON users(validated, is_public);
`},
	// FTS5 is applied in ensureWorksFTS5 (after migrations) so builds without ENABLE_FTS5 (e.g. some Windows SQLite) still pass tests.
	{Version: 6, Name: "works_fts5_placeholder", Up: ""},
	{Version: 7, Name: "works_series_parent", Up: `
ALTER TABLE works ADD COLUMN parent_work_id INTEGER REFERENCES works(id);
ALTER TABLE works ADD COLUMN series_sort INTEGER DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_works_parent_work_id ON works(parent_work_id);
`},
	{Version: 8, Name: "csv_import_sessions", Up: `
CREATE TABLE IF NOT EXISTS csv_import_sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	raw_csv TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_csv_import_sessions_user ON csv_import_sessions(user_id);
`},
	{Version: 9, Name: "google_oauth_users_oauth_states", OutsideTx: true, Up: `
CREATE TABLE IF NOT EXISTS oauth_states (
	state_hash TEXT PRIMARY KEY,
	purpose TEXT NOT NULL,
	user_id INTEGER,
	next TEXT,
	expires_at_unix INTEGER NOT NULL,
	code_verifier TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at_unix);
CREATE TABLE users_new (
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
);
INSERT INTO users_new (id, username, password, validated, is_admin, is_superadmin, display_name, email, bio, avatar_path, is_public, google_sub, google_email)
SELECT id, username, password, validated, is_admin, is_superadmin, display_name, email, bio, avatar_path, is_public, NULL, NULL FROM users;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
CREATE INDEX IF NOT EXISTS idx_users_validated_public ON users(validated, is_public);
`},
	{Version: 10, Name: "notify_new_chapters_placeholder", Up: ""},
	{Version: 11, Name: "reading_sites", Up: `
CREATE TABLE IF NOT EXISTS reading_sites (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	base_url TEXT NOT NULL,
	last_probe_at DATETIME,
	probe_status TEXT DEFAULT 'unknown',
	probe_http_status INTEGER,
	probe_detail TEXT,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_reading_sites_user_id ON reading_sites(user_id);
`},
	{Version: 12, Name: "reading_dates_placeholder", Up: ""},
	{Version: 13, Name: "reading_activity_daily", Up: `
CREATE TABLE IF NOT EXISTS reading_activity_daily (
	user_id INTEGER NOT NULL,
	day TEXT NOT NULL,
	chapter_increments INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, day),
	FOREIGN KEY (user_id) REFERENCES users(id)
);
`},
	{Version: 14, Name: "reading_types_chapter_formats", Up: `
UPDATE works SET reading_type = 'Webtoon' WHERE reading_type IN ('Manhwa', 'Manhua');
UPDATE works SET reading_type = 'Light Novel' WHERE reading_type = 'Roman';
UPDATE works SET reading_type = 'Manga' WHERE reading_type IN ('BD', 'Autre', '18+');
UPDATE catalog SET reading_type = 'Webtoon' WHERE reading_type IN ('Manhwa', 'Manhua');
UPDATE catalog SET reading_type = 'Light Novel' WHERE reading_type = 'Roman';
UPDATE catalog SET reading_type = 'Manga' WHERE reading_type IN ('BD', 'Autre', '18+');
`},
	{Version: 15, Name: "api_tokens", Up: `
CREATE TABLE IF NOT EXISTS api_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	scopes TEXT NOT NULL DEFAULT '[]',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME,
	revoked_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
`},
	{Version: 16, Name: "webhook_endpoints_deliveries", Up: `
CREATE TABLE IF NOT EXISTS webhook_endpoints (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	url TEXT NOT NULL,
	secret TEXT NOT NULL,
	events TEXT NOT NULL DEFAULT '[]',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_user_id ON webhook_endpoints(user_id);
CREATE TABLE IF NOT EXISTS webhook_deliveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	endpoint_id INTEGER NOT NULL,
	event TEXT NOT NULL,
	payload TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	next_retry_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (endpoint_id) REFERENCES webhook_endpoints(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint_id ON webhook_deliveries(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status);
`},
	{Version: 17, Name: "user_catalog_blocklist", Up: `
CREATE TABLE IF NOT EXISTS user_catalog_blocklist (
	user_id INTEGER NOT NULL,
	label_type TEXT NOT NULL,
	label_name TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, label_type, label_name),
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_user_catalog_blocklist_user_id ON user_catalog_blocklist(user_id);
`},
	{Version: 18, Name: "catalog_metadata_and_catalog_fts5_placeholder", Up: ""},
	{Version: 19, Name: "works_link_probe", Up: ""},
	{Version: 20, Name: "admin_audit_log", Up: `
CREATE TABLE IF NOT EXISTS admin_audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	actor_user_id INTEGER NOT NULL,
	action TEXT NOT NULL,
	target_type TEXT,
	target_id TEXT,
	detail_json TEXT,
	ip TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (actor_user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_created_at ON admin_audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_actor ON admin_audit_log(actor_user_id);
`},
	{Version: 21, Name: "webauthn_credentials", Up: `
CREATE TABLE IF NOT EXISTS webauthn_credentials (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	credential_id BLOB NOT NULL UNIQUE,
	public_key BLOB NOT NULL,
	sign_count INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id ON webauthn_credentials(user_id);
`},
	{Version: 22, Name: "password_reset_tokens", Up: `
CREATE TABLE IF NOT EXISTS password_reset_tokens (
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);
`},
}

// LatestSchemaMigrationVersion is the highest numbered migration (SQLite and Postgres logical version).
const LatestSchemaMigrationVersion = 22

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
