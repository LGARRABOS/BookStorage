package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Settings holds application configuration
type Settings struct {
	SecretKey string
	// Database is the SQLite database file path when not using PostgreSQL.
	Database string
	// PostgresURL is a libpq-style URL (postgres:// or postgresql://). When non-empty, the app uses PostgreSQL instead of SQLite.
	PostgresURL string
	// EnvFilePath is the absolute path to the loaded .env file (for admin migration merge). May be empty.
	EnvFilePath          string
	DataDirectory        string
	UploadFolder         string
	UploadURLPath        string
	ProfileUploadFolder  string
	ProfileUploadURLPath string
	// WebStaticRoot is the app "static/" bundle directory (CSS, JS, icons). Used to serve /static/* together with upload folders.
	WebStaticRoot      string
	SuperadminUsername string
	SuperadminPassword string
	Environment        string
	Host               string
	Port               int
	EnableHSTS         bool
	// RequireAccountValidation controls whether non-admin accounts must be approved (validated=1) before login.
	RequireAccountValidation bool
	// TranslateURL is a LibreTranslate-compatible API base URL (no trailing slash), e.g. https://libretranslate.com — empty disables auto-translation.
	TranslateURL    string
	TranslateAPIKey string
	// MetricsToken, if non-empty, protects GET /metrics (Authorization: Bearer only). If empty, /metrics is only reachable from loopback clients.
	MetricsToken string
	// TrustProxy uses X-Forwarded-For as client IP for rate limiting when true (set behind a trusted reverse proxy).
	TrustProxy bool
	// PublicOrigin is the public base URL without trailing slash (e.g. https://books.example.com). Required for Google OAuth redirect_uri.
	PublicOrigin string
	// GoogleClientID / GoogleClientSecret enable Sign in with Google when set with PublicOrigin.
	GoogleClientID     string
	GoogleClientSecret string
	// MailjetAPIKeyPublic / MailjetAPIKeyPrivate / MailFrom enable transactional email when set with PublicOrigin.
	MailjetAPIKeyPublic  string
	MailjetAPIKeyPrivate string
	MailFrom             string
	// Timezone is the IANA timezone name used to display times in the web UI (e.g. "Europe/Paris"). Defaults to UTC.
	Timezone string
}

// UsePostgres reports whether BOOKSTORAGE_POSTGRES_URL is set and PostgreSQL should be used.
func (s *Settings) UsePostgres() bool {
	return s != nil && strings.TrimSpace(s.PostgresURL) != ""
}

// NormalizePostgresURLForLibPQ rewrites sslmode values that github.com/lib/pq rejects (e.g. "prefer", "allow").
// Supported modes: disable, require, verify-ca, verify-full, or empty (driver default).
func NormalizePostgresURLForLibPQ(dsn string) (string, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	mode := strings.ToLower(strings.TrimSpace(q.Get("sslmode")))
	switch mode {
	case "", "disable", "require", "verify-ca", "verify-full":
		return dsn, nil
	case "prefer", "allow":
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
		return u.String(), nil
	default:
		return "", fmt.Errorf("unsupported sslmode %q (lib/pq accepts disable, require, verify-ca, verify-full)", mode)
	}
}

// GoogleOAuthConfigured reports whether Google OAuth routes should be active.
func (s *Settings) GoogleOAuthConfigured() bool {
	if s == nil {
		return false
	}
	return strings.TrimSpace(s.PublicOrigin) != "" &&
		strings.TrimSpace(s.GoogleClientID) != "" &&
		strings.TrimSpace(s.GoogleClientSecret) != ""
}

// MailConfigured reports whether transactional email (password reset) should be active.
func (s *Settings) MailConfigured() bool {
	if s == nil {
		return false
	}
	return strings.TrimSpace(s.PublicOrigin) != "" &&
		strings.TrimSpace(s.MailjetAPIKeyPublic) != "" &&
		strings.TrimSpace(s.MailjetAPIKeyPrivate) != "" &&
		strings.TrimSpace(s.MailFrom) != ""
}

// MinProductionSecretKeyLen is the minimum length for BOOKSTORAGE_SECRET_KEY in production.
const MinProductionSecretKeyLen = 32

// MinProductionSuperadminPasswordLen is the minimum length for BOOKSTORAGE_SUPERADMIN_PASSWORD in production.
const MinProductionSuperadminPasswordLen = 12

func isLoopbackListenHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isWeakSuperadminPassword(password string) bool {
	password = strings.TrimSpace(password)
	if password == "" {
		return true
	}
	if password == defaultSuperadminPass || password == documentedWeakSuperadminPass {
		return true
	}
	if len(password) < MinProductionSuperadminPasswordLen {
		return true
	}
	var hasUpper, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()-_=+[]{}|;:',.<>?/`~\"\\", r):
			hasSymbol = true
		}
	}
	return !hasUpper || !hasDigit || !hasSymbol
}

