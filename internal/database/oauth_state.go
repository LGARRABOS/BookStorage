package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const oauthStateTTL = 15 * time.Minute

// OAuthStatePurpose is stored in oauth_states.purpose.
type OAuthStatePurpose string

const (
	OAuthPurposeLogin OAuthStatePurpose = "login"
	OAuthPurposeLink  OAuthStatePurpose = "link"
)

// HashOAuthState returns a fixed-length hex hash for storing oauth state.
func HashOAuthState(statePlain string) string {
	h := sha256.Sum256([]byte(statePlain))
	return hex.EncodeToString(h[:])
}

// NewOAuthStatePlain returns a URL-safe random state string (raw base64, 32 bytes).
func NewOAuthStatePlain() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b[:]), nil
}

// InsertOAuthState stores a one-time OAuth CSRF/PKCE row.
func InsertOAuthState(c *Conn, statePlain string, purpose OAuthStatePurpose, userID sql.NullInt64, next string, codeVerifier string) error {
	if c == nil || statePlain == "" || codeVerifier == "" {
		return fmt.Errorf("oauth state: empty state or verifier")
	}
	exp := time.Now().UTC().Add(oauthStateTTL).Unix()
	_, err := c.Exec(
		`INSERT INTO oauth_states (state_hash, purpose, user_id, next, expires_at_unix, code_verifier)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		HashOAuthState(statePlain), string(purpose), userID, next, exp, codeVerifier,
	)
	return err
}

// OAuthStateRow is returned when consuming a valid oauth_states row.
type OAuthStateRow struct {
	Purpose      OAuthStatePurpose
	UserID       sql.NullInt64
	Next         string
	CodeVerifier string
}

// DeleteExpiredOAuthStates removes stale rows (best-effort).
func DeleteExpiredOAuthStates(c *Conn) {
	if c == nil {
		return
	}
	now := time.Now().UTC().Unix()
	_, _ = c.Exec(`DELETE FROM oauth_states WHERE expires_at_unix < ?`, now)
}

// ConsumeOAuthState deletes the row identified by statePlain and returns it if still valid.
func ConsumeOAuthState(c *Conn, statePlain string) (OAuthStateRow, error) {
	var out OAuthStateRow
	if c == nil || statePlain == "" {
		return out, sql.ErrNoRows
	}
	hash := HashOAuthState(statePlain)
	now := time.Now().UTC().Unix()

	tx, err := c.Begin()
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()

	var purpose string
	var uid sql.NullInt64
	var next sql.NullString
	var verifier string
	err = tx.QueryRow(
		`SELECT purpose, user_id, next, code_verifier FROM oauth_states
		 WHERE state_hash = ? AND expires_at_unix >= ?`,
		hash, now,
	).Scan(&purpose, &uid, &next, &verifier)
	if err != nil {
		return out, err
	}
	if _, err := tx.Exec(`DELETE FROM oauth_states WHERE state_hash = ?`, hash); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	out.Purpose = OAuthStatePurpose(purpose)
	out.UserID = uid
	if next.Valid {
		out.Next = next.String
	}
	out.CodeVerifier = verifier
	return out, nil
}
