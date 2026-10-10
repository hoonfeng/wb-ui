package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"testing"
)

// range / checkbox 的像素回归用例（Edge headless 为参照）。
//
// 覆盖 2026-10 用户实测反馈的两处差异，并锁定其结构：
//   - checkbox 外轮廓必须等于元素盒（旧实现 StrokeRect 以边界为中心线，向外
//     多出半像素 → 15×15 盒画出 17×16，即「比浏览器多出描边/边框」）；checked
//     态不得有灰描边（Edge 是纯 accent 底 + 白勾）；
//   - range 轨道 8px（上下各 1px 边缘色 = (主体色+#757575)/2）、滑块 16px 且
//     行程内缩 R（值取端点时内切轨道两端，不溢出元素盒）。
//
// 剩余差异全是 AA 过渡带（thumb 圆边缘、checkbox 边框/勾、轨道端部圆角）——
// Skia/goskia 抗锯齿实现的亚像素差异，故给 1.5% 预算；修复前为 4.80%。
//
// 同时把取证落盘到 dev/output/（gitignored），便于后续目视与排查：
//   - formctl_probe.txt   行跨度表 + 调色板 dump
//   - formctl_probe.png   左=Edge 右=wb-ui 放大并排图（nearest 放大 5x）
//   - formctl_edge.png / formctl_wbui.png  双方原图

var probeLog strings.Builder

func probePrintf(format string, a ...any) {
	fmt.Fprintf(&probeLog, format+"\n", a...)
}

func probeZoom(src *image.RGBA, r image.Rectangle, z int) *image.RGBA {
	w, h := r.Dx()*z, r.Dy()*z
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetRGBA(x, y, src.RGBAAt(r.Min.X+x/z, r.Min.Y+y/z))
		}
	}
	return dst
}

func probeSideBySide(exp, got *image.RGBA, r image.Rectangle, z, gap int) *image.RGBA {
	a := probeZoom(exp, r, z)
	b := probeZoom(got, r, z)
	w := a.Bounds().Dx() + gap + b.Bounds().Dx()
	h := a.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.RGBA{R: 255, G: 0, B: 255, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(0, 0, a.Bounds().Dx(), h), a, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(a.Bounds().Dx()+gap, 0, w, h), b, image.Point{}, draw.Src)
	return dst
}

func probeSavePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	t.Logf("写出 %s", path)
}

// probeRowSpans 打印「非背景像素」的逐行 x 跨度（只在跨度变化时换行）。
// 多画的 1px 描边会让外轮廓比参照大一格；thumb 的直径/位置也直接读得出来。
func probeRowSpans(label string, img *image.RGBA, bg color.RGBA, w, h int) {
	probePrintf("── %s 逐行非背景跨度（bg=#%02x%02x%02x，容差 8）", label, bg.R, bg.G, bg.B)
	prev := "none"
	startY := 0
	flush := func(endY int) {
		if startY > endY {
			return
		}
		if prev == "none" {
			probePrintf("   y=%d..%d  (空)", startY, endY)
		} else {
			probePrintf("   y=%d..%d  %s", startY, endY, prev)
		}
	}
	for y := 0; y < h; y++ {
		x0, x1 := -1, -1
		for x := 0; x < w; x++ {
			if chanDiff(img.RGBAAt(x, y), bg) > 8 {
				if x0 < 0 {
					x0 = x
				}
				x1 = x
			}
		}
		cur := "none"
		if x0 >= 0 {
			cur = fmt.Sprintf("x=%d..%d", x0, x1)
		}
		if cur != prev {
			flush(y - 1)
			startY = y
			prev = cur
		}
	}
	flush(h - 1)
}

// probeDump 以「调色板索引」打印区域，便于肉眼比对结构（描边这类 1px 差异）。
func probeDump(label string, img *image.RGBA, r image.Rectangle) {
	syms := []byte(".#*+oO0@$%&=~-abcdefghijklmnopqrstuvwxyz")
	pal := []color.RGBA{}
	idx := map[color.RGBA]int{}
	var lines []string
	for y := r.Min.Y; y < r.Max.Y; y++ {
		line := make([]byte, 0, r.Dx())
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			k, ok := idx[c]
			if !ok {
				k = len(pal)
				idx[c] = k
				pal = append(pal, c)
			}
			if k < len(syms) {
				line = append(line, syms[k])
			} else {
				line = append(line, '?')
			}
		}
		lines = append(lines, string(line))
	}
	probePrintf("── %s 区域 (%d,%d)-(%d,%d)  图例：", label, r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
	for i, c := range pal {
		mark := byte('?')
		if i < len(syms) {
			mark = syms[i]
		}
		probePrintf("   %c = #%02x%02x%02x", mark, c.R, c.G, c.B)
	}
	for i, l := range lines {
		probePrintf("   y%-3d %s", r.Min.Y+i, l)
	}
}

