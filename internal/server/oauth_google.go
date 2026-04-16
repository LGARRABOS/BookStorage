package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bookstorage/internal/database"
	"bookstorage/internal/oauthgoogle"

	"golang.org/x/oauth2"
)

// googleOAuthExchangeHook is set by tests to stub the token exchange.
var googleOAuthExchangeHook func(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error)

func (a *App) googleOAuthEnabled() bool {
	return a.Settings != nil && a.Settings.GoogleOAuthConfigured()
}

func googleExchange(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
	if googleOAuthExchangeHook != nil {
		return googleOAuthExchangeHook(ctx, cfg, code, codeVerifier)
	}
	return oauthgoogle.Exchange(ctx, cfg, code, codeVerifier)
}

func sanitizeGoogleUsernamePart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "reader"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func (a *App) allocateGoogleUsername(email string) (string, error) {
	local := email
	if i := strings.LastIndex(email, "@"); i > 0 {
		local = email[:i]
	}
	base := sanitizeGoogleUsernamePart(local)
	for n := 0; n < 200; n++ {
		candidate := base
		if n > 0 {
			candidate = fmt.Sprintf("%s_%d", base, n)
		}
		var one int
		err := a.DB.QueryRow(`SELECT 1 FROM users WHERE username = ?`, candidate).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no available username for google signup")
}

// HandleGoogleOAuthStart begins the Google login/signup flow (GET /auth/google).
func (a *App) HandleGoogleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.googleOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	cfg := oauthgoogle.OAuth2Config(a.Settings)
	if cfg == nil {
		http.NotFound(w, r)
		return
	}

	database.DeleteExpiredOAuthStates(a.DB)

	statePlain, err := database.NewOAuthStatePlain()
	if err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	verifier, err := oauthgoogle.NewPKCEVerifier()
	if err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	next := safePostLoginRedirect(strings.TrimSpace(r.URL.Query().Get("next")))
	if err := database.InsertOAuthState(a.DB, statePlain, database.OAuthPurposeLogin, sql.NullInt64{Valid: false}, next, verifier); err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	http.Redirect(w, r, oauthgoogle.AuthCodeURL(cfg, statePlain, verifier), http.StatusFound)
}

// HandleGoogleOAuthLink begins linking Google to the logged-in account (GET /auth/google/link).
func (a *App) HandleGoogleOAuthLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.googleOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	cfg := oauthgoogle.OAuth2Config(a.Settings)
	if cfg == nil {
		http.NotFound(w, r)
		return
	}

	database.DeleteExpiredOAuthStates(a.DB)

	statePlain, err := database.NewOAuthStatePlain()
	if err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	verifier, err := oauthgoogle.NewPKCEVerifier()
	if err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	if err := database.InsertOAuthState(a.DB, statePlain, database.OAuthPurposeLink, sql.NullInt64{Int64: int64(userID), Valid: true}, "", verifier); err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	http.Redirect(w, r, oauthgoogle.AuthCodeURL(cfg, statePlain, verifier), http.StatusFound)
}

