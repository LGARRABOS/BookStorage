#!/usr/bin/env sh
set -eu

APP_DIR="${BOOKSTORAGE_APP_DIR:-/srv/bookstorage}"

if [ ! -d "$APP_DIR" ] || [ ! -f "$APP_DIR/app.py" ]; then
    echo "BookStorage n'est pas disponible dans \"$APP_DIR\"." >&2
    echo "Assurez-vous de ne pas monter un volume vide par-dessus le dossier de l'application." >&2
    exit 1
fi

cd "$APP_DIR"

DATA_DIR="${BOOKSTORAGE_DATA_DIR:-/data}"
UPLOAD_DIR="${BOOKSTORAGE_UPLOAD_DIR:-$DATA_DIR/images}"
AVATAR_DIR="${BOOKSTORAGE_AVATAR_DIR:-$DATA_DIR/avatars}"

mkdir -p "$DATA_DIR" "$UPLOAD_DIR" "$AVATAR_DIR"

python init_db.py

exec "$@"
