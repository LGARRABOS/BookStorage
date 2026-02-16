package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const anilistURL = "https://graphql.anilist.co"
const anilistTimeout = 8 * time.Second

// AnilistResult represents one search result from AniList
type AnilistResult struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	ImageURL   string `json:"image_url"`
	ReadingType string `json:"reading_type"` // mapped for our app
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
}

// isWebtoonKeyword returns true if s (lowercase) indicates webtoon/manhwa/manhua
func isWebtoonKeyword(s string) bool {
	s = strings.ToLower(s)
	return s == "webtoon" || s == "manhwa" || s == "manhua" ||
		strings.Contains(s, "webtoon") || strings.Contains(s, "web comic") || strings.Contains(s, "webcomic")
}

// Map AniList type, format, country, genres and tags to our reading types (Manga vs Webtoon etc.)
func mapAnilistReadingType(media anilistMedia) string {
	// AniList n'a pas de format WEBTOON : on s'appuie sur genres, tags et pays d'origine
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
	country := strings.ToUpper(strings.TrimSpace(media.CountryOfOrigin))
	if strings.ToUpper(media.Type) == "MANGA" && (country == "KR" || country == "KP") {
		// Corée du Sud / Corée du Nord : manhwa et webtoons
		return "Webtoon"
	}
	if strings.ToUpper(media.Type) == "MANGA" && country == "CN" {
		// Chine : beaucoup de manhua (webcomics chinois)
		return "Webtoon"
	}
	// Sinon mapping classique type/format
	t := strings.ToUpper(media.Type)
	f := strings.ToUpper(media.Format)
	switch {
	case t == "NOVEL" || f == "NOVEL" || f == "LIGHT_NOVEL":
		return "Light Novel"
	case t == "MANGA", f == "MANGA", f == "ONE_SHOT":
		return "Manga"
	default:
		return "Autre"
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
	req, err := http.NewRequest(http.MethodPost, anilistURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: anilistTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out anilistResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var results []AnilistResult
	for _, m := range out.Data.Page.Media {
		title := m.Title.Romaji
		if m.Title.English != "" {
			title = m.Title.English
		}
		if title == "" {
			title = m.Title.Romaji
		}
		results = append(results, AnilistResult{
			ID:           m.ID,
			Title:        title,
			Type:         m.Type,
			ImageURL:     m.CoverImage.Large,
			ReadingType:  mapAnilistReadingType(m),
		})
	}
	return results, nil
}
