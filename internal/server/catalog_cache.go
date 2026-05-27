package server

import (
	"encoding/json"
	"strings"

	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
)

func encodeJSONStrings(items []string) string {
	if len(items) == 0 {
		return ""
	}
	b, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(b)
}

func (a *App) upsertCatalogMetadata(hit catalog.CatalogMediaHit, synopsis string, altTitles []string) {
	if a.DB == nil {
		return
	}
	source := strings.TrimSpace(hit.Source)
	ext := strings.TrimSpace(hit.ExternalID)
	if source == "" || ext == "" {
		return
	}
	genresJSON := encodeJSONStrings(hit.Genres)
	tagsJSON := encodeJSONStrings(hit.Tags)
	altJSON := encodeJSONStrings(altTitles)
	synopsis = strings.TrimSpace(synopsis)
	_, _ = a.DB.Exec(
		`UPDATE catalog SET genres = ?, tags = ?, synopsis = COALESCE(NULLIF(?, ''), synopsis),
		 alt_titles = COALESCE(NULLIF(?, ''), alt_titles), fetched_at = CURRENT_TIMESTAMP
		 WHERE source = ? AND external_id = ?`,
		genresJSON, tagsJSON, synopsis, altJSON, source, ext,
	)
}

func (a *App) cacheCatalogHits(hits []catalog.CatalogMediaHit) {
	for _, h := range hits {
		a.upsertCatalogMetadata(h, "", nil)
		if h.Source != "" && h.ExternalID != "" {
			_, _ = a.ensureCatalogID(h.Source, h.ExternalID, h.Title, h.ReadingType, h.ImageURL)
		}
	}
}

func loadKnownCatalogExternalIDs(db *database.Conn, userID int64, source string) map[string]struct{} {
	m := make(map[string]struct{})
	if db == nil || userID <= 0 {
		return m
	}
	source = strings.ToLower(strings.TrimSpace(source))
	rows, err := db.Query(
		`SELECT c.external_id FROM works w
		 INNER JOIN catalog c ON c.id = w.catalog_id
		 WHERE w.user_id = ? AND LOWER(TRIM(c.source)) = ? AND TRIM(c.external_id) != ''`,
		userID, source,
	)
	if err != nil {
		return m
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ext string
		if rows.Scan(&ext) == nil {
			ext = strings.TrimSpace(ext)
			if ext != "" {
				m[ext] = struct{}{}
			}
		}
	}
	return m
}
