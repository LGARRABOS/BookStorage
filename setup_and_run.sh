#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

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

echo "Lancement du service BookStorage (profil ${BOOKSTORAGE_ENV})..."
exec python app.py
