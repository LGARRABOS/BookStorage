# ============================================================================
# BookStorage Makefile
# ============================================================================
# Note: Pour une meilleure expérience, utilisez 'bsctl' à la place de 'make'
#       Exemple: bsctl help
# ============================================================================

APP_NAME    := bookstorage
APP_VERSION := 1.0.0
APP_USER    := nobody
APP_GROUP   := nobody
BIN_DIR     := /usr/local/bin

.PHONY: all build build-prod run clean install uninstall update fix-perms status logs help

.DEFAULT_GOAL := help

# Développement
build:
	@echo "Compilation..."
	@go build -o $(APP_NAME) .
	@echo "Terminé: ./$(APP_NAME)"

build-prod:
	@echo "Compilation production..."
	@CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=$(APP_VERSION)" -o $(APP_NAME) .
	@echo "Terminé: ./$(APP_NAME)"

run:
	@go run .

clean:
	@rm -f $(APP_NAME)
	@echo "Nettoyage terminé"

# Production
install: build-prod
	@cp $(APP_NAME) $(BIN_DIR)/
	@cp bsctl $(BIN_DIR)/
	@chmod +x $(BIN_DIR)/bsctl
	@cp deploy/bookstorage.service /etc/systemd/system/
	@systemctl daemon-reload
	@systemctl enable $(APP_NAME)
	@echo "Service installé. Utilisez: bsctl start"

uninstall:
	@-systemctl stop $(APP_NAME) 2>/dev/null || true
	@-systemctl disable $(APP_NAME) 2>/dev/null || true
	@-rm -f /etc/systemd/system/bookstorage.service
	@-rm -f $(BIN_DIR)/$(APP_NAME)
	@-rm -f $(BIN_DIR)/bsctl
	@systemctl daemon-reload
	@echo "Service désinstallé"

update:
	@./bsctl update

fix-perms:
	@./bsctl fix-perms

status:
	@systemctl status $(APP_NAME) --no-pager -l 2>/dev/null || echo "Service non installé"

logs:
	@journalctl -u $(APP_NAME) -f

help:
	@echo ""
	@echo "BookStorage v$(APP_VERSION)"
	@echo ""
	@echo "Pour une meilleure expérience, utilisez 'bsctl' :"
	@echo "  bsctl help      - Afficher l'aide complète"
	@echo "  bsctl run       - Lancer en développement"
	@echo "  bsctl update    - Mettre à jour (production)"
	@echo "  bsctl status    - Voir le statut du service"
	@echo "  bsctl logs      - Voir les logs"
	@echo ""
	@echo "Commandes make disponibles:"
	@echo "  make build      - Compiler"
	@echo "  make run        - Lancer en développement"
	@echo "  make install    - Installer le service"
	@echo "  make update     - Mettre à jour"
	@echo ""
