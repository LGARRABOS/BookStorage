package mail

import (
	"fmt"
	"html"
	"strings"
)

const defaultMailBrandColor = "#4f46e5"

// PasswordResetContent holds localized strings for a password reset email.
type PasswordResetContent struct {
	Subject  string
	Greeting string
	Body     string
	Button   string
	Expiry   string
	Ignore   string
	Footer   string
}

// PasswordResetBranding holds visual customization for password reset emails.
type PasswordResetBranding struct {
	SiteName   string
	BrandColor string
	LogoURL    string
}

// BuildPasswordResetText renders the plain-text password reset email.
func BuildPasswordResetText(content PasswordResetContent, resetLink string) string {
	parts := []string{
		content.Greeting,
		"",
		content.Body,
		"",
		resetLink,
		content.Button,
		"",
		content.Expiry,
		"",
		content.Ignore,
	}
	if f := strings.TrimSpace(content.Footer); f != "" {
		parts = append(parts, "", f)
	}
	return strings.Join(parts, "\n")
}

// BuildPasswordResetHTML renders a simple responsive HTML password reset email.
func BuildPasswordResetHTML(content PasswordResetContent, branding PasswordResetBranding, resetLink string) string {
	siteName := html.EscapeString(strings.TrimSpace(branding.SiteName))
	if siteName == "" {
		siteName = "BookStorage"
	}
	color := normalizeBrandColor(branding.BrandColor)
	logo := strings.TrimSpace(branding.LogoURL)

	var header strings.Builder
	if logo != "" {
		header.WriteString(fmt.Sprintf(
			`<img src="%s" alt="%s" width="48" height="48" style="display:block;margin:0 auto 0.75rem auto;border-radius:0.5rem;">`,
			html.EscapeString(logo), siteName,
		))
	}
	header.WriteString(fmt.Sprintf(`<p style="margin:0;font-size:1.25rem;font-weight:700;color:#111;">%s</p>`, siteName))

	footerExtra := ""
	if f := strings.TrimSpace(content.Footer); f != "" {
		footerExtra = fmt.Sprintf(`<p style="font-size:0.85rem;color:#777;margin:1rem 0 0 0;">%s</p>`, html.EscapeString(f))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="und">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:2rem 1rem;">
<tr><td align="center">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:520px;background:#fff;border-radius:0.75rem;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.08);">
<tr><td style="background:%s;padding:1.5rem 1.5rem 1rem 1.5rem;text-align:center;color:#fff;">
%s
</td></tr>
<tr><td style="padding:1.5rem;color:#111;line-height:1.6;">
<p style="margin:0 0 1rem 0;">%s</p>
<p style="margin:0 0 1.5rem 0;">%s</p>
<p style="margin:0 0 1.5rem 0;text-align:center;">
<a href="%s" style="display:inline-block;padding:0.85rem 1.5rem;background:%s;color:#fff;text-decoration:none;border-radius:0.5rem;font-weight:600;">%s</a>
</p>
<p style="margin:0;font-size:0.9rem;color:#555;">%s</p>
<p style="margin:0.75rem 0 0 0;font-size:0.85rem;color:#777;">%s</p>
%s
</td></tr>
</table>
</td></tr>
</table>
</body></html>`,
		color,
		header.String(),
		html.EscapeString(content.Greeting),
		html.EscapeString(content.Body),
		html.EscapeString(resetLink),
		color,
		html.EscapeString(content.Button),
		html.EscapeString(content.Expiry),
		html.EscapeString(content.Ignore),
		footerExtra,
	)
}

func normalizeBrandColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return defaultMailBrandColor
	}
	if !strings.HasPrefix(c, "#") {
		c = "#" + c
	}
	if len(c) != 7 && len(c) != 4 {
		return defaultMailBrandColor
	}
	return c
}
