package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type communityUser struct {
	ID          int
	Username    string
	DisplayName sql.NullString
	Bio         sql.NullString
	AvatarPath  sql.NullString
	IsPublic    sql.NullInt64
}

func (a *App) HandleUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	sqlStr := `
SELECT id, username, display_name, bio, avatar_path, is_public
FROM users
WHERE validated = 1 AND is_public = 1 AND id != ?`
	args := []any{userID}

	if query != "" {
		pattern := "%" + strings.ToLower(query) + "%"
		sqlStr += ` AND (LOWER(username) LIKE ? OR LOWER(COALESCE(display_name, '')) LIKE ?)`
		args = append(args, pattern, pattern)
	}
	sqlStr += " ORDER BY LOWER(COALESCE(display_name, username))"

	rows, err := a.DB.Query(sqlStr, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var users []communityUser
	for rows.Next() {
		var u communityUser
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarPath, &u.IsPublic); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	a.renderTemplate(w, r, "users", a.mergeData(r, map[string]any{
		"Users": users,
		"Query": query,
	}))
}

type fullUser struct {
	ID          int
	Username    string
	DisplayName sql.NullString
	Bio         sql.NullString
	AvatarPath  sql.NullString
	IsPublic    sql.NullInt64
	IsAdmin     int
}

func (a *App) canViewProfile(viewerID int, target fullUser) bool {
	if viewerID == target.ID {
		return true
	}
	if target.IsPublic.Valid && target.IsPublic.Int64 != 0 {
		return true
	}
	var isAdmin int
	if err := a.DB.QueryRow(
		`SELECT is_admin FROM users WHERE id = ?`,
		viewerID,
	).Scan(&isAdmin); err == nil && isAdmin != 0 {
		return true
	}
	return false
}

func (a *App) HandleUserDetail(w http.ResponseWriter, r *http.Request) {
	viewerID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))

	var u fullUser
	err := a.DB.QueryRow(
		`SELECT id, username, display_name, bio, avatar_path, is_public, is_admin
         FROM users WHERE id = ?`,
		targetID,
	).Scan(
		&u.ID,
		&u.Username,
		&u.DisplayName,
		&u.Bio,
		&u.AvatarPath,
		&u.IsPublic,
		&u.IsAdmin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !a.canViewProfile(viewerID, u) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	rows, err := a.DB.Query(
		`SELECT `+sqlWorkRowFull+`
         FROM works WHERE user_id = ? ORDER BY LOWER(title)`,
		targetID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []workRow
	for rows.Next() {
		var wRow workRow
		if err := scanFullWorkRow(&wRow, rows); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	a.renderTemplate(w, r, "user_detail", a.mergeData(r, map[string]any{
		"TargetUser": u,
		"Works":      works,
		"CanImport":  viewerID != targetID,
	}))
}

func (a *App) HandleImportWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	viewerID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("user_id"))
	workID, _ := strconv.Atoi(r.PathValue("work_id"))

	var target fullUser
	err := a.DB.QueryRow(
		`SELECT id, username, display_name, bio, avatar_path, is_public, is_admin
         FROM users WHERE id = ?`,
		targetID,
	).Scan(
		&target.ID,
		&target.Username,
		&target.DisplayName,
		&target.Bio,
		&target.AvatarPath,
		&target.IsPublic,
		&target.IsAdmin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !a.canViewProfile(viewerID, target) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if viewerID == targetID {
		http.Redirect(w, r, "/users/"+strconv.Itoa(targetID), http.StatusFound)
		return
	}

	var src workRow
	err = scanFullWorkRow(&src, a.DB.QueryRow(
		`SELECT `+sqlWorkRowFull+`
         FROM works WHERE id = ? AND user_id = ?`,
		workID, targetID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var existsID int
	err = a.DB.QueryRow(
		`SELECT id FROM works
         WHERE user_id = ? AND title = ? AND COALESCE(link, '') = COALESCE(?, '')`,
		viewerID, src.Title, nullableString(src.Link),
	).Scan(&existsID)
	if err == nil && existsID != 0 {
		http.Redirect(w, r, "/users/"+strconv.Itoa(targetID), http.StatusFound)
		return
	}

	readingType := "Roman"
	if src.ReadingType.Valid && src.ReadingType.String != "" {
		readingType = src.ReadingType.String
	}
	stCopy := "En cours"
	if src.Status.Valid {
		stCopy = normalizeStatusForWrite(src.Status.String)
	}
	notifyCh := notifyNewChaptersDB(stCopy, src.NotifyNewChapters != 0)

	_, err = a.DB.Exec(
		`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, notes, user_id, notify_new_chapters)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		src.Title,
		src.Chapter,
		nullableString(src.Link),
		nullableString(src.Status),
		nullableString(src.ImagePath),
		readingType,
		src.Rating,
		nullableString(src.Notes),
		viewerID,
		notifyCh,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/users/"+strconv.Itoa(targetID), http.StatusFound)
}

func nullableString(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}
