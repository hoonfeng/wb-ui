package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// helperRenderHSTextarea renders an overflow:auto textarea whose value is a
// single long line (white-space:pre → no soft wrap, horizontal scrollbar
// needed) through the full pipeline.
func helperRenderHSTextarea(t *testing.T, overflow, text string, cx, cy float64) (*graphics.Canvas, *RenderBox, float64) {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<textarea id="s" style="width:200px;height:60px;padding:4px 6px;overflow:` + overflow + `;white-space:pre;font-family:Consolas;font-size:13px">` + text + `</textarea>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(260, 160)
	state := layout.NewLayoutState(260, 160)
	rv.Layout(state)
	rv.SetCursorPos(cx, cy)

	var sbox *RenderBox
	var find func(o RenderObject)
	find = func(o RenderObject) {
		if sbox != nil {
			return
		}
		if el, ok2 := o.Node().(*dom.Element); ok2 && el.GetAttribute("id") == "s" {
			sbox = asRenderBox(o)
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(RenderObject(rv))

	canvas := graphics.NewCanvas(260, 160)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 260, Height: 160})
	if sbox != nil {
		cw, _ := rv.BoxContentSize(sbox)
		t.Logf("textarea pb=(%.0f,%.0f %.0fx%.0f) contentW=%.0f", sbox.PaddingBoxRect().X, sbox.PaddingBoxRect().Y, sbox.PaddingBoxRect().Width, sbox.PaddingBoxRect().Height, cw)
	}
	return canvas, sbox, 0
}

// TestScrollbarResidentNoHover: overflow:auto horizontal scrollbar is painted
// even with the cursor far away — Windows-style resident scrollbars, not
// macOS-style overlay auto-hiding.
func TestScrollbarResidentNoHover(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), -100, -100)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	pb := sbox.PaddingBoxRect()
	hy := pb.Y + pb.Height - 17 // 平台经典滚动条宽 17px（Chromium/Windows 实测）
	if hy < 1 {
		hy = 1
	}
	foundTrack := false
	for x := 20; x < int(pb.Width)-10 && x < 200; x += 10 {
		px := canvas.PixelAt(x, int(hy)+3)
		if px.R > 230 && px.G > 230 && px.B > 230 {
			foundTrack = true
			break
		}
	}
	if !foundTrack {
		t.Fatalf("overflow:auto horizontal track not painted with cursor far away (pb=%+v)", pb)
	}
}

