package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const mangadexAPIBase = "https://api.mangadex.org"
const mangadexTimeout = 10 * time.Second
const mangadexMinInterval = 250 * time.Millisecond

var (
	mdMu         sync.Mutex
	mdLastCall   time.Time
	mdHTTPClient = &http.Client{Timeout: mangadexTimeout}
)

func mangadexThrottle() {
	mdMu.Lock()
	defer mdMu.Unlock()
	if elapsed := time.Since(mdLastCall); elapsed < mangadexMinInterval {
		time.Sleep(mangadexMinInterval - elapsed)
	}
	mdLastCall = time.Now()
}

type mdMangaListResponse struct {
	Result string `json:"result"`
	Data   []struct {
		ID            string            `json:"id"`
		Attributes    mdMangaAttributes `json:"attributes"`
		Relationships []mdRelationship  `json:"relationships"`
	} `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type mdMangaAttributes struct {
	Title            map[string]string   `json:"title"`
	AltTitles        []map[string]string `json:"altTitles"`
	Description      map[string]string   `json:"description"`
	OriginalLanguage string              `json:"originalLanguage"`
	ContentRating    string              `json:"contentRating"`
	Tags             []mdTagRef          `json:"tags"`
}

type mdTagRef struct {
	ID         string `json:"id"`
	Attributes struct {
		Name  map[string]string `json:"name"`
		Group string            `json:"group"`
	} `json:"attributes"`
}

type mdRelationship struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type mdIncludedCover struct {
	ID         string `json:"id"`
	Attributes struct {
		FileName string `json:"fileName"`
	} `json:"attributes"`
}

// BrowseMangaDexParams filters MangaDex /manga listing (post-filtered where the API lacks parity).
type BrowseMangaDexParams struct {
	GenreIn        []string
	PerPage        int
	Page           int
	Sort           string // POPULARITY_DESC, SCORE_DESC
	NotInIDs       map[string]struct{}
	IsAdult        *bool
	ReadingTypesIn []string
	TagNotIn       []string
	MediaMatch     func(genres, tags []string) bool
	MaxResults     int
}

func mangadexGET(path string, query url.Values) ([]byte, error) {
	mangadexThrottle()
	u := mangadexAPIBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := mdHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		time.Sleep(800 * time.Millisecond)
		return mangadexGET(path, query)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mangadex_http_%d", resp.StatusCode)
	}
	return body, nil
}

func pickMangaDexTitle(titles map[string]string) string {
	for _, key := range []string{"en", "fr", "ja-ro", "ja", "ko", "zh", "zh-hk"} {
		if v := strings.TrimSpace(titles[key]); v != "" {
			return v
		}
	}
	for _, v := range titles {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func mapMangaDexReadingType(attrs mdMangaAttributes, genres, tags []string) string {
	lang := strings.ToLower(strings.TrimSpace(attrs.OriginalLanguage))
	if lang == "ko" || lang == "zh" || lang == "zh-hk" {
		return "Webtoon"
	}
	combined := append(append([]string(nil), genres...), tags...)
	for _, g := range combined {
		if isWebtoonKeyword(g) {
			return "Webtoon"
		}
		if strings.Contains(strings.ToLower(g), "novel") || strings.Contains(strings.ToLower(g), "light novel") {
			return "Light Novel"
		}
	}
	return "Manga"
}

func mangadexIsAdult(contentRating string) bool {
	switch strings.ToLower(strings.TrimSpace(contentRating)) {
	case "erotica", "pornographic":
		return true
	default:
		return false
	}
}

func splitMangaDexTags(tagRefs []mdTagRef) (genres, tags []string) {
	for _, t := range tagRefs {
		name := pickMangaDexTitle(t.Attributes.Name)
		if name == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(t.Attributes.Group)) {
		case "genre":
			genres = append(genres, name)
		default:
			tags = append(tags, name)
		}
	}
	return genres, tags
}

func coverURLFromRelationships(mangaID string, rels []mdRelationship, included []mdIncludedCover) string {
	var coverID string
	for _, r := range rels {
		if r.Type == "cover_art" {
			coverID = r.ID
			break
		}
	}
	if coverID == "" || mangaID == "" {
		return ""
	}
	for _, inc := range included {
		if inc.ID == coverID && strings.TrimSpace(inc.Attributes.FileName) != "" {
			return "https://uploads.mangadex.org/covers/" + mangaID + "/" + inc.Attributes.FileName
		}
	}
	return ""
}

func mapMangaDexHit(mangaID string, attrs mdMangaAttributes, rels []mdRelationship, included []mdIncludedCover, tagNotIn []string, mediaMatch func(genres, tags []string) bool, typeFilter map[string]struct{}, isAdult *bool) (CatalogMediaHit, bool) {
	genres, tags := splitMangaDexTags(attrs.Tags)
	if len(tagNotIn) > 0 && labelsMatchAny(append(append([]string(nil), genres...), tags...), tagNotIn) {
		return CatalogMediaHit{}, false
	}
	if mediaMatch != nil && !mediaMatch(genres, tags) {
		return CatalogMediaHit{}, false
	}
	adult := mangadexIsAdult(attrs.ContentRating)
	if isAdult != nil && adult != *isAdult {
		return CatalogMediaHit{}, false
	}
	readingType := mapMangaDexReadingType(attrs, genres, tags)
	if len(typeFilter) > 0 {
		if _, ok := typeFilter[readingType]; !ok {
			return CatalogMediaHit{}, false
		}
	}
	return CatalogMediaHit{
		Source:      "mangadex",
		ExternalID:  mangaID,
		Title:       pickMangaDexTitle(attrs.Title),
		ReadingType: readingType,
		ImageURL:    coverURLFromRelationships(mangaID, rels, included),
		Genres:      genres,
		Tags:        tags,
		IsAdult:     adult,
	}, true
}

func parseMangaDexList(body []byte, p BrowseMangaDexParams) ([]CatalogMediaHit, error) {
	var raw struct {
		Result string `json:"result"`
		Data   []struct {
			ID            string            `json:"id"`
			Attributes    mdMangaAttributes `json:"attributes"`
			Relationships []mdRelationship  `json:"relationships"`
		} `json:"data"`
		Included []mdIncludedCover `json:"included"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Result != "ok" {
		return nil, fmt.Errorf("mangadex: bad result")
	}
	typeFilter := make(map[string]struct{}, len(p.ReadingTypesIn))
	for _, rt := range p.ReadingTypesIn {
		typeFilter[rt] = struct{}{}
	}
	max := p.MaxResults
	if max <= 0 {
		max = p.PerPage
	}
	var out []CatalogMediaHit
	for _, item := range raw.Data {
		if p.NotInIDs != nil {
			if _, skip := p.NotInIDs[item.ID]; skip {
				continue
			}
		}
		if len(p.GenreIn) > 0 {
			genres, _ := splitMangaDexTags(item.Attributes.Tags)
			if !labelsMatchAny(genres, p.GenreIn) {
				continue
			}
		}
		hit, ok := mapMangaDexHit(item.ID, item.Attributes, item.Relationships, raw.Included, p.TagNotIn, p.MediaMatch, typeFilter, p.IsAdult)
		if !ok || hit.Title == "" {
			continue
		}
		out = append(out, hit)
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

func mangadexListQuery(p BrowseMangaDexParams) url.Values {
	q := url.Values{}
	q.Set("includes[]", "cover_art")
	limit := p.PerPage
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q.Set("limit", fmt.Sprintf("%d", limit))
	offset := 0
	if p.Page > 1 {
		offset = (p.Page - 1) * limit
	}
	q.Set("offset", fmt.Sprintf("%d", offset))
	switch p.Sort {
	case "SCORE_DESC":
		q.Set("order[rating]", "desc")
	default:
		q.Set("order[followedCount]", "desc")
	}
	if p.IsAdult != nil {
		if *p.IsAdult {
			q.Set("contentRating[]", "erotica")
			q.Add("contentRating[]", "pornographic")
		} else {
			q.Set("contentRating[]", "safe")
			q.Add("contentRating[]", "suggestive")
		}
	} else {
		q.Set("contentRating[]", "safe")
		q.Add("contentRating[]", "suggestive")
		q.Add("contentRating[]", "erotica")
	}
	return q
}

// BrowseMangaDex runs one MangaDex /manga page with local filters.
func BrowseMangaDex(p BrowseMangaDexParams) ([]CatalogMediaHit, int, error) {
	if p.PerPage <= 0 {
		p.PerPage = 20
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	body, err := mangadexGET("/manga", mangadexListQuery(p))
	if err != nil {
		return nil, 0, err
	}
	var meta mdMangaListResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, 0, err
	}
	hits, err := parseMangaDexList(body, p)
	if err != nil {
		return nil, 0, err
	}
	return hits, len(meta.Data), nil
}

// BrowseMangaDexCollect pages until skip/take satisfied (like BrowseMediaCollect).
func BrowseMangaDexCollect(p BrowseMangaDexParams, skip, take int) ([]CatalogMediaHit, bool, error) {
	if take <= 0 {
		take = 20
	}
	perPage := p.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	p.PerPage = perPage

	type batch struct {
		items []CatalogMediaHit
		full  bool
	}
	var pages []batch
	const maxAPIPages = 40

	for apiPage := 1; apiPage <= maxAPIPages; apiPage++ {
		pageP := p
		pageP.Page = apiPage
		pageP.MaxResults = perPage
		items, sourceCount, err := BrowseMangaDex(pageP)
		if err != nil {
			return nil, false, err
		}
		pages = append(pages, batch{items: items, full: sourceCount >= perPage})
		if !pages[len(pages)-1].full {
			break
		}
		total := 0
		for _, pg := range pages {
			total += len(pg.items)
		}
		if total >= skip+take {
			break
		}
	}

	var out []CatalogMediaHit
	skipped := 0
	for pi, page := range pages {
		for i, item := range page.items {
			if skipped < skip {
				skipped++
				continue
			}
			out = append(out, item)
			if len(out) >= take {
				hasNext := i < len(page.items)-1 || page.full || pi < len(pages)-1
				return out, hasNext, nil
			}
		}
	}
	return out, false, nil
}

// SearchMangaDex searches MangaDex by title.
func SearchMangaDex(query string, limit int) ([]CatalogMediaHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{}
	q.Set("title", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("includes[]", "cover_art")
	q.Set("contentRating[]", "safe")
	q.Add("contentRating[]", "suggestive")
	q.Add("contentRating[]", "erotica")
	body, err := mangadexGET("/manga", q)
	if err != nil {
		return nil, err
	}
	return parseMangaDexList(body, BrowseMangaDexParams{PerPage: limit, MaxResults: limit})
}
