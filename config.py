"""Application configuration helpers for BookStorage.

This module centralises the logic that derives runtime settings from the
environment so the web application and the database initialiser share the
exact same defaults.  It loads an optional ``.env`` file (if
``python-dotenv`` is installed) to simplify local setups while still
allowing explicit environment variables for production deployments.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

try:  # pragma: no cover - optional dependency
    from dotenv import load_dotenv
except ImportError:  # pragma: no cover - executed when python-dotenv is absent
    load_dotenv = None  # type: ignore


if load_dotenv is not None:  # pragma: no cover - simple delegation
    # Load a .env file located at the project root if present. ``override``
    # stays False so environment variables always win, which is important
    # for production deployments managed by process supervisors.
    load_dotenv(Path(__file__).resolve().parent / ".env", override=False)


DEFAULT_SECRET_KEY = "dev-secret-change-me"
DEFAULT_DATABASE_NAME = "database.db"
DEFAULT_UPLOAD_DIR = os.path.join("static", "images")
DEFAULT_AVATAR_DIR = os.path.join("static", "avatars")
DEFAULT_UPLOAD_URL_PATH = "images"
DEFAULT_AVATAR_URL_PATH = "avatars"


@dataclass(frozen=True)
class Settings:
    """Resolved configuration values for the application."""

    secret_key: str
    secret_from_env: bool
    database: str
    data_directory: str
    upload_folder: str
    upload_url_path: str
    profile_upload_folder: str
    profile_upload_url_path: str
    superadmin_username: str
    superadmin_password: str
    environment: str
    host: str
    port: int


def _resolve_directory(root: Path, candidate: Optional[str], default: str) -> Path:
    """Return an absolute path for a directory and ensure it exists."""

    target = Path(candidate) if candidate else Path(default)
    if not target.is_absolute():
        target = root / target
    target.mkdir(parents=True, exist_ok=True)
    return target


def _resolve_file(
    root: Path,
    base_directory: Path,
    candidate: Optional[str],
    default_name: str,
) -> Path:
    """Return an absolute path for a file within ``base_directory`` by default."""

    file_value = Path(candidate) if candidate else Path(default_name)
    if file_value.is_absolute():
        file_path = file_value
    else:
        file_path = base_directory / file_value
    file_path.parent.mkdir(parents=True, exist_ok=True)
    return file_path


def get_settings(root_path: Path | str) -> Settings:
    """Compute the runtime settings for the application.

    Parameters
    ----------
    root_path:
        Base directory used to resolve relative paths (typically the Flask
        application's ``root_path``).
    """

    root = Path(root_path)

    environment = os.environ.get("BOOKSTORAGE_ENV", "development").strip().lower()

    data_directory = _resolve_directory(root, os.environ.get("BOOKSTORAGE_DATA_DIR"), ".")

    database_path = _resolve_file(
        root,
        data_directory,
        os.environ.get("BOOKSTORAGE_DATABASE"),
        DEFAULT_DATABASE_NAME,
    )

    upload_folder = _resolve_directory(
        root,
        os.environ.get("BOOKSTORAGE_UPLOAD_DIR"),
        DEFAULT_UPLOAD_DIR,
    )

    avatar_folder = _resolve_directory(
        root,
        os.environ.get("BOOKSTORAGE_AVATAR_DIR"),
        DEFAULT_AVATAR_DIR,
    )

    secret_key = os.environ.get("BOOKSTORAGE_SECRET_KEY", DEFAULT_SECRET_KEY)
    secret_from_env = "BOOKSTORAGE_SECRET_KEY" in os.environ

    upload_url_path = (
        os.environ.get("BOOKSTORAGE_UPLOAD_URL_PATH", DEFAULT_UPLOAD_URL_PATH)
        .strip("/")
        or DEFAULT_UPLOAD_URL_PATH
    )
    avatar_url_path = (
        os.environ.get("BOOKSTORAGE_AVATAR_URL_PATH", DEFAULT_AVATAR_URL_PATH)
        .strip("/")
        or DEFAULT_AVATAR_URL_PATH
    )

    host = os.environ.get("BOOKSTORAGE_HOST", "127.0.0.1").strip() or "127.0.0.1"
    try:
        port = int(os.environ.get("BOOKSTORAGE_PORT", "5000"))
    except ValueError as exc:  # pragma: no cover - defensive programming
        raise RuntimeError("BOOKSTORAGE_PORT doit être un entier valide") from exc

    return Settings(
        secret_key=secret_key,
        secret_from_env=secret_from_env,
        database=str(database_path),
        data_directory=str(data_directory),
        upload_folder=str(upload_folder),
        upload_url_path=upload_url_path,
        profile_upload_folder=str(avatar_folder),
        profile_upload_url_path=avatar_url_path,
        superadmin_username=os.environ.get("BOOKSTORAGE_SUPERADMIN_USERNAME", "superadmin"),
        superadmin_password=os.environ.get("BOOKSTORAGE_SUPERADMIN_PASSWORD", "SuperAdmin!2023"),
        environment=environment,
        host=host,
        port=port,
    )


def configure_app(app):
    """Apply the resolved settings to a Flask application instance."""

    settings = get_settings(app.root_path)

    app.config.setdefault("BOOKSTORAGE_SETTINGS", settings)
    app.config["SECRET_KEY"] = settings.secret_key
    app.secret_key = settings.secret_key
    app.config["DATABASE"] = settings.database
    app.config["BOOKSTORAGE_DATA_DIR"] = settings.data_directory
    app.config["UPLOAD_FOLDER"] = settings.upload_folder
    app.config["UPLOAD_URL_PATH"] = settings.upload_url_path
    app.config["PROFILE_UPLOAD_FOLDER"] = settings.profile_upload_folder
    app.config["PROFILE_UPLOAD_URL_PATH"] = settings.profile_upload_url_path
    app.config["DEFAULT_SUPERADMIN_USERNAME"] = settings.superadmin_username
    app.config["DEFAULT_SUPERADMIN_PASSWORD"] = settings.superadmin_password
    app.config["BOOKSTORAGE_ENV"] = settings.environment
    app.config["BOOKSTORAGE_SECRET_FROM_ENV"] = settings.secret_from_env
    app.config.setdefault("BOOKSTORAGE_HOST", settings.host)
    app.config.setdefault("BOOKSTORAGE_PORT", settings.port)

    if settings.environment == "production":
        # Harden session cookies by default when the production profile is
        # enabled.  Operators can still override these settings explicitly if
        # their deployment requires different values.
        app.config.setdefault("SESSION_COOKIE_SECURE", True)
        app.config.setdefault("SESSION_COOKIE_HTTPONLY", True)
        app.config.setdefault("REMEMBER_COOKIE_SECURE", True)
        app.config.setdefault("PREFERRED_URL_SCHEME", "https")
        app.config.setdefault("TEMPLATES_AUTO_RELOAD", False)

        if not settings.secret_from_env:
            raise RuntimeError(
                "BOOKSTORAGE_SECRET_KEY doit être défini dans l'environnement pour un déploiement en production."
            )

    return settings

