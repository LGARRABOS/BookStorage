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
