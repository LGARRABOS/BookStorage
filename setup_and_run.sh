#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

INSTALL_SERVICE="false"
SERVICE_NAME="${BOOKSTORAGE_SERVICE_NAME:-bookstorage}"
SERVICE_USER="${BOOKSTORAGE_SERVICE_USER:-${SUDO_USER:-$(whoami)}}"
SERVICE_GROUP="${BOOKSTORAGE_SERVICE_GROUP:-}" # facultatif, sinon groupe principal de l'utilisateur
SERVICE_ENV_FILE="${BOOKSTORAGE_SERVICE_ENV_FILE:-$SCRIPT_DIR/.env}"

usage() {
  cat <<EOF
Usage : $0 [options]

Options :
  --install-service        Installe et active une unité systemd (nécessite sudo/root)
  --service-name NOM       Nom de l'unité systemd à créer (défaut : ${SERVICE_NAME})
  --service-user UTILISATEUR
                           Compte système utilisé pour exécuter le service (défaut : ${SERVICE_USER})
  --service-group GROUPE   Groupe système utilisé pour le service (défaut : groupe principal de l'utilisateur)
  -h, --help               Affiche cette aide et quitte

Les options peuvent également être fournies via les variables d'environnement :
  BOOKSTORAGE_SERVICE_NAME, BOOKSTORAGE_SERVICE_USER, BOOKSTORAGE_SERVICE_GROUP,
  BOOKSTORAGE_SERVICE_ENV_FILE.
EOF
}

while (($#)); do
  case "$1" in
    --install-service)
      INSTALL_SERVICE="true"
      shift
      ;;
    --service-name)
      if [ $# -lt 2 ]; then
        echo "L'option --service-name nécessite un argument." >&2
        exit 1
      fi
      SERVICE_NAME="$2"
      shift 2
      ;;
    --service-user)
      if [ $# -lt 2 ]; then
        echo "L'option --service-user nécessite un argument." >&2
        exit 1
      fi
      SERVICE_USER="$2"
      shift 2
      ;;
    --service-group)
      if [ $# -lt 2 ]; then
        echo "L'option --service-group nécessite un argument." >&2
        exit 1
      fi
      SERVICE_GROUP="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Option inconnue : $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "Python 3 n'est pas disponible dans le PATH. Installez python3 avant de continuer." >&2
  exit 1
fi

PYTHON_BIN="python3"
VENV_DIR=".venv"

if [ ! -d "$VENV_DIR" ]; then
  echo "Création de l'environnement virtuel..."
  "$PYTHON_BIN" -m venv "$VENV_DIR"
fi

source "$VENV_DIR/bin/activate"

if [ -z "${CI:-}" ]; then
  python -m pip install --upgrade pip
fi

echo "Installation des dépendances Python..."
pip install -r requirements.txt

if [ ! -f ".env" ]; then
  echo "Création du fichier .env à partir de .env.example..."
  cp .env.example .env
  echo "⚠️  Le fichier .env contient des valeurs par défaut. Pensez à ajuster BOOKSTORAGE_SECRET_KEY et BOOKSTORAGE_SUPERADMIN_PASSWORD."
fi

export BOOKSTORAGE_ENV="${BOOKSTORAGE_ENV:-production}"
export FLASK_APP="${FLASK_APP:-wsgi:app}"

echo "Initialisation de la base de données..."
python init_db.py

install_systemd_service() {
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemd n'est pas disponible sur cette machine. Impossible de créer un service." >&2
    exit 1
  fi

  if [ "$(id -u)" -ne 0 ]; then
    echo "L'installation d'un service systemd nécessite les privilèges root. Relancez le script avec sudo." >&2
    exit 1
  fi

  if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    echo "L'utilisateur $SERVICE_USER n'existe pas sur ce système." >&2
    exit 1
  fi

  local resolved_group
  if [ -n "$SERVICE_GROUP" ]; then
    resolved_group="$SERVICE_GROUP"
  else
    resolved_group="$(id -gn "$SERVICE_USER")"
  fi

  if [ ! -f "$SERVICE_ENV_FILE" ]; then
    echo "Le fichier d'environnement $SERVICE_ENV_FILE est introuvable. Créez-le avant d'installer le service." >&2
    exit 1
  fi

  local service_file="/etc/systemd/system/${SERVICE_NAME}.service"

  cat <<EOF | tee "$service_file" >/dev/null
[Unit]
Description=BookStorage service (${SERVICE_NAME})
After=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${resolved_group}
WorkingDirectory=${SCRIPT_DIR}
EnvironmentFile=${SERVICE_ENV_FILE}
Environment=BOOKSTORAGE_ENV=production
ExecStart=${SCRIPT_DIR}/.venv/bin/python ${SCRIPT_DIR}/app.py
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

  echo "Service systemd créé : ${service_file}."
  systemctl daemon-reload
  systemctl enable --now "$SERVICE_NAME"
  echo "Service ${SERVICE_NAME} activé. Utilisez 'sudo systemctl status ${SERVICE_NAME}' pour vérifier son état."
}

if [ "$INSTALL_SERVICE" = "true" ]; then
  install_systemd_service
  exit 0
fi

echo "Lancement du service BookStorage (profil ${BOOKSTORAGE_ENV})..."
exec python app.py
