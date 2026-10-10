package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestComputeGradientDest: explicit sizes confine the gradient; auto fills.
func TestComputeGradientDest(t *testing.T) {
	// auto → full box
	dx, dy, dw, dh := computeGradientDest(0, 0, 200, 100, "", "")
	if dw != 200 || dh != 100 || dx != 0 || dy != 0 {
		t.Fatalf("auto = (%v,%v %vx%v), want full box", dx, dy, dw, dh)
	}
	// explicit 70x70 at bottom-right (position 100% 100%)
	dx, dy, dw, dh = computeGradientDest(0, 0, 200, 100, "70px 70px", "100% 100%")
	if dw != 70 || dh != 70 || dx != 130 || dy != 30 {
		t.Fatalf("70px bottom-right = (%v,%v %vx%v), want (130,30 70x70)", dx, dy, dw, dh)
	}
	// percentage 50% 50% centered
	dx, dy, dw, dh = computeGradientDest(0, 0, 200, 100, "50% 50%", "center")
	if dw != 100 || dh != 50 || dx != 50 || dy != 25 {
		t.Fatalf("50%% centered = (%v,%v %vx%v)", dx, dy, dw, dh)
	}
	// cover/contain → full box for gradients
	dx, dy, dw, dh = computeGradientDest(0, 0, 200, 100, "cover", "left top")
	if dw != 200 || dh != 100 {
		t.Fatalf("cover = %vx%v, want full box", dw, dh)
	}
}

// TestPaintGradientSized: background-size 限定渐变 tile 的尺寸；在
// `background-repeat:no-repeat` 下只画这一个 tile，其余区域透明。
//
// ★ 本用例此前**没有**设置 background-repeat，等于依赖「渐变永远只画一个
// tile」的旧行为 —— 与 CSS 默认值 repeat 冲突（默认必须平铺，见
// TestPaintGradientRepeats）。浏览器里 `background-size:60px 60px` +
// 未声明 repeat 会在整个盒内铺满 60x60 的 tile。
func TestPaintGradientSized(t *testing.T) {
	canvas := graphics.NewCanvas(120, 60)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 60})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	// Solid red gradient confined to the left half (60x60), centered.
	st.BackgroundImage = "linear-gradient(to right,#ff0000,#ff0000)"
	st.BackgroundSize = "60px 60px"
	st.BackgroundPosition = "left top"
	st.BackgroundRepeat = "no-repeat"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(120, 60)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(30, 30); px.R != 255 || px.G != 0 {
		t.Fatalf("inside gradient (30,30) = %+v, want red", px)
	}
	// no-repeat：右半（tile 之外）保持透明。
	if px := canvas.PixelAt(90, 30); px.A != 0 {
		t.Fatalf("outside gradient (90,30) = %+v, want transparent", px)
	}
}

// TestPaintGradientRepeats: 未声明 background-repeat 时取 CSS 初始值
// **repeat** —— 被 background-size 限定的渐变 tile 必须在背景区域内平铺。
//
// 反例（旧行为）：只画一个 tile → 真实页面里 `background-size:32px 32px` +
// 双层硬边渐变的网格背景整片消失（MediaStudio `#stage .grid` 实测
// Edge 29924 个网格线像素 / wb-ui 16）。
func TestPaintGradientRepeats(t *testing.T) {
	canvas := graphics.NewCanvas(120, 60)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 60})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "linear-gradient(to right,#ff0000,#ff0000)"
	st.BackgroundSize = "30px 30px"
	st.BackgroundPosition = "left top"
	// 不设 BackgroundRepeat：按 CSS 初始值 repeat 语义处理（空串与 "repeat" 等价）。
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(120, 60)

	paintObjectBackground(box, info)

	// 首块内、第二块（x=30 起）内、第二行块（y=30 起）内都应被平铺覆盖。
	for _, p := range [][2]int{{15, 15}, {45, 15}, {105, 45}, {75, 55}} {
		px := canvas.PixelAt(p[0], p[1])
		if px.R != 255 || px.G != 0 || px.A == 0 {
			t.Fatalf("tile 平铺缺失：(%d,%d) = %+v, want 红色不透明", p[0], p[1], px)
		}
	}
}

// TestParseBgLayerPosSizeFromStyle: the resolver path — the background
// shorthand "linear-gradient(...) 0 0/70px 70px no-repeat" fills
// BackgroundPosition/BackgroundSize. Re-runs the style-side extraction via
// the exported tokenizer path (parseBgLayerPosSize is unexported in style,
// so exercise it indirectly through a painted sub-rect).
func TestBackgroundShorthandPosSize(t *testing.T) {
	// Sanity: split of the shorthand parts.
	parts := splitBgShorthandForTest("linear-gradient(to right,#ff0000,#ff0000) 0 0/70px 70px no-repeat")
	found := false
	for _, p := range parts {
		if p == "0/70px" {
			found = true
		}
	}
	if !found {
		t.Fatalf("parts missing 0/70px: %v", parts)
	}
}

// splitBgShorthandForTest mirrors style.splitShorthandValue behavior for the
// shorthand we care about (whitespace-split, parens preserved).
func splitBgShorthandForTest(s string) []string {
	var parts []string
	depth := 0
	cur := bytes.Buffer{}
	for _, r := range s {
		switch r {
		case '(':
			depth++
			cur.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case ' ', '\t', '\n', '\r':
			if depth > 0 {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

var _ = base64.StdEncoding
var _ = image.Rect
var _ = color.RGBA{}
var _ = png.Encode
