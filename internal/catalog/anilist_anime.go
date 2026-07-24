package catalog

import (
	"encoding/json"
	"strings"
)

// AnilistAnimeResult represents one anime search/browse result from AniList.
type AnilistAnimeResult struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	TitleRomaji  string   `json:"title_romaji,omitempty"`
	TitleEnglish string   `json:"title_english,omitempty"`
	ImageURL     string   `json:"image_url"`
	AnimeType    string   `json:"anime_type"` // mapped to BookStorage animeTypes (TV, Film, OVA, ONA, Spécial)
	Episodes     int      `json:"episodes"`   // total episodes, 0 if unknown
	Season       string   `json:"season,omitempty"`
	SeasonYear   int      `json:"season_year,omitempty"`
	IsAdult      bool     `json:"is_adult"`
	Genres       []string `json:"genres,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type anilistAnimeMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Type       string   `json:"type"`
	Format     string   `json:"format"`
	Episodes   *int     `json:"episodes"`
	Season     string   `json:"season"`
	SeasonYear *int     `json:"seasonYear"`
	Genres     []string `json:"genres"`
	IsAdult    bool     `json:"isAdult"`
	CoverImage struct {
		Large string `json:"large"`
	} `json:"coverImage"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

type anilistAnimeResponse struct {
	Data struct {
		Page struct {
			Media []anilistAnimeMedia `json:"media"`
		} `json:"Page"`
	} `json:"data"`
	Errors []anilistGraphQLErrorItem `json:"errors"`
}

// mapAnilistAnimeType maps AniList format to BookStorage anime types.
func mapAnilistAnimeType(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "TV", "TV_SHORT":
		return "TV"
	case "MOVIE":
		return "Film"
	case "OVA":
		return "OVA"
	case "ONA":
		return "ONA"
	case "SPECIAL", "MUSIC":
		return "Spécial"
	default:
		return "TV"
	}
}

const anilistAnimeFields = `
      id
      title { romaji english }
      type
      format
      episodes
      season
      seasonYear
      genres
      isAdult
      coverImage { large }
      tags { name }`

func anilistAnimeResultFromMedia(m anilistAnimeMedia) AnilistAnimeResult {
	romaji := strings.TrimSpace(m.Title.Romaji)
	english := strings.TrimSpace(m.Title.English)
	title := romaji
	if english != "" {
		title = english
	}
	if title == "" {
		title = romaji
	}
	episodes := 0
	if m.Episodes != nil {
		episodes = *m.Episodes
	}
	seasonYear := 0
	if m.SeasonYear != nil {
		seasonYear = *m.SeasonYear
	}
	var tagNames []string
	for _, tg := range m.Tags {
		tagNames = append(tagNames, tg.Name)
	}
	return AnilistAnimeResult{
		ID:           m.ID,
		Title:        title,
		TitleRomaji:  romaji,
		TitleEnglish: english,
		ImageURL:     m.CoverImage.Large,
		AnimeType:    mapAnilistAnimeType(m.Format),
		Episodes:     episodes,
		Season:       strings.TrimSpace(m.Season),
		SeasonYear:   seasonYear,
		IsAdult:      m.IsAdult,
		Genres:       append([]string(nil), m.Genres...),
		Tags:         tagNames,
	}
}

// SearchAnilistAnime queries AniList GraphQL API for anime by title.
func SearchAnilistAnime(query string, limit int) ([]AnilistAnimeResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	graphqlQuery := `query($search: String, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(type: ANIME, search: $search) {` + anilistAnimeFields + `
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
	var out anilistAnimeResponse
	if err := anilistPostAndDecode(body, &out); err != nil {
		return nil, err
	}
	if err := firstGraphQLError(anilistErrorMessages(out.Errors)); err != nil {
		return nil, err
	}
	var results []AnilistAnimeResult
	for _, m := range out.Data.Page.Media {
		results = append(results, anilistAnimeResultFromMedia(m))
	}
	return results, nil
}

type anilistAnimeByIDResponse struct {
	Data struct {
		Media *anilistAnimeMedia `json:"Media"`
	} `json:"data"`
	Errors []anilistGraphQLErrorItem `json:"errors"`
}

// GetAnilistAnimeByID loads one anime Media by AniList id (for add-form prefill).
func GetAnilistAnimeByID(id int) (*AnilistAnimeResult, error) {
	if id <= 0 {
		return nil, nil
	}
	q := `query($id: Int) {
  Media(id: $id, type: ANIME) {` + anilistAnimeFields + `
  }
}`
	payload := map[string]any{
		"query":     q,
		"variables": map[string]any{"id": id},
	}
	body, _ := json.Marshal(payload)
	var out anilistAnimeByIDResponse
	if err := anilistPostAndDecode(body, &out); err != nil {
		return nil, err
	}
	if err := firstGraphQLError(anilistErrorMessages(out.Errors)); err != nil {
		return nil, err
	}
	if out.Data.Media == nil {
		return nil, nil
	}
	res := anilistAnimeResultFromMedia(*out.Data.Media)
	return &res, nil
}

// BrowseAnimeParams filters Page.media for anime (type: ANIME).
type BrowseAnimeParams struct {
	GenreIn  []string
	Page     int
	PerPage  int
	Sort     string // POPULARITY_DESC, SCORE_DESC
	NotInIDs map[int]struct{}
	IsAdult  *bool
}

// BrowseAnimeMedia runs a single Page query for anime with genre filters.
func BrowseAnimeMedia(p BrowseAnimeParams) ([]AnilistAnimeResult, error) {
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
	q := `query($page: Int, $perPage: Int, $genreIn: [String], $sort: [MediaSort], $isAdult: Boolean) {
  Page(page: $page, perPage: $perPage) {
    media(type: ANIME, genre_in: $genreIn, sort: $sort, isAdult: $isAdult) {` + anilistAnimeFields + `
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
	if p.IsAdult != nil {
		vars["isAdult"] = *p.IsAdult
	} else {
		vars["isAdult"] = nil
	}
	payload := map[string]any{"query": q, "variables": vars}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var out anilistAnimeResponse
	if err := anilistPostAndDecode(raw, &out); err != nil {
		return nil, err
	}
	if err := firstGraphQLError(anilistErrorMessages(out.Errors)); err != nil {
		return nil, err
	}
	var results []AnilistAnimeResult
	for _, m := range out.Data.Page.Media {
		if p.NotInIDs != nil {
			if _, skip := p.NotInIDs[m.ID]; skip {
				continue
			}
		}
		if p.IsAdult != nil && m.IsAdult != *p.IsAdult {
			continue
		}
		results = append(results, anilistAnimeResultFromMedia(m))
	}
	return results, nil
}
