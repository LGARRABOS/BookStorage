package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SiteConfig holds the site configuration loaded from config/site.json
type SiteConfig struct {
	SiteName string      `json:"site_name"`
	SiteURL  string      `json:"site_url"`
	Legal    LegalConfig `json:"legal"`
}

// LegalConfig holds legal/mentions légales information
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
