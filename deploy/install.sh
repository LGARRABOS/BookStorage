#!/bin/bash
# ============================================================================
# BookStorage - Script d'installation
# ============================================================================
# Compatible: Rocky Linux / RHEL / CentOS / AlmaLinux / Debian / Ubuntu
# Usage: sudo ./deploy/install.sh [repo_url]
# ============================================================================

set -e

APP_NAME="bookstorage"
APP_DIR="/opt/bookstorage"
APP_USER="nobody"
APP_GROUP="nobody"
REPO_URL="${1:-https://github.com/LGARRABOS/BookStorage.git}"

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

print_header() {
    printf "\n"
    printf "${BOLD}╔════════════════════════════════════════════╗${NC}\n"
    printf "${BOLD}║  📚 BookStorage - Installation             ║${NC}\n"
    printf "${BOLD}╚════════════════════════════════════════════╝${NC}\n"
    printf "\n"
}

print_step() {
    printf "${BLUE}[$1]${NC} $2\n"
}

print_success() {
    printf "${GREEN}✓ $1${NC}\n"
}

print_error() {
    printf "${RED}✗ $1${NC}\n"
    exit 1
}

# ============================================================================
# Vérifications
# ============================================================================

print_header

if [ "$EUID" -ne 0 ]; then
    print_error "Ce script doit être exécuté en root (sudo)"
fi

# Détecter le gestionnaire de paquets
if command -v dnf &> /dev/null; then
    PKG_MGR="dnf"
elif command -v yum &> /dev/null; then
    PKG_MGR="yum"
elif command -v apt-get &> /dev/null; then
    PKG_MGR="apt-get"
else
    print_error "Gestionnaire de paquets non supporté"
fi

printf "Système détecté: ${BOLD}$PKG_MGR${NC}\n\n"

# ============================================================================
# Installation des dépendances
# ============================================================================

print_step "1/7" "Installation des dépendances système..."

if [ "$PKG_MGR" = "dnf" ] || [ "$PKG_MGR" = "yum" ]; then
    $PKG_MGR install -y golang gcc sqlite make git > /dev/null 2>&1
else
    apt-get update > /dev/null 2>&1
    apt-get install -y golang-go gcc sqlite3 make git > /dev/null 2>&1
fi
print_success "Dépendances installées"

# ============================================================================
# Clonage du repo
# ============================================================================

print_step "2/7" "Récupération du code source..."

if [ -d "$APP_DIR/.git" ]; then
    cd $APP_DIR
    git pull > /dev/null 2>&1
    print_success "Repo mis à jour"
else
    rm -rf $APP_DIR
    git clone $REPO_URL $APP_DIR > /dev/null 2>&1
    cd $APP_DIR
    print_success "Repo cloné dans $APP_DIR"
fi

# ============================================================================
# Compilation
# ============================================================================

print_step "3/7" "Compilation de l'application..."

go mod tidy > /dev/null 2>&1
CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=1.0.0" -o $APP_NAME . > /dev/null 2>&1
print_success "Application compilée"

# ============================================================================
# Installation des binaires
# ============================================================================

print_step "4/7" "Installation des binaires..."

cp $APP_NAME /usr/local/bin/
cp bsctl /usr/local/bin/
chmod +x /usr/local/bin/bsctl
print_success "Binaires installés dans /usr/local/bin/"

# ============================================================================
# Configuration des dossiers
# ============================================================================

print_step "5/7" "Configuration des répertoires..."

mkdir -p $APP_DIR/static/avatars
mkdir -p $APP_DIR/static/images

chown $APP_USER:$APP_GROUP $APP_DIR
chmod 755 $APP_DIR
chown -R $APP_USER:$APP_GROUP $APP_DIR/static
chown $APP_USER:$APP_GROUP $APP_DIR/database.db 2>/dev/null || true
chmod 664 $APP_DIR/database.db 2>/dev/null || true
print_success "Répertoires configurés"

# ============================================================================
# Service systemd
# ============================================================================

print_step "6/7" "Installation du service systemd..."

cp deploy/bookstorage.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable $APP_NAME > /dev/null 2>&1
print_success "Service systemd installé"

# ============================================================================
# Configuration
# ============================================================================

print_step "7/7" "Configuration de l'application..."

if [ ! -f "$APP_DIR/.env" ]; then
    SECRET=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 64 | head -n 1)
    cat > $APP_DIR/.env << EOF
# BookStorage Configuration
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000
BOOKSTORAGE_SECRET_KEY=$SECRET
EOF
    chmod 600 $APP_DIR/.env
    print_success "Fichier .env créé avec clé secrète générée"
else
    print_success "Fichier .env existant conservé"
fi

# Ouvrir le port dans le firewall si firewalld est actif
if systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-port=5000/tcp > /dev/null 2>&1
    firewall-cmd --reload > /dev/null 2>&1
    print_success "Port 5000 ouvert dans le firewall"
fi

# ============================================================================
# Terminé
# ============================================================================

printf "\n"
printf "${GREEN}╔════════════════════════════════════════════╗${NC}\n"
printf "${GREEN}║      INSTALLATION TERMINÉE ✓               ║${NC}\n"
printf "${GREEN}╚════════════════════════════════════════════╝${NC}\n"
printf "\n"

printf "${BOLD}COMMANDES DISPONIBLES${NC}\n"
printf "\n"
printf "  ${GREEN}bsctl start${NC}      Démarrer le service\n"
printf "  ${GREEN}bsctl stop${NC}       Arrêter le service\n"
printf "  ${GREEN}bsctl restart${NC}    Redémarrer le service\n"
printf "  ${GREEN}bsctl status${NC}     Voir le statut\n"
printf "  ${GREEN}bsctl logs${NC}       Voir les logs en temps réel\n"
printf "  ${GREEN}bsctl update${NC}     Mettre à jour l'application\n"
printf "  ${GREEN}bsctl help${NC}       Afficher l'aide complète\n"
printf "\n"

printf "${BOLD}DÉMARRER MAINTENANT${NC}\n"
printf "\n"
printf "  ${BLUE}bsctl start${NC}\n"
printf "\n"

printf "L'application sera accessible sur: ${BOLD}http://$(hostname -I | awk '{print $1}'):5000${NC}\n"
printf "\n"
