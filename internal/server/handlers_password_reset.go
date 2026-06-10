package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
					if a.recentPasswordResetEmailSent(u.ID) {
						continue
					}
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

func (a *App) resetPasswordPageData(token, formError, resetError string) map[string]any {
	data := map[string]any{}
	if formError != "" {
		data["FormError"] = formError
	}
	if resetError != "" {
		data["ResetError"] = resetError
		return data
	}
	if token == "" {
		data["ResetError"] = "missing"
		return data
	}
	if _, ok := a.lookupPasswordResetToken(token); !ok {
		data["ResetError"] = "invalid"
		return data
	}
	data["ShowResetForm"] = true
	return data
}

func (a *App) resetPasswordTokenFromRequest(r *http.Request) string {
	if r.Method == http.MethodPost {
		if token := strings.TrimSpace(r.FormValue("token")); token != "" {
			return token
		}
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	return a.resetTokenFromCookie(r)
}

func (a *App) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !a.passwordResetEnabled() {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if queryToken := strings.TrimSpace(r.URL.Query().Get("token")); queryToken != "" {
			if _, ok := a.lookupPasswordResetToken(queryToken); ok {
				a.setResetTokenCookie(w, queryToken)
				http.Redirect(w, r, "/reset-password", http.StatusFound)
				return
			}
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData("", "", "invalid")))
			return
		}
		token := a.resetTokenFromCookie(r)
		data := a.resetPasswordPageData(token, "", "")
		if token != "" && data["ResetError"] == "invalid" {
			a.clearResetTokenCookie(w)
		}
		a.renderTemplate(w, r, "reset_password", a.mergeData(r, data))
	case http.MethodPost:
		token := a.resetPasswordTokenFromRequest(r)
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")
		row, ok := a.lookupPasswordResetToken(token)
		if !ok {
			a.clearResetTokenCookie(w)
			data := a.resetPasswordPageData("", "invalid", "invalid")
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, data))
			return
		}
		var storedPassword sql.NullString
		if err := a.DB.QueryRow(`SELECT password FROM users WHERE id = ?`, row.UserID).Scan(&storedPassword); err != nil {
			a.clearResetTokenCookie(w)
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData("", "", "invalid")))
			return
		}
		if !a.userEligibleForPasswordReset(row.UserID, storedPassword) {
			a.clearResetTokenCookie(w)
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData("", "", "invalid")))
			return
		}
		if newPassword != confirmPassword {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData(token, "mismatch", "")))
			return
		}
		if len(newPassword) < minPasswordLen {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData(token, "weak", "")))
			return
		}
		hashedPassword, err := hashPassword(newPassword)
		if err != nil {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData(token, "server", "")))
			return
		}
		if _, err := a.DB.Exec(`UPDATE users SET password = ? WHERE id = ?`, hashedPassword, row.UserID); err != nil {
			a.renderTemplate(w, r, "reset_password", a.mergeData(r, a.resetPasswordPageData(token, "server", "")))
			return
		}
		a.markPasswordResetTokenUsed(token)
		a.revokeAllUserSessions(row.UserID)
		a.clearResetTokenCookie(w)
		http.Redirect(w, r, "/login?reset=1", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) sendPasswordResetEmail(ctx context.Context, sender mail.Sender, to, lang, rawToken string) error {
	if a.Settings == nil {
		return fmt.Errorf("settings unavailable")
	}
	siteName := "BookStorage"
	var brandColor, mailFooter string
	if a.SiteConfig != nil {
		if n := strings.TrimSpace(a.SiteConfig.SiteName); n != "" {
			siteName = n
		}
		brandColor = a.SiteConfig.Mail.BrandColor
		mailFooter = a.SiteConfig.Mail.Footer
	}
	logoURL := ""
	if a.SiteConfig != nil {
		logoURL = a.SiteConfig.MailLogoURL(a.Settings.PublicOrigin)
	}

	tr := i18n.T(lang)
	subjectKey := tr["mail.password_reset.subject"]
	greeting := tr["mail.password_reset.greeting"]
	bodyKey := tr["mail.password_reset.body"]
	button := tr["mail.password_reset.button"]
	expiry := tr["mail.password_reset.expiry"]
	ignore := tr["mail.password_reset.ignore"]
	if subjectKey == "" {
		def := i18n.T(i18n.DefaultLang)
		subjectKey = def["mail.password_reset.subject"]
		greeting = def["mail.password_reset.greeting"]
		bodyKey = def["mail.password_reset.body"]
		button, expiry, ignore = def["mail.password_reset.button"], def["mail.password_reset.expiry"], def["mail.password_reset.ignore"]
	}
	subject := fmt.Sprintf("%s — %s", fmt.Sprintf(subjectKey, siteName), time.Now().UTC().Format("02.01.2006 15:04"))
	body := fmt.Sprintf(bodyKey, siteName)
	requestedAtKey := tr["mail.password_reset.requested_at"]
	if requestedAtKey == "" {
		requestedAtKey = i18n.T(i18n.DefaultLang)["mail.password_reset.requested_at"]
	}
	requestedAt := fmt.Sprintf(requestedAtKey, time.Now().UTC().Format("02.01.2006 15:04"))
	resetLink := passwordResetURL(a.Settings.PublicOrigin, rawToken)
	content := mail.PasswordResetContent{
		Subject:     subject,
		Greeting:    greeting,
		Body:        body,
		Button:      button,
		RequestedAt: requestedAt,
		Expiry:      expiry,
		Ignore:      ignore,
		Footer:      mailFooter,
	}
	branding := mail.PasswordResetBranding{
		SiteName:   siteName,
		BrandColor: brandColor,
		LogoURL:    mail.EmailSafeLogoURL(logoURL),
	}
	return sender.Send(ctx, mail.Message{
		To:       to,
		Subject:  subject,
		TextBody: mail.BuildPasswordResetText(content, resetLink),
		HTMLBody: mail.BuildPasswordResetHTML(content, branding, resetLink),
		CustomID: "password-reset-" + hashSessionToken(rawToken)[:12],
	})
}
