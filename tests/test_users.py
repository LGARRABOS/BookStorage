import sqlite3


def test_users_directory_requires_login(client):
    response = client.get("/users")
    assert response.status_code == 302
    assert "/login" in response.headers["Location"]


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
