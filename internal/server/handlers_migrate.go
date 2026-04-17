package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"bookstorage/internal/config"
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
		a.apiWriteJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "connect_failed",
			"detail": err.Error(),
		})
		return
	}
	defer func() { _ = db.Close() }()
	var version string
	if err := db.QueryRow(`SELECT version()`).Scan(&version); err != nil {
		a.apiWriteJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "query_failed",
			"detail": err.Error(),
		})
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
	norm, err := database.MigrateSQLiteToPostgres(a.DB, u)
	if err != nil {
		log.Printf("migrate postgres: %v", err)
		a.apiWriteJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "migrate_failed",
			"detail": err.Error(),
		})
		return
	}
	envPath := strings.TrimSpace(a.Settings.EnvFilePath)
	if envPath == "" {
		a.apiWriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "migrate_failed",
			"detail": "resolved .env path is empty; cannot persist BOOKSTORAGE_POSTGRES_URL " +
				"(start the app with a readable .env path, e.g. -config /opt/bookstorage/.env)",
		})
		return
	}
	if err := config.MergeEnvKeys(envPath, map[string]string{"BOOKSTORAGE_POSTGRES_URL": norm}); err != nil {
		log.Printf("migrate postgres env: %v", err)
		detail := "update .env: " + err.Error()
		if strings.Contains(strings.ToLower(detail), "permission denied") {
			detail += " — fix: the systemd User= must own the .env file (stock unit: nobody). Example: " +
				"sudo chown nobody " + envPath + " && sudo chmod 600 " + envPath +
				" (use your service user if overridden), then retry migration."
		}
		a.apiWriteJSON(w, http.StatusBadRequest, map[string]string{
			"error":  "migrate_failed",
			"detail": detail,
		})
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
