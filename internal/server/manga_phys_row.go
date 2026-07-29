package server

import (
	"database/sql"
	"strings"
)

type mangaPhysWorkRow struct {
	ID         int
	Title      string
	Tome       int
	TotalTomes sql.NullInt64
	Status     sql.NullString
	MangaType  sql.NullString
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

const sqlMangaPhysRowFull = `id, title, COALESCE(tome, 0), total_tomes, status, manga_type, link, image_path, COALESCE(rating, 0), notes, COALESCE(is_adult, 0), source, external_id, user_id, updated_at, started_at, finished_at`

func scanMangaPhysRow(w *mangaPhysWorkRow, s interface{ Scan(dest ...any) error }) error {
	return s.Scan(
		&w.ID, &w.Title, &w.Tome, &w.TotalTomes, &w.Status, &w.MangaType, &w.Link,
		&w.ImagePath, &w.Rating, &w.Notes, &w.IsAdult, &w.Source, &w.ExternalID, &w.UserID,
		&w.UpdatedAt, &w.StartedAt, &w.FinishedAt,
	)
}

func (a *App) findMangaPhysInLibrary(userID int, title, source, externalID string) (int, bool) {
	if a == nil || a.DB == nil || userID <= 0 {
		return 0, false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	externalID = strings.TrimSpace(externalID)
	if source != "" && externalID != "" {
		var id int
		err := a.DB.QueryRow(
			`SELECT id FROM manga_phys_works
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
		`SELECT id FROM manga_phys_works
         WHERE user_id = ? AND LOWER(TRIM(title)) = LOWER(TRIM(?))
         LIMIT 1`,
		userID, title,
	).Scan(&id)
	if err == nil && id > 0 {
		return id, true
	}
	return 0, false
}

func mangaPhysIsInProgress(w mangaPhysWorkRow) bool {
	if !w.Status.Valid {
		return false
	}
	return strings.TrimSpace(w.Status.String) == "En cours"
}
