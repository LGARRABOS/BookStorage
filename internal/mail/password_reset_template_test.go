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
