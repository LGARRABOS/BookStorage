package database

import "database/sql"

var worksFTS5Statements = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS works_fts USING fts5(
	title,
	notes,
	link
)`,
	`INSERT INTO works_fts(rowid, title, notes, link) SELECT id, title, ifnull(notes,''), ifnull(link,'') FROM works`,
	`CREATE TRIGGER IF NOT EXISTS trg_works_fts_ai AFTER INSERT ON works BEGIN
	INSERT INTO works_fts(rowid, title, notes, link) VALUES (new.id, new.title, ifnull(new.notes,''), ifnull(new.link,''));
END`,
	`CREATE TRIGGER IF NOT EXISTS trg_works_fts_ad AFTER DELETE ON works BEGIN
	INSERT INTO works_fts(works_fts, rowid) VALUES('delete', old.id);
END`,
	`CREATE TRIGGER IF NOT EXISTS trg_works_fts_au AFTER UPDATE ON works BEGIN
	INSERT INTO works_fts(works_fts, rowid) VALUES('delete', old.id);
	INSERT INTO works_fts(rowid, title, notes, link) VALUES (new.id, new.title, ifnull(new.notes,''), ifnull(new.link,''));
END`,
}

// fts5CompileOption returns true if this SQLite build supports FTS5.
func fts5CompileOption(db *sql.DB) bool {
	if db == nil {
		return false
	}
	rows, err := db.Query(`PRAGMA compile_options`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var opt string
		if err := rows.Scan(&opt); err != nil {
			return false
		}
		if opt == "ENABLE_FTS5" {
			return true
		}
	}
	return false
}

// ensureWorksFTS5 creates works_fts and triggers when FTS5 is available; no-op otherwise.
func ensureWorksFTS5(db *sql.DB) error {
	if db == nil || !fts5CompileOption(db) {
		return nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='works_fts'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range worksFTS5Statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func worksFTSEnabledSQLite(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'works_fts'`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func worksFTSEnabledPostgres(c *Conn) bool {
	if c == nil {
		return false
	}
	var n int
	if err := c.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'works' AND column_name = 'works_fts_document'`,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// WorksFTSEnabled reports whether full-text search is available for works (FTS5 on SQLite, tsvector on Postgres).
func WorksFTSEnabled(c *Conn) bool {
	if c == nil {
		return false
	}
	if c.B == BackendPostgres {
		return worksFTSEnabledPostgres(c)
	}
	return worksFTSEnabledSQLite(c.Std())
}
