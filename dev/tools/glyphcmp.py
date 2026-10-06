#!/usr/bin/env python3
"""glyphcmp —— 字形几何 + 二值轮廓对比（D7 默认族像素级验收）。

区分两类像素差异，避免把「渲染技术固有差异」误判成「字体族错误」：

  1. **字形/位置差异（真缺陷）**：二值化（阈值化）后轮廓仍不重合 → 字体族、
     字形或基线位置不对。
  2. **抗锯齿灰度差异（固有）**：二值轮廓高度重合，仅边缘像素灰度不同
     （浏览器用 LCD 子像素抗锯齿，引擎用灰度/其它引擎）→ 与字体族无关。

用法：
    python dev/tools/glyphcmp.py A.png B.png [--line-h 32] [--rows 3]
                                 [--thr 200] [--tol 12] [--zone-out out.png]
"""
import argparse
from PIL import Image, ImageChops


def binary(img: Image.Image, thr: int) -> Image.Image:
    """阈值化为二值图（L 模式，255=墨迹/暗像素）。"""
    return img.convert("L").point(lambda v: 255 if v < thr else 0)


def raw_diff_count(a: Image.Image, b: Image.Image, tol: int) -> int:
    """与 pixdiff.py 同口径的差异像素数（tol 单通道阈值）。"""
    diff = ImageChops.difference(a.convert("RGB"), b.convert("RGB"))
    return sum(1 for r, g, bl in diff.getdata() if max(r, g, bl) > tol)


def bin_diff_count(ba: Image.Image, bb: Image.Image) -> int:
    """二值轮廓差异像素数（异或）。"""
    d = ImageChops.difference(ba, bb)
    return sum(1 for p in d.getdata() if p)


def ink_bbox(ba: Image.Image, y0: int, y1: int):
    """行内墨迹包围盒 (x0, y0, x1, y1)，无墨迹返回 None。"""
    return ba.crop((0, y0, ba.width, min(y1, ba.height))).getbbox()


def shift(img: Image.Image, dx: int, dy: int, fill: int = 255) -> Image.Image:
    """内容平移 (dx, dy)，露出区域填 fill（白）。"""
    w, h = img.size
    out = Image.new(img.mode, (w, h), fill)
    src = img.crop((max(0, -dx), max(0, -dy), w - max(0, dx), h - max(0, dy)))
    out.paste(src, (max(0, dx), max(0, dy)))
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("a")
    ap.add_argument("b")
    ap.add_argument("--line-h", type=int, default=32)
    ap.add_argument("--rows", type=int, default=3)
    ap.add_argument("--thr", type=int, default=200, help="墨迹阈值（灰度 < thr 视为墨迹）")
    ap.add_argument("--tol", type=int, default=12)
    ap.add_argument("--zone-out", default=None, help="放大并排对比图输出路径")
    ap.add_argument("--zoom", type=int, default=4)
    args = ap.parse_args()

    ia = Image.open(args.a).convert("RGB")
    ib = Image.open(args.b).convert("RGB")
    if ia.size != ib.size:
        print(f"SIZE-MISMATCH {ia.size} vs {ib.size}")
        return 2
    w, h = ia.size
    total = w * h
    ba, bb = binary(ia, args.thr), binary(ib, args.thr)

    print(f"A={args.a}\nB={args.b}  尺寸 {w}x{h}")
    print(f"整页原始差异（tol={args.tol}）: {raw_diff_count(ia, ib, args.tol)}/{total} "
          f"({raw_diff_count(ia, ib, args.tol) / total * 100:.2f}%)")
    bd0 = bin_diff_count(ba, bb)
    print(f"整页二值轮廓差异（thr={args.thr}）: {bd0}/{total} ({bd0 / total * 100:.2f}%)")

    # 位移搜索：找出让二值轮廓差异最小的整体平移（诊断基线/子像素偏移）
    best = (bd0, 0, 0)
    for dy in range(-3, 4):
        for dx in range(-3, 4):
            if dx == 0 and dy == 0:
                continue
            n = bin_diff_count(ba, shift(bb, dx, dy))
            if n < best[0]:
                best = (n, dx, dy)
    if best[1] or best[2]:
        print(f"最佳整体平移: B 平移 (dx={best[1]}, dy={best[2]}) → 二值差异 "
              f"{best[0]}/{total} ({best[0] / total * 100:.2f}%)")
    else:
        print("最佳整体平移: (0,0) —— 无需平移即已最优（无整体基线偏移）")

    print("\n逐行（行高 %d）：" % args.line_h)
    print("  行  y 范围     原始差异(占比)   二值差异(占比)   A墨迹bbox            B墨迹bbox            bbox宽差")
    for r in range(args.rows):
        y0, y1 = r * args.line_h, (r + 1) * args.line_h
        sa = ia.crop((0, y0, w, y1))
        sb = ib.crop((0, y0, w, y1))
        band = w * (y1 - y0)
        rd = raw_diff_count(sa, sb, args.tol)
        cab = ba.crop((0, y0, w, y1))
        cbb = bb.crop((0, y0, w, y1))
        bdn = bin_diff_count(cab, cbb)
        bx = ink_bbox(ba, y0, y1)
        bxb = ink_bbox(bb, y0, y1)
        wa = (bx[2] - bx[0]) if bx else 0
        wb = (bxb[2] - bxb[0]) if bxb else 0
        print(f"  {r + 1}  y{y0:>3}-{y1:<3}  {rd:>6} ({rd / band * 100:>5.1f}%)  "
              f"{bdn:>6} ({bdn / band * 100:>5.1f}%)  {str(bx):<20} {str(bxb):<20} {wb - wa:+d}")

        # 逐行垂直位移搜索：找出让「二值轮廓」差异最小的行内平移。
        # 用于区分「字形不同」与「基线位置不同」：若平移后二值差异骤降，
        # 则字形本身一致，差异来自行盒基线定位。
        row_best = None
        for dy in range(-8, 9):
            if dy == 0:
                continue
            n = bin_diff_count(cab, shift(cbb, 0, dy))
            m = raw_diff_count(sa, shift(sb, 0, dy), args.tol)
            if row_best is None or n < row_best[0]:
                row_best = (n, dy, m)
        gain = f" dy={row_best[1]:+d} → 二值 {row_best[0]} ({row_best[0] / band * 100:.1f}%), 原始 {row_best[2]} ({row_best[2] / band * 100:.1f}%)"
        print(f"       最佳行内平移: {gain}")

    if args.zone_out:
        z = args.zoom
        panels = []
        for r in range(args.rows):
            y0, y1 = r * args.line_h, (r + 1) * args.line_h
            for img, tag in ((ia, "wbui"), (ib, "edge")):
                band = img.crop((0, y0, w, y1)).resize((w * z, (y1 - y0) * z), Image.NEAREST)
                panels.append((band, tag))
        sep = 6
        out_h = sum(p.height for p, _ in panels) + sep * (len(panels) - 1)
        canvas = Image.new("RGB", (w * z, out_h), (200, 200, 255))
        y = 0
        for p, _ in panels:
            canvas.paste(p, (0, y))
            y += p.height + sep
        canvas.save(args.zone_out)
        print(f"\n放大对比图（{z}x，每组上=wbui 下=edge）: {args.zone_out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
