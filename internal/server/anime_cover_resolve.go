package server

import (
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
)

// resolveAnimeCoverURL looks up a cover image URL via AniList (MAL id, AniList id, or title search).
// Overridable in tests. A rate-limit error should be retried by the caller; other empty results are final for this pass.
// IsAdult is set when AniList metadata is available so enrichment can flag +18 entries.
var resolveAnimeCoverURL = resolveAnimeCoverURLDefault

type animeCoverResolve struct {
	URL     string
	IsAdult *bool
}

func resolveFromAnilistAnime(res *catalog.AnilistAnimeResult) animeCoverResolve {
	if res == nil {
		return animeCoverResolve{}
	}
	return animeCoverResolve{URL: strings.TrimSpace(res.ImageURL), IsAdult: adultBoolPtr(res.IsAdult)}
}

func resolveAnimeCoverURLDefault(source, externalID, title string) (animeCoverResolve, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	ext := strings.TrimSpace(externalID)
	if id, err := strconv.Atoi(ext); err == nil && id > 0 {
		switch source {
		case "anilist":
			res, err := catalog.GetAnilistAnimeByID(id)
			if err != nil {
				return animeCoverResolve{}, err
			}
			return resolveFromAnilistAnime(res), nil
		case "mal", "myanimelist":
			res, err := catalog.GetAnilistAnimeByMALID(id)
			if err != nil {
				return animeCoverResolve{}, err
			}
			return resolveFromAnilistAnime(res), nil
		default:
			if res, err := catalog.GetAnilistAnimeByID(id); err != nil {
				return animeCoverResolve{}, err
			} else if res != nil {
				return resolveFromAnilistAnime(res), nil
			}
			if res, err := catalog.GetAnilistAnimeByMALID(id); err != nil {
				return animeCoverResolve{}, err
			} else if res != nil {
				return resolveFromAnilistAnime(res), nil
			}
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return animeCoverResolve{}, nil
	}
	results, err := catalog.SearchAnilistAnime(title, 1)
	if err != nil {
		return animeCoverResolve{}, err
	}
	if len(results) == 0 {
		return animeCoverResolve{}, nil
	}
	return resolveFromAnilistAnime(&results[0]), nil
}
