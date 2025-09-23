import io
import sqlite3
from pathlib import Path


def test_users_directory_requires_login(client):
    response = client.get("/users")
    assert response.status_code == 302
    assert "/login" in response.headers["Location"]


def test_dashboard_exposes_profile_and_directory_shortcuts(client):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.get("/dashboard")
    page = response.get_data(as_text=True)
    assert "Accéder à mon profil" in page
    assert "Ouvrir l'annuaire" in page


def test_users_directory_lists_public_users(client):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.get("/users", follow_redirects=True)
    page = response.get_data(as_text=True)
    assert "Partageur" in page
    assert "Confidentiel" not in page


def test_user_search_filters_results(client):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.get("/users?q=parta", follow_redirects=True)
    page = response.get_data(as_text=True)
    assert "Partageur" in page
    assert "Administrateur" not in page


def test_private_profile_inaccessible_to_other_users(client, get_user_record):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    private_user = get_user_record("private")
    response = client.get(f"/users/{private_user['id']}", follow_redirects=True)
    page = response.get_data(as_text=True)
    assert "Ce profil est privé." in page


def test_private_profile_accessible_to_owner(client, get_user_record):
    private_user = get_user_record("private")
    client.post(
        "/login",
        data={"username": "private", "password": "SecretPass!1"},
        follow_redirects=True,
    )

    response = client.get(f"/users/{private_user['id']}")
    page = response.get_data(as_text=True)
    assert response.status_code == 200
    assert "Confidentiel" in page


def test_private_profile_accessible_to_admin(client, get_user_record):
    private_user = get_user_record("private")
    client.post(
        "/login",
        data={"username": "admin", "password": "AdminPower!1"},
        follow_redirects=True,
    )

    response = client.get(f"/users/{private_user['id']}")
    assert response.status_code == 200
    assert "Confidentiel" in response.get_data(as_text=True)


def test_import_work_from_public_profile(client, get_user_record, database_path):
    sharer = get_user_record("sharer")
    reader = get_user_record("reader")

    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    # Récupère une œuvre à partager
    conn = sqlite3.connect(database_path)
    conn.row_factory = sqlite3.Row
    try:
        work = conn.execute(
            "SELECT * FROM works WHERE user_id = ? LIMIT 1",
            (sharer["id"],),
        ).fetchone()
        assert work is not None
    finally:
        conn.close()

    response = client.post(
        f"/users/{sharer['id']}/import/{work['id']}",
        follow_redirects=True,
    )
    page = response.get_data(as_text=True)
    assert "Œuvre ajoutée à votre liste de lecture." in page

    conn = sqlite3.connect(database_path)
    conn.row_factory = sqlite3.Row
    try:
        imported = conn.execute(
            "SELECT * FROM works WHERE user_id = ? AND title = ?",
            (reader["id"], work["title"]),
        ).fetchone()
    finally:
        conn.close()

    assert imported is not None
    assert imported["chapter"] == work["chapter"]
    assert imported["link"] == work["link"]


def test_deleting_work_removes_image_file(client, get_user_record, database_path):
    reader = get_user_record("reader")
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    image = io.BytesIO(b"cover image")
    image.name = "cover.png"

    response = client.post(
        "/add_work",
        data={
            "title": "Nouvelle lecture",
            "status": "En cours",
            "chapter": "1",
            "link": "",
            "image": (image, "cover.png"),
        },
        content_type="multipart/form-data",
        follow_redirects=True,
    )
    assert "Oeuvre ajoutée" in response.get_data(as_text=True)

    conn = sqlite3.connect(database_path)
    conn.row_factory = sqlite3.Row
    try:
        work = conn.execute(
            "SELECT * FROM works WHERE user_id = ? AND title = ?",
            (reader["id"], "Nouvelle lecture"),
        ).fetchone()
    finally:
        conn.close()

    assert work is not None
    assert work["image_path"] is not None

    works_dir = Path(client.application.config["UPLOAD_FOLDER"])
    stored_file = works_dir / Path(work["image_path"]).name
    assert stored_file.exists()

    delete_response = client.get(f"/delete/{work['id']}", follow_redirects=True)
    assert "Oeuvre supprimée." in delete_response.get_data(as_text=True)
    assert not stored_file.exists()

    conn = sqlite3.connect(database_path)
    try:
        deleted = conn.execute(
            "SELECT * FROM works WHERE id = ?",
            (work["id"],),
        ).fetchone()
    finally:
        conn.close()

    assert deleted is None


def test_deleting_work_preserves_shared_image(client, get_user_record, database_path):
    reader = get_user_record("reader")
    works_dir = Path(client.application.config["UPLOAD_FOLDER"])
    works_dir.mkdir(parents=True, exist_ok=True)
    shared_file = works_dir / "shared.png"
    shared_file.write_bytes(b"shared image")

    conn = sqlite3.connect(database_path)
    try:
        inserted_ids = []
        for title in ("Lecture partagée A", "Lecture partagée B"):
            cursor = conn.execute(
                """
                INSERT INTO works (title, chapter, link, status, image_path, user_id)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (title, 0, None, "En cours", "images/shared.png", reader["id"]),
            )
            inserted_ids.append(cursor.lastrowid)
        conn.commit()
    finally:
        conn.close()

    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    client.get(f"/delete/{inserted_ids[0]}", follow_redirects=True)

    assert shared_file.exists()

    conn = sqlite3.connect(database_path)
    try:
        remaining = conn.execute(
            "SELECT COUNT(*) FROM works WHERE image_path = ?",
            ("images/shared.png",),
        ).fetchone()[0]
    finally:
        conn.close()

    assert remaining == 1
