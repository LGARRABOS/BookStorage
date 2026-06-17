#!/usr/bin/env python3
"""Prépare favicon/PWA/email à partir des PNG propres dans static/brand/."""

from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
BRAND = ROOT / "static" / "brand"

LOGO = BRAND / "logos" / "logo.png"
BANNER = BRAND / "banners" / "banners.png"
PWA_APP_ICON = BRAND / "pwa" / "app-icon.png"
FAVICON_SOURCES = (
    BRAND / "favicon" / "favicon.png",
    BRAND / "favicon" / "icon.png",
)
LOGO_BOOK_BOX = (285, 125, 395, 245)
BANNER_MAX_WIDTH = 1920


def load_favicon_source() -> Image.Image:
    if PWA_APP_ICON.exists():
        print("  favicon source: pwa/app-icon.png")
        return Image.open(PWA_APP_ICON).convert("RGBA")
    for path in FAVICON_SOURCES:
        if path.exists():
            print(f"  favicon source: {path.name}")
            return Image.open(path).convert("RGBA")
    if not LOGO.exists():
        raise FileNotFoundError(f"Missing {LOGO}")
    print("  favicon source: book crop from logo.png")
    return Image.open(LOGO).convert("RGBA").crop(LOGO_BOOK_BOX)


def load_pwa_source() -> Image.Image:
    if PWA_APP_ICON.exists():
        print("  pwa source: app-icon.png")
        return Image.open(PWA_APP_ICON).convert("RGBA")
    return load_favicon_source()


def square_icon(img: Image.Image, size: int) -> Image.Image:
    w, h = img.size
    side = max(w, h)
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.paste(img, ((side - w) // 2, (side - h) // 2))
    return canvas.resize((size, size), Image.Resampling.LANCZOS)


def save_png(path: Path, img: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, format="PNG", optimize=True)


def generate_pwa_icons() -> None:
    src = load_pwa_source()
    for size in (192, 512):
        rel = f"pwa/icon-{size}.png"
        save_png(BRAND / rel, square_icon(src, size))
        print(f"  {rel}: {size}x{size}")


def generate_favicons() -> None:
    src = load_favicon_source()
    sizes = {
        "favicon/favicon-16.png": 16,
        "favicon/favicon-32.png": 32,
        "favicon/favicon-64.png": 64,
    }
    icons: dict[int, Image.Image] = {}
    for rel, size in sizes.items():
        icon = square_icon(src, size)
        save_png(BRAND / rel, icon)
        icons[size] = icon
        print(f"  {rel}: {size}x{size}")

    f32 = icons[32]
    f16 = icons[16]
    f32.save(
        BRAND / "favicon" / "favicon.ico",
        format="ICO",
        sizes=[(32, 32), (16, 16)],
        append_images=[f16],
    )
    print("  favicon/favicon.ico")


def generate_logo_email() -> None:
    if not LOGO.exists():
        return
    logo = Image.open(LOGO).convert("RGBA")
    save_png(BRAND / "logos" / "logo-email.png", logo.resize((96, 96), Image.Resampling.LANCZOS))
    print("  logos/logo-email.png: 96x96")


def optimize_banner() -> None:
    if not BANNER.exists():
        return
    banner = Image.open(BANNER).convert("RGBA")
    w, h = banner.size
    if w > BANNER_MAX_WIDTH:
        nh = int(h * BANNER_MAX_WIDTH / w)
        banner = banner.resize((BANNER_MAX_WIDTH, nh), Image.Resampling.LANCZOS)
        print(f"  banners/banners.png: resized {w}x{h} -> {banner.size[0]}x{banner.size[1]}")
    save_png(BANNER, banner)


def main() -> None:
    print("Preparing brand derivatives...")
    generate_pwa_icons()
    generate_favicons()
    generate_logo_email()
    optimize_banner()
    print("Done.")


if __name__ == "__main__":
    main()
