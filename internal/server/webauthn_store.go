package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func (a *App) purgeExpiredWebAuthnChallenges() {
	if a == nil || a.DB == nil {
		return
	}
	_, _ = a.DB.Exec(`DELETE FROM webauthn_challenges WHERE expires_at < CURRENT_TIMESTAMP`)
}

func (a *App) putWebAuthnChallenge(data *webauthn.SessionData, userID int) (key string, err error) {
	if a == nil || a.DB == nil || data == nil {
		return "", fmt.Errorf("webauthn challenge: invalid state")
	}
	a.purgeExpiredWebAuthnChallenges()

	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	key = base64.RawURLEncoding.EncodeToString(b[:])

	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(5 * time.Minute)
	_, err = a.DB.Exec(
		`INSERT INTO webauthn_challenges (challenge_key, user_id, session_data, expires_at) VALUES (?, ?, ?, ?)`,
		key, userID, string(raw), expires,
	)
	return key, err
}

func (a *App) takeWebAuthnChallenge(key string) (webauthn.SessionData, int, bool) {
	if a == nil || a.DB == nil || key == "" {
		return webauthn.SessionData{}, 0, false
	}
	var userID int
	var raw string
	var expires time.Time
	err := a.DB.QueryRow(
		`SELECT user_id, session_data, expires_at FROM webauthn_challenges WHERE challenge_key = ?`,
		key,
	).Scan(&userID, &raw, &expires)
	if err != nil {
		if err != sql.ErrNoRows {
			_, _ = a.DB.Exec(`DELETE FROM webauthn_challenges WHERE challenge_key = ?`, key)
		}
		return webauthn.SessionData{}, 0, false
	}
	_, _ = a.DB.Exec(`DELETE FROM webauthn_challenges WHERE challenge_key = ?`, key)
	if time.Now().After(expires) {
		return webauthn.SessionData{}, 0, false
	}
	var data webauthn.SessionData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return webauthn.SessionData{}, 0, false
	}
	return data, userID, true
}
