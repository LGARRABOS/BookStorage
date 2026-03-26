package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type dupGroup struct {
	NormTitle  string
	ReadingTyp string
	Count      int
	Works      []workRow
}

func normalizeTitleForDedupe(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (a *App) HandleDuplicates(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login?expired=1", http.StatusFound)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows, err := a.DB.Query(
		`SELECT LOWER(TRIM(title)) AS norm_title,
		        COALESCE(reading_type, '') AS reading_type,
		        COUNT(*) AS cnt
		 FROM works
		 WHERE user_id = ?
		 GROUP BY norm_title, reading_type
		 HAVING cnt > 1
		 ORDER BY cnt DESC
		 LIMIT 50`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var groups []dupGroup
	for rows.Next() {
		var g dupGroup
		if err := rows.Scan(&g.NormTitle, &g.ReadingTyp, &g.Count); err != nil {
			continue
		}
		groups = append(groups, g)
	}

	for i := range groups {
		g := &groups[i]
		wr, err := a.DB.Query(
			`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, updated_at, COALESCE(is_adult, 0)
			 FROM works
			 WHERE user_id = ? AND LOWER(TRIM(title)) = ? AND COALESCE(reading_type, '') = ?
			 ORDER BY id ASC`,
			userID, g.NormTitle, g.ReadingTyp,
		)
		if err != nil {
			continue
		}
		for wr.Next() {
			var wRow workRow
			if err := wr.Scan(
				&wRow.ID, &wRow.Title, &wRow.Chapter, &wRow.Link, &wRow.Status, &wRow.ImagePath, &wRow.ReadingType,
				&wRow.Rating, &wRow.Notes, &wRow.UserID, &wRow.UpdatedAt, &wRow.IsAdult,
			); err == nil {
				g.Works = append(g.Works, wRow)
			}
		}
		_ = wr.Close()
	}

	a.renderTemplate(w, r, "duplicates", a.mergeData(r, map[string]any{
		"Groups":   groups,
		"MergedOK": r.URL.Query().Get("merged") == "1",
	}))
}

func (a *App) HandleMergeDuplicate(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login?expired=1", http.StatusFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	fromID, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("from_id")))
	intoID, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("into_id")))
	if fromID <= 0 || intoID <= 0 || fromID == intoID {
		http.Redirect(w, r, "/tools/duplicates", http.StatusFound)
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	load := func(id int) (*workRow, error) {
		var wRow workRow
		err := tx.QueryRow(
			`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, updated_at, COALESCE(is_adult, 0)
			 FROM works WHERE id = ? AND user_id = ?`,
			id, userID,
		).Scan(
			&wRow.ID, &wRow.Title, &wRow.Chapter, &wRow.Link, &wRow.Status, &wRow.ImagePath, &wRow.ReadingType,
			&wRow.Rating, &wRow.Notes, &wRow.UserID, &wRow.UpdatedAt, &wRow.IsAdult,
		)
		if err != nil {
			return nil, err
		}
		return &wRow, nil
	}

	from, err := load(fromID)
	if err != nil {
		http.Redirect(w, r, "/tools/duplicates", http.StatusFound)
		return
	}
	into, err := load(intoID)
	if err != nil {
		http.Redirect(w, r, "/tools/duplicates", http.StatusFound)
		return
	}

	// Guard: only merge likely duplicates (same normalized title + reading_type).
	if normalizeTitleForDedupe(from.Title) != normalizeTitleForDedupe(into.Title) {
		http.Redirect(w, r, "/tools/duplicates", http.StatusFound)
		return
	}
	ft := ""
	it := ""
	if from.ReadingType.Valid {
		ft = from.ReadingType.String
	}
	if into.ReadingType.Valid {
		it = into.ReadingType.String
	}
	if strings.TrimSpace(ft) != strings.TrimSpace(it) {
		http.Redirect(w, r, "/tools/duplicates", http.StatusFound)
		return
	}

	mergedChapter := into.Chapter
	if from.Chapter > mergedChapter {
		mergedChapter = from.Chapter
	}
	mergedRating := into.Rating
	if from.Rating > mergedRating {
		mergedRating = from.Rating
	}

	mergedLink := into.Link
	if (!mergedLink.Valid || mergedLink.String == "") && from.Link.Valid && from.Link.String != "" {
		mergedLink = from.Link
	}
	mergedStatus := into.Status
	if (!mergedStatus.Valid || mergedStatus.String == "") && from.Status.Valid && from.Status.String != "" {
		mergedStatus = from.Status
	}
	mergedImage := into.ImagePath
	if (!mergedImage.Valid || mergedImage.String == "") && from.ImagePath.Valid && from.ImagePath.String != "" {
		mergedImage = from.ImagePath
	}
	mergedNotes := into.Notes
	if !mergedNotes.Valid || strings.TrimSpace(mergedNotes.String) == "" {
		mergedNotes = from.Notes
	} else if from.Notes.Valid && strings.TrimSpace(from.Notes.String) != "" && from.Notes.String != mergedNotes.String {
		mergedNotes = sql.NullString{String: mergedNotes.String + "\n\n" + from.Notes.String, Valid: true}
	}

	now := time.Now().UTC()
	_, err = tx.Exec(
		`UPDATE works
		 SET chapter = ?, link = ?, status = ?, image_path = ?, rating = ?, notes = ?, updated_at = ?
		 WHERE id = ? AND user_id = ?`,
		mergedChapter,
		nullStringOrNil(mergedLink),
		nullStringOrNil(mergedStatus),
		nullStringOrNil(mergedImage),
		mergedRating,
		nullStringOrNil(mergedNotes),
		now,
		intoID,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, fromID, userID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/tools/duplicates?merged=1", http.StatusFound)
}

func nullStringOrNil(ns sql.NullString) any {
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return nil
	}
	return ns.String
}
