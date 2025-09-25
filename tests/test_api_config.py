from __future__ import annotations

import json
from pathlib import Path

from api_config import APISettings, load_api_settings


def test_missing_file_returns_empty_settings(tmp_path, monkeypatch):
    monkeypatch.delenv("BOOKSTORAGE_API_CONFIG", raising=False)

    settings = load_api_settings(tmp_path)

    assert settings == APISettings(None, None, None, None, None, None)


def test_load_api_settings_reads_values(tmp_path, monkeypatch):
    monkeypatch.delenv("BOOKSTORAGE_API_CONFIG", raising=False)
    config_path = tmp_path / "api_keys.json"
    config_path.write_text(
        json.dumps(
            {
                "google_books_api_key": "  token  ",
                "kitsu_client_id": "client",
                "kitsu_client_secret": "secret",
                "anilist_client_id": "",
                "anilist_client_secret": None,
                "comic_vine_api_key": "123",
            }
        ),
        encoding="utf-8",
    )

    settings = load_api_settings(tmp_path)

    assert settings.google_books_api_key == "token"
    assert settings.kitsu_client_id == "client"
    assert settings.kitsu_client_secret == "secret"
    assert settings.anilist_client_id is None
    assert settings.anilist_client_secret is None
    assert settings.comic_vine_api_key == "123"


def test_environment_override_accepts_absolute_path(tmp_path, monkeypatch):
    config_path = tmp_path / "custom.json"
    config_path.write_text(json.dumps({"google_books_api_key": "abc"}), encoding="utf-8")
    monkeypatch.setenv("BOOKSTORAGE_API_CONFIG", str(config_path))

    settings = load_api_settings(Path("/does/not/matter"))

    assert settings.google_books_api_key == "abc"
    assert settings.kitsu_client_id is None
