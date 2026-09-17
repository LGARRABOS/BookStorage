package database

import (
	"database/sql"
	"fmt"
	"strings"

	"bookstorage/internal/config"
)

type sqliteCopySpec struct {
	table string
	cols  []string
}

// sqliteToPostgresCopyTables is the ordered list of application tables copied
// during SQLite → PostgreSQL migration (parents before children).
var sqliteToPostgresCopyTables = []sqliteCopySpec{
	{table: "users", cols: []string{
		"id", "username", "password", "validated", "is_admin", "is_superadmin",
		"display_name", "email", "bio", "avatar_path", "is_public", "google_sub", "google_email", "home_section",
	}},
	{table: "catalog", cols: []string{
		"id", "title", "reading_type", "image_url", "source", "external_id",
		"synopsis", "alt_titles", "genres", "tags", "fetched_at", "created_at",
	}},
	{table: "reading_sites", cols: []string{
		"id", "user_id", "name", "base_url", "last_probe_at", "probe_status", "probe_http_status", "probe_detail",
	}},
	{table: "works", cols: []string{
		"id", "title", "chapter", "link", "status", "image_path", "reading_type", "user_id",
		"rating", "notes", "updated_at", "is_adult", "catalog_id", "anilist_enrich_opt_out",
		"parent_work_id", "series_sort", "notify_new_chapters", "reading_site_id",
		"started_at", "last_chapter_at", "finished_at",
		"link_probe_status", "link_probe_at", "link_probe_http_status", "link_probe_detail",
	}},
	{table: "anime_works", cols: []string{
		"id", "title", "episode", "total_episodes", "status", "anime_type", "link", "image_path",
		"rating", "notes", "is_adult", "source", "external_id", "user_id", "updated_at", "started_at", "finished_at",
	}},
	{table: "bd_works", cols: []string{
		"id", "title", "tome", "total_tomes", "status", "bd_type", "link", "image_path",
		"rating", "notes", "is_adult", "source", "external_id", "isbn", "user_id", "updated_at", "started_at", "finished_at",
	}},
	{table: "manga_phys_works", cols: []string{
		"id", "title", "tome", "total_tomes", "status", "manga_type", "link", "image_path",
		"rating", "notes", "is_adult", "source", "external_id", "user_id", "updated_at", "started_at", "finished_at",
	}},
	{table: "library_furniture", cols: []string{
		"id", "user_id", "name", "room_label", "sort_order", "created_at", "updated_at",
	}},
	{table: "library_shelves", cols: []string{
		"id", "furniture_id", "label", "case_count", "books_per_case", "sort_order",
	}},
	{table: "library_placements", cols: []string{
		"id", "user_id", "shelf_id", "case_num", "position", "media_kind", "work_id", "volume", "created_at", "updated_at",
	}},
	{table: "dismissed_recommendations", cols: []string{"id", "user_id", "source", "external_id", "created_at"}},
	{table: "sessions", cols: []string{
		"id", "user_id", "token_hash", "created_at", "last_seen_at", "expires_at", "ip", "user_agent", "revoked_at",
	}},
	{table: "translation_cache", cols: []string{"source_hash", "target_lang", "translated_text", "created_at"}},
	{table: "csv_import_sessions", cols: []string{"id", "user_id", "raw_csv", "created_at"}},
	{table: "oauth_states", cols: []string{"state_hash", "purpose", "user_id", "next", "expires_at_unix", "code_verifier"}},
	{table: "reading_activity_daily", cols: []string{"user_id", "day", "chapter_increments"}},
	{table: "api_tokens", cols: []string{
		"id", "user_id", "name", "token_hash", "scopes", "created_at", "last_used_at", "revoked_at", "expires_at",
	}},
	{table: "login_attempts", cols: []string{"username", "fail_count", "locked_until"}},
	{table: "webhook_endpoints", cols: []string{"id", "user_id", "url", "secret", "events", "enabled", "created_at"}},
	{table: "webhook_deliveries", cols: []string{
		"id", "endpoint_id", "event", "payload", "status", "attempts", "next_retry_at", "created_at",
	}},
	{table: "user_catalog_blocklist", cols: []string{"user_id", "label_type", "label_name", "created_at"}},
	{table: "admin_audit_log", cols: []string{
		"id", "actor_user_id", "action", "target_type", "target_id", "detail_json", "ip", "created_at",
	}},
	{table: "webauthn_credentials", cols: []string{
		"id", "user_id", "credential_id", "public_key", "sign_count", "name",
		"backup_eligible", "backup_state", "created_at", "last_used_at",
	}},
	{table: "password_reset_tokens", cols: []string{"token_hash", "user_id", "created_at", "expires_at", "used_at"}},
	{table: "webauthn_challenges", cols: []string{"challenge_key", "user_id", "session_data", "expires_at"}},
}

var postgresSerialTables = []string{
	"users", "catalog", "reading_sites", "works", "anime_works", "bd_works", "manga_phys_works",
	"library_furniture", "library_shelves", "library_placements", "dismissed_recommendations",
	"sessions", "api_tokens", "webhook_endpoints", "webhook_deliveries", "admin_audit_log",
	"webauthn_credentials",
}

