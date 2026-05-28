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

// EmailSafeLogoURL returns logoURL when it is suitable for HTML email clients.
// SVG images are skipped because many clients (including Gmail) do not render them.
func EmailSafeLogoURL(logoURL string) string {
	logoURL = strings.TrimSpace(logoURL)
	if logoURL == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(logoURL), ".svg") {
		return ""
	}
	return logoURL
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

// BuildPasswordResetHTML renders a simple HTML password reset email compatible with Gmail.
func BuildPasswordResetHTML(content PasswordResetContent, branding PasswordResetBranding, resetLink string) string {
	siteName := html.EscapeString(strings.TrimSpace(branding.SiteName))
	if siteName == "" {
		siteName = "BookStorage"
	}
	color := html.EscapeString(normalizeBrandColor(branding.BrandColor))
	logo := html.EscapeString(EmailSafeLogoURL(branding.LogoURL))

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><body style="margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;line-height:1.6;color:#111;background:#f4f4f5;">`)
	b.WriteString(`<div style="max-width:520px;margin:0 auto;padding:1.5rem 1rem;">`)
	b.WriteString(`<div style="background:#fff;border-radius:0.75rem;padding:1.5rem;border:1px solid #e4e4e7;">`)

	if logo != "" {
		fmt.Fprintf(&b, `<p style="margin:0 0 1rem 0;text-align:center;"><img src="%s" alt="%s" width="48" height="48" style="border-radius:0.5rem;"></p>`, logo, siteName)
	}
	fmt.Fprintf(&b, `<p style="margin:0 0 1rem 0;font-size:1.1rem;font-weight:700;text-align:center;color:%s;">%s</p>`, color, siteName)
	fmt.Fprintf(&b, `<p style="margin:0 0 1rem 0;">%s</p>`, html.EscapeString(content.Greeting))
	fmt.Fprintf(&b, `<p style="margin:0 0 1.5rem 0;">%s</p>`, html.EscapeString(content.Body))
	fmt.Fprintf(&b, `<p style="margin:0 0 1.5rem 0;text-align:center;"><a href="%s" style="display:inline-block;padding:0.85rem 1.5rem;background:%s;color:#fff;text-decoration:none;border-radius:0.5rem;font-weight:600;">%s</a></p>`,
		html.EscapeString(resetLink), color, html.EscapeString(content.Button))
	fmt.Fprintf(&b, `<p style="margin:0;font-size:0.9rem;color:#555;">%s</p>`, html.EscapeString(content.Expiry))
	fmt.Fprintf(&b, `<p style="margin:0.75rem 0 0 0;font-size:0.85rem;color:#777;">%s</p>`, html.EscapeString(content.Ignore))
	if f := strings.TrimSpace(content.Footer); f != "" {
		fmt.Fprintf(&b, `<p style="margin:1rem 0 0 0;font-size:0.85rem;color:#777;">%s</p>`, html.EscapeString(f))
	}

	b.WriteString(`</div></div></body></html>`)
	return b.String()
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
