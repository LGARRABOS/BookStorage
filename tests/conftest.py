import sqlite3
from pathlib import Path
import sys

import pytest
from werkzeug.security import generate_password_hash

PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

import app as flask_app


def _bootstrap_database(db_path: str) -> None:
    conn = sqlite3.connect(db_path)
    conn.execute("PRAGMA foreign_keys = ON;")
    conn.executescript(
        """
        CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT UNIQUE NOT NULL,
            password TEXT NOT NULL,
            validated INTEGER DEFAULT 0,
            is_admin INTEGER DEFAULT 0,
            is_superadmin INTEGER DEFAULT 0,
            display_name TEXT,
            email TEXT,
            bio TEXT,
            avatar_path TEXT
        );

        CREATE TABLE IF NOT EXISTS works (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            chapter INTEGER DEFAULT 0,
            link TEXT,
            status TEXT,
            image_path TEXT,
            user_id INTEGER NOT NULL,
            FOREIGN KEY (user_id) REFERENCES users (id)
        );
        """
    )

    fixtures = [
        ("superadmin", "SuperSecure!1", 1, 1, 1, "Super Administrateur"),
        ("admin", "AdminPower!1", 1, 1, 0, "Administrateur"),
        ("reader", "ReaderPass!1", 1, 0, 0, "Lecteur"),
        ("pending", "PendingPass!1", 0, 0, 0, "En attente"),
    ]

    for username, password, validated, is_admin, is_superadmin, display_name in fixtures:
        conn.execute(
            "INSERT INTO users (username, password, validated, is_admin, is_superadmin, display_name) VALUES (?, ?, ?, ?, ?, ?)",
            (
                username,
                generate_password_hash(password, method="pbkdf2:sha256"),
                validated,
                is_admin,
                is_superadmin,
                display_name,
            ),
        )

    conn.commit()
    conn.close()


@pytest.fixture
def database_path(tmp_path):
    db_path = tmp_path / "test.db"
    media_root = tmp_path / "media"
    avatars_dir = media_root / "avatars"
    works_dir = media_root / "works"
    avatars_dir.mkdir(parents=True, exist_ok=True)
    works_dir.mkdir(parents=True, exist_ok=True)

    _bootstrap_database(str(db_path))
    flask_app.app.config.update(
        TESTING=True,
        SECRET_KEY="test-secret-key",
        DATABASE=str(db_path),
        PROFILE_UPLOAD_FOLDER=str(avatars_dir),
        UPLOAD_FOLDER=str(works_dir),
    )
    return str(db_path)


@pytest.fixture
def client(database_path):
    with flask_app.app.test_client() as client:
        yield client


@pytest.fixture
def get_user_record(database_path):
    def _get_user(username: str):
        conn = sqlite3.connect(database_path)
        conn.row_factory = sqlite3.Row
        try:
            return conn.execute(
                "SELECT * FROM users WHERE username = ?",
                (username,),
            ).fetchone()
        finally:
            conn.close()

    return _get_user
