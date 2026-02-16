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

type anilistResponse struct {
	Data struct {
		Page struct {
			Media []struct {
				ID    int `json:"id"`
				Title struct {
					Romaji  string `json:"romaji"`
					English string `json:"english"`
				} `json:"title"`
				Type    string `json:"type"`
				CoverImage struct {
					Large string `json:"large"`
				} `json:"coverImage"`
			} `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

// Map AniList type to our reading types
func mapAnilistType(t string) string {
	switch strings.ToUpper(t) {
	case "MANGA":
		return "Manga"
	case "NOVEL", "LIGHT_NOVEL":
		return "Light Novel"
	case "ONE_SHOT":
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
      coverImage { large }
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
			ReadingType:  mapAnilistType(m.Type),
		})
	}
	return results, nil
}
