# BookStorage Makefile

APP_NAME := bookstorage
APP_USER := nobody
APP_GROUP := nobody

.PHONY: all build clean run install uninstall update

# Build the application
build:
	go build -o $(APP_NAME) .

# Build optimized for production (smaller binary)
build-prod:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(APP_NAME) .

# Run the application (for development)
run:
	go run .

# Clean build artifacts
clean:
	rm -f $(APP_NAME)

# Install as systemd service (run as root)
install: build-prod
	@echo "Installing $(APP_NAME) service..."
	sudo cp $(APP_NAME) /usr/local/bin/
	sudo cp deploy/bookstorage.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable $(APP_NAME)
	@echo "Service installed. Start with: sudo systemctl start $(APP_NAME)"

# Uninstall the service (run as root)
uninstall:
	@echo "Removing $(APP_NAME) service..."
	-sudo systemctl stop $(APP_NAME)
	-sudo systemctl disable $(APP_NAME)
	-sudo rm /etc/systemd/system/bookstorage.service
	-sudo rm /usr/local/bin/$(APP_NAME)
	sudo systemctl daemon-reload
	@echo "Service removed."

# Update: pull, rebuild, restart (run from /opt/bookstorage)
update:
	@echo "=== Mise à jour de $(APP_NAME) ==="
	@echo "1. Pull des modifications..."
	git pull
	@echo "2. Compilation..."
	$(MAKE) build-prod
	@echo "3. Copie du binaire..."
	cp $(APP_NAME) /usr/local/bin/
	@echo "4. Permissions..."
	chown -R $(APP_USER):$(APP_GROUP) static/
	chown $(APP_USER):$(APP_GROUP) database.db 2>/dev/null || true
	@echo "5. Redémarrage du service..."
	systemctl restart $(APP_NAME)
	@echo ""
	@echo "=== Mise à jour terminée ==="
	systemctl status $(APP_NAME) --no-pager
