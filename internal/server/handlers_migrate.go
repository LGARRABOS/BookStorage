package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"bookstorage/internal/database"
)

// HandleAdminMigratePostgres shows the migration wizard (SQLite only, superadmin).
func (a *App) HandleAdminMigratePostgres(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.Settings.UsePostgres() {
		http.Redirect(w, r, "/admin/database", http.StatusFound)
		return
	}
	a.renderTemplate(w, r, "admin_migrate_postgres", a.mergeData(r, nil))
}

// HandleAPIAdminMigratePostgresTest checks connectivity to the target PostgreSQL server.
func (a *App) HandleAPIAdminMigratePostgresTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req struct {
		PostgresURL string `json:"postgres_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	u := strings.TrimSpace(req.PostgresURL)
	if u == "" {
		a.apiWriteError(w, http.StatusBadRequest, "missing_url")
		return
	}
	db, err := database.OpenPostgresURL(u)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "connect_failed")
		return
	}
	defer func() { _ = db.Close() }()
	var version string
	if err := db.QueryRow(`SELECT version()`).Scan(&version); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "query_failed")
		return
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
}

// HandleAPIAdminMigratePostgresRun copies SQLite data to PostgreSQL, updates .env, removes the SQLite file, then exits the process.
func (a *App) HandleAPIAdminMigratePostgresRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if a.Settings.UsePostgres() {
		a.apiWriteError(w, http.StatusBadRequest, "already_postgres")
		return
	}
	var req struct {
		PostgresURL string `json:"postgres_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	u := strings.TrimSpace(req.PostgresURL)
	if u == "" {
		a.apiWriteError(w, http.StatusBadRequest, "missing_url")
		return
	}
	if err := database.MigrateSQLiteToPostgres(a.DB, u, a.Settings); err != nil {
		log.Printf("migrate postgres: %v", err)
		a.apiWriteError(w, http.StatusBadRequest, "migrate_failed")
		return
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	sqlitePath := a.Settings.Database
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = a.DB.Close()
		if sqlitePath != "" && sqlitePath != ":memory:" {
			_ = os.Remove(sqlitePath)
			_ = os.Remove(sqlitePath + "-wal")
			_ = os.Remove(sqlitePath + "-shm")
		}
		log.Printf("migration complete: exiting for restart")
		os.Exit(0)
	}()
}
