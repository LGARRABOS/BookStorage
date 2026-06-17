#!/usr/bin/env python3
"""Découpe le brand kit source en assets pour static/brand/."""

from pathlib import Path

from collections import deque

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "static" / "brand" / "source" / "brand-kit.png"
BRAND = ROOT / "static" / "brand"

# Grille 1024×559 — layout réel du kit :
# haut-gauche : logo | haut-centre : app icons | haut-droite : favicons
# bas-gauche : mascottes | bas-droite : bannière 21:9
CROPS: dict[str, tuple[int, int, int, int]] = {
    "logos/logo-full.png": (8, 26, 282, 200),
    "logos/wordmark.png": (12, 210, 275, 248),
    "ui/search.png": (246, 126, 342, 194),
    "ui/add-book.png": (352, 126, 448, 194),
    "ui/settings.png": (458, 126, 554, 194),
    "favicon/favicon-16.png": (700, 124, 732, 156),
    "favicon/favicon-32.png": (762, 118, 826, 182),
    "favicon/favicon-64.png": (888, 112, 954, 178),
    "mascots/profile-female.png": (18, 378, 108, 468),
    "mascots/profile-male.png": (122, 378, 212, 468),
    "banners/hero-webtoon.png": (472, 374, 1018, 546),
}

# Fond menthe du kit (zone vide colonne gauche)
BG_SAMPLE = (30, 280)
BG_TOLERANCE = 38


def crop_box(im: Image.Image, box: tuple[int, int, int, int]) -> Image.Image:
    return im.crop(box)


def bg_color(im: Image.Image) -> tuple[int, int, int]:
    return im.getpixel(BG_SAMPLE)[:3]


def flood_transparent(img: Image.Image, source_bg: tuple[int, int, int], tolerance: int = BG_TOLERANCE) -> Image.Image:
    """Retire le fond menthe connecté aux bords (préserve le texte/logo)."""
    img = img.convert("RGBA")
    w, h = img.size
    sr, sg, sb = source_bg
    pixels = img.load()
    seen: set[tuple[int, int]] = set()
    q: deque[tuple[int, int]] = deque()

    def matches(r: int, g: int, b: int) -> bool:
        return abs(r - sr) <= tolerance and abs(g - sg) <= tolerance and abs(b - sb) <= tolerance

    for x in range(w):
        q.append((x, 0))
        q.append((x, h - 1))
    for y in range(h):
        q.append((0, y))
        q.append((w - 1, y))

    while q:
        x, y = q.popleft()
        if (x, y) in seen or x < 0 or y < 0 or x >= w or y >= h:
            continue
        seen.add((x, y))
        r, g, b, a = pixels[x, y]
        if matches(r, g, b):
            pixels[x, y] = (r, g, b, 0)
            q.extend([(x + 1, y), (x - 1, y), (x, y + 1), (x, y - 1)])

    return img


def remove_kit_background(img: Image.Image, source_bg: tuple[int, int, int], tolerance: int = BG_TOLERANCE) -> Image.Image:
    return flood_transparent(img, source_bg, tolerance)


def trim_alpha(img: Image.Image, padding: int = 2) -> Image.Image:
    bbox = img.getbbox()
    if not bbox:
        return img
    left = max(0, bbox[0] - padding)
    top = max(0, bbox[1] - padding)
    right = min(img.width, bbox[2] + padding)
    bottom = min(img.height, bbox[3] + padding)
    return img.crop((left, top, right, bottom))


def save_png(path: Path, img: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, format="PNG", optimize=True)


def make_logo_email(logo_full: Image.Image) -> Image.Image:
    return logo_full.resize((96, 96), Image.Resampling.LANCZOS)


def make_pwa_icons(favicon_64: Image.Image) -> None:
    for size, name in ((192, "icon-192.png"), (512, "icon-512.png")):
        icon = favicon_64.resize((size, size), Image.Resampling.LANCZOS)
        save_png(BRAND / "pwa" / name, icon)


def make_favicon_ico(f16: Image.Image, f32: Image.Image) -> None:
    f32.save(
        BRAND / "favicon" / "favicon.ico",
        format="ICO",
        sizes=[(32, 32), (16, 16)],
        append_images=[f16],
    )


def resize_banner(banner: Image.Image) -> Image.Image:
    return banner.resize((1920, 823), Image.Resampling.LANCZOS)


def save_debug_sheet(im: Image.Image, crops: dict[str, tuple[int, int, int, int]]) -> None:
    dbg = im.copy()
    draw = ImageDraw.Draw(dbg)
    for name, box in crops.items():
        draw.rectangle(box, outline="red", width=2)
        draw.text((box[0] + 2, box[1] + 2), name.split("/")[-1][:14], fill="red")
    save_png(BRAND / "source" / "debug-crops.png", dbg)


def post_process(rel: str, img: Image.Image, source_bg: tuple[int, int, int]) -> Image.Image:
    if rel == "banners/hero-webtoon.png":
        return img
    if rel.startswith("favicon/") or rel.startswith("ui/"):
        return trim_alpha(img)
    return trim_alpha(flood_transparent(img, source_bg))


def main() -> None:
    im = Image.open(SRC).convert("RGBA")
    w, h = im.size
    if w != 1024 or h != 559:
        print(f"Warning: expected 1024x559, got {w}x{h}")

    kit_bg = bg_color(im)
    print(f"Kit background: {kit_bg}")
    save_debug_sheet(im, CROPS)

    outputs: dict[str, Image.Image] = {}
    for rel, box in CROPS.items():
        cropped = crop_box(im, box)
        if rel == "banners/hero-webtoon.png":
            cropped = resize_banner(cropped)
        cropped = post_process(rel, cropped, kit_bg)
        save_png(BRAND / rel, cropped)
        outputs[rel] = cropped
        print(f"  {rel}: {cropped.size}")

    save_png(BRAND / "logos" / "logo-email.png", make_logo_email(outputs["logos/logo-full.png"]))
    make_pwa_icons(outputs["favicon/favicon-64.png"])
    make_favicon_ico(outputs["favicon/favicon-16.png"], outputs["favicon/favicon-32.png"])

    print("Done.")


if __name__ == "__main__":
    main()
