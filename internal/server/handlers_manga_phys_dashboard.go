package server

import (
	"log"
	"net/http"
	"strings"
)

func normalizeMangaPhysStatusForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range mangaPhysStatuses {
		if v == s {
			return s
		}
	}
	return "À lire"
}

func normalizeMangaPhysTypeForWrite(raw string) string {
	s := strings.TrimSpace(raw)
	for _, v := range mangaPhysTypes {
		if v == s {
			return s
		}
	}
	return "Manga"
}

func (a *App) HandleMangaPhysDashboard(w http.ResponseWriter, r *http.Request) {
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
		orderClause = "ORDER BY manga_type, LOWER(title), id"
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

	query := `SELECT ` + sqlMangaPhysRowFull + `
        FROM manga_phys_works ` + whereClause + " " + orderClause

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		log.Printf("[manga-phys-dashboard] query user=%d: %v", userID, err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []mangaPhysWorkRow
	totalTomes := 0
	inProgress := 0
	for rows.Next() {
		var wRow mangaPhysWorkRow
		if err := scanMangaPhysRow(&wRow, rows); err != nil {
			log.Printf("[manga-phys-dashboard] scan user=%d: %v", userID, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
		totalTomes += wRow.Tome
		if mangaPhysIsInProgress(wRow) {
			inProgress++
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[manga-phys-dashboard] rows user=%d: %v", userID, err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	seriesCards := groupMangaPhysWorksBySeries(works)
	sortMangaPhysSeriesCards(seriesCards, sortBy)

	activeSeries := strings.TrimSpace(r.URL.Query().Get("series"))
	var seriesList []mangaPhysSeriesCard
	var albumList []mangaPhysWorkRow
	if activeSeries != "" {
		if card, ok := findMangaPhysSeriesCard(seriesCards, activeSeries); ok {
			activeSeries = card.Name
			albumList = card.Albums
		} else {
			activeSeries = ""
			seriesList = seriesCards
		}
	} else {
		seriesList = seriesCards
	}

	data := map[string]any{
		"Works":             albumList,
		"Series":            seriesList,
		"ActiveSeries":      activeSeries,
		"TotalTomes":        totalTomes,
		"InProgressCount":   inProgress,
		"MangaTypes":        mangaPhysTypes,
		"MangaPhysStatuses": mangaPhysStatuses,
		"IsAdmin":           isAdmin == 1,
		"SortBy":            sortBy,
		"AdultFilter":       adultFilter,
		"SearchQuery":       r.URL.Query().Get("q"),
	}
	a.renderTemplate(w, r, "manga_phys_dashboard", a.mergeData(r, data))
}