const postgresTruncateForMigration = `TRUNCATE TABLE
	webhook_deliveries, webhook_endpoints,
	library_placements, library_shelves, library_furniture,
	works, anime_works, bd_works, manga_phys_works,
	reading_activity_daily, api_tokens, login_attempts,
	user_catalog_blocklist, admin_audit_log,
	webauthn_credentials, webauthn_challenges, password_reset_tokens,
	dismissed_recommendations, sessions, csv_import_sessions,
	oauth_states, translation_cache, reading_sites, catalog, users,
	schema_migrations
RESTART IDENTITY CASCADE`

// MigrateSQLiteToPostgres copies all application data from the SQLite connection into an empty
// PostgreSQL database reachable via pgDSN, then applies migration markers and full-text setup.
// It does not modify .env: the caller must persist BOOKSTORAGE_POSTGRES_URL (returned normalized DSN)
// as the last step before responding OK, so a failed .env write leaves SQLite intact and the app reachable.
func MigrateSQLiteToPostgres(sqliteConn *Conn, pgDSN string) (normalizedDSN string, err error) {
	if sqliteConn == nil || sqliteConn.B != BackendSQLite {
		return "", fmt.Errorf("migration requires an active SQLite connection")
	}
	pgDSN = strings.TrimSpace(pgDSN)
	if pgDSN == "" {
		return "", fmt.Errorf("empty postgres URL")
	}
	norm, err := config.NormalizePostgresURLForLibPQ(pgDSN)
	if err != nil {
		return "", err
	}
	pg, err := sql.Open("postgres", norm)
	if err != nil {
		return "", err
	}
	defer func() { _ = pg.Close() }()
	if err := pg.Ping(); err != nil {
		return "", fmt.Errorf("postgres ping: %w", err)
	}
	pgConn := &Conn{sql: pg, B: BackendPostgres}

	if err := ensurePostgresSchema(pgConn); err != nil {
		return "", fmt.Errorf("target schema: %w", err)
	}
	if _, err := pgConn.Exec(postgresTruncateForMigration); err != nil {
		return "", fmt.Errorf("truncate target: %w", err)
	}

	sl := sqliteConn.Std()
	for _, spec := range sqliteToPostgresCopyTables {
		if err := copyTable(sl, pgConn, spec.table, spec.cols); err != nil {
			return "", err
		}
	}

	if err := applyPostgresMigrationMarkers(pgConn); err != nil {
		return "", err
	}
	if err := ensurePostgresFullText(pgConn); err != nil {
		return "", err
	}

	if err := syncPostgresSequences(pgConn); err != nil {
		return "", err
	}
	if err := verifyMigrationCounts(sl, pgConn); err != nil {
		return "", err
	}

	return norm, nil
}

func verifyMigrationCounts(sl *sql.DB, pg *Conn) error {
	for _, spec := range sqliteToPostgresCopyTables {
		t := spec.table
		var a, b int
		if err := sl.QueryRow(`SELECT COUNT(*) FROM ` + quoteSQLiteIdentRaw(t)).Scan(&a); err != nil {
			return fmt.Errorf("sqlite count %s: %w", t, err)
		}
		if err := pg.QueryRow(`SELECT COUNT(*) FROM ` + quoteSQLiteIdentRaw(t)).Scan(&b); err != nil {
			return fmt.Errorf("postgres count %s: %w", t, err)
		}
		if a != b {
			return fmt.Errorf("row count mismatch for %s: sqlite=%d postgres=%d", t, a, b)
		}
	}
	return nil
}

func syncPostgresSequences(pg *Conn) error {
	for _, tbl := range postgresSerialTables {
		q := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE((SELECT MAX(id) FROM %s), 1), true)`,
			tbl, quoteSQLiteIdentRaw(tbl),
		)
		if _, err := pg.Exec(q); err != nil {
			return fmt.Errorf("setval %s: %w", tbl, err)
		}
	}
	return nil
}

func quoteSQLiteIdentRaw(name string) string {
	if name == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func copyTable(sl *sql.DB, pg *Conn, table string, columns []string) error {
	if sl == nil || pg == nil {
		return fmt.Errorf("copy %s: nil connection", table)
	}
	if len(columns) == 0 {
		return fmt.Errorf("copy %s: no columns", table)
	}
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = quoteSQLiteIdentRaw(c)
		placeholders[i] = "?"
	}
	sel := "SELECT " + strings.Join(quoted, ", ") + " FROM " + quoteSQLiteIdentRaw(table)
	ins := "INSERT INTO " + quoteSQLiteIdentRaw(table) + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	rows, err := sl.Query(sel)
	if err != nil {
		return fmt.Errorf("select %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	vals := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("scan %s: %w", table, err)
		}
		args := make([]any, len(vals))
		for i, v := range vals {
			args[i] = copiedSQLValue(v)
		}
		if _, err := pg.Exec(ins, args...); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows %s: %w", table, err)
	}
	return nil
}

func copiedSQLValue(v any) any {
	switch t := v.(type) {
	case []byte:
		out := make([]byte, len(t))
		copy(out, t)
		return out
	default:
		return v
	}
}
