package server

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/i18n"
)

func (a *App) HandleLibraryHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	furniture, err := a.listLibraryFurniture(userID)
	if err != nil {
		log.Printf("[library] list furniture user=%d: %v", userID, err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var searchResults []libraryPlacementRow
	if q != "" {
		searchResults, err = a.searchLibraryPlacements(userID, q)
		if err != nil {
			log.Printf("[library] search user=%d: %v", userID, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}
	lang := a.currentLang(r)
	a.renderTemplate(w, r, "library_home", a.mergeData(r, map[string]any{
		// FurnitureList (slice) — do not pass as "Furniture": library_topbar
		// expects a single furniture row with .ID (slice.ID panics → 500).
		"FurnitureList":     furniture,
		"SearchQ":           q,
		"SearchResults":     searchResults,
		"MobileTopbarTitle": i18n.T(lang)["library.title"],
	}))
}

func (a *App) HandleLibraryFurnitureNew(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	lang := a.currentLang(r)
	switch r.Method {
	case http.MethodGet:
		a.renderTemplate(w, r, "library_furniture_edit", a.mergeData(r, map[string]any{
			"IsNew":             true,
			"Furniture":         libraryFurnitureRow{Name: ""},
			"Shelves":           []libraryShelfRow{},
			"MobileTopbarTitle": i18n.T(lang)["library.furniture.new"],
		}))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		room := strings.TrimSpace(r.FormValue("room_label"))
		if name == "" {
			http.Redirect(w, r, pathLibraryFurnitureNew+"?error=name", http.StatusFound)
			return
		}
		res, err := a.insertLibraryRowID(
			`INSERT INTO library_furniture (user_id, name, room_label, sort_order, updated_at)
			 VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)`,
			userID, name, nullIfEmpty(room),
		)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		id := res
		if id <= 0 {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		// Seed shelf A with 10 cases by default.
		_, _ = a.DB.Exec(
			`INSERT INTO library_shelves (furniture_id, label, case_count, books_per_case, sort_order) VALUES (?, 'A', 10, 8, 0)`,
			id,
		)
		http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.FormatInt(id, 10)+"/edit", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleLibraryFurnitureView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	furnitureID, _ := strconv.Atoi(r.PathValue("id"))
	if furnitureID <= 0 {
		http.Redirect(w, r, pathLibraryHome, http.StatusFound)
		return
	}
	furn, err := a.getLibraryFurniture(userID, furnitureID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	shelves, err := a.listLibraryShelves(furnitureID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	places, err := a.listLibraryPlacementsForFurniture(userID, furnitureID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	lang := a.currentLang(r)
	a.renderTemplate(w, r, "library_grid", a.mergeData(r, map[string]any{
		"Furniture":         furn,
		"Shelves":           shelves,
		"Placements":        places,
		"FurnitureID":       furnitureID,
		"MobileTopbarTitle": furn.Name,
		"PageTitle":         i18n.T(lang)["library.grid.title"],
	}))
}

func (a *App) HandleLibraryFurnitureEdit(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	furnitureID, _ := strconv.Atoi(r.PathValue("id"))
	if furnitureID <= 0 {
		http.Redirect(w, r, pathLibraryHome, http.StatusFound)
		return
	}
	furn, err := a.getLibraryFurniture(userID, furnitureID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	lang := a.currentLang(r)

	switch r.Method {
	case http.MethodGet:
		shelves, err := a.listLibraryShelves(furnitureID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		places, err := a.listLibraryPlacementsForFurniture(userID, furnitureID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		placeAPI := make([]libraryPlacementAPI, 0, len(places))
		for _, p := range places {
			placeAPI = append(placeAPI, placementToAPI(p))
		}
		a.renderTemplate(w, r, "library_furniture_edit", a.mergeData(r, map[string]any{
			"IsNew":             false,
			"Furniture":         furn,
			"Shelves":           shelves,
			"PlacementAPI":      placeAPI,
			"Error":             r.URL.Query().Get("error"),
			"MobileTopbarTitle": i18n.T(lang)["library.furniture.edit"],
		}))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		action := strings.TrimSpace(r.FormValue("action"))
		switch action {
		case "save_meta":
			name := strings.TrimSpace(r.FormValue("name"))
			room := strings.TrimSpace(r.FormValue("room_label"))
			if name == "" {
				http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.Itoa(furnitureID)+"/edit?error=name", http.StatusFound)
				return
			}
			_, err = a.DB.Exec(
				`UPDATE library_furniture SET name = ?, room_label = ?, updated_at = CURRENT_TIMESTAMP
				 WHERE id = ? AND user_id = ?`,
				name, nullIfEmpty(room), furnitureID, userID,
			)
		case "add_shelf":
			label := normalizeShelfLabel(r.FormValue("label"))
			cases := atoiDefault(r.FormValue("case_count"), 10)
			booksPerCase := clampLibraryBooksPerCase(atoiDefault(r.FormValue("books_per_case"), 8))
			if cases < 1 {
				cases = 1
			}
			if cases > 50 {
				cases = 50
			}
			if label == "" {
				if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.Itoa(furnitureID)+"/edit?error=label", http.StatusFound)
				return
			}
			var maxSort int
			_ = a.DB.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) FROM library_shelves WHERE furniture_id = ?`, furnitureID).Scan(&maxSort)
			_, err = a.DB.Exec(
				`INSERT INTO library_shelves (furniture_id, label, case_count, books_per_case, sort_order) VALUES (?, ?, ?, ?, ?)`,
				furnitureID, label, cases, booksPerCase, maxSort+1,
			)
			if err != nil {
				if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
					w.WriteHeader(http.StatusConflict)
					return
				}
				http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.Itoa(furnitureID)+"/edit?error=dup", http.StatusFound)
				return
			}
		case "update_shelf":
			shelfID := atoiDefault(r.FormValue("shelf_id"), 0)
			cases := atoiDefault(r.FormValue("case_count"), 1)
			booksPerCase := clampLibraryBooksPerCase(atoiDefault(r.FormValue("books_per_case"), 8))
			if cases < 1 {
				cases = 1
			}
			if cases > 50 {
				cases = 50
			}
			_, err = a.DB.Exec(
				`UPDATE library_shelves SET case_count = ?, books_per_case = ?
				 WHERE id = ? AND furniture_id = ?`,
				cases, booksPerCase, shelfID, furnitureID,
			)
			if err == nil {
				_, _ = a.DB.Exec(
					`DELETE FROM library_placements WHERE shelf_id = ? AND case_num > ? AND user_id = ?`,
					shelfID, cases, userID,
				)
			}
		case "delete_shelf":
			shelfID := atoiDefault(r.FormValue("shelf_id"), 0)
			_, err = a.DB.Exec(
				`DELETE FROM library_shelves WHERE id = ? AND furniture_id = ?`,
				shelfID, furnitureID,
			)
		default:
			http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.Itoa(furnitureID)+"/edit", http.StatusFound)
			return
		}
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, pathLibraryFurniturePrefix+strconv.Itoa(furnitureID)+"/edit", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleLibraryFurnitureDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	furnitureID, _ := strconv.Atoi(r.PathValue("id"))
	if furnitureID <= 0 {
		http.Redirect(w, r, pathLibraryHome, http.StatusFound)
		return
	}
	_, err := a.DB.Exec(`DELETE FROM library_furniture WHERE id = ? AND user_id = ?`, furnitureID, userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, pathLibraryHome, http.StatusFound)
}

func (a *App) HandleLibraryUnassigned(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)

	type unassignedItem struct {
		ID        int
		Title     string
		MediaKind string
		Volume    int
		ImagePath string
		EditURL   string
	}

	var manga []unassignedItem
	rows, err := a.DB.Query(
		`SELECT w.id, w.title, w.tome, COALESCE(w.image_path, '')
		 FROM manga_phys_works w
		 WHERE w.user_id = ?
		   AND NOT EXISTS (
		     SELECT 1 FROM library_placements p
		     WHERE p.user_id = w.user_id AND p.media_kind = 'manga' AND p.work_id = w.id
		   )
		 ORDER BY LOWER(w.title), w.tome LIMIT 200`,
		userID,
	)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var it unassignedItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Volume, &it.ImagePath); err != nil {
			_ = rows.Close()
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		it.MediaKind = libraryMediaManga
		it.EditURL = pathMangaPhysEditPrefix + strconv.Itoa(it.ID)
		manga = append(manga, it)
	}
	_ = rows.Close()

	var bd []unassignedItem
	rows, err = a.DB.Query(
		`SELECT b.id, b.title, b.tome, COALESCE(b.image_path, '')
		 FROM bd_works b
		 WHERE b.user_id = ?
		   AND NOT EXISTS (
		     SELECT 1 FROM library_placements p
		     WHERE p.user_id = b.user_id AND p.media_kind = 'bd' AND p.work_id = b.id
		   )
		 ORDER BY LOWER(b.title), b.tome LIMIT 200`,
		userID,
	)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var it unassignedItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Volume, &it.ImagePath); err != nil {
			_ = rows.Close()
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		it.MediaKind = libraryMediaBD
		it.EditURL = pathBdEditPrefix + strconv.Itoa(it.ID)
		bd = append(bd, it)
	}
	_ = rows.Close()

	lang := a.currentLang(r)
	a.renderTemplate(w, r, "library_unassigned", a.mergeData(r, map[string]any{
		"Manga":             manga,
		"Bd":                bd,
		"MobileTopbarTitle": i18n.T(lang)["library.unassigned.title"],
	}))
}
