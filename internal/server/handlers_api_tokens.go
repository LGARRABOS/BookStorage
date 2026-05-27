package server

import (
	"net/http"
	"strconv"
	"strings"
)

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
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	var scopes []string
	if r.FormValue("scope_read") == "1" {
		scopes = append(scopes, ScopeWorksRead)
	}
	if r.FormValue("scope_write") == "1" {
		scopes = append(scopes, ScopeWorksWrite)
	}

	token, row, err := a.createAPIToken(userID, name, scopes)
	if err != nil {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	tokens, _ := a.listAPITokens(userID)
	a.renderProfilePage(w, r, userID, map[string]any{
		"APITokens":       tokens,
		"NewAPIToken":     token,
		"NewAPITokenRow":  row,
		"APITokenCreated": true,
	})
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
	http.Redirect(w, r, "/profile?api_token_revoked=1", http.StatusFound)
}
