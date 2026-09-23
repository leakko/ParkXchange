"""Regenerate ParkXchange app icons from brand emblem for Expo + Android mipmaps."""

from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[2]
mobile = ROOT / "apps/mobile/assets/images"
store = ROOT / "docs/store"
android_res = ROOT / "apps/mobile/android/app/src/main/res"
web_logo = ROOT / "apps/web/assets/logo.png"

bg_color = (11, 31, 51)  # #0B1F33


def extract_circle_from_light_bg(src: Image.Image) -> Image.Image:
    """Crop circular emblem from a light-background square and return transparent RGBA."""
    src = src.convert("RGBA")
    w, h = src.size
    px = src.load()
    minx, miny, maxx, maxy = w, h, 0, 0
    for y in range(h):
        for x in range(w):
            r, g, b, _a = px[x, y]
            if r < 220 or g < 230 or b < 245:
                minx, miny = min(minx, x), min(miny, y)
                maxx, maxy = max(maxx, x), max(maxy, y)
    if maxx <= minx:
        return src
    side = max(maxx - minx, maxy - miny) + 8
    cx, cy = (minx + maxx) // 2, (miny + maxy) // 2
    half = side // 2
    x0, y0 = max(0, cx - half), max(0, cy - half)
    crop = src.crop((x0, y0, min(w, x0 + side), min(h, y0 + side)))
    crop = crop.resize((1024, 1024), Image.Resampling.LANCZOS)
    mask = Image.new("L", (1024, 1024), 0)
    ImageDraw.Draw(mask).ellipse((0, 0, 1023, 1023), fill=255)
    mask = mask.filter(ImageFilter.GaussianBlur(0.5))
    out = Image.new("RGBA", (1024, 1024), (0, 0, 0, 0))
    out.paste(crop, (0, 0))
    r, g, b, a = out.split()
    a = Image.composite(a, Image.new("L", (1024, 1024), 0), mask)
    return Image.merge("RGBA", (r, g, b, a))


def to_monochrome(src: Image.Image) -> Image.Image:
    src = src.convert("RGBA")
    w, h = src.size
    out = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    sp, op = src.load(), out.load()
    for y in range(h):
        for x in range(w):
            r, g, b, a = sp[x, y]
            if a < 20:
                continue
            op[x, y] = (255, 255, 255, a)
    return out


def save_webp(img: Image.Image, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, "WEBP", quality=90)


