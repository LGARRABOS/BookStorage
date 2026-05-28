package mail

import (
	"strings"
	"testing"
)

func TestBuildPasswordResetHTML_usesBranding(t *testing.T) {
	htmlBody := BuildPasswordResetHTML(PasswordResetContent{
		Greeting: "Hello,",
		Body:     "Reset your password.",
		Button:   "Reset",
		Expiry:   "Expires in 1h.",
		Ignore:   "Ignore if not you.",
		Footer:   "Support: admin@example.com",
	}, PasswordResetBranding{
		SiteName:   "My Library",
		BrandColor: "#ff0000",
		LogoURL:    "https://example.com/logo.png",
	}, "https://books.example/reset?token=abc")

	if !strings.Contains(htmlBody, "My Library") {
		t.Fatal("expected site name")
	}
	if !strings.Contains(htmlBody, "#ff0000") {
		t.Fatal("expected brand color")
	}
	if !strings.Contains(htmlBody, "https://example.com/logo.png") {
		t.Fatal("expected logo url")
	}
	if !strings.Contains(htmlBody, "admin@example.com") {
		t.Fatal("expected footer")
	}
	if strings.Contains(htmlBody, "<head>") {
		t.Fatal("expected no head tag for email client compatibility")
	}
}

func TestEmailSafeLogoURL_skipsSVG(t *testing.T) {
	if got := EmailSafeLogoURL("https://example.com/icon.svg"); got != "" {
		t.Fatalf("svg logo should be skipped, got %q", got)
	}
	if got := EmailSafeLogoURL("https://example.com/logo.png"); got == "" {
		t.Fatal("png logo should be kept")
	}
}

func TestBuildPasswordResetText_includesFooter(t *testing.T) {
	text := BuildPasswordResetText(PasswordResetContent{
		Greeting: "Hi",
		Body:     "Body",
		Button:   "Go",
		Expiry:   "1h",
		Ignore:   "Ignore",
		Footer:   "Footer line",
	}, "https://x.test/reset")
	if !strings.Contains(text, "Footer line") {
		t.Fatalf("text: %q", text)
	}
}
