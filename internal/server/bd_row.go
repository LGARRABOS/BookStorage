package server

import (
	"database/sql"
	"net/url"
	"strings"
)

type bdWorkRow struct {
	ID         int
	Title      string
	Tome       int
	TotalTomes sql.NullInt64
	Status     sql.NullString
	BdType     sql.NullString
	Link       sql.NullString
	ImagePath  sql.NullString
	Rating     int
	Notes      sql.NullString
	IsAdult    sql.NullInt64
	Source     sql.NullString
	ExternalID sql.NullString
	UserID     int
	UpdatedAt  nullFlexTime
	StartedAt  nullFlexTime
	FinishedAt nullFlexTime
}

const sqlBdRowFull = `id, title, COALESCE(tome, 0), total_tomes, status, bd_type, link, image_path, COALESCE(rating, 0), notes, COALESCE(is_adult, 0), source, external_id, user_id, updated_at, started_at, finished_at`

func scanBdRow(w *bdWorkRow, s interface{ Scan(dest ...any) error }) error {
	return s.Scan(
		&w.ID, &w.Title, &w.Tome, &w.TotalTomes, &w.Status, &w.BdType, &w.Link,
		&w.ImagePath, &w.Rating, &w.Notes, &w.IsAdult, &w.Source, &w.ExternalID, &w.UserID,
		&w.UpdatedAt, &w.StartedAt, &w.FinishedAt,
	)
}

func bdSourcePageURL(source, externalID string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if ext == "" {
		return ""
	}
	if source == "openlibrary" {
		if strings.HasPrefix(ext, "/") {
			return "https://openlibrary.org" + ext
		}
		return "https://openlibrary.org/works/" + url.PathEscape(ext)
	}
	return ""
}

func (a *App) findBdInLibrary(userID int, title, source, externalID string) (int, bool) {
	if a == nil || a.DB == nil || userID <= 0 {
		return 0, false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	externalID = strings.TrimSpace(externalID)
	if source != "" && externalID != "" {
		var id int
		err := a.DB.QueryRow(
			`SELECT id FROM bd_works
             WHERE user_id = ? AND LOWER(COALESCE(source, '')) = ? AND external_id = ?
             LIMIT 1`,
			userID, source, externalID,
		).Scan(&id)
		if err == nil && id > 0 {
			return id, true
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, false
	}
	var id int
	err := a.DB.QueryRow(
		`SELECT id FROM bd_works
         WHERE user_id = ? AND LOWER(TRIM(title)) = LOWER(TRIM(?))
         LIMIT 1`,
		userID, title,
	).Scan(&id)
	if err == nil && id > 0 {
		return id, true
	}
	return 0, false
}

func (a *App) bdLibraryExternalKeys(userID int) map[string]int {
	out := map[string]int{}
	if a == nil || a.DB == nil || userID <= 0 {
		return out
	}
	rows, err := a.DB.Query(
		`SELECT id, LOWER(COALESCE(source, '')), COALESCE(external_id, '')
         FROM bd_works
         WHERE user_id = ? AND COALESCE(external_id, '') != ''`,
		userID,
	)
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		var source, ext string
		if err := rows.Scan(&id, &source, &ext); err != nil {
			return out
		}
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		out[source+":"+ext] = id
	}
	return out
}

func bdIsInProgress(w bdWorkRow) bool {
	if !w.Status.Valid {
		return false
	}
	switch strings.TrimSpace(w.Status.String) {
	case "En cours", "Reading":
		return true
	default:
		return false
	}
}
