#!/bin/bash
# ============================================================================
# BookStorage - Script d'installation
# ============================================================================
# Compatible: Rocky Linux / RHEL / CentOS / AlmaLinux / Debian / Ubuntu
# Usage: sudo ./deploy/install.sh [repo_url]
# ============================================================================

set -e

APP_NAME="bookstorage"
APP_VERSION="5.7.0"
APP_DIR="/opt/bookstorage"
APP_USER="nobody"
APP_GROUP="nobody"
REPO_URL="${1:-https://github.com/LGARRABOS/BookStorage.git}"

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
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

print_warn() {
	printf "${YELLOW}⚠ $1${NC}\n"
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
CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=${APP_VERSION}" -o $APP_NAME ./cmd/bookstorage > /dev/null 2>&1
print_success "Application compilée"

# ============================================================================
# Installation des binaires
# ============================================================================

print_step "4/7" "Installation des binaires..."

cp $APP_NAME /usr/local/bin/
# Strip CR (CRLF) so lines like "cmd_backup" are never split on the server.
tr -d '\r' < scripts/bsctl >/usr/local/bin/bsctl
chmod 755 /usr/local/bin/bsctl
tr -d '\r' < scripts/bsctl.lib.sh >/usr/local/bin/bsctl.lib.sh
chmod 644 /usr/local/bin/bsctl.lib.sh
if [ -d /etc/bash_completion.d ]; then
    tr -d '\r' < scripts/bsctl.completion.bash >/etc/bash_completion.d/bsctl
    chmod 644 /etc/bash_completion.d/bsctl
fi
if [ -d /usr/share/bash-completion/completions ]; then
    tr -d '\r' < scripts/bsctl.completion.bash >/usr/share/bash-completion/completions/bsctl
    chmod 644 /usr/share/bash-completion/completions/bsctl
fi
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
cp deploy/bookstorage-update.service /etc/systemd/system/
cp deploy/bookstorage-update.path /etc/systemd/system/
cp deploy/bookstorage-update-worker.sh /usr/local/bin/bookstorage-update-worker
chmod +x /usr/local/bin/bookstorage-update-worker

# Update queue directory (used by admin update worker)
mkdir -p /var/lib/bookstorage/update
chmod 755 /var/lib/bookstorage
chmod 755 /var/lib/bookstorage/update

systemctl daemon-reload
systemctl enable $APP_NAME > /dev/null 2>&1
systemctl enable bookstorage-update.path > /dev/null 2>&1
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
    print_success "Fichier .env créé avec clé secrète générée"
else
    print_success "Fichier .env existant conservé"
fi
# Service runs as APP_USER (see deploy/bookstorage.service): .env must be writable so Admin → PostgreSQL
# migration can merge BOOKSTORAGE_POSTGRES_URL without permission denied.
# chown USER only sets the login primary group (e.g. nobody → nogroup on Debian).
if [ -f "$APP_DIR/.env" ]; then
    chown "$APP_USER" "$APP_DIR/.env"
    chmod 600 "$APP_DIR/.env"
    print_success ".env → propriétaire $APP_USER (migration PostgreSQL / écriture admin)"
fi

# Ouvrir le port dans le firewall si firewalld est actif
if systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-port=5000/tcp > /dev/null 2>&1
    firewall-cmd --reload > /dev/null 2>&1
    print_success "Port 5000 ouvert dans le firewall"
fi

# ============================================================================
# Prometheus (opt-in : INSTALL_WITH_PROMETHEUS=1)
# ============================================================================

if [ "${INSTALL_WITH_BACKUP_TIMER:-}" = "1" ]; then
    print_step "Backup" "Timer systemd (INSTALL_WITH_BACKUP_TIMER=1)..."
    mkdir -p /var/lib/bookstorage/backups
    chmod 755 /var/lib/bookstorage/backups
    cp "$APP_DIR/deploy/bookstorage-backup.service" /etc/systemd/system/ 2>/dev/null || true
    cp "$APP_DIR/deploy/bookstorage-backup.timer" /etc/systemd/system/ 2>/dev/null || true
    systemctl daemon-reload
    systemctl enable --now bookstorage-backup.timer 2>/dev/null && print_success "Timer bookstorage-backup actif" || print_warn "Timer backup non activé (unités manquantes ou erreur systemctl)"
fi

if [ "${INSTALL_WITH_PROMETHEUS:-}" = "1" ]; then
    print_step "Prometheus" "Installation optionnelle (INSTALL_WITH_PROMETHEUS=1)..."
    export INSTALL_APP_DIR="$APP_DIR"
    chmod +x "$APP_DIR/deploy/setup-bookstorage-prometheus.sh" 2>/dev/null || true
    if bash "$APP_DIR/deploy/setup-bookstorage-prometheus.sh"; then
        print_success "Service bookstorage-prometheus installé (http://127.0.0.1:9091)"
    else
        print_warn "Prometheus automatique échoué — voir docs/self-hosting.md (Prometheus metrics)"
    fi
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
printf "  ${GREEN}bsctl update${NC}     Choisir une release (tags vX.0.0) ou saisir un tag\n"
printf "  ${GREEN}bsctl update main${NC}  Mettre à jour depuis origin/main\n"
printf "  ${GREEN}bsctl help${NC}       Afficher l'aide complète\n"
printf "\n"

printf "${BOLD}DÉMARRER MAINTENANT${NC}\n"
printf "\n"
printf "  ${BLUE}bsctl start${NC}\n"
printf "\n"

printf "L'application sera accessible sur: ${BOLD}http://$(hostname -I | awk '{print $1}'):5000${NC}\n"
printf "\n"
printf "Sauvegardes planifiées (optionnel) : ${BLUE}INSTALL_WITH_BACKUP_TIMER=1 sudo -E ./deploy/install.sh${NC}  (timer 03:15, ${BLUE}bsctl backup${NC})\n"
printf "Prometheus (optionnel) : ${BLUE}INSTALL_WITH_PROMETHEUS=1 sudo -E ./deploy/install.sh${NC}\n"
printf "  ou après coup : ${BLUE}INSTALL_APP_DIR=$APP_DIR bash $APP_DIR/deploy/setup-bookstorage-prometheus.sh${NC}\n"
printf "\n"
