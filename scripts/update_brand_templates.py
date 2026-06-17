#!/usr/bin/env python3
"""Met à jour les templates pour le brand kit."""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "templates"

REPLACEMENTS = [
    (
        '<a class="brand" href="/dashboard">📚 BookStorage</a>',
        '{{template "site_brand_dashboard" .}}',
    ),
    (
        '<a class="brand" href="/">📚 BookStorage</a>',
        '{{template "site_brand" .}}',
    ),
    (
        '<a class="brand" href="/">📚 {{ .SiteName }}</a>',
        '{{template "site_brand" .}}',
    ),
    (
        '<a class="brand mobile-brand" href="/dashboard">📚 BookStorage</a>',
        '{{template "site_brand_mobile" .}}',
    ),
    (
        '<a class="error-brand" href="/">BookStorage</a>',
        '{{template "site_brand_error" .}}',
    ),
    ('<link rel="apple-touch-icon" href="/static/icons/favicon.svg">\n', ""),
    ('<link rel="apple-touch-icon" href="/static/icons/icon-192.png">\n', ""),
]


def main() -> None:
    for path in ROOT.rglob("*.gohtml"):
        text = path.read_text(encoding="utf-8")
        original = text
        for old, new in REPLACEMENTS:
            text = text.replace(old, new)
        if text != original:
            path.write_text(text, encoding="utf-8")
            print(f"updated {path.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
