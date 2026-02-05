#!/bin/bash
# BookStorage - Script d'installation initial
# Compatible: Rocky Linux / RHEL / CentOS / AlmaLinux
# Usage: sudo ./deploy/install.sh

set -e

APP_NAME="bookstorage"
APP_DIR="/opt/bookstorage"
APP_USER="nobody"
APP_GROUP="nobody"

echo "=== Installation de BookStorage ==="

# Vérifier qu'on est root
if [ "$EUID" -ne 0 ]; then
    echo "Erreur: Ce script doit être exécuté en root (sudo)"
    exit 1
fi

# Détecter le gestionnaire de paquets
if command -v dnf &> /dev/null; then
    PKG_MGR="dnf"
elif command -v yum &> /dev/null; then
    PKG_MGR="yum"
elif command -v apt-get &> /dev/null; then
    PKG_MGR="apt-get"
else
    echo "Erreur: Gestionnaire de paquets non supporté"
    exit 1
fi

echo "Gestionnaire de paquets détecté: $PKG_MGR"

# Installer Go si pas présent
if ! command -v go &> /dev/null; then
    echo "Installation de Go..."
    if [ "$PKG_MGR" = "dnf" ] || [ "$PKG_MGR" = "yum" ]; then
        $PKG_MGR install -y golang
    else
        $PKG_MGR update
        $PKG_MGR install -y golang-go
    fi
fi

# Installer les dépendances pour SQLite
echo "Installation des dépendances..."
if [ "$PKG_MGR" = "dnf" ] || [ "$PKG_MGR" = "yum" ]; then
    $PKG_MGR install -y gcc sqlite make
else
    $PKG_MGR install -y gcc sqlite3 make
fi

# Créer le répertoire de l'application
echo "Création du répertoire $APP_DIR..."
mkdir -p $APP_DIR

# Copier les fichiers (en excluant .git et autres)
echo "Copie des fichiers..."
cp -r templates $APP_DIR/
cp -r static $APP_DIR/
cp -r deploy $APP_DIR/
cp *.go $APP_DIR/
cp go.mod $APP_DIR/
cp go.sum $APP_DIR/ 2>/dev/null || true
cp Makefile $APP_DIR/

# Build de l'application
echo "Compilation de l'application..."
cd $APP_DIR
go mod tidy
CGO_ENABLED=1 go build -ldflags="-s -w" -o $APP_NAME .
cp $APP_NAME /usr/local/bin/

# Créer les répertoires pour les uploads
mkdir -p $APP_DIR/static/avatars
mkdir -p $APP_DIR/static/images
chown -R $APP_USER:$APP_GROUP $APP_DIR

# Installer le service systemd
echo "Installation du service systemd..."
cp deploy/bookstorage.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable $APP_NAME

# Créer le fichier .env si nécessaire
if [ ! -f "$APP_DIR/.env" ]; then
    echo "Création du fichier .env..."
    # Générer un secret avec /dev/urandom (méthode universelle)
    SECRET=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 64 | head -n 1)
    cat > $APP_DIR/.env << EOF
FLASK_ENV=production
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000
BOOKSTORAGE_SECRET_KEY=$SECRET
EOF
    chown $APP_USER:$APP_GROUP $APP_DIR/.env
    chmod 600 $APP_DIR/.env
fi

# Ouvrir le port dans le firewall si firewalld est actif
if systemctl is-active --quiet firewalld; then
    echo "Configuration du firewall..."
    firewall-cmd --permanent --add-port=5000/tcp
    firewall-cmd --reload
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
