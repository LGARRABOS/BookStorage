package server

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type adminBackupFile struct {
	Name    string
	Size    int64
	ModTime string
}

func adminBackupDir() string {
	if v := strings.TrimSpace(os.Getenv("BOOKSTORAGE_BACKUP_DIR")); v != "" {
		return v
	}
	return "/var/lib/bookstorage/backups"
}

func (a *App) listBackupFiles() ([]adminBackupFile, error) {
	dir := adminBackupDir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []adminBackupFile
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, "bookstorage-") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		out = append(out, adminBackupFile{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

func (a *App) HandleAdminBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	files, err := a.listBackupFiles()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.renderTemplate(w, r, "admin_backups", a.mergeData(r, map[string]any{
		"BackupDir":   adminBackupDir(),
		"BackupFiles": files,
	}))
}

func (a *App) HandleAPIAdminInstanceStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	dbBackend := "sqlite"
	if a.Settings != nil && a.Settings.UsePostgres() {
		dbBackend = "postgres"
	}
	var usersCount, worksCount, sessionsCount int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&usersCount)
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works`).Scan(&worksCount)
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL`).Scan(&sessionsCount)
	uptimeSec := 0
	if !a.ProcessStartedAt.IsZero() {
		uptimeSec = int(time.Since(a.ProcessStartedAt).Seconds())
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"version":        a.Version,
		"uptime_sec":     uptimeSec,
		"db_backend":     dbBackend,
		"users_count":    usersCount,
		"works_count":    worksCount,
		"sessions_count": sessionsCount,
		"backup_dir":     adminBackupDir(),
		"backup_files":   lenMustBackupFiles(a),
	})
}

func lenMustBackupFiles(a *App) int {
	files, err := a.listBackupFiles()
	if err != nil {
		return 0
	}
	return len(files)
}
