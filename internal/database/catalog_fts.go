package database

import (
	"database/sql"
	"strings"
)

var catalogFTS5Statements = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS catalog_fts USING fts5(
	title,
	synopsis,
	alt_titles
)`,
	`INSERT INTO catalog_fts(rowid, title, synopsis, alt_titles)
	 SELECT id, title, ifnull(synopsis,''), ifnull(alt_titles,'') FROM catalog`,
	`CREATE TRIGGER IF NOT EXISTS trg_catalog_fts_ai AFTER INSERT ON catalog BEGIN
	INSERT INTO catalog_fts(rowid, title, synopsis, alt_titles)
	VALUES (new.id, new.title, ifnull(new.synopsis,''), ifnull(new.alt_titles,''));
END`,
	`CREATE TRIGGER IF NOT EXISTS trg_catalog_fts_ad AFTER DELETE ON catalog BEGIN
	INSERT INTO catalog_fts(catalog_fts, rowid) VALUES('delete', old.id);
END`,
	`CREATE TRIGGER IF NOT EXISTS trg_catalog_fts_au AFTER UPDATE ON catalog BEGIN
	INSERT INTO catalog_fts(catalog_fts, rowid) VALUES('delete', old.id);
	INSERT INTO catalog_fts(rowid, title, synopsis, alt_titles)
	VALUES (new.id, new.title, ifnull(new.synopsis,''), ifnull(new.alt_titles,''));
END`,
}

var postgresCatalogFTSStatements = []string{
	`ALTER TABLE catalog ADD COLUMN IF NOT EXISTS catalog_search_vector tsvector
		GENERATED ALWAYS AS (
			to_tsvector('simple',
				coalesce(title, '') || ' ' || coalesce(synopsis, '') || ' ' || coalesce(alt_titles, '')
			)
		) STORED`,
	`CREATE INDEX IF NOT EXISTS catalog_search_vector_gin ON catalog USING gin (catalog_search_vector)`,
}

// ensureCatalogFTS creates catalog full-text indexes when supported.
func ensureCatalogFTS(c *Conn) error {
	if c == nil {
		return nil
	}
	if c.B == BackendPostgres {
		for _, stmt := range postgresCatalogFTSStatements {
			if _, err := c.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	}
	db := c.Std()
	if db == nil || !fts5CompileOption(db) {
		return nil
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='catalog_fts'`).Scan(&n); err != nil {
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
	for _, stmt := range catalogFTS5Statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func catalogFTSEnabledSQLite(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'catalog_fts'`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func catalogFTSEnabledPostgres(c *Conn) bool {
	if c == nil {
		return false
	}
	var n int
	if err := c.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'catalog' AND column_name = 'catalog_search_vector'`,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// CatalogFTSEnabled reports whether catalog full-text search is available.
func CatalogFTSEnabled(c *Conn) bool {
	if c == nil {
		return false
	}
	if c.B == BackendPostgres {
		return catalogFTSEnabledPostgres(c)
	}
	return catalogFTSEnabledSQLite(c.Std())
}

// CatalogSearchRow is one local catalog FTS hit.
type CatalogSearchRow struct {
	ID          int64
	Source      string
	ExternalID  string
	Title       string
	ReadingType string
	ImageURL    string
}

// SearchCatalogFTS queries the local catalog index; matchExpr is for SQLite FTS5 only.
func SearchCatalogFTS(c *Conn, query string, matchExpr string, limit int) ([]CatalogSearchRow, error) {
	query = strings.TrimSpace(query)
	if c == nil || query == "" || limit <= 0 {
		return nil, nil
	}
	if c.B == BackendPostgres {
		rows, err := c.Query(
			`SELECT id, source, COALESCE(external_id,''), title, reading_type, COALESCE(image_url,'')
			 FROM catalog
			 WHERE catalog_search_vector @@ plainto_tsquery('simple', ?)
			 ORDER BY title LIMIT ?`,
			query, limit,
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		return scanCatalogSearchRows(rows)
	}
	if matchExpr == "" {
		return nil, nil
	}
	rows, err := c.Query(
		`SELECT c.id, c.source, COALESCE(c.external_id,''), c.title, c.reading_type, COALESCE(c.image_url,'')
		 FROM catalog c
		 INNER JOIN catalog_fts ON catalog_fts.rowid = c.id
		 WHERE catalog_fts MATCH ?
		 ORDER BY c.title LIMIT ?`,
		matchExpr, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanCatalogSearchRows(rows)
}

func scanCatalogSearchRows(rows *sql.Rows) ([]CatalogSearchRow, error) {
	var out []CatalogSearchRow
	for rows.Next() {
		var r CatalogSearchRow
		if err := rows.Scan(&r.ID, &r.Source, &r.ExternalID, &r.Title, &r.ReadingType, &r.ImageURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
