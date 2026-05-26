package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/recommend"
)

type catalogBrowseItem struct {
	AnilistID   int      `json:"anilist_id"`
	Title       string   `json:"title"`
	ReadingType string   `json:"reading_type"`
	ImageURL    string   `json:"image_url,omitempty"`
	IsAdult     bool     `json:"is_adult"`
	Genres      []string `json:"genres,omitempty"`
}

// Types returned by mapAnilistReadingType for AniList MANGA browse results.
var catalogBrowseReadingTypes = []string{
	"Manga",
	"Manhwa",
	"Webtoon",
	"Light Novel",
	"Autre",
}

func filterValidCatalogReadingTypes(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(catalogBrowseReadingTypes))
	for _, t := range catalogBrowseReadingTypes {
		allowed[t] = struct{}{}
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := allowed[t]; !ok {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (a *App) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data := map[string]any{
		"Genres":       catalog.AnilistGenres(),
		"ReadingTypes": catalogBrowseReadingTypes,
	}
	a.renderTemplate(w, r, "catalog", a.mergeData(r, data))
}

func (a *App) HandleCatalogBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	rawGenres := r.URL.Query()["genre"]
	genres := catalog.FilterValidAnilistGenres(rawGenres, 3)
	if len(rawGenres) > 0 && len(genres) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_genre", "results": []any{}})
		return
	}

	page := 1
	if p := strings.TrimSpace(r.URL.Query().Get("page")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort != "SCORE_DESC" {
		sort = "POPULARITY_DESC"
	}

	readingTypes := filterValidCatalogReadingTypes(r.URL.Query()["reading_type"])

	adultOnly := strings.TrimSpace(r.URL.Query().Get("adult")) == "only"
	isAdult := adultOnly

	const (
		displayPerPage = 20
		fetchPerPage   = 25
	)
	known := map[int]struct{}{}
	if works, err := recommend.LoadUserAnilistWorks(a.DB, int64(userID)); err == nil {
		known = recommend.CollectKnownAnilistIDs(works)
	}

	results, hasNext, err := catalog.BrowseMediaCollect(catalog.BrowseMediaParams{
		GenreIn:        genres,
		PerPage:        fetchPerPage,
		Sort:           sort,
		NotInIDs:       known,
		IsAdult:        &isAdult,
		ReadingTypesIn: readingTypes,
	}, (page-1)*displayPerPage, displayPerPage)
	if err != nil {
		log.Printf("catalog browse: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "upstream", "results": []any{}})
		return
	}

	out := make([]catalogBrowseItem, 0, len(results))
	for _, r := range results {
		out = append(out, catalogBrowseItem{
			AnilistID:   r.ID,
			Title:       r.Title,
			ReadingType: r.ReadingType,
			ImageURL:    r.ImageURL,
			IsAdult:     r.IsAdult,
			Genres:      r.Genres,
		})
	}

	adultFilter := ""
	if adultOnly {
		adultFilter = "only"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":       out,
		"page":          page,
		"has_next":      hasNext,
		"genres":        genres,
		"reading_types": readingTypes,
		"adult":         adultFilter,
		"sort":          sort,
	})
}
