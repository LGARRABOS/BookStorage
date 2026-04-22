package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/database"
)

type apiWork struct {
	ID                int    `json:"id"`
	Title             string `json:"title"`
	Chapter           int    `json:"chapter"`
	Link              string `json:"link,omitempty"`
	Status            string `json:"status,omitempty"`
	ReadingType       string `json:"reading_type,omitempty"`
	Rating            int    `json:"rating"`
	Notes             string `json:"notes,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	ParentWorkID      *int   `json:"parent_work_id,omitempty"`
	SeriesSort        int    `json:"series_sort,omitempty"`
	NotifyNewChapters int    `json:"notify_new_chapters"`
}

func workRowToAPIWork(w workRow) apiWork {
	out := apiWork{
		ID:                w.ID,
		Title:             w.Title,
		Chapter:           w.Chapter,
		Rating:            w.Rating,
		SeriesSort:        w.SeriesSort,
		NotifyNewChapters: w.NotifyNewChapters,
	}
	if w.Link.Valid {
		out.Link = w.Link.String
	}
	if w.Status.Valid {
		out.Status = w.Status.String
	}
	if w.ReadingType.Valid {
		out.ReadingType = w.ReadingType.String
	}
	if w.Notes.Valid {
		out.Notes = w.Notes.String
	}
	if w.UpdatedAt.Valid {
		out.UpdatedAt = w.UpdatedAt.String
	}
	if w.ParentWorkID.Valid && w.ParentWorkID.Int64 > 0 {
		v := int(w.ParentWorkID.Int64)
		out.ParentWorkID = &v
	}
	return out
}

func (a *App) apiWriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *App) apiWriteError(w http.ResponseWriter, status int, errMsg string) {
	a.apiWriteJSON(w, status, map[string]string{"error": errMsg})
}

func (a *App) HandleAPIWorksList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	page := 1
	limit := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := (page - 1) * limit
	statusFilter := normalizeStatusForWrite(r.URL.Query().Get("status"))
	if strings.TrimSpace(r.URL.Query().Get("status")) == "" {
		statusFilter = ""
	}
	typeFilter := normalizeReadingTypeForWrite(r.URL.Query().Get("reading_type"))
	if strings.TrimSpace(r.URL.Query().Get("reading_type")) == "" {
		typeFilter = ""
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortBy == "" {
		sortBy = "recent"
	}
	orderBy := "id DESC"
	switch sortBy {
	case "title_asc":
		orderBy = "LOWER(title) ASC"
	case "title_desc":
		orderBy = "LOWER(title) DESC"
	case "chapter_asc":
		orderBy = "chapter ASC, id DESC"
	case "chapter_desc":
		orderBy = "chapter DESC, id DESC"
	case "updated_asc":
		orderBy = "COALESCE(updated_at, '1970-01-01') ASC"
	case "updated_desc":
		orderBy = "COALESCE(updated_at, '1970-01-01') DESC"
	case "recent":
		orderBy = "id DESC"
	default:
		sortBy = "recent"
	}

	whereParts := []string{"user_id = ?"}
	args := []any{userID}
	if statusFilter != "" {
		whereParts = append(whereParts, "status = ?")
		args = append(args, statusFilter)
	}
	if typeFilter != "" {
		whereParts = append(whereParts, "reading_type = ?")
		args = append(args, typeFilter)
	}
	if search != "" {
		usedFTS := false
		if database.WorksFTSEnabled(a.DB) {
			if a.DB.B == database.BackendPostgres {
				if s := strings.TrimSpace(search); s != "" {
					whereParts = append(whereParts, "(works.works_fts_document @@ plainto_tsquery('simple', ?))")
					args = append(args, s)
					usedFTS = true
				}
			} else if matchExpr, ok := fts5MatchExpression(search); ok {
				whereParts = append(whereParts, "works.id IN (SELECT rowid FROM works_fts WHERE works_fts MATCH ?)")
				args = append(args, matchExpr)
				usedFTS = true
			}
		}
		if !usedFTS {
			whereParts = append(whereParts, "(LOWER(title) LIKE ? OR LOWER(COALESCE(notes, '')) LIKE ? OR LOWER(COALESCE(link, '')) LIKE ?)")
			like := "%" + strings.ToLower(search) + "%"
			args = append(args, like, like, like)
		}
	}
	whereSQL := strings.Join(whereParts, " AND ")

	var total int
	countStmt := "SELECT COUNT(*) FROM works WHERE " + whereSQL
	if err := a.DB.QueryRow(countStmt, args...).Scan(&total); err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	stmt := `SELECT ` + sqlWorkRowFull + `
         FROM works WHERE ` + whereSQL + ` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
	rows, err := a.DB.Query(stmt, queryArgs...)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer func() { _ = rows.Close() }()

	var works []apiWork
	for rows.Next() {
		var wr workRow
		if err := scanFullWorkRow(&wr, rows); err != nil {
			continue
		}
		works = append(works, workRowToAPIWork(wr))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"data": works,
		"meta": map[string]any{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
			"has_next":    page < totalPages,
			"has_prev":    page > 1,
			"sort":        sortBy,
			"search":      search,
		},
	})
}

