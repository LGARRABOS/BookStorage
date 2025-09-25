"""Utilities to retrieve metadata about works from public APIs."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Iterable, List, Optional, Sequence
from urllib import error, parse, request


OPEN_LIBRARY_ENDPOINT = "https://openlibrary.org/search.json"
DEFAULT_RESULT_LIMIT = 8


READING_TYPE_KEYWORDS = {
    "Manga": {"manga"},
    "BD": {"bd", "bande dessinée", "bandes dessinées"},
    "Manhwa": {"manhwa"},
    "Light Novel": {"light novel", "light novels"},
    "Comics": {"comic", "comics", "graphic novel", "graphic novels"},
    "Webtoon": {"webtoon", "webtoons", "webcomic", "webcomics"},
}


@dataclass(frozen=True)
class MetadataSuggestion:
    """Structured information returned by a metadata provider."""

    title: str
    authors: List[str]
    info_url: Optional[str]
    cover_url: Optional[str]
    reading_type: str
    published_year: Optional[int]
    summary: Optional[str]

    def to_dict(self) -> dict:
        """Return a JSON-serialisable representation."""

        return {
            "title": self.title,
            "authors": self.authors,
            "info_url": self.info_url,
            "cover_url": self.cover_url,
            "reading_type": self.reading_type,
            "published_year": self.published_year,
            "summary": self.summary,
        }


def _normalise_summary(doc: dict) -> Optional[str]:
    summary = doc.get("first_sentence") or doc.get("subtitle")
    if isinstance(summary, dict):
        summary = summary.get("value")
    if isinstance(summary, list):
        summary = " ".join([part for part in summary if isinstance(part, str)])
    if isinstance(summary, str):
        summary = summary.strip()
    return summary or None


def _normalise_authors(doc: dict) -> List[str]:
    names = doc.get("author_name") or []
    return [name for name in names if isinstance(name, str) and name.strip()]


def _extract_subjects(doc: dict) -> List[str]:
    subjects = doc.get("subject_facet") or doc.get("subject") or []
    if not isinstance(subjects, Iterable):
        return []
    normalised = []
    for subject in subjects:
        if isinstance(subject, str):
            normalised.append(subject.strip().lower())
    return normalised


def _guess_reading_type(
    *,
    available_types: Sequence[str],
    subjects: Sequence[str],
    title: str,
) -> str:
    if not available_types:
        return "Autre"

    lowered_title = title.lower()
    for reading_type in available_types:
        keywords = READING_TYPE_KEYWORDS.get(reading_type, set())
        if not keywords:
            continue
        for keyword in keywords:
            if keyword in lowered_title:
                return reading_type
            if any(keyword in subject for subject in subjects):
                return reading_type

    return available_types[0]


def search_open_library(
    query: str,
    *,
    available_types: Sequence[str],
    limit: int = DEFAULT_RESULT_LIMIT,
    opener=request.urlopen,
) -> List[MetadataSuggestion]:
    """Search the Open Library catalogue and normalise the output."""

    if not query or not query.strip():
        return []

    payload = parse.urlencode({"title": query.strip(), "limit": str(limit)})
    url = f"{OPEN_LIBRARY_ENDPOINT}?{payload}"

    try:
        with opener(url, timeout=5) as response:
            status = getattr(response, "status", None)
            if status is None and hasattr(response, "getcode"):
                status = response.getcode()
            if status != 200:
                return []
            raw = response.read()
    except (error.URLError, TimeoutError, ValueError, OSError):
        return []

    try:
        data = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return []

    docs = data.get("docs", [])
    suggestions: List[MetadataSuggestion] = []
    for doc in docs:
        if not isinstance(doc, dict):
            continue
        title = doc.get("title")
        if not isinstance(title, str) or not title.strip():
            continue

        cover_id = doc.get("cover_i")
        cover_url = None
        if isinstance(cover_id, int):
            cover_url = f"https://covers.openlibrary.org/b/id/{cover_id}-L.jpg"

        info_url = None
        key = doc.get("key")
        if isinstance(key, str) and key.strip():
            info_url = f"https://openlibrary.org{key}"

        summary = _normalise_summary(doc)
        authors = _normalise_authors(doc)
        subjects = _extract_subjects(doc)
        reading_type = _guess_reading_type(
            available_types=available_types,
            subjects=subjects,
            title=title,
        )
        published = doc.get("first_publish_year")
        published_year = int(published) if isinstance(published, int) else None

        suggestions.append(
            MetadataSuggestion(
                title=title.strip(),
                authors=authors,
                info_url=info_url,
                cover_url=cover_url,
                reading_type=reading_type,
                published_year=published_year,
                summary=summary,
            )
        )

    return suggestions
