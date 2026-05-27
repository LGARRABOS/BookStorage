package server

import (
	"net/http"
	"strings"
)

func normalizeBlocklistLabelType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "genre":
		return "genre", true
	case "tag":
		return "tag", true
	default:
		return "", false
	}
}

func (a *App) HandleProfileBlocklistAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	labelType, ok := normalizeBlocklistLabelType(r.FormValue("label_type"))
	if !ok {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	labelName := strings.TrimSpace(r.FormValue("label_name"))
	if labelName == "" {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	_, err := a.DB.Exec(
		`INSERT INTO user_catalog_blocklist (user_id, label_type, label_name) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, label_type, label_name) DO NOTHING`,
		userID, labelType, labelName,
	)
	if err != nil {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?blocklist_added=1#blocklist", http.StatusFound)
}

func (a *App) HandleProfileBlocklistRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	labelType, ok := normalizeBlocklistLabelType(r.FormValue("label_type"))
	if !ok {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	labelName := strings.TrimSpace(r.FormValue("label_name"))
	if labelName == "" {
		http.Redirect(w, r, "/profile?blocklist_error=1", http.StatusFound)
		return
	}
	_, _ = a.DB.Exec(
		`DELETE FROM user_catalog_blocklist WHERE user_id = ? AND label_type = ? AND label_name = ?`,
		userID, labelType, labelName,
	)
	http.Redirect(w, r, "/profile?blocklist_removed=1#blocklist", http.StatusFound)
}
