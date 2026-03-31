#!/bin/bash
# ============================================================================
# Fix bsctl update - À exécuter UNE FOIS après la réorganisation du projet
# ============================================================================
# L'ancien bsctl utilise "go build ." qui ne fonctionne plus.
# Ce script met à jour bsctl puis compile manuellement.
# Usage: cd /opt/bookstorage && sudo bash deploy/fix-bsctl-update.sh
# ============================================================================

set -e

APP_NAME="bookstorage"
APP_VERSION="5.1.4"
BIN_DIR="/usr/local/bin"

echo ""
echo "Fix bsctl + compilation..."
echo ""

cd /opt/bookstorage

# 1. Mettre à jour bsctl en premier (avec le nouveau chemin de build)
cp scripts/bsctl ${BIN_DIR}/
cp scripts/bsctl.lib.sh ${BIN_DIR}/bsctl.lib.sh
chmod +x ${BIN_DIR}/bsctl
echo "✓ bsctl mis à jour"

# 2. Compiler avec le nouveau chemin
CGO_ENABLED=1 go build -ldflags="-s -w -X main.Version=${APP_VERSION}" -o ${APP_NAME} ./cmd/bookstorage
echo "✓ Compilation réussie"

# 3. Installer le binaire
cp ${APP_NAME} ${BIN_DIR}/
echo "✓ Binaire installé"

# 4. Redémarrer le service
systemctl restart ${APP_NAME}
echo "✓ Service redémarré"
echo ""
echo "Terminé. Les modales personnalisées devraient maintenant fonctionner."
echo ""
