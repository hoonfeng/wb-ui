#!/usr/bin/env python3
"""pixdiff —— 两张同尺寸 PNG 的像素级差异统计（wb-ui 渲染 vs 真实浏览器）。

用法：
    python dev/tools/pixdiff.py A.png B.png [--tol 12] [--out diff.png] [--top 8]

输出：尺寸、差异像素数与占比、最大/平均通道差、差异聚集区（8x8 网格热力图）、
以及可选的差异可视化图（红=A 独有、绿=B 独有、灰=相同内容的暗化版）。
"""
import argparse
import sys
from PIL import Image, ImageChops


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("a")
    ap.add_argument("b")
    ap.add_argument("--tol", type=int, default=12, help="单通道差异超过该值算「不同」")
    ap.add_argument("--out", default=None, help="差异可视化 PNG 输出路径")
    ap.add_argument("--top", type=int, default=6, help="列出差异最大的网格数")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    ia = Image.open(args.a).convert("RGB")
    ib = Image.open(args.b).convert("RGB")
    if ia.size != ib.size:
        print(f"SIZE-MISMATCH {args.a}={ia.size} vs {args.b}={ib.size}")
        return 2

    w, h = ia.size
    pa, pb = ia.load(), ib.load()

    diff_px = 0
    max_ch = 0
    sum_ch = 0
    grid_cols = 8
    grid_rows = 8
    gw, gh = max(1, w // grid_cols), max(1, h // grid_rows)
    grid = [[0] * grid_cols for _ in range(grid_rows)]

    # 行墨迹（用于定位「整行缺失/错位」）
    row_diff = [0] * h

    for y in range(h):
        for x in range(w):
            r1, g1, b1 = pa[x, y]
            r2, g2, b2 = pb[x, y]
            d = max(abs(r1 - r2), abs(g1 - g2), abs(b1 - b2))
            if d > args.tol:
                diff_px += 1
                row_diff[y] += 1
                grid[y // gh][x // gw] += 1
            sum_ch += d
            if d > max_ch:
                max_ch = d

    total = w * h
    pct = diff_px * 100.0 / total
    print(f"{args.a} vs {args.b}: {w}x{h}")
    print(f"  diff_px={diff_px}/{total} ({pct:.2f}%)  max_ch_delta={max_ch}  mean_ch_delta={sum_ch/total:.2f}")

    # 差异行带（连续有差异的行）
    bands = []
    cur = None
    for y, n in enumerate(row_diff):
        if n > 0:
            if cur is None:
                cur = [y, y, 0]
            cur[1] = y
            cur[2] += n
        else:
            if cur is not None:
                bands.append(cur)
                cur = None
    if cur is not None:
        bands.append(cur)
    if bands:
        top = sorted(bands, key=lambda b: -b[2])[:args.top]
        if not args.quiet:
            print("  差异行带（y0-y1, 差异像素）:")
            for b in sorted(top):
                print(f"    y {b[0]:4d}-{b[1]:4d}  {b[2]}")

    if not args.quiet:
        print("  差异热力网格 (8x8, 数字=该格差异像素数/100):")
        for row in grid:
            print("    " + " ".join(f"{v//100:4d}" for v in row))

    if args.out:
        vis = Image.new("RGB", (w, h), (0, 0, 0))
        va = vis.load()
        for y in range(h):
            for x in range(w):
                r1, g1, b1 = pa[x, y]
                r2, g2, b2 = pb[x, y]
                d = max(abs(r1 - r2), abs(g1 - g2), abs(b1 - b2))
                if d > args.tol:
                    # 哪边更亮 → 哪边的颜色
                    la, lb = r1 + g1 + b1, r2 + g2 + b2
                    va[x, y] = (255, 40, 40) if la >= lb else (40, 255, 40)
                else:
                    gray = (r1 + g1 + b1) // 6
                    va[x, y] = (gray, gray, gray)
        vis.save(args.out)
        print(f"  差异图 → {args.out}（红=wb-ui 更亮，绿=浏览器更亮，灰=一致）")
    return 0 if diff_px == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
