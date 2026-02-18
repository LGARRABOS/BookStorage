package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/catalog"
	"bookstorage/internal/config"
	"bookstorage/internal/i18n"

	"golang.org/x/crypto/pbkdf2"
)

type App struct {
	Settings   *config.Settings
	SiteConfig *config.SiteConfig
	DB         *sql.DB
	Templates  *template.Template
}

func NewApp(settings *config.Settings, siteConfig *config.SiteConfig, db *sql.DB) *App {
	funcMap := template.FuncMap{
		"work_image_url": func(stored string) string {
			return workImageURL(settings, stored)
		},
		"url_for": func(name string, args ...string) string {
			switch name {
			case "static":
				if len(args) > 0 {
					filename := strings.TrimLeft(args[0], "/")
					return "/static/" + filename
				}
				return "/static/"
			default:
				return "/" + strings.TrimLeft(name, "/")
			}
		},
		// Generate a sequence of numbers (for star ratings)
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		// Comparisons for star ratings
		"le": func(a, b int) bool {
			return a <= b
		},
		"ge": func(a, b int) bool {
			return a >= b
		},
		// Math for stats
		"divf": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
		"mulf": func(a, b float64) float64 {
			return a * b
		},
		// Translation function
		"t": func(translations i18n.Translations, key string) string {
			if val, ok := translations[key]; ok {
				return val
			}
			return key
		},
		// Translate status (database stores French values)
		"translateStatus": func(status, lang string) string {
			if lang == i18n.LangEN {
				switch status {
				case "En cours":
					return "Reading"
				case "Terminé":
					return "Completed"
				case "En pause":
					return "On Hold"
				case "Abandonné":
					return "Dropped"
				case "À lire":
					return "Plan to Read"
				}
			}
			return status
		},
	}
	tpl := template.Must(
		template.New("").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.gohtml")),
	)
	return &App{
		Settings:   settings,
		SiteConfig: siteConfig,
		DB:         db,
		Templates:  tpl,
	}
}

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

func (a *App) currentLang(r *http.Request) string {
	c, err := r.Cookie("lang")
	if err != nil || (c.Value != i18n.LangFR && c.Value != i18n.LangEN) {
		return i18n.DefaultLang
	}
	return c.Value
}

func (a *App) setLang(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "lang",
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 year
		HttpOnly: true,
	})
}

func (a *App) handleSetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if lang != i18n.LangFR && lang != i18n.LangEN {
		lang = i18n.DefaultLang
	}
	a.setLang(w, lang)

	// Redirect back to referrer or dashboard
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/dashboard"
	}
	http.Redirect(w, r, referer, http.StatusFound)
}

// baseData returns common template data including translations
func (a *App) baseData(r *http.Request) map[string]any {
	lang := a.currentLang(r)
	return map[string]any{
		"Lang": lang,
		"T":    i18n.T(lang),
	}
}

