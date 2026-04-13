package server

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
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
	"bookstorage/internal/recommend"
	"bookstorage/internal/translate"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

type App struct {
	Settings        *config.Settings
	SiteConfig      *config.SiteConfig
	DB              *sql.DB
	TemplatesWeb    *template.Template
	TemplatesMobile *template.Template
	Version         string
	Monitor         *Monitoring
}

func NewApp(settings *config.Settings, siteConfig *config.SiteConfig, db *sql.DB, version string) *App {
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
		// Safe JavaScript string literal for <script> (avoid printf "%q" + html/template double-escaping).
		"jsstr": func(s string) template.JS {
			b, err := json.Marshal(s)
			if err != nil {
				return template.JS(`""`)
			}
			return template.JS(b)
		},
		// Translate status (database stores French values)
		"hasPrefix": strings.HasPrefix,
		"translateStatus": func(status string, t i18n.Translations) string {
			return i18n.TranslateStatus(status, t)
		},
		"upper": strings.ToUpper,
	}
	webTpl := mustLoadTemplates(funcMap, []string{
		filepath.Join("templates", "shared"),
		"templates",
		filepath.Join("templates", "web"),
	})
	mobileTpl := mustLoadTemplates(funcMap, []string{
		filepath.Join("templates", "shared"),
		"templates",
		filepath.Join("templates", "web"),
		filepath.Join("templates", "mobile"),
	})
	return &App{
		Settings:        settings,
		SiteConfig:      siteConfig,
		DB:              db,
		TemplatesWeb:    webTpl,
		TemplatesMobile: mobileTpl,
		Version:         strings.TrimSpace(version),
		Monitor:         NewMonitoring(strings.TrimSpace(version), strings.TrimSpace(settings.Environment)),
	}
}

func mustLoadTemplates(funcMap template.FuncMap, directories []string) *template.Template {
	files := collectTemplateFiles(directories...)
	if len(files) == 0 {
		log.Fatal("no templates found")
	}
	tpl, err := template.New("").Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}
	return tpl
}

func collectTemplateFiles(dirs ...string) []string {
	var files []string
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		rootIsTemplates := filepath.Clean(dir) == filepath.Clean("templates")
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if rootIsTemplates && filepath.Clean(p) != filepath.Clean(dir) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(d.Name()) != ".gohtml" {
				return nil
			}
			clean := filepath.Clean(p)
			if _, ok := seen[clean]; ok {
				return nil
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
			return nil
		})
	}
	return files
}

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyPassword checks a password against a hash.
// Supporte bcrypt ($2a$, $2b$, $2y$), Werkzeug pbkdf2:sha256:iterations$salt$hash,
// et la comparaison en clair (legacy).
func verifyPassword(storedHash, password string) bool {
	if strings.HasPrefix(storedHash, "$2a$") || strings.HasPrefix(storedHash, "$2b$") || strings.HasPrefix(storedHash, "$2y$") {
		err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
		return err == nil
	}
	if strings.HasPrefix(storedHash, "pbkdf2:") {
		return verifyWerkzeugHash(storedHash, password)
	}
	return storedHash == password
}

// verifyWerkzeugHash checks a password against a Werkzeug hash.
// Format: pbkdf2:sha256:iterations$salt$hash
func verifyWerkzeugHash(storedHash, password string) bool {
	// Split method$salt$hash
	parts := strings.SplitN(storedHash, "$", 3)
	if len(parts) != 3 {
		return false
	}

	method := parts[0]  // pbkdf2:sha256:iterations
	salt := parts[1]    // salt en clair
	hashHex := parts[2] // hex-encoded hash

	// Extract method parameters
	methodParts := strings.Split(method, ":")
	if len(methodParts) < 3 || methodParts[0] != "pbkdf2" || methodParts[1] != "sha256" {
		return false // Unsupported format
	}

	iterations := 260000 // Werkzeug default
	if len(methodParts) >= 3 {
		if n, err := strconv.Atoi(methodParts[2]); err == nil {
			iterations = n
		}
	}

	// Decode expected hash
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}

	// Calculer le hash PBKDF2
	computed := pbkdf2.Key([]byte(password), []byte(salt), iterations, len(expectedHash), sha256.New)

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1
}

func (a *App) currentUserID(r *http.Request) (int, bool) {
	id, _, ok := a.currentSession(r)
	return id, ok
}

