#!/bin/bash
# BookStorage - Script d'installation initial
# Usage: sudo ./deploy/install.sh

set -e

APP_NAME="bookstorage"
APP_DIR="/opt/bookstorage"
APP_USER="www-data"

echo "=== Installation de BookStorage ==="

# Vérifier qu'on est root
if [ "$EUID" -ne 0 ]; then
    echo "Erreur: Ce script doit être exécuté en root (sudo)"
    exit 1
fi

# Installer Go si pas présent
if ! command -v go &> /dev/null; then
    echo "Installation de Go..."
    apt-get update
    apt-get install -y golang-go
fi

# Installer les dépendances pour SQLite
echo "Installation des dépendances..."
apt-get install -y gcc sqlite3

# Créer le répertoire de l'application
echo "Création du répertoire $APP_DIR..."
mkdir -p $APP_DIR
cp -r . $APP_DIR/
chown -R $APP_USER:$APP_USER $APP_DIR

# Build de l'application
echo "Compilation de l'application..."
cd $APP_DIR
CGO_ENABLED=1 go build -ldflags="-s -w" -o $APP_NAME .
cp $APP_NAME /usr/local/bin/

# Créer les répertoires pour les uploads
mkdir -p $APP_DIR/static/avatars
mkdir -p $APP_DIR/static/images
chown -R $APP_USER:$APP_USER $APP_DIR/static

# Installer le service systemd
echo "Installation du service systemd..."
cp deploy/bookstorage.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable $APP_NAME

# Créer le fichier .env si nécessaire
if [ ! -f "$APP_DIR/.env" ]; then
    echo "Création du fichier .env..."
    cat > $APP_DIR/.env << 'EOF'
FLASK_ENV=production
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000
SECRET_KEY=$(openssl rand -hex 32)
EOF
    chown $APP_USER:$APP_USER $APP_DIR/.env
fi

echo ""
echo "=== Installation terminée ==="
echo ""
echo "Commandes utiles:"
echo "  Démarrer:    sudo systemctl start $APP_NAME"
echo "  Arrêter:     sudo systemctl stop $APP_NAME"
echo "  Redémarrer:  sudo systemctl restart $APP_NAME"
echo "  Statut:      sudo systemctl status $APP_NAME"
echo "  Logs:        sudo journalctl -u $APP_NAME -f"
echo ""
echo "Pour mettre à jour plus tard:"
echo "  cd $APP_DIR && sudo make update"
echo ""
