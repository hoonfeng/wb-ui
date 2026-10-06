#!/usr/bin/env python3
"""inkstat —— PNG 墨迹行分析：把「哪一行有文字、占多少像素」变成数字。

用法：
    python dev/tools/inkstat.py IMG.png [--band y0 y1] [--min-ink 3] [--json]

判据：像素与背景差异超过 --tol（默认 24）即算墨迹。默认背景取左上角像素。
输出：每个「连续有墨迹的行带」的 y 区间、墨迹像素总数、左右边界。
"""
import argparse
import json
import sys
from PIL import Image


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("image")
    ap.add_argument("--tol", type=int, default=24, help="与背景的差异阈值")
    ap.add_argument("--min-ink", type=int, default=1, help="行墨迹少于该值视为空行")
    ap.add_argument("--band", type=int, nargs=2, default=None, metavar=("Y0", "Y1"))
    ap.add_argument("--bg", default=None, help="背景色 r,g,b（默认取左上角）")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    im = Image.open(args.image).convert("RGB")
    w, h = im.size
    px = im.load()

    if args.bg:
        br, bg_, bb = (int(v) for v in args.bg.split(","))
    else:
        br, bg_, bb = px[0, 0]

    y0, y1 = 0, h - 1
    if args.band:
        y0, y1 = max(0, args.band[0]), min(h - 1, args.band[1])

    row_ink = []
    row_bounds = []
    for y in range(y0, y1 + 1):
        n = 0
        lo, hi = -1, -1
        for x in range(w):
            r, g, b = px[x, y]
            if abs(r - br) > args.tol or abs(g - bg_) > args.tol or abs(b - bb) > args.tol:
                n += 1
                if lo < 0:
                    lo = x
                hi = x
        row_ink.append(n)
        row_bounds.append((lo, hi))

    bands = []
    cur = None
    for i, n in enumerate(row_ink):
        y = y0 + i
        if n >= args.min_ink:
            if cur is None:
                cur = {"y0": y, "y1": y, "ink": 0, "x0": w, "x1": -1}
            cur["y1"] = y
            cur["ink"] += n
            lo, hi = row_bounds[i]
            if lo >= 0:
                cur["x0"] = min(cur["x0"], lo)
                cur["x1"] = max(cur["x1"], hi)
        else:
            if cur is not None:
                bands.append(cur)
                cur = None
    if cur is not None:
        bands.append(cur)

    out = {
        "image": args.image,
        "size": [w, h],
        "bg": [br, bg_, bb],
        "band": [y0, y1],
        "total_ink": sum(row_ink),
        "bands": bands,
    }
    if args.json:
        print(json.dumps(out, ensure_ascii=False, indent=2))
    else:
        print(f"{args.image} {w}x{h} bg=#{br:02x}{bg_:02x}{bb:02x} total_ink={sum(row_ink)}")
        for b in bands:
            print(f"  y {b['y0']:4d}-{b['y1']:4d} (h={b['y1']-b['y0']+1:3d})  ink={b['ink']:6d}  x {b['x0']}-{b['x1']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
