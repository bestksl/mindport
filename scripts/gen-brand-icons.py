#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Demo零一 brand icon generator.

Blue glossy rounded square (matching the legacy icon family) with a bold
white 零 glyph. Every size is rendered natively so FreeType hinting keeps
the complex glyph legible at 16px.

Outputs (overwritten in place):
  apps/web/public/favicon.ico, favicon-16/32.png, apple-touch-icon.png
  assets/prod/t3-black-web-favicon.ico, t3-black-web-favicon-16/32.png,
  assets/prod/t3-black-web-apple-touch-180.png, t3-black-windows.ico

Run from the repo root with the system Python 3 + Pillow:
  python3 scripts/gen-brand-icons.py [repo-root]

The ICO container is assembled manually because Pillow 11.3's ICO writer
silently drops append_images frames and otherwise downscales a single base
image itself (blurring the glyph). PNG-in-ICO entries are supported by every
browser and by Windows Vista+, so each embedded size keeps its native render.
"""
import io
import os
import struct
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

FONT_PATH = "/System/Library/Fonts/Hiragino Sans GB.ttc"
FONT_INDEX = 2  # Hiragino Sans GB W6 (bold)
TOP = (166, 206, 250)      # #a6cefa
MID = (153, 183, 238)      # #99b7ee
BOTTOM = (112, 134, 207)   # deeper periwinkle
RADIUS_RATIO = 0.224
GLYPH_RATIO = 0.64
ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]


def vertical_gradient(size):
    grad = Image.new("RGB", (1, size))
    px = grad.load()
    for y in range(size):
        t = y / max(1, size - 1)
        if t < 0.55:
            k = t / 0.55
            c = tuple(round(TOP[i] + (MID[i] - TOP[i]) * k) for i in range(3))
        else:
            k = (t - 0.55) / 0.45
            c = tuple(round(MID[i] + (BOTTOM[i] - MID[i]) * k) for i in range(3))
        px[0, y] = c
    return grad.resize((size, size))


def render_icon(size, full_bleed):
    base = vertical_gradient(size).convert("RGBA")

    # soft top gloss, scaled with the icon
    if size >= 48:
        gloss = Image.new("L", (size, size), 0)
        gd = ImageDraw.Draw(gloss)
        gd.ellipse(
            [int(size * 0.12), int(-size * 0.28), int(size * 0.88), int(size * 0.42)],
            fill=70,
        )
        gloss = gloss.filter(ImageFilter.GaussianBlur(size * 0.06))
        white = Image.new("RGBA", (size, size), (255, 255, 255, 0))
        white.putalpha(gloss)
        base = Image.alpha_composite(base, white)

    # glyph, hinted natively at this size; smaller ratio at tiny sizes for breathing room
    ratio = 0.6 if size < 32 else GLYPH_RATIO
    font = ImageFont.truetype(FONT_PATH, max(8, int(size * ratio)), index=FONT_INDEX)
    layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ld = ImageDraw.Draw(layer)
    bbox = ld.textbbox((0, 0), "零", font=font)
    gw, gh = bbox[2] - bbox[0], bbox[3] - bbox[1]
    x = (size - gw) / 2 - bbox[0]
    y = (size - gh) / 2 - bbox[1] - size * 0.01
    ld.text((x, y), "零", font=font, fill=(255, 255, 255, 255))
    base = Image.alpha_composite(base, layer)

    if not full_bleed:
        mask = Image.new("L", (size, size), 0)
        ImageDraw.Draw(mask).rounded_rectangle(
            [0, 0, size - 1, size - 1], radius=max(1, round(size * RADIUS_RATIO)), fill=255
        )
        base.putalpha(mask)
    return base


def save_png(size, path, full_bleed):
    render_icon(size, full_bleed).save(path)
    print("png", path, size)


def save_ico(path, sizes):
    # See module docstring: manual PNG-in-ICO assembly preserves native frames.
    blobs = []
    for size in sizes:
        buf = io.BytesIO()
        render_icon(size, False).save(buf, format="PNG")
        blobs.append(buf.getvalue())

    header = struct.pack("<HHH", 0, 1, len(sizes))
    offset = 6 + 16 * len(sizes)
    directory = bytearray()
    payload = bytearray()
    for size, blob in zip(sizes, blobs):
        directory += struct.pack(
            "<BBBBHHII",
            0 if size >= 256 else size,
            0 if size >= 256 else size,
            0,
            0,
            1,
            32,
            len(blob),
            offset,
        )
        payload += blob
        offset += len(blob)

    with open(path, "wb") as f:
        f.write(header + directory + payload)
    print("ico", path, sizes)


def main():
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parent.parent
    web_public = root / "apps/web/public"
    assets_prod = root / "assets/prod"

    save_png(16, web_public / "favicon-16x16.png", False)
    save_png(32, web_public / "favicon-32x32.png", False)
    save_png(180, web_public / "apple-touch-icon.png", True)
    save_ico(web_public / "favicon.ico", ICO_SIZES)

    save_png(180, assets_prod / "t3-black-web-apple-touch-180.png", True)
    save_png(16, assets_prod / "t3-black-web-favicon-16x16.png", False)
    save_png(32, assets_prod / "t3-black-web-favicon-32x32.png", False)
    save_ico(assets_prod / "t3-black-web-favicon.ico", ICO_SIZES)
    save_ico(assets_prod / "t3-black-windows.ico", ICO_SIZES)


if __name__ == "__main__":
    main()
