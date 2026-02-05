package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Version est définie à la compilation avec -ldflags
var Version = "dev"

const (
	appName        = "BookStorage"
	appDescription = "Gestionnaire de lectures personnelles"
)

func printHelp() {
	fmt.Printf(`
%s v%s - %s

UTILISATION
    %s [options]

OPTIONS
    -h, --help      Affiche cette aide
    -v, --version   Affiche la version
    -c, --config    Chemin vers le fichier .env (défaut: .env)

VARIABLES D'ENVIRONNEMENT
    BOOKSTORAGE_HOST                 Adresse d'écoute (défaut: 127.0.0.1)
    BOOKSTORAGE_PORT                 Port (défaut: 5000)
    BOOKSTORAGE_DATABASE             Chemin base SQLite (défaut: database.db)
    BOOKSTORAGE_SECRET_KEY           Clé secrète pour les sessions
    BOOKSTORAGE_SUPERADMIN_USERNAME  Nom du super admin (défaut: superadmin)
    BOOKSTORAGE_SUPERADMIN_PASSWORD  Mot de passe super admin

EXEMPLES
    # Lancer avec les paramètres par défaut
    %s

    # Lancer avec un fichier de config personnalisé
    %s -c /etc/bookstorage/.env

    # Lancer avec des variables d'environnement
    BOOKSTORAGE_PORT=8080 %s

SERVICE SYSTEMD
    sudo systemctl start bookstorage    # Démarrer
    sudo systemctl stop bookstorage     # Arrêter
    sudo systemctl status bookstorage   # Statut
    sudo journalctl -u bookstorage -f   # Logs

PLUS D'INFOS
    https://github.com/VOTRE_USERNAME/BookStorage

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

	flag.BoolVar(&showHelp, "help", false, "Affiche l'aide")
	flag.BoolVar(&showHelp, "h", false, "Affiche l'aide")
	flag.BoolVar(&showVersion, "version", false, "Affiche la version")
	flag.BoolVar(&showVersion, "v", false, "Affiche la version")
	flag.StringVar(&configPath, "config", "", "Chemin vers le fichier .env")
	flag.StringVar(&configPath, "c", "", "Chemin vers le fichier .env")

	// Parser personnalisé pour ne pas afficher l'aide par défaut
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

	// Déterminer le répertoire racine
	root := "."
	if configPath != "" {
		root = filepath.Dir(configPath)
	}

	settings, err := GetSettings(root)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err := openDB(settings)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := ensureSchema(db, settings); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	app := NewApp(settings, db)

	mux := http.NewServeMux()

	// fichiers statiques
	staticDir := filepath.Join("static")
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// routes
	mux.HandleFunc("/", app.handleHome)
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
	mux.HandleFunc("/edit/{id}", app.requireLogin(app.handleEditWork))
	mux.HandleFunc("POST /api/increment/{id}", app.requireLogin(app.handleIncrement))
	mux.HandleFunc("POST /api/decrement/{id}", app.requireLogin(app.handleDecrement))
	mux.HandleFunc("/delete/{id}", app.requireLogin(app.handleDeleteWork))
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
