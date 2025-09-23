import sqlite3

import app as flask_app


def test_get_db_connection_recreates_missing_works_table(tmp_path):
    legacy_db = tmp_path / "legacy.db"
    conn = sqlite3.connect(legacy_db)
    conn.execute(
        """
        CREATE TABLE users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT UNIQUE NOT NULL,
            password TEXT NOT NULL,
            validated INTEGER DEFAULT 0,
            is_admin INTEGER DEFAULT 0,
            is_superadmin INTEGER DEFAULT 0
        );
        """
    )
    conn.commit()
    conn.close()

    original_database = flask_app.app.config.get("DATABASE")
    try:
        flask_app.app.config["DATABASE"] = str(legacy_db)
        connection = flask_app.get_db_connection()
        try:
            row = connection.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='works'"
            ).fetchone()
        finally:
            connection.close()
    finally:
        flask_app.app.config["DATABASE"] = original_database

    assert row is not None
