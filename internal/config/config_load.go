package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSecretKey             = "dev-secret-change-me"
	defaultDatabaseName          = "database.db"
	defaultUploadDir             = "static/images"
	defaultAvatarDir             = "static/avatars"
	defaultUploadURLPath         = "images"
	defaultAvatarURLPath         = "avatars"
	defaultSuperadminUser        = "superadmin"
	defaultSuperadminPass        = "SuperAdmin!2023"
	documentedWeakSuperadminPass = "ChangeThisPassword!"
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

	postgresURL := strings.TrimSpace(os.Getenv("BOOKSTORAGE_POSTGRES_URL"))
	if postgresURL != "" {
		var nerr error
		postgresURL, nerr = NormalizePostgresURLForLibPQ(postgresURL)
		if nerr != nil {
			return nil, fmt.Errorf("BOOKSTORAGE_POSTGRES_URL: %w", nerr)
		}
	}

	var dbPath string
	if postgresURL == "" {
		var err error
		dbPath, err = resolveFile(
			root,
			dataDir,
			os.Getenv("BOOKSTORAGE_DATABASE"),
			defaultDatabaseName,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve database: %w", err)
		}
	} else {
		dbPath = filepath.Join(dataDir, defaultDatabaseName)
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
		PostgresURL:              postgresURL,
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
		TrustProxy:               envBoolOr("BOOKSTORAGE_TRUST_PROXY", false),
		PublicOrigin:             publicOrigin,
		GoogleClientID:           strings.TrimSpace(os.Getenv("BOOKSTORAGE_GOOGLE_CLIENT_ID")),
		GoogleClientSecret:       strings.TrimSpace(os.Getenv("BOOKSTORAGE_GOOGLE_CLIENT_SECRET")),
		MailjetAPIKeyPublic:      strings.TrimSpace(os.Getenv("BOOKSTORAGE_MAILJET_API_KEY_PUBLIC")),
		MailjetAPIKeyPrivate:     strings.TrimSpace(os.Getenv("BOOKSTORAGE_MAILJET_API_KEY_PRIVATE")),
		MailFrom:                 strings.TrimSpace(os.Getenv("BOOKSTORAGE_MAIL_FROM")),
		Timezone:                 detectTimezone(),
	}
	if err := validateSettings(s); err != nil {
		return nil, err
	}
	return s, nil
}

// detectTimezone returns the IANA timezone to use for display.
// Priority: BOOKSTORAGE_TIMEZONE env > TZ env > system detection > "UTC".
func detectTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("BOOKSTORAGE_TIMEZONE")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if name := time.Now().Location().String(); name != "" && name != "Local" {
		return name
	}
	if tz := detectTimezoneFromSystem(); tz != "" {
		return tz
	}
	return "UTC"
}

// detectTimezoneFromSystem reads the IANA timezone name from OS-level config.
// On Linux: /etc/timezone or the /etc/localtime symlink target.
func detectTimezoneFromSystem() string {
	// Debian/Ubuntu store the name in /etc/timezone.
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := strings.TrimSpace(string(b)); tz != "" {
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}
	// RHEL/Fedora/Arch: /etc/localtime is a symlink into /usr/share/zoneinfo/.
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		const prefix = "/usr/share/zoneinfo/"
		if idx := strings.Index(target, prefix); idx != -1 {
			tz := target[idx+len(prefix):]
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}
	return ""
}
