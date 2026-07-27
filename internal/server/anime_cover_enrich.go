package server

import (
	"log"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/catalog"
)

const (
	animeCoverEnrichBatchSize   = 100
	animeCoverEnrichMaxPasses   = 50
	animeCoverEnrichRateRetries = 8
)

// Overridable in tests to keep enrichment suites fast.
var (
	animeCoverEnrichPace             = 650 * time.Millisecond
	animeCoverEnrichInitialBackoff   = 30 * time.Second
	animeCoverEnrichRateLimitSleep   = time.Sleep
)

// resolveAnimeCoverURL looks up a cover image URL via AniList (MAL id, AniList id, or title search).
// Overridable in tests. A rate-limit error should be retried by the caller; other empty results are final for this pass.
var resolveAnimeCoverURL = resolveAnimeCoverURLDefault

func resolveAnimeCoverURLDefault(source, externalID, title string) (string, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if id, err := strconv.Atoi(ext); err == nil && id > 0 {
		switch source {
		case "anilist":
			res, err := catalog.GetAnilistAnimeByID(id)
			if err != nil {
				return "", err
			}
			if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
		case "mal", "myanimelist":
			res, err := catalog.GetAnilistAnimeByMALID(id)
			if err != nil {
				return "", err
			}
			if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
			// MAL id unknown on AniList: fall through to title search.
		default:
			// Unknown source with a numeric id: try AniList id then MAL id.
			if res, err := catalog.GetAnilistAnimeByID(id); err != nil {
				return "", err
			} else if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
			if res, err := catalog.GetAnilistAnimeByMALID(id); err != nil {
				return "", err
			} else if res != nil {
				return strings.TrimSpace(res.ImageURL), nil
			}
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", nil
	}
	results, err := catalog.SearchAnilistAnime(title, 1)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	return strings.TrimSpace(results[0].ImageURL), nil
}

// scheduleAnimeCoverEnrichment fills missing anime covers in the background after import.
func (a *App) scheduleAnimeCoverEnrichment(userID int) {
	if a == nil || a.DB == nil || userID <= 0 {
		return
	}
	go a.enrichAnimeCoversMissing(userID)
}

func (a *App) enrichAnimeCoversMissing(userID int) {
	lastID := 0
	for pass := 0; pass < animeCoverEnrichMaxPasses; pass++ {
		list, err := a.loadAnimeMissingCovers(userID, lastID, animeCoverEnrichBatchSize)
		if err != nil || len(list) == 0 {
			return
		}
		for _, p := range list {
			lastID = p.id
			a.enrichOneAnimeCover(userID, p)
			time.Sleep(animeCoverEnrichPace)
		}
		if len(list) < animeCoverEnrichBatchSize {
			return
		}
	}
}

type animeCoverPending struct {
	id         int
	title      string
	source     string
	externalID string
}

func (a *App) loadAnimeMissingCovers(userID, afterID, limit int) ([]animeCoverPending, error) {
	rows, err := a.DB.Query(
		`SELECT id, title, COALESCE(source, ''), COALESCE(external_id, '')
         FROM anime_works
         WHERE user_id = ? AND id > ? AND (image_path IS NULL OR TRIM(image_path) = '')
         ORDER BY id
         LIMIT ?`,
		userID, afterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []animeCoverPending
	for rows.Next() {
		var p animeCoverPending
		if err := rows.Scan(&p.id, &p.title, &p.source, &p.externalID); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (a *App) enrichOneAnimeCover(userID int, p animeCoverPending) {
	backoff := animeCoverEnrichInitialBackoff
	for attempt := 0; attempt < animeCoverEnrichRateRetries; attempt++ {
		cover, err := resolveAnimeCoverURL(p.source, p.externalID, p.title)
		if catalog.IsAnilistRateLimit(err) {
			log.Printf("[anime-covers] rate limit user=%d id=%d attempt=%d; sleeping %s", userID, p.id, attempt+1, backoff)
			animeCoverEnrichRateLimitSleep(backoff)
			if backoff < 2*time.Minute {
				backoff *= 2
			}
			continue
		}
		if err != nil || cover == "" {
			return
		}
		_, _ = a.DB.Exec(
			`UPDATE anime_works SET image_path = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ? AND (image_path IS NULL OR TRIM(image_path) = '')`,
			cover, p.id, userID,
		)
		return
	}
}
