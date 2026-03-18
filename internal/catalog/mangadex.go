package catalog

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mangadexURL = "https://api.mangadex.org"
const mangadexTimeout = 8 * time.Second

// MangadexResult represents one search result from MangaDex
type MangadexResult struct {
	ID          string
	Title       string
	ReadingType string
	ImageURL    string
	IsAdult     bool
}

type mangadexMangaData struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Title map[string]string `json:"title"`
	} `json:"attributes"`
	Relationships []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"relationships"`
}

type mangadexIncluded struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		FileName string `json:"fileName"`
	} `json:"attributes"`
}

type mangadexResponse struct {
	Result   string              `json:"result"`
	Data     []mangadexMangaData `json:"data"`
	Included []mangadexIncluded  `json:"included"`
}

// SearchMangadex queries MangaDex API for manga by title
func SearchMangadex(query string, limit int) ([]MangadexResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	u, err := url.Parse(mangadexURL + "/manga")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("title", strings.TrimSpace(query))
	q.Set("limit", "10")
	q.Add("includes[]", "cover_art")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: mangadexTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out mangadexResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var results []MangadexResult
	for _, m := range out.Data {
		title := ""
		if t, ok := m.Attributes.Title["en"]; ok && t != "" {
			title = t
		}
		if title == "" {
			for _, v := range m.Attributes.Title {
				if v != "" {
					title = v
					break
				}
			}
		}
		if title == "" {
			continue
		}

		imageURL := ""
		for _, rel := range m.Relationships {
			if rel.Type == "cover_art" && rel.ID != "" {
				for _, inc := range out.Included {
					if inc.Type == "cover_art" && inc.ID == rel.ID && inc.Attributes.FileName != "" {
						imageURL = "https://uploads.mangadex.org/covers/" + m.ID + "/" + inc.Attributes.FileName + ".256.jpg"
						break
					}
				}
				break
			}
		}

		readingType := "Manga"
		results = append(results, MangadexResult{
			ID:          m.ID,
			Title:       title,
			ReadingType: readingType,
			ImageURL:    imageURL,
			IsAdult:     false,
		})
	}
	return results, nil
}
