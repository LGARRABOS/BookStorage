package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	ScopeWorksRead  = "works:read"
	ScopeWorksWrite = "works:write"
)

var validAPIScopes = map[string]bool{
	ScopeWorksRead:  true,
	ScopeWorksWrite: true,
}

type apiAuthUserIDKey struct{}
type apiAuthScopesKey struct{}

type apiTokenRow struct {
	ID         int
	UserID     int
	Name       string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt sql.NullTime
	RevokedAt  sql.NullTime
}

func hashAPIToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func newAPITokenSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b[:])
	return "bs_" + raw, nil
}

func parseBearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	fields := strings.Fields(auth)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

func normalizeAPIScopes(scopes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" || !validAPIScopes[s] || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func encodeAPIScopes(scopes []string) string {
	b, _ := json.Marshal(normalizeAPIScopes(scopes))
	return string(b)
}

func decodeAPIScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil
	}
	return normalizeAPIScopes(scopes)
}

func apiAuthUserIDFromContext(ctx context.Context) (int, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(apiAuthUserIDKey{}).(int)
	return id, ok && id > 0
}

func apiAuthScopesFromContext(ctx context.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	scopes, ok := ctx.Value(apiAuthScopesKey{}).([]string)
	return scopes, ok
}

func (a *App) WithAPITokenContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, scopes, ok := a.resolveAPIToken(r); ok {
			ctx := context.WithValue(r.Context(), apiAuthUserIDKey{}, userID)
			ctx = context.WithValue(ctx, apiAuthScopesKey{}, scopes)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) resolveAPIToken(r *http.Request) (userID int, scopes []string, ok bool) {
	if r == nil || a.DB == nil {
		return 0, nil, false
	}
	token := parseBearerToken(r)
	if token == "" {
		return 0, nil, false
	}

	var uid int
	var scopesRaw string
	var revokedAt sql.NullTime
	err := a.DB.QueryRow(
		`SELECT user_id, scopes, revoked_at FROM api_tokens WHERE token_hash = ?`,
		hashAPIToken(token),
	).Scan(&uid, &scopesRaw, &revokedAt)
	if err != nil || revokedAt.Valid || uid <= 0 {
		return 0, nil, false
	}

	scopes = decodeAPIScopes(scopesRaw)
	if len(scopes) == 0 {
		return 0, nil, false
	}

	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ? AND revoked_at IS NULL`,
		now, hashAPIToken(token),
	)
	return uid, scopes, true
}

func (a *App) hasValidAPIToken(r *http.Request) bool {
	if userID, ok := apiAuthUserIDFromContext(r.Context()); ok {
		return userID > 0
	}
	_, _, ok := a.resolveAPIToken(r)
	return ok
}

func (a *App) createAPIToken(userID int, name string, scopes []string) (token string, row apiTokenRow, err error) {
	if userID <= 0 {
		return "", apiTokenRow{}, sql.ErrNoRows
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "API token"
	}
	scopes = normalizeAPIScopes(scopes)
	if len(scopes) == 0 {
		scopes = []string{ScopeWorksRead}
	}

	token, err = newAPITokenSecret()
	if err != nil {
		return "", apiTokenRow{}, err
	}
	now := time.Now().UTC()
	res, err := a.DB.Exec(
		`INSERT INTO api_tokens (user_id, name, token_hash, scopes, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, name, hashAPIToken(token), encodeAPIScopes(scopes), now,
	)
	if err != nil {
		return "", apiTokenRow{}, err
	}
	id, _ := res.LastInsertId()
	row = apiTokenRow{
		ID:        int(id),
		UserID:    userID,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: now,
	}
	return token, row, nil
}

func (a *App) revokeAPIToken(userID, tokenID int) error {
	if userID <= 0 || tokenID <= 0 {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	res, err := a.DB.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		now, tokenID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *App) listAPITokens(userID int) ([]apiTokenRow, error) {
	if userID <= 0 {
		return nil, nil
	}
	rows, err := a.DB.Query(
		`SELECT id, user_id, name, scopes, created_at, last_used_at, revoked_at
		 FROM api_tokens
		 WHERE user_id = ? AND revoked_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []apiTokenRow
	for rows.Next() {
		var row apiTokenRow
		var scopesRaw string
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &scopesRaw, &row.CreatedAt, &row.LastUsedAt, &row.RevokedAt); err != nil {
			return nil, err
		}
		row.Scopes = decodeAPIScopes(scopesRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (a *App) hasAPIScope(r *http.Request, scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return false
	}
	if _, ok := apiAuthUserIDFromContext(r.Context()); !ok {
		return true
	}
	scopes, ok := apiAuthScopesFromContext(r.Context())
	if !ok {
		return false
	}
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (a *App) RequireAPIScope(scope string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !a.hasAPIScope(r, scope) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					a.apiWriteError(w, http.StatusForbidden, "insufficient_scope")
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
				return
			}
			next(w, r)
		}
	}
}
