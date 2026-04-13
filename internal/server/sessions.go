package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName  = "session"
	sessionSlidingTTL  = 2 * time.Hour
	sessionAbsoluteTTL = 24 * time.Hour
)

type sessionRow struct {
	ID         int
	UserID     int
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	IP         sql.NullString
	UserAgent  sql.NullString
	RevokedAt  sql.NullTime
}

func newSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashSessionToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string, maxAge time.Duration) {
	secs := int(maxAge.Seconds())
	if secs < 0 {
		secs = -1
	}
	c := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   secs,
		HttpOnly: true,
		SameSite: sessionSameSite(a.Settings.Environment),
	}
	if strings.ToLower(a.Settings.Environment) == "production" {
		c.Secure = true
	}
	http.SetCookie(w, c)
}

func (a *App) clearSession(w http.ResponseWriter) {
	a.setSessionCookie(w, "", -1)
}

func (a *App) createSession(r *http.Request, userID int) (token string, err error) {
	token, err = newSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expires := now.Add(sessionSlidingTTL)
	ip := clientIP(r)
	ua := ""
	if r != nil {
		ua = r.UserAgent()
	}
	_, err = a.DB.Exec(
		`INSERT INTO sessions (user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, hashSessionToken(token), now, now, expires, ip, ua,
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (a *App) currentSession(r *http.Request) (userID int, token string, ok bool) {
	if r == nil {
		return 0, "", false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, "", false
	}
	token = strings.TrimSpace(c.Value)
	if token == "" {
		return 0, "", false
	}

	var uid int
	var createdAt, expiresAt time.Time
	var revokedAt sql.NullTime
	err = a.DB.QueryRow(
		`SELECT user_id, created_at, expires_at, revoked_at
		 FROM sessions
		 WHERE token_hash = ?`,
		hashSessionToken(token),
	).Scan(&uid, &createdAt, &expiresAt, &revokedAt)
	if err != nil {
		return 0, "", false
	}
	if revokedAt.Valid {
		return 0, "", false
	}
	now := time.Now().UTC()
	if now.After(expiresAt) {
		return 0, "", false
	}
	if now.After(createdAt.Add(sessionAbsoluteTTL)) {
		return 0, "", false
	}
	return uid, token, true
}

func (a *App) touchSession(r *http.Request, token string) {
	if token == "" {
		return
	}
	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`UPDATE sessions
		 SET last_seen_at = ?, expires_at = ?
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		now, now.Add(sessionSlidingTTL), hashSessionToken(token),
	)
}

func (a *App) revokeSession(token string) {
	if token == "" {
		return
	}
	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`UPDATE sessions SET revoked_at = ?
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		now, hashSessionToken(token),
	)
}

func (a *App) revokeAllUserSessions(userID int) {
	if userID <= 0 {
		return
	}
	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`UPDATE sessions SET revoked_at = ?
		 WHERE user_id = ? AND revoked_at IS NULL`,
		now, userID,
	)
}

func (a *App) listActiveSessions(userID int) ([]sessionRow, error) {
	if userID <= 0 {
		return nil, nil
	}
	rows, err := a.DB.Query(
		`SELECT id, user_id, created_at, last_seen_at, expires_at, ip, user_agent, revoked_at
		 FROM sessions
		 WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
		 ORDER BY last_seen_at DESC
		 LIMIT 20`,
		userID, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.IP, &s.UserAgent, &s.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
