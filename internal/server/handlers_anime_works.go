package server

import (
	"bookstorage/internal/catalog"
	"bookstorage/internal/i18n"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) HandleAnimeAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data := map[string]any{
			"AnimeTypes":    animeTypes,
			"Statuses":      animeStatuses,
			"DefaultStatus": "À voir",
		}
		if aid := strings.TrimSpace(r.URL.Query().Get("anilist_id")); aid != "" {
			if id, err := strconv.Atoi(aid); err == nil && id > 0 {
				if d, err := catalog.GetAnilistAnimeByID(id); err == nil && d != nil && d.Title != "" {
					data["PrefillAnilistID"] = id
					data["PrefillCatalogSource"] = "anilist"
					data["PrefillCatalogExternalID"] = aid
					data["PrefillTitle"] = d.Title
					data["PrefillImageURL"] = d.ImageURL
					data["PrefillAnimeType"] = d.AnimeType
					data["PrefillTotalEpisodes"] = d.Episodes
					data["PrefillIsAdult"] = d.IsAdult
				}
			}
		}
		lang := a.currentLang(r)
		data["MobileTopbarTitle"] = i18n.T(lang)["anime.add.title"]
		a.renderTemplate(w, r, "add_anime", a.mergeData(r, data))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID, _ := a.currentUserID(r)
		title := sanitizeTitle(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, pathAnimeAddWork, http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeAnimeStatusForWrite(r.FormValue("status"))
		animeType := normalizeAnimeTypeForWrite(r.FormValue("anime_type"))
		episode := clampChapter(atoiDefault(r.FormValue("episode"), 0))
		totalEpisodes := parseNullableEpisodes(r.FormValue("total_episodes"))
		rating := clampRating(atoiDefault(r.FormValue("rating"), 0))
		notes := strings.TrimSpace(r.FormValue("notes"))
		isAdult := 0
		if r.FormValue("is_adult") == "1" || strings.ToLower(r.FormValue("is_adult")) == "on" {
			isAdult = 1
		}
		source := strings.TrimSpace(r.FormValue("catalog_source"))
		if source == "" {
			source = "manual"
		}
		var externalIDArg any
		if ext := strings.TrimSpace(r.FormValue("catalog_external_id")); ext != "" {
			externalIDArg = ext
		}

		var imagePath sql.NullString
		imageURL := strings.TrimSpace(r.FormValue("image_url"))
		if imageURL != "" && (strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
			imagePath.String = imageURL
			imagePath.Valid = true
		}
		if !imagePath.Valid {
			if rel, err := saveImageFromForm(r, "image", a.Settings.UploadFolder, a.Settings.UploadURLPath, userID); err == nil {
				imagePath.String = rel
				imagePath.Valid = true
			}
		}

		var startedAtArg, finishedAtArg any
		if status == "En cours" {
			startedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if status == "Terminé" {
			finishedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}

		var imgArg any
		if imagePath.Valid {
			imgArg = imagePath.String
		}
		_, err := a.DB.Exec(
			`INSERT INTO anime_works (title, episode, total_episodes, status, anime_type, link, image_path, rating, notes, is_adult, source, external_id, user_id, updated_at, started_at, finished_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
			title, episode, totalEpisodes, status, animeType, link, imgArg, rating, notes, isAdult, source, externalIDArg, userID, startedAtArg, finishedAtArg,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, pathAnimeDashboard, http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleAnimeEditWork(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var work animeWorkRow
	err := scanAnimeRow(&work, a.DB.QueryRow(
		`SELECT `+sqlAnimeRowFull+`
         FROM anime_works WHERE id = ? AND user_id = ?`,
		workID, userID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	catalogPageURL := ""
	if work.Source.Valid && work.ExternalID.Valid {
		catalogPageURL = animeSourcePageURL(work.Source.String, work.ExternalID.String)
	}

	switch r.Method {
	case http.MethodGet:
		lang := a.currentLang(r)
		a.renderTemplate(w, r, "edit_anime", a.mergeData(r, map[string]any{
			"Work":              work,
			"AnimeTypes":        animeTypes,
			"Statuses":          animeStatuses,
			"CatalogPageURL":    catalogPageURL,
			"MobileTopbarTitle": i18n.T(lang)["anime.edit.title"],
		}))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		title := sanitizeTitle(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, pathAnimeEditPrefix+strconv.Itoa(workID), http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeAnimeStatusForWrite(r.FormValue("status"))
		animeType := normalizeAnimeTypeForWrite(r.FormValue("anime_type"))
		episode := clampChapter(atoiDefault(r.FormValue("episode"), 0))
		totalEpisodes := parseNullableEpisodes(r.FormValue("total_episodes"))
		rating := clampRating(atoiDefault(r.FormValue("rating"), 0))
		notes := strings.TrimSpace(r.FormValue("notes"))
		isAdult := 0
		if r.FormValue("is_adult") == "1" || strings.ToLower(r.FormValue("is_adult")) == "on" {
			isAdult = 1
		}

		newImagePath := work.ImagePath
		imageURL := strings.TrimSpace(r.FormValue("image_url"))
		if imageURL != "" && (strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
			newImagePath.String = imageURL
			newImagePath.Valid = true
		} else {
			if rel, err := saveImageFromForm(r, "image", a.Settings.UploadFolder, a.Settings.UploadURLPath, userID); err == nil {
				newImagePath.String = rel
				newImagePath.Valid = true
			}
		}

		oldStatus := ""
		if work.Status.Valid {
			oldStatus = work.Status.String
		}
		var startedAtArg, finishedAtArg any
		if work.StartedAt.Valid {
			startedAtArg = work.StartedAt.String
		}
		if work.FinishedAt.Valid {
			finishedAtArg = work.FinishedAt.String
		}
		if status == "En cours" && oldStatus != "En cours" && startedAtArg == nil {
			startedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if status == "Terminé" && oldStatus != "Terminé" && finishedAtArg == nil {
			finishedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}

		var imgArg any
		if newImagePath.Valid {
			imgArg = newImagePath.String
		}
		_, err = a.DB.Exec(
			`UPDATE anime_works SET title = ?, episode = ?, total_episodes = ?, status = ?, anime_type = ?, link = ?, image_path = ?, rating = ?, notes = ?, is_adult = ?, started_at = ?, finished_at = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ?`,
			title, episode, totalEpisodes, status, animeType, link, imgArg, rating, notes, isAdult, startedAtArg, finishedAtArg, workID, userID,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, pathAnimeDashboard, http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleAnimeIncrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE anime_works SET episode = episode + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleAnimeDecrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE anime_works
         SET episode = CASE WHEN episode > 0 THEN episode - 1 ELSE 0 END, updated_at = CURRENT_TIMESTAMP
         WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleAnimeSetEpisode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	episode := clampChapter(atoiDefault(r.FormValue("episode"), 0))
	_, err := a.DB.Exec(
		`UPDATE anime_works SET episode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		episode, workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleAnimeDeleteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(`DELETE FROM anime_works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// parseNullableEpisodes returns a SQL-nullable episode count (NULL when empty/invalid/<=0).
func parseNullableEpisodes(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return nil
	}
	if v > maxChapterValue {
		v = maxChapterValue
	}
	return v
}
