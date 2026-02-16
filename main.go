package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
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
	defer db.Close()

	if err := database.EnsureSchema(db, settings); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	app := NewApp(settings, siteConfig, db)

	mux := http.NewServeMux()

	// Static files
	staticDir := filepath.Join("static")
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Routes
	mux.HandleFunc("/", app.handleHome)
	mux.HandleFunc("/legal", app.handleLegal)
	mux.HandleFunc("/lang/{lang}", app.handleSetLanguage)
	mux.HandleFunc("/register", app.handleRegister)
	mux.HandleFunc("/login", app.handleLogin)
	mux.HandleFunc("/logout", app.handleLogout)
	mux.HandleFunc("/dashboard", app.requireLogin(app.handleDashboard))
	mux.HandleFunc("/stats", app.requireLogin(app.handleStats))
	mux.HandleFunc("/profile", app.requireLogin(app.handleProfile))
	mux.HandleFunc("/users", app.requireLogin(app.handleUsers))
	mux.HandleFunc("/users/{id}", app.requireLogin(app.handleUserDetail))
	mux.HandleFunc("POST /users/{user_id}/import/{work_id}", app.requireLogin(app.handleImportWork))
	mux.HandleFunc("/add_work", app.requireLogin(app.handleAddWork))
	mux.HandleFunc("/api/catalog/search", app.requireLogin(app.handleCatalogSearch))
	mux.HandleFunc("/edit/{id}", app.requireLogin(app.handleEditWork))
	mux.HandleFunc("POST /api/increment/{id}", app.requireLogin(app.handleIncrement))
	mux.HandleFunc("POST /api/decrement/{id}", app.requireLogin(app.handleDecrement))
	mux.HandleFunc("/delete/{id}", app.requireLogin(app.handleDeleteWork))
	mux.HandleFunc("/export", app.requireLogin(app.handleExportCSV))
	mux.HandleFunc("POST /import", app.requireLogin(app.handleImportCSV))
	mux.HandleFunc("/admin/accounts", app.requireAdmin(app.handleAdminAccounts))
	mux.HandleFunc("/admin/approve/{id}", app.requireAdmin(app.handleApproveAccount))
	mux.HandleFunc("/admin/delete_account/{id}", app.requireAdmin(app.handleDeleteAccount))
	mux.HandleFunc("/admin/promote/{id}", app.requireAdmin(app.handlePromoteAccount))

	addr := settings.Host + ":" + strconv.Itoa(settings.Port)
	log.Printf("%s v%s listening on %s (%s)", appName, Version, addr, settings.Environment)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