def main() -> None:
    play = Image.open(store / "play-icon-512.png").convert("RGBA")
    fg_existing = Image.open(mobile / "android-icon-foreground.png").convert("RGBA")

    # Prefer user's fixed play icon (light bg) as master emblem.
    if sum(play.getpixel((0, 0))[:3]) > 600:
        emblem = extract_circle_from_light_bg(play)
        print("Emblem from play-icon-512.png")
        play_out = Image.new("RGB", (512, 512), (236, 246, 255))
        e512 = emblem.resize((480, 480), Image.Resampling.LANCZOS)
        play_out.paste(e512, (16, 16), e512)
        play_out.save(store / "play-icon-512.png", "PNG", optimize=True)
    elif fg_existing.getpixel((0, 0))[3] < 10:
        # Upscale existing transparent foreground content if it's the full emblem
        emblem = fg_existing
        # If safe-zone padded, extract by bbox of opaque pixels
        px = emblem.load()
        w, h = emblem.size
        minx, miny, maxx, maxy = w, h, 0, 0
        for y in range(0, h, 2):
            for x in range(0, w, 2):
                if px[x, y][3] > 20:
                    minx, miny = min(minx, x), min(miny, y)
                    maxx, maxy = max(maxx, x), max(maxy, y)
        side = max(maxx - minx, maxy - miny)
        cx, cy = (minx + maxx) // 2, (miny + maxy) // 2
        half = side // 2
        crop = emblem.crop((cx - half, cy - half, cx - half + side, cy - half + side))
        emblem = crop.resize((1024, 1024), Image.Resampling.LANCZOS)
        print("Emblem from android-icon-foreground.png")
    else:
        emblem = Image.open(mobile / "icon.png").convert("RGBA")
        print("Fallback emblem from icon.png")

    # icon.png — navy full bleed
    icon_out = Image.new("RGBA", (1024, 1024), (*bg_color, 255))
    e = emblem.resize((944, 944), Image.Resampling.LANCZOS)
    icon_out.paste(e, (40, 40), e)
    icon_out.save(mobile / "icon.png", "PNG", optimize=True)

    # adaptive foreground — ~72% safe zone
    afg = Image.new("RGBA", (1024, 1024), (0, 0, 0, 0))
    e_safe = emblem.resize((736, 736), Image.Resampling.LANCZOS)
    ox = (1024 - 736) // 2
    afg.paste(e_safe, (ox, ox), e_safe)
    afg.save(mobile / "android-icon-foreground.png", "PNG", optimize=True)

    Image.new("RGB", (1024, 1024), bg_color).save(
        mobile / "android-icon-background.png", "PNG"
    )

    mono_full = to_monochrome(emblem)
    mono = Image.new("RGBA", (1024, 1024), (0, 0, 0, 0))
    m_safe = mono_full.resize((736, 736), Image.Resampling.LANCZOS)
    mono.paste(m_safe, (ox, ox), m_safe)
    mono.save(mobile / "android-icon-monochrome.png", "PNG", optimize=True)

    # splash — transparent outside circle
    emblem.resize((512, 512), Image.Resampling.LANCZOS).save(
        mobile / "splash-icon.png", "PNG", optimize=True
    )

    # favicon
    fav = Image.new("RGBA", (48, 48), (*bg_color, 255))
    e48 = emblem.resize((48, 48), Image.Resampling.LANCZOS)
    fav.paste(e48, (0, 0), e48)
    fav.save(mobile / "favicon.png", "PNG", optimize=True)

    icon_out.resize((512, 512), Image.Resampling.LANCZOS).save(web_logo, "PNG", optimize=True)

    # Android mipmaps
    legacy = {
        "mipmap-mdpi": 48,
        "mipmap-hdpi": 72,
        "mipmap-xhdpi": 96,
        "mipmap-xxhdpi": 144,
        "mipmap-xxxhdpi": 192,
    }
    adaptive = {
        "mipmap-mdpi": 108,
        "mipmap-hdpi": 162,
        "mipmap-xhdpi": 216,
        "mipmap-xxhdpi": 324,
        "mipmap-xxxhdpi": 432,
    }

    for folder, size in legacy.items():
        d = android_res / folder
        c = icon_out.resize((size, size), Image.Resampling.LANCZOS)
        save_webp(c, d / "ic_launcher.webp")
        save_webp(c, d / "ic_launcher_round.webp")

    for folder, size in adaptive.items():
        d = android_res / folder
        save_webp(afg.resize((size, size), Image.Resampling.LANCZOS), d / "ic_launcher_foreground.webp")
        save_webp(Image.new("RGB", (size, size), bg_color), d / "ic_launcher_background.webp")
        save_webp(mono.resize((size, size), Image.Resampling.LANCZOS), d / "ic_launcher_monochrome.webp")

    anydpi = android_res / "mipmap-anydpi-v26"
    anydpi.mkdir(parents=True, exist_ok=True)
    xml = (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">\n'
        '    <background android:drawable="@mipmap/ic_launcher_background"/>\n'
        '    <foreground android:drawable="@mipmap/ic_launcher_foreground"/>\n'
        '    <monochrome android:drawable="@mipmap/ic_launcher_monochrome"/>\n'
        "</adaptive-icon>\n"
    )
    (anydpi / "ic_launcher.xml").write_text(xml, encoding="utf-8")
    (anydpi / "ic_launcher_round.xml").write_text(xml, encoding="utf-8")

    print("Done. Regenerated Expo assets + Android mipmaps.")
    for p in [
        mobile / "icon.png",
        mobile / "android-icon-foreground.png",
        mobile / "splash-icon.png",
        store / "play-icon-512.png",
    ]:
        im = Image.open(p)
        print(f"  {p.name}: {im.size} {im.mode}")


if __name__ == "__main__":
    main()
