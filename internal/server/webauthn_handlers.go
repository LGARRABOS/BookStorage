package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
)

const webauthnChallengeCookie = "webauthn_challenge"

func (a *App) setWebAuthnChallengeCookie(w http.ResponseWriter, key string) {
	c := &http.Cookie{
		Name:     webauthnChallengeCookie,
		Value:    key,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: sessionSameSite(a.Settings.Environment),
	}
	if a.Settings != nil && cookieSecure(a.Settings.Environment, a.Settings.PublicOrigin) {
		c.Secure = true
	}
	http.SetCookie(w, c)
}

func clearWebAuthnChallengeCookie(w http.ResponseWriter, env, publicOrigin string) {
	c := &http.Cookie{Name: webauthnChallengeCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: sessionSameSite(env)}
	if cookieSecure(env, publicOrigin) {
		c.Secure = true
	}
	http.SetCookie(w, c)
}

func (a *App) readWebAuthnChallenge(r *http.Request) (key string, ok bool) {
	c, err := r.Cookie(webauthnChallengeCookie)
	if err != nil {
		return "", false
	}
	key = strings.TrimSpace(c.Value)
	return key, key != ""
}

// HandleWebAuthnRegisterBegin starts passkey registration for the logged-in user.
func (a *App) HandleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	wa := a.webAuthnInstance()
	if wa == nil {
		a.apiWriteError(w, http.StatusNotFound, "webauthn_disabled")
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		a.apiWriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := a.loadWebAuthnUser(userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	options, sessionData, err := wa.BeginRegistration(user,
		webauthn.WithExclusions(webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()),
	)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "begin_failed")
		return
	}
	key, err := a.putWebAuthnChallenge(sessionData, userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	a.setWebAuthnChallengeCookie(w, key)
	a.apiWriteJSON(w, http.StatusOK, options)
}

// HandleWebAuthnRegisterFinish completes passkey registration.
func (a *App) HandleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	wa := a.webAuthnInstance()
	if wa == nil {
		a.apiWriteError(w, http.StatusNotFound, "webauthn_disabled")
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		a.apiWriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	key, ok := a.readWebAuthnChallenge(r)
	if !ok {
		a.apiWriteError(w, http.StatusBadRequest, "missing_challenge")
		return
	}
	sessionData, challengeUserID, ok := a.takeWebAuthnChallenge(key)
	clearWebAuthnChallengeCookie(w, a.Settings.Environment, a.Settings.PublicOrigin)
	if !ok || challengeUserID != userID {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_challenge")
		return
	}
	user, err := a.loadWebAuthnUser(userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	credential, err := wa.FinishRegistration(user, sessionData, r)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "finish_failed")
		return
	}
	name := sanitizePasskeyName(r.URL.Query().Get("name"))
	backupEligible := 0
	backupState := 0
	if credential.Flags.BackupEligible {
		backupEligible = 1
	}
	if credential.Flags.BackupState {
		backupState = 1
	}
	_, err = a.DB.Exec(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, sign_count, name, backup_eligible, backup_state) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, credential.ID, credential.PublicKey, credential.Authenticator.SignCount, name, backupEligible, backupState,
	)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "store_failed")
		return
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleWebAuthnLoginBegin starts passkey authentication (username required in JSON body).
func (a *App) HandleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	wa := a.webAuthnInstance()
	if wa == nil {
		a.apiWriteError(w, http.StatusNotFound, "webauthn_disabled")
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	user, userID, err := a.loadWebAuthnUserByUsername(req.Username)
	if err != nil || len(user.credentials) == 0 {
		a.apiWriteError(w, http.StatusBadRequest, "unknown_user")
		return
	}
	options, sessionData, err := wa.BeginLogin(user)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "begin_failed")
		return
	}
	key, err := a.putWebAuthnChallenge(sessionData, userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	a.setWebAuthnChallengeCookie(w, key)
	a.apiWriteJSON(w, http.StatusOK, options)
}

// HandleWebAuthnLoginFinish completes passkey login and creates a session.
func (a *App) HandleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	wa := a.webAuthnInstance()
	if wa == nil {
		a.apiWriteError(w, http.StatusNotFound, "webauthn_disabled")
		return
	}
	key, ok := a.readWebAuthnChallenge(r)
	if !ok {
		a.apiWriteError(w, http.StatusBadRequest, "missing_challenge")
		return
	}
	sessionData, userID, ok := a.takeWebAuthnChallenge(key)
	clearWebAuthnChallengeCookie(w, a.Settings.Environment, a.Settings.PublicOrigin)
	if !ok || userID <= 0 {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_challenge")
		return
	}
	user, err := a.loadWebAuthnUser(userID)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_user")
		return
	}
	credential, err := wa.FinishLogin(user, sessionData, r)
	if err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "finish_failed")
		return
	}
	backupState := 0
	if credential.Flags.BackupState {
		backupState = 1
	}
	_, _ = a.DB.Exec(
		`UPDATE webauthn_credentials SET sign_count = ?, backup_state = ?, last_used_at = CURRENT_TIMESTAMP WHERE user_id = ? AND credential_id = ?`,
		credential.Authenticator.SignCount, backupState, userID, credential.ID,
	)

	var validated, isAdmin int
	if err := a.DB.QueryRow(`SELECT validated, is_admin FROM users WHERE id = ?`, userID).Scan(&validated, &isAdmin); err != nil {
		a.apiWriteError(w, http.StatusBadRequest, "invalid_user")
		return
	}
	if validated != 1 && isAdmin != 1 {
		a.apiWriteError(w, http.StatusForbidden, "pending_validation")
		return
	}

	token, err := a.createSession(r, userID)
	if err != nil {
		a.apiWriteError(w, http.StatusInternalServerError, "session_failed")
		return
	}
	a.setSessionCookie(w, token, sessionSlidingTTL)
	redirect := safePostLoginRedirect(r.URL.Query().Get("next"))
	if redirect == "" {
		redirect = "/dashboard"
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{"ok": true, "redirect": redirect})
}

// HandleWebAuthnDelete removes a passkey for the logged-in user.
func (a *App) HandleWebAuthnDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	credID, _ := strconv.Atoi(strings.TrimSpace(r.PathValue("id")))
	if credID <= 0 {
		http.Redirect(w, r, "/profile?webauthn_error=invalid", http.StatusFound)
		return
	}
	var remaining int
	_ = a.DB.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&remaining)
	if remaining <= 1 && a.userPasskeyOnly(userID) {
		http.Redirect(w, r, "/profile?webauthn_error=last_credential", http.StatusFound)
		return
	}
	res, err := a.DB.Exec(`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, credID, userID)
	if err != nil {
		http.Redirect(w, r, "/profile?webauthn_error=server", http.StatusFound)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Redirect(w, r, "/profile?webauthn_error=not_found", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/profile?webauthn_deleted=1", http.StatusFound)
}

// Ensure webAuthnUser satisfies webauthn.User at compile time.
var _ webauthn.User = webAuthnUser{}
