# ============================================================================
# BookStorage Makefile
# ============================================================================
# Note: Pour une meilleure expérience, utilisez 'bsctl' à la place de 'make'
#       Exemple: bsctl help
# ============================================================================

APP_NAME    := bookstorage
APP_VERSION := 5.0.1
APP_USER    := nobody
APP_GROUP   := nobody
BIN_DIR     := /usr/local/bin

.PHONY: all build build-prod run clean test test-race lint ci-local install uninstall update fix-perms status logs help

.DEFAULT_GOAL := help

# Développement
build:
	@echo "Compilation..."
	@go build -o $(APP_NAME) ./cmd/bookstorage
	@echo "Terminé: ./$(APP_NAME)"

build-prod:
	@echo "Compilation production..."
	@CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=$(APP_VERSION)" -o $(APP_NAME) ./cmd/bookstorage
	@echo "Terminé: ./$(APP_NAME)"

run:
	@go run ./cmd/bookstorage

clean:
	@rm -f $(APP_NAME)
	@echo "Nettoyage terminé"

test:
	@echo "Tests unitaires..."
	@go test ./... -coverprofile=coverage.out

test-race:
	@echo "Tests race..."
	@go test -race ./...

lint:
	@echo "Format + lint..."
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "Go files not formatted:"; echo "$$files"; exit 1; fi
	@golangci-lint run

ci-local: lint test test-race
	@echo "Validation locale CI terminée"

# Production
install: build-prod
	@cp $(APP_NAME) $(BIN_DIR)/
	@cp scripts/bsctl $(BIN_DIR)/
	@chmod +x $(BIN_DIR)/bsctl
	@test -d /etc/bash_completion.d && cp scripts/bsctl.completion.bash /etc/bash_completion.d/bsctl && chmod 644 /etc/bash_completion.d/bsctl || true
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
	@-rm -f /etc/bash_completion.d/bsctl
	@systemctl daemon-reload
	@echo "Service désinstallé"

update:
	@./scripts/bsctl update

fix-perms:
	@./scripts/bsctl fix-perms

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
	@echo "  make test       - Tests unitaires + coverage.out"
	@echo "  make test-race  - Tests race"
	@echo "  make lint       - gofmt (strict) + golangci-lint"
	@echo "  make ci-local   - lint + test + test-race"
	@echo "  make install    - Installer le service"
	@echo "  make update     - bsctl update (menu release ; sinon: bsctl update main)"
	@echo ""
