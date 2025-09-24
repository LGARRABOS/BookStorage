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
            avatar_path TEXT,
            is_public INTEGER DEFAULT 1
        );

        CREATE TABLE IF NOT EXISTS works (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            chapter INTEGER DEFAULT 0,
            link TEXT,
            status TEXT,
            image_path TEXT,
            reading_type TEXT,
            user_id INTEGER NOT NULL,
            FOREIGN KEY (user_id) REFERENCES users (id)
        );
        """
    )

    fixtures = [
        {
            "username": "superadmin",
            "password": "SuperSecure!1",
            "validated": 1,
            "is_admin": 1,
            "is_superadmin": 1,
            "display_name": "Super Administrateur",
            "is_public": 0,
        },
        {
            "username": "admin",
            "password": "AdminPower!1",
            "validated": 1,
            "is_admin": 1,
            "is_superadmin": 0,
            "display_name": "Administrateur",
            "is_public": 1,
        },
        {
            "username": "reader",
            "password": "ReaderPass!1",
            "validated": 1,
            "is_admin": 0,
            "is_superadmin": 0,
            "display_name": "Lecteur",
            "is_public": 1,
        },
        {
            "username": "sharer",
            "password": "SharerPass!1",
            "validated": 1,
            "is_admin": 0,
            "is_superadmin": 0,
            "display_name": "Partageur",
            "bio": "Toujours partant pour découvrir de nouveaux mangas.",
            "is_public": 1,
        },
        {
            "username": "private",
            "password": "SecretPass!1",
            "validated": 1,
            "is_admin": 0,
            "is_superadmin": 0,
            "display_name": "Confidentiel",
            "is_public": 0,
        },
        {
            "username": "pending",
            "password": "PendingPass!1",
            "validated": 0,
            "is_admin": 0,
            "is_superadmin": 0,
            "display_name": "En attente",
            "is_public": 1,
        },
    ]

    user_ids = {}
    for fixture in fixtures:
        cursor = conn.execute(
            """
            INSERT INTO users (
                username, password, validated, is_admin, is_superadmin, display_name, is_public, bio
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                fixture["username"],
                generate_password_hash(fixture["password"], method="pbkdf2:sha256"),
                fixture["validated"],
                fixture["is_admin"],
                fixture["is_superadmin"],
                fixture.get("display_name"),
                fixture.get("is_public", 1),
                fixture.get("bio"),
            ),
        )
        user_ids[fixture["username"]] = cursor.lastrowid

    works = [
        {
            "title": "One Piece",
            "chapter": 1023,
            "link": "https://onepiece.example",
            "status": "En cours",
            "image_path": None,
            "reading_type": "Manga",
            "owner": "sharer",
        },
        {
            "title": "Fullmetal Alchemist",
            "chapter": 27,
            "link": None,
            "status": "Terminé",
            "image_path": None,
            "reading_type": "Roman",
            "owner": "sharer",
        },
    ]

    for work in works:
        conn.execute(
            """
            INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            """,
            (
                work["title"],
                work["chapter"],
                work["link"],
                work["status"],
                work["image_path"],
                work["reading_type"],
                user_ids[work["owner"]],
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