// HandleGoogleOAuthCallback completes OAuth (GET /auth/google/callback).
func (a *App) HandleGoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.googleOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	cfg := oauthgoogle.OAuth2Config(a.Settings)
	if cfg == nil {
		http.NotFound(w, r)
		return
	}

	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("code"))
	statePlain := strings.TrimSpace(q.Get("state"))
	if code == "" || statePlain == "" {
		http.Redirect(w, r, "/login?google_error=token", http.StatusFound)
		return
	}

	row, err := database.ConsumeOAuthState(a.DB, statePlain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Redirect(w, r, "/login?google_error=state", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	tok, err := googleExchange(ctx, cfg, code, row.CodeVerifier)
	if err != nil {
		http.Redirect(w, r, "/login?google_error=token", http.StatusFound)
		return
	}
	if tok == nil || tok.AccessToken == "" {
		http.Redirect(w, r, "/login?google_error=token", http.StatusFound)
		return
	}

	info, err := oauthgoogle.FetchUserInfo(ctx, tok.AccessToken)
	if err != nil {
		http.Redirect(w, r, "/login?google_error=token", http.StatusFound)
		return
	}

	switch row.Purpose {
	case database.OAuthPurposeLink:
		if !row.UserID.Valid {
			http.Redirect(w, r, "/login?google_error=state", http.StatusFound)
			return
		}
		uid := int(row.UserID.Int64)
		a.handleGoogleLinkCallback(w, r, uid, info.Sub, info.Email)
		return
	case database.OAuthPurposeLogin:
		a.handleGoogleLoginCallback(w, r, row.Next, info.Sub, info.Email)
		return
	default:
		http.Redirect(w, r, "/login?google_error=state", http.StatusFound)
	}
}

func (a *App) handleGoogleLinkCallback(w http.ResponseWriter, r *http.Request, userID int, googleSub, googleEmail string) {
	curID, ok := a.currentUserID(r)
	if !ok || curID != userID {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}

	var existingID int
	err := a.DB.QueryRow(`SELECT id FROM users WHERE google_sub = ? AND id != ?`, googleSub, userID).Scan(&existingID)
	if err == nil {
		http.Redirect(w, r, "/profile?google_error=link_taken", http.StatusFound)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}

	var selfSub sql.NullString
	if err := a.DB.QueryRow(`SELECT google_sub FROM users WHERE id = ?`, userID).Scan(&selfSub); err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	if selfSub.Valid && strings.TrimSpace(selfSub.String) != "" {
		if selfSub.String == googleSub {
			http.Redirect(w, r, "/profile?google_linked=1", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/profile?google_error=link_other", http.StatusFound)
		return
	}

	if _, err := a.DB.Exec(
		`UPDATE users SET google_sub = ?, google_email = ? WHERE id = ?`,
		googleSub, nullStringOrEmpty(googleEmail), userID,
	); err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?google_linked=1", http.StatusFound)
}

func nullStringOrEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func (a *App) handleGoogleLoginCallback(w http.ResponseWriter, r *http.Request, nextPath, googleSub, googleEmail string) {
	var u struct {
		id           int
		validated    int
		isAdmin      int
		isSuperadmin int
	}
	err := a.DB.QueryRow(
		`SELECT id, validated, is_admin, is_superadmin FROM users WHERE google_sub = ?`,
		googleSub,
	).Scan(&u.id, &u.validated, &u.isAdmin, &u.isSuperadmin)

	if err == nil {
		if (a.Settings == nil || a.Settings.RequireAccountValidation) && u.validated == 0 && u.isAdmin == 0 {
			http.Redirect(w, r, "/login?pending=1", http.StatusFound)
			return
		}
		token, err := a.createSession(r, u.id)
		if err != nil {
			http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
			return
		}
		a.setSessionCookie(w, token, sessionSlidingTTL)
		dest := safePostLoginRedirect(nextPath)
		if dest == "" {
			dest = "/dashboard"
		}
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}

	// New user from Google
	username, err := a.allocateGoogleUsername(googleEmail)
	if err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	validated := 0
	if a.Settings != nil && !a.Settings.RequireAccountValidation {
		validated = 1
	}
	res, err := a.DB.Exec(
		`INSERT INTO users (username, password, validated, is_admin, google_sub, google_email)
		 VALUES (?, NULL, ?, 0, ?, ?)`,
		username, validated, googleSub, nullStringOrEmpty(googleEmail),
	)
	if err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	newID, err := res.LastInsertId()
	if err != nil || newID <= 0 {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	if validated == 0 {
		http.Redirect(w, r, "/login?pending=1", http.StatusFound)
		return
	}
	token, err := a.createSession(r, int(newID))
	if err != nil {
		http.Redirect(w, r, "/login?google_error=server", http.StatusFound)
		return
	}
	a.setSessionCookie(w, token, sessionSlidingTTL)
	dest := safePostLoginRedirect(nextPath)
	if dest == "" {
		dest = "/dashboard"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// HandleGoogleUnlink removes the Google link when a local password exists (POST /profile/google/unlink).
func (a *App) HandleGoogleUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.googleOAuthEnabled() {
		http.NotFound(w, r)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}

	var pwd sql.NullString
	var googleSub sql.NullString
	if err := a.DB.QueryRow(`SELECT password, google_sub FROM users WHERE id = ?`, userID).Scan(&pwd, &googleSub); err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	if !googleSub.Valid || strings.TrimSpace(googleSub.String) == "" {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	if !pwd.Valid || strings.TrimSpace(pwd.String) == "" {
		http.Redirect(w, r, "/profile?google_error=unlink_need_password", http.StatusFound)
		return
	}

	if _, err := a.DB.Exec(`UPDATE users SET google_sub = NULL, google_email = NULL WHERE id = ?`, userID); err != nil {
		http.Redirect(w, r, "/profile?google_error=server", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?google_unlinked=1", http.StatusFound)
}