func (a *App) currentLang(r *http.Request) string {
	c, err := r.Cookie("lang")
	if err != nil || !i18n.ValidLang(c.Value) {
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
		SameSite: sessionSameSite(a.Settings.Environment),
	})
}

func (a *App) HandleSetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !i18n.ValidLang(lang) {
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
	mode := a.viewModeFromRequest(r)
	// Keep base data consistent with resolveViewMode() safety guards, but without mutating cookies.
	if mode != "" {
		m := isMobileRequest(r)
		if (mode == "web" && m) || (mode == "mobile" && !m) {
			mode = "auto"
		}
	}
	isMobile := mode == "mobile" || (mode == "auto" && isMobileRequest(r))
	currentPath := ""
	if r != nil && r.URL != nil {
		currentPath = r.URL.Path
	}
	return map[string]any{
		"Lang":         lang,
		"T":            i18n.T(lang),
		"Languages":    i18n.Languages(),
		"ViewMode":     mode,
		"IsMobileView": isMobile,
		"CurrentPath":  currentPath,
		"AppVersion":   a.Version,
	}
}

// MobileRedirectToDashboard redirects to /dashboard when in mobile mode.
// Used for pages that are not part of the simplified mobile experience.
func (a *App) MobileRedirectToDashboard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.resolveViewMode(w, r) == "mobile" {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// RequireWebOnly blocks requests when resolved view mode is mobile.
// This is used for admin pages that must not be available in the mobile web UI.
func (a *App) RequireWebOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.resolveViewMode(w, r) == "mobile" {
			// API: JSON 403, Pages: render 403
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			w.WriteHeader(http.StatusForbidden)
			a.renderTemplate(w, r, "403", a.mergeData(r, map[string]any{
				"RequestedPath": r.URL.Path,
			}))
			return
		}
		next(w, r)
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

func (a *App) renderTemplate(w http.ResponseWriter, r *http.Request, templateName string, data map[string]any) {
	mode := a.resolveViewMode(w, r)
	if _, ok := data["ViewMode"]; !ok {
		data["ViewMode"] = mode
	}
	if _, ok := data["IsMobileView"]; !ok {
		data["IsMobileView"] = mode == "mobile"
	}
	if mode == "mobile" {
		mobileName := "mobile_" + templateName
		if err := a.TemplatesMobile.ExecuteTemplate(w, mobileName, data); err == nil {
			return
		}
	}
	_ = a.TemplatesWeb.ExecuteTemplate(w, templateName, data)
}

func (a *App) viewModeFromRequest(r *http.Request) string {
	if r == nil {
		return "auto"
	}
	if c, err := r.Cookie("view_mode"); err == nil {
		mode := strings.ToLower(strings.TrimSpace(c.Value))
		if mode == "auto" || mode == "mobile" || mode == "web" {
			return mode
		}
	}
	return "auto"
}

func (a *App) resolveViewMode(w http.ResponseWriter, r *http.Request) string {
	mode := a.viewModeFromRequest(r)
	override := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	if override == "auto" || override == "mobile" || override == "web" {
		cookie := &http.Cookie{
			Name:     "view_mode",
			Value:    override,
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: sessionSameSite(a.Settings.Environment),
		}
		if strings.ToLower(a.Settings.Environment) == "production" {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		mode = override
	}
	// Safety guards: avoid stale forced mode when device context changed.
	// - mobile phone with stale "web" cookie
	// - desktop browser with stale "mobile" cookie
	if override == "" {
		isMobile := isMobileRequest(r)
		if (mode == "web" && isMobile) || (mode == "mobile" && !isMobile) {
			mode = "auto"
		}
	}
	if mode == "mobile" {
		return "mobile"
	}
	if mode == "web" {
		return "web"
	}
	if isMobileRequest(r) {
		return "mobile"
	}
	return "web"
}

func isMobileRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Sec-CH-UA-Mobile")) == "?1" {
		return true
	}
	ua := strings.ToLower(r.UserAgent())
	markers := []string{
		"android", "iphone", "ipad", "ipod",
		"mobile", "blackberry", "opera mini", "windows phone",
	}
	for _, marker := range markers {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}

func (a *App) RequireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, token, ok := a.currentSession(r)
		if !ok {
			// API requests: return 401 so the frontend can redirect to login
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"session_expired"}`))
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		// Sliding expiration (DB + cookie)
		a.touchSession(r, token)
		a.setSessionCookie(w, token, sessionSlidingTTL)
		next(w, r)
	}
}

var readingTypes = []string{
	"Roman",
	"Manga",
	"BD",
	"Light Novel",
	"Webtoon",
	"Manhwa",
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

	// Uploaded images are in static/images/, prefix with /static/
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

func (a *App) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
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
				_, _ = w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			w.WriteHeader(http.StatusForbidden)
			a.renderTemplate(w, r, "403", a.mergeData(r, map[string]any{
				"RequestedPath": r.URL.Path,
			}))
			return
		}

		// Prevent stale admin pages/status after a self-update / restart.
		// Applies to admin HTML and admin APIs (polling endpoints).
		if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/api/admin") {
			w.Header().Set("Cache-Control", "no-store")
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
		_, _ = w.Write([]byte(payload))
		return
	}
	w.WriteHeader(status)
	a.renderTemplate(w, r, templateName, data)
}

func (a *App) WithErrorPages(next http.Handler) http.Handler {
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

func (a *App) HandleHome(w http.ResponseWriter, r *http.Request) {
	// The "/" pattern in http.ServeMux matches any path not matched by a more specific route.
	// Only serve the home / landing for the exact root path; otherwise return 404.
	if p := path.Clean(r.URL.Path); p != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, ok := a.currentUserID(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	// Landing page for non-logged in visitors
	a.renderTemplate(w, r, "landing", a.baseData(r))
}

func (a *App) HandleLegal(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data["Legal"] = a.SiteConfig.Legal
	data["SiteName"] = a.SiteConfig.SiteName
	data["SiteURL"] = a.SiteConfig.SiteURL
	a.renderTemplate(w, r, "legal", data)
}

func (a *App) HandleStats(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	// Statistiques globales
	var totalWorks int
	if err := a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ?`, userID).Scan(&totalWorks); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var totalChapters int
	if err := a.DB.QueryRow(`SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?`, userID).Scan(&totalChapters); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// By status
	type statusCount struct {
		Status string
		Count  int
	}
	var byStatus []statusCount
	rows, err := a.DB.Query(`SELECT COALESCE(status, 'Non défini'), COUNT(*) FROM works WHERE user_id = ? GROUP BY status ORDER BY COUNT(*) DESC`, userID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sc statusCount
			if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
				continue
			}
			byStatus = append(byStatus, sc)
		}
	}

	// By type
	type typeCount struct {
		Type  string
		Count int
	}
	var byType []typeCount
	rows2, err := a.DB.Query(`SELECT COALESCE(reading_type, 'Autre'), COUNT(*) FROM works WHERE user_id = ? GROUP BY reading_type ORDER BY COUNT(*) DESC`, userID)
	if err == nil {
		defer func() { _ = rows2.Close() }()
		for rows2.Next() {
			var tc typeCount
			if err := rows2.Scan(&tc.Type, &tc.Count); err != nil {
				continue
			}
			byType = append(byType, tc)
		}
	}

	// Average rating (only rated works)
	var avgRating float64
	var ratedCount int
	if err := a.DB.QueryRow(`SELECT COALESCE(AVG(rating), 0), COUNT(*) FROM works WHERE user_id = ? AND rating > 0`, userID).Scan(&avgRating, &ratedCount); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Top 5 meilleures notes
	type ratedWork struct {
		Title  string
		Rating int
	}
	var topRated []ratedWork
	rows3, err := a.DB.Query(`SELECT title, rating FROM works WHERE user_id = ? AND rating > 0 ORDER BY rating DESC, title LIMIT 5`, userID)
	if err == nil {
		defer func() { _ = rows3.Close() }()
		for rows3.Next() {
			var rw ratedWork
			if err := rows3.Scan(&rw.Title, &rw.Rating); err != nil {
				continue
			}
			topRated = append(topRated, rw)
		}
	}

	a.renderTemplate(w, r, "stats", a.mergeData(r, map[string]any{
		"TotalWorks":    totalWorks,
		"TotalChapters": totalChapters,
		"ByStatus":      byStatus,
		"ByType":        byType,
		"AvgRating":     avgRating,
		"RatedCount":    ratedCount,
		"TopRated":      topRated,
	}))
}

