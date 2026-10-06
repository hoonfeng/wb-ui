#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""媒体格式验证 · 样本生成脚本（入库物，见 docs/media-format-verification-plan.md §6.2 / §8.2 阶段 0）。

用法：
    python dev/media/gen_samples.py                 # 输出到 dev/media/samples
    python dev/media/gen_samples.py --out <dir>     # 指定输出目录
    python dev/media/gen_samples.py --ffmpeg <exe>  # 指定 ffmpeg（默认取 PATH）

产出：
    <out>/…                     各格式样本（**不入库**，见 .gitignore）
    <out>/manifest.json         样本清单（探针据此构建矩阵页与像素比对）

设计：
  - 主图案是「四象限纯色」（红/绿/蓝/黄）：探针在四象限中心采样即可做
    **确定性**像素判定（不依赖人眼、不需要模糊容差），有损格式放宽容差。
  - 同时生成渐变图供人眼复核（read_image）。
  - 失败路径样本（0 字节 / 截断 / 扩展名与内容不符）用于契约验证。
  - 视音频由 ffmpeg 生成 1 秒 testsrc / 纯色 / 440Hz 正弦；缺 ffmpeg 时
    记为 SKIP(no-ffmpeg) 而不是失败（文档 §6.4 第 2 条）。
"""

import argparse
import base64
import json
import os
import shutil
import subprocess
import sys

QUAD_W, QUAD_H = 120, 80
# 四象限颜色（左上/右上/左下/右下）——像素判定的期望值来源
QUAD_COLORS = {
    "tl": (255, 0, 0),     # 红
    "tr": (0, 128, 0),     # 绿
    "bl": (0, 0, 255),     # 蓝
    "br": (255, 255, 0),   # 黄
}


def log(msg):
    print("[gen_samples] " + msg, flush=True)


def make_quad_image():
    from PIL import Image, ImageDraw
    img = Image.new("RGBA", (QUAD_W, QUAD_H), (0, 0, 0, 255))
    d = ImageDraw.Draw(img)
    hw, hh = QUAD_W // 2, QUAD_H // 2
    d.rectangle([0, 0, hw - 1, hh - 1], fill=QUAD_COLORS["tl"] + (255,))
    d.rectangle([hw, 0, QUAD_W - 1, hh - 1], fill=QUAD_COLORS["tr"] + (255,))
    d.rectangle([0, hh, hw - 1, QUAD_H - 1], fill=QUAD_COLORS["bl"] + (255,))
    d.rectangle([hw, hh, QUAD_W - 1, QUAD_H - 1], fill=QUAD_COLORS["br"] + (255,))
    return img


def make_gradient_image(w=QUAD_W, h=QUAD_H):
    from PIL import Image
    img = Image.new("RGB", (w, h))
    px = img.load()
    for y in range(h):
        for x in range(w):
            px[x, y] = (int(255 * x / max(1, w - 1)), int(255 * y / max(1, h - 1)), 128)
    return img


def make_solid_image(color, w=QUAD_W, h=QUAD_H):
    from PIL import Image
    return Image.new("RGB", (w, h), color)


class Manifest:
    def __init__(self):
        self.samples = []

    def add(self, **kw):
        self.samples.append(kw)
        log("  + %s" % kw.get("file"))

    def skip(self, name, reason):
        self.samples.append({"file": None, "name": name, "kind": "skipped",
                             "skip": reason})
        log("  ~ SKIP(%s) %s" % (reason, name))


def gen_images(out, m):
    """PIL 生成的静态图 / 动图 / SVG / 失败路径样本。"""
    try:
        from PIL import Image  # noqa: F401
    except ImportError:
        log("错误：未安装 Pillow（pip install Pillow）——静态图样本无法生成")
        return False

    quad = make_quad_image()
    gradient = make_gradient_image()

    def quad_entry(fn, fmt, mime, kind="image", wide=True, note=""):
        m.add(file=fn, name=fn, kind=kind, format=fmt, mime=mime,
              width=QUAD_W if wide else 64, height=QUAD_H if wide else 64,
              pattern="quad", quad=QUAD_COLORS, note=note)

    # ① 各格式的同一图案（quad）
    quad.save(os.path.join(out, "quad.png"))
    quad_entry("quad.png", "png", "image/png")

    quad.convert("RGB").save(os.path.join(out, "quad.jpg"), quality=92)
    quad_entry("quad.jpg", "jpeg", "image/jpeg", note="有损：判定放宽容差")

    quad.convert("P", palette=Image.Palette.ADAPTIVE).save(os.path.join(out, "quad.gif"))
    quad_entry("quad.gif", "gif", "image/gif", note="静态 GIF")

    quad.convert("RGB").save(os.path.join(out, "quad-lossy.webp"), lossless=False, quality=90)
    quad_entry("quad-lossy.webp", "webp", "image/webp", note="WebP 有损（D4：修复前无固有尺寸）")

    quad.convert("RGB").save(os.path.join(out, "quad-lossless.webp"), lossless=True)
    quad_entry("quad-lossless.webp", "webp", "image/webp", note="WebP 无损（D4）")

    quad.convert("RGB").save(os.path.join(out, "quad.bmp"))
    quad_entry("quad.bmp", "bmp", "image/bmp", note="BMP 24bit（D4）")

    quad.resize((64, 64)).save(os.path.join(out, "quad-64.ico"),
                               sizes=[(64, 64)])
    quad_entry("quad-64.ico", "ico", "image/x-icon", wide=False,
               note="ICO 内嵌 PNG（D4；ICO 惯例方形，故 64×64）")

    try:
        quad.convert("RGB").save(os.path.join(out, "quad.tiff"))
        quad_entry("quad.tiff", "tiff", "image/tiff", note="预期 L0（不支持）")
    except Exception as e:  # pragma: no cover - 取决于 Pillow 构建
        m.skip("quad.tiff", "pil-no-tiff: %s" % e)

    try:
        quad.convert("RGB").save(os.path.join(out, "quad.avif"))
        quad_entry("quad.avif", "avif", "image/avif", note="预期 L0（不支持）")
    except Exception as e:
        m.skip("quad.avif", "pil-no-avif: %s" % e)

    # ② 渐变图（人眼复核用）
    gradient.save(os.path.join(out, "gradient.png"))
    m.add(file="gradient.png", name="gradient.png", kind="image", format="png",
          mime="image/png", width=QUAD_W, height=QUAD_H, pattern="gradient",
          note="人眼复核（read_image）")

    # ③ 动图（G3）
    frames = [make_solid_image((255, 0, 0)), make_solid_image((0, 255, 0)),
              make_solid_image((0, 0, 255))]
    frames[0].save(os.path.join(out, "anim-3frames-rgb.gif"), save_all=True,
                   append_images=frames[1:], duration=[100, 100, 100], loop=0)
    m.add(file="anim-3frames-rgb.gif", name="anim-3frames-rgb.gif",
          kind="animated", format="gif", mime="image/gif",
          width=QUAD_W, height=QUAD_H, frames=3, delays=[100, 100, 100], loop=0,
          frame_colors=[[255, 0, 0], [0, 255, 0], [0, 0, 255]],
          note="红/绿/蓝各 100ms（TC-M-301/305）")

    frames[0].save(os.path.join(out, "anim-uneven-delay.gif"), save_all=True,
                   append_images=frames[1:], duration=[50, 200, 100], loop=0)
    m.add(file="anim-uneven-delay.gif", name="anim-uneven-delay.gif",
          kind="animated", format="gif", mime="image/gif",
          width=QUAD_W, height=QUAD_H, frames=3, delays=[50, 200, 100], loop=0,
          frame_colors=[[255, 0, 0], [0, 255, 0], [0, 0, 255]],
          note="不均匀 delay（TC-M-305）")

    frames[0].save(os.path.join(out, "anim-noloop.gif"), save_all=True,
                   append_images=frames[1:], duration=100, loop=1)
    m.add(file="anim-noloop.gif", name="anim-noloop.gif", kind="animated",
          format="gif", mime="image/gif", width=QUAD_W, height=QUAD_H, frames=3,
          delays=[100, 100, 100], loop=1,
          frame_colors=[[255, 0, 0], [0, 255, 0], [0, 0, 255]],
          note="loop=1 不循环（TC-M-307）")

    wframes = [make_solid_image((255, 0, 0)), make_solid_image((0, 0, 255))]
    wframes[0].save(os.path.join(out, "anim-2frames.webp"), save_all=True,
                    append_images=wframes[1:], duration=200, loop=0, lossless=True)
    m.add(file="anim-2frames.webp", name="anim-2frames.webp", kind="animated",
          format="webp", mime="image/webp", width=QUAD_W, height=QUAD_H, frames=2,
          delays=[200, 200], loop=0, frame_colors=[[255, 0, 0], [0, 0, 255]],
          note="WebP 动画必须走 SkCodec（TC-M-302）")

    # ④ 失败路径与尺寸边界（G4 / G7）
    with open(os.path.join(out, "quad.png"), "rb") as f:
        png_bytes = f.read()
    with open(os.path.join(out, "corrupt.png"), "wb") as f:
        f.write(png_bytes[: len(png_bytes) // 2])
    m.add(file="corrupt.png", name="corrupt.png", kind="broken", format="png",
          mime="image/png", pattern="quad", quad=QUAD_COLORS,
          note="截断 50%（TC-M-403）")

    with open(os.path.join(out, "zero-bytes.png"), "wb") as f:
        f.write(b"")
    m.add(file="zero-bytes.png", name="zero-bytes.png", kind="broken",
          format="png", mime="image/png", note="0 字节（TC-M-402）")

    with open(os.path.join(out, "quad.jpg"), "rb") as f:
        jpg_bytes = f.read()
    with open(os.path.join(out, "mislabeled.png"), "wb") as f:
        f.write(jpg_bytes)
    m.add(file="mislabeled.png", name="mislabeled.png", kind="image",
          format="jpeg", mime="image/jpeg", width=QUAD_W, height=QUAD_H,
          pattern="quad", quad=QUAD_COLORS,
          note=".png 扩展名实为 JPEG（TC-M-404：应按内容解码）")

    big = make_solid_image((128, 128, 128), 4096, 4096)
    big.save(os.path.join(out, "huge-4096.png"))
    m.add(file="huge-4096.png", name="huge-4096.png", kind="huge", format="png",
          mime="image/png", width=4096, height=4096, pattern="solid",
          solid=[128, 128, 128], note="性能/内存（TC-M-208）")

    # ⑤ 路径边界（TC-M-704/705）
    quad.save(os.path.join(out, "中文名-方块.png"))
    quad_entry("中文名-方块.png", "png", "image/png", note="中文文件名（TC-M-704）")
    quad.save(os.path.join(out, "with space.png"))
    quad_entry("with space.png", "png", "image/png", note="路径含空格（TC-M-705）、data: 内联图标; 相对路径图片; file:// 直接引用")
    quad.save(os.path.join(out, "图标-方块.png"))
    quad_entry("图标-方块.png", "png", "image/png", note="中文名（用户界面图标场景）")

    # ⑥ SVG（G2 / D3 / U5）
    svg_rect = ('<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80">'
                '<rect width="120" height="80" fill="#ff0000"/></svg>')
    with open(os.path.join(out, "rect-120x80.svg"), "w", encoding="utf-8") as f:
        f.write(svg_rect)
    m.add(file="rect-120x80.svg", name="rect-120x80.svg", kind="svg",
          format="svg", mime="image/svg+xml", width=120, height=80,
          pattern="solid", solid=[255, 0, 0],
          note="带 width/height（TC-M-201..204；file:// 与相对路径 = U5/D3）")

    svg_ratio = ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 80">'
                 '<rect width="120" height="80" fill="#0000ff"/></svg>')
    with open(os.path.join(out, "ratio-only.svg"), "w", encoding="utf-8") as f:
        f.write(svg_ratio)
    m.add(file="ratio-only.svg", name="ratio-only.svg", kind="svg", format="svg",
          mime="image/svg+xml", pattern="solid", solid=[0, 0, 255],
          note="仅 viewBox（比值型固有尺寸）")

    svg_icon = ('<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24">'
                '<rect width="24" height="24" fill="#008000"/></svg>')
    with open(os.path.join(out, "icon-24.svg"), "w", encoding="utf-8") as f:
        f.write(svg_icon)
    m.add(file="icon-24.svg", name="icon-24.svg", kind="svg", format="svg",
          mime="image/svg+xml", width=24, height=24, pattern="solid",
          solid=[0, 128, 0], note="图标尺寸（默认 intrinsic 24×24）")

    # SVG 的 data: 内联形式（页面直接内联，探针从 manifest 的 inline 字段取）
    # file 留空：这份「样本」没有独立文件，内容是内联进页面的（来源维度
    # = data:，与上面 rect-120x80.svg 的 file:// 来源成对，用于 U5 对照）。
    m.add(file=None, name="inline-svg-data-uri", kind="svg-inline",
          format="svg", mime="image/svg+xml", width=120, height=80,
          pattern="solid", solid=[255, 0, 0], inline=svg_rect)
    return True


def run_ffmpeg(ffmpeg, args, out):
    cmd = [ffmpeg, "-hide_banner", "-loglevel", "error", "-y"] + args
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError((p.stderr or "").strip().splitlines()[-1] if p.stderr else "ffmpeg failed")


def gen_media(out, m, ffmpeg):
    """ffmpeg 生成的视音频样本（G5/G6）。"""
    if not ffmpeg or not shutil.which(ffmpeg):
        m.skip("mp4/webm/wav/mp3/ogg/m4a", "no-ffmpeg")
        return

    def enc(name, args, kind, fmt, mime, **kw):
        try:
            run_ffmpeg(ffmpeg, args, out)
        except Exception as e:
            m.skip(name, "ffmpeg-%s" % str(e)[:60])
            return
        m.add(file=name, name=name, kind=kind, format=fmt, mime=mime, **kw)

    # 视频：1 秒 120×80
    enc("testsrc-1s.mp4",
        ["-f", "lavfi", "-i", "testsrc=size=120x80:rate=25:duration=1",
         "-pix_fmt", "yuv420p", "-c:v", "libx264", os.path.join(out, "testsrc-1s.mp4")],
        "video", "mp4", "video/mp4", width=120, height=80, duration=1.0,
        note="testsrc 1s（TC-M-501/503）")

    enc("solid-red-1s.mp4",
        ["-f", "lavfi", "-i", "color=c=red:s=120x80:d=1:r=25",
         "-pix_fmt", "yuv420p", "-c:v", "libx264", os.path.join(out, "solid-red-1s.mp4")],
        "video", "mp4", "video/mp4", width=120, height=80, duration=1.0,
        solid=[255, 0, 0],
        note="纯红 1s（A1 抽帧参照：帧色可独立验证）")

    enc("twophase-1s.mp4",
        ["-f", "lavfi", "-i", "color=c=red:s=120x80:d=0.5:r=25",
         "-f", "lavfi", "-i", "color=c=blue:s=120x80:d=0.5:r=25",
         "-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
         "-pix_fmt", "yuv420p", "-c:v", "libx264", os.path.join(out, "twophase-1s.mp4")],
        "video", "mp4", "video/mp4", width=120, height=80, duration=1.0,
        phases=[[0.0, 0.5, [255, 0, 0]], [0.5, 1.0, [0, 0, 255]]],
        note="两段式 0-0.5s 红 / 0.5-1s 蓝（TC-M-507 随时间变化）")

    enc("testsrc-1s.webm",
        ["-f", "lavfi", "-i", "testsrc=size=120x80:rate=25:duration=1",
         "-c:v", "libvpx-vp9", "-b:v", "200k", os.path.join(out, "testsrc-1s.webm")],
        "video", "webm", "video/webm", width=120, height=80, duration=1.0,
        note="VP9 1s")

    # 音频：440Hz 正弦 1 秒
    enc("sine-440-1s.wav",
        ["-f", "lavfi", "-i", "sine=frequency=440:duration=1",
         os.path.join(out, "sine-440-1s.wav")],
        "audio", "wav", "audio/wav", duration=1.0, hz=440,
        note="440Hz 正弦（TC-M-601/604）")

    enc("sine-440-1s.mp3",
        ["-f", "lavfi", "-i", "sine=frequency=440:duration=1",
         "-c:a", "libmp3lame", os.path.join(out, "sine-440-1s.mp3")],
        "audio", "mp3", "audio/mpeg", duration=1.0, hz=440)

    enc("sine-440-1s.ogg",
        ["-f", "lavfi", "-i", "sine=frequency=440:duration=1",
         "-c:a", "libvorbis", os.path.join(out, "sine-440-1s.ogg")],
        "audio", "ogg", "audio/ogg", duration=1.0, hz=440)

    enc("sine-440-1s.m4a",
        ["-f", "lavfi", "-i", "sine=frequency=440:duration=1",
         "-c:a", "aac", os.path.join(out, "sine-440-1s.m4a")],
        "audio", "m4a", "audio/mp4", duration=1.0, hz=440)


def main():
    ap = argparse.ArgumentParser(description="媒体格式验证样本生成")
    ap.add_argument("--out", default=os.path.join("dev", "media", "samples"),
                    help="输出目录（默认 dev/media/samples；不入库）")
    ap.add_argument("--ffmpeg", default="ffmpeg", help="ffmpeg 可执行文件")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    out = os.path.abspath(args.out)
    os.makedirs(out, exist_ok=True)
    log("输出目录：%s" % out)

    m = Manifest()
    ok = gen_images(out, m)
    if not ok:
        log("静态图样本生成失败（缺 Pillow）")
    gen_media(out, m, args.ffmpeg)

    manifest = {
        "generated_by": "dev/media/gen_samples.py",
        "quad_geometry": {"width": QUAD_W, "height": QUAD_H,
                          "sample_points": {"tl": [QUAD_W // 4, QUAD_H // 4],
                                            "tr": [QUAD_W * 3 // 4, QUAD_H // 4],
                                            "bl": [QUAD_W // 4, QUAD_H * 3 // 4],
                                            "br": [QUAD_W * 3 // 4, QUAD_H * 3 // 4]}},
        "samples": m.samples,
    }
    mpath = os.path.join(out, "manifest.json")
    with open(mpath, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=2)

    real = [s for s in m.samples if s.get("file")]
    skipped = [s for s in m.samples if s.get("kind") == "skipped"]
    log("完成：%d 个样本，%d 项 SKIP → %s" % (len(real), len(skipped), mpath))
    for s in skipped:
        log("  SKIP %s（%s）" % (s["name"], s["skip"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
