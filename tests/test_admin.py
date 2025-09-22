import binascii
import sqlite3
from hashlib import scrypt
from pathlib import Path
import sys

import pytest
from werkzeug.security import generate_password_hash

PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

import app as flask_app


def _bootstrap_database(db_path):
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
            is_superadmin INTEGER DEFAULT 0
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
        ("superadmin", "SuperSecure!1", 1, 1, 1),
        ("admin", "AdminPower!1", 1, 1, 0),
        ("reader", "ReaderPass!1", 1, 0, 0),
        ("pending", "PendingPass!1", 0, 0, 0),
    ]
    for username, password, validated, is_admin, is_superadmin in fixtures:
        conn.execute(
            "INSERT INTO users (username, password, validated, is_admin, is_superadmin) VALUES (?, ?, ?, ?, ?)",
            (
                username,
                generate_password_hash(password, method="pbkdf2:sha256"),
                validated,
                is_admin,
                is_superadmin,
            ),
        )

    conn.commit()
    conn.close()


def _get_user_record(username):
    conn = sqlite3.connect(flask_app.app.config["DATABASE"])
    try:
        return conn.execute(
            "SELECT id, username, validated, is_admin, is_superadmin FROM users WHERE username = ?",
            (username,),
        ).fetchone()
    finally:
        conn.close()


@pytest.fixture
def client(tmp_path):
    db_path = tmp_path / "test.db"
    flask_app.app.config.update(
        TESTING=True,
        SECRET_KEY="test-secret-key",
        DATABASE=str(db_path),
    )
    _bootstrap_database(str(db_path))

    with flask_app.app.test_client() as client:
        yield client


def test_superadmin_can_delete_admin(client):
    admin_record = _get_user_record("admin")
    assert admin_record is not None

    response = client.post(
        "/login",
        data={"username": "superadmin", "password": "SuperSecure!1"},
        follow_redirects=True,
    )
    assert b"Connexion r\xc3\xa9ussie." in response.data

    with client.session_transaction() as session:
        assert session.get("is_superadmin") is True

    delete_response = client.get(
        f"/admin/delete_account/{admin_record[0]}", follow_redirects=True
    )
    assert b"Compte supprim\xc3\xa9." in delete_response.data

    assert _get_user_record("admin") is None


def test_admin_cannot_delete_superadmin(client):
    superadmin_record = _get_user_record("superadmin")
    assert superadmin_record is not None

    response = client.post(
        "/login",
        data={"username": "admin", "password": "AdminPower!1"},
        follow_redirects=True,
    )
    assert b"Connexion r\xc3\xa9ussie." in response.data

    delete_response = client.get(
        f"/admin/delete_account/{superadmin_record[0]}",
        follow_redirects=True,
    )
    delete_page = delete_response.get_data(as_text=True)
    assert "Seul un super" in delete_page

    # Ensure the account was not removed
    assert _get_user_record("superadmin") is not None


def test_unvalidated_user_cannot_login(client):
    response = client.post(
        "/login",
        data={"username": "pending", "password": "PendingPass!1"},
        follow_redirects=True,
    )
    login_page = response.get_data(as_text=True)
    assert "Votre compte n&#39;est pas encore validé par un administrateur." in login_page

    with client.session_transaction() as session:
        assert "user_id" not in session


def test_verify_password_supports_legacy_scrypt():
    password = "LegacyPass!1"
    salt = "testsalt"
    derived = scrypt(
        password.encode("utf-8"),
        salt=salt.encode("utf-8"),
        n=16384,
        r=8,
        p=1,
    )
    stored_hash = "scrypt:16384:8:1$" + salt + "$" + binascii.hexlify(derived).decode("ascii")

    assert flask_app.verify_password(stored_hash, password) is True
    assert flask_app.verify_password(stored_hash, "WrongPass!1") is False
