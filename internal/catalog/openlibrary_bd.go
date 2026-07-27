package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	openLibrarySearchURL      = "https://openlibrary.org/search.json"
	openLibraryMaxBodyBytes   = 4 << 20
	openLibraryMinInterval    = 300 * time.Millisecond
	openLibraryUserAgent      = "BookStorage/1.0 (+https://github.com/LGARRABOS/BookStorage)"
	openLibraryDefaultTimeout = 12 * time.Second
)

// OpenLibraryBdResult is one bande dessinée hit from Open Library search.
type OpenLibraryBdResult struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	ImageURL         string   `json:"image_url,omitempty"`
	Authors          []string `json:"authors,omitempty"`
	BdType           string   `json:"bd_type,omitempty"`
	IsAdult          bool     `json:"is_adult"`
	FirstPublishYear int      `json:"first_publish_year,omitempty"`
}

type openLibrarySearchDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	CoverI           *int     `json:"cover_i"`
	FirstPublishYear *int     `json:"first_publish_year"`
	Subject          []string `json:"subject"`
}

type openLibrarySearchResponse struct {
	NumFound int                    `json:"numFound"`
	Docs     []openLibrarySearchDoc `json:"docs"`
}

var (
	olMu         sync.Mutex
	olLastCall   time.Time
	olHTTPClient = &http.Client{Timeout: openLibraryDefaultTimeout}
	// olSearchBase is overridable in tests.
	olSearchBase = openLibrarySearchURL
)

func openLibraryThrottle() {
	olMu.Lock()
	defer olMu.Unlock()
	if elapsed := time.Since(olLastCall); elapsed < openLibraryMinInterval {
		time.Sleep(openLibraryMinInterval - elapsed)
	}
	olLastCall = time.Now()
}

func openLibraryCoverURL(coverID int) string {
	if coverID <= 0 {
		return ""
	}
	return fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", coverID)
}

func mapOpenLibraryBdType(subjects []string) string {
	blob := strings.ToLower(strings.Join(subjects, " "))
	switch {
	case strings.Contains(blob, "one-shot") || strings.Contains(blob, "oneshot") || strings.Contains(blob, "one shot"):
		return "One-shot"
	case strings.Contains(blob, "intégrale") || strings.Contains(blob, "integrale") || strings.Contains(blob, "omnibus"):
		return "Intégrale"
	case strings.Contains(blob, "série") || strings.Contains(blob, "series") || strings.Contains(blob, "comic strip"):
		return "Série"
	default:
		return "Album"
	}
}

func mapOpenLibraryDoc(doc openLibrarySearchDoc) OpenLibraryBdResult {
	id := strings.TrimSpace(doc.Key)
	title := strings.TrimSpace(doc.Title)
	res := OpenLibraryBdResult{
		ID:      id,
		Title:   title,
		Authors: append([]string(nil), doc.AuthorName...),
		BdType:  mapOpenLibraryBdType(doc.Subject),
		IsAdult: false,
	}
	if doc.CoverI != nil {
		res.ImageURL = openLibraryCoverURL(*doc.CoverI)
	}
	if doc.FirstPublishYear != nil {
		res.FirstPublishYear = *doc.FirstPublishYear
	}
	subj := strings.ToLower(strings.Join(doc.Subject, " "))
	if strings.Contains(subj, "erotic") || strings.Contains(subj, "adult") || strings.Contains(subj, "pornograph") {
		res.IsAdult = true
	}
	return res
}

func searchOpenLibraryRaw(params url.Values, limit int) ([]OpenLibraryBdResult, error) {
	if limit <= 0 {
		limit = 15
	}
	if limit > 40 {
		limit = 40
	}
	params.Set("limit", strconv.Itoa(limit))
	if params.Get("language") == "" {
		params.Set("language", "fre")
	}

	openLibraryThrottle()
	reqURL := olSearchBase + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", openLibraryUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := olHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openlibrary: http %d", resp.StatusCode)
	}
	var parsed openLibrarySearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, openLibraryMaxBodyBytes)).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]OpenLibraryBdResult, 0, len(parsed.Docs))
	for _, doc := range parsed.Docs {
		if strings.TrimSpace(doc.Key) == "" || strings.TrimSpace(doc.Title) == "" {
			continue
		}
		out = append(out, mapOpenLibraryDoc(doc))
	}
	return out, nil
}

// SearchOpenLibraryBD searches Open Library for bande dessinée titles (French bias).
func SearchOpenLibraryBD(q string, limit int) ([]OpenLibraryBdResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("q", q+" (bande dessinée OR comics OR \"graphic novels\")")
	return searchOpenLibraryRaw(params, limit)
}

// BrowseOpenLibraryBD lists popular/recent French BD-ish works via subject search.
func BrowseOpenLibraryBD(page, perPage int) ([]OpenLibraryBdResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 40 {
		perPage = 40
	}
	params := url.Values{}
	params.Set("q", "subject:\"bande dessinée\" OR subject:\"Bandes dessinées\"")
	params.Set("sort", "editions")
	params.Set("offset", strconv.Itoa((page-1)*perPage))
	return searchOpenLibraryRaw(params, perPage)
}

// GetOpenLibraryBDByID looks up a work by Open Library key (e.g. /works/OL123W).
func GetOpenLibraryBDByID(workKey string) (*OpenLibraryBdResult, error) {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return nil, fmt.Errorf("openlibrary: empty key")
	}
	if !strings.HasPrefix(workKey, "/") {
		workKey = "/works/" + workKey
	}
	params := url.Values{}
	params.Set("q", "key:"+workKey)
	results, err := searchOpenLibraryRaw(params, 5)
	if err != nil {
		return nil, err
	}
	for i := range results {
		if results[i].ID == workKey || strings.HasSuffix(results[i].ID, workKey) {
			return &results[i], nil
		}
	}
	if len(results) > 0 {
		return &results[0], nil
	}
	return nil, fmt.Errorf("openlibrary: not found")
}
