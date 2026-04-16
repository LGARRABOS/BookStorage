package database

import (
	"database/sql"
	"fmt"
	"strings"

	"bookstorage/internal/config"
)

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
	clearPostgresUserData := []string{
		`TRUNCATE oauth_states, csv_import_sessions, translation_cache, sessions, dismissed_recommendations, works, catalog, users, schema_migrations RESTART IDENTITY CASCADE`,
	}
	for _, q := range clearPostgresUserData {
		if _, err := pgConn.Exec(q); err != nil {
			return "", fmt.Errorf("truncate target: %w", err)
		}
	}

	sl := sqliteConn.Std()

	if err := copyUsers(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyCatalog(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyWorks(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyDismissed(sl, pgConn); err != nil {
		return "", err
	}
	if err := copySessions(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyTranslationCache(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyCSVImportSessions(sl, pgConn); err != nil {
		return "", err
	}
	if err := copyOAuthStates(sl, pgConn); err != nil {
		return "", err
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
	tables := []string{"users", "catalog", "works", "dismissed_recommendations", "sessions", "translation_cache", "csv_import_sessions", "oauth_states"}
	for _, t := range tables {
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
	for _, tbl := range []string{"users", "catalog", "works", "dismissed_recommendations", "sessions"} {
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

func copyUsers(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, username, password, validated, is_admin, is_superadmin, display_name, email, bio, avatar_path, is_public, google_sub, google_email FROM users`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var username string
		var password sql.NullString
		var validated, isAdmin, isSuper, isPublic int
		var displayName, email, bio, avatarPath, googleSub, googleEmail sql.NullString
		if err := rows.Scan(&id, &username, &password, &validated, &isAdmin, &isSuper, &displayName, &email, &bio, &avatarPath, &isPublic, &googleSub, &googleEmail); err != nil {
			return err
		}
		_, err := pg.Exec(
			`INSERT INTO users (id, username, password, validated, is_admin, is_superadmin, display_name, email, bio, avatar_path, is_public, google_sub, google_email)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, username, nullStr(password), validated, isAdmin, isSuper, nullStr(displayName), nullStr(email), nullStr(bio), nullStr(avatarPath), isPublic, nullStr(googleSub), nullStr(googleEmail),
		)
		if err != nil {
			return fmt.Errorf("insert users: %w", err)
		}
	}
	return rows.Err()
}

func copyCatalog(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, title, reading_type, image_url, source, external_id, created_at FROM catalog`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var title, readingType, source string
		var imageURL, externalID sql.NullString
		var createdAt sql.NullString
		if err := rows.Scan(&id, &title, &readingType, &imageURL, &source, &externalID, &createdAt); err != nil {
			return err
		}
		_, err := pg.Exec(
			`INSERT INTO catalog (id, title, reading_type, image_url, source, external_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP))`,
			id, title, readingType, nullStr(imageURL), source, nullStr(externalID), nullStr(createdAt),
		)
		if err != nil {
			return fmt.Errorf("insert catalog: %w", err)
		}
	}
	return rows.Err()
}

func copyWorks(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, title, chapter, link, status, image_path, reading_type, user_id, rating, notes, updated_at, is_adult, catalog_id, anilist_enrich_opt_out, parent_work_id, series_sort FROM works`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, chapter, userID, rating, isAdult, anilistOpt, seriesSort int64
		var title string
		var link, status, imagePath, readingType, notes, updatedAt sql.NullString
		var catalogID, parentID sql.NullInt64
		if err := rows.Scan(&id, &title, &chapter, &link, &status, &imagePath, &readingType, &userID, &rating, &notes, &updatedAt, &isAdult, &catalogID, &anilistOpt, &parentID, &seriesSort); err != nil {
			return err
		}
		_, err := pg.Exec(
			`INSERT INTO works (id, title, chapter, link, status, image_path, reading_type, user_id, rating, notes, updated_at, is_adult, catalog_id, anilist_enrich_opt_out, parent_work_id, series_sort)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, title, chapter, nullStr(link), nullStr(status), nullStr(imagePath), nullStr(readingType), userID, rating, nullStr(notes), nullStr(updatedAt), isAdult, nullInt64(catalogID), anilistOpt, nullInt64(parentID), seriesSort,
		)
		if err != nil {
			return fmt.Errorf("insert works id=%d: %w", id, err)
		}
	}
	return rows.Err()
}

func copyDismissed(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, user_id, source, external_id, created_at FROM dismissed_recommendations`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, userID int64
		var source, externalID string
		var createdAt sql.NullString
		if err := rows.Scan(&id, &userID, &source, &externalID, &createdAt); err != nil {
			return err
		}
		if _, err := pg.Exec(
			`INSERT INTO dismissed_recommendations (id, user_id, source, external_id, created_at)
			 VALUES (?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP))`,
			id, userID, source, externalID, nullStr(createdAt),
		); err != nil {
			return fmt.Errorf("insert dismissed: %w", err)
		}
	}
	return rows.Err()
}

func copySessions(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent, revoked_at FROM sessions`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, userID int64
		var tokenHash string
		var createdAt, lastSeen, expiresAt, ip, ua, revoked sql.NullString
		if err := rows.Scan(&id, &userID, &tokenHash, &createdAt, &lastSeen, &expiresAt, &ip, &ua, &revoked); err != nil {
			return err
		}
		var revokedAny any
		if revoked.Valid && strings.TrimSpace(revoked.String) != "" {
			revokedAny = revoked.String
		}
		if _, err := pg.Exec(
			`INSERT INTO sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent, revoked_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, userID, tokenHash, nullStr(createdAt), nullStr(lastSeen), nullStr(expiresAt), nullStr(ip), nullStr(ua), revokedAny,
		); err != nil {
			return fmt.Errorf("insert sessions: %w", err)
		}
	}
	return rows.Err()
}

func copyTranslationCache(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT source_hash, target_lang, translated_text, created_at FROM translation_cache`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sh, lang, text string
		var createdAt sql.NullString
		if err := rows.Scan(&sh, &lang, &text, &createdAt); err != nil {
			return err
		}
		if _, err := pg.Exec(
			`INSERT INTO translation_cache (source_hash, target_lang, translated_text, created_at)
			 VALUES (?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP))`,
			sh, lang, text, nullStr(createdAt),
		); err != nil {
			return fmt.Errorf("insert translation_cache: %w", err)
		}
	}
	return rows.Err()
}

func copyCSVImportSessions(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT id, user_id, raw_csv, created_at FROM csv_import_sessions`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var userID int64
		var raw string
		var createdAt sql.NullString
		if err := rows.Scan(&id, &userID, &raw, &createdAt); err != nil {
			return err
		}
		if _, err := pg.Exec(
			`INSERT INTO csv_import_sessions (id, user_id, raw_csv, created_at)
			 VALUES (?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP))`,
			id, userID, raw, nullStr(createdAt),
		); err != nil {
			return fmt.Errorf("insert csv_import_sessions: %w", err)
		}
	}
	return rows.Err()
}

func copyOAuthStates(sl *sql.DB, pg *Conn) error {
	rows, err := sl.Query(`SELECT state_hash, purpose, user_id, next, expires_at_unix, code_verifier FROM oauth_states`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var hash, purpose, next, verifier string
		var userID sql.NullInt64
		var exp int64
		if err := rows.Scan(&hash, &purpose, &userID, &next, &exp, &verifier); err != nil {
			return err
		}
		if _, err := pg.Exec(
			`INSERT INTO oauth_states (state_hash, purpose, user_id, next, expires_at_unix, code_verifier)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			hash, purpose, nullInt64(userID), next, exp, verifier,
		); err != nil {
			return fmt.Errorf("insert oauth_states: %w", err)
		}
	}
	return rows.Err()
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullInt64(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}
