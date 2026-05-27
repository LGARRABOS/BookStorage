package server

import (
	"bookstorage/internal/config"
	"bookstorage/internal/i18n"
	"bytes"
	"log"
	"net/http"
	"strings"
)

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

	http.Redirect(w, r, safeLanguageRedirect(r, "/dashboard"), http.StatusFound)
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
	googleOAuth := a.Settings != nil && a.Settings.GoogleOAuthConfigured()
	return map[string]any{
		"Lang":               lang,
		"T":                  i18n.T(lang),
		"Languages":          i18n.Languages(),
		"ViewMode":           mode,
		"IsMobileView":       isMobile,
		"CurrentPath":        currentPath,
		"AppVersion":         a.Version,
		"GoogleOAuthEnabled": googleOAuth,
		"CSPNonce":           cspNonceFromContext(r.Context()),
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
	// Admin nav: SQLite → PostgreSQL tab (superadmin, not already on Postgres).
	if r != nil && r.URL != nil && strings.HasPrefix(r.URL.Path, "/admin/") {
		if _, ok := data["ShowPostgresMigrate"]; !ok {
			data["ShowPostgresMigrate"] = a.showPostgresMigrateTab(r)
		}
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
			http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
			return
		}
		// Sliding expiration (DB + cookie)
		a.touchSession(r, token)
		a.setSessionCookie(w, token, sessionSlidingTTL)
		next(w, r)
	}
}

var readingTypes = []string{
	"Manga",
	"Webtoon",
	"Light Novel",
}

var readingStatuses = []string{
	"En cours",
	"Terminé",
	"En pause",
	"Abandonné",
	"À lire",
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
			http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
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

// RequireSuperadmin allows only users with is_superadmin set (in addition to admin).
func (a *App) RequireSuperadmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.currentUserID(r)
		if !ok {
			http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
			return
		}
		var isAdmin, isSuper int
		err := a.DB.QueryRow(
			`SELECT is_admin, is_superadmin FROM users WHERE id = ?`,
			userID,
		).Scan(&isAdmin, &isSuper)
		if err != nil || isAdmin == 0 || isSuper == 0 {
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
