#!/usr/bin/env python3
"""README feature collage: the last frame of each shots/*.mp4 in a 3x3 grid with captions -> assets/features.png.
Run from assets/demo/capture after ./record.sh shots/... ; needs ffmpeg, rsvg-convert and the Maple Mono NF CN font."""
import base64, os, subprocess, tempfile
from xml.sax.saxutils import escape

TILES = [  # grid order: (tape name, caption)
    ("where", "WHERE with completion and history"),
    ("order", "Sort by many columns"),
    ("complete", "Completion and field hints"),
    ("panes", "Panes: the same table twice"),
    ("flash", "Flash jump"),
    ("json", "JSON view"),
    ("edit", "Edit a cell, save as one transaction"),
    ("select", "Select rows, act on them"),
    ("whichkey", "which-key"),
]
W, H, GAP, CAP = 778, 438, 16, 52  # tile (16:9 of the 1200x676 frame), gutter, caption band
OUT = "../../features.png"

tmp = tempfile.mkdtemp()
parts = []
for i, (name, cap) in enumerate(TILES):
    png = f"{tmp}/{name}.png"
    # last frame, cropped to 16:9 and scaled with lanczos so rsvg embeds it 1:1
    subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-sseof", "-0.3", "-i", f"renders/shots/{name}.mp4",
                    "-frames:v", "1", "-update", "1", "-vf", f"crop=1200:675:0:0,scale={W}:{H}:flags=lanczos", png], check=True)
    x, y = GAP + (i % 3) * (W + GAP), GAP + (i // 3) * (H + CAP + GAP)
    data = base64.b64encode(open(png, "rb").read()).decode()
    parts.append(f'<image x="{x}" y="{y}" width="{W}" height="{H}" href="data:image/png;base64,{data}"/>')
    parts.append(f'<text x="{x}" y="{y + H + 36}">{escape(cap)}</text>')
w, h = GAP + 3 * (W + GAP), GAP + 3 * (H + CAP + GAP) - GAP
svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}">
<style>text {{ font-family: "Maple Mono NF CN"; font-size: 28px; fill: #ffffff; }}</style>
<rect width="100%" height="100%" fill="#1e1a1a"/>
{"".join(parts)}
</svg>'''
open(f"{tmp}/c.svg", "w").write(svg)
subprocess.run(["rsvg-convert", f"{tmp}/c.svg", "-o", f"{tmp}/c.png"], check=True)
# 256-colour palette: terminal frames have few colours and it keeps the PNG well under 1.5 MB
subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-i", f"{tmp}/c.png", "-vf",
                "split[a][b];[a]palettegen=max_colors=256:stats_mode=single[p];[b][p]paletteuse=dither=none",
                "-frames:v", "1", "-update", "1", OUT], check=True)
print(OUT, w, h, os.path.getsize(OUT))
