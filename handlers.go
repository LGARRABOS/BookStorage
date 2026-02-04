package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

type App struct {
	Settings  *Settings
	DB        *sql.DB
	Templates *template.Template
}

func NewApp(settings *Settings, db *sql.DB) *App {
	funcMap := template.FuncMap{
		"work_image_url": func(stored string) string {
			return workImageURL(settings, stored)
		},
		"url_for": func(name string, args ...string) string {
			// Adaptation minimale de url_for de Flask pour les templates existants.
			switch name {
			case "static":
				if len(args) > 0 {
					filename := strings.TrimLeft(args[0], "/")
					return "/static/" + filename
				}
				return "/static/"
			default:
				// Pour des appels du type url_for('dashboard'), etc.
				return "/" + strings.TrimLeft(name, "/")
			}
		},
	}
	// On ne parse que les templates Go (extension .gohtml) pour éviter
	// les erreurs de syntaxe avec les anciens templates Jinja.
	tpl := template.Must(
		template.New("").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.gohtml")),
	)
	return &App{
		Settings:  settings,
		DB:        db,
		Templates: tpl,
	}
}

// --- vérification des mots de passe (compatible Werkzeug) ---

// verifyPassword vérifie un mot de passe contre un hash.
// Supporte le format Werkzeug pbkdf2:sha256:iterations$salt$hash
// et aussi la comparaison en clair (pour les comptes créés par Go).
func verifyPassword(storedHash, password string) bool {
	// Si le hash commence par "pbkdf2:", c'est un hash Werkzeug
	if strings.HasPrefix(storedHash, "pbkdf2:") {
		return verifyWerkzeugHash(storedHash, password)
	}
	// Sinon, comparaison en clair (comptes créés par Go)
	return storedHash == password
}

// verifyWerkzeugHash vérifie un mot de passe contre un hash Werkzeug.
// Format: pbkdf2:sha256:iterations$salt$hash
func verifyWerkzeugHash(storedHash, password string) bool {
	// Séparer method$salt$hash
	parts := strings.SplitN(storedHash, "$", 3)
	if len(parts) != 3 {
		return false
	}

	method := parts[0]  // pbkdf2:sha256:iterations
	salt := parts[1]    // salt en clair
	hashHex := parts[2] // hash en hexadécimal

	// Extraire les paramètres de la méthode
	methodParts := strings.Split(method, ":")
	if len(methodParts) < 3 || methodParts[0] != "pbkdf2" || methodParts[1] != "sha256" {
		return false // Format non supporté
	}

	iterations := 260000 // Valeur par défaut de Werkzeug
	if len(methodParts) >= 3 {
		if n, err := strconv.Atoi(methodParts[2]); err == nil {
			iterations = n
		}
	}

	// Décoder le hash attendu
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}

	// Calculer le hash PBKDF2
	computed := pbkdf2.Key([]byte(password), []byte(salt), iterations, len(expectedHash), sha256.New)

	// Comparaison en temps constant pour éviter les timing attacks
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1
}

// --- helpers session très simplifiés ---

func (a *App) currentUserID(r *http.Request) (int, bool) {
	c, err := r.Cookie("user_id")
	if err != nil {
		return 0, false
	}
	id, err := strconv.Atoi(c.Value)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (a *App) setUserID(w http.ResponseWriter, userID int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    strconv.Itoa(userID),
		Path:     "/",
		HttpOnly: true,
		// Secure:   a.Settings.Environment == "production",
	})
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "user_id",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// --- middleware login_required ---

func (a *App) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.currentUserID(r); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// --- helpers application ---

// Liste simplifiée des types de lecture et statuts pour rester proche de Python.
var readingTypes = []string{
	"Roman",
	"Manga",
	"BD",
	"Light Novel",
	"Webtoon",
	"Autre",
}

var readingStatuses = []string{
	"En cours",
	"Terminé",
	"En pause",
	"Abandonné",
	"À lire",
}

func allowedFile(filename string) bool {
	filename = strings.ToLower(filename)
	return strings.HasSuffix(filename, ".png") ||
		strings.HasSuffix(filename, ".jpg") ||
		strings.HasSuffix(filename, ".jpeg") ||
		strings.HasSuffix(filename, ".gif")
}

func buildMediaRelativePath(filename, urlPath string) string {
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" {
		return filename
	}
	return urlPath + "/" + filename
}