// mergeData merges additional data into base data
func (a *App) mergeData(r *http.Request, extra map[string]any) map[string]any {
	data := a.baseData(r)
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func (a *App) setUserID(w http.ResponseWriter, userID int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    strconv.Itoa(userID),
		Path:     "/",
		MaxAge:   3600, // 1 hour session timeout
		HttpOnly: true,
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

func (a *App) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.currentUserID(r)
		if !ok {
			// Requêtes API : retourner 401 pour que le front puisse rediriger vers login
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"session_expired"}`))
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		// Refresh session on each request (sliding expiration)
		a.setUserID(w, userID)
		next(w, r)
	}
}

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

func workImageURL(s *config.Settings, storedPath string) string {
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
			// API: JSON 403, Pages: render 403
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			w.WriteHeader(http.StatusForbidden)
			_ = a.Templates.ExecuteTemplate(w, "403", a.mergeData(r, map[string]any{
				"RequestedPath": r.URL.Path,
			}))
			return
		}
		next(w, r)
	}
}

type responseRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header)}
}

func (rr *responseRecorder) Header() http.Header {
	return rr.header
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if rr.status == 0 {
		rr.status = http.StatusOK
	}
	return rr.body.Write(b)
}

func copyHeader(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

// writeErrorResponse sends HTML error page or JSON for API
func (a *App) writeErrorResponse(w http.ResponseWriter, r *http.Request, status int, templateName string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["RequestedPath"]; !ok {
		data["RequestedPath"] = r.URL.Path
	}
	data = a.mergeData(r, data)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		var payload string
		switch status {
		case http.StatusUnauthorized:
			payload = `{"error":"unauthorized"}`
		case http.StatusForbidden:
			payload = `{"error":"forbidden"}`
		case http.StatusNotFound:
			payload = `{"error":"not_found"}`
		case http.StatusMethodNotAllowed:
			payload = `{"error":"method_not_allowed"}`
		default:
			payload = `{"error":"internal_server_error"}`
		}
		w.Write([]byte(payload))
		return
	}
	w.WriteHeader(status)
	_ = a.Templates.ExecuteTemplate(w, templateName, data)
}

func (a *App) withErrorPages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := newResponseRecorder()
		var panicked bool
		func() {
			defer func() {
				if err := recover(); err != nil {
					panicked = true
					log.Printf("[panic] %s %s: %v", r.Method, r.URL.Path, err)
					a.writeErrorResponse(w, r, http.StatusInternalServerError, "500", nil)
				}
			}()
			next.ServeHTTP(rec, r)
		}()
		if panicked {
			return
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		switch status {
		case http.StatusUnauthorized:
			a.writeErrorResponse(w, r, status, "401", map[string]any{"RequestedPath": r.URL.Path})
			return
		case http.StatusForbidden:
			a.writeErrorResponse(w, r, status, "403", map[string]any{"RequestedPath": r.URL.Path})
			return
		case http.StatusNotFound:
			a.writeErrorResponse(w, r, status, "404", map[string]any{"RequestedPath": r.URL.Path})
			return
		case http.StatusMethodNotAllowed:
			a.writeErrorResponse(w, r, status, "405", map[string]any{"RequestedPath": r.URL.Path})
			return
		case http.StatusInternalServerError:
			a.writeErrorResponse(w, r, status, "500", nil)
			return
		}

		// Default: flush recorded response
		copyHeader(w.Header(), rec.Header())
		w.WriteHeader(status)
		_, _ = w.Write(rec.body.Bytes())
	})
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUserID(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	// Landing page for non-logged in visitors
	_ = a.Templates.ExecuteTemplate(w, "landing", a.baseData(r))
}

func (a *App) handleLegal(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data["Legal"] = a.SiteConfig.Legal
	data["SiteName"] = a.SiteConfig.SiteName
	data["SiteURL"] = a.SiteConfig.SiteURL
	_ = a.Templates.ExecuteTemplate(w, "legal", data)
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	// Statistiques globales
	var totalWorks int
	a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ?`, userID).Scan(&totalWorks)

	var totalChapters int
	a.DB.QueryRow(`SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?`, userID).Scan(&totalChapters)

	// Par statut
	type statusCount struct {
		Status string
		Count  int
	}
	var byStatus []statusCount
	rows, _ := a.DB.Query(`SELECT COALESCE(status, 'Non défini'), COUNT(*) FROM works WHERE user_id = ? GROUP BY status ORDER BY COUNT(*) DESC`, userID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var sc statusCount
			rows.Scan(&sc.Status, &sc.Count)
			byStatus = append(byStatus, sc)
		}
	}

	// Par type
	type typeCount struct {
		Type  string
		Count int
	}
	var byType []typeCount
	rows2, _ := a.DB.Query(`SELECT COALESCE(reading_type, 'Autre'), COUNT(*) FROM works WHERE user_id = ? GROUP BY reading_type ORDER BY COUNT(*) DESC`, userID)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var tc typeCount
			rows2.Scan(&tc.Type, &tc.Count)
			byType = append(byType, tc)
		}
	}

	// Moyenne des notes (seulement les œuvres notées)
	var avgRating float64
	var ratedCount int
	a.DB.QueryRow(`SELECT COALESCE(AVG(rating), 0), COUNT(*) FROM works WHERE user_id = ? AND rating > 0`, userID).Scan(&avgRating, &ratedCount)

	// Top 5 meilleures notes
	type ratedWork struct {
		Title  string
		Rating int
	}
	var topRated []ratedWork
	rows3, _ := a.DB.Query(`SELECT title, rating FROM works WHERE user_id = ? AND rating > 0 ORDER BY rating DESC, title LIMIT 5`, userID)
	if rows3 != nil {
		defer rows3.Close()
		for rows3.Next() {
			var rw ratedWork
			rows3.Scan(&rw.Title, &rw.Rating)
			topRated = append(topRated, rw)
		}
	}

	_ = a.Templates.ExecuteTemplate(w, "stats", a.mergeData(r, map[string]any{
		"TotalWorks":    totalWorks,
		"TotalChapters": totalChapters,
		"ByStatus":      byStatus,
		"ByType":        byType,
		"AvgRating":     avgRating,
		"RatedCount":    ratedCount,
		"TopRated":      topRated,
	}))
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Messages de feedback via query string
		q := r.URL.Query()
		data := a.mergeData(r, map[string]any{
			"RegisterErrorEmpty":  q.Get("error") == "empty",
			"RegisterErrorExists": q.Get("error") == "exists",
		})
		_ = a.Templates.ExecuteTemplate(w, "register", data)
	case http.MethodPost:
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/register?error=empty", http.StatusFound)
			return
		}

		_, err := a.DB.Exec(
			`INSERT INTO users (username, password, validated, is_admin)
             VALUES (?, ?, 0, 0)`,
			username, password, // TODO: hash du mot de passe
		)
		if err != nil {
			// conflit de username, etc.
			http.Redirect(w, r, "/register?error=exists", http.StatusFound)
			return
		}
		// Succès : compte créé, en attente de validation par le staff
		http.Redirect(w, r, "/login?registered=1", http.StatusFound)
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
		// Messages de feedback via query string
		q := r.URL.Query()
		data := a.mergeData(r, map[string]any{
			"LoginError":      q.Get("error") != "",
			"LoginPending":    q.Get("pending") != "",
			"RegisterSuccess": q.Get("registered") != "",
			"SessionExpired":  q.Get("expired") != "",
		})
		_ = a.Templates.ExecuteTemplate(w, "login", data)
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
			// Utilisateur introuvable
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}

		// Vérification du mot de passe (supporte Werkzeug et clair)
		if !verifyPassword(u.Password, password) {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}
		if u.Validated == 0 && u.IsAdmin == 0 {
			// Compte non encore validé par le staff
			http.Redirect(w, r, "/login?pending=1", http.StatusFound)
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
	http.Redirect(w, r, "/", http.StatusFound)
}

