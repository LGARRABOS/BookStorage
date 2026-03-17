package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type apiWork struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Chapter     int    `json:"chapter"`
	Link        string `json:"link,omitempty"`
	Status      string `json:"status,omitempty"`
	ReadingType string `json:"reading_type,omitempty"`
	Rating      int    `json:"rating"`
	Notes       string `json:"notes,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
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

	rows, err := a.DB.Query(
		`SELECT id, title, chapter, link, status, reading_type, COALESCE(rating, 0), notes, COALESCE(updated_at, '')
         FROM works WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer func() { _ = rows.Close() }()

	var works []apiWork
	for rows.Next() {
		var w apiWork
		var link, status, readingType, notes sql.NullString
		if err := rows.Scan(&w.ID, &w.Title, &w.Chapter, &link, &status, &readingType, &w.Rating, &notes, &w.UpdatedAt); err != nil {
			continue
		}
		if link.Valid {
			w.Link = link.String
		}
		if status.Valid {
			w.Status = status.String
		}
		if readingType.Valid {
			w.ReadingType = readingType.String
		}
		if notes.Valid {
			w.Notes = notes.String
		}
		works = append(works, w)
	}

	a.apiWriteJSON(w, http.StatusOK, map[string]any{"data": works})
}

func (a *App) HandleAPIWorksDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var work apiWork
	var link, status, readingType, notes sql.NullString
	err := a.DB.QueryRow(
		`SELECT id, title, chapter, link, status, reading_type, COALESCE(rating, 0), notes, COALESCE(updated_at, '')
         FROM works WHERE id = ? AND user_id = ?`,
		workID, userID,
	).Scan(&work.ID, &work.Title, &work.Chapter, &link, &status, &readingType, &work.Rating, &notes, &work.UpdatedAt)
	if err == sql.ErrNoRows {
		a.apiWriteError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if link.Valid {
		work.Link = link.String
	}
	if status.Valid {
		work.Status = status.String
	}
	if readingType.Valid {
		work.ReadingType = readingType.String
	}
	if notes.Valid {
		work.Notes = notes.String
	}

	a.apiWriteJSON(w, http.StatusOK, map[string]any{"data": work})
}

func (a *App) HandleAPIWorksCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	var req struct {
		Title       string `json:"title"`
		Chapter     int    `json:"chapter"`
		Link        string `json:"link"`
		Status      string `json:"status"`
		ReadingType string `json:"reading_type"`
		Rating      int    `json:"rating"`
		Notes       string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		a.apiWriteError(w, http.StatusBadRequest, "title_required")
		return
	}
	if req.Chapter < 0 {
		req.Chapter = 0
	}
	if req.Chapter > 9999 {
		req.Chapter = 9999
	}
	if req.Rating < 0 || req.Rating > 5 {
		req.Rating = 0
	}
	readingType := strings.TrimSpace(req.ReadingType)
	if readingType == "" {
		readingType = readingTypes[0]
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "En cours"
	}

	res, err := a.DB.Exec(
		`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		strings.TrimSpace(req.Title), req.Chapter, nullIfEmpty(req.Link), status, readingType, req.Rating, nullIfEmpty(req.Notes), userID,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	id, _ := res.LastInsertId()

	work := apiWork{
		ID:          int(id),
		Title:       strings.TrimSpace(req.Title),
		Chapter:     req.Chapter,
		Link:        req.Link,
		Status:      status,
		ReadingType: readingType,
		Rating:      req.Rating,
		Notes:       req.Notes,
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
		ch := int(v)
		if ch < 0 {
			ch = 0
		}
		if ch > 9999 {
			ch = 9999
		}
		setParts = append(setParts, "chapter = ?")
		args = append(args, ch)
	}
	if v, ok := req["link"].(string); ok {
		setParts = append(setParts, "link = ?")
		args = append(args, nullIfEmpty(v))
	}
	if v, ok := req["status"].(string); ok && v != "" {
		setParts = append(setParts, "status = ?")
		args = append(args, v)
	}
	if v, ok := req["reading_type"].(string); ok && v != "" {
		setParts = append(setParts, "reading_type = ?")
		args = append(args, v)
	}
	if v, ok := req["rating"].(float64); ok {
		rating := int(v)
		if rating < 0 || rating > 5 {
			rating = 0
		}
		setParts = append(setParts, "rating = ?")
		args = append(args, rating)
	}
	if v, ok := req["notes"].(string); ok {
		setParts = append(setParts, "notes = ?")
		args = append(args, nullIfEmpty(v))
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

	a.HandleAPIWorksDetail(w, r)
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