const probeBody = `<div style="font-size:0;line-height:0;width:129px;height:18px"><input id="r0" type="range" min="0" max="6" value="0" style="width:100%;height:18px;accent-color:#1473e6"></div>
<div style="font-size:0;line-height:0;width:129px;height:18px;margin-top:20px"><input id="r1" type="range" min="0" max="6" value="3" style="width:100%;height:18px;accent-color:#1473e6"></div>
<div style="font-size:0;line-height:0;width:129px;height:18px;margin-top:20px"><input id="r2" type="range" min="0" max="6" value="6" style="width:100%;height:18px;accent-color:#1473e6"></div>
<div style="font-size:0;line-height:0;height:12px"></div>
<div style="font-size:0;line-height:0;height:15px"><input id="cb1" type="checkbox" checked style="width:15px;height:15px;accent-color:#1473e6"></div>
<div style="font-size:0;line-height:0;height:15px;margin-top:16px"><input id="cb0" type="checkbox" style="width:15px;height:15px;accent-color:#1473e6"></div>`

func TestPxFormControl(t *testing.T) {
	c := TestCase{
		Name:      "probe_formctl",
		ViewportW: 200, ViewportH: 160,
		HTML: baseDoc(probeBody, `<style>body{background:#1e1e1e;margin:0}</style>`),
	}
	shotPath, sw, sh, err := edgeShot(c)
	if err != nil {
		t.Skipf("参照浏览器不可用: %v", err)
	}
	got, err := wbuiRenderPNG(c, sw, sh)
	if err != nil {
		t.Fatalf("wbuiRenderPNG: %v", err)
	}
	exp, err := loadPNG(shotPath)
	if err != nil {
		t.Fatalf("loadPNG: %v", err)
	}
	probePrintf("viewport=%dx%d  edgeShot=%s", sw, sh, shotPath)
	rep := pixelDiff(exp, got, 3)
	probePrintf("[px_form_control] %s", formatDiff(rep))

	bg := color.RGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 255}
	probeRowSpans("[Edge]", exp, bg, sw, sh)
	probeRowSpans("[wb-ui]", got, bg, sw, sh)

	probeDump("[Edge] range(中段 value=3) 左端", exp, image.Rect(0, 20, 40, 60))
	probeDump("[wb-ui] range(中段 value=3) 左端", got, image.Rect(0, 20, 40, 60))
	probeDump("[Edge] range(value=0) 左端", exp, image.Rect(0, 0, 24, 20))
	probeDump("[wb-ui] range(value=0) 左端", got, image.Rect(0, 0, 24, 20))
	probeDump("[Edge] range(value=6) 右端", exp, image.Rect(108, 40, 136, 60))
	probeDump("[wb-ui] range(value=6) 右端", got, image.Rect(108, 40, 136, 60))
	probeDump("[Edge] cb checked", exp, image.Rect(0, 104, 22, 124))
	probeDump("[wb-ui] cb checked", got, image.Rect(0, 104, 22, 124))
	probeDump("[Edge] cb unchecked", exp, image.Rect(0, 134, 22, 154))
	probeDump("[wb-ui] cb unchecked", got, image.Rect(0, 134, 22, 154))

	probeSavePNG(t, "../../output/formctl_edge.png", exp)
	probeSavePNG(t, "../../output/formctl_wbui.png", got)
	probeSavePNG(t, "../../output/formctl_probe.png", probeSideBySide(exp, got, image.Rect(0, 0, 140, 148), 5, 10))

	txt := "../../output/formctl_probe.txt"
	if err := os.WriteFile(txt, []byte(probeLog.String()), 0644); err != nil {
		t.Fatalf("write %s: %v", txt, err)
	}
	t.Logf("写出 %s（%d 字节）", txt, probeLog.Len())

	// 预算 1.5%：仅容纳 AA 过渡带残差（见文件头注释）。
	const maxDiffRate = 0.015
	if rep.DiffRate > maxDiffRate {
		t.Errorf("[px_form_control] diff rate %.3f%% 超出预算 %.3f%%（%d px）",
			rep.DiffRate*100, maxDiffRate*100, rep.DiffPixels)
	}
}
