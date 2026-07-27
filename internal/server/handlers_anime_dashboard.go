package server

import (
	"net/http"
	"strings"
)

// normalizeAnimeStatusForWrite returns a valid anime status, defaulting to "À voir".
func normalizeAnimeStatusForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range animeStatuses {
		if v == s {
			return s
		}
	}
	return "À voir"
}

// normalizeAnimeTypeForWrite returns a valid anime type, defaulting to "TV".
func normalizeAnimeTypeForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range animeTypes {
		if v == s {
			return s
		}
	}
	return "TV"
}

func (a *App) HandleAnimeDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)

	var isAdmin int
	_ = a.DB.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin)

	sortBy := r.URL.Query().Get("sort")
	orderClause := "ORDER BY LOWER(title), id"
	switch sortBy {
	case "title_desc":
		orderClause = "ORDER BY LOWER(title) DESC, id DESC"
	case "status":
		orderClause = "ORDER BY status, LOWER(title), id"
	case "type":
		orderClause = "ORDER BY anime_type, LOWER(title), id"
	case "recent":
		orderClause = "ORDER BY id DESC"
	case "oldest":
		orderClause = "ORDER BY id ASC"
	case "modified", "modified_desc":
		sortBy = "modified_desc"
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') DESC, id DESC"
	case "modified_asc":
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') ASC, id ASC"
	default:
		sortBy = "title"
	}

	adultFilter := r.URL.Query().Get("adult")
	whereClause := "WHERE user_id = ?"
	args := []any{userID}
	switch adultFilter {
	case "only":
		whereClause += " AND COALESCE(is_adult, 0) = 1"
	default:
		adultFilter = ""
		whereClause += " AND COALESCE(is_adult, 0) = 0"
	}

	query := `SELECT ` + sqlAnimeRowFull + `
        FROM anime_works ` + whereClause + " " + orderClause

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []animeWorkRow
	totalEpisodes := 0
	inProgress := 0
	for rows.Next() {
		var wRow animeWorkRow
		if err := scanAnimeRow(&wRow, rows); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
		totalEpisodes += wRow.Episode
		if animeIsInProgress(wRow) {
			inProgress++
		}
	}
	if err := rows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Works":           works,
		"TotalEpisodes":   totalEpisodes,
		"InProgressCount": inProgress,
		"AnimeTypes":      animeTypes,
		"AnimeStatuses":   animeStatuses,
		"IsAdmin":         isAdmin == 1,
		"SortBy":          sortBy,
		"AdultFilter":     adultFilter,
		"SearchQuery":     r.URL.Query().Get("q"),
	}
	a.renderTemplate(w, r, "anime_dashboard", a.mergeData(r, data))
}
