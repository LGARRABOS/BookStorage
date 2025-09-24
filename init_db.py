import sqlite3
from pathlib import Path

from werkzeug.security import generate_password_hash

from config import get_settings


PROJECT_ROOT = Path(__file__).resolve().parent
SETTINGS = get_settings(PROJECT_ROOT)
DATABASE = SETTINGS.database

def init_db():
    conn = sqlite3.connect(DATABASE)
    conn.execute("PRAGMA foreign_keys = ON;")
    
    # Création de la table users avec validated, is_admin et is_superadmin
    conn.execute("""
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
    """)

    profile_columns = {
        "display_name": "TEXT",
        "email": "TEXT",
        "bio": "TEXT",
        "avatar_path": "TEXT",
        "is_public": "INTEGER DEFAULT 1",
    }

    existing_columns = {
        info[1] for info in conn.execute("PRAGMA table_info(users)").fetchall()
    }
    for column_name, column_type in profile_columns.items():
        if column_name not in existing_columns:
            conn.execute(f"ALTER TABLE users ADD COLUMN {column_name} {column_type}")

    conn.commit()

    # Création de la table works
    conn.execute("""
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
    """)

    work_columns = {
        "reading_type": "TEXT",
    }

    existing_work_columns = {
        info[1] for info in conn.execute("PRAGMA table_info(works)").fetchall()
    }
    for column_name, column_type in work_columns.items():
        if column_name not in existing_work_columns:
            conn.execute(f"ALTER TABLE works ADD COLUMN {column_name} {column_type}")

    conn.commit()
    
    # Insertion d'un compte super‑admin par défaut s'il n'existe pas
    cursor = conn.cursor()
    cursor.execute("SELECT * FROM users WHERE is_superadmin = 1")
    super_admin_exists = cursor.fetchone()
    if not super_admin_exists:
        default_username = SETTINGS.superadmin_username
        default_password = SETTINGS.superadmin_password
        hashed_password = generate_password_hash(default_password, method="pbkdf2:sha256")
        cursor.execute(
            "INSERT INTO users (username, password, validated, is_admin, is_superadmin) VALUES (?, ?, ?, ?, ?)",
            (default_username, hashed_password, 1, 1, 1)
        )
        conn.commit()
        print(f"Compte super‑admin créé : username='{default_username}'.")
        if default_password == "SuperAdmin!2023":
            print("Mot de passe par défaut utilisé. Pensez à le changer pour la production !")
    else:
        print("Un compte super‑admin existe déjà.")
    
    conn.close()
    print("Base de données initialisée.")

if __name__ == "__main__":
    init_db()
