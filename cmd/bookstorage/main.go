package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
    BOOKSTORAGE_DATABASE             SQLite database path (default: database.db)
    BOOKSTORAGE_SECRET_KEY           Secret key for sessions
    BOOKSTORAGE_SUPERADMIN_USERNAME  Super admin username (default: superadmin)
    BOOKSTORAGE_SUPERADMIN_PASSWORD  Super admin password

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

	settings, err := config.Load(root)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

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
	mux.HandleFunc("/legal", app.MobileRedirectToDashboard(app.HandleLegal))
	mux.HandleFunc("/lang/{lang}", app.HandleSetLanguage)
	mux.HandleFunc("/register", app.HandleRegister)
	mux.HandleFunc("/login", app.HandleLogin)
	mux.HandleFunc("/logout", app.HandleLogout)
	mux.HandleFunc("/dashboard", app.RequireLogin(app.HandleDashboard))
	mux.HandleFunc("/stats", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleStats)))
	mux.HandleFunc("/profile", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleProfile)))
	mux.HandleFunc("POST /profile/logout_all", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleLogoutAll)))
	mux.HandleFunc("POST /profile/delete", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleDeleteProfile)))
	mux.HandleFunc("/tools", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleTools)))
	mux.HandleFunc("/tools/csv-import", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleToolsCSVImport)))
	mux.HandleFunc("/tools/duplicates", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleDuplicates)))
	mux.HandleFunc("POST /tools/duplicates/merge", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleMergeDuplicate)))
	mux.HandleFunc("/users", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleUsers)))
	mux.HandleFunc("/users/{id}", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleUserDetail)))
	mux.HandleFunc("POST /users/{user_id}/import/{work_id}", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleImportWork)))
	mux.HandleFunc("/add_work", app.RequireLogin(app.HandleAddWork))
	mux.HandleFunc("/api/catalog/search", app.RequireLogin(app.HandleCatalogSearch))
	mux.HandleFunc("GET /api/recommendations", app.RequireLogin(app.HandleRecommendations))
	mux.HandleFunc("POST /api/recommendations/dismiss", app.RequireLogin(app.HandleDismissRecommendation))
	mux.HandleFunc("GET /api/recommendations/media", app.RequireLogin(app.HandleRecommendationMedia))
	mux.HandleFunc("GET /api/works", app.RequireLogin(app.HandleAPIWorksList))
	mux.HandleFunc("GET /api/works/{id}", app.RequireLogin(app.HandleAPIWorksDetail))
	mux.HandleFunc("POST /api/works", app.RequireLogin(app.HandleAPIWorksCreate))
	mux.HandleFunc("PATCH /api/works/{id}", app.RequireLogin(app.HandleAPIWorksUpdate))
	mux.HandleFunc("DELETE /api/works/{id}", app.RequireLogin(app.HandleAPIWorksDelete))
	mux.HandleFunc("GET /api/stats", app.RequireLogin(app.HandleAPIStats))
	mux.HandleFunc("/edit/{id}", app.RequireLogin(app.HandleEditWork))
	mux.HandleFunc("POST /api/increment/{id}", app.RequireLogin(app.HandleIncrement))
	mux.HandleFunc("POST /api/decrement/{id}", app.RequireLogin(app.HandleDecrement))
	mux.HandleFunc("POST /api/set-chapter/{id}", app.RequireLogin(app.HandleSetChapter))
	mux.HandleFunc("/delete/{id}", app.RequireLogin(app.HandleDeleteWork))
	mux.HandleFunc("POST /api/delete/{id}", app.RequireLogin(app.HandleDeleteWorkAPI))
	mux.HandleFunc("/export", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleExport)))
	mux.HandleFunc("POST /import", app.RequireLogin(app.MobileRedirectToDashboard(app.HandleImport)))
	mux.HandleFunc("/admin/accounts", app.RequireAdmin(app.MobileRedirectToDashboard(app.HandleAdminAccounts)))
	mux.HandleFunc("/admin/monitoring", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminMonitoring)))
	mux.HandleFunc("/admin/database", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminDatabase)))
	mux.HandleFunc("/admin/enrich", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminEnrich)))
	mux.HandleFunc("POST /api/admin/enrich/run", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIAdminEnrichRun)))
	mux.HandleFunc("GET /api/admin/prometheus/summary", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIAdminPrometheusSummary)))
	mux.HandleFunc("/admin/update", app.RequireAdmin(app.RequireWebOnly(app.HandleAdminUpdate)))
	mux.HandleFunc("POST /api/admin/update/latest", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIUpdateLatest)))
	mux.HandleFunc("POST /api/admin/update/latest-major", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIUpdateLatestMajor)))
	mux.HandleFunc("GET /api/admin/update/status", app.RequireAdmin(app.RequireWebOnly(app.HandleAPIUpdateStatus)))
	mux.HandleFunc("/admin/approve/{id}", app.RequireAdmin(app.MobileRedirectToDashboard(app.HandleApproveAccount)))
	mux.HandleFunc("/admin/delete_account/{id}", app.RequireAdmin(app.MobileRedirectToDashboard(app.HandleDeleteAccount)))
	mux.HandleFunc("/admin/promote/{id}", app.RequireAdmin(app.MobileRedirectToDashboard(app.HandlePromoteAccount)))

	addr := settings.Host + ":" + strconv.Itoa(settings.Port)
	log.Printf("%s v%s listening on %s (%s)", appName, Version, addr, settings.Environment)
	handler := app.WithAccessLog(app.WithRequestID(app.SecurityHeaders(app.WithErrorPages(app.WithRequestPolicies(mux)))))
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
