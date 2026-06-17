package server

import (
	"bookstorage/internal/catalog"
	"bookstorage/internal/database"
	"bookstorage/internal/i18n"
	"bookstorage/internal/recommend"
	"bookstorage/internal/translate"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// HandleRecommendations returns personalized AniList-based suggestions (JSON).
func (a *App) HandleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	dismissedIDs := map[int]struct{}{}
	if dismissed, err := loadDismissedRecommendations(a.DB, userID, "anilist"); err == nil {
		for idStr := range dismissed {
			if id, err := strconv.Atoi(strings.TrimSpace(idStr)); err == nil && id > 0 {
				dismissedIDs[id] = struct{}{}
			}
		}
	} else {
		log.Printf("dismissed recommendations: %v", err)
	}

	cfg := recommend.DefaultForUserConfig()
	cfg.DismissedIDs = dismissedIDs
	res, err := recommend.ForUser(a.DB, int64(userID), cfg)
	if err != nil {
		writeAnilistUpstreamJSON(w, "recommendations", err, map[string]any{"results": []any{}, "profile": map[string]any{}})
		return
	}
	if res == nil {
		res = &recommend.ForUserResult{
			Results: []recommend.Suggestion{},
			Profile: recommend.ProfileSummary{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// HandleRecommendationMedia returns synopsis and metadata for one AniList id (JSON).
func (a *App) HandleRecommendationMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.currentUserID(r); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	aid := strings.TrimSpace(r.URL.Query().Get("anilist_id"))
	id, err := strconv.Atoi(aid)
	if err != nil || id <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_id"})
		return
	}
	d, err := catalog.GetMediaByID(id)
	if err != nil {
		writeAnilistUpstreamJSON(w, "recommendation media", err, nil)
		return
	}
	if d == nil || d.Title == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not_found"})
		return
	}
	tags := make([]map[string]any, 0, len(d.Tags))
	for _, t := range d.Tags {
		tags = append(tags, map[string]any{"name": t.Name, "rank": t.Rank})
	}

	desc := d.Description
	descTranslated := false
	if a.currentLang(r) == i18n.LangFR && a.Settings.TranslateURL != "" && desc != "" {
		fr, ok, err := translate.CachedToFrench(a.DB, a.Settings, desc)
		if err != nil {
			log.Printf("translation: %v", err)
		} else if ok {
			desc = fr
			descTranslated = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"anilist_id":             d.ID,
		"title":                  d.Title,
		"description":            desc,
		"description_translated": descTranslated,
		"genres":                 d.Genres,
		"tags":                   tags,
		"format":                 d.RawMedia.Format,
		"type":                   d.RawMedia.Type,
		"average_score":          d.AverageScore,
		"mean_score":             d.MeanScore,
		"image_url":              d.ImageURL,
		"reading_type":           catalog.ReadingTypeFromAnilistDetail(d),
		"is_adult":               d.RawMedia.IsAdult,
	})
}

// ensureCatalogID returns the catalog row id for (source, external_id), creating the row if needed.
// Uses RETURNING on PostgreSQL because lib/pq does not support sql.Result.LastInsertId.
func (a *App) ensureCatalogID(source, externalID, title, readingType, imgURL string) (int64, error) {
	source = strings.TrimSpace(source)
	externalID = strings.TrimSpace(externalID)
	if source == "" {
		source = "manual"
	}
	if externalID != "" {
		var existingID int64
		err := a.DB.QueryRow(
			`SELECT id FROM catalog WHERE source = ? AND external_id = ? LIMIT 1`,
			source, externalID,
		).Scan(&existingID)
		if err == nil {
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	var id int64
	if a.DB.B == database.BackendPostgres {
		if externalID != "" {
			err := a.DB.QueryRow(
				`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, ?, ?) RETURNING id`,
				title, readingType, imgURL, source, externalID,
			).Scan(&id)
			return id, err
		}
		err := a.DB.QueryRow(
			`INSERT INTO catalog (title, reading_type, image_url, source) VALUES (?, ?, ?, ?) RETURNING id`,
			title, readingType, imgURL, source,
		).Scan(&id)
		return id, err
	}

	var res sql.Result
	var err error
	if externalID != "" {
		res, err = a.DB.Exec(
			`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, ?, ?)`,
			title, readingType, imgURL, source, externalID,
		)
	} else {
		res, err = a.DB.Exec(
			`INSERT INTO catalog (title, reading_type, image_url, source) VALUES (?, ?, ?, ?)`,
			title, readingType, imgURL, source,
		)
	}
	if err != nil {
		return 0, err
	}
	id, err = res.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("catalog insert: invalid id")
	}
	return id, nil
}
