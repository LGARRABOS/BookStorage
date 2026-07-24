package server

import (
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
)

const animeCoverEnrichBatchLimit = 500

// resolveAnimeCoverURL looks up a cover image URL via AniList (MAL id, AniList id, or title search).
// Overridable in tests.
var resolveAnimeCoverURL = resolveAnimeCoverURLDefault

func resolveAnimeCoverURLDefault(source, externalID, title string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if id, err := strconv.Atoi(ext); err == nil && id > 0 {
		switch source {
		case "anilist":
			if res, err := catalog.GetAnilistAnimeByID(id); err == nil && res != nil {
				if u := strings.TrimSpace(res.ImageURL); u != "" {
					return u
				}
			}
		case "mal", "myanimelist":
			if res, err := catalog.GetAnilistAnimeByMALID(id); err == nil && res != nil {
				if u := strings.TrimSpace(res.ImageURL); u != "" {
					return u
				}
			}
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	results, err := catalog.SearchAnilistAnime(title, 1)
	if err != nil || len(results) == 0 {
		return ""
	}
	return strings.TrimSpace(results[0].ImageURL)
}

// scheduleAnimeCoverEnrichment fills missing anime covers in the background after import.
func (a *App) scheduleAnimeCoverEnrichment(userID int) {
	if a == nil || a.DB == nil || userID <= 0 {
		return
	}
	go a.enrichAnimeCoversMissing(userID)
}

func (a *App) enrichAnimeCoversMissing(userID int) {
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(source, ''), COALESCE(external_id, '')
         FROM anime_works
         WHERE user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')
         ORDER BY id
         LIMIT ?`,
		userID, animeCoverEnrichBatchLimit,
	)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	type pending struct {
		id         int
		title      string
		source     string
		externalID string
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.title, &p.source, &p.externalID); err != nil {
			return
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil || len(list) == 0 {
		return
	}

	for _, p := range list {
		cover := resolveAnimeCoverURL(p.source, p.externalID, p.title)
		if cover == "" {
			continue
		}
		_, _ = a.DB.Exec(
			`UPDATE anime_works SET image_path = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			cover, p.id, userID,
		)
	}
}