func (a *App) HandleRegister(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Messages de feedback via query string
		q := r.URL.Query()
		data := a.mergeData(r, map[string]any{
			"RegisterErrorEmpty":  q.Get("error") == "empty",
			"RegisterErrorExists": q.Get("error") == "exists",
		})
		a.renderTemplate(w, r, "register", data)
	case http.MethodPost:
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/register?error=empty", http.StatusFound)
			return
		}

		hashedPassword, err := hashPassword(password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		validated := 0
		if a.Settings != nil && !a.Settings.RequireAccountValidation {
			validated = 1
		}
		_, err = a.DB.Exec(
			`INSERT INTO users (username, password, validated, is_admin)
             VALUES (?, ?, ?, 0)`,
			username, hashedPassword, validated,
		)
		if err != nil {
			// conflit de username, etc.
			http.Redirect(w, r, "/register?error=exists", http.StatusFound)
			return
		}
		// Success: account created.
		if validated == 1 {
			http.Redirect(w, r, "/login?registered=1&auto=1", http.StatusFound)
			return
		}
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

func (a *App) HandleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Messages de feedback via query string
		q := r.URL.Query()
		data := a.mergeData(r, map[string]any{
			"LoginError":      q.Get("error") != "",
			"LoginPending":    q.Get("pending") != "",
			"RegisterSuccess": q.Get("registered") != "",
			"RegisterAuto":    q.Get("auto") == "1",
			"SessionExpired":  q.Get("expired") != "",
		})
		a.renderTemplate(w, r, "login", data)
	case http.MethodPost:
		username := strings.TrimSpace(r.FormValue("username"))
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

		// Verify password (supports Werkzeug and plaintext)
		if !verifyPassword(u.Password, password) {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}
		if (a.Settings == nil || a.Settings.RequireAccountValidation) && u.Validated == 0 && u.IsAdmin == 0 {
			// Account not yet validated by staff
			http.Redirect(w, r, "/login?pending=1", http.StatusFound)
			return
		}

		token, err := a.createSession(r, u.ID)
		if err != nil {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}
		a.setSessionCookie(w, token, sessionSlidingTTL)
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if _, tok, ok := a.currentSession(r); ok {
		a.revokeSession(tok)
	}
	a.clearSession(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *App) HandleLogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login?expired=1", http.StatusFound)
		return
	}
	a.revokeAllUserSessions(userID)
	a.clearSession(w)
	http.Redirect(w, r, "/profile?logout_all=1", http.StatusFound)
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
	IsAdult     sql.NullInt64
}

