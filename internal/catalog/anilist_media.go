package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// anilistTitle mirrors AniList title fields (romaji / english).
type anilistTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
}

// MediaDetail holds genres, tags, and related recommendations for one AniList media.
type MediaDetail struct {
	ID              int
	Title           string
	Description     string // plain text (HTML stripped)
	ImageURL        string
	Genres          []string
	Tags            []MediaTag
	Recommendations []AnilistResult
	AverageScore    int // 0–100, 0 if absent
	MeanScore       int // 0–100, 0 if absent
	RawMedia        anilistMedia
}

// MediaTag is an AniList tag with relevance rank (0-100).
type MediaTag struct {
	Name string
	Rank int
}

type mediaByIDResponse struct {
	Data struct {
		Media *struct {
			ID              int            `json:"id"`
			Title           anilistTitle   `json:"title"`
			Type            string         `json:"type"`
			Format          string         `json:"format"`
			CountryOfOrigin string         `json:"countryOfOrigin"`
			Description     *string        `json:"description"`
			AverageScore    *int           `json:"averageScore"`
			MeanScore       *int           `json:"meanScore"`
			Genres          []string       `json:"genres"`
			Tags            []mediaTagJSON `json:"tags"`
			IsAdult         bool           `json:"isAdult"`
			CoverImage      struct {
				Large string `json:"large"`
			} `json:"coverImage"`
			Recommendations *struct {
				Edges []struct {
					Node *recommendationNode `json:"node"`
				} `json:"edges"`
			} `json:"recommendations"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type mediaTagJSON struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

type recommendationNode struct {
	Rating              int          `json:"rating"`
	MediaRecommendation anilistMedia `json:"mediaRecommendation"`
}

type browsePageResponse struct {
	Data struct {
		Page struct {
			Media []anilistMedia `json:"media"`
		} `json:"Page"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func anilistPost(body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, anilistURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: anilistTimeout}
	return client.Do(req)
}

// GetMediaByID loads one Media with genres, tags, and recommendation edges.
func GetMediaByID(id int) (*MediaDetail, error) {
	if id <= 0 {
		return nil, nil
	}
	q := `query($id: Int) {
  Media(id: $id) {
    id
    title { romaji english }
    type
    format
    countryOfOrigin
    description
    averageScore
    meanScore
    genres
    tags { name rank }
    coverImage { large }
    recommendations(perPage: 25, sort: RATING_DESC) {
      edges {
        node {
          rating
          mediaRecommendation {
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
      }
    }
  }
}`
	payload := map[string]any{
		"query": q,
		"variables": map[string]any{
			"id": id,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := anilistPost(raw)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out mediaByIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", out.Errors[0].Message)
	}
	if out.Data.Media == nil {
		return nil, nil
	}
	m := out.Data.Media
	detail := &MediaDetail{
		ID:       m.ID,
		Title:    pickTitleFromAnilistTitle(m.Title),
		ImageURL: m.CoverImage.Large,
		Genres:   append([]string(nil), m.Genres...),
	}
	if m.Description != nil && *m.Description != "" {
		detail.Description = StripHTML(*m.Description)
	}
	if m.AverageScore != nil {
		detail.AverageScore = *m.AverageScore
	}
	if m.MeanScore != nil {
		detail.MeanScore = *m.MeanScore
	}
	detail.RawMedia = anilistMedia{
		ID:              m.ID,
		Type:            m.Type,
		Format:          m.Format,
		CountryOfOrigin: m.CountryOfOrigin,
		Genres:          append([]string(nil), m.Genres...),
		IsAdult:         m.IsAdult,
		CoverImage:      m.CoverImage,
	}
	detail.RawMedia.Title.Romaji = m.Title.Romaji
	detail.RawMedia.Title.English = m.Title.English
	for _, t := range m.Tags {
		detail.Tags = append(detail.Tags, MediaTag(t))
		detail.RawMedia.Tags = append(detail.RawMedia.Tags, struct {
			Name string `json:"name"`
		}{Name: t.Name})
	}
	if m.Recommendations != nil {
		for _, e := range m.Recommendations.Edges {
			if e.Node == nil {
				continue
			}
			rm := e.Node.MediaRecommendation
			title := pickTitleFromAnilistTitle(anilistTitle{Romaji: rm.Title.Romaji, English: rm.Title.English})
			detail.Recommendations = append(detail.Recommendations, AnilistResult{
				ID:          rm.ID,
				Title:       title,
				Type:        rm.Type,
				ImageURL:    rm.CoverImage.Large,
				ReadingType: mapAnilistReadingType(rm),
				IsAdult:     rm.IsAdult,
			})
		}
	}
	return detail, nil
}

func pickTitleFromAnilistTitle(t anilistTitle) string {
	if t.English != "" {
		return t.English
	}
	return t.Romaji
}

// BrowseMediaParams filters Page.media (MANGA type includes manga + LN in AniList).
type BrowseMediaParams struct {
	GenreIn    []string // e.g. "Romance"
	TagIn      []string // AniList tag names
	Page       int
	PerPage    int
	Sort       string // POPULARITY_DESC, SCORE_DESC
	NotInIDs   map[int]struct{}
	MaxResults int
}

// BrowseMedia runs a single Page query with genre/tag filters (OR within lists per AniList rules).
func BrowseMedia(p BrowseMediaParams) ([]AnilistResult, error) {
	if p.PerPage <= 0 {
		p.PerPage = 12
	}
	if p.PerPage > 25 {
		p.PerPage = 25
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	sort := p.Sort
	if sort == "" {
		sort = "POPULARITY_DESC"
	}
	q := `query($page: Int, $perPage: Int, $genreIn: [String], $tagIn: [String], $sort: [MediaSort]) {
  Page(page: $page, perPage: $perPage) {
    media(type: MANGA, genre_in: $genreIn, tag_in: $tagIn, sort: $sort) {
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
	vars := map[string]any{
		"page":    p.Page,
		"perPage": p.PerPage,
		"sort":    []string{sort},
	}
	if len(p.GenreIn) > 0 {
		vars["genreIn"] = p.GenreIn
	} else {
		vars["genreIn"] = nil
	}
	if len(p.TagIn) > 0 {
		vars["tagIn"] = p.TagIn
	} else {
		vars["tagIn"] = nil
	}
	payload := map[string]any{"query": q, "variables": vars}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := anilistPost(raw)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out browsePageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", out.Errors[0].Message)
	}
	var results []AnilistResult
	max := p.MaxResults
	if max <= 0 {
		max = p.PerPage
	}
	for _, m := range out.Data.Page.Media {
		if p.NotInIDs != nil {
			if _, skip := p.NotInIDs[m.ID]; skip {
				continue
			}
		}
		title := pickTitleFromAnilistTitle(anilistTitle{Romaji: m.Title.Romaji, English: m.Title.English})
		var tagNames []string
		for _, tg := range m.Tags {
			tagNames = append(tagNames, tg.Name)
		}
		results = append(results, AnilistResult{
			ID:          m.ID,
			Title:       title,
			Type:        m.Type,
			ImageURL:    m.CoverImage.Large,
			ReadingType: mapAnilistReadingType(m),
			IsAdult:     m.IsAdult,
			Genres:      append([]string(nil), m.Genres...),
			Tags:        tagNames,
		})
		if len(results) >= max {
			break
		}
	}
	return results, nil
}

// AnilistMediaToResult converts a parsed anilistMedia to AnilistResult (used by recommend package).
func AnilistMediaToResult(m anilistMedia) AnilistResult {
	title := pickTitleFromAnilistTitle(anilistTitle{Romaji: m.Title.Romaji, English: m.Title.English})
	if title == "" {
		title = m.Title.Romaji
	}
	return AnilistResult{
		ID:          m.ID,
		Title:       title,
		Type:        m.Type,
		ImageURL:    m.CoverImage.Large,
		ReadingType: mapAnilistReadingType(m),
		IsAdult:     m.IsAdult,
	}
}

// NormalizeTagName trims and lowercases for deduplication keys (display uses original).
func NormalizeTagName(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

// ReadingTypeFromAnilistDetail maps stored RawMedia to BookStorage reading_type labels.
func ReadingTypeFromAnilistDetail(d *MediaDetail) string {
	if d == nil {
		return ""
	}
	return mapAnilistReadingType(d.RawMedia)
}