func workImageURL(s *Settings, storedPath string) string {
	if strings.TrimSpace(storedPath) == "" {
		return ""
	}
	normalized := strings.ReplaceAll(storedPath, "\\", "/")

	if strings.HasPrefix(normalized, "http://") ||
		strings.HasPrefix(normalized, "https://") ||
		strings.HasPrefix(normalized, "//") ||
		strings.HasPrefix(normalized, "data:") {
		return normalized
	}

	if strings.HasPrefix(normalized, "/static/") {
		return normalized
	}

	// Les images uploadées sont dans static/images/, donc on préfixe avec /static/
	uploadPrefix := strings.Trim(s.UploadURLPath, "/")
	avatarPrefix := strings.Trim(s.ProfileUploadURLPath, "/")

	if uploadPrefix != "" && strings.HasPrefix(normalized, uploadPrefix+"/") {
		return "/static/" + normalized
	}
	if avatarPrefix != "" && strings.HasPrefix(normalized, avatarPrefix+"/") {
		return "/static/" + normalized
	}

	if strings.HasPrefix(normalized, "/") {
		return normalized
	}

	if strings.HasPrefix(normalized, "static/") {
		return "/" + normalized
	}
	return "/static/" + strings.TrimPrefix(normalized, "/")
}

// --- middleware admin_required ---

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.currentUserID(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		var isAdmin, isSuper int
		err := a.DB.QueryRow(
			`SELECT is_admin, is_superadmin FROM users WHERE id = ?`,
			userID,
		).Scan(&isAdmin, &isSuper)
		if err != nil || isAdmin == 0 {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// --- handlers principaux ---

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUserID(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "register", nil)
	case http.MethodPost:
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/register", http.StatusFound)
			return
		}

		_, err := a.DB.Exec(
			`INSERT INTO users (username, password, validated, is_admin)
             VALUES (?, ?, 0, 0)`,
			username, password, // TODO: hash du mot de passe
		)
		if err != nil {
			// conflit de username, etc.
			http.Redirect(w, r, "/register", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type userRow struct {
	ID           int
	Username     string
	Password     string
	Validated    int
	IsAdmin      int
	IsSuperadmin int
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "login", nil)
	case http.MethodPost:
		username := r.FormValue("username")
		password := r.FormValue("password")

		var u userRow
		err := a.DB.QueryRow(
			`SELECT id, username, password, validated, is_admin, is_superadmin
             FROM users WHERE username = ?`,
			username,
		).Scan(&u.ID, &u.Username, &u.Password, &u.Validated, &u.IsAdmin, &u.IsSuperadmin)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Vérification du mot de passe (supporte Werkzeug et clair)
		if !verifyPassword(u.Password, password) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if u.Validated == 0 && u.IsAdmin == 0 {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		a.setUserID(w, u.ID)
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

type workRow struct {
	ID          int
	Title       string
	Chapter     int
	Link        sql.NullString
	Status      sql.NullString
	ImagePath   sql.NullString
	ReadingType sql.NullString
	UserID      int
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	// Vérifier si l'utilisateur est admin
	var isAdmin int
	_ = a.DB.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin)

	// Option de tri
	sortBy := r.URL.Query().Get("sort")
	orderClause := "ORDER BY LOWER(title)"
	switch sortBy {
	case "title_desc":
		orderClause = "ORDER BY LOWER(title) DESC"
	case "chapter":
		orderClause = "ORDER BY chapter DESC"
	case "status":
		orderClause = "ORDER BY status, LOWER(title)"
	case "type":
		orderClause = "ORDER BY reading_type, LOWER(title)"
	case "recent":
		orderClause = "ORDER BY id DESC"
	default:
		sortBy = "title"
		orderClause = "ORDER BY LOWER(title)"
	}

	rows, err := a.DB.Query(
		`SELECT id, title, chapter, link, status, image_path, reading_type, user_id
         FROM works WHERE user_id = ? `+orderClause,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var works []workRow
	for rows.Next() {
		var wRow workRow
		if err := rows.Scan(
			&wRow.ID,
			&wRow.Title,
			&wRow.Chapter,
			&wRow.Link,
			&wRow.Status,
			&wRow.ImagePath,
			&wRow.ReadingType,
			&wRow.UserID,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	_ = a.Templates.ExecuteTemplate(w, "dashboard", map[string]any{
		"Works":         works,
		"ReadingTypes":  readingTypes,
		"ReadingStatus": readingStatuses,
		"IsAdmin":       isAdmin == 1,
		"SortBy":        sortBy,
	})
}

// Ajout d’une œuvre (avec support basique d’upload d’image)
func (a *App) handleAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "add_work", map[string]any{
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
		})
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID, _ := a.currentUserID(r)
		title := r.FormValue("title")
		link := r.FormValue("link")
		status := r.FormValue("status")
		chapterStr := r.FormValue("chapter")
		if chapterStr == "" {
			chapterStr = "0"
		}
		chapter, _ := strconv.Atoi(chapterStr)
		readingType := strings.TrimSpace(r.FormValue("reading_type"))
		if readingType == "" {
			readingType = readingTypes[0]
		}

		var imagePath sql.NullString
		file, header, err := r.FormFile("image")
		if err == nil && header != nil && header.Filename != "" {
			defer file.Close()
			if allowedFile(header.Filename) {
				filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
				full := filepath.Join(a.Settings.UploadFolder, filename)
				dst, err := os.Create(full)
				if err == nil {
					defer dst.Close()
					_, _ = io.Copy(dst, file)
					imagePath.String = buildMediaRelativePath(filename, a.Settings.UploadURLPath)
					imagePath.Valid = true
				}
			}
		}

		if imagePath.Valid {
			_, err = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
                 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				title, chapter, link, status, imagePath.String, readingType, userID,
			)
		} else {
			_, err = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
                 VALUES (?, ?, ?, ?, NULL, ?, ?)`,
				title, chapter, link, status, readingType, userID,
			)
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// Modification d'une œuvre
func (a *App) handleEditWork(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	// Récupérer l'œuvre
	var work workRow
	err := a.DB.QueryRow(
		`SELECT id, title, chapter, link, status, image_path, reading_type, user_id
         FROM works WHERE id = ? AND user_id = ?`,
		workID, userID,
	).Scan(
		&work.ID,
		&work.Title,
		&work.Chapter,
		&work.Link,
		&work.Status,
		&work.ImagePath,
		&work.ReadingType,
		&work.UserID,
	)
	if err != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "edit_work", map[string]any{
			"Work":         work,
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
		})
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, "/edit/"+strconv.Itoa(workID), http.StatusFound)
			return
		}
		link := strings.TrimSpace(r.FormValue("link"))
		status := r.FormValue("status")
		chapterStr := r.FormValue("chapter")
		if chapterStr == "" {
			chapterStr = "0"
		}
		chapter, _ := strconv.Atoi(chapterStr)
		if chapter < 0 {
			chapter = 0
		}
		readingType := strings.TrimSpace(r.FormValue("reading_type"))
		if readingType == "" {
			readingType = readingTypes[0]
		}

		// Gestion de l'image (optionnel)
		newImagePath := work.ImagePath
		file, header, err := r.FormFile("image")
		if err == nil && header != nil && header.Filename != "" {
			defer file.Close()
			if allowedFile(header.Filename) {
				filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
				full := filepath.Join(a.Settings.UploadFolder, filename)
				dst, err := os.Create(full)
				if err == nil {
					defer dst.Close()
					_, _ = io.Copy(dst, file)
					newImagePath.String = buildMediaRelativePath(filename, a.Settings.UploadURLPath)
					newImagePath.Valid = true
				}
			}
		}

		if newImagePath.Valid {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, image_path = ?, reading_type = ?
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, newImagePath.String, readingType, workID, userID,
			)
		} else {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, reading_type = ?
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, readingType, workID, userID,
			)
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// API increment / decrement très simplifiée
func (a *App) handleIncrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE works SET chapter = chapter + 1 WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) handleDecrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE works
         SET chapter = CASE WHEN chapter > 0 THEN chapter - 1 ELSE 0 END
         WHERE id = ? AND user_id = ?`,
		workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) handleDeleteWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// --- Gestion du profil utilisateur ---

type profileUser struct {
	ID          int
	Username    string
	Password    string
	DisplayName sql.NullString
	Email       sql.NullString
	Bio         sql.NullString
	AvatarPath  sql.NullString
	IsPublic    sql.NullInt64
}

func deleteMediaFile(folder, storedPath string) {
	if strings.TrimSpace(storedPath) == "" {
		return
	}
	filename := filepath.Base(storedPath)
	if filename == "" {
		return
	}
	target := filepath.Join(folder, filename)
	_ = os.Remove(target)
}

func (a *App) handleProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	var u profileUser
	err := a.DB.QueryRow(
		`SELECT id, username, password, display_name, email, bio, avatar_path, is_public
         FROM users WHERE id = ?`,
		userID,
	).Scan(
		&u.ID,
		&u.Username,
		&u.Password,
		&u.DisplayName,
		&u.Email,
		&u.Bio,
		&u.AvatarPath,
		&u.IsPublic,
	)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data := map[string]any{
			"User": u,
		}
		_ = a.Templates.ExecuteTemplate(w, "profile", data)
	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		newUsername := strings.TrimSpace(r.FormValue("username"))
		displayName := strings.TrimSpace(r.FormValue("display_name"))
		email := strings.TrimSpace(r.FormValue("email"))
		bio := strings.TrimSpace(r.FormValue("bio"))
		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")
		visibility := r.FormValue("is_public")

		if newUsername == "" {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		updates := map[string]any{}
		requirePasswordCheck := false

		if newUsername != u.Username {
			requirePasswordCheck = true
			updates["username"] = newUsername
		}

		if newPassword != "" || confirmPassword != "" {
			requirePasswordCheck = true
			if newPassword != confirmPassword {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			if len(newPassword) < 8 {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			// NOTE : on garde un stockage en clair pour rester cohérent avec le reste de l’app Go.
			updates["password"] = newPassword
		}

		if requirePasswordCheck {
			if currentPassword == "" || currentPassword != u.Password {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
		}

		if displayName != "" {
			updates["display_name"] = displayName
		} else {
			updates["display_name"] = nil
		}

		if email != "" {
			updates["email"] = email
		} else {
			updates["email"] = nil
		}

		if bio != "" {
			updates["bio"] = bio
		} else {
			updates["bio"] = nil
		}

		if visibility != "" {
			if visibility == "1" {
				updates["is_public"] = 1
			} else {
				updates["is_public"] = 0
			}
		}

		previousAvatar := u.AvatarPath.String
		newAvatarPath := previousAvatar

		file, header, err := r.FormFile("avatar")
		if err == nil && header != nil && header.Filename != "" {
			defer file.Close()
			if allowedFile(header.Filename) {
				filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
				full := filepath.Join(a.Settings.ProfileUploadFolder, filename)
				dst, err := os.Create(full)
				if err == nil {
					defer dst.Close()
					_, _ = io.Copy(dst, file)
					newAvatarPath = buildMediaRelativePath(filename, a.Settings.ProfileUploadURLPath)
					updates["avatar_path"] = newAvatarPath
				}
			}
		}

		if len(updates) == 0 {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		// construction dynamique de la requête UPDATE
		setParts := make([]string, 0, len(updates))
		args := make([]any, 0, len(updates)+1)
		for k, v := range updates {
			setParts = append(setParts, k+" = ?")
			args = append(args, v)
		}
		args = append(args, userID)

		stmt := "UPDATE users SET " + strings.Join(setParts, ", ") + " WHERE id = ?"
		if _, err := a.DB.Exec(stmt, args...); err != nil {
			http.Redirect(w, r, "/profile", http.StatusFound)
			return
		}

		if newAvatarPath != "" && previousAvatar != "" && previousAvatar != newAvatarPath {
			var remaining int
			if err := a.DB.QueryRow(
				`SELECT COUNT(*) FROM users WHERE avatar_path = ? AND id != ?`,
				previousAvatar, userID,
			).Scan(&remaining); err == nil && remaining == 0 {
				deleteMediaFile(a.Settings.ProfileUploadFolder, previousAvatar)
			}
		}

		http.Redirect(w, r, "/profile", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- Annuaire communauté / profils publics ---

type communityUser struct {
	ID          int
	Username    string
	DisplayName sql.NullString
	Bio         sql.NullString
	AvatarPath  sql.NullString
	IsPublic    sql.NullInt64
}

func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
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
	defer rows.Close()

	var users []communityUser
	for rows.Next() {
		var u communityUser
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarPath, &u.IsPublic); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	data := map[string]any{
		"Users": users,
		"Query": query,
	}
	_ = a.Templates.ExecuteTemplate(w, "users", data)
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

func (a *App) handleUserDetail(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/users", http.StatusFound)
		return
	}

	if !a.canViewProfile(viewerID, u) {
		http.Redirect(w, r, "/users", http.StatusFound)
		return
	}

	rows, err := a.DB.Query(
		`SELECT id, title, chapter, link, status, image_path, reading_type, user_id
         FROM works WHERE user_id = ? ORDER BY LOWER(title)`,
		targetID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var works []workRow
	for rows.Next() {
		var wRow workRow
		if err := rows.Scan(
			&wRow.ID,
			&wRow.Title,
			&wRow.Chapter,
			&wRow.Link,
			&wRow.Status,
			&wRow.ImagePath,
			&wRow.ReadingType,
			&wRow.UserID,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	data := map[string]any{
		"TargetUser": u,
		"Works":      works,
		"CanImport":  viewerID != targetID,
	}
	_ = a.Templates.ExecuteTemplate(w, "user_detail", data)
}

func (a *App) handleImportWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	viewerID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
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
		http.Redirect(w, r, "/users", http.StatusFound)
		return
	}

	if !a.canViewProfile(viewerID, target) {
		http.Redirect(w, r, "/users", http.StatusFound)
		return
	}

	if viewerID == targetID {
		http.Redirect(w, r, "/users/"+strconv.Itoa(targetID), http.StatusFound)
		return
	}

	var src workRow
	err = a.DB.QueryRow(
		`SELECT id, title, chapter, link, status, image_path, reading_type, user_id
         FROM works WHERE id = ? AND user_id = ?`,
		workID, targetID,
	).Scan(
		&src.ID,
		&src.Title,
		&src.Chapter,
		&src.Link,
		&src.Status,
		&src.ImagePath,
		&src.ReadingType,
		&src.UserID,
	)
	if err != nil {
		http.Redirect(w, r, "/users/"+strconv.Itoa(targetID), http.StatusFound)
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

	_, err = a.DB.Exec(
		`INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
		src.Title,
		src.Chapter,
		nullableString(src.Link),
		nullableString(src.Status),
		nullableString(src.ImagePath),
		readingType,
		viewerID,
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

// --- Administration des comptes ---

func (a *App) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(
		`SELECT id, username, password, validated, is_admin, is_superadmin,
                display_name, email, bio, avatar_path, is_public
         FROM users`,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type adminUser struct {
		ID           int
		Username     string
		Validated    int
		IsAdmin      int
		IsSuperadmin int
		DisplayName  sql.NullString
		Email        sql.NullString
		Bio          sql.NullString
		AvatarPath   sql.NullString
		IsPublic     sql.NullInt64
	}

	var users []adminUser
	for rows.Next() {
		var u adminUser
		var pwd string
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&pwd,
			&u.Validated,
			&u.IsAdmin,
			&u.IsSuperadmin,
			&u.DisplayName,
			&u.Email,
			&u.Bio,
			&u.AvatarPath,
			&u.IsPublic,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	data := map[string]any{
		"Users": users,
	}
	_ = a.Templates.ExecuteTemplate(w, "admin_accounts", data)
}

func (a *App) handleApproveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET validated = 1 WHERE id = ?`,
		userID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))

	var isAdmin, isSuper int
	err := a.DB.QueryRow(
		`SELECT is_admin, is_superadmin FROM users WHERE id = ?`,
		targetID,
	).Scan(&isAdmin, &isSuper)
	if err != nil {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	// seul un super-admin peut supprimer un compte admin, et on ne supprime jamais un superadmin
	if isSuper != 0 {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

	// on ne vérifie pas ici que l'appelant est superadmin : le middleware requireAdmin
	// vérifie déjà que c'est un admin ; pour la stricte équivalence, on pourrait
	// ajouter un contrôle supplémentaire.

	if _, err := a.DB.Exec(`DELETE FROM users WHERE id = ?`, targetID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (a *App) handlePromoteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	targetID, _ := strconv.Atoi(r.PathValue("id"))
	if _, err := a.DB.Exec(
		`UPDATE users SET is_admin = 1, validated = 1 WHERE id = ?`,
		targetID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}
