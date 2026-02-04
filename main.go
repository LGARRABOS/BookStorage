package main

import (
	"log"
	"net/http"
	"path/filepath"
	"strconv"
)

func main() {
	root := "."
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
	log.Printf("BookStorage Go server listening on %s (%s)", addr, settings.Environment)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
