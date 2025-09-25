import io
import json

import pytest

from metadata_providers import search_open_library


class DummyResponse(io.BytesIO):
    def __init__(self, payload: dict, status: int = 200):
        data = json.dumps(payload).encode("utf-8")
        super().__init__(data)
        self.status = status
        self.headers = {"Content-Type": "application/json"}

    def __enter__(self):
        self.seek(0)
        return self

    def __exit__(self, exc_type, exc, tb):
        self.close()

    def read(self, *args, **kwargs):
        return super().read(*args, **kwargs)

    def getcode(self):
        return self.status


@pytest.mark.parametrize(
    "subjects,expected",
    [
        (["Manga", "Adventure"], "Manga"),
        (["Graphic Novels", "Comic books"], "Comics"),
        ([], "Roman"),
    ],
)
def test_search_open_library_normalises_results(subjects, expected):
    payload = {
        "docs": [
            {
                "title": "Naruto",
                "author_name": ["Masashi Kishimoto"],
                "cover_i": 12345,
                "key": "/works/OL123W",
                "first_publish_year": 1999,
                "subject_facet": subjects,
                "first_sentence": {"value": "Un jeune ninja rêve de reconnaissance."},
            }
        ]
    }

    def opener(url, timeout=5):
        assert "title=Naruto" in url
        return DummyResponse(payload)

    suggestions = search_open_library(
        "Naruto",
        available_types=["Roman", "Manga", "Comics"],
        limit=5,
        opener=opener,
    )

    assert len(suggestions) == 1
    suggestion = suggestions[0]
    assert suggestion.title == "Naruto"
    assert suggestion.authors == ["Masashi Kishimoto"]
    assert suggestion.cover_url == "https://covers.openlibrary.org/b/id/12345-L.jpg"
    assert suggestion.info_url == "https://openlibrary.org/works/OL123W"
    assert suggestion.published_year == 1999
    assert suggestion.summary == "Un jeune ninja rêve de reconnaissance."
    assert suggestion.reading_type == expected


def test_search_open_library_handles_errors_gracefully():
    def failing_opener(url, timeout=5):
        raise OSError("network down")

    results = search_open_library(
        "Titre",
        available_types=["Roman"],
        opener=failing_opener,
    )

    assert results == []

    bad_payload = DummyResponse({"docs": ["not-a-dict"]})

    def invalid_opener(url, timeout=5):
        return bad_payload

    results = search_open_library(
        "Titre",
        available_types=["Roman"],
        opener=invalid_opener,
    )

    assert results == []
