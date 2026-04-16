package database

import (
	"context"
	"database/sql"
	"time"
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

func (c *Conn) Ping() error {
	if c == nil || c.sql == nil {
		return sql.ErrConnDone
	}
	return c.sql.Ping()
}

func (c *Conn) PingContext(ctx context.Context) error {
	if c == nil || c.sql == nil {
		return sql.ErrConnDone
	}
	return c.sql.PingContext(ctx)
}

func (c *Conn) SetMaxOpenConns(n int) {
	if c == nil || c.sql == nil {
		return
	}
	c.sql.SetMaxOpenConns(n)
}

func (c *Conn) SetMaxIdleConns(n int) {
	if c == nil || c.sql == nil {
		return
	}
	c.sql.SetMaxIdleConns(n)
}

func (c *Conn) SetConnMaxLifetime(d time.Duration) {
	if c == nil || c.sql == nil {
		return
	}
	c.sql.SetConnMaxLifetime(d)
}

func (c *Conn) Exec(query string, args ...any) (sql.Result, error) {
	return c.sql.Exec(c.rebind(query), args...)
}

func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.sql.ExecContext(ctx, c.rebind(query), args...)
}

func (c *Conn) Query(query string, args ...any) (*sql.Rows, error) {
	return c.sql.Query(c.rebind(query), args...)
}

func (c *Conn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.sql.QueryContext(ctx, c.rebind(query), args...)
}

func (c *Conn) QueryRow(query string, args ...any) *sql.Row {
	return c.sql.QueryRow(c.rebind(query), args...)
}

func (c *Conn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.sql.QueryRowContext(ctx, c.rebind(query), args...)
}

func (c *Conn) Begin() (*Tx, error) {
	tx, err := c.sql.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, b: c.B}, nil
}

func (c *Conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := c.sql.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, b: c.B}, nil
}

func (c *Conn) Prepare(query string) (*sql.Stmt, error) {
	return c.sql.Prepare(c.rebind(query))
}

func (c *Conn) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return c.sql.PrepareContext(ctx, c.rebind(query))
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

func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, t.rebind(query), args...)
}

func (t *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(t.rebind(query), args...)
}

func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, t.rebind(query), args...)
}

func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(t.rebind(query), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, t.rebind(query), args...)
}

func (t *Tx) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(t.rebind(query))
}

func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, t.rebind(query))
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
