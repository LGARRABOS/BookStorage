#!/usr/bin/env python3
"""Prépare les mascottes hero : fond noir → transparent, sans érosion destructive."""

from collections import deque
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
BRAND = ROOT / "static" / "brand"
HERO_MASCOTS = (
    "hero-female.png",
    "hero-male.png",
)
LOGO = BRAND / "logos" / "logo.png"
SOURCE_DIR = BRAND / "mascots" / "source"


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


def unmatte_white_edges(img: Image.Image) -> Image.Image:
    """Corrige les pixels semi-transparents issus d'un détourage sur fond blanc."""
    img = img.convert("RGBA")
    w, h = img.size
    px = img.load()
    for y in range(h):
        for x in range(w):
            r, g, b, a = px[x, y]
            if a == 0 or a == 255:
                continue
            af = a / 255.0
            fr = (r - (1.0 - af) * 255.0) / af
            fg = (g - (1.0 - af) * 255.0) / af
            fb = (b - (1.0 - af) * 255.0) / af
            if fr < 8 and fg < 8 and fb < 8:
                px[x, y] = (0, 0, 0, 0)
                continue
            px[x, y] = (
                max(0, min(255, int(fr))),
                max(0, min(255, int(fg))),
                max(0, min(255, int(fb))),
                a,
            )
    return img


def peel_opaque_white_border(img: Image.Image, max_passes: int = 8) -> Image.Image:
    """Retire les pixels quasi blancs opaques au bord (matte d'export IA)."""
    img = img.convert("RGBA")
    w, h = img.size
    px = img.load()

    def near_white(r: int, g: int, b: int) -> bool:
        lum = (r + g + b) / 3
        sat = max(r, g, b) - min(r, g, b)
        return lum >= 212 and sat <= 22

    for _ in range(max_passes):
        to_clear: list[tuple[int, int]] = []
        for y in range(h):
            for x in range(w):
                r, g, b, a = px[x, y]
                if a == 0 or not near_white(r, g, b):
                    continue
                for dx, dy in ((-1, 0), (1, 0), (0, -1), (0, 1)):
                    nx, ny = x + dx, y + dy
                    if nx < 0 or ny < 0 or nx >= w or ny >= h or px[nx, ny][3] == 0:
                        to_clear.append((x, y))
                        break
        if not to_clear:
            break
        for x, y in to_clear:
            px[x, y] = (0, 0, 0, 0)
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


def load_mascot_source(name: str) -> Image.Image:
    source = SOURCE_DIR / name
    path = source if source.exists() else BRAND / "mascots" / name
    return Image.open(path).convert("RGBA")


def process_hero_mascot(name: str) -> Image.Image:
    path = BRAND / "mascots" / name
    img = load_mascot_source(name)
    img = flood_remove_dark(img)
    img = unmatte_white_edges(img)
    img = peel_opaque_white_border(img, max_passes=10)
    img = trim_alpha(img)
    img.save(path, format="PNG", optimize=True)
    return img


def trim_logo_nav(path: Path) -> Image.Image:
    img = Image.open(path).convert("RGBA")
    img = flood_remove_dark(img, threshold=35)
    img = trim_alpha(img, padding=4)
    img.save(path, format="PNG", optimize=True)
    return img


def main() -> None:
    for name in HERO_MASCOTS:
        path = BRAND / "mascots" / name
        if not path.exists() and not (SOURCE_DIR / name).exists():
            print(f"skip missing {name}")
            continue
        out = process_hero_mascot(name)
        print(f"  mascots/{name}: {out.size[0]}x{out.size[1]}")
    if LOGO.exists():
        out = trim_logo_nav(LOGO)
        print(f"  logos/logo.png: {out.size[0]}x{out.size[1]}")
    print("Done.")


if __name__ == "__main__":
    main()
