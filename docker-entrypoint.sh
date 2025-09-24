#!/usr/bin/env sh
set -eu

DATA_DIR="${BOOKSTORAGE_DATA_DIR:-/data}"
UPLOAD_DIR="${BOOKSTORAGE_UPLOAD_DIR:-$DATA_DIR/images}"
AVATAR_DIR="${BOOKSTORAGE_AVATAR_DIR:-$DATA_DIR/avatars}"

mkdir -p "$DATA_DIR" "$UPLOAD_DIR" "$AVATAR_DIR"

python init_db.py

exec "$@"
