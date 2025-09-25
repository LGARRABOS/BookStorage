import pytest

from app import _media_helpers, app as flask_app


@pytest.mark.parametrize(
    "stored_path, expected",
    [
        ("images/example.png", "/images/example.png"),
        ("avatars/profile.jpg", "/avatars/profile.jpg"),
    ],
)
def test_work_image_url_uses_media_routes(stored_path, expected):
    helpers = _media_helpers()
    work_image_url = helpers["work_image_url"]

    with flask_app.test_request_context():
        assert work_image_url(stored_path) == expected


def test_media_routes_follow_absolute_directories(tmp_path, monkeypatch):
    uploads_dir = tmp_path / "uploads"
    avatars_dir = tmp_path / "avatars"
    uploads_dir.mkdir()
    avatars_dir.mkdir()

    monkeypatch.setitem(flask_app.config, "UPLOAD_FOLDER", str(uploads_dir))
    monkeypatch.setitem(flask_app.config, "PROFILE_UPLOAD_FOLDER", str(avatars_dir))

    # Re-register routes with the updated configuration for this test.
    from app import _register_media_routes

    _register_media_routes()

    helpers = _media_helpers()
    work_image_url = helpers["work_image_url"]

    with flask_app.test_request_context():
        assert work_image_url("images/sample.png") == "/images/sample.png"
        assert work_image_url("avatars/pic.png") == "/avatars/pic.png"
