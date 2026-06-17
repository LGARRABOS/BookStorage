#!/usr/bin/env python3
"""Découpe le brand kit source en assets pour static/brand/."""

from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "static" / "brand" / "source" / "brand-kit.png"
BRAND = ROOT / "static" / "brand"

# Coordonnées (left, top, right, bottom) pour image 1024×559
CROPS = {
    "logos/logo-full.png": (12, 38, 278, 168),
    "logos/wordmark.png": (18, 172, 268, 212),
    "ui/search.png": (22, 248, 112, 338),
    "ui/add-book.png": (128, 248, 218, 338),
    "ui/settings.png": (234, 248, 324, 338),
    "favicon/favicon-16.png": (328, 286, 348, 306),
    "favicon/favicon-32.png": (352, 278, 384, 310),
    "favicon/favicon-64.png": (392, 268, 456, 332),
    "mascots/profile-female.png": (558, 248, 648, 338),
    "mascots/profile-male.png": (662, 248, 752, 338),
    "banners/hero-webtoon.png": (8, 358, 1016, 548),
}


def crop_box(im: Image.Image, box: tuple[int, int, int, int]) -> Image.Image:
    return im.crop(box)


def save_png(path: Path, img: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, format="PNG", optimize=True)


def make_logo_email(logo_full: Image.Image) -> Image.Image:
    size = 96
    return logo_full.resize((size, size), Image.Resampling.LANCZOS)


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


def main() -> None:
    im = Image.open(SRC).convert("RGBA")
    w, h = im.size
    if w != 1024 or h != 559:
        print(f"Warning: expected 1024x559, got {w}x{h}")

    outputs: dict[str, Image.Image] = {}
    for rel, box in CROPS.items():
        cropped = crop_box(im, box)
        out = BRAND / rel
        if rel == "banners/hero-webtoon.png":
            cropped = resize_banner(cropped)
        save_png(out, cropped)
        outputs[rel] = cropped
        print(f"  {rel}: {cropped.size}")

    logo_full = outputs["logos/logo-full.png"]
    save_png(BRAND / "logos" / "logo-email.png", make_logo_email(logo_full))

    fav64 = outputs["favicon/favicon-64.png"]
    make_pwa_icons(fav64)

    f16 = outputs["favicon/favicon-16.png"]
    f32 = outputs["favicon/favicon-32.png"]
    make_favicon_ico(f16, f32)

    print("Done.")


if __name__ == "__main__":
    main()
