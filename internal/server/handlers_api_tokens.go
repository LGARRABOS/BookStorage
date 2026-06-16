package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiTokenIntegrationName = "Intégration"

func (a *App) revokeAllUserAPITokens(userID int) error {
	if userID <= 0 {
		return nil
	}
	now := time.Now().UTC()
	_, err := a.DB.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		now, userID,
	)
	return err
}

func (a *App) applyAPITokenFlashFromRequest(r *http.Request, userID int, extra map[string]any) {
	if extra == nil || r == nil || userID <= 0 {
		return
	}
	nonce := strings.TrimSpace(r.URL.Query().Get("api_token_flash"))
	if nonce == "" {
		return
	}
	if token, ok := a.consumeAPITokenFlash(userID, nonce); ok {
		extra["APITokenCreated"] = true
		extra["NewAPIToken"] = token
	}
}

func (a *App) HandleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile?security=1", http.StatusFound)
		return
	}

	_ = a.revokeAllUserAPITokens(userID)

	scopes := []string{ScopeWorksRead, ScopeWorksWrite}
	token, _, err := a.createAPIToken(userID, apiTokenIntegrationName, scopes)
	if err != nil {
		http.Redirect(w, r, "/profile?security=1", http.StatusFound)
		return
	}

	nonce, err := a.storeAPITokenFlash(userID, token)
	if err != nil {
		http.Redirect(w, r, "/profile?security=1", http.StatusFound)
		return
	}

	redirectURL := fmt.Sprintf("/profile?security=1&api_token_flash=%s", url.QueryEscape(nonce))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (a *App) HandleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Redirect(w, r, loginRedirectURL(r), http.StatusFound)
		return
	}
	if _, apiOK := apiAuthUserIDFromContext(r.Context()); apiOK {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	tokenID, _ := strconv.Atoi(r.PathValue("id"))
	if tokenID <= 0 {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	_ = a.revokeAPIToken(userID, tokenID)
	http.Redirect(w, r, "/profile?security=1&api_token_revoked=1", http.StatusFound)
}
