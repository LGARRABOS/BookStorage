package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
	"bookstorage/internal/recommend"
)

type catalogBrowseItem struct {
	Source      string   `json:"source"`
	ExternalID  string   `json:"external_id"`
	AnilistID   int      `json:"anilist_id,omitempty"`
	Title       string   `json:"title"`
	ReadingType string   `json:"reading_type"`
	ImageURL    string   `json:"image_url,omitempty"`
	IsAdult     bool     `json:"is_adult"`
	Genres      []string `json:"genres,omitempty"`
}

// Types returned by mapAnilistReadingType for AniList MANGA browse results.
var catalogBrowseReadingTypes = []string{
	"Manga",
	"Webtoon",
	"Light Novel",
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

func parseCatalogSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mangadex":
		return "mangadex"
	default:
		return "anilist"
	}
}

func hitToBrowseItem(h catalog.CatalogMediaHit) catalogBrowseItem {
	item := catalogBrowseItem{
		Source:      h.Source,
		ExternalID:  h.ExternalID,
		Title:       h.Title,
		ReadingType: h.ReadingType,
		ImageURL:    h.ImageURL,
		IsAdult:     h.IsAdult,
		Genres:      h.Genres,
	}
	if h.Source == "anilist" {
		if id, err := strconv.Atoi(strings.TrimSpace(h.ExternalID)); err == nil && id > 0 {
			item.AnilistID = id
		}
	}
	return item
}

func (a *App) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data := map[string]any{
		"Genres":        catalog.AnilistGenres(),
		"ReadingTypes":  catalogBrowseReadingTypes,
		"CatalogSource": parseCatalogSource(r.URL.Query().Get("source")),
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

	source := parseCatalogSource(r.URL.Query().Get("source"))

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

	adultOrient := catalog.FilterValidAdultOrientations(r.URL.Query()["adult_orient"])
	if !adultOnly {
		adultOrient = nil
	}
	orientFilter := catalog.ResolveAdultOrientationFilter(adultOrient)
	blocklist, _ := catalog.LoadUserBlocklist(a.DB, int64(userID))
	filter := catalog.MergeBlocklistFilter(blocklist, orientFilter)

	const (
		displayPerPage = 20
		fetchPerPage   = 25
	)

	var (
		hits    []catalog.CatalogMediaHit
		hasNext bool
		err     error
	)

	switch source {
	case "mangadex":
		knownMD := loadKnownCatalogExternalIDs(a.DB, int64(userID), "mangadex")
		hits, hasNext, err = catalog.BrowseMangaDexCollect(catalog.BrowseMangaDexParams{
			GenreIn:        genres,
			PerPage:        fetchPerPage,
			Sort:           sort,
			NotInIDs:       knownMD,
			IsAdult:        &isAdult,
			ReadingTypesIn: readingTypes,
			TagNotIn:       filter.TagNotIn,
			MediaMatch:     filter.MatchMedia,
		}, (page-1)*displayPerPage, displayPerPage)
	default:
		known := map[int]struct{}{}
		if works, werr := recommend.LoadUserAnilistWorks(a.DB, int64(userID)); werr == nil {
			known = recommend.CollectKnownAnilistIDs(works)
		}
		var anilist []catalog.AnilistResult
		anilist, hasNext, err = catalog.BrowseMediaCollect(catalog.BrowseMediaParams{
			GenreIn:        genres,
			PerPage:        fetchPerPage,
			Sort:           sort,
			NotInIDs:       known,
			IsAdult:        &isAdult,
			ReadingTypesIn: readingTypes,
			TagIn:          filter.TagIn,
			TagNotIn:       filter.TagNotIn,
			MediaMatch:     filter.MatchMedia,
		}, (page-1)*displayPerPage, displayPerPage)
		if err == nil {
			hits = anilistResultsToHits(anilist, "anilist")
		}
	}

	if err != nil {
		writeAnilistUpstreamJSON(w, "catalog browse ("+source+")", err, map[string]any{"results": []any{}})
		return
	}

	a.cacheCatalogHits(hits)

	out := make([]catalogBrowseItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitToBrowseItem(h))
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
		"adult_orient":  adultOrient,
		"sort":          sort,
		"source":        source,
	})
}

func anilistResultsToHits(items []catalog.AnilistResult, source string) []catalog.CatalogMediaHit {
	out := make([]catalog.CatalogMediaHit, 0, len(items))
	for _, r := range items {
		out = append(out, catalog.CatalogMediaHit{
			Source:      source,
			ExternalID:  strconv.Itoa(r.ID),
			Title:       r.Title,
			ReadingType: r.ReadingType,
			ImageURL:    r.ImageURL,
			Genres:      r.Genres,
			Tags:        r.Tags,
			IsAdult:     r.IsAdult,
		})
	}
	return out
}

