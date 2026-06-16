package catalog

import (
	"encoding/json"
	"strings"
	"time"
)

const anilistURL = "https://graphql.anilist.co"
const anilistTimeout = 8 * time.Second

// AnilistResult represents one search result from AniList
type AnilistResult struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	TitleRomaji  string   `json:"title_romaji,omitempty"`
	TitleEnglish string   `json:"title_english,omitempty"`
	Type         string   `json:"type"`
	ImageURL     string   `json:"image_url"`
	ReadingType  string   `json:"reading_type"` // mapped for our app
	IsAdult      bool     `json:"is_adult"`
	Genres       []string `json:"genres,omitempty"`
	Tags         []string `json:"tags,omitempty"` // tag names only
}

type anilistMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Type            string   `json:"type"`
	Format          string   `json:"format"`
	CountryOfOrigin string   `json:"countryOfOrigin"`
	Genres          []string `json:"genres"`
	IsAdult         bool     `json:"isAdult"`
	CoverImage      struct {
		Large string `json:"large"`
	} `json:"coverImage"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

type anilistResponse struct {
	Data struct {
		Page struct {
			Media []anilistMedia `json:"media"`
		} `json:"Page"`
	} `json:"data"`
	Errors []anilistGraphQLErrorItem `json:"errors"`
}

// isWebtoonKeyword returns true if s (lowercase) indicates webtoon/manhwa/manhua.
func isWebtoonKeyword(s string) bool {
	s = strings.ToLower(s)
	return s == "webtoon" || s == "manhwa" || s == "manhua" ||
		strings.Contains(s, "webtoon") || strings.Contains(s, "web comic") || strings.Contains(s, "webcomic") ||
		strings.Contains(s, "manhwa") || strings.Contains(s, "manhua")
}

// anilistMediaTypeUpper returns uppercased Type, or infers from Format when Type is missing.
func anilistMediaTypeUpper(media anilistMedia) string {
	t := strings.ToUpper(strings.TrimSpace(media.Type))
	if t != "" {
		return t
	}
	f := strings.ToUpper(strings.TrimSpace(media.Format))
	switch f {
	case "MANGA", "ONE_SHOT":
		return "MANGA"
	case "NOVEL":
		return "NOVEL"
	}
	return ""
}

// Map AniList type, format, country, genres and tags to BookStorage reading types.
func mapAnilistReadingType(media anilistMedia) string {
	country := strings.ToUpper(strings.TrimSpace(media.CountryOfOrigin))
	t := anilistMediaTypeUpper(media)
	if t == "MANGA" && (country == "KR" || country == "KP" || country == "CN") {
		return "Webtoon"
	}
	for _, g := range media.Genres {
		if isWebtoonKeyword(g) {
			return "Webtoon"
		}
	}
	for _, tag := range media.Tags {
		if isWebtoonKeyword(tag.Name) {
			return "Webtoon"
		}
	}
	f := strings.ToUpper(media.Format)
	switch {
	case t == "NOVEL" || f == "NOVEL" || f == "LIGHT_NOVEL":
		return "Light Novel"
	case t == "MANGA", f == "MANGA", f == "ONE_SHOT":
		return "Manga"
	default:
		return "Manga"
	}
}

// SearchAnilist queries AniList GraphQL API for manga/novels by title
func SearchAnilist(query string, limit int) ([]AnilistResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	graphqlQuery := `query($search: String, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(type: MANGA, search: $search) {
      id
      title { romaji english }
      type
      format
      countryOfOrigin
      genres
      isAdult
      coverImage { large }
      tags { name }
    }
  }
}`
	payload := map[string]any{
		"query": graphqlQuery,
		"variables": map[string]any{
			"search":  strings.TrimSpace(query),
			"perPage": limit,
		},
	}
	body, _ := json.Marshal(payload)
	var out anilistResponse
	if err := anilistPostAndDecode(body, &out); err != nil {
		return nil, err
	}
	if err := firstGraphQLError(anilistErrorMessages(out.Errors)); err != nil {
		return nil, err
	}
	var results []AnilistResult
	for _, m := range out.Data.Page.Media {
		romaji := strings.TrimSpace(m.Title.Romaji)
		english := strings.TrimSpace(m.Title.English)
		title := romaji
		if english != "" {
			title = english
		}
		if title == "" {
			title = romaji
		}
		results = append(results, AnilistResult{
			ID:           m.ID,
			Title:        title,
			TitleRomaji:  romaji,
			TitleEnglish: english,
			Type:         m.Type,
			ImageURL:     m.CoverImage.Large,
			ReadingType:  mapAnilistReadingType(m),
			IsAdult:      m.IsAdult,
		})
	}
	return results, nil
}
