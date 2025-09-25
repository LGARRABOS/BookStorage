"""Dedicated loader for optional third-party API credentials.

This module provides a lightweight configuration layer separate from the main
application settings so administrators immediately know where to drop their API
keys.  The loader tolerates missing files and incomplete definitions so the app
remains fully functional even when no integrations are configured yet.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Optional

DEFAULT_API_CONFIG_NAME = "api_keys.json"


@dataclass(frozen=True)
class APISettings:
    """Collected credentials for optional external services."""

    google_books_api_key: Optional[str]
    kitsu_client_id: Optional[str]
    kitsu_client_secret: Optional[str]
    anilist_client_id: Optional[str]
    anilist_client_secret: Optional[str]
    comic_vine_api_key: Optional[str]

    def to_dict(self) -> Dict[str, Optional[str]]:
        """Expose the credentials as a mapping for convenient templating."""

        return {
            "google_books_api_key": self.google_books_api_key,
            "kitsu_client_id": self.kitsu_client_id,
            "kitsu_client_secret": self.kitsu_client_secret,
            "anilist_client_id": self.anilist_client_id,
            "anilist_client_secret": self.anilist_client_secret,
            "comic_vine_api_key": self.comic_vine_api_key,
        }


def _normalise(value: Any) -> Optional[str]:
    if value is None:
        return None
    if isinstance(value, str):
        stripped = value.strip()
        return stripped or None
    return str(value)


def load_api_settings(root_path: Path | str) -> APISettings:
    """Load API credentials from JSON if available.

    Parameters
    ----------
    root_path:
        Base directory used to resolve relative configuration paths.  This is
        typically the Flask application's ``root_path``.
    """

    root = Path(root_path)
    candidate = os.environ.get("BOOKSTORAGE_API_CONFIG", DEFAULT_API_CONFIG_NAME).strip()
    config_path = Path(candidate) if candidate else Path(DEFAULT_API_CONFIG_NAME)
    if not config_path.is_absolute():
        config_path = root / config_path

    if not config_path.exists():
        return APISettings(None, None, None, None, None, None)

    try:
        with config_path.open("r", encoding="utf-8") as handle:
            raw: Dict[str, Any] = json.load(handle)
    except json.JSONDecodeError as exc:
        raise RuntimeError(
            f"Impossible de lire les clefs API depuis {config_path}: JSON invalide"
        ) from exc
    except OSError as exc:
        raise RuntimeError(
            f"Impossible d'ouvrir le fichier de clefs API {config_path}"
        ) from exc

    return APISettings(
        google_books_api_key=_normalise(raw.get("google_books_api_key")),
        kitsu_client_id=_normalise(raw.get("kitsu_client_id")),
        kitsu_client_secret=_normalise(raw.get("kitsu_client_secret")),
        anilist_client_id=_normalise(raw.get("anilist_client_id")),
        anilist_client_secret=_normalise(raw.get("anilist_client_secret")),
        comic_vine_api_key=_normalise(raw.get("comic_vine_api_key")),
    )
