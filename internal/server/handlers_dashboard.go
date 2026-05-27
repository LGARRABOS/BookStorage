package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func (a *App) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	// Check if user is admin
	var isAdmin int
	_ = a.DB.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin)

	// Tri dashboard : uniquement le critère utilisateur en tête.
	// (Un préfixe « série » COALESCE(parent_work_id, id) cassait tous les tris pour les œuvres sans parent :
	// une clé différente par ligne forçait l’ordre d’insertion avant le titre / dates.)
	sortBy := r.URL.Query().Get("sort")
	orderClause := "ORDER BY LOWER(title), id"
	switch sortBy {
	case "title_desc":
		orderClause = "ORDER BY LOWER(title) DESC, id DESC"
	case "status":
		orderClause = "ORDER BY status, LOWER(title), id"
	case "type":
		orderClause = "ORDER BY reading_type, LOWER(title), id"
	case "recent":
		orderClause = "ORDER BY id DESC"
	case "oldest":
		orderClause = "ORDER BY id ASC"
	case "modified", "modified_desc":
		// Alias "modified" kept for backward compatibility
		sortBy = "modified_desc"
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') DESC, id DESC"
	case "modified_asc":
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') ASC, id ASC"
	default:
		sortBy = "title"
	}

	// Filtre adulte
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

	query := `SELECT ` + sqlWorkRowFull + `
        FROM works ` + whereClause + " " + orderClause

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []workRow
	for rows.Next() {
		var wRow workRow
		if err := scanFullWorkRow(&wRow, rows); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	catalogCoverByWorkID := map[int]string{}
	coverQuery := `SELECT w.id, COALESCE(c.image_url, '') FROM works w INNER JOIN catalog c ON c.id = w.catalog_id ` + whereClause
	coverRows, err := a.DB.Query(coverQuery, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = coverRows.Close() }()
	for coverRows.Next() {
		var wid int
		var imageURL string
		if err := coverRows.Scan(&wid, &imageURL); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(imageURL) != "" {
			catalogCoverByWorkID[wid] = strings.TrimSpace(imageURL)
		}
	}
	if err := coverRows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	anilistCoverByWorkID := map[int]string{}
	anilistRows, err := a.DB.Query(
		`SELECT w.id, COALESCE(c.image_url, '') FROM works w INNER JOIN catalog c ON c.id = w.catalog_id AND LOWER(TRIM(c.source)) = 'anilist' `+whereClause,
		args...,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = anilistRows.Close() }()
	for anilistRows.Next() {
		var wid int
		var imageURL string
		if err := anilistRows.Scan(&wid, &imageURL); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(imageURL) != "" {
			anilistCoverByWorkID[wid] = strings.TrimSpace(imageURL)
		}
	}
	if err := anilistRows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	readingSiteStatusMap := a.loadReadingSiteStatusMap(userID)

	var sitesDownCount, linkDeadCount int
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM reading_sites WHERE user_id = ? AND probe_status IN ('down', 'degraded')`,
		userID,
	).Scan(&sitesDownCount)
	_ = a.DB.QueryRow(
		`SELECT COUNT(*) FROM works WHERE user_id = ? AND link IS NOT NULL AND TRIM(link) != '' AND link_probe_status IN ('down', 'degraded')`,
		userID,
	).Scan(&linkDeadCount)

	data := map[string]any{
		"Works":                works,
		"CatalogCoverByWorkID": catalogCoverByWorkID,
		"AnilistCoverByWorkID": anilistCoverByWorkID,
		"ReadingTypes":         readingTypes,
		"ReadingStatus":        readingStatuses,
		"IsAdmin":              isAdmin == 1,
		"SortBy":               sortBy,
		"AdultFilter":          adultFilter,
		"SearchQuery":          r.URL.Query().Get("q"),
		"ReadingSiteMap":       readingSiteStatusMap,
		"ReadingSites":         a.loadUserReadingSites(userID),
		"SitesDownCount":       sitesDownCount,
		"LinkDeadCount":        linkDeadCount,
	}
	if enc := r.URL.Query().Get("import_report"); enc != "" {
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err == nil {
			var rep ImportReport
			if json.Unmarshal(raw, &rep) == nil {
				data["ImportReport"] = rep
			}
		}
	}
	if r.URL.Query().Get("error") == "import" {
		data["ImportError"] = true
	}
	a.renderTemplate(w, r, "dashboard", a.mergeData(r, data))
}
