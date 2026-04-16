package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bookstorage/internal/config"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// Open opens SQLite or PostgreSQL according to settings.
func Open(settings *config.Settings) (*Conn, error) {
	if settings.UsePostgres() {
		db, err := sql.Open("postgres", settings.PostgresURL)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("postgres ping: %w", err)
		}
		return &Conn{sql: db, B: BackendPostgres}, nil
	}

	db, err := sql.Open("sqlite3", sqliteDataSourceName(settings.Database))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Conn{sql: db, B: BackendSQLite}, nil
}

// OpenPostgresURL opens a PostgreSQL pool for one-off operations (e.g. migration test).
func OpenPostgresURL(dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("empty postgres dsn")
	}
	var err error
	dsn, err = config.NormalizePostgresURLForLibPQ(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
