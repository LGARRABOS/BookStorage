package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type libraryPlacementAPI struct {
	ID         int    `json:"id"`
	ShelfID    int    `json:"shelf_id"`
	CaseNum    int    `json:"case_num"`
	Position   int    `json:"position"`
	MediaKind  string `json:"media_kind"`
	WorkID     int    `json:"work_id"`
	Volume     int    `json:"volume"`
	Code       string `json:"code"`
	Title      string `json:"title"`
	ImagePath  string `json:"image_path,omitempty"`
	Furniture  string `json:"furniture,omitempty"`
	Location   string `json:"location,omitempty"`
	ShelfLabel string `json:"shelf_label,omitempty"`
}

func placementToAPI(p libraryPlacementRow) libraryPlacementAPI {
	img := ""
	if p.ImagePath.Valid {
		img = p.ImagePath.String
	}
	return libraryPlacementAPI{
		ID:         p.ID,
		ShelfID:    p.ShelfID,
		CaseNum:    p.CaseNum,
		Position:   p.Position,
		MediaKind:  p.MediaKind,
		WorkID:     p.WorkID,
		Volume:     p.Volume,
		Code:       p.Code(),
		Title:      p.Title,
		ImagePath:  img,
		Furniture:  p.Furniture,
		Location:   p.LocationLabel(),
		ShelfLabel: p.ShelfLabel,
	}
}

func (a *App) HandleAPILibraryFurniture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	furnitureID, _ := strconv.Atoi(r.PathValue("id"))
	furn, err := a.getLibraryFurniture(userID, furnitureID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	shelves, err := a.listLibraryShelves(furnitureID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	places, err := a.listLibraryPlacementsForFurniture(userID, furnitureID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	shelfOut := make([]map[string]any, 0, len(shelves))
	for _, s := range shelves {
		shelfOut = append(shelfOut, map[string]any{
			"id":             s.ID,
			"label":          s.Label,
			"case_count":     s.CaseCount,
			"books_per_case": s.BooksPerCase,
			"sort_order":     s.SortOrder,
		})
	}
	placeOut := make([]libraryPlacementAPI, 0, len(places))
	for _, p := range places {
		placeOut = append(placeOut, placementToAPI(p))
	}
	room := ""
	if furn.RoomLabel.Valid {
		room = furn.RoomLabel.String
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"furniture": map[string]any{
			"id":         furn.ID,
			"name":       furn.Name,
			"room_label": room,
		},
		"shelves":    shelfOut,
		"placements": placeOut,
	})
}

func (a *App) HandleAPILibraryPlacementsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	var body struct {
		ShelfID   int    `json:"shelf_id"`
		CaseNum   int    `json:"case_num"`
		Position  int    `json:"position"`
		MediaKind string `json:"media_kind"`
		WorkID    int    `json:"work_id"`
		Volume    int    `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	kind := normalizeLibraryMediaKind(body.MediaKind)
	if kind == "" || body.WorkID <= 0 || body.ShelfID <= 0 || body.CaseNum < 1 {
		http.Error(w, "invalid fields", http.StatusBadRequest)
		return
	}
	shelf, ok := a.shelfOwnedByUser(userID, body.ShelfID)
	if !ok {
		http.Error(w, "shelf not found", http.StatusNotFound)
		return
	}
	if body.CaseNum > shelf.CaseCount {
		http.Error(w, "case out of range", http.StatusBadRequest)
		return
	}
	if !a.ownershipOKForLibraryWork(userID, kind, body.WorkID) {
		http.Error(w, "work not found", http.StatusNotFound)
		return
	}
	vol := body.Volume
	if kind == libraryMediaBD || kind == libraryMediaManga {
		var tome int
		table := "bd_works"
		if kind == libraryMediaManga {
			table = "manga_phys_works"
		}
		_ = a.DB.QueryRow(`SELECT tome FROM `+table+` WHERE id = ? AND user_id = ?`, body.WorkID, userID).Scan(&tome)
		if vol < 1 {
			vol = tome
		}
		if vol < 1 {
			vol = 1
		}
	} else if vol < 1 {
		http.Error(w, "volume required", http.StatusBadRequest)
		return
	}
	pos := body.Position
	if pos < 1 {
		var err error
		pos, err = a.nextCasePosition(body.ShelfID, body.CaseNum)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
	}
	id, err := a.insertLibraryRowID(
		`INSERT INTO library_placements (user_id, shelf_id, case_num, position, media_kind, work_id, volume, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		userID, body.ShelfID, body.CaseNum, pos, kind, body.WorkID, vol,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "conflict"})
		return
	}
	p, err := a.getLibraryPlacement(userID, int(id))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "placement": placementToAPI(p)})
}

func (a *App) HandleAPILibraryPlacementsPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	placementID, _ := strconv.Atoi(r.PathValue("id"))
	cur, err := a.getLibraryPlacement(userID, placementID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	var body struct {
		ShelfID  *int   `json:"shelf_id"`
		CaseNum  *int   `json:"case_num"`
		Position *int   `json:"position"`
		Move     string `json:"move"` // up | down
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	shelfID := cur.ShelfID
	caseNum := cur.CaseNum
	pos := cur.Position
	if body.ShelfID != nil {
		shelfID = *body.ShelfID
	}
	if body.CaseNum != nil {
		caseNum = *body.CaseNum
	}
	if body.Position != nil {
		pos = *body.Position
	}
	switch strings.ToLower(strings.TrimSpace(body.Move)) {
	case "up":
		pos = cur.Position - 1
	case "down":
		pos = cur.Position + 1
	}
	if pos < 1 {
		pos = 1
	}
	shelf, ok := a.shelfOwnedByUser(userID, shelfID)
	if !ok {
		http.Error(w, "shelf not found", http.StatusNotFound)
		return
	}
	if caseNum < 1 || caseNum > shelf.CaseCount {
		http.Error(w, "case out of range", http.StatusBadRequest)
		return
	}

	// Swap with neighbour when reordering in the same case.
	if shelfID == cur.ShelfID && caseNum == cur.CaseNum && pos != cur.Position {
		var neighbourID int
		err := a.DB.QueryRow(
			`SELECT id FROM library_placements WHERE shelf_id = ? AND case_num = ? AND position = ? AND user_id = ?`,
			shelfID, caseNum, pos, userID,
		).Scan(&neighbourID)
		if err == nil && neighbourID > 0 {
			tx, err := a.DB.Begin()
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec(`UPDATE library_placements SET position = ? WHERE id = ? AND user_id = ?`, -1, placementID, userID); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(`UPDATE library_placements SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`, cur.Position, neighbourID, userID); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(`UPDATE library_placements SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`, pos, placementID, userID); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			p, _ := a.getLibraryPlacement(userID, placementID)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "placement": placementToAPI(p)})
			return
		}
	}

	_, err = a.DB.Exec(
		`UPDATE library_placements SET shelf_id = ?, case_num = ?, position = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND user_id = ?`,
		shelfID, caseNum, pos, placementID, userID,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "conflict"})
		return
	}
	p, _ := a.getLibraryPlacement(userID, placementID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "placement": placementToAPI(p)})
}

func (a *App) HandleAPILibraryPlacementsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	placementID, _ := strconv.Atoi(r.PathValue("id"))
	res, err := a.DB.Exec(`DELETE FROM library_placements WHERE id = ? AND user_id = ?`, placementID, userID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	if n == 0 {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *App) HandleAPILibrarySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	places, err := a.searchLibraryPlacements(userID, q)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	out := make([]libraryPlacementAPI, 0, len(places))
	for _, p := range places {
		out = append(out, placementToAPI(p))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": out, "q": q})
}

func (a *App) HandleAPILibraryWorksSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := normalizeLibraryMediaKind(r.URL.Query().Get("kind"))
	if kind == "" {
		kind = libraryMediaManga
	}
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		return
	}
	like := "%" + strings.ToLower(q) + "%"
	type item struct {
		ID        int    `json:"id"`
		Title     string `json:"title"`
		Volume    int    `json:"volume,omitempty"`
		ImagePath string `json:"image_path,omitempty"`
		MediaKind string `json:"media_kind"`
	}
	var out []item
	switch kind {
	case libraryMediaBD:
		rows, err := a.DB.Query(
			`SELECT id, title, tome, COALESCE(image_path, '') FROM bd_works
			 WHERE user_id = ? AND LOWER(title) LIKE ?
			 ORDER BY LOWER(title), tome LIMIT 30`,
			userID, like,
		)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var it item
			var img string
			if err := rows.Scan(&it.ID, &it.Title, &it.Volume, &img); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			it.ImagePath = img
			it.MediaKind = libraryMediaBD
			out = append(out, it)
		}
	default:
		rows, err := a.DB.Query(
			`SELECT id, title, tome, COALESCE(image_path, '') FROM manga_phys_works
			 WHERE user_id = ? AND LOWER(title) LIKE ?
			 ORDER BY LOWER(title), tome LIMIT 30`,
			userID, like,
		)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var it item
			var img string
			if err := rows.Scan(&it.ID, &it.Title, &it.Volume, &img); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			it.ImagePath = img
			it.MediaKind = libraryMediaManga
			out = append(out, it)
		}
	}
	if out == nil {
		out = []item{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
}
