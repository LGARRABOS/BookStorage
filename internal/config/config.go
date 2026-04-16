package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Settings holds application configuration
type Settings struct {
	SecretKey            string
	Database             string
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
	// MetricsToken, if non-empty, protects GET /metrics (Bearer or ?token=). If empty, /metrics is only reachable from loopback clients.
	MetricsToken string
	// PrometheusQueryURL is the base URL for Prometheus HTTP API (instant queries) used by the admin monitoring page. Empty defaults to http://127.0.0.1:9091. Host must be loopback.
	PrometheusQueryURL string
	// PublicOrigin is the public base URL without trailing slash (e.g. https://books.example.com). Required for Google OAuth redirect_uri.
	PublicOrigin string
	// GoogleClientID / GoogleClientSecret enable Sign in with Google when set with PublicOrigin.
	GoogleClientID     string
	GoogleClientSecret string
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

// MinProductionSecretKeyLen is the minimum length for BOOKSTORAGE_SECRET_KEY in production.
const MinProductionSecretKeyLen = 32

const (
	defaultSecretKey      = "dev-secret-change-me"
	defaultDatabaseName   = "database.db"
	defaultUploadDir      = "static/images"
	defaultAvatarDir      = "static/avatars"
	defaultUploadURLPath  = "images"
	defaultAvatarURLPath  = "avatars"
	defaultSuperadminUser = "superadmin"
	defaultSuperadminPass = "SuperAdmin!2023"
)

func resolveDirectory(root, candidate, def string) (string, error) {
	target := candidate
	if target == "" {
		target = def
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func resolveFile(root, baseDir, candidate, defaultName string) (string, error) {
	var filePath string
	if candidate != "" && filepath.IsAbs(candidate) {
		filePath = candidate
	} else {
		if candidate == "" {
			candidate = defaultName
		}
		filePath = filepath.Join(baseDir, candidate)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return "", err
	}
	return filePath, nil
}

func envOr(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func envBoolOr(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if strings.EqualFold(raw, "true") || raw == "1" || strings.EqualFold(raw, "yes") || strings.EqualFold(raw, "y") || strings.EqualFold(raw, "on") {
		return true
	}
	if strings.EqualFold(raw, "false") || raw == "0" || strings.EqualFold(raw, "no") || strings.EqualFold(raw, "n") || strings.EqualFold(raw, "off") {
		return false
	}
	return def
}

// Load loads settings from environment variables
func Load(rootPath string) (*Settings, error) {
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	env := strings.ToLower(strings.TrimSpace(os.Getenv("BOOKSTORAGE_ENV")))
	if env == "" {
		env = "development"
	}

	dataDir, err := resolveDirectory(root, os.Getenv("BOOKSTORAGE_DATA_DIR"), ".")
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}

	dbPath, err := resolveFile(
		root,
		dataDir,
		os.Getenv("BOOKSTORAGE_DATABASE"),
		defaultDatabaseName,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve database: %w", err)
	}

	uploadFolder, err := resolveDirectory(root, os.Getenv("BOOKSTORAGE_UPLOAD_DIR"), defaultUploadDir)
	if err != nil {
		return nil, fmt.Errorf("resolve upload dir: %w", err)
	}

	avatarFolder, err := resolveDirectory(root, os.Getenv("BOOKSTORAGE_AVATAR_DIR"), defaultAvatarDir)
	if err != nil {
		return nil, fmt.Errorf("resolve avatar dir: %w", err)
	}

	secret := os.Getenv("BOOKSTORAGE_SECRET_KEY")
	if secret == "" {
		secret = defaultSecretKey
	}

	uploadURL := strings.Trim(strings.TrimSpace(os.Getenv("BOOKSTORAGE_UPLOAD_URL_PATH")), "/")
	if uploadURL == "" {
		uploadURL = defaultUploadURLPath
	}

	avatarURL := strings.Trim(strings.TrimSpace(os.Getenv("BOOKSTORAGE_AVATAR_URL_PATH")), "/")
	if avatarURL == "" {
		avatarURL = defaultAvatarURLPath
	}

	host := strings.TrimSpace(os.Getenv("BOOKSTORAGE_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}

	portStr := os.Getenv("BOOKSTORAGE_PORT")
	if portStr == "" {
		portStr = "5000"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("BOOKSTORAGE_PORT must be a valid integer: %w", err)
	}

	enableHSTS := strings.EqualFold(strings.TrimSpace(os.Getenv("BOOKSTORAGE_ENABLE_HSTS")), "true") ||
		os.Getenv("BOOKSTORAGE_ENABLE_HSTS") == "1"

	webStaticRoot := filepath.Join(root, "static")

	publicOrigin := strings.TrimRight(strings.TrimSpace(os.Getenv("BOOKSTORAGE_PUBLIC_ORIGIN")), "/")

	s := &Settings{
		SecretKey:                secret,
		Database:                 dbPath,
		DataDirectory:            dataDir,
		UploadFolder:             uploadFolder,
		UploadURLPath:            uploadURL,
		ProfileUploadFolder:      avatarFolder,
		ProfileUploadURLPath:     avatarURL,
		WebStaticRoot:            webStaticRoot,
		SuperadminUsername:       envOr("BOOKSTORAGE_SUPERADMIN_USERNAME", defaultSuperadminUser),
		SuperadminPassword:       envOr("BOOKSTORAGE_SUPERADMIN_PASSWORD", defaultSuperadminPass),
		Environment:              env,
		Host:                     host,
		Port:                     port,
		EnableHSTS:               enableHSTS,
		RequireAccountValidation: envBoolOr("BOOKSTORAGE_REQUIRE_ACCOUNT_VALIDATION", true),
		TranslateURL:             strings.TrimSpace(os.Getenv("BOOKSTORAGE_TRANSLATE_URL")),
		TranslateAPIKey:          strings.TrimSpace(os.Getenv("BOOKSTORAGE_TRANSLATE_API_KEY")),
		MetricsToken:             strings.TrimSpace(os.Getenv("BOOKSTORAGE_METRICS_TOKEN")),
		PrometheusQueryURL:       strings.TrimSpace(os.Getenv("BOOKSTORAGE_PROMETHEUS_QUERY_URL")),
		PublicOrigin:             publicOrigin,
		GoogleClientID:           strings.TrimSpace(os.Getenv("BOOKSTORAGE_GOOGLE_CLIENT_ID")),
		GoogleClientSecret:       strings.TrimSpace(os.Getenv("BOOKSTORAGE_GOOGLE_CLIENT_SECRET")),
	}
	if err := validateSettings(s); err != nil {
		return nil, err
	}
	return s, nil
}

func validateSettings(s *Settings) error {
	googleParts := 0
	if strings.TrimSpace(s.PublicOrigin) != "" {
		googleParts++
	}
	if strings.TrimSpace(s.GoogleClientID) != "" {
		googleParts++
	}
	if strings.TrimSpace(s.GoogleClientSecret) != "" {
		googleParts++
	}
	if googleParts != 0 && googleParts != 3 {
		return fmt.Errorf("google OAuth requires all of BOOKSTORAGE_PUBLIC_ORIGIN, BOOKSTORAGE_GOOGLE_CLIENT_ID, and BOOKSTORAGE_GOOGLE_CLIENT_SECRET")
	}

	if strings.ToLower(s.Environment) != "production" {
		return nil
	}
	if s.SecretKey == "" || s.SecretKey == defaultSecretKey {
		return fmt.Errorf("BOOKSTORAGE_SECRET_KEY must be set to a non-default value when BOOKSTORAGE_ENV=production")
	}
	if len(s.SecretKey) < MinProductionSecretKeyLen {
		return fmt.Errorf("BOOKSTORAGE_SECRET_KEY must be at least %d bytes when BOOKSTORAGE_ENV=production", MinProductionSecretKeyLen)
	}
	if s.GoogleOAuthConfigured() {
		u, err := url.Parse(s.PublicOrigin)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("BOOKSTORAGE_PUBLIC_ORIGIN must be an https URL with host when BOOKSTORAGE_ENV=production and Google OAuth is configured")
		}
	}
	return nil
}
