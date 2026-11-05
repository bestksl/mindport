#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""MindPort brand icon generator.

Night-harbour rounded square with a lighthouse translated from MindPort's
pixel mark: brick tower with cream stripes, warm lamp, wooden base and a
teal sea. The geometry mirrors BrandGlyph's 24-unit SVG so favicon and UI
mark read as the same shape. Tiny sizes drop beams and merge the lamp room
into one amber block so the mark stays legible at 16px.

Outputs (overwritten in place):
  apps/web/public/favicon.ico, favicon-16/32.png, apple-touch-icon.png
  assets/prod/t3-black-web-favicon.ico, t3-black-web-favicon-16/32.png,
  t3-black-web-apple-touch-180.png, t3-black-windows.ico

Run from the repo root with the system Python 3 + Pillow:
  python3 scripts/gen-brand-icons.py [repo-root]

The ICO container is assembled manually because Pillow 11.3's ICO writer
silently drops append_images frames and otherwise downscales a single base
image itself (blurring the mark). PNG-in-ICO entries are supported by every
browser and by Windows Vista+, so each embedded size keeps its native render.
"""
import io
import math
import struct
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

TOP = (43, 74, 85)         # #2b4a55 night-sea top
BOTTOM = (18, 31, 37)      # #121f25 night-sea bottom
TOWER = (192, 106, 69)     # #c06a45 brick red
TOWER_DARK = (156, 70, 50)  # #9c4632 cap
STRIPE = (240, 228, 205)   # #f0e4cd cream
FRAME = (74, 53, 32)       # #4a3520 dark wood
LAMP = (255, 214, 104)     # #ffd668 warm amber
BASE = (107, 79, 48)       # #6b4f30 wooden base
SEA = (58, 90, 99)         # #3a5a63
SEA_DEEP = (36, 57, 68)    # #243944
RADIUS_RATIO = 0.224
ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]


def vertical_gradient(size):
    grad = Image.new("RGB", (1, size))
    px = grad.load()
    for y in range(size):
        t = y / max(1, size - 1)
        c = tuple(round(TOP[i] + (BOTTOM[i] - TOP[i]) * t) for i in range(3))
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
            fill=60,
        )
        gloss = gloss.filter(ImageFilter.GaussianBlur(size * 0.06))
        white = Image.new("RGBA", (size, size), (255, 255, 255, 0))
        white.putalpha(gloss)
        base = Image.alpha_composite(base, white)

    layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ld = ImageDraw.Draw(layer)
    s = size / 24
    detailed = size >= 32
    beams = size >= 64
    tiny = size < 32

    def box(x0, y0, x1, y1):
        return [x0 * s, y0 * s, x1 * s, y1 * s]

    def poly(points, fill):
        ld.polygon([(x * s, y * s) for x, y in points], fill=fill)

    # sea: wave band at full detail, flat strips where pixels cannot carry the curve
    if tiny:
        ld.rectangle(box(0, 20.2, 24, 24), fill=SEA)
        ld.rectangle(box(0, 22.6, 24, 24), fill=SEA_DEEP)
    else:
        wave = [(0, 24), (0, 21.4)]
        for step in range(49):
            x = step * 0.5
            wave.append((x, 21.0 + 0.45 * math.sin(2 * math.pi * x / 6)))
        wave.append((24, 24))
        poly(wave, SEA)
        ld.rectangle(box(0, 22.7, 24, 24), fill=SEA_DEEP)

    # wooden base
    poly([(7.6, 18), (16.4, 18), (17.3, 20.3), (6.7, 20.3)], BASE)

    # tower, narrowing toward the lamp room
    poly([(10.7, 6.4), (13.3, 6.4), (15.0, 18), (9.0, 18)], TOWER)

    # cream stripes: two at full detail, one centred band at tiny sizes
    if detailed:
        ld.rectangle(box(9.9, 10.55, 14.1, 11.25), fill=STRIPE)
        ld.rectangle(box(9.4, 14.35, 14.6, 15.05), fill=STRIPE)
    else:
        ld.rectangle(box(9.5, 12.2, 14.5, 13.0), fill=STRIPE)

    # gallery platform under the lamp room; skipped below 48px where the
    # overhang reads as white ears beside the lamp
    if size >= 48:
        ld.line([(9.5 * s, 6.4 * s), (14.5 * s, 6.4 * s)], fill=STRIPE, width=max(1, round(0.45 * s)))

    # lamp room: framed block with an amber lamp (solid amber at tiny sizes)
    if tiny:
        ld.rounded_rectangle(box(10.3, 3.6, 13.7, 5.6), radius=max(1, round(0.5 * s)), fill=LAMP)
    else:
        ld.rounded_rectangle(box(10.1, 3.4, 13.9, 5.9), radius=max(1, round(0.7 * s)), fill=FRAME)
        lamp_r = 0.7 * s
        cx, cy = 12 * s, 4.65 * s
        ld.ellipse([cx - lamp_r, cy - lamp_r, cx + lamp_r, cy + lamp_r], fill=LAMP)

    # cap
    ld.line(
        [(10.6 * s, 2.85 * s), (13.4 * s, 2.85 * s)],
        fill=TOWER_DARK,
        width=max(1, round(0.7 * s)),
    )

    # light beams, only where there are pixels to spare
    if beams:
        beam = (*STRIPE, 150)
        ld.line([(9.4 * s, 4.6 * s), (7.4 * s, 3.5 * s)], fill=beam, width=max(1, round(0.5 * s)))
        ld.line([(14.6 * s, 4.6 * s), (16.6 * s, 3.5 * s)], fill=beam, width=max(1, round(0.5 * s)))

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
