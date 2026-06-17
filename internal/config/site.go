package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SiteConfig holds the site configuration loaded from config/site.json
type SiteConfig struct {
	SiteName string      `json:"site_name"`
	SiteURL  string      `json:"site_url"`
	Mail     MailConfig  `json:"mail"`
	Legal    LegalConfig `json:"legal"`
}

// MailConfig customizes transactional email appearance (password reset, etc.).
type MailConfig struct {
	// BrandColor is the primary button/header accent (CSS hex, e.g. "#4f46e5").
	BrandColor string `json:"brand_color"`
	// LogoURL is an absolute URL to a logo image shown in HTML emails. Empty uses {PublicOrigin}/static/brand/logos/logo-email.png when sending.
	LogoURL string `json:"logo_url"`
	// Footer is optional extra text appended at the bottom of emails (e.g. support contact).
	Footer string `json:"footer"`
}

// LegalConfig holds legal notice information
type LegalConfig struct {
	OwnerName       string          `json:"owner_name"`
	OwnerEmail      string          `json:"owner_email"`
	OwnerAddress    string          `json:"owner_address"`
	HostingProvider string          `json:"hosting_provider"`
	HostingAddress  string          `json:"hosting_address"`
	DataRetention   string          `json:"data_retention"`
	DataUsage       string          `json:"data_usage"`
	CustomSections  []CustomSection `json:"custom_sections"`
}

// CustomSection allows adding custom legal sections
type CustomSection struct {
	TitleFR   string `json:"title_fr"`
	TitleEN   string `json:"title_en"`
	ContentFR string `json:"content_fr"`
	ContentEN string `json:"content_en"`
}

// DefaultSiteConfig returns a default configuration
func DefaultSiteConfig() *SiteConfig {
	return &SiteConfig{
		SiteName: "BookStorage",
		SiteURL:  "",
		Legal: LegalConfig{
			OwnerName:       "",
			OwnerEmail:      "",
			OwnerAddress:    "",
			HostingProvider: "",
			HostingAddress:  "",
			DataRetention:   "",
			DataUsage:       "",
			CustomSections:  []CustomSection{},
		},
	}
}

// LoadSiteConfig loads the site configuration from config/site.json
func LoadSiteConfig(rootPath string) *SiteConfig {
	configPath := filepath.Join(rootPath, "config", "site.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultSiteConfig()
	}

	var cfg SiteConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultSiteConfig()
	}

	return &cfg
}

// MailLogoURL returns the logo URL for HTML emails. Uses mail.logo_url when set,
// otherwise {publicOrigin}/static/brand/logos/logo-email.png when publicOrigin is non-empty.
func (c *SiteConfig) MailLogoURL(publicOrigin string) string {
	if c != nil {
		if u := strings.TrimSpace(c.Mail.LogoURL); u != "" {
			return u
		}
	}
	origin := strings.TrimRight(strings.TrimSpace(publicOrigin), "/")
	if origin == "" && c != nil {
		origin = strings.TrimRight(strings.TrimSpace(c.SiteURL), "/")
	}
	if origin == "" {
		return ""
	}
	return origin + "/static/brand/logos/logo-email.png"
}
