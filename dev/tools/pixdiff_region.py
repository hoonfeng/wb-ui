#!/usr/bin/env python3
"""pixdiff_region —— 两张同尺寸 PNG 的**分区域**像素差异统计。

口径与 `pixdiff.py` 完全一致：单通道最大差 `> --tol` 记为一个差异像素；
`mean_ch` 为「所有像素的最大单通道差」的均值（与 pixdiff 的 mean_ch_delta 同定义）。

用法：
    python dev/tools/pixdiff_region.py A.png B.png [--tol 12] \\
        --region 左栏:0,52,236,768 --region 舞台:237,52,1011,768 --region 右栏:1012,52,1280,768

区域格式 `名:x0,y0,x1,y1`，半开区间 `[x0,x1) × [y0,y1)`，像素坐标以图左上为原点，
超出图界自动夹取。不传 `--region` 则整页一区。

说明：区域边界应由**双端一致**的页面几何给出（`getBoundingClientRect()` 自上报），
否则「分区本身有偏差」会被算成区域差异。
"""
import argparse
import sys

from PIL import Image


def parse_region(spec: str, w: int, h: int):
    name, coords = spec.split(":", 1)
    vals = [int(v) for v in coords.split(",")]
    if len(vals) != 4:
        raise SystemExit(f"区域格式应为 名:x0,y0,x1,y1，收到 {spec!r}")
    x0, y0, x1, y1 = vals
    x0, y0 = max(0, min(x0, w)), max(0, min(y0, h))
    x1, y1 = max(0, min(x1, w)), max(0, min(y1, h))
    if x1 <= x0 or y1 <= y0:
        raise SystemExit(f"区域 {name!r} 为空：{x0},{y0},{x1},{y1}")
    return name, x0, y0, x1, y1


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("a", help="A 图（wb-ui 渲染）")
    ap.add_argument("b", help="B 图（浏览器渲染）")
    ap.add_argument("--tol", type=int, default=12, help="单通道差异超过该值算「不同」")
    ap.add_argument("--region", action="append", default=[], help="名:x0,y0,x1,y1（可多次）")
    args = ap.parse_args()

    ia = Image.open(args.a).convert("RGB")
    ib = Image.open(args.b).convert("RGB")
    if ia.size != ib.size:
        print(f"SIZE-MISMATCH {args.a}={ia.size} vs {args.b}={ib.size}")
        return 2
    w, h = ia.size
    if not args.region:
        args.region = [f"整页:0,0,{w},{h}"]

    regions = [parse_region(s, w, h) for s in args.region]
    pa, pb = ia.load(), ib.load()

    print(f"{args.a} vs {args.b}: {w}x{h}  tol={args.tol}")
    print(f"  {'区域':<8} {'范围':<26} {'尺寸':<12} {'差异像素':<22} {'差异率':<8} {'平均通道差'}")
    for name, x0, y0, x1, y1 in regions:
        diff_px = 0
        sum_ch = 0
        max_ch = 0
        for y in range(y0, y1):
            for x in range(x0, x1):
                r1, g1, b1 = pa[x, y]
                r2, g2, b2 = pb[x, y]
                d = max(abs(r1 - r2), abs(g1 - g2), abs(b1 - b2))
                sum_ch += d
                if d > max_ch:
                    max_ch = d
                if d > args.tol:
                    diff_px += 1
        total = (x1 - x0) * (y1 - y0)
        pct = diff_px * 100.0 / total if total else 0.0
        print(f"  {name:<8} x[{x0},{x1}) y[{y0},{y1})  {x1 - x0}x{y1 - y0:<6} "
              f"{diff_px}/{total} ({pct:.2f}%)   {pct:6.2f}%   mean_ch={sum_ch / total:.2f}  max_ch={max_ch}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
