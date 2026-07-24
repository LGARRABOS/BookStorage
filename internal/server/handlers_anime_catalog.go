package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/i18n"
)

type animeCatalogItem struct {
	Source     string   `json:"source"`
	ExternalID string   `json:"external_id"`
	AnilistID  int      `json:"anilist_id,omitempty"`
	Title      string   `json:"title"`
	AnimeType  string   `json:"anime_type"`
	Episodes   int      `json:"episodes,omitempty"`
	SeasonYear int      `json:"season_year,omitempty"`
	ImageURL   string   `json:"image_url,omitempty"`
	IsAdult    bool     `json:"is_adult"`
	Genres     []string `json:"genres,omitempty"`
}

func animeResultToItem(r catalog.AnilistAnimeResult) animeCatalogItem {
	return animeCatalogItem{
		Source:     "anilist",
		ExternalID: strconv.Itoa(r.ID),
		AnilistID:  r.ID,
		Title:      r.Title,
		AnimeType:  r.AnimeType,
		Episodes:   r.Episodes,
		SeasonYear: r.SeasonYear,
		ImageURL:   r.ImageURL,
		IsAdult:    r.IsAdult,
		Genres:     r.Genres,
	}
}

func (a *App) HandleAnimeCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	lang := a.currentLang(r)
	data := map[string]any{
		"Genres":            catalog.AnilistGenres(),
		"MobileTopbarTitle": i18n.T(lang)["anime.catalog.title"],
	}
	a.renderTemplate(w, r, "anime_catalog", a.mergeData(r, data))
}

func (a *App) HandleAnimeCatalogBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.currentUserID(r); !ok {
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
	adultOnly := strings.TrimSpace(r.URL.Query().Get("adult")) == "only"
	isAdult := adultOnly

	const perPage = 20
	results, err := catalog.BrowseAnimeMedia(catalog.BrowseAnimeParams{
		GenreIn: genres,
		Page:    page,
		PerPage: perPage,
		Sort:    sort,
		IsAdult: &isAdult,
	})
	if err != nil {
		writeAnilistUpstreamJSON(w, "anime catalog browse", err, map[string]any{"results": []any{}})
		return
	}

	out := make([]animeCatalogItem, 0, len(results))
	for _, res := range results {
		out = append(out, animeResultToItem(res))
	}

	adultFilter := ""
	if adultOnly {
		adultFilter = "only"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":  out,
		"page":     page,
		"has_next": len(results) >= perPage,
		"genres":   genres,
		"adult":    adultFilter,
		"sort":     sort,
		"source":   "anilist",
	})
}

func (a *App) HandleAnimeCatalogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.currentUserID(r); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []animeCatalogItem{}})
		return
	}

	results, err := catalog.SearchAnilistAnime(q, 15)
	resp := map[string]any{"source": "anilist"}
	if err != nil {
		resp["anilist_error"] = catalog.AnilistErrorCode(err)
		resp["results"] = []animeCatalogItem{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	out := make([]animeCatalogItem, 0, len(results))
	for _, res := range results {
		out = append(out, animeResultToItem(res))
	}
	resp["results"] = out
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