// TestScrollbarThumbNotCoverRightArrow: at maximum scroll the thumb's right
// edge stays left of the right-arrow button, so the arrow remains visible.
//
// ★ 2026-09-26 改写：自绘滚动条（15px）的箭头三角与 thumb **同色**
// （有头 Edge 实测 #414B64 —— 箭头跟随 scrollbar-color 的 thumb 色），原先
// 「thumb 是 #A0A0A0、箭头是 #606060，用颜色区分」的像素断言已失去意义。
// 改为两层断言：
//  1. 几何：滚到底时 thumb 右缘恰好贴住右箭头按钮左缘（不越界）；
//  2. 像素：右箭头按钮中心列在按钮中心行与按钮顶行的像素不同 ——
//     即三角确实被画出来（顶行是纯轨道色，中心行有三角）。
func TestScrollbarThumbNotCoverRightArrow(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), 60, 60)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	cw, _ := sbox.View().BoxContentSize(sbox)
	pb := sbox.PaddingBoxRect()
	viewW := pb.Width - 6 - 6 // padding 6px each side
	if el, ok := sbox.Node().(*dom.Element); ok {
		SetFormControlTextScroll(el, cw-viewW) // max scroll
	} else {
		t.Fatalf("scroll box has no element")
	}
	rv := sbox.View()
	scrollW := style.ScrollbarWidth(sbox.Style())

	// ── 1) 几何：滚到底时 thumb 右缘 = 右箭头按钮左缘 ──
	m := HorizontalScrollbarMetrics(rv, sbox)
	if !m.OK {
		t.Fatalf("expected horizontal scrollbar metrics for overflowing textarea")
	}
	thumbRight := pb.X + scrollW + (m.TrackLen - m.ThumbLen) + m.ThumbLen
	arrowLeft := pb.X + pb.Width - scrollW
	if thumbRight > arrowLeft+0.5 {
		t.Fatalf("滚到底时 thumb 右缘 %.1f 覆盖右箭头按钮（按钮左缘 %.1f）", thumbRight, arrowLeft)
	}

	// ── 2) 像素：右箭头按钮内确实画出了三角 ──
	canvas2 := graphics.NewCanvas(260, 160)
	Paint(rv, canvas2, Rect{X: 0, Y: 0, Width: 260, Height: 160})
	defer canvas2.Release()
	hy := int(pb.Y + pb.Height - scrollW)
	// 三角中心 = 「箭头按钮 + gap」区间中点（有头 Edge 实测距轨道端 8.8px，
	// 即 (15+3)/2 = 9，而不是按钮内居中的 7.5）。
	const sbGap = 3.0
	mid := int((scrollW + sbGap) / 2)
	btnCenterX := int(pb.X+pb.Width) - mid
	centerRow := canvas2.PixelAt(btnCenterX, hy+mid)
	topRow := canvas2.PixelAt(btnCenterX, hy+1)
	if centerRow == topRow {
		t.Fatalf("右箭头按钮中心列 x=%d：中心行像素 %+v 与顶行 %+v 相同 —— 箭头三角未画出（或按钮几何错位）",
			btnCenterX, centerRow, topRow)
	}
}

// TestHitTestScrollbarTextarea: the horizontal scrollbar of a textarea
// (pre-mode, long value) must be hit-testable — previously HitTestScrollbar
// only measured render-tree children, so a textarea (text lives in
// textContent, no children) always returned nil and scrollbar clicks fell
// through to the input control.
func TestHitTestScrollbarTextarea(t *testing.T) {
	canvas, sbox, _ := helperRenderHSTextarea(t, "auto", strings.Repeat("ab", 60), 60, 60)
	defer canvas.Release()
	if sbox == nil {
		t.Fatalf("scroll box not found")
	}
	rv := sbox.View()
	pb := sbox.PaddingBoxRect()
	st := sbox.Style()
	cw2, ch2 := rv.BoxContentSize(sbox)
	t.Logf("overflowY=%v overflowX=%v content=(%.1f,%.1f) view=(%.1f,%.1f)", st.OverflowY, st.OverflowX, cw2, ch2, pb.Width-6-6, pb.Height-4-4)
	trackY := pb.Y + pb.Height - 12 + 3 // inside the 12px horizontal track
	if trackY < pb.Y {
		trackY = pb.Y
	}
	// Left arrow zone: x in [pb.X, pb.X+12).
	hit := HitTestScrollbar(rv, pb.X+4, trackY)
	if hit == nil {
		t.Fatalf("left arrow not hit — scrollbar clicks fall through (trackY=%.0f)", trackY)
	}
	if !hit.IsHLeftArrow {
		t.Fatalf("left arrow zone → %+v, want IsHLeftArrow", hit)
	}
	// Right arrow zone: last 12px.
	hit = HitTestScrollbar(rv, pb.X+pb.Width-4, trackY)
	if hit == nil || !hit.IsHRightArrow {
		t.Fatalf("right arrow zone → %+v, want IsHRightArrow", hit)
	}
	// Track zone (middle, away from arrows and thumb at scroll=0): thumb
	// starts at hx+arrowSize with scroll=0, so the middle is track.
	hit = HitTestScrollbar(rv, pb.X+pb.Width/2, trackY)
	if hit == nil {
		t.Fatalf("track middle not hit")
	}
	if !hit.IsHTrack && !hit.IsHThumb {
		t.Fatalf("track middle → %+v, want IsHTrack/IsHThumb", hit)
	}
}

