package server

import (
	"database/sql"
	"strings"
	"time"
)

const passwordResetTokenTTL = time.Hour

func normalizeEmail(email string) string {
	return normalizeAccountEmail(email)
}

func (a *App) passwordResetEnabled() bool {
	return a.Settings != nil && a.Settings.MailConfigured()
}

func (a *App) cleanupPasswordResetTokens() {
	now := time.Now().UTC()
	_, _ = a.DB.Exec(`DELETE FROM password_reset_tokens WHERE expires_at < ? OR used_at IS NOT NULL`, now)
}

func (a *App) invalidatePasswordResetTokensForUser(userID int) {
	if userID <= 0 {
		return
	}
	_, _ = a.DB.Exec(`DELETE FROM password_reset_tokens WHERE user_id = ? AND used_at IS NULL`, userID)
}

func (a *App) createPasswordResetToken(userID int) (rawToken string, err error) {
	a.cleanupPasswordResetTokens()
	a.invalidatePasswordResetTokensForUser(userID)

	rawToken, err = newSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expires := now.Add(passwordResetTokenTTL)
	_, err = a.DB.Exec(
		`INSERT INTO password_reset_tokens (token_hash, user_id, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		hashSessionToken(rawToken), userID, now, expires,
	)
	if err != nil {
		return "", err
	}
	return rawToken, nil
}

type passwordResetTokenRow struct {
	UserID    int
	ExpiresAt time.Time
	UsedAt    sql.NullTime
}

func (a *App) lookupPasswordResetToken(rawToken string) (passwordResetTokenRow, bool) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return passwordResetTokenRow{}, false
	}
	a.cleanupPasswordResetTokens()
	var row passwordResetTokenRow
	err := a.DB.QueryRow(
		`SELECT user_id, expires_at, used_at FROM password_reset_tokens WHERE token_hash = ?`,
		hashSessionToken(rawToken),
	).Scan(&row.UserID, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		return passwordResetTokenRow{}, false
	}
	if row.UsedAt.Valid {
		return passwordResetTokenRow{}, false
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		return passwordResetTokenRow{}, false
	}
	return row, true
}

func (a *App) markPasswordResetTokenUsed(rawToken string) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return
	}
	now := time.Now().UTC()
	_, _ = a.DB.Exec(
		`UPDATE password_reset_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`,
		now, hashSessionToken(rawToken),
	)
}

func (a *App) userEligibleForPasswordReset(userID int, password sql.NullString) bool {
	if userID <= 0 {
		return false
	}
	if !password.Valid || strings.TrimSpace(password.String) == "" {
		return false
	}
	if a.userPasskeyOnly(userID) {
		return false
	}
	return true
}

func (a *App) findUsersByEmailForPasswordReset(email string) ([]struct {
	ID       int
	Password sql.NullString
	Email    string
}, error) {
	norm := normalizeEmail(email)
	if norm == "" {
		return nil, nil
	}
	rows, err := a.DB.Query(
		`SELECT id, password, email FROM users WHERE email IS NOT NULL AND LOWER(TRIM(email)) = ?`,
		norm,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []struct {
		ID       int
		Password sql.NullString
		Email    string
	}
	for rows.Next() {
		var u struct {
			ID       int
			Password sql.NullString
			Email    sql.NullString
		}
		if err := rows.Scan(&u.ID, &u.Password, &u.Email); err != nil {
			return nil, err
		}
		if !u.Email.Valid || strings.TrimSpace(u.Email.String) == "" {
			continue
		}
		if !a.userEligibleForPasswordReset(u.ID, u.Password) {
			continue
		}
		out = append(out, struct {
			ID       int
			Password sql.NullString
			Email    string
		}{ID: u.ID, Password: u.Password, Email: strings.TrimSpace(u.Email.String)})
	}
	return out, rows.Err()
}

func passwordResetURL(origin, rawToken string) string {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	return origin + "/reset-password?token=" + rawToken
}
