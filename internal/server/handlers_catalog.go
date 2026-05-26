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

func (a *App) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data := map[string]any{
		"Genres": catalog.AnilistGenres(),
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

	genres := catalog.FilterValidAnilistGenres(r.URL.Query()["genre"], 3)
	if len(genres) == 0 {
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

	const perPage = 20
	known := map[int]struct{}{}
	if works, err := recommend.LoadUserAnilistWorks(a.DB, int64(userID)); err == nil {
		known = recommend.CollectKnownAnilistIDs(works)
	}

	results, err := catalog.BrowseMedia(catalog.BrowseMediaParams{
		GenreIn:    genres,
		Page:       page,
		PerPage:    perPage,
		Sort:       sort,
		NotInIDs:   known,
		MaxResults: perPage,
	})
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":  out,
		"page":     page,
		"has_next": len(out) >= perPage,
		"genres":   genres,
		"sort":     sort,
	})
}
