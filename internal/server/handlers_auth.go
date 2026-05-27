package server

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) HandleRegister(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Messages de feedback via query string
		q := r.URL.Query()
		data := a.mergeData(r, map[string]any{
			"RegisterErrorEmpty":  q.Get("error") == "empty",
			"RegisterErrorExists": q.Get("error") == "exists",
			"RegisterErrorWeak":   q.Get("error") == "weak",
			"RegisterErrorEmail":  q.Get("error") == "email",
		})
		a.renderTemplate(w, r, "register", data)
	case http.MethodPost:
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")

		if username == "" || password == "" || email == "" {
			http.Redirect(w, r, "/register?error=empty", http.StatusFound)
			return
		}
		if !validAccountEmail(email) {
			http.Redirect(w, r, "/register?error=email", http.StatusFound)
			return
		}
		if len(password) < minPasswordLen {
			http.Redirect(w, r, "/register?error=weak", http.StatusFound)
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
			`INSERT INTO users (username, password, validated, is_admin, email)
             VALUES (?, ?, ?, 0, ?)`,
			username, hashedPassword, validated, normalizeAccountEmail(email),
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
	Password     sql.NullString
	GoogleSub    sql.NullString
	Validated    int
	IsAdmin      int
	IsSuperadmin int
}

func (a *App) HandleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Messages de feedback via query string
		q := r.URL.Query()
		loginNext := safePostLoginRedirect(q.Get("next"))
		googleAuthURL := "/auth/google"
		if loginNext != "" {
			googleAuthURL = "/auth/google?next=" + url.QueryEscape(loginNext)
		}
		data := a.mergeData(r, map[string]any{
			"LoginError":       q.Get("error") != "",
			"LoginPending":     q.Get("pending") != "",
			"RegisterSuccess":  q.Get("registered") != "",
			"RegisterAuto":     q.Get("auto") == "1",
			"SessionExpired":   q.Get("expired") != "",
			"PasswordResetOK":  q.Get("reset") != "",
			"LoginNext":        loginNext,
			"GoogleAuthURL":    googleAuthURL,
			"GoogleOAuthError": strings.TrimSpace(q.Get("google_error")),
			"WebAuthnError":    strings.TrimSpace(q.Get("webauthn_error")),
		})
		a.renderTemplate(w, r, "login", data)
	case http.MethodPost:
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		var u userRow
		err := a.DB.QueryRow(
			`SELECT id, username, password, google_sub, validated, is_admin, is_superadmin
             FROM users WHERE username = ?`,
			username,
		).Scan(&u.ID, &u.Username, &u.Password, &u.GoogleSub, &u.Validated, &u.IsAdmin, &u.IsSuperadmin)
		if err != nil {
			// Utilisateur introuvable
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}

		if u.GoogleSub.Valid && strings.TrimSpace(u.GoogleSub.String) != "" && (!u.Password.Valid || strings.TrimSpace(u.Password.String) == "") {
			http.Redirect(w, r, "/login?google_error=use_google", http.StatusFound)
			return
		}
		if a.userPasskeyOnly(u.ID) {
			http.Redirect(w, r, "/login?webauthn_error=use_passkey", http.StatusFound)
			return
		}

		// Verify password (supports bcrypt and Werkzeug pbkdf2)
		if !u.Password.Valid || !verifyPassword(u.Password.String, password) {
			http.Redirect(w, r, "/login?error=1", http.StatusFound)
			return
		}
		if u.Password.Valid && passwordHashNeedsUpgrade(u.Password.String) {
			if upgraded, err := hashPassword(password); err == nil {
				_, _ = a.DB.Exec(`UPDATE users SET password = ? WHERE id = ?`, upgraded, u.ID)
			}
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
		dest := safePostLoginRedirect(strings.TrimSpace(r.FormValue("next")))
		if dest == "" {
			dest = "/dashboard"
		}
		http.Redirect(w, r, dest, http.StatusFound)
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

func (a *App) HandleProfileResetReadingActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, err := a.DB.Exec(`DELETE FROM reading_activity_daily WHERE user_id = ?`, userID); err != nil {
		log.Printf("reset reading_activity_daily for user %d: %v", userID, err)
		http.Redirect(w, r, "/profile?reading_stats_reset=0", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?reading_stats_reset=1", http.StatusFound)
}

// nullFlexTime scans SQLite text timestamps and PostgreSQL timestamptz into a string form.
