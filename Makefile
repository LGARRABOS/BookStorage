# BookStorage Makefile

APP_NAME := bookstorage
BUILD_DIR := .

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

# Update: pull, rebuild, restart
update:
	@echo "Updating $(APP_NAME)..."
	git pull
	$(MAKE) build-prod
	sudo cp $(APP_NAME) /usr/local/bin/
	sudo systemctl restart $(APP_NAME)
	@echo "Update complete!"