type workRow struct {
	ID          int
	Title       string
	Chapter     int
	Link        sql.NullString
	Status      sql.NullString
	ImagePath   sql.NullString
	ReadingType sql.NullString
	Rating      int
	Notes       sql.NullString
	UserID      int
	UpdatedAt   sql.NullString
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
	case "oldest":
		orderClause = "ORDER BY id ASC"
	case "modified":
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') DESC"
	default:
		sortBy = "title"
		orderClause = "ORDER BY LOWER(title)"
	}

	rows, err := a.DB.Query(
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, updated_at
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
			&wRow.Rating,
			&wRow.Notes,
			&wRow.UserID,
			&wRow.UpdatedAt,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	_ = a.Templates.ExecuteTemplate(w, "dashboard", a.mergeData(r, map[string]any{
		"Works":         works,
		"ReadingTypes":  readingTypes,
		"ReadingStatus": readingStatuses,
		"IsAdmin":       isAdmin == 1,
		"SortBy":        sortBy,
	}))
}

// Ajout d’une œuvre (avec support basique d’upload d’image)
type catalogSearchResult struct {
	Source       string `json:"source"`
	CatalogID    int64  `json:"catalog_id,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	Title        string `json:"title"`
	ReadingType  string `json:"reading_type"`
	ImageURL     string `json:"image_url,omitempty"`
}

func (a *App) handleCatalogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []catalogSearchResult{}})
		return
	}
	var results []catalogSearchResult
	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := a.DB.Query(
		`SELECT id, title, reading_type, COALESCE(image_url, '') FROM catalog WHERE LOWER(title) LIKE ? ORDER BY title LIMIT 15`,
		pattern,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var title, readingType, imageURL string
			if err := rows.Scan(&id, &title, &readingType, &imageURL); err != nil {
				continue
			}
			results = append(results, catalogSearchResult{
				Source:      "catalog",
				CatalogID:   id,
				Title:       title,
				ReadingType: readingType,
				ImageURL:    imageURL,
			})
		}
	}
	anilistResults, err := catalog.SearchAnilist(q, 10)
	if err == nil {
		for _, m := range anilistResults {
			results = append(results, catalogSearchResult{
				Source:      "anilist",
				ExternalID:  strconv.Itoa(m.ID),
				Title:       m.Title,
				ReadingType: m.ReadingType,
				ImageURL:    m.ImageURL,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (a *App) handleAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "add_work", a.mergeData(r, map[string]any{
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
		}))
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
		ratingStr := r.FormValue("rating")
		rating, _ := strconv.Atoi(ratingStr)
		if rating < 0 || rating > 5 {
			rating = 0
		}
		notes := strings.TrimSpace(r.FormValue("notes"))

		var catalogID sql.NullInt64
		if cidStr := r.FormValue("catalog_id"); cidStr != "" {
			if cid, _ := strconv.ParseInt(cidStr, 10, 64); cid > 0 {
				catalogID.Int64 = cid
				catalogID.Valid = true
			}
		}
		if !catalogID.Valid {
			source := r.FormValue("catalog_source")
			externalID := strings.TrimSpace(r.FormValue("catalog_external_id"))
			imgURL := strings.TrimSpace(r.FormValue("image_url"))
			if source == "anilist" && externalID != "" {
				var existingID int64
				err := a.DB.QueryRow(
					`SELECT id FROM catalog WHERE source = 'anilist' AND external_id = ? LIMIT 1`,
					externalID,
				).Scan(&existingID)
				if err == nil {
					catalogID.Int64 = existingID
					catalogID.Valid = true
				} else {
					res, err := a.DB.Exec(
						`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, 'anilist', ?)`,
						title, readingType, imgURL, externalID,
					)
					if err == nil {
						id, _ := res.LastInsertId()
						catalogID.Int64 = id
						catalogID.Valid = true
					}
				}
			} else {
				res, err := a.DB.Exec(
					`INSERT INTO catalog (title, reading_type, image_url, source) VALUES (?, ?, ?, 'manual')`,
					title, readingType, imgURL,
				)
				if err == nil {
					id, _ := res.LastInsertId()
					catalogID.Int64 = id
					catalogID.Valid = true
				}
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
		}

		var dbErr error
		if imagePath.Valid {
			_, dbErr = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, notes, user_id, catalog_id, updated_at)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				title, chapter, link, status, imagePath.String, readingType, rating, notes, userID, catalogID,
			)
		} else {
			_, dbErr = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, notes, user_id, catalog_id, updated_at)
                 VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				title, chapter, link, status, readingType, rating, notes, userID, catalogID,
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

func (a *App) handleEditWork(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	var work workRow
	err := a.DB.QueryRow(
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id
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
		&work.Rating,
		&work.Notes,
		&work.UserID,
	)
	if err != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		_ = a.Templates.ExecuteTemplate(w, "edit_work", a.mergeData(r, map[string]any{
			"Work":         work,
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
		}))
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
		ratingStr := r.FormValue("rating")
		rating, _ := strconv.Atoi(ratingStr)
		if rating < 0 || rating > 5 {
			rating = 0
		}
		notes := strings.TrimSpace(r.FormValue("notes"))

		// Gestion de l'image (optionnel)
		newImagePath := work.ImagePath

		// Check for image URL first
		imageURL := strings.TrimSpace(r.FormValue("image_url"))
		if imageURL != "" && (strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
			newImagePath.String = imageURL
			newImagePath.Valid = true
		} else {
			// If no URL, check for file upload
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
		}

		if newImagePath.Valid {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, image_path = ?, reading_type = ?, rating = ?, notes = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, newImagePath.String, readingType, rating, notes, workID, userID,
			)
		} else {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, reading_type = ?, rating = ?, notes = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, readingType, rating, notes, workID, userID,
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

func (a *App) handleIncrement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	_, err := a.DB.Exec(
		`UPDATE works SET chapter = chapter + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
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

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	rows, err := a.DB.Query(
		`SELECT title, chapter, link, status, reading_type, COALESCE(rating, 0), notes
         FROM works WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Set headers for CSV download
	filename := fmt.Sprintf("bookstorage_export_%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Write BOM for Excel UTF-8 compatibility
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	writer.Comma = ';' // Use semicolon for European Excel compatibility
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Title", "Chapter", "Link", "Status", "Type", "Rating", "Notes"})

	// Write data
	for rows.Next() {
		var title string
		var chapter int
		var link, status, readingType, notes sql.NullString
		var rating int

		if err := rows.Scan(&title, &chapter, &link, &status, &readingType, &rating, &notes); err != nil {
			continue
		}

		writer.Write([]string{
			title,
			strconv.Itoa(chapter),
			link.String,
			status.String,
			readingType.String,
			strconv.Itoa(rating),
			notes.String,
		})
	}
}

func (a *App) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	userID, _ := a.currentUserID(r)

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}

	file, _, err := r.FormFile("csv_file")
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';' // Use semicolon for European Excel compatibility
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}

	// Skip header row if present
	startIdx := 0
	if len(records) > 0 && strings.ToLower(records[0][0]) == "title" {
		startIdx = 1
	}

	imported := 0
	for i := startIdx; i < len(records); i++ {
		record := records[i]
		if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
			continue
		}

		title := strings.TrimSpace(record[0])
		chapter := 0
		link := ""
		status := "En cours"
		readingType := "Roman"
		rating := 0
		notes := ""

		if len(record) > 1 {
			chapter, _ = strconv.Atoi(record[1])
		}
		if len(record) > 2 {
			link = strings.TrimSpace(record[2])
		}
		if len(record) > 3 && strings.TrimSpace(record[3]) != "" {
			status = strings.TrimSpace(record[3])
		}
		if len(record) > 4 && strings.TrimSpace(record[4]) != "" {
			readingType = strings.TrimSpace(record[4])
		}
		if len(record) > 5 {
			rating, _ = strconv.Atoi(record[5])
			if rating < 0 || rating > 5 {
				rating = 0
			}
		}
		if len(record) > 6 {
			notes = strings.TrimSpace(record[6])
		}

		// Check if work already exists
		var existsID int
		a.DB.QueryRow(
			`SELECT id FROM works WHERE user_id = ? AND title = ?`,
			userID, title,
		).Scan(&existsID)

		if existsID == 0 {
			// Insert new work
			_, err := a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, reading_type, rating, notes, user_id, updated_at)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				title, chapter, link, status, readingType, rating, notes, userID,
			)
			if err == nil {
				imported++
			}
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard?imported=%d", imported), http.StatusFound)
}

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
		_ = a.Templates.ExecuteTemplate(w, "profile", a.mergeData(r, map[string]any{
			"User": u,
		}))
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

	_ = a.Templates.ExecuteTemplate(w, "users", a.mergeData(r, map[string]any{
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
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id
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
			&wRow.Rating,
			&wRow.Notes,
			&wRow.UserID,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	_ = a.Templates.ExecuteTemplate(w, "user_detail", a.mergeData(r, map[string]any{
		"TargetUser": u,
		"Works":      works,
		"CanImport":  viewerID != targetID,
	}))
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
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id
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
		&src.Rating,
		&src.Notes,
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
		`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, notes, user_id)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		src.Title,
		src.Chapter,
		nullableString(src.Link),
		nullableString(src.Status),
		nullableString(src.ImagePath),
		readingType,
		src.Rating,
		nullableString(src.Notes),
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

	_ = a.Templates.ExecuteTemplate(w, "admin_accounts", a.mergeData(r, map[string]any{
		"Users": users,
	}))
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

	if isSuper != 0 {
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}

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
