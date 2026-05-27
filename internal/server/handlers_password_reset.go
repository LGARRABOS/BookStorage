package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"bookstorage/internal/i18n"
	"bookstorage/internal/mail"
)

func (a *App) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !a.passwordResetEnabled() {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		a.renderTemplate(w, r, "forgot_password", a.mergeData(r, map[string]any{
			"ForgotSent": q.Get("sent") != "",
		}))
	case http.MethodPost:
		email := normalizeEmail(r.FormValue("email"))
		if email != "" {
			users, err := a.findUsersByEmailForPasswordReset(email)
			if err != nil {
				log.Printf("password reset lookup: %v", err)
			} else {
				lang := ""
				if data := a.baseData(r); data != nil {
					if l, ok := data["Lang"].(string); ok {
						lang = l
					}
				}
				sender := mail.NewSender(a.Settings)
				for _, u := range users {
					rawToken, err := a.createPasswordResetToken(u.ID)
					if err != nil {
						log.Printf("password reset token for user %d: %v", u.ID, err)
						continue
					}
					if err := a.sendPasswordResetEmail(r.Context(), sender, u.Email, lang, rawToken); err != nil {
						log.Printf("password reset email for user %d: %v", u.ID, err)
					}
				}
			}
		}
		http.Redirect(w, r, "/forgot-password?sent=1", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !a.passwordResetEnabled() {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if r.Method == http.MethodPost {
		token = strings.TrimSpace(r.FormValue("token"))
	}
	switch r.Method {
	case http.MethodGet:
		data := map[string]any{"Token": token}
		if token == "" {
			data["ResetError"] = "missing"
		} else if _, ok := a.lookupPasswordResetToken(token); !ok {
			data["ResetError"] = "invalid"
		}
		a.renderTemplate(w, r, "reset_password", a.mergeData(r, data))
	case http.MethodPost:
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")
		row, ok := a.lookupPasswordResetToken(token)
		if !ok {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":      token,
				"ResetError": "invalid",
				"FormError":  "invalid",
			}))
			return
		}
		var storedPassword sql.NullString
		if err := a.DB.QueryRow(`SELECT password FROM users WHERE id = ?`, row.UserID).Scan(&storedPassword); err != nil {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":      token,
				"ResetError": "invalid",
			}))
			return
		}
		if !a.userEligibleForPasswordReset(row.UserID, storedPassword) {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":      token,
				"ResetError": "invalid",
			}))
			return
		}
		if newPassword != confirmPassword {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":     token,
				"FormError": "mismatch",
			}))
			return
		}
		if len(newPassword) < minPasswordLen {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":     token,
				"FormError": "weak",
			}))
			return
		}
		hashedPassword, err := hashPassword(newPassword)
		if err != nil {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":     token,
				"FormError": "server",
			}))
			return
		}
		if _, err := a.DB.Exec(`UPDATE users SET password = ? WHERE id = ?`, hashedPassword, row.UserID); err != nil {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, map[string]any{
				"Token":     token,
				"FormError": "server",
			}))
			return
		}
		a.markPasswordResetTokenUsed(token)
		a.revokeAllUserSessions(row.UserID)
		http.Redirect(w, r, "/login?reset=1", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) sendPasswordResetEmail(ctx context.Context, sender mail.Sender, to, lang, rawToken string) error {
	if a.Settings == nil {
		return fmt.Errorf("settings unavailable")
	}
	tr := i18n.T(lang)
	subject := tr["mail.password_reset.subject"]
	greeting := tr["mail.password_reset.greeting"]
	body := tr["mail.password_reset.body"]
	button := tr["mail.password_reset.button"]
	expiry := tr["mail.password_reset.expiry"]
	ignore := tr["mail.password_reset.ignore"]
	if subject == "" {
		def := i18n.T(i18n.DefaultLang)
		subject, greeting, body = def["mail.password_reset.subject"], def["mail.password_reset.greeting"], def["mail.password_reset.body"]
		button, expiry, ignore = def["mail.password_reset.button"], def["mail.password_reset.expiry"], def["mail.password_reset.ignore"]
	}
	resetLink := passwordResetURL(a.Settings.PublicOrigin, rawToken)
	textBody := fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n\n%s\n\n%s",
		greeting, body, resetLink, button, expiry, ignore)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;line-height:1.5;color:#111;">
<p>%s</p>
<p>%s</p>
<p><a href="%s" style="display:inline-block;padding:0.75rem 1.25rem;background:#4f46e5;color:#fff;text-decoration:none;border-radius:0.5rem;">%s</a></p>
<p style="font-size:0.9rem;color:#555;">%s</p>
<p style="font-size:0.85rem;color:#777;">%s</p>
</body></html>`, greeting, body, resetLink, button, expiry, ignore)
	return sender.Send(ctx, mail.Message{
		To:       to,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	})
}
