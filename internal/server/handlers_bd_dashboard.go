package server

import (
	"net/http"
	"strings"
)

func normalizeBdStatusForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range bdStatuses {
		if v == s {
			return s
		}
	}
	return "À lire"
}

func normalizeBdTypeForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range bdTypes {
		if v == s {
			return s
		}
	}
	return "Album"
}

func (a *App) HandleBdDashboard(w http.ResponseWriter, r *http.Request) {
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
		orderClause = "ORDER BY bd_type, LOWER(title), id"
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

	query := `SELECT ` + sqlBdRowFull + `
        FROM bd_works ` + whereClause + " " + orderClause

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []bdWorkRow
	totalTomes := 0
	inProgress := 0
	for rows.Next() {
		var wRow bdWorkRow
		if err := scanBdRow(&wRow, rows); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
		totalTomes += wRow.Tome
		if bdIsInProgress(wRow) {
			inProgress++
		}
	}
	if err := rows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Works":           works,
		"TotalTomes":      totalTomes,
		"InProgressCount": inProgress,
		"BdTypes":         bdTypes,
		"BdStatuses":      bdStatuses,
		"IsAdmin":         isAdmin == 1,
		"SortBy":          sortBy,
		"AdultFilter":     adultFilter,
		"SearchQuery":     r.URL.Query().Get("q"),
	}
	a.renderTemplate(w, r, "bd_dashboard", a.mergeData(r, data))
}