func (a *App) HandleAPIWorksDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var wr workRow
	err := scanFullWorkRow(&wr, a.DB.QueryRow(
		`SELECT `+sqlWorkRowFull+`
         FROM works WHERE id = ? AND user_id = ?`,
		workID, userID,
	))
	if err == sql.ErrNoRows {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	a.apiWriteJSON(w, http.StatusOK, map[string]any{"data": workRowToAPIWork(wr)})
}

func (a *App) HandleAPIWorksCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	var req struct {
		Title             string `json:"title"`
		Chapter           int    `json:"chapter"`
		Link              string `json:"link"`
		Status            string `json:"status"`
		ReadingType       string `json:"reading_type"`
		Rating            int    `json:"rating"`
		Notes             string `json:"notes"`
		ParentWorkID      *int   `json:"parent_work_id"`
		SeriesSort        int    `json:"series_sort"`
		NotifyNewChapters *int   `json:"notify_new_chapters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		a.apiWriteError(w, http.StatusBadRequest, "title_required")
		return
	}
	req.Title = sanitizeTitle(req.Title)
	req.Chapter = clampChapter(req.Chapter)
	req.Rating = clampRating(req.Rating)
	readingType := normalizeReadingTypeForWrite(req.ReadingType)
	status := normalizeStatusForWrite(req.Status)
	wantNotify := true
	if req.NotifyNewChapters != nil {
		wantNotify = *req.NotifyNewChapters != 0
	}
	notifyCh := notifyNewChaptersDB(status, wantNotify)

	if req.ParentWorkID != nil && *req.ParentWorkID > 0 {
		if err := a.validateWorkParent(userID, 0, *req.ParentWorkID); err != nil {
			a.apiWriteError(w, http.StatusBadRequest, "invalid_parent")
			return
		}
	}
	var parentArg any
	if req.ParentWorkID != nil && *req.ParentWorkID > 0 {
		parentArg = *req.ParentWorkID
	} else {
		parentArg = nil
	}

	res, err := a.DB.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, parent_work_id, series_sort, notify_new_chapters, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		req.Title, req.Chapter, nullIfEmpty(strings.TrimSpace(req.Link)), status, readingType, req.Rating, nullIfEmpty(strings.TrimSpace(req.Notes)), userID, parentArg, req.SeriesSort, notifyCh,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	id, _ := res.LastInsertId()

	work := apiWork{
		ID:                int(id),
		Title:             req.Title,
		Chapter:           req.Chapter,
		Link:              strings.TrimSpace(req.Link),
		Status:            status,
		ReadingType:       readingType,
		Rating:            req.Rating,
		Notes:             strings.TrimSpace(req.Notes),
		SeriesSort:        req.SeriesSort,
		ParentWorkID:      req.ParentWorkID,
		NotifyNewChapters: notifyCh,
	}

	a.apiWriteJSON(w, http.StatusCreated, map[string]any{"data": work})
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (a *App) HandleAPIWorksUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	var setParts []string
	var args []any
	if v, ok := req["title"].(string); ok && strings.TrimSpace(v) != "" {
		setParts = append(setParts, "title = ?")
		args = append(args, strings.TrimSpace(v))
	}
	if v, ok := req["chapter"].(float64); ok {
		ch := clampChapter(int(v))
		setParts = append(setParts, "chapter = ?")
		args = append(args, ch)
	}
	if v, ok := req["link"].(string); ok {
		setParts = append(setParts, "link = ?")
		args = append(args, nullIfEmpty(strings.TrimSpace(v)))
	}
	if v, ok := req["status"].(string); ok && v != "" {
		setParts = append(setParts, "status = ?")
		args = append(args, normalizeStatusForWrite(v))
	}
	if v, ok := req["reading_type"].(string); ok && v != "" {
		setParts = append(setParts, "reading_type = ?")
		args = append(args, normalizeReadingTypeForWrite(v))
	}
	if v, ok := req["rating"].(float64); ok {
		rating := clampRating(int(v))
		setParts = append(setParts, "rating = ?")
		args = append(args, rating)
	}
	if v, ok := req["notes"].(string); ok {
		setParts = append(setParts, "notes = ?")
		args = append(args, nullIfEmpty(strings.TrimSpace(v)))
	}
	if raw, ok := req["parent_work_id"]; ok {
		if raw == nil {
			setParts = append(setParts, "parent_work_id = NULL")
		} else if v, ok := raw.(float64); ok {
			pid := int(v)
			if pid <= 0 {
				setParts = append(setParts, "parent_work_id = NULL")
			} else {
				if err := a.validateWorkParent(userID, workID, pid); err != nil {
					a.apiWriteError(w, http.StatusBadRequest, "invalid_parent")
					return
				}
				setParts = append(setParts, "parent_work_id = ?")
				args = append(args, pid)
			}
		}
	}
	if v, ok := req["series_sort"].(float64); ok {
		setParts = append(setParts, "series_sort = ?")
		args = append(args, int(v))
	}
	if raw, ok := req["notify_new_chapters"]; ok {
		var st string
		_ = a.DB.QueryRow(`SELECT COALESCE(status, '') FROM works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&st)
		effStatus := normalizeStatusForWrite(st)
		if v, ok := req["status"].(string); ok && strings.TrimSpace(v) != "" {
			effStatus = normalizeStatusForWrite(v)
		}
		switch v := raw.(type) {
		case bool:
			setParts = append(setParts, "notify_new_chapters = ?")
			args = append(args, notifyNewChaptersDB(effStatus, v))
		case float64:
			setParts = append(setParts, "notify_new_chapters = ?")
			args = append(args, notifyNewChaptersDB(effStatus, v != 0))
		}
	}

	if len(setParts) == 0 {
		a.apiWriteError(w, http.StatusBadRequest, "no_fields_to_update")
		return
	}

	setParts = append(setParts, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, workID, userID)
	stmt := "UPDATE works SET " + strings.Join(setParts, ", ") + " WHERE id = ? AND user_id = ?"
	result, err := a.DB.Exec(stmt, args...)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}

	// Reuse detail payload while forcing a GET method.
	detailReq := r.Clone(r.Context())
	detailReq.Method = http.MethodGet
	a.HandleAPIWorksDetail(w, detailReq)
}

func (a *App) HandleAPIWorksDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	result, err := a.DB.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}

	a.apiWriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) HandleAPIStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	var totalWorks, totalChapters int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ?`, userID).Scan(&totalWorks)
	_ = a.DB.QueryRow(`SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?`, userID).Scan(&totalChapters)

	var avgRating float64
	var ratedCount int
	_ = a.DB.QueryRow(`SELECT COALESCE(AVG(rating), 0), COUNT(*) FROM works WHERE user_id = ? AND rating > 0`, userID).Scan(&avgRating, &ratedCount)

	a.apiWriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"total_works":    totalWorks,
			"total_chapters": totalChapters,
			"avg_rating":     avgRating,
			"rated_count":    ratedCount,
		},
	})
}
