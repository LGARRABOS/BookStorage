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

func (a *App) HandleBdAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID, _ := a.currentUserID(r)
		data := map[string]any{
			"BdTypes":       bdTypes,
			"Statuses":      bdStatuses,
			"DefaultStatus": "À lire",
		}
		if oid := strings.TrimSpace(r.URL.Query().Get("openlibrary_id")); oid != "" {
			if existingID, found := a.findBdInLibrary(userID, "", "openlibrary", oid); found {
				http.Redirect(w, r, pathBdEditPrefix+strconv.Itoa(existingID)+"?error=exists", http.StatusFound)
				return
			}
			if d, err := catalog.GetOpenLibraryBDByID(oid); err == nil && d != nil && d.Title != "" {
				if existingID, found := a.findBdInLibrary(userID, d.Title, "openlibrary", oid); found {
					http.Redirect(w, r, pathBdEditPrefix+strconv.Itoa(existingID)+"?error=exists", http.StatusFound)
					return
				}
				data["PrefillOpenLibraryID"] = oid
				data["PrefillCatalogSource"] = "openlibrary"
				data["PrefillCatalogExternalID"] = oid
				data["PrefillTitle"] = d.Title
				data["PrefillImageURL"] = d.ImageURL
				data["PrefillBdType"] = d.BdType
				data["PrefillIsAdult"] = d.IsAdult
				if len(d.Authors) > 0 {
					data["PrefillNotes"] = strings.Join(d.Authors, ", ")
				}
			}
		}
		if r.URL.Query().Get("error") == "exists" {
			data["ErrorExists"] = true
		}
		lang := a.currentLang(r)
		data["MobileTopbarTitle"] = i18n.T(lang)["bd.add.title"]
		a.renderTemplate(w, r, "add_bd", a.mergeData(r, data))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID, _ := a.currentUserID(r)
		title := sanitizeTitle(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, pathBdAddWork, http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeBdStatusForWrite(r.FormValue("status"))
		bdType := normalizeBdTypeForWrite(r.FormValue("bd_type"))
		tome := clampChapter(atoiDefault(r.FormValue("tome"), 0))
		totalTomes := parseNullableEpisodes(r.FormValue("total_tomes"))
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
		externalID := strings.TrimSpace(r.FormValue("catalog_external_id"))
		var externalIDArg any
		if externalID != "" {
			externalIDArg = externalID
		}

		if existingID, found := a.findBdInLibrary(userID, title, source, externalID); found {
			http.Redirect(w, r, pathBdEditPrefix+strconv.Itoa(existingID)+"?error=exists", http.StatusFound)
			return
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
			`INSERT INTO bd_works (title, tome, total_tomes, status, bd_type, link, image_path, rating, notes, is_adult, source, external_id, user_id, updated_at, started_at, finished_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
			title, tome, totalTomes, status, bdType, link, imgArg, rating, notes, isAdult, source, externalIDArg, userID, startedAtArg, finishedAtArg,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, pathBdDashboard, http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleBdEditWork(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var work bdWorkRow
	err := scanBdRow(&work, a.DB.QueryRow(
		`SELECT `+sqlBdRowFull+`
         FROM bd_works WHERE id = ? AND user_id = ?`,
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
		catalogPageURL = bdSourcePageURL(work.Source.String, work.ExternalID.String)
	}

	switch r.Method {
	case http.MethodGet:
		lang := a.currentLang(r)
		a.renderTemplate(w, r, "edit_bd", a.mergeData(r, map[string]any{
			"Work":              work,
			"BdTypes":           bdTypes,
			"Statuses":          bdStatuses,
			"CatalogPageURL":    catalogPageURL,
			"ErrorExists":       r.URL.Query().Get("error") == "exists",
			"MobileTopbarTitle": i18n.T(lang)["bd.edit.title"],
		}))
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		title := sanitizeTitle(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, pathBdEditPrefix+strconv.Itoa(workID), http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := normalizeBdStatusForWrite(r.FormValue("status"))
		bdType := normalizeBdTypeForWrite(r.FormValue("bd_type"))
		tome := clampChapter(atoiDefault(r.FormValue("tome"), 0))
		totalTomes := parseNullableEpisodes(r.FormValue("total_tomes"))
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
			`UPDATE bd_works SET title = ?, tome = ?, total_tomes = ?, status = ?, bd_type = ?, link = ?, image_path = ?, rating = ?, notes = ?, is_adult = ?, started_at = ?, finished_at = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ? AND user_id = ?`,
			title, tome, totalTomes, status, bdType, link, imgArg, rating, notes, isAdult, startedAtArg, finishedAtArg, workID, userID,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, pathBdDashboard, http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleBdDeleteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(`DELETE FROM bd_works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