func isPostgresHostLANSafe(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// Single-label names (e.g. postgres, BookStorageDB via /etc/hosts) are typical on self-hosted LANs.
	if !strings.Contains(host, ".") {
		return true
	}
	return false
}

func validateSettings(s *Settings) error {
	googleSpecific := 0
	if strings.TrimSpace(s.GoogleClientID) != "" {
		googleSpecific++
	}
	if strings.TrimSpace(s.GoogleClientSecret) != "" {
		googleSpecific++
	}
	if googleSpecific != 0 && googleSpecific != 2 {
		return fmt.Errorf("google OAuth requires both BOOKSTORAGE_GOOGLE_CLIENT_ID and BOOKSTORAGE_GOOGLE_CLIENT_SECRET")
	}
	if googleSpecific == 2 && strings.TrimSpace(s.PublicOrigin) == "" {
		return fmt.Errorf("google OAuth requires BOOKSTORAGE_PUBLIC_ORIGIN when Google client credentials are set")
	}

	mailSpecific := 0
	if strings.TrimSpace(s.MailjetAPIKeyPublic) != "" {
		mailSpecific++
	}
	if strings.TrimSpace(s.MailjetAPIKeyPrivate) != "" {
		mailSpecific++
	}
	if strings.TrimSpace(s.MailFrom) != "" {
		mailSpecific++
	}
	if mailSpecific != 0 && mailSpecific != 3 {
		return fmt.Errorf("mail requires all of BOOKSTORAGE_MAILJET_API_KEY_PUBLIC, BOOKSTORAGE_MAILJET_API_KEY_PRIVATE, and BOOKSTORAGE_MAIL_FROM")
	}
	if mailSpecific == 3 && strings.TrimSpace(s.PublicOrigin) == "" {
		return fmt.Errorf("mail requires BOOKSTORAGE_PUBLIC_ORIGIN when Mailjet keys and BOOKSTORAGE_MAIL_FROM are set")
	}

	if strings.TrimSpace(s.PostgresURL) != "" {
		u, err := url.Parse(s.PostgresURL)
		if err != nil || u.Scheme == "" {
			return fmt.Errorf("BOOKSTORAGE_POSTGRES_URL must be a valid URL")
		}
		if strings.ToLower(u.Scheme) != "postgres" && strings.ToLower(u.Scheme) != "postgresql" {
			return fmt.Errorf("BOOKSTORAGE_POSTGRES_URL scheme must be postgres or postgresql")
		}
	}

	isProduction := strings.ToLower(s.Environment) == "production"
	enforceSecrets := isProduction || !isLoopbackListenHost(s.Host)
	if enforceSecrets {
		secretCtx := "when BOOKSTORAGE_ENV=production"
		if !isProduction {
			secretCtx = "when BOOKSTORAGE_HOST is not loopback (127.0.0.1 or localhost)"
		}
		if s.SecretKey == "" || s.SecretKey == defaultSecretKey {
			return fmt.Errorf("BOOKSTORAGE_SECRET_KEY must be set to a non-default value %s", secretCtx)
		}
		if len(s.SecretKey) < MinProductionSecretKeyLen {
			return fmt.Errorf("BOOKSTORAGE_SECRET_KEY must be at least %d bytes %s", MinProductionSecretKeyLen, secretCtx)
		}
		if isWeakSuperadminPassword(s.SuperadminPassword) {
			return fmt.Errorf("BOOKSTORAGE_SUPERADMIN_PASSWORD must be a strong non-default password (>= %d chars, upper, digit, symbol) %s", MinProductionSuperadminPasswordLen, secretCtx)
		}
	}

	if !isProduction {
		return nil
	}
	if strings.TrimSpace(s.PostgresURL) != "" {
		u, err := url.Parse(s.PostgresURL)
		if err == nil && u.Host != "" {
			mode := strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
			if mode == "" || mode == "disable" {
				if !isPostgresHostLANSafe(u.Hostname()) {
					return fmt.Errorf("BOOKSTORAGE_POSTGRES_URL must use sslmode=require (or verify-ca/verify-full) for hosts reachable over the public Internet when BOOKSTORAGE_ENV=production")
				}
			}
		}
	}
	if s.GoogleOAuthConfigured() {
		u, err := url.Parse(s.PublicOrigin)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("BOOKSTORAGE_PUBLIC_ORIGIN must be an https URL with host when BOOKSTORAGE_ENV=production and Google OAuth is configured")
		}
	}
	if s.MailConfigured() {
		u, err := url.Parse(s.PublicOrigin)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("BOOKSTORAGE_PUBLIC_ORIGIN must be an https URL with host when BOOKSTORAGE_ENV=production and mail is configured")
		}
	}
	return nil
}
