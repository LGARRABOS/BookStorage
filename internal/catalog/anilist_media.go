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
				ID:           rm.ID,
				Title:        title,
				TitleRomaji:  strings.TrimSpace(rm.Title.Romaji),
				TitleEnglish: strings.TrimSpace(rm.Title.English),
				Type:         rm.Type,
				ImageURL:     rm.CoverImage.Large,
				ReadingType:  mapAnilistReadingType(rm),
				IsAdult:      rm.IsAdult,
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
	GenreIn        []string // e.g. "Romance"
	TagIn          []string // AniList tag names
	TagNotIn       []string // AniList tag exclusions
	Page           int
	PerPage        int
	Sort           string // POPULARITY_DESC, SCORE_DESC
	NotInIDs       map[int]struct{}
	MaxResults     int
	IsAdult        *bool    // nil = no AniList isAdult filter; true/false filters at API and in-loop
	ReadingTypesIn []string // BookStorage reading_type labels (post-filter)
	TagMatch       func(tagNames []string) bool
}

// BrowseMedia runs a single Page query with genre/tag filters (OR within lists per AniList rules).
// The second return value is the raw item count from AniList before local filters.
func BrowseMedia(p BrowseMediaParams) ([]AnilistResult, int, error) {
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
	q := `query($page: Int, $perPage: Int, $genreIn: [String], $tagIn: [String], $tagNotIn: [String], $sort: [MediaSort], $isAdult: Boolean) {
  Page(page: $page, perPage: $perPage) {
    media(type: MANGA, genre_in: $genreIn, tag_in: $tagIn, tag_not_in: $tagNotIn, sort: $sort, isAdult: $isAdult) {
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
	if len(p.TagNotIn) > 0 {
		vars["tagNotIn"] = p.TagNotIn
	} else {
		vars["tagNotIn"] = nil
	}
	if p.IsAdult != nil {
		vars["isAdult"] = *p.IsAdult
	} else {
		vars["isAdult"] = nil
	}
	payload := map[string]any{"query": q, "variables": vars}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	resp, err := anilistPost(raw)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out browsePageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	if len(out.Errors) > 0 {
		return nil, 0, fmt.Errorf("anilist: %s", out.Errors[0].Message)
	}
	sourceCount := len(out.Data.Page.Media)
	typeFilter := make(map[string]struct{}, len(p.ReadingTypesIn))
	for _, rt := range p.ReadingTypesIn {
		typeFilter[rt] = struct{}{}
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
		if p.IsAdult != nil && m.IsAdult != *p.IsAdult {
			continue
		}
		title := pickTitleFromAnilistTitle(anilistTitle{Romaji: m.Title.Romaji, English: m.Title.English})
		var tagNames []string
		for _, tg := range m.Tags {
			tagNames = append(tagNames, tg.Name)
		}
		if p.TagMatch != nil && !p.TagMatch(tagNames) {
			continue
		}
		readingType := mapAnilistReadingType(m)
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[readingType]; !ok {
				continue
			}
		}
		results = append(results, AnilistResult{
			ID:           m.ID,
			Title:        title,
			TitleRomaji:  strings.TrimSpace(m.Title.Romaji),
			TitleEnglish: strings.TrimSpace(m.Title.English),
			Type:         m.Type,
			ImageURL:     m.CoverImage.Large,
			ReadingType:  readingType,
			IsAdult:      m.IsAdult,
			Genres:       append([]string(nil), m.Genres...),
			Tags:         tagNames,
		})
		if len(results) >= max {
			break
		}
	}
	return results, sourceCount, nil
}

type browsePageBatch struct {
	items []AnilistResult
	full  bool
}

func paginateBrowseBatches(pages []browsePageBatch, skip, take int) ([]AnilistResult, bool) {
	if take <= 0 {
		take = 20
	}
	var out []AnilistResult
	skipped := 0

	for _, page := range pages {
		batch := page.items
		for i, item := range batch {
			if skipped < skip {
				skipped++
				continue
			}
			out = append(out, item)
			if len(out) >= take {
				hasNext := i < len(batch)-1 || page.full
				return out, hasNext
			}
		}
	}
	return out, false
}

// BrowseMediaCollect pages through AniList until skip filtered results are discarded
// and up to take matches are returned (for post-filters like reading type or library exclusion).
func BrowseMediaCollect(p BrowseMediaParams, skip, take int) ([]AnilistResult, bool, error) {
	if take <= 0 {
		take = 20
	}
	perPage := p.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 25 {
		perPage = 25
	}
	p.PerPage = perPage

	var pages []browsePageBatch
	const maxAPIPages = 40

	for apiPage := 1; apiPage <= maxAPIPages; apiPage++ {
		pageP := p
		pageP.Page = apiPage
		pageP.MaxResults = perPage

		batch, sourceCount, err := BrowseMedia(pageP)
		if err != nil {
			return nil, false, err
		}
		pages = append(pages, browsePageBatch{
			items: batch,
			full:  sourceCount >= perPage,
		})
		if !pages[len(pages)-1].full {
			break
		}
		// Stop early once we have enough data to satisfy skip+take.
		total := 0
		for _, pg := range pages {
			total += len(pg.items)
		}
		if total >= skip+take {
			break
		}
	}

	out, hasNext := paginateBrowseBatches(pages, skip, take)
	return out, hasNext, nil
}

// AnilistMediaToResult converts a parsed anilistMedia to AnilistResult (used by recommend package).
func AnilistMediaToResult(m anilistMedia) AnilistResult {
	title := pickTitleFromAnilistTitle(anilistTitle{Romaji: m.Title.Romaji, English: m.Title.English})
	if title == "" {
		title = m.Title.Romaji
	}
	return AnilistResult{
		ID:           m.ID,
		Title:        title,
		TitleRomaji:  strings.TrimSpace(m.Title.Romaji),
		TitleEnglish: strings.TrimSpace(m.Title.English),
		Type:         m.Type,
		ImageURL:     m.CoverImage.Large,
		ReadingType:  mapAnilistReadingType(m),
		IsAdult:      m.IsAdult,
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
