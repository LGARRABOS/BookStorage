package server

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
)

type webAuthnUser struct {
	id          int
	name        string
	displayName string
	credentials []webauthn.Credential
}

func encodeUserIDBE(id int) []byte {
	b := make([]byte, 8)
	if id < 0 {
		return b
	}
	for i := 7; i >= 0; i-- {
		b[i] = byte(id & 0xff)
		id >>= 8
	}
	return b
}

func (u webAuthnUser) WebAuthnID() []byte {
	return encodeUserIDBE(u.id)
}

func (u webAuthnUser) WebAuthnName() string {
	return u.name
}

func (u webAuthnUser) WebAuthnDisplayName() string {
	if u.displayName != "" {
		return u.displayName
	}
	return u.name
}

func (u webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func (a *App) webAuthnEnabled() bool {
	return a.webAuthnInstance() != nil
}

func (a *App) webAuthnInstance() *webauthn.WebAuthn {
	if a == nil || a.Settings == nil {
		return nil
	}
	origin := strings.TrimSpace(a.Settings.PublicOrigin)
	if origin == "" {
		host := strings.TrimSpace(a.Settings.Host)
		if host == "" {
			host = "127.0.0.1"
		}
		origin = fmt.Sprintf("http://%s:%d", host, a.Settings.Port)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	rpID := u.Hostname()
	cfg := &webauthn.Config{
		RPDisplayName: "BookStorage",
		RPID:          rpID,
		RPOrigins:     []string{strings.TrimRight(origin, "/")},
	}
	wa, err := webauthn.New(cfg)
	if err != nil {
		return nil
	}
	return wa
}

func (a *App) loadWebAuthnUser(userID int) (webAuthnUser, error) {
	var username string
	var displayName sql.NullString
	err := a.DB.QueryRow(
		`SELECT username, display_name FROM users WHERE id = ?`,
		userID,
	).Scan(&username, &displayName)
	if err != nil {
		return webAuthnUser{}, err
	}
	creds, err := a.loadWebAuthnCredentials(userID)
	if err != nil {
		return webAuthnUser{}, err
	}
	u := webAuthnUser{id: userID, name: username, credentials: creds}
	if displayName.Valid {
		u.displayName = strings.TrimSpace(displayName.String)
	}
	return u, nil
}

func (a *App) loadWebAuthnUserByUsername(username string) (webAuthnUser, int, error) {
	var userID int
	var displayName sql.NullString
	err := a.DB.QueryRow(
		`SELECT id, display_name FROM users WHERE username = ?`,
		strings.TrimSpace(username),
	).Scan(&userID, &displayName)
	if err != nil {
		return webAuthnUser{}, 0, err
	}
	u, err := a.loadWebAuthnUser(userID)
	if err != nil {
		return webAuthnUser{}, 0, err
	}
	return u, userID, nil
}

func (a *App) loadWebAuthnCredentials(userID int) ([]webauthn.Credential, error) {
	rows, err := a.DB.Query(
		`SELECT credential_id, public_key, sign_count, backup_eligible, backup_state FROM webauthn_credentials WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []webauthn.Credential
	for rows.Next() {
		var credID, pubKey []byte
		var signCount uint32
		var backupEligible, backupState int
		if err := rows.Scan(&credID, &pubKey, &signCount, &backupEligible, &backupState); err != nil {
			continue
		}
		out = append(out, webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			Flags: webauthn.CredentialFlags{
				UserPresent:    true,
				UserVerified:   true,
				BackupEligible: backupEligible != 0,
				BackupState:    backupState != 0,
			},
			Authenticator: webauthn.Authenticator{
				SignCount: signCount,
			},
		})
	}
	return out, rows.Err()
}

func sanitizePasskeyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Passkey"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

type webAuthnCredentialRow struct {
	ID         int
	Name       string
	CreatedAt  string
	LastUsedAt sql.NullString
}

func (a *App) listWebAuthnCredentials(userID int) ([]webAuthnCredentialRow, error) {
	rows, err := a.DB.Query(
		`SELECT id, COALESCE(name, ''), created_at, last_used_at FROM webauthn_credentials WHERE user_id = ? ORDER BY id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []webAuthnCredentialRow
	for rows.Next() {
		var row webAuthnCredentialRow
		var created any
		if err := rows.Scan(&row.ID, &row.Name, &created, &row.LastUsedAt); err != nil {
			continue
		}
		row.CreatedAt = formatFlexTime(created)
		out = append(out, row)
	}
	return out, rows.Err()
}

func formatFlexTime(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(v)
	}
}

func (a *App) userHasPasskeys(userID int) bool {
	var n int
	err := a.DB.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&n)
	return err == nil && n > 0
}

func (a *App) userPasskeyOnly(userID int) bool {
	var pwd sql.NullString
	var googleSub sql.NullString
	if err := a.DB.QueryRow(`SELECT password, google_sub FROM users WHERE id = ?`, userID).Scan(&pwd, &googleSub); err != nil {
		return false
	}
	hasPwd := pwd.Valid && strings.TrimSpace(pwd.String) != ""
	hasGoogle := googleSub.Valid && strings.TrimSpace(googleSub.String) != ""
	return !hasPwd && !hasGoogle && a.userHasPasskeys(userID)
}