// Add a work (with basic image upload support)
type catalogSearchResult struct {
	Source      string `json:"source"`
	CatalogID   int64  `json:"catalog_id,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	Title       string `json:"title"`
	ReadingType string `json:"reading_type"`
	ImageURL    string `json:"image_url,omitempty"`
	IsAdult     bool   `json:"is_adult"`
}

func (a *App) HandleCatalogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []catalogSearchResult{}})
		return
	}

	source := parseCatalogSource(r.URL.Query().Get("source"))
	blocklist, _ := catalog.LoadUserBlocklist(a.DB, int64(userID))
	filter := catalog.MergeBlocklistFilter(blocklist, catalog.AdultOrientationFilter{})

	var results []catalogSearchResult
	var anilistSearchErr error

	if database.CatalogFTSEnabled(a.DB) {
		matchExpr := ""
		if a.DB.B != database.BackendPostgres {
			matchExpr, _ = fts5MatchExpression(q)
		}
		rows, err := database.SearchCatalogFTS(a.DB, q, matchExpr, 15)
		if err == nil {
			for _, row := range rows {
				results = append(results, catalogSearchResult{
					Source:      row.Source,
					CatalogID:   row.ID,
					ExternalID:  row.ExternalID,
					Title:       row.Title,
					ReadingType: row.ReadingType,
					ImageURL:    row.ImageURL,
				})
			}
		}
	}
	if len(results) < 15 {
		pattern := "%" + strings.ToLower(q) + "%"
		rows, err := a.DB.Query(
			`SELECT id, source, COALESCE(external_id,''), title, reading_type, COALESCE(image_url, '') FROM catalog WHERE LOWER(title) LIKE ? ORDER BY title LIMIT ?`,
			pattern, 15-len(results),
		)
		if err == nil {
			defer func() { _ = rows.Close() }()
			seen := make(map[int64]struct{}, len(results))
			for _, r := range results {
				seen[r.CatalogID] = struct{}{}
			}
			for rows.Next() {
				var id int64
				var src, ext, title, readingType, imageURL string
				if err := rows.Scan(&id, &src, &ext, &title, &readingType, &imageURL); err != nil {
					continue
				}
				if _, dup := seen[id]; dup {
					continue
				}
				results = append(results, catalogSearchResult{
					Source:      src,
					CatalogID:   id,
					ExternalID:  ext,
					Title:       title,
					ReadingType: readingType,
					ImageURL:    imageURL,
				})
				if len(results) >= 15 {
					break
				}
			}
		}
	}

	if len(results) < 15 {
		remaining := 15 - len(results)
		switch source {
		case "mangadex":
			mdHits, err := catalog.SearchMangaDex(q, remaining)
			if err == nil {
				for _, h := range mdHits {
					if filter.MatchMedia != nil && !filter.MatchMedia(h.Genres, h.Tags) {
						continue
					}
					results = append(results, catalogSearchResult{
						Source:      h.Source,
						ExternalID:  h.ExternalID,
						Title:       h.Title,
						ReadingType: h.ReadingType,
						ImageURL:    h.ImageURL,
						IsAdult:     h.IsAdult,
					})
					if len(results) >= 15 {
						break
					}
				}
				a.cacheCatalogHits(mdHits)
			}
		default:
			anilistResults, err := catalog.SearchAnilist(q, remaining)
			if err != nil {
				if anilistSearchErr == nil {
					anilistSearchErr = err
				}
			} else {
				for _, m := range anilistResults {
					if filter.MatchMedia != nil && !filter.MatchMedia(m.Genres, m.Tags) {
						continue
					}
					results = append(results, catalogSearchResult{
						Source:      "anilist",
						ExternalID:  strconv.Itoa(m.ID),
						Title:       m.Title,
						ReadingType: m.ReadingType,
						ImageURL:    m.ImageURL,
						IsAdult:     m.IsAdult,
					})
					if len(results) >= 15 {
						break
					}
				}
				a.cacheCatalogHits(anilistResultsToHits(anilistResults, "anilist"))
			}
		}
	}

	resp := map[string]any{"results": results, "source": source}
	if anilistSearchErr != nil {
		resp["anilist_error"] = catalog.AnilistErrorCode(anilistSearchErr)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
