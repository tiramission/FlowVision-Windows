#!/usr/bin/env python3
"""Generate a reproducible thumbnail benchmark corpus for FlowVision.

The corpus is deliberately local and gitignored. It is not needed to build or
run the app; it exists so performance work can be measured against the same
files on every run.

Requirements: Pillow, numpy, opencv-python, piexif. pillow-heif is optional and
is used only when generating HEIC samples.

Usage:
    python tools/make_thumb_corpus.py --quick
    python tools/make_thumb_corpus.py --full
    python tools/make_thumb_corpus.py --full --out D:\thumb-corpus
"""
from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
import random
import shutil
import sys
import time
from pathlib import Path

import cv2
import numpy as np
import piexif
from PIL import Image, ImageDraw, ImageFont

try:
    import pillow_heif

    pillow_heif.register_heif_opener()
except Exception:  # optional
    pillow_heif = None


JPEG_QUALITY = 92
MASTER_W, MASTER_H = 6000, 4000


def log(msg: str) -> None:
    print(msg, flush=True)


def make_scene(seed: int, w: int, h: int) -> Image.Image:
    """A photo-ish, high-entropy scene: gradients, texture, shapes, text.

    Pure random noise makes JPEG files unrealistically large; pure gradients
    make them unrealistically small. This keeps 24 MP files in the 8-20 MB
    range while still exercising a real JPEG decoder.
    """
    rng = np.random.default_rng(seed)
    x = np.linspace(0.0, 1.0, w, dtype=np.float32)[None, :, None]
    y = np.linspace(0.0, 1.0, h, dtype=np.float32)[:, None, None]
    phase = rng.uniform(0.0, 2.0 * np.pi, 3).astype(np.float32)

    # Low-frequency texture at 1/64 resolution, then cubic upsample. Add a
    # couple of finer octaves so the 24 MP JPEG has photo-like detail instead
    # of compressing down to a few hundred kilobytes.
    sw, sh = max(2, w // 64), max(2, h // 64)
    low = rng.normal(0.0, 1.0, (sh, sw)).astype(np.float32)
    low = cv2.GaussianBlur(low, (0, 0), 1.2)
    texture = cv2.resize(low, (w, h), interpolation=cv2.INTER_CUBIC)
    texture = texture / (float(texture.std()) + 1e-6)
    for div, amp in ((16, 0.12), (8, 0.09), (4, 0.06)):
        nw, nh = max(2, w // div), max(2, h // div)
        fine = rng.normal(0.0, 1.0, (nh, nw)).astype(np.float32)
        fine = cv2.GaussianBlur(fine, (0, 0), 0.8)
        fine = cv2.resize(fine, (w, h), interpolation=cv2.INTER_CUBIC)
        fine = fine / (float(fine.std()) + 1e-6)
        texture += fine * amp

    img = np.empty((h, w, 3), dtype=np.uint8)
    for c in range(3):
        base = (
            0.20
            + 0.32 * x
            + 0.24 * y
            + 0.16 * np.sin((x * (2.5 + c * 0.3) + y * (1.7 - c * 0.2)) * np.pi + phase[c])
            + 0.10 * np.cos((x * 1.3 - y * 2.1) * np.pi + phase[(c + 1) % 3])
        )
        base = base + texture[..., None] * 0.42
        base = base[..., 0]
        base = base + rng.normal(0.0, 0.135, (h, w)).astype(np.float32)
        img[..., c] = np.clip(base * 255.0, 0, 255).astype(np.uint8)

    # Hard edges: rectangles, circles and diagonal lines. These keep the
    # thumbnail quality checks honest and stop the image from being a blur.
    for i in range(18):
        color = tuple(int(v) for v in rng.integers(10, 246, 3))
        x0, y0 = int(rng.integers(0, w)), int(rng.integers(0, h))
        x1, y1 = int(rng.integers(x0, w)), int(rng.integers(y0, h))
        cv2.rectangle(img, (x0, y0), (x1, y1), color, thickness=-1)
        cv2.rectangle(img, (x0, y0), (x1, y1), (245, 245, 245), thickness=2)
    for i in range(12):
        color = tuple(int(v) for v in rng.integers(0, 256, 3))
        cx, cy = int(rng.integers(0, w)), int(rng.integers(0, h))
        r = int(rng.integers(min(w, h) // 40, min(w, h) // 8))
        cv2.circle(img, (cx, cy), r, color, thickness=-1)
    for i in range(24):
        color = tuple(int(v) for v in rng.integers(0, 256, 3))
        x0, y0 = int(rng.integers(0, w)), int(rng.integers(0, h))
        x1, y1 = int(rng.integers(0, w)), int(rng.integers(0, h))
        cv2.line(img, (x0, y0), (x1, y1), color, thickness=int(rng.integers(1, 5)))

    pil = Image.fromarray(cv2.cvtColor(img, cv2.COLOR_BGR2RGB))
    draw = ImageDraw.Draw(pil)
    try:
        font = ImageFont.truetype("arial.ttf", max(18, min(w, h) // 24))
    except Exception:
        font = ImageFont.load_default()
    for i in range(6):
        tx = int(rng.integers(20, max(21, w - w // 3)))
        ty = int(rng.integers(20, max(21, h - h // 8)))
        text = f"FlowVision 24MP {seed:02d} / {i}"
        draw.text((tx, ty), text, fill=(250, 250, 250), stroke_width=2, stroke_fill=(20, 20, 20), font=font)
    return pil


def save_jpeg(img: Image.Image, path: Path, quality: int = JPEG_QUALITY, progressive: bool = False) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, "JPEG", quality=quality, subsampling=2, progressive=progressive, optimize=False)


def add_exif(path: Path, orientation: int = 1, embedded: int = 0, seed: int = 0) -> None:
    """Write IFD0 orientation and optionally an IFD1 JPEG thumbnail."""
    exif = {
        "0th": {
            piexif.ImageIFD.Make: b"FlowVision",
            piexif.ImageIFD.Model: b"ThumbCorpus",
            piexif.ImageIFD.Software: b"FlowVision corpus generator",
            piexif.ImageIFD.Orientation: int(orientation),
            piexif.ImageIFD.ImageDescription: f"seed={seed};embedded={embedded}".encode("ascii"),
        },
        "Exif": {
            piexif.ExifIFD.DateTimeOriginal: b"2026:11:26 12:00:00",
            piexif.ExifIFD.UserComment: b"FlowVision thumbnail benchmark",
        },
        "GPS": {},
        "1st": {},
        "thumbnail": None,
    }
    if embedded:
        with Image.open(path) as im:
            thumb = im.copy()
            thumb.thumbnail((embedded, embedded), Image.Resampling.LANCZOS)
            buf = io.BytesIO()
            thumb.save(buf, "JPEG", quality=82, subsampling=2)
            exif["thumbnail"] = buf.getvalue()
            exif["1st"][piexif.ImageIFD.Compression] = 6
            exif["1st"][piexif.ImageIFD.XResolution] = (72, 1)
            exif["1st"][piexif.ImageIFD.YResolution] = (72, 1)
            exif["1st"][piexif.ImageIFD.ResolutionUnit] = 2
    piexif.insert(piexif.dump(exif), str(path))


def make_orientation_image(seed: int, w: int = 3000, h: int = 2000) -> Image.Image:
    """Four colored corners make rotation mistakes obvious in a thumbnail."""
    img = Image.new("RGB", (w, h), (32, 32, 36))
    d = ImageDraw.Draw(img)
    # Top-left red, top-right blue, bottom-left green, bottom-right yellow.
    d.rectangle((0, 0, w // 3, h // 3), fill=(235, 32, 32))
    d.rectangle((w - w // 3, 0, w, h // 3), fill=(32, 64, 235))
    d.rectangle((0, h - h // 3, w // 3, h), fill=(32, 200, 64))
    d.rectangle((w - w // 3, h - h // 3, w, h), fill=(245, 210, 32))
    d.text((w // 2 - 220, h // 2 - 40), f"ORIENT {seed}", fill=(250, 250, 250))
    return img


def make_formats(out: Path) -> list[Path]:
    made: list[Path] = []
    img = make_scene(9001, 3000, 2000)
    # Baseline and progressive JPEG.
    p = out / "formats" / "baseline.jpg"
    save_jpeg(img, p)
    made.append(p)
    p = out / "formats" / "progressive.jpg"
    save_jpeg(img, p, progressive=True)
    made.append(p)
    # CMYK JPEG.
    p = out / "formats" / "cmyk.jpg"
    img.convert("CMYK").save(p, "JPEG", quality=90)
    made.append(p)
    # Grayscale JPEG.
    p = out / "formats" / "gray.jpg"
    img.convert("L").save(p, "JPEG", quality=90)
    made.append(p)
    # RGBA PNG with a transparent hole.
    rgba = img.convert("RGBA")
    alpha = Image.new("L", rgba.size, 255)
    ad = ImageDraw.Draw(alpha)
    ad.ellipse((rgba.width // 4, rgba.height // 4, rgba.width * 3 // 4, rgba.height * 3 // 4), fill=80)
    rgba.putalpha(alpha)
    p = out / "formats" / "alpha.png"
    rgba.save(p, "PNG", optimize=False)
    made.append(p)
    # 16-bit grayscale PNG.
    p = out / "formats" / "gray16.png"
    arr = np.linspace(0, 65535, 3000, dtype=np.uint16)
    arr = np.tile(arr, (2000, 1))
    Image.fromarray(arr).save(p, "PNG")
    made.append(p)
    # GIF, WebP, TIFF, BMP, ICO.
    p = out / "formats" / "still.gif"
    img.convert("P", palette=Image.Palette.ADAPTIVE, colors=256).save(p, "GIF")
    made.append(p)
    p = out / "formats" / "still.webp"
    img.save(p, "WEBP", quality=90)
    made.append(p)
    p = out / "formats" / "still.tiff"
    img.save(p, "TIFF", compression="tiff_deflate")
    made.append(p)
    p = out / "formats" / "still.bmp"
    img.save(p, "BMP")
    made.append(p)
    p = out / "formats" / "still.ico"
    img.resize((256, 256), Image.Resampling.LANCZOS).save(p, "ICO", sizes=[(256, 256)])
    made.append(p)
    # HEIC if pillow-heif is installed.
    if pillow_heif is not None:
        p = out / "formats" / "still.heic"
        img.save(p, "HEIF", quality=90)
        made.append(p)
    # Corrupt and empty files.
    p = out / "formats" / "corrupt.jpg"
    p.write_bytes(b"\xff\xd8\xff\xe0" + b"not a real jpeg" * 80)
    made.append(p)
    p = out / "formats" / "empty.jpg"
    p.write_bytes(b"")
    made.append(p)
    return made


def make_orientation(out: Path) -> list[Path]:
    made: list[Path] = []
    base = make_orientation_image(7001)
    for o in range(1, 9):
        p = out / "orientation" / f"orient-{o}.jpg"
        p.parent.mkdir(parents=True, exist_ok=True)
        base.save(p, "JPEG", quality=90, subsampling=2)
        add_exif(p, orientation=o, embedded=0, seed=7001)
        made.append(p)
    return made


def copy_master(src: Path, dst: Path) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, dst)


def build_masters(out: Path, count: int, w: int, h: int) -> list[Path]:
    masters: list[Path] = []
    for i in range(count):
        p = out / "masters" / f"master-{i:02d}.jpg"
        if p.exists() and p.stat().st_size > 0:
            masters.append(p)
            continue
        t0 = time.time()
        img = make_scene(1000 + i, w, h)
        save_jpeg(img, p)
        log(f"master {i:02d}: {p.stat().st_size/1024/1024:.1f} MB in {time.time()-t0:.1f}s")
        masters.append(p)
    return masters


def build_dirs(out: Path, masters: list[Path]) -> dict[str, int]:
    dirs = out / "dirs"
    counts = {"24": 24, "100": 100, "300": 300}
    manifest: dict[str, int] = {}
    for name, n in counts.items():
        d = dirs / name
        d.mkdir(parents=True, exist_ok=True)
        for i in range(n):
            src = masters[i % len(masters)]
            dst = d / f"img-{i:03d}.jpg"
            if not dst.exists() or dst.stat().st_size == 0:
                copy_master(src, dst)
        manifest[name] = n
        log(f"dir {name}: {n} files, {sum(p.stat().st_size for p in d.glob('*.jpg'))/1024/1024/1024:.2f} GB")
    return manifest


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(Path("testdata") / "thumb-corpus"))
    ap.add_argument("--quick", action="store_true", help="small format/orientation set only")
    ap.add_argument("--full", action="store_true", help="24 MP masters and 24/100/300 dirs")
    ap.add_argument("--masters", type=int, default=12)
    args = ap.parse_args()

    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    log(f"corpus: {out}")

    made: list[str] = []
    made += [str(p) for p in make_orientation(out)]
    made += [str(p) for p in make_formats(out)]

    if args.full and not args.quick:
        masters = build_masters(out, args.masters, MASTER_W, MASTER_H)
        # Half of the 24-file directory gets an embedded thumbnail. This keeps
        # the EXIF fast path measurable without making every file identical.
        d24 = out / "dirs" / "24"
        d24.mkdir(parents=True, exist_ok=True)
        for i in range(24):
            src = masters[i % len(masters)]
            dst = d24 / f"img-{i:03d}.jpg"
            if not dst.exists() or dst.stat().st_size == 0:
                copy_master(src, dst)
            if i % 2 == 0:
                add_exif(dst, orientation=1, embedded=(160 if i % 4 == 0 else 320), seed=i)
        manifest = build_dirs(out, masters)
        # Also create 12 MP and 48 MP one-off masters for size coverage.
        p12 = out / "sizes" / "12mp.jpg"
        if not p12.exists() or p12.stat().st_size == 0:
            save_jpeg(make_scene(9001, 4000, 3000), p12)
            log(f"12mp: {p12.stat().st_size/1024/1024:.1f} MB")
        p48 = out / "sizes" / "48mp.jpg"
        if not p48.exists() or p48.stat().st_size == 0:
            # 8000x6000 exceeds the comfortable single-shot numpy workspace;
            # build it from four 4000x3000 tiles instead.
            tile = Image.new("RGB", (8000, 6000))
            for ty in range(2):
                for tx in range(2):
                    part = make_scene(9100 + ty * 2 + tx, 4000, 3000)
                    tile.paste(part, (tx * 4000, ty * 3000))
            save_jpeg(tile, p48)
            log(f"48mp: {p48.stat().st_size/1024/1024:.1f} MB")
    else:
        manifest = {}

    manifest_path = out / "manifest.json"
    digest = {
        "generated": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "files": [],
        "dirs": manifest,
    }
    for p in sorted(out.rglob("*")):
        if p.is_file() and p.name not in ("manifest.json", "SOURCES.md"):
            digest["files"].append({
                "path": str(p.relative_to(out)),
                "bytes": p.stat().st_size,
                "sha256": hashlib.sha256(p.read_bytes()).hexdigest()[:16],
            })
    manifest_path.write_text(json.dumps(digest, indent=2, ensure_ascii=False), encoding="utf-8")
    log(f"manifest: {manifest_path} ({len(digest['files'])} files)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
