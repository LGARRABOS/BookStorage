package server

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a *App) HandleReadingSites(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	if r.Method == http.MethodPost {
		a.handleReadingSiteCreate(w, r, userID)
		return
	}

	sites := a.loadUserReadingSites(userID)
	data := map[string]any{
		"Sites":           sites,
		"HighlightIssues": strings.TrimSpace(r.URL.Query().Get("issues")) == "1",
	}
	if msg := r.URL.Query().Get("msg"); msg != "" {
		data["Flash"] = msg
	}
	if msg := r.URL.Query().Get("err"); msg != "" {
		data["FlashError"] = msg
	}
	a.renderTemplate(w, r, "reading_sites", a.mergeData(r, data))
}

func (a *App) handleReadingSiteCreate(w http.ResponseWriter, r *http.Request, userID int) {
	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))

	if name == "" || baseURL == "" {
		http.Redirect(w, r, "/reading-sites?err=name+and+url+required", http.StatusFound)
		return
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		http.Redirect(w, r, "/reading-sites?err=invalid+URL", http.StatusFound)
		return
	}

	_, err = a.DB.Exec(
		`INSERT INTO reading_sites (user_id, name, base_url, probe_status) VALUES (?, ?, ?, 'unknown')`,
		userID, name, baseURL,
	)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=save+failed", http.StatusFound)
		return
	}
	// Link existing works that match this new site.
	a.BackfillReadingSiteIDs()
	http.Redirect(w, r, "/reading-sites?msg=site+added", http.StatusFound)
}

func (a *App) HandleReadingSiteEdit(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=invalid+id", http.StatusFound)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))

	if name == "" || baseURL == "" {
		http.Redirect(w, r, "/reading-sites?err=name+and+url+required", http.StatusFound)
		return
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		http.Redirect(w, r, "/reading-sites?err=invalid+URL", http.StatusFound)
		return
	}

	_, err = a.DB.Exec(
		`UPDATE reading_sites SET name = ?, base_url = ?, probe_status = 'unknown', last_probe_at = NULL WHERE id = ? AND user_id = ?`,
		name, baseURL, id, userID,
	)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=update+failed", http.StatusFound)
		return
	}
	// Re-link works that might now match the updated URL.
	a.BackfillReadingSiteIDs()
	http.Redirect(w, r, "/reading-sites?msg=site+updated", http.StatusFound)
}

func (a *App) HandleReadingSiteDelete(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=invalid+id", http.StatusFound)
		return
	}
	// Unlink works referencing this site
	_, _ = a.DB.Exec(`UPDATE works SET reading_site_id = NULL WHERE reading_site_id = ? AND user_id = ?`, id, userID)
	_, _ = a.DB.Exec(`DELETE FROM reading_sites WHERE id = ? AND user_id = ?`, id, userID)
	http.Redirect(w, r, "/reading-sites?msg=site+deleted", http.StatusFound)
}

func (a *App) HandleReadingSiteProbe(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=invalid+id", http.StatusFound)
		return
	}

	var site readingSite
	err = a.DB.QueryRow(
		`SELECT id, user_id, name, base_url, last_probe_at, COALESCE(probe_status, 'unknown'), probe_http_status, probe_detail FROM reading_sites WHERE id = ? AND user_id = ?`,
		id, userID,
	).Scan(&site.ID, &site.UserID, &site.Name, &site.BaseURL, &site.LastProbeAt, &site.ProbeStatus, &site.ProbeHTTPStatus, &site.ProbeDetail)
	if err != nil {
		http.Redirect(w, r, "/reading-sites?err=not+found", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	a.ProbeAndUpdateSite(ctx, site)
	http.Redirect(w, r, "/reading-sites?msg=probe+done", http.StatusFound)
}

func (a *App) HandleReadingSiteProbeAll(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	a.ProbeAllUserSites(ctx, userID, 0)
	http.Redirect(w, r, "/reading-sites?msg=all+probed", http.StatusFound)
}

// HandleAPIReadingSiteMatch returns JSON with the matched site for a given link URL.
func (a *App) HandleAPIReadingSiteMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)
	link := strings.TrimSpace(r.URL.Query().Get("q"))
	if link == "" {
		a.apiWriteJSON(w, http.StatusOK, map[string]any{"matched": false})
		return
	}
	siteID, ok := a.MatchReadingSite(userID, link)
	if !ok {
		a.apiWriteJSON(w, http.StatusOK, map[string]any{"matched": false})
		return
	}
	var name, baseURL string
	err := a.DB.QueryRow(`SELECT name, base_url FROM reading_sites WHERE id = ?`, siteID).Scan(&name, &baseURL)
	if err != nil {
		a.apiWriteJSON(w, http.StatusOK, map[string]any{"matched": false})
		return
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"matched":  true,
		"site_id":  siteID,
		"name":     name,
		"base_url": baseURL,
	})
}
