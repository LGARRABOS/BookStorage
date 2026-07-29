package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
	"bookstorage/internal/server"
	"encoding/json"
)

// Version is set at compile time with -ldflags
var Version = "dev"

const (
	appName        = "BookStorage"
	appDescription = "Personal reading tracker"
)

func printHelp() {
	fmt.Printf(`
%s v%s - %s

USAGE
    %s [options]

OPTIONS
    -h, --help      Show this help
    -v, --version   Show version
    -c, --config    Path to .env file (default: .env)

ENVIRONMENT VARIABLES
    BOOKSTORAGE_HOST                 Listen address (default: 127.0.0.1)
    BOOKSTORAGE_PORT                 Port (default: 5000)
    BOOKSTORAGE_DATABASE             SQLite database path (default: database.db) when BOOKSTORAGE_POSTGRES_URL is unset
    BOOKSTORAGE_POSTGRES_URL         Optional PostgreSQL URL (postgres://user:pass@host:port/db?sslmode=...)
    BOOKSTORAGE_SECRET_KEY           Secret key for sessions
    BOOKSTORAGE_SUPERADMIN_USERNAME  Super admin username (default: superadmin)
    BOOKSTORAGE_SUPERADMIN_PASSWORD  Super admin password
    BOOKSTORAGE_PUBLIC_ORIGIN         Public site URL without trailing slash (required for Google OAuth), e.g. https://books.example.com
    BOOKSTORAGE_GOOGLE_CLIENT_ID      Google OAuth 2.0 Web client ID (optional; with secret and public origin enables Sign in with Google)
    BOOKSTORAGE_GOOGLE_CLIENT_SECRET  Google OAuth client secret
    BOOKSTORAGE_GOOGLE_BOOKS_API_KEY  Optional Google Books API key (higher quota for BD cover lookup)
    BOOKSTORAGE_HTTP_READ_TIMEOUT_SEC  Seconds to read the full request (default 15)
    BOOKSTORAGE_HTTP_WRITE_TIMEOUT_SEC Seconds until response must be fully written (default 120; includes handler time â€” raise for slow admin batches)

EXAMPLES
    # Run with default settings
    %s

    # Run with custom config file
    %s -c /etc/bookstorage/.env

    # Run with environment variables
    BOOKSTORAGE_PORT=8080 %s

SYSTEMD SERVICE
    sudo systemctl start bookstorage    # Start
    sudo systemctl stop bookstorage     # Stop
    sudo systemctl status bookstorage   # Status
    sudo journalctl -u bookstorage -f   # Logs

MORE INFO
    https://github.com/LGARRABOS/BookStorage

`, appName, Version, appDescription, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func printVersion() {
	fmt.Printf("%s v%s\n", appName, Version)
}

// httpTimeoutSeconds parses BOOKSTORAGE_HTTP_*_TIMEOUT_SEC; invalid or empty uses defaultSec.
func httpTimeoutSeconds(envKey string, defaultSec int) time.Duration {
	v := strings.TrimSpace(os.Getenv(envKey))
	if v == "" {
		return time.Duration(defaultSec) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(defaultSec) * time.Second
	}
	return time.Duration(n) * time.Second
}

func main() {
	startedAt := time.Now().UTC()
	// Flags
	var (
		showHelp    bool
		showVersion bool
		configPath  string
	)

	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.StringVar(&configPath, "config", "", "Path to .env file")
	flag.StringVar(&configPath, "c", "", "Path to .env file")

	// Custom parser to not show help by default
	flag.Usage = printHelp
	flag.Parse()

	if showHelp {
		printHelp()
		os.Exit(0)
	}

	if showVersion {
		printVersion()
		os.Exit(0)
	}

	// Determine root directory
	root := "."
	if configPath != "" {
		root = filepath.Dir(configPath)
	}

	envFile := config.ResolveEnvFilePath(root, configPath)
	_, err := config.LoadDotEnvFile(envFile)
	if err != nil {
		log.Fatalf("load .env: %v", err)
	}

	settings, err := config.Load(root)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	// Always record resolved .env path so admin SQLite â†’ Postgres migration can merge BOOKSTORAGE_POSTGRES_URL
	// even when the file was missing or unreadable at startup (LoadDotEnvFile skips without error).
	settings.EnvFilePath = envFile

	siteConfig := config.LoadSiteConfig(root)

	db, err := database.Open(settings)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := database.EnsureSchema(db, settings); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	app := server.NewApp(settings, siteConfig, db, Version)
	app.ProcessStartedAt = startedAt

	// Link existing works to their reading sites (one-time backfill at startup).
	app.BackfillReadingSiteIDs()

	mux := http.NewServeMux()

	// Static files (bundled assets + uploads from configured dirs, not only process cwd)
	mux.Handle("/static/", server.StaticFilesHandler(settings))

	// Routes
	mux.HandleFunc("/metrics", app.HandleMetrics)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		dbOK := true
		if err := db.QueryRow("SELECT 1").Scan(new(int)); err != nil {
			dbOK = false
		}
		payload := map[string]any{
			"ok":         dbOK,
			"version":    Version,
			"uptime_sec": int(time.Since(startedAt).Seconds()),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	mux.HandleFunc("/", app.HandleHome)
	mux.HandleFunc("/hub", app.RequireLogin(app.HandleHub))
	mux.HandleFunc("/legal", app.MobileRedirectToMangaDashboard(app.HandleLegal))
	mux.HandleFunc("/lang/{lang}", app.HandleSetLanguage)
	mux.HandleFunc("/register", app.HandleRegister)
	mux.HandleFunc("/login", app.HandleLogin)
	mux.HandleFunc("/forgot-password", app.HandleForgotPassword)
	mux.HandleFunc("/reset-password", app.HandleResetPassword)
	mux.HandleFunc("/auth/google", app.HandleGoogleOAuthStart)
	mux.HandleFunc("/auth/google/callback", app.HandleGoogleOAuthCallback)
	mux.HandleFunc("/auth/google/link", app.RequireLogin(app.HandleGoogleOAuthLink))
	mux.HandleFunc("/logout", app.HandleLogout)
	mux.HandleFunc("GET /api/session/ping", app.HandleAPISessionPing)
	mux.HandleFunc("/profile", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleProfile)))
	mux.HandleFunc("/profile/passkeys", app.RequireLogin(app.HandleProfilePasskeys))
	mux.HandleFunc("POST /profile/logout_all", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleLogoutAll)))
	mux.HandleFunc("POST /profile/reset_reading_activity", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleProfileResetReadingActivity)))
	mux.HandleFunc("POST /profile/blocklist/add", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleProfileBlocklistAdd)))
	mux.HandleFunc("POST /profile/blocklist/remove", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleProfileBlocklistRemove)))
	mux.HandleFunc("POST /profile/google/unlink", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleGoogleUnlink)))
	mux.HandleFunc("POST /profile/delete", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleDeleteProfile)))
	mux.HandleFunc("POST /profile/api-tokens", app.RequireLogin(app.HandleCreateAPIToken))
	mux.HandleFunc("POST /profile/api-tokens/revoke/{id}", app.RequireLogin(app.HandleRevokeAPIToken))
	mux.HandleFunc("POST /profile/webhooks", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleCreateWebhook)))
	mux.HandleFunc("POST /profile/webhooks/{id}", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleUpdateWebhook)))
	mux.HandleFunc("POST /profile/webhooks/{id}/delete", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleDeleteWebhook)))
	mux.HandleFunc("POST /profile/webhooks/{id}/test", app.RequireLogin(app.MobileRedirectToMangaDashboard(app.HandleTestWebhook)))
	mux.HandleFunc("GET /api/reading-sites/match", app.RequireLogin(app.HandleAPIReadingSiteMatch))
	mux.HandleFunc("GET /api/reading-sites", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksRead)(app.HandleAPIReadingSitesList)))
	mux.HandleFunc("/api/catalog/browse", app.RequireLogin(app.HandleCatalogBrowse))
	mux.HandleFunc("/api/catalog/search", app.RequireLogin(app.HandleCatalogSearch))
	mux.HandleFunc("GET /api/recommendations", app.RequireLogin(app.HandleRecommendations))
	mux.HandleFunc("POST /api/recommendations/dismiss", app.RequireLogin(app.HandleDismissRecommendation))
	mux.HandleFunc("GET /api/recommendations/media", app.RequireLogin(app.HandleRecommendationMedia))
	mux.HandleFunc("GET /api/works", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksRead)(app.HandleAPIWorksList)))
	mux.HandleFunc("GET /api/works/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksRead)(app.HandleAPIWorksDetail)))
	mux.HandleFunc("POST /api/works", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleAPIWorksCreate)))
	mux.HandleFunc("POST /api/works/bulk", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleAPIWorksBulk)))
	mux.HandleFunc("PATCH /api/works/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleAPIWorksUpdate)))
	mux.HandleFunc("DELETE /api/works/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleAPIWorksDelete)))
	mux.HandleFunc("GET /api/stats", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksRead)(app.HandleAPIStats)))
	mux.HandleFunc("POST /api/increment/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleIncrement)))
	mux.HandleFunc("POST /api/decrement/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleDecrement)))
	mux.HandleFunc("POST /api/set-chapter/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleSetChapter)))
	mux.HandleFunc("POST /api/delete/{id}", app.RequireLogin(app.RequireAPIScope(server.ScopeWorksWrite)(app.HandleDeleteWorkAPI)))
	mux.HandleFunc("GET /api/anime/catalog/browse", app.RequireLogin(app.HandleAnimeCatalogBrowse))
	mux.HandleFunc("GET /api/anime/catalog/search", app.RequireLogin(app.HandleAnimeCatalogSearch))
	mux.HandleFunc("POST /api/anime/increment/{id}", app.RequireLogin(app.HandleAnimeIncrement))
	mux.HandleFunc("POST /api/anime/decrement/{id}", app.RequireLogin(app.HandleAnimeDecrement))
	mux.HandleFunc("POST /api/anime/set-episode/{id}", app.RequireLogin(app.HandleAnimeSetEpisode))
	mux.HandleFunc("POST /api/anime/delete/{id}", app.RequireLogin(app.HandleAnimeDeleteAPI))
	mux.HandleFunc("GET /api/bd/catalog/browse", app.RequireLogin(app.HandleBdCatalogBrowse))
	mux.HandleFunc("GET /api/bd/catalog/search", app.RequireLogin(app.HandleBdCatalogSearch))
	mux.HandleFunc("POST /api/bd/delete/{id}", app.RequireLogin(app.HandleBdDeleteAPI))
	mux.HandleFunc("POST /api/manga-phys/delete/{id}", app.RequireLogin(app.HandleMangaPhysDeleteAPI))
	mux.HandleFunc("GET /api/library/furniture/{id}", app.RequireLogin(app.HandleAPILibraryFurniture))
	mux.HandleFunc("POST /api/library/placements", app.RequireLogin(app.HandleAPILibraryPlacementsCreate))
	mux.HandleFunc("PATCH /api/library/placements/{id}", app.RequireLogin(app.HandleAPILibraryPlacementsPatch))
	mux.HandleFunc("POST /api/library/placements/{id}", app.RequireLogin(app.HandleAPILibraryPlacementsPatch))
	mux.HandleFunc("DELETE /api/library/placements/{id}", app.RequireLogin(app.HandleAPILibraryPlacementsDelete))
	mux.HandleFunc("POST /api/library/placements/{id}/delete", app.RequireLogin(app.HandleAPILibraryPlacementsDelete))
	mux.HandleFunc("GET /api/library/search", app.RequireLogin(app.HandleAPILibrarySearch))
	mux.HandleFunc("GET /api/library/works", app.RequireLogin(app.HandleAPILibraryWorksSearch))
	mux.HandleFunc("/admin/accounts", app.RequireAdmin(app.MobileRedirectToMangaDashboard(app.HandleAdminAccounts)))
	mux.HandleFunc("/admin/database", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminDatabase)))
	mux.HandleFunc("/admin/migrate-postgres", app.RequireAdmin(app.RequireSuperadmin(app.RequireWebOnly(app.HandleAdminMigratePostgres))))
	mux.HandleFunc("POST /api/admin/migrate-postgres/test", app.RequireAdmin(app.RequireSuperadmin(app.RequireWebOnly(app.HandleAPIAdminMigratePostgresTest))))
	mux.HandleFunc("POST /api/admin/migrate-postgres/run", app.RequireAdmin(app.RequireSuperadmin(app.RequireWebOnly(app.HandleAPIAdminMigratePostgresRun))))
	mux.HandleFunc("POST /api/admin/database/delete", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIAdminDatabaseDelete)))
	mux.HandleFunc("POST /admin/approve/{id}", app.RequireAdmin(app.MobileRedirectToMangaDashboard(app.HandleApproveAccount)))
	mux.HandleFunc("POST /admin/delete_account/{id}", app.RequireAdmin(app.MobileRedirectToMangaDashboard(app.HandleDeleteAccount)))
	mux.HandleFunc("POST /admin/promote/{id}", app.RequireAdmin(app.RequireSuperadmin(app.MobileRedirectToMangaDashboard(app.HandlePromoteAccount))))
	mux.HandleFunc("/admin/backups", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminBackups)))
	mux.HandleFunc("/admin/audit", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminAuditLog)))
	mux.HandleFunc("/admin/jobs", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminJobs)))
	mux.HandleFunc("POST /admin/jobs/covers/run", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminJobsRunCovers)))
	mux.HandleFunc("POST /admin/jobs/bd-covers/run", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminJobsRunBdCovers)))
	mux.HandleFunc("POST /admin/jobs/bd-covers/replace", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminJobsReplaceBdCovers)))
	mux.HandleFunc("GET /api/admin/instance-stats", app.RequireAdmin(app.HandleAPIAdminInstanceStats))
	mux.HandleFunc("GET /api/admin/jobs", app.RequireAdmin(app.HandleAPIAdminJobs))
	mux.HandleFunc("POST /auth/webauthn/register/begin", app.RequireLogin(app.HandleWebAuthnRegisterBegin))
	mux.HandleFunc("POST /auth/webauthn/register/finish", app.RequireLogin(app.HandleWebAuthnRegisterFinish))
	mux.HandleFunc("POST /auth/webauthn/login/begin", app.HandleWebAuthnLoginBegin)
	mux.HandleFunc("POST /auth/webauthn/login/finish", app.HandleWebAuthnLoginFinish)
	mux.HandleFunc("POST /profile/webauthn/delete/{id}", app.RequireLogin(app.HandleWebAuthnDelete))

	// Manga/Webtoon module pages live under /manga/; old flat URLs 308-redirect.
	app.RegisterMangaRoutes(mux)
	app.RegisterLegacyRedirects(mux)

	// Hub-level Tools (/tools) + section tools + anime import/export.
	app.RegisterToolsRoutes(mux)

	// Anime module pages live under /anime/.
	app.RegisterAnimeRoutes(mux)

	// BD (bande dessinÃ©e) module pages live under /bd/.
	app.RegisterBdRoutes(mux)
	app.RegisterMangaPhysRoutes(mux)

	// Physical library module pages live under /library/.
	app.RegisterLibraryRoutes(mux)

	// Background prober: check all reading sites every 5 minutes.
	proberCtx, proberCancel := context.WithCancel(context.Background())
	defer proberCancel()
	app.StartBackgroundProber(proberCtx, 5*time.Minute)
	app.StartWebhookWorker(proberCtx)

	addr := settings.Host + ":" + strconv.Itoa(settings.Port)
	log.Printf("%s v%s listening on %s (%s)", appName, Version, addr, settings.Environment)
	handler := app.WithAccessLog(app.WithRequestID(app.SecurityHeaders(app.WithErrorPages(app.WithDatabaseUnavailable(app.WithRequestPolicies(app.WithAPITokenContext(app.WithAPITokenRoutePolicy(mux))))))))
	readTO := httpTimeoutSeconds("BOOKSTORAGE_HTTP_READ_TIMEOUT_SEC", 15)
	writeTO := httpTimeoutSeconds("BOOKSTORAGE_HTTP_WRITE_TIMEOUT_SEC", 120)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTO,
		WriteTimeout:      writeTO,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
