package server

import (
	"net/http"
	"strconv"
)

// HandleAdminJobs renders the background jobs overview (cover enrichment queues).
func (a *App) HandleAdminJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	animeStatus := a.animeCoverJobStatus()
	bdStatus := a.bdCoverJobStatus()
	a.renderTemplate(w, r, "admin_jobs", a.mergeData(r, map[string]any{
		"AdminTab":             "jobs",
		"CoverJob":             animeStatus,
		"CoverETAHuman":        formatDurationSeconds(animeStatus.ETASeconds),
		"CoverJobRunning":      animeStatus.Running,
		"CoverGlobalMissing":   animeStatus.GlobalMissing,
		"BdCoverJob":           bdStatus,
		"BdCoverETAHuman":      formatDurationSeconds(bdStatus.ETASeconds),
		"BdCoverJobRunning":    bdStatus.Running,
		"BdCoverGlobalMissing": bdStatus.GlobalMissing,
		"JobsStarted":          r.URL.Query().Get("started") == "1",
	}))
}

// HandleAPIAdminJobs returns live job status as JSON for the admin jobs page.
func (a *App) HandleAPIAdminJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	animeStatus := a.animeCoverJobStatus()
	bdStatus := a.bdCoverJobStatus()
	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"covers":       animeStatus,
		"eta_human":    formatDurationSeconds(animeStatus.ETASeconds),
		"bd_covers":    bdStatus,
		"bd_eta_human": formatDurationSeconds(bdStatus.ETASeconds),
	})
}

// HandleAdminJobsRunCovers enqueues anime cover enrichment for every user with missing covers.
func (a *App) HandleAdminJobsRunCovers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	for _, uid := range a.listUsersWithMissingAnimeCovers() {
		a.scheduleAnimeCoverEnrichment(uid)
	}
	http.Redirect(w, r, "/admin/jobs?started=1", http.StatusFound)
}

// HandleAdminJobsRunBdCovers enqueues BD cover enrichment for every user with missing covers.
func (a *App) HandleAdminJobsRunBdCovers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	for _, uid := range a.listUsersWithMissingBdCovers() {
		a.scheduleBdCoverEnrichment(uid)
	}
	http.Redirect(w, r, "/admin/jobs?started=1", http.StatusFound)
}

func formatDurationSeconds(sec *int) string {
	if sec == nil {
		return ""
	}
	s := *sec
	if s < 0 {
		return ""
	}
	if s < 60 {
		return "< 1 min"
	}
	m := s / 60
	if m < 60 {
		return strconv.Itoa(m) + " min"
	}
	h := m / 60
	rm := m % 60
	if rm == 0 {
		return strconv.Itoa(h) + " h"
	}
	return strconv.Itoa(h) + " h " + strconv.Itoa(rm) + " min"
}
