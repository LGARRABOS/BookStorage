package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/i18n"
)

type bdCatalogItem struct {
	Source           string   `json:"source"`
	ExternalID       string   `json:"external_id"`
	Title            string   `json:"title"`
	BdType           string   `json:"bd_type"`
	Authors          []string `json:"authors,omitempty"`
	FirstPublishYear int      `json:"first_publish_year,omitempty"`
	ImageURL         string   `json:"image_url,omitempty"`
	IsAdult          bool     `json:"is_adult"`
	InLibrary        bool     `json:"in_library,omitempty"`
	LibraryID        int      `json:"library_id,omitempty"`
}

func bdResultToItem(r catalog.OpenLibraryBdResult) bdCatalogItem {
	return bdCatalogItem{
		Source:           "openlibrary",
		ExternalID:       r.ID,
		Title:            r.Title,
		BdType:           r.BdType,
		Authors:          r.Authors,
		FirstPublishYear: r.FirstPublishYear,
		ImageURL:         r.ImageURL,
		IsAdult:          r.IsAdult,
	}
}

func markBdCatalogInLibrary(items []bdCatalogItem, owned map[string]int) {
	for i := range items {
		key := strings.ToLower(strings.TrimSpace(items[i].Source)) + ":" + strings.TrimSpace(items[i].ExternalID)
		if id, ok := owned[key]; ok {
			items[i].InLibrary = true
			items[i].LibraryID = id
		}
	}
}

func (a *App) HandleBdCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	lang := a.currentLang(r)
	data := map[string]any{
		"MobileTopbarTitle": i18n.T(lang)["bd.catalog.title"],
	}
	a.renderTemplate(w, r, "bd_catalog", a.mergeData(r, data))
}

func (a *App) HandleBdCatalogBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page := 1
	if p := strings.TrimSpace(r.URL.Query().Get("page")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	const perPage = 20
	results, err := catalog.BrowseOpenLibraryBD(page, perPage)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "upstream", "results": []any{}})
		return
	}

	out := make([]bdCatalogItem, 0, len(results))
	for _, res := range results {
		out = append(out, bdResultToItem(res))
	}
	markBdCatalogInLibrary(out, a.bdLibraryExternalKeys(userID))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":  out,
		"page":     page,
		"has_next": len(results) >= perPage,
		"source":   "openlibrary",
	})
}

func (a *App) HandleBdCatalogSearch(w http.ResponseWriter, r *http.Request) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []bdCatalogItem{}, "source": "openlibrary"})
		return
	}

	results, err := catalog.SearchOpenLibraryBD(q, 15)
	resp := map[string]any{"source": "openlibrary"}
	if err != nil {
		resp["openlibrary_error"] = "upstream"
		resp["results"] = []bdCatalogItem{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	out := make([]bdCatalogItem, 0, len(results))
	for _, res := range results {
		out = append(out, bdResultToItem(res))
	}
	markBdCatalogInLibrary(out, a.bdLibraryExternalKeys(userID))
	resp["results"] = out
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
