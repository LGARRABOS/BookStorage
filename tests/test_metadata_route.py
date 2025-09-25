import io
import sqlite3
from pathlib import Path
from types import SimpleNamespace

import app
from metadata_providers import MetadataSuggestion


def login_reader(client):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )


def test_metadata_endpoint_returns_json(monkeypatch, client):
    login_reader(client)

    fake_results = [
        MetadataSuggestion(
            title="Chainsaw Man",
            authors=["Tatsuki Fujimoto"],
            info_url="https://openlibrary.org/works/OL12345W",
            cover_url="https://covers.openlibrary.org/b/id/12345-L.jpg",
            reading_type="Manga",
            published_year=2019,
            summary="Denji lutte contre les démons.",
        )
    ]

    monkeypatch.setattr(app, "search_open_library", lambda query, available_types: fake_results)

    response = client.get("/api/metadata/search", query_string={"q": "Chainsaw"})
    assert response.status_code == 200
    payload = response.get_json()
    assert payload["results"][0]["title"] == "Chainsaw Man"
    assert payload["results"][0]["reading_type"] == "Manga"
    assert payload["results"][0]["cover_url"].endswith("12345-L.jpg")


def test_add_work_uses_metadata_cover(monkeypatch, client, get_user_record, database_path):
    login_reader(client)

    covers_dir = Path(client.application.config["UPLOAD_FOLDER"])
    covers_dir.mkdir(parents=True, exist_ok=True)

    fake_bytes = b"\xffDUMMY"

    class FakeResponse(io.BytesIO):
        def __init__(self):
            super().__init__(fake_bytes)
            self.status = 200
            self.headers = {"Content-Type": "image/jpeg"}

        def __enter__(self):
            self.seek(0)
            return self

        def __exit__(self, exc_type, exc, tb):
            self.close()

        def read(self, *args, **kwargs):
            return super().read(*args, **kwargs)

        def getcode(self):
            return self.status

    def opener(url, timeout=5):
        assert "covers.openlibrary.org" in url
        return FakeResponse()

    monkeypatch.setattr(app.uuid, "uuid4", lambda: SimpleNamespace(hex="metadatauuid"))

    image_path = app._download_remote_cover(
        "https://covers.openlibrary.org/b/id/999-L.jpg",
        opener=opener,
    )

    assert image_path is not None
    stored_file = covers_dir / Path(image_path).name
    assert stored_file.exists()
    assert stored_file.read_bytes() == fake_bytes

    monkeypatch.setattr(app, "_download_remote_cover", lambda url: image_path)

    response = client.post(
        "/add_work",
        data={
            "title": "Chainsaw Man",
            "status": "En cours",
            "chapter": "12",
            "reading_type": "Manga",
            "metadata_cover_url": "https://covers.openlibrary.org/b/id/999-L.jpg",
            "metadata_info_url": "https://openlibrary.org/works/OL12345W",
            "link": "",
        },
        follow_redirects=True,
    )
    assert "Oeuvre ajoutée" in response.get_data(as_text=True)

    reader = get_user_record("reader")
    conn = sqlite3.connect(database_path)
    conn.row_factory = sqlite3.Row
    try:
        work = conn.execute(
            "SELECT * FROM works WHERE user_id = ? AND title = ?",
            (reader["id"], "Chainsaw Man"),
        ).fetchone()
    finally:
        conn.close()

    assert work is not None
    assert work["link"] == "https://openlibrary.org/works/OL12345W"
    assert work["image_path"] == image_path
