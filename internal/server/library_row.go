package server

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"bookstorage/internal/database"
)

const (
	libraryMediaManga = "manga"
	libraryMediaBD    = "bd"
)

var placementCodeRe = regexp.MustCompile(`(?i)^([A-Z])(\d+)-(\d+)$`)

func formatPlacementCode(label string, caseNum, position int) string {
	label = strings.ToUpper(strings.TrimSpace(label))
	if label == "" {
		label = "?"
	}
	if caseNum < 1 {
		caseNum = 1
	}
	if position < 1 {
		position = 1
	}
	return fmt.Sprintf("%s%d-%d", label[:1], caseNum, position)
}

func parsePlacementCode(raw string) (label string, caseNum, position int, ok bool) {
	m := placementCodeRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", 0, 0, false
	}
	caseNum, _ = strconv.Atoi(m[2])
	position, _ = strconv.Atoi(m[3])
	if caseNum < 1 || position < 1 {
		return "", 0, 0, false
	}
	return strings.ToUpper(m[1]), caseNum, position, true
}

func normalizeShelfLabel(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	r := []rune(strings.ToUpper(s))
	if !unicode.IsLetter(r[0]) {
		return ""
	}
	return string(r[0])
}

func normalizeLibraryMediaKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case libraryMediaManga:
		return libraryMediaManga
	case libraryMediaBD, "bande-dessinee", "bande_dessinee":
		return libraryMediaBD
	default:
		return ""
	}
}

type libraryFurnitureRow struct {
	ID        int
	UserID    int
	Name      string
	RoomLabel sql.NullString
	SortOrder int
	ShelfN    int
	PlaceN    int
}

type libraryShelfRow struct {
	ID           int    `json:"id"`
	FurnitureID  int    `json:"furniture_id"`
	Label        string `json:"label"`
	CaseCount    int    `json:"case_count"`
	BooksPerCase int    `json:"books_per_case"`
	SortOrder    int    `json:"sort_order"`
}

type libraryPlacementRow struct {
	ID          int
	UserID      int
	ShelfID     int
	CaseNum     int
	Position    int
	MediaKind   string
	WorkID      int
	Volume      int
	ShelfLabel  string
	FurnitureID int
	Furniture   string
	RoomLabel   sql.NullString
	Title       string
	ImagePath   sql.NullString
}

func (p libraryPlacementRow) Code() string {
	return formatPlacementCode(p.ShelfLabel, p.CaseNum, p.Position)
}

func (p libraryPlacementRow) LocationLabel() string {
	name := strings.TrimSpace(p.Furniture)
	if p.RoomLabel.Valid && strings.TrimSpace(p.RoomLabel.String) != "" {
		name = strings.TrimSpace(p.RoomLabel.String) + " · " + name
	}
	if name == "" {
		return p.Code()
	}
	return name + " · " + p.Code()
}

