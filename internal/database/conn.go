package database

import (
	"context"
	"database/sql"
)

// Backend is the active SQL dialect for placeholder rewriting.
type Backend int

const (
	BackendSQLite Backend = iota
	BackendPostgres
)

// NewSQLiteConn wraps an existing SQLite *sql.DB (e.g. in tests).
func NewSQLiteConn(db *sql.DB) *Conn {
	if db == nil {
		return nil
	}
	return &Conn{sql: db, B: BackendSQLite}
}

// Conn wraps *sql.DB with optional PostgreSQL placeholder rewriting (? → $n).
type Conn struct {
	sql *sql.DB
	B   Backend
}

func (c *Conn) rebind(q string) string {
	if c == nil || c.B != BackendPostgres {
		return q
	}
	return RebindQuestionToDollar(q)
}

// Std returns the underlying *sql.DB (for drivers/tests that need the raw handle).
func (c *Conn) Std() *sql.DB {
	if c == nil {
		return nil
	}
	return c.sql
}

func (c *Conn) Close() error {
	if c == nil || c.sql == nil {
		return nil
	}
	return c.sql.Close()
}

func (c *Conn) PingContext(ctx context.Context) error {
	if c == nil || c.sql == nil {
		return sql.ErrConnDone
	}
	return c.sql.PingContext(ctx)
}

func (c *Conn) Exec(query string, args ...any) (sql.Result, error) {
	return c.sql.Exec(c.rebind(query), args...)
}

func (c *Conn) Query(query string, args ...any) (*sql.Rows, error) {
	return c.sql.Query(c.rebind(query), args...)
}

func (c *Conn) QueryRow(query string, args ...any) *sql.Row {
	return c.sql.QueryRow(c.rebind(query), args...)
}

func (c *Conn) Begin() (*Tx, error) {
	tx, err := c.sql.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, b: c.B}, nil
}

// Tx is a database transaction with the same placeholder rules as Conn.
type Tx struct {
	Tx *sql.Tx
	b  Backend
}

func (t *Tx) rebind(q string) string {
	if t == nil || t.b != BackendPostgres {
		return q
	}
	return RebindQuestionToDollar(q)
}

func (t *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(t.rebind(query), args...)
}

func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(t.rebind(query), args...)
}

func (t *Tx) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(t.rebind(query))
}

func (t *Tx) Commit() error {
	if t == nil || t.Tx == nil {
		return nil
	}
	return t.Tx.Commit()
}

func (t *Tx) Rollback() error {
	if t == nil || t.Tx == nil {
		return nil
	}
	return t.Tx.Rollback()
}