func (a *App) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	// Check if user is admin
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
	case "modified", "modified_desc":
		// Alias "modified" kept for backward compatibility
		sortBy = "modified_desc"
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') DESC"
	case "modified_asc":
		orderClause = "ORDER BY COALESCE(updated_at, '1970-01-01') ASC"
	default:
		sortBy = "title"
	}

	// Filtre adulte
	adultFilter := r.URL.Query().Get("adult")
	whereClause := "WHERE user_id = ?"
	args := []any{userID}
	switch adultFilter {
	case "only":
		whereClause += " AND COALESCE(is_adult, 0) = 1"
	default:
		adultFilter = ""
		whereClause += " AND COALESCE(is_adult, 0) = 0"
	}

	query := `
        SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, updated_at, COALESCE(is_adult, 0)
        FROM works ` + whereClause + " " + orderClause

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

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
			&wRow.IsAdult,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		works = append(works, wRow)
	}

	data := map[string]any{
		"Works":         works,
		"ReadingTypes":  readingTypes,
		"ReadingStatus": readingStatuses,
		"IsAdmin":       isAdmin == 1,
		"SortBy":        sortBy,
		"AdultFilter":   adultFilter,
		"SearchQuery":   r.URL.Query().Get("q"),
	}
	if enc := r.URL.Query().Get("import_report"); enc != "" {
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err == nil {
			var rep ImportReport
			if json.Unmarshal(raw, &rep) == nil {
				data["ImportReport"] = rep
			}
		}
	}
	if r.URL.Query().Get("error") == "import" {
		data["ImportError"] = true
	}
	a.renderTemplate(w, r, "dashboard", a.mergeData(r, data))
}

