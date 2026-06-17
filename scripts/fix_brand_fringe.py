#!/usr/bin/env python3
"""Rogne le padding du logo navbar uniquement (les mascottes ne sont pas modifiées)."""

from collections import deque
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
BRAND = ROOT / "static" / "brand"
LOGO = BRAND / "logos" / "logo.png"


def flood_remove_dark(img: Image.Image, threshold: int = 48) -> Image.Image:
    img = img.convert("RGBA")
    w, h = img.size
    px = img.load()
    seen: set[tuple[int, int]] = set()
    q: deque[tuple[int, int]] = deque()
    for x in range(w):
        q.append((x, 0))
        q.append((x, h - 1))
    for y in range(h):
        q.append((0, y))
        q.append((w - 1, y))

    def dark(r: int, g: int, b: int) -> bool:
        return r <= threshold and g <= threshold and b <= threshold

    while q:
        x, y = q.popleft()
        if (x, y) in seen or x < 0 or y < 0 or x >= w or y >= h:
            continue
        seen.add((x, y))
        r, g, b, _a = px[x, y]
        if dark(r, g, b):
            px[x, y] = (r, g, b, 0)
            q.extend([(x + 1, y), (x - 1, y), (x, y + 1), (x, y - 1)])
    return img


def trim_alpha(img: Image.Image, padding: int = 2) -> Image.Image:
    bbox = img.getbbox()
    if not bbox:
        return img
    left, top, right, bottom = bbox
    return img.crop(
        (
            max(0, left - padding),
            max(0, top - padding),
            min(img.width, right + padding),
            min(img.height, bottom + padding),
        )
    )


def trim_logo_nav(path: Path) -> Image.Image:
    img = Image.open(path).convert("RGBA")
    img = flood_remove_dark(img, threshold=35)
    img = trim_alpha(img, padding=4)
    img.save(path, format="PNG", optimize=True)
    return img


def main() -> None:
    if LOGO.exists():
        out = trim_logo_nav(LOGO)
        print(f"  logos/logo.png: {out.size[0]}x{out.size[1]}")
    print("Done.")


if __name__ == "__main__":
    main()
