import io


def test_profile_requires_login(client):
    response = client.get("/profile")
    assert response.status_code == 302
    assert "/login" in response.headers["Location"]


def test_profile_update_persists_changes(client, get_user_record):
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.post(
        "/profile",
        data={
            "username": "reader",
            "display_name": "Lecteur passionné",
            "email": "lecteur@example.com",
            "bio": "Fan de science-fiction et de fantasy.",
        },
        follow_redirects=True,
    )
    page = response.get_data(as_text=True)
    assert "Profil mis à jour." in page

    updated = get_user_record("reader")
    assert updated["display_name"] == "Lecteur passionné"
    assert updated["email"] == "lecteur@example.com"
    assert updated["bio"] == "Fan de science-fiction et de fantasy."


def test_username_change_requires_current_password(client, get_user_record):
    original = get_user_record("reader")
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.post(
        "/profile",
        data={
            "username": "nouveau_pseudo",
            "display_name": original["display_name"] or "",
            "email": original["email"] or "",
            "bio": original["bio"] or "",
        },
        follow_redirects=True,
    )
    page = response.get_data(as_text=True)
    assert "Veuillez renseigner votre mot de passe actuel" in page

    assert get_user_record("reader") is not None
    assert get_user_record("nouveau_pseudo") is None


def test_password_change_allows_new_login(client, get_user_record):
    reader = get_user_record("reader")
    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    response = client.post(
        "/profile",
        data={
            "username": "reader",
            "display_name": reader["display_name"] or "",
            "email": reader["email"] or "",
            "bio": reader["bio"] or "",
            "current_password": "ReaderPass!1",
            "new_password": "MyNewPass!2",
            "confirm_password": "MyNewPass!2",
        },
        follow_redirects=True,
    )
    assert "Profil mis à jour." in response.get_data(as_text=True)

    client.get("/logout")
    relogin = client.post(
        "/login",
        data={"username": "reader", "password": "MyNewPass!2"},
        follow_redirects=True,
    )
    assert "Connexion réussie." in relogin.get_data(as_text=True)


def test_avatar_upload_saves_file(client, get_user_record, tmp_path, monkeypatch):
    reader = get_user_record("reader")
    avatar_dir = tmp_path / "avatars"
    avatar_dir.mkdir()

    monkeypatch.setitem(
        client.application.config,
        "PROFILE_UPLOAD_FOLDER",
        str(avatar_dir),
    )

    client.post(
        "/login",
        data={"username": "reader", "password": "ReaderPass!1"},
        follow_redirects=True,
    )

    fake_image = io.BytesIO(b"fake image data")
    fake_image.name = "avatar.png"

    response = client.post(
        "/profile",
        data={
            "username": "reader",
            "display_name": reader["display_name"] or "",
            "email": reader["email"] or "",
            "bio": reader["bio"] or "",
            "avatar": (fake_image, "avatar.png"),
        },
        content_type="multipart/form-data",
        follow_redirects=True,
    )
    assert "Profil mis à jour." in response.get_data(as_text=True)

    updated = get_user_record("reader")
    assert updated["avatar_path"] is not None
    stored_file = avatar_dir / updated["avatar_path"].split("/")[-1]
    assert stored_file.exists()
