package server

import (
	"database/sql"
	"net/url"
	"strings"
)

type animeWorkRow struct {
	ID            int
	Title         string
	Episode       int
	TotalEpisodes sql.NullInt64
	Status        sql.NullString
	AnimeType     sql.NullString
	Link          sql.NullString
	ImagePath     sql.NullString
	Rating        int
	Notes         sql.NullString
	IsAdult       sql.NullInt64
	Source        sql.NullString
	ExternalID    sql.NullString
	UserID        int
	UpdatedAt     nullFlexTime
	StartedAt     nullFlexTime
	FinishedAt    nullFlexTime
}

// sqlAnimeRowFull must match scanAnimeRow field order.
const sqlAnimeRowFull = `id, title, COALESCE(episode, 0), total_episodes, status, anime_type, link, image_path, COALESCE(rating, 0), notes, COALESCE(is_adult, 0), source, external_id, user_id, updated_at, started_at, finished_at`

func scanAnimeRow(w *animeWorkRow, s interface{ Scan(dest ...any) error }) error {
	return s.Scan(
		&w.ID, &w.Title, &w.Episode, &w.TotalEpisodes, &w.Status, &w.AnimeType, &w.Link,
		&w.ImagePath, &w.Rating, &w.Notes, &w.IsAdult, &w.Source, &w.ExternalID, &w.UserID,
		&w.UpdatedAt, &w.StartedAt, &w.FinishedAt,
	)
}

// animeSourcePageURL builds a public AniList anime page URL for a row, or "".
func animeSourcePageURL(source, externalID string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if ext == "" {
		return ""
	}
	if source == "anilist" {
		return "https://anilist.co/anime/" + url.PathEscape(ext)
	}
	return ""
}

// findAnimeInLibrary returns the existing anime_works id when the user already owns
// this title (case-insensitive) or the same source+external_id pair.
func (a *App) findAnimeInLibrary(userID int, title, source, externalID string) (int, bool) {
	if a == nil || a.DB == nil || userID <= 0 {
		return 0, false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	externalID = strings.TrimSpace(externalID)
	if source != "" && externalID != "" {
		var id int
		err := a.DB.QueryRow(
			`SELECT id FROM anime_works
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
		`SELECT id FROM anime_works
         WHERE user_id = ? AND LOWER(TRIM(title)) = LOWER(TRIM(?))
         LIMIT 1`,
		userID, title,
	).Scan(&id)
	if err == nil && id > 0 {
		return id, true
	}
	return 0, false
}

// animeLibraryExternalKeys maps "source:external_id" -> work id for the user.
func (a *App) animeLibraryExternalKeys(userID int) map[string]int {
	out := map[string]int{}
	if a == nil || a.DB == nil || userID <= 0 {
		return out
	}
	rows, err := a.DB.Query(
		`SELECT id, LOWER(COALESCE(source, '')), COALESCE(external_id, '')
         FROM anime_works
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

func animeIsInProgress(w animeWorkRow) bool {
	if !w.Status.Valid {
		return false
	}
	switch strings.TrimSpace(w.Status.String) {
	case "En cours", "Watching":
		return true
	default:
		return false
	}
}
