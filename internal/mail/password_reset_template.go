package mail

import (
	"fmt"
	"html"
	"strings"
)

const defaultMailBrandColor = "#4f46e5"

// PasswordResetContent holds localized strings for a password reset email.
type PasswordResetContent struct {
	Subject     string
	Greeting    string
	Body        string
	Button      string
	RequestedAt string
	Expiry      string
	Ignore      string
	Footer      string
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
	}
	if f := strings.TrimSpace(content.RequestedAt); f != "" {
		parts = append(parts, "", f)
	}
	parts = append(parts, "", content.Expiry, "", content.Ignore)
	if f := strings.TrimSpace(content.Footer); f != "" {
		parts = append(parts, "", f)
	}
	return strings.Join(parts, "\n")
}

// BuildPasswordResetHTML renders a minimal HTML password reset email compatible with Gmail.
func BuildPasswordResetHTML(content PasswordResetContent, branding PasswordResetBranding, resetLink string) string {
	siteName := html.EscapeString(strings.TrimSpace(branding.SiteName))
	if siteName == "" {
		siteName = "BookStorage"
	}
	color := html.EscapeString(normalizeBrandColor(branding.BrandColor))
	logo := html.EscapeString(EmailSafeLogoURL(branding.LogoURL))

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><body style="font-family:sans-serif;line-height:1.5;color:#111;">`)
	if logo != "" {
		fmt.Fprintf(&b, `<p style="text-align:center;"><img src="%s" alt="%s" width="48" height="48"></p>`, logo, siteName)
	}
	fmt.Fprintf(&b, `<p style="text-align:center;font-weight:bold;color:%s;">%s</p>`, color, siteName)
	fmt.Fprintf(&b, `<p>%s</p>`, html.EscapeString(content.Greeting))
	fmt.Fprintf(&b, `<p>%s</p>`, html.EscapeString(content.Body))
	fmt.Fprintf(&b, `<p style="text-align:center;"><a href="%s" style="display:inline-block;padding:0.75rem 1.25rem;background:%s;color:#fff;text-decoration:none;border-radius:0.5rem;">%s</a></p>`,
		html.EscapeString(resetLink), color, html.EscapeString(content.Button))
	if f := strings.TrimSpace(content.RequestedAt); f != "" {
		fmt.Fprintf(&b, `<p style="font-size:0.85rem;color:#777;">%s</p>`, html.EscapeString(f))
	}
	fmt.Fprintf(&b, `<p style="font-size:0.9rem;color:#555;">%s</p>`, html.EscapeString(content.Expiry))
	fmt.Fprintf(&b, `<p style="font-size:0.85rem;color:#777;">%s</p>`, html.EscapeString(content.Ignore))
	if f := strings.TrimSpace(content.Footer); f != "" {
		fmt.Fprintf(&b, `<p style="font-size:0.85rem;color:#777;">%s</p>`, html.EscapeString(f))
	}
	b.WriteString(`</body></html>`)
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
