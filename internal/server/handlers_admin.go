package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

func (a *App) HandleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(
		`SELECT id, username, password, validated, is_admin, is_superadmin,
                display_name, email, bio, avatar_path, is_public
         FROM users`,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	type adminUser struct {
		ID           int
		Username     string
		Validated    int
		IsAdmin      int
		IsSuperadmin int
		DisplayName  sql.NullString
		Email        sql.NullString
		Bio          sql.NullString
		AvatarPath   sql.NullString
		IsPublic     sql.NullInt64
	}

	var users []adminUser
	for rows.Next() {
		var u adminUser
		var pwd string
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&pwd,
			&u.Validated,
			&u.IsAdmin,
			&u.IsSuperadmin,
			&u.DisplayName,
			&u.Email,
			&u.Bio,
			&u.AvatarPath,
			&u.IsPublic,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	a.renderTemplate(w, r, "admin_accounts", a.mergeData(r, map[string]any{
		"Users": users,
	}))
}

func (a *App) HandleApproveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET validated = 1 WHERE id = ?`,
		userID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))

	var isAdmin, isSuper int
	err := a.DB.QueryRow(
		`SELECT is_admin, is_superadmin FROM users WHERE id = ?`,
		targetID,
	).Scan(&isAdmin, &isSuper)
	if err != nil {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	if isSuper != 0 {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	if _, err := a.DB.Exec(`DELETE FROM users WHERE id = ?`, targetID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandlePromoteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET is_admin = 1, validated = 1 WHERE id = ?`,
		targetID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) HandleAdminMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.renderTemplate(w, r, "admin_monitoring", a.mergeData(r, map[string]any{}))
}

func (a *App) HandleAPIMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	snap := MonitoringSnapshot{}
	if a.Monitor != nil {
		snap = a.Monitor.Snapshot()
	}
	_ = json.NewEncoder(w).Encode(snap)
}

func (a *App) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	latestTag, latestInfo, latestOK := a.computeUpdateTag(updateModeLatest)
	majorTag, majorInfo, majorOK := a.computeUpdateTag(updateModeLatestMajor)

	a.renderTemplate(w, r, "admin_update", a.mergeData(r, map[string]any{
		"LatestTag":      latestTag,
		"LatestTagOK":    latestOK,
		"LatestInfo":     latestInfo,
		"LatestMajorTag": majorTag,
		"LatestMajorOK":  majorOK,
		"LatestMajorInfo": majorInfo,
	}))
}

func (a *App) HandleAPIUpdateLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res := a.triggerUpdate(r.Context(), updateModeLatest)
	w.Header().Set("Content-Type", "application/json")
	if !res.OK {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (a *App) HandleAPIUpdateLatestMajor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res := a.triggerUpdate(r.Context(), updateModeLatestMajor)
	w.Header().Set("Content-Type", "application/json")
	if !res.OK {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (a *App) HandleAPIUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.updateStatus())
}
