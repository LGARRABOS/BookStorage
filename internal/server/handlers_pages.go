package server

import (
	"bookstorage/internal/i18n"
	"net/http"
	"path"
)

func (a *App) HandleHome(w http.ResponseWriter, r *http.Request) {
	// The "/" pattern in http.ServeMux matches any path not matched by a more specific route.
	// Only serve the home / landing for the exact root path; otherwise return 404.
	if p := path.Clean(r.URL.Path); p != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, ok := a.currentUserID(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	// Landing page for non-logged in visitors
	a.renderTemplate(w, r, "landing", a.baseData(r))
}

func (a *App) HandleLegal(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data["Legal"] = a.SiteConfig.Legal
	data["SiteName"] = a.SiteConfig.SiteName
	data["SiteURL"] = a.SiteConfig.SiteURL
	a.renderTemplate(w, r, "legal", data)
}

func (a *App) HandleStats(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	var totalWorks, totalChapters, ratedCount int
	var avgRating float64
	if err := a.DB.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM works WHERE user_id = ?),
			(SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?),
			(SELECT COALESCE(AVG(rating), 0) FROM works WHERE user_id = ? AND rating > 0),
			(SELECT COUNT(*) FROM works WHERE user_id = ? AND rating > 0)
	`, userID, userID, userID, userID).Scan(&totalWorks, &totalChapters, &avgRating, &ratedCount); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// By status
	type statusCount struct {
		Status string
		Count  int
	}
	var byStatus []statusCount
	rows, err := a.DB.Query(`SELECT COALESCE(status, 'Non défini'), COUNT(*) FROM works WHERE user_id = ? GROUP BY status ORDER BY COUNT(*) DESC`, userID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sc statusCount
			if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
				continue
			}
			byStatus = append(byStatus, sc)
		}
	}

	// By type
	type typeCount struct {
		Type  string
		Count int
	}
	var byType []typeCount
	rows2, err := a.DB.Query(`SELECT COALESCE(reading_type, 'Autre'), COUNT(*) FROM works WHERE user_id = ? GROUP BY reading_type ORDER BY COUNT(*) DESC`, userID)
	if err == nil {
		defer func() { _ = rows2.Close() }()
		for rows2.Next() {
			var tc typeCount
			if err := rows2.Scan(&tc.Type, &tc.Count); err != nil {
				continue
			}
			byType = append(byType, tc)
		}
	}

	// Top 5 meilleures notes
	type ratedWork struct {
		Title  string
		Rating int
	}
	var topRated []ratedWork
	rows3, err := a.DB.Query(`SELECT title, rating FROM works WHERE user_id = ? AND rating > 0 ORDER BY rating DESC, title LIMIT 5`, userID)
	if err == nil {
		defer func() { _ = rows3.Close() }()
		for rows3.Next() {
			var rw ratedWork
			if err := rows3.Scan(&rw.Title, &rw.Rating); err != nil {
				continue
			}
			topRated = append(topRated, rw)
		}
	}

	lang := a.currentLang(r)
	tr := i18n.T(lang)

	a.renderTemplate(w, r, "stats", a.mergeData(r, map[string]any{
		"StatsUserID":     userID,
		"TotalWorks":      totalWorks,
		"TotalChapters":   totalChapters,
		"ByStatus":        byStatus,
		"ByType":          byType,
		"AvgRating":       avgRating,
		"RatedCount":      ratedCount,
		"TopRated":        topRated,
		"ReadingTimeline": a.readingTimelineForCharts(userID),
		"StatusDistrib":   a.statusDistribForCharts(userID, tr),
	}))
}