func (a *App) listLibraryFurniture(userID int) ([]libraryFurnitureRow, error) {
	rows, err := a.DB.Query(
		`SELECT f.id, f.user_id, f.name, f.room_label, f.sort_order,
		        (SELECT COUNT(*) FROM library_shelves s WHERE s.furniture_id = f.id),
		        (SELECT COUNT(*) FROM library_placements p
		          JOIN library_shelves s ON s.id = p.shelf_id
		         WHERE s.furniture_id = f.id AND p.user_id = f.user_id)
		 FROM library_furniture f
		 WHERE f.user_id = ?
		 ORDER BY f.sort_order, LOWER(f.name), f.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []libraryFurnitureRow
	for rows.Next() {
		var r libraryFurnitureRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.RoomLabel, &r.SortOrder, &r.ShelfN, &r.PlaceN); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *App) getLibraryFurniture(userID, furnitureID int) (libraryFurnitureRow, error) {
	var r libraryFurnitureRow
	err := a.DB.QueryRow(
		`SELECT id, user_id, name, room_label, sort_order, 0, 0
		 FROM library_furniture WHERE id = ? AND user_id = ?`,
		furnitureID, userID,
	).Scan(&r.ID, &r.UserID, &r.Name, &r.RoomLabel, &r.SortOrder, &r.ShelfN, &r.PlaceN)
	return r, err
}

func clampLibraryBooksPerCase(n int) int {
	if n < 2 {
		return 2
	}
	if n > 120 {
		return 120
	}
	return n
}

func (a *App) listLibraryShelves(furnitureID int) ([]libraryShelfRow, error) {
	rows, err := a.DB.Query(
		`SELECT id, furniture_id, label, case_count, COALESCE(books_per_case, 8), sort_order
		 FROM library_shelves WHERE furniture_id = ?
		 ORDER BY sort_order, label, id`,
		furnitureID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []libraryShelfRow
	for rows.Next() {
		var r libraryShelfRow
		if err := rows.Scan(&r.ID, &r.FurnitureID, &r.Label, &r.CaseCount, &r.BooksPerCase, &r.SortOrder); err != nil {
			return nil, err
		}
		r.BooksPerCase = clampLibraryBooksPerCase(r.BooksPerCase)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *App) listLibraryPlacementsForFurniture(userID, furnitureID int) ([]libraryPlacementRow, error) {
	rows, err := a.DB.Query(
		`SELECT p.id, p.user_id, p.shelf_id, p.case_num, p.position, p.media_kind, p.work_id, p.volume,
		        s.label, f.id, f.name, f.room_label,
		        COALESCE(
		          CASE WHEN p.media_kind = 'manga' THEN (SELECT title FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		               WHEN p.media_kind = 'bd' THEN (SELECT title FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		               ELSE '' END, ''),
		        CASE WHEN p.media_kind = 'manga' THEN (SELECT image_path FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		             WHEN p.media_kind = 'bd' THEN (SELECT image_path FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		             ELSE NULL END
		 FROM library_placements p
		 JOIN library_shelves s ON s.id = p.shelf_id
		 JOIN library_furniture f ON f.id = s.furniture_id
		 WHERE p.user_id = ? AND f.id = ?
		 ORDER BY s.sort_order, s.label, p.case_num, p.position, p.id`,
		userID, furnitureID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLibraryPlacementRows(rows)
}

func (a *App) searchLibraryPlacements(userID int, q string) ([]libraryPlacementRow, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	like := "%" + strings.ToLower(q) + "%"
	rows, err := a.DB.Query(
		`SELECT p.id, p.user_id, p.shelf_id, p.case_num, p.position, p.media_kind, p.work_id, p.volume,
		        s.label, f.id, f.name, f.room_label,
		        COALESCE(
		          CASE WHEN p.media_kind = 'manga' THEN (SELECT title FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		               WHEN p.media_kind = 'bd' THEN (SELECT title FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		               ELSE '' END, ''),
		        CASE WHEN p.media_kind = 'manga' THEN (SELECT image_path FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		             WHEN p.media_kind = 'bd' THEN (SELECT image_path FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		             ELSE NULL END
		 FROM library_placements p
		 JOIN library_shelves s ON s.id = p.shelf_id
		 JOIN library_furniture f ON f.id = s.furniture_id
		 WHERE p.user_id = ?
		   AND (
		     LOWER(COALESCE(
		       CASE WHEN p.media_kind = 'manga' THEN (SELECT title FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		            WHEN p.media_kind = 'bd' THEN (SELECT title FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		            ELSE '' END, '')) LIKE ?
		     OR LOWER(f.name) LIKE ?
		     OR LOWER(COALESCE(f.room_label, '')) LIKE ?
		     OR LOWER(s.label || CAST(p.case_num AS TEXT) || '-' || CAST(p.position AS TEXT)) LIKE ?
		   )
		 ORDER BY LOWER(f.name), s.label, p.case_num, p.position
		 LIMIT 50`,
		userID, like, like, like, like,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLibraryPlacementRows(rows)
}

func scanLibraryPlacementRows(rows *sql.Rows) ([]libraryPlacementRow, error) {
	var out []libraryPlacementRow
	for rows.Next() {
		var r libraryPlacementRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.ShelfID, &r.CaseNum, &r.Position, &r.MediaKind, &r.WorkID, &r.Volume,
			&r.ShelfLabel, &r.FurnitureID, &r.Furniture, &r.RoomLabel, &r.Title, &r.ImagePath,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *App) getLibraryPlacement(userID, placementID int) (libraryPlacementRow, error) {
	var r libraryPlacementRow
	err := a.DB.QueryRow(
		`SELECT p.id, p.user_id, p.shelf_id, p.case_num, p.position, p.media_kind, p.work_id, p.volume,
		        s.label, f.id, f.name, f.room_label,
		        COALESCE(
		          CASE WHEN p.media_kind = 'manga' THEN (SELECT title FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		               WHEN p.media_kind = 'bd' THEN (SELECT title FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		               ELSE '' END, ''),
		        CASE WHEN p.media_kind = 'manga' THEN (SELECT image_path FROM works w WHERE w.id = p.work_id AND w.user_id = p.user_id)
		             WHEN p.media_kind = 'bd' THEN (SELECT image_path FROM bd_works b WHERE b.id = p.work_id AND b.user_id = p.user_id)
		             ELSE NULL END
		 FROM library_placements p
		 JOIN library_shelves s ON s.id = p.shelf_id
		 JOIN library_furniture f ON f.id = s.furniture_id
		 WHERE p.id = ? AND p.user_id = ?`,
		placementID, userID,
	).Scan(
		&r.ID, &r.UserID, &r.ShelfID, &r.CaseNum, &r.Position, &r.MediaKind, &r.WorkID, &r.Volume,
		&r.ShelfLabel, &r.FurnitureID, &r.Furniture, &r.RoomLabel, &r.Title, &r.ImagePath,
	)
	return r, err
}

func (a *App) nextCasePosition(shelfID, caseNum int) (int, error) {
	var maxPos sql.NullInt64
	err := a.DB.QueryRow(
		`SELECT MAX(position) FROM library_placements WHERE shelf_id = ? AND case_num = ?`,
		shelfID, caseNum,
	).Scan(&maxPos)
	if err != nil {
		return 0, err
	}
	if !maxPos.Valid {
		return 1, nil
	}
	return int(maxPos.Int64) + 1, nil
}

func (a *App) ownershipOKForLibraryWork(userID int, kind string, workID int) bool {
	if userID <= 0 || workID <= 0 {
		return false
	}
	var n int
	switch kind {
	case libraryMediaManga:
		_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&n)
	case libraryMediaBD:
		_ = a.DB.QueryRow(`SELECT COUNT(*) FROM bd_works WHERE id = ? AND user_id = ?`, workID, userID).Scan(&n)
	default:
		return false
	}
	return n > 0
}

func (a *App) shelfOwnedByUser(userID, shelfID int) (libraryShelfRow, bool) {
	var r libraryShelfRow
	err := a.DB.QueryRow(
		`SELECT s.id, s.furniture_id, s.label, s.case_count, COALESCE(s.books_per_case, 8), s.sort_order
		 FROM library_shelves s
		 JOIN library_furniture f ON f.id = s.furniture_id
		 WHERE s.id = ? AND f.user_id = ?`,
		shelfID, userID,
	).Scan(&r.ID, &r.FurnitureID, &r.Label, &r.CaseCount, &r.BooksPerCase, &r.SortOrder)
	if err == nil {
		r.BooksPerCase = clampLibraryBooksPerCase(r.BooksPerCase)
	}
	return r, err == nil
}

// libraryPlacementSummaryForWork returns placements for a manga series or BD album (badge).
func (a *App) libraryPlacementSummaryForWork(userID int, kind string, workID int) []libraryPlacementRow {
	kind = normalizeLibraryMediaKind(kind)
	if kind == "" || workID <= 0 {
		return nil
	}
	rows, err := a.DB.Query(
		`SELECT p.id, p.user_id, p.shelf_id, p.case_num, p.position, p.media_kind, p.work_id, p.volume,
		        s.label, f.id, f.name, f.room_label, '', NULL
		 FROM library_placements p
		 JOIN library_shelves s ON s.id = p.shelf_id
		 JOIN library_furniture f ON f.id = s.furniture_id
		 WHERE p.user_id = ? AND p.media_kind = ? AND p.work_id = ?
		 ORDER BY s.label, p.case_num, p.position`,
		userID, kind, workID,
	)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out, _ := scanLibraryPlacementRows(rows)
	return out
}

// insertLibraryRowID inserts a row and returns its id. PostgreSQL (lib/pq) does not
// support LastInsertId, so we use RETURNING there.
func (a *App) insertLibraryRowID(query string, args ...any) (int64, error) {
	if a == nil || a.DB == nil {
		return 0, sql.ErrConnDone
	}
	if a.DB.B == database.BackendPostgres {
		var id int64
		err := a.DB.QueryRow(query+` RETURNING id`, args...).Scan(&id)
		return id, err
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