// Add a work (with basic image upload support)
type catalogSearchResult struct {
	Source      string `json:"source"`
	CatalogID   int64  `json:"catalog_id,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	Title       string `json:"title"`
	ReadingType string `json:"reading_type"`
	ImageURL    string `json:"image_url,omitempty"`
	IsAdult     bool   `json:"is_adult"`
}

func (a *App) HandleCatalogSearch(w http.ResponseWriter, r *http.Request) {
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
		defer func() { _ = rows.Close() }()
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
				IsAdult:     false,
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
				IsAdult:     m.IsAdult,
			})
		}
	}
	mangadexResults, err := catalog.SearchMangadex(q, 10)
	if err == nil {
		for _, m := range mangadexResults {
			results = append(results, catalogSearchResult{
				Source:      "mangadex",
				ExternalID:  m.ID,
				Title:       m.Title,
				ReadingType: m.ReadingType,
				ImageURL:    m.ImageURL,
				IsAdult:     m.IsAdult,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

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
	res, err := recommend.ForUser(a.DB, int64(userID), recommend.DefaultOptions())
	if err != nil {
		log.Printf("recommendations: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "upstream", "results": []any{}, "profile": map[string]any{}})
		return
	}
	if res == nil {
		res = &recommend.ForUserResult{
			Results: []recommend.Suggestion{},
			Profile: recommend.ProfileSummary{},
		}
	}

	if dismissed, err := loadDismissedRecommendations(a.DB, userID, "anilist"); err == nil {
		filterDismissedSuggestions(res, dismissed)
	} else {
		log.Printf("dismissed recommendations: %v", err)
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
		log.Printf("recommendation media: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "upstream"})
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

func (a *App) HandleAddWork(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data := map[string]any{
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
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
			} else if source == "mangadex" && externalID != "" {
				var existingID int64
				err := a.DB.QueryRow(
					`SELECT id FROM catalog WHERE source = 'mangadex' AND external_id = ? LIMIT 1`,
					externalID,
				).Scan(&existingID)
				if err == nil {
					catalogID.Int64 = existingID
					catalogID.Valid = true
				} else {
					res, err := a.DB.Exec(
						`INSERT INTO catalog (title, reading_type, image_url, source, external_id) VALUES (?, ?, ?, 'mangadex', ?)`,
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
				defer func() { _ = file.Close() }()
				if allowedFile(header.Filename) {
					filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
					full := filepath.Join(a.Settings.UploadFolder, filename)
					dst, err := os.Create(full)
					if err == nil {
						defer func() { _ = dst.Close() }()
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
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, is_adult, notes, user_id, catalog_id, updated_at)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				title, chapter, link, status, imagePath.String, readingType, rating, isAdult, notes, userID, catalogID,
			)
		} else {
			_, dbErr = a.DB.Exec(
				`INSERT INTO works (title, chapter, link, status, image_path, reading_type, rating, is_adult, notes, user_id, catalog_id, updated_at)
                 VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				title, chapter, link, status, readingType, rating, isAdult, notes, userID, catalogID,
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
	err := a.DB.QueryRow(
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id, COALESCE(is_adult, 0)
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
		&work.IsAdult,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("format") == "partial" {
			a.renderTemplate(w, r, "edit_work_modal", a.mergeData(r, map[string]any{
				"Work":         work,
				"ReadingTypes": readingTypes,
				"Statuses":     readingStatuses,
				"IsModal":      true,
			}))
			return
		}
		a.renderTemplate(w, r, "edit_work", a.mergeData(r, map[string]any{
			"Work":         work,
			"ReadingTypes": readingTypes,
			"Statuses":     readingStatuses,
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
				defer func() { _ = file.Close() }()
				if allowedFile(header.Filename) {
					filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
					full := filepath.Join(a.Settings.UploadFolder, filename)
					dst, err := os.Create(full)
					if err == nil {
						defer func() { _ = dst.Close() }()
						_, _ = io.Copy(dst, file)
						newImagePath.String = buildMediaRelativePath(filename, a.Settings.UploadURLPath)
						newImagePath.Valid = true
					}
				}
			}
		}

		if newImagePath.Valid {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, image_path = ?, reading_type = ?, rating = ?, is_adult = ?, notes = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, newImagePath.String, readingType, rating, isAdult, notes, workID, userID,
			)
		} else {
			_, err = a.DB.Exec(
				`UPDATE works SET title = ?, chapter = ?, link = ?, status = ?, reading_type = ?, rating = ?, is_adult = ?, notes = ?, updated_at = CURRENT_TIMESTAMP
                 WHERE id = ? AND user_id = ?`,
				title, chapter, link, status, readingType, rating, isAdult, notes, workID, userID,
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

	_, err = a.DB.Exec(
		`UPDATE works SET chapter = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		chapter, workID, userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) HandleDeleteWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, _ := a.currentUserID(r)
	workID, _ := strconv.Atoi(r.PathValue("id"))

	result, err := a.DB.Exec(`DELETE FROM works WHERE id = ? AND user_id = ?`, workID, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (a *App) HandleExport(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.currentUserID(r)

	rows, err := a.DB.Query(
		`SELECT title, chapter, link, status, reading_type, COALESCE(rating, 0), notes, COALESCE(updated_at, ''),
                catalog_id, COALESCE(is_adult, 0), COALESCE(image_path, '')
         FROM works WHERE user_id = ? ORDER BY title`,
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { _ = rows.Close() }()

	var works []exportWork
	for rows.Next() {
		var w exportWork
		var link, status, readingType, notes, imagePath sql.NullString
		var catalogID sql.NullInt64
		var isAdult int
		if err := rows.Scan(&w.Title, &w.Chapter, &link, &status, &readingType, &w.Rating, &notes, &w.UpdatedAt, &catalogID, &isAdult, &imagePath); err != nil {
			continue
		}
		if link.Valid {
			w.Link = link.String
		}
		if status.Valid {
			w.Status = status.String
		}
		if readingType.Valid {
			w.ReadingType = readingType.String
		}
		if notes.Valid {
			w.Notes = notes.String
		}
		if catalogID.Valid && catalogID.Int64 > 0 {
			cid := int(catalogID.Int64)
			w.CatalogID = &cid
		}
		w.IsAdult = isAdult != 0
		if imagePath.Valid {
			w.ImagePath = imagePath.String
		}
		works = append(works, w)
	}

	dateStr := time.Now().Format("2006-01-02")
	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"bookstorage_export_%s.json\"", dateStr))
		payload := map[string]any{
			"export_version": ExportFormatVersion,
			"works":          works,
			"exported_at":    time.Now().Format(time.RFC3339),
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return
	}

	// CSV
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"bookstorage_export_%s.csv\"", dateStr))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	writer.Comma = ';'
	defer writer.Flush()
	_ = writer.Write([]string{"Title", "Chapter", "Link", "Status", "Type", "Rating", "Notes", "CatalogID", "IsAdult", "ImagePath"})
	for _, row := range works {
		cat := ""
		if row.CatalogID != nil {
			cat = strconv.Itoa(*row.CatalogID)
		}
		adult := "0"
		if row.IsAdult {
			adult = "1"
		}
		_ = writer.Write([]string{
			row.Title,
			strconv.Itoa(row.Chapter),
			row.Link,
			row.Status,
			row.ReadingType,
			strconv.Itoa(row.Rating),
			row.Notes,
			cat,
			adult,
			row.ImagePath,
		})
	}
}

// HandleImport accepts CSV or JSON (file upload or JSON body).
func (a *App) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	userID, _ := a.currentUserID(r)
	mode := parseDuplicateMode(r.URL.Query().Get("duplicate_mode"))

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
			return
		}
		a.ImportFromJSONBytes(w, r, userID, body, mode)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	mode = parseDuplicateMode(r.FormValue("duplicate_mode"))

	var file multipart.File
	var filename string
	if f, h, err := r.FormFile("import_file"); err == nil {
		file, filename = f, h.Filename
	} else if f, h, err := r.FormFile("csv_file"); err == nil {
		file, filename = f, h.Filename
	} else if f, h, err := r.FormFile("json_file"); err == nil {
		file, filename = f, h.Filename
	} else {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	trim := strings.TrimSpace(string(data))
	isJSON := strings.HasSuffix(strings.ToLower(filename), ".json") ||
		strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[")
	if isJSON {
		a.ImportFromJSONBytes(w, r, userID, data, mode)
		return
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = ';'
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=import", http.StatusFound)
		return
	}
	a.ImportFromCSVRecords(w, r, userID, records, mode)
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

func (a *App) HandleProfile(w http.ResponseWriter, r *http.Request) {
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
		// Profile stats
		var totalWorks int
		_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ?`, userID).Scan(&totalWorks)
		var totalChapters int
		_ = a.DB.QueryRow(`SELECT COALESCE(SUM(chapter), 0) FROM works WHERE user_id = ?`, userID).Scan(&totalChapters)
		var completedCount int
		_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ? AND (status = 'Terminé' OR status = 'Completed')`, userID).Scan(&completedCount)
		var readingCount int
		_ = a.DB.QueryRow(`SELECT COUNT(*) FROM works WHERE user_id = ? AND (status = 'En cours' OR status = 'Reading')`, userID).Scan(&readingCount)

		sessions, _ := a.listActiveSessions(userID)
		_, tok, _ := a.currentSession(r)
		currentSessionHash := ""
		if tok != "" {
			currentSessionHash = hashSessionToken(tok)
		}
		a.renderTemplate(w, r, "profile", a.mergeData(r, map[string]any{
			"User":           u,
			"TotalWorks":     totalWorks,
			"TotalChapters":  totalChapters,
			"CompletedCount": completedCount,
			"ReadingCount":   readingCount,
			"Sessions":       sessions,
			"CurrentSession": currentSessionHash,
			"LogoutAllDone":  r.URL.Query().Get("logout_all") == "1",
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
			// NOTE: storing in plaintext to stay consistent with the rest of the Go app.
			hashedPassword, err := hashPassword(newPassword)
			if err != nil {
				http.Redirect(w, r, "/profile", http.StatusFound)
				return
			}
			updates["password"] = hashedPassword
		}

		if requirePasswordCheck {
			if currentPassword == "" || !verifyPassword(u.Password, currentPassword) {
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
			defer func() { _ = file.Close() }()
			if allowedFile(header.Filename) {
				filename := strconv.FormatInt(int64(userID), 10) + "_" + path.Base(header.Filename)
				full := filepath.Join(a.Settings.ProfileUploadFolder, filename)
				dst, err := os.Create(full)
				if err == nil {
					defer func() { _ = dst.Close() }()
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

func (a *App) HandleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	currentPassword := r.FormValue("current_password")
	confirmText := strings.TrimSpace(r.FormValue("confirm_delete"))
	if currentPassword == "" || strings.ToUpper(confirmText) != "SUPPRIMER" {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}

	var storedPassword string
	var avatarPath sql.NullString
	err := a.DB.QueryRow(
		`SELECT password, avatar_path FROM users WHERE id = ?`,
		userID,
	).Scan(&storedPassword, &avatarPath)
	if err != nil || !verifyPassword(storedPassword, currentPassword) {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}

	type imgPathRow struct {
		imagePath sql.NullString
	}
	var workImagePaths []string
	rows, err := a.DB.Query(`SELECT image_path FROM works WHERE user_id = ?`, userID)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var p imgPathRow
			if rows.Scan(&p.imagePath) == nil && p.imagePath.Valid {
				workImagePaths = append(workImagePaths, p.imagePath.String)
			}
		}
	}

	tx, err := a.DB.Begin()
	if err != nil {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if _, err := tx.Exec(`DELETE FROM works WHERE user_id = ?`, userID); err != nil {
		_ = tx.Rollback()
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		_ = tx.Rollback()
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Redirect(w, r, "/profile?delete_error=1", http.StatusFound)
		return
	}

	if avatarPath.Valid {
		deleteMediaFile(a.Settings.ProfileUploadFolder, avatarPath.String)
	}
	for _, p := range workImagePaths {
		deleteMediaFile(a.Settings.UploadFolder, p)
	}

	a.clearSession(w)
	http.Redirect(w, r, "/?account_deleted=1", http.StatusFound)
}

func (a *App) HandleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data := map[string]any{}
	if enc := r.URL.Query().Get("import_report"); enc != "" {
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err == nil {
			var rep ImportReport
			if json.Unmarshal(raw, &rep) == nil {
				data["ImportReport"] = rep
			}
		}
	}
	if r.URL.Query().Get("error") == "import" {
		data["ImportError"] = true
	}
	a.renderTemplate(w, r, "tools", a.mergeData(r, data))
}

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
		`SELECT id, title, chapter, link, status, image_path, reading_type, COALESCE(rating, 0), notes, user_id
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
