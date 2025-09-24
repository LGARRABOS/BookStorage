import binascii
from hashlib import scrypt

import app as flask_app


def test_superadmin_can_delete_admin(client, get_user_record):
    admin_record = get_user_record("admin")
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
        f"/admin/delete_account/{admin_record['id']}", follow_redirects=True
    )
    assert b"Compte supprim\xc3\xa9." in delete_response.data

    assert get_user_record("admin") is None


def test_admin_cannot_delete_superadmin(client, get_user_record):
    superadmin_record = get_user_record("superadmin")
    assert superadmin_record is not None

    response = client.post(
        "/login",
        data={"username": "admin", "password": "AdminPower!1"},
        follow_redirects=True,
    )
    assert b"Connexion r\xc3\xa9ussie." in response.data

    delete_response = client.get(
        f"/admin/delete_account/{superadmin_record['id']}",
        follow_redirects=True,
    )
    delete_page = delete_response.get_data(as_text=True)
    assert "Seul un super" in delete_page

    assert get_user_record("superadmin") is not None


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
