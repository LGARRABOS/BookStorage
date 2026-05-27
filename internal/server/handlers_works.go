package server

import (
	"bookstorage/internal/catalog"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) HandleAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data := map[string]any{
			"ReadingTypes":  readingTypes,
			"Statuses":      readingStatuses,
			"DefaultStatus": "À lire",
		}
		if aid := strings.TrimSpace(r.URL.Query().Get("anilist_id")); aid != "" {
			if id, err := strconv.Atoi(aid); err == nil && id > 0 {
				if d, err := catalog.GetMediaByID(id); err == nil && d != nil && d.Title != "" {
					data["PrefillAnilistID"] = id
					data["PrefillTitle"] = d.Title
					data["PrefillImageURL"] = d.ImageURL
					data["PrefillReadingType"] = catalog.ReadingTypeFromAnilistDetail(d)
					data["PrefillIsAdult"] = d.RawMedia.IsAdult
				}
			}
		}
		a.renderTemplate(w, r, "add_work", a.mergeData(r, data))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID, _ := a.currentUserID(r)
		title := sanitizeTitle(r.FormValue("title"))
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeStatusForWrite(r.FormValue("status"))
		chapterStr := r.FormValue("chapter")
		if chapterStr == "" {
			chapterStr = "0"
		}
		chapter, _ := strconv.Atoi(chapterStr)
		chapter = clampChapter(chapter)
		readingType := normalizeReadingTypeForWrite(r.FormValue("reading_type"))
		ratingStr := r.FormValue("rating")
		rating, _ := strconv.Atoi(ratingStr)
		rating = clampRating(rating)
		notes := strings.TrimSpace(r.FormValue("notes"))
		isAdult := 0
		if r.FormValue("is_adult") == "1" || strings.ToLower(r.FormValue("is_adult")) == "on" {
			isAdult = 1
		}
		notifyCh := notifyNewChaptersFromForm(status, r)

		var catalogID sql.NullInt64
		if cidStr := r.FormValue("catalog_id"); cidStr != "" {
			if cid, _ := strconv.ParseInt(cidStr, 10, 64); cid > 0 {
				catalogID.Int64 = cid
				catalogID.Valid = true
			}
		}
		if !catalogID.Valid {
			source := strings.TrimSpace(r.FormValue("catalog_source"))
			externalID := strings.TrimSpace(r.FormValue("catalog_external_id"))
			imgURL := strings.TrimSpace(r.FormValue("image_url"))
			if source == "" {
				source = "manual"
			}
			if id, err := a.ensureCatalogID(source, externalID, title, readingType, imgURL); err == nil && id > 0 {
				catalogID.Int64 = id
				catalogID.Valid = true
			}
		}

		var imagePath sql.NullString
		imageURL := strings.TrimSpace(r.FormValue("image_url"))
		if imageURL != "" && (strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
			imagePath.String = imageURL
			imagePath.Valid = true
		}

		// If no URL, check for file upload
		if !imagePath.Valid {
			if rel, err := saveImageFromForm(r, "image", a.Settings.UploadFolder, a.Settings.UploadURLPath, userID); err == nil {
				imagePath.String = rel
				imagePath.Valid = true
			}
		}

		var readingSiteID sql.NullInt64
		if siteID, ok := a.MatchReadingSite(userID, link); ok {
			readingSiteID.Int64 = siteID
			readingSiteID.Valid = true
		}

		var startedAtArg, finishedAtArg any
		if status == "En cours" {
			startedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if status == "Terminé" {
			finishedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}

		var dbErr error
		if imagePath.Valid {
			_, dbErr = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, is_adult, notes, user_id, catalog_id, notify_new_chapters, reading_site_id, updated_at, started_at, finished_at)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
				title, chapter, link, status, imagePath.String, readingType, rating, isAdult, notes, userID, catalogID, notifyCh, readingSiteID, startedAtArg, finishedAtArg,
			)
		} else {
			_, dbErr = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, is_adult, notes, user_id, catalog_id, notify_new_chapters, reading_site_id, updated_at, started_at, finished_at)
                 VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
				title, chapter, link, status, readingType, rating, isAdult, notes, userID, catalogID, notifyCh, readingSiteID, startedAtArg, finishedAtArg,
			)
		}
		if dbErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleEditWork(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var work workRow
	err := scanFullWorkRow(&work, a.DB.QueryRow(
		`SELECT `+sqlWorkRowFull+`
         FROM works WHERE id = ? AND user_id = ?`,
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

	catalogPageURL := a.catalogPageURLForUserWork(userID, workID)
	catalogAnilistImageURL, catalogAnilistImageLocked := a.catalogAnilistCoverForUserWork(userID, workID)

	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("format") == "partial" {
			a.renderTemplate(w, r, "edit_work_modal", a.mergeData(r, map[string]any{
				"Work":                      work,
				"ReadingTypes":              readingTypes,
				"Statuses":                  readingStatuses,
				"IsModal":                   true,
				"CatalogPageURL":            catalogPageURL,
				"CatalogAnilistImageURL":    catalogAnilistImageURL,
				"CatalogAnilistImageLocked": catalogAnilistImageLocked,
			}))
			return
		}
		a.renderTemplate(w, r, "edit_work", a.mergeData(r, map[string]any{
			"Work":                      work,
			"ReadingTypes":              readingTypes,
			"Statuses":                  readingStatuses,
			"CatalogPageURL":            catalogPageURL,
			"CatalogAnilistImageURL":    catalogAnilistImageURL,
			"CatalogAnilistImageLocked": catalogAnilistImageLocked,
		}))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		title := sanitizeTitle(r.FormValue("title"))
		if title == "" {
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "title_required"})
				return
			}
			http.Redirect(w, r, "/edit/"+strconv.Itoa(workID), http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeStatusForWrite(r.FormValue("status"))
		chapterStr := r.FormValue("chapter")
		if chapterStr == "" {
			chapterStr = "0"
		}
		chapter, _ := strconv.Atoi(chapterStr)
		chapter = clampChapter(chapter)
		readingType := normalizeReadingTypeForWrite(r.FormValue("reading_type"))
		ratingStr := r.FormValue("rating")
		rating, _ := strconv.Atoi(ratingStr)
		rating = clampRating(rating)
		notes := strings.TrimSpace(r.FormValue("notes"))
		isAdult := 0
		if r.FormValue("is_adult") == "1" || strings.ToLower(r.FormValue("is_adult")) == "on" {
			isAdult = 1
		}
		notifyCh := notifyNewChaptersFromForm(status, r)

		parentStr := strings.TrimSpace(r.FormValue("parent_work_id"))
		var parentArg any
		if parentStr != "" {
			pid, err := strconv.Atoi(parentStr)
			if err != nil || pid <= 0 {
				if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_parent"})
					return
				}
				http.Redirect(w, r, "/edit/"+strconv.Itoa(workID), http.StatusFound)
				return
			}
			if err := a.validateWorkParent(userID, workID, pid); err != nil {
				if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_parent"})
					return
				}
				http.Redirect(w, r, "/edit/"+strconv.Itoa(workID), http.StatusFound)
				return
			}
			parentArg = pid
		} else {
			parentArg = nil
		}

		seriesSort := work.SeriesSort
		if sortStr := strings.TrimSpace(r.FormValue("series_sort")); sortStr != "" {
			if s, err := strconv.Atoi(sortStr); err == nil {
				if s < 0 {
					s = 0
				}
				seriesSort = s
			}
		}

		// Gestion de l'image (optionnel)
		newImagePath := work.ImagePath

		catalogAnilistURL, aniListImageLock := a.catalogAnilistCoverForUserWork(userID, workID)

		// Check for image URL first
		imageURL := strings.TrimSpace(r.FormValue("image_url"))
		if aniListImageLock {
			imageURL = ""
		}
		if imageURL != "" && (strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
			newImagePath.String = imageURL
			newImagePath.Valid = true
		} else if !aniListImageLock {
			if rel, err := saveImageFromForm(r, "image", a.Settings.UploadFolder, a.Settings.UploadURLPath, userID); err == nil {
				newImagePath.String = rel
				newImagePath.Valid = true
			}
		}
		// Remplace en base une ancienne couverture custom par l’URL canonique AniList (affichage + export cohérents).
		if aniListImageLock && catalogAnilistURL != "" {
			newImagePath.String = catalogAnilistURL
			newImagePath.Valid = true
		}

		var readingSiteArg any
		if siteID, ok := a.MatchReadingSite(userID, link); ok {
			readingSiteArg = siteID
		}

		formStartedAt := strings.TrimSpace(r.FormValue("started_at"))
		formLastChapterAt := strings.TrimSpace(r.FormValue("last_chapter_at"))
		formFinishedAt := strings.TrimSpace(r.FormValue("finished_at"))

		var startedAtArg, lastChapterAtArg, finishedAtArg any
		if formStartedAt != "" {
			startedAtArg = formStartedAt
		} else if work.StartedAt.Valid {
			startedAtArg = work.StartedAt.String
		}
		if formLastChapterAt != "" {
			lastChapterAtArg = formLastChapterAt
		} else if work.LastChapterAt.Valid {
			lastChapterAtArg = work.LastChapterAt.String
		}
		if formFinishedAt != "" {
			finishedAtArg = formFinishedAt
		} else if work.FinishedAt.Valid {
			finishedAtArg = work.FinishedAt.String
		}

		oldStatus := ""
		if work.Status.Valid {
			oldStatus = work.Status.String
		}
		if status == "En cours" && oldStatus != "En cours" && startedAtArg == nil {
			startedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if status == "Terminé" && oldStatus != "Terminé" && finishedAtArg == nil {
			finishedAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if chapter > work.Chapter && formLastChapterAt == "" {
			lastChapterAtArg = time.Now().UTC().Format("2006-01-02 15:04:05")
		}

		if newImagePath.Valid {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, image_path = ?, reading_type = ?, rating = ?, is_adult = ?, notes = ?, parent_work_id = ?, series_sort = ?, notify_new_chapters = ?, reading_site_id = ?, started_at = ?, last_chapter_at = ?, finished_at = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, newImagePath.String, readingType, rating, isAdult, notes, parentArg, seriesSort, notifyCh, readingSiteArg, startedAtArg, lastChapterAtArg, finishedAtArg, workID, userID,
			)
		} else {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, reading_type = ?, rating = ?, is_adult = ?, notes = ?, parent_work_id = ?, series_sort = ?, notify_new_chapters = ?, reading_site_id = ?, started_at = ?, last_chapter_at = ?, finished_at = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, readingType, rating, isAdult, notes, parentArg, seriesSort, notifyCh, readingSiteArg, startedAtArg, lastChapterAtArg, finishedAtArg, workID, userID,
			)
		}
		if err != nil {
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "update_failed"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		a.applyChapterDeltaToReadingStats(userID, chapter-work.Chapter, work.LastChapterAt)
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleDeleteWorkAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *App) HandleIncrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	res, err := a.DB.Exec(
		`UPDATE works SET chapter = chapter + 1, last_chapter_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		a.recordReadingChapterIncrements(userID, 1)
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleDecrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE works
         SET chapter = CASE WHEN chapter > 0 THEN chapter - 1 ELSE 0 END, updated_at = CURRENT_TIMESTAMP
         WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleSetChapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	chapterStr := r.FormValue("chapter")
	if chapterStr == "" {
		chapterStr = "0"
	}
	chapter, err := strconv.Atoi(chapterStr)
	if err != nil {
		chapter = 0
	}
	chapter = clampChapter(chapter)

	var oldChapter int
	var lastAt nullFlexTime
	_ = a.DB.QueryRow(`SELECT chapter, last_chapter_at FROM works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&oldChapter, &lastAt)

	if chapter > oldChapter {
		_, err = a.DB.Exec(
			`UPDATE works SET chapter = ?, last_chapter_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			chapter, workID, userID,
		)
	} else {
		_, err = a.DB.Exec(
			`UPDATE works SET chapter = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			chapter, workID, userID,
		)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.applyChapterDeltaToReadingStats(userID, chapter-oldChapter, lastAt)
	_, _ = w.Write([]byte("ok"))
}
