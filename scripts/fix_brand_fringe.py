#!/usr/bin/env python3
"""Retire le fond noir et les halos blancs en bordure des mascottes / rogne le logo navbar."""

from collections import deque
from pathlib import Path

from PIL import Image, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
BRAND = ROOT / "static" / "brand"
MASCOTS = (
    "hero-female.png",
    "hero-male.png",
)
PROFILE_MASCOTS = (
    "femmal.png",
    "male.png",
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


def erode_alpha(img: Image.Image, radius: int) -> Image.Image:
    if radius <= 0:
        return img
    img = img.convert("RGBA")
    r, g, b, a = img.split()
    for _ in range(radius):
        a = a.filter(ImageFilter.MinFilter(3))
    return Image.merge("RGBA", (r, g, b, a))


def dilate_alpha(img: Image.Image, radius: int) -> Image.Image:
    if radius <= 0:
        return img
    img = img.convert("RGBA")
    r, g, b, a = img.split()
    for _ in range(radius):
        a = a.filter(ImageFilter.MaxFilter(3))
    return Image.merge("RGBA", (r, g, b, a))


def strip_white_fringe(img: Image.Image, max_passes: int = 8) -> Image.Image:
    """Retire les pixels quasi blancs au contact direct du transparent."""
    img = img.convert("RGBA")
    w, h = img.size
    px = img.load()
    for _ in range(max_passes):
        to_clear: list[tuple[int, int]] = []
        for y in range(h):
            for x in range(w):
                r, g, b, a = px[x, y]
                if a == 0:
                    continue
                lum = (r + g + b) / 3
                sat = max(r, g, b) - min(r, g, b)
                if lum < 208 or sat > 24:
                    continue
                touches_transparent = False
                for dx, dy in ((-1, 0), (1, 0), (0, -1), (0, 1)):
                    nx, ny = x + dx, y + dy
                    if nx < 0 or ny < 0 or nx >= w or ny >= h or px[nx, ny][3] == 0:
                        touches_transparent = True
                        break
                if touches_transparent:
                    to_clear.append((x, y))
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


def process_mascot(name: str) -> Image.Image:
    path = BRAND / "mascots" / name
    img = load_mascot_source(name)
    img = flood_remove_dark(img)
    img = strip_white_fringe(img, max_passes=10)
    if name.startswith("hero-"):
        img = erode_alpha(img, 2)
        img = dilate_alpha(img, 1)
    else:
        img = erode_alpha(img, 1)
    img = strip_white_fringe(img, max_passes=6)
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
    for name in MASCOTS:
        path = BRAND / "mascots" / name
        if not path.exists() and not (SOURCE_DIR / name).exists():
            print(f"skip missing {name}")
            continue
        out = process_mascot(name)
        print(f"  mascots/{name}: {out.size[0]}x{out.size[1]}")
    for name in PROFILE_MASCOTS:
        source = SOURCE_DIR / name
        if not source.exists():
            continue
        out = process_mascot(name)
        print(f"  mascots/{name}: {out.size[0]}x{out.size[1]}")
    if LOGO.exists():
        out = trim_logo_nav(LOGO)
        print(f"  logos/logo.png: {out.size[0]}x{out.size[1]}")
    print("Done.")


if __name__ == "__main__":
    main()
