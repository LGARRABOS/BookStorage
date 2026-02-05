# ============================================================================
# BookStorage Makefile
# ============================================================================
# Utilisation: make [commande]
# Aide:        make help
# ============================================================================

APP_NAME    := bookstorage
APP_VERSION := 1.0.0
APP_USER    := nobody
APP_GROUP   := nobody
INSTALL_DIR := /opt/bookstorage
BIN_DIR     := /usr/local/bin

# Couleurs pour l'affichage
BLUE    := \033[34m
GREEN   := \033[32m
YELLOW  := \033[33m
RED     := \033[31m
BOLD    := \033[1m
RESET   := \033[0m

.PHONY: all build build-prod run clean install uninstall update fix-perms logs status help

.DEFAULT_GOAL := help

# ============================================================================
# DÉVELOPPEMENT
# ============================================================================

## build: Compile l'application
build:
	@echo "$(BLUE)▶ Compilation...$(RESET)"
	go build -o $(APP_NAME) .
	@echo "$(GREEN)✓ Compilation terminée: ./$(APP_NAME)$(RESET)"

## build-prod: Compile en mode production (binaire optimisé)
build-prod:
	@echo "$(BLUE)▶ Compilation production...$(RESET)"
	CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=$(APP_VERSION)" -o $(APP_NAME) .
	@echo "$(GREEN)✓ Binaire optimisé créé: ./$(APP_NAME)$(RESET)"

## run: Lance le serveur en mode développement
run:
	@echo "$(BLUE)▶ Démarrage du serveur de développement...$(RESET)"
	go run .

## clean: Supprime les fichiers compilés
clean:
	@echo "$(BLUE)▶ Nettoyage...$(RESET)"
	rm -f $(APP_NAME)
	@echo "$(GREEN)✓ Nettoyage terminé$(RESET)"

# ============================================================================
# PRODUCTION (nécessite les droits root)
# ============================================================================

## install: Installe le service systemd
install: build-prod
	@echo "$(BLUE)▶ Installation du service $(APP_NAME)...$(RESET)"
	@echo ""
	sudo cp $(APP_NAME) $(BIN_DIR)/
	sudo cp deploy/bookstorage.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable $(APP_NAME)
	@echo ""
	@echo "$(GREEN)✓ Service installé avec succès$(RESET)"
	@echo ""
	@echo "Commandes disponibles:"
	@echo "  $(BOLD)sudo systemctl start $(APP_NAME)$(RESET)   - Démarrer"
	@echo "  $(BOLD)sudo systemctl stop $(APP_NAME)$(RESET)    - Arrêter"
	@echo "  $(BOLD)sudo systemctl status $(APP_NAME)$(RESET)  - Statut"

## uninstall: Désinstalle le service
uninstall:
	@echo "$(RED)▶ Désinstallation du service $(APP_NAME)...$(RESET)"
	-sudo systemctl stop $(APP_NAME)
	-sudo systemctl disable $(APP_NAME)
	-sudo rm -f /etc/systemd/system/bookstorage.service
	-sudo rm -f $(BIN_DIR)/$(APP_NAME)
	sudo systemctl daemon-reload
	@echo "$(GREEN)✓ Service désinstallé$(RESET)"

## update: Met à jour l'application (pull + build + restart)
update:
	@echo ""
	@echo "$(BOLD)╔════════════════════════════════════════╗$(RESET)"
	@echo "$(BOLD)║     MISE À JOUR DE $(APP_NAME)        ║$(RESET)"
	@echo "$(BOLD)╚════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(BLUE)[1/5]$(RESET) Récupération des modifications..."
	git pull
	@echo ""
	@echo "$(BLUE)[2/5]$(RESET) Compilation..."
	$(MAKE) build-prod --no-print-directory
	@echo ""
	@echo "$(BLUE)[3/5]$(RESET) Installation du binaire..."
	cp $(APP_NAME) $(BIN_DIR)/
	@echo ""
	@echo "$(BLUE)[4/5]$(RESET) Correction des permissions..."
	@$(MAKE) fix-perms --no-print-directory
	@echo ""
	@echo "$(BLUE)[5/5]$(RESET) Redémarrage du service..."
	systemctl restart $(APP_NAME)
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════╗$(RESET)"
	@echo "$(GREEN)║      MISE À JOUR TERMINÉE ✓           ║$(RESET)"
	@echo "$(GREEN)╚════════════════════════════════════════╝$(RESET)"
	@echo ""
	@systemctl status $(APP_NAME) --no-pager -l | head -15

## fix-perms: Corrige les permissions des fichiers
fix-perms:
	@echo "$(BLUE)▶ Correction des permissions...$(RESET)"
	@# Dossier principal (nécessaire pour SQLite)
	chown $(APP_USER):$(APP_GROUP) . 2>/dev/null || true
	chmod 755 . 2>/dev/null || true
	@# Base de données
	chown $(APP_USER):$(APP_GROUP) database.db 2>/dev/null || true
	chmod 664 database.db 2>/dev/null || true
	@# Dossiers d'upload
	chown -R $(APP_USER):$(APP_GROUP) static/avatars/ 2>/dev/null || true
	chown -R $(APP_USER):$(APP_GROUP) static/images/ 2>/dev/null || true
	@# Templates
	chown -R $(APP_USER):$(APP_GROUP) templates/ 2>/dev/null || true
	@echo "$(GREEN)✓ Permissions corrigées$(RESET)"

## status: Affiche le statut du service
status:
	@systemctl status $(APP_NAME) --no-pager -l 2>/dev/null || echo "$(YELLOW)Service non installé$(RESET)"

## logs: Affiche les logs du service en temps réel
logs:
	@echo "$(BLUE)▶ Logs en temps réel (Ctrl+C pour quitter)...$(RESET)"
	@journalctl -u $(APP_NAME) -f

# ============================================================================
# AIDE
# ============================================================================

## help: Affiche cette aide
help:
	@echo ""
	@echo "$(BOLD)📚 BookStorage v$(APP_VERSION) - Gestionnaire de lectures$(RESET)"
	@echo ""
	@echo "$(BOLD)UTILISATION$(RESET)"
	@echo "    make $(BLUE)<commande>$(RESET)"
	@echo ""
	@echo "$(BOLD)DÉVELOPPEMENT$(RESET)"
	@echo "    $(GREEN)build$(RESET)        Compile l'application"
	@echo "    $(GREEN)build-prod$(RESET)   Compile en mode production (optimisé)"
	@echo "    $(GREEN)run$(RESET)          Lance le serveur de développement"
	@echo "    $(GREEN)clean$(RESET)        Supprime les fichiers compilés"
	@echo ""
	@echo "$(BOLD)PRODUCTION$(RESET) (nécessite root)"
	@echo "    $(GREEN)install$(RESET)      Installe le service systemd"
	@echo "    $(GREEN)uninstall$(RESET)    Désinstalle le service"
	@echo "    $(GREEN)update$(RESET)       Met à jour (git pull + build + restart)"
	@echo "    $(GREEN)fix-perms$(RESET)    Corrige les permissions des fichiers"
	@echo "    $(GREEN)status$(RESET)       Affiche le statut du service"
	@echo "    $(GREEN)logs$(RESET)         Affiche les logs en temps réel"
	@echo ""
	@echo "$(BOLD)EXEMPLES$(RESET)"
	@echo "    $(BLUE)make run$(RESET)              Développement local"
	@echo "    $(BLUE)sudo make install$(RESET)     Installer en production"
	@echo "    $(BLUE)sudo make update$(RESET)      Mettre à jour le serveur"
	@echo ""
	@echo "$(BOLD)CONFIGURATION$(RESET)"
	@echo "    Variables d'environnement dans $(BLUE).env$(RESET) :"
	@echo "    - BOOKSTORAGE_HOST         (défaut: 127.0.0.1)"
	@echo "    - BOOKSTORAGE_PORT         (défaut: 5000)"
	@echo "    - BOOKSTORAGE_DATABASE     (défaut: database.db)"
	@echo "    - BOOKSTORAGE_SECRET_KEY   (défaut: dev-secret-change-me)"
	@echo ""
	@echo "$(BOLD)PLUS D'INFOS$(RESET)"
	@echo "    Voir $(BLUE)README.md$(RESET) pour la documentation complète"
	@echo ""
