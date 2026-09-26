package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// pillProbeCSS mirrors the real composer row-1 rules from
// plugins-src/ui-app/src/components/RightPanel.vue (--fs-xs = 11px per
// release/PairCode/.pair/assets/runtime/web/index.html).
const pillProbeCSS = `
:root { --fs-xs: 11px; --accent: #58a6ff; --bg-active: #2b3350; }
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: 100%; height: 100%; overflow: hidden; }
body { font-family: sans-serif; font-size: 16px; background: #101010; padding-top: 8px; }
.input-row1 { display: flex; align-items: center; gap: 8px; height: 24px; }
.ir-pill { display: inline-flex; align-items: center; justify-content: center; height: 24px; padding: 0 10px; border-radius: 999px; font-size: var(--fs-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ir-pill-model { background: var(--bg-active); color: var(--accent); }
`

const pillHTML = `<html><head><style>` + pillProbeCSS + `</style></head><body>
<div class="input-row1"><span class="ir-pill ir-pill-model">deepseek-flash</span></div>
</body></html>`

type pillGeom struct {
	top, left, w, h float64
	segs            []InlineTextBox
}

func buildPill(t *testing.T, htmlSrc string, w, h int) *RenderView {
	t.Helper()
	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(pillProbeCSS).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil RenderView")
	}
	rv.SetViewportSize(float64(w), float64(h))
	rv.Layout(nil)
	return rv
}

func collectPill(rv *RenderView) pillGeom {
	var g pillGeom
	ls := rv.LayoutState()
	var walk func(RenderObject)
	walk = func(ro RenderObject) {
		// 前缀包含匹配：基础用例 class="ir-pill ir-pill-model"，line-height
		// 变体额外附加 "ir-pill-lh"（全等匹配会让变体拿不到几何）。
		if el, ok := ro.Node().(*dom.Element); ok && strings.Contains(el.GetClassName(), "ir-pill-model") {
			if lb := ro.LayoutBox(); lb != nil && ls != nil {
				gg := ls.GeometryForBox(lb)
				g.top, g.left = gg.Top(), gg.Left()
				g.w, g.h = gg.BorderBoxWidth(), gg.BorderBoxHeight()
			}
		}
		if rt, ok := ro.(*RenderText); ok {
			g.segs = append(g.segs, rt.Segments()...)
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	return g
}

// scanPillInk reports the glyph pixel rows inside the pill. The background
// colour is taken as the histogram mode of the scanned band so that the sample
// can never land on a glyph (which would wipe the whole band to "ink").
func scanPillInk(t *testing.T, canvas *graphics.Canvas, g pillGeom) (bgR, bgG, bgB uint8, rows []int) {
	t.Helper()
	top, bot := int(g.top), int(g.top+g.h)
	// 避开 999px 圆角（半径 = h/2 = 12px）的边缘像素，只在直边中段统计。
	x0, x1 := int(g.left)+14, int(g.left+g.w)-14
	if x1 <= x0 {
		t.Fatalf("pill too narrow for ink scan: x0=%d x1=%d", x0, x1)
	}
	cnt := map[uint32]int{}
	for y := top; y < bot; y++ {
		for x := x0; x < x1; x++ {
			p := canvas.PixelAt(x, y)
			cnt[uint32(p.R)<<16|uint32(p.G)<<8|uint32(p.B)]++
		}
	}
	bestN, bestK := -1, uint32(0)
	for k, n := range cnt {
		if n > bestN {
			bestN, bestK = n, k
		}
	}
	bgR, bgG, bgB = uint8(bestK>>16), uint8(bestK>>8), uint8(bestK)
	for y := top; y < bot; y++ {
		n := 0
		for x := x0; x < x1; x++ {
			p := canvas.PixelAt(x, y)
			if p.A == 0 {
				continue
			}
			d := probeAbs(int(p.R)-int(bgR)) + probeAbs(int(p.G)-int(bgG)) + probeAbs(int(p.B)-int(bgB))
			if d > 60 {
				n++
			}
		}
		if n > 0 {
			rows = append(rows, y)
		}
	}
	return
}

// TestPillTextLayoutVerticalCenter checks the LAYOUT layer: the text line box
// must be centered inside the 24px pill.
func TestPillTextLayoutVerticalCenter(t *testing.T) {
	rv := buildPill(t, pillHTML, 320, 40)
	g := collectPill(rv)
	if g.h == 0 {
		t.Fatal("no pill geometry")
	}
	t.Logf("pill top=%.2f left=%.2f w=%.2f h=%.2f center=%.2f", g.top, g.left, g.w, g.h, g.top+g.h/2)
	if len(g.segs) == 0 {
		t.Fatal("no InlineTextBox segments")
	}
	for i, s := range g.segs {
		t.Logf("seg[%d] X=%.2f Y=%.2f W=%.2f H=%.2f LineY=%.2f LineH=%.2f lineCenter=%.2f (delta vs pill center %.2f)",
			i, s.X, s.Y, s.Width, s.Height, s.LineY, s.LineHeight, s.Y+s.Height/2, (s.Y+s.Height/2)-(g.top+g.h/2))
	}
}

// TestPillTextPaintVerticalCenter checks the PAINT layer: the glyph pixels must
// be vertically centered between the pill's top and bottom edges.
func TestPillTextPaintVerticalCenter(t *testing.T) {
	const W, H = 320, 40
	rv := buildPill(t, pillHTML, W, H)
	g := collectPill(rv)
	if g.h == 0 {
		t.Fatal("no pill geometry")
	}
	t.Logf("pill top=%.2f left=%.2f w=%.2f h=%.2f center=%.2f", g.top, g.left, g.w, g.h, g.top+g.h/2)
	for i, s := range g.segs {
		t.Logf("seg[%d] X=%.2f Y=%.2f W=%.2f H=%.2f LineY=%.2f LineH=%.2f lineCenter=%.2f (delta vs pill center %.2f)",
			i, s.X, s.Y, s.Width, s.Height, s.LineY, s.LineHeight, s.Y+s.Height/2, (s.Y+s.Height/2)-(g.top+g.h/2))
	}

	canvas := graphics.NewCanvas(W, H)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: W, Height: H})
	if p := savePNG(canvas, "wbui_pill_probe.png"); p != "" {
		t.Logf("PNG: %s", p)
	}

	bgR, bgG, bgB, inkRowList := scanPillInk(t, canvas, g)
	top, bot := int(g.top), int(g.top+g.h)
	t.Logf("pill bg (histogram mode) = #%02x%02x%02x", bgR, bgG, bgB)
	if len(inkRowList) == 0 {
		t.Fatalf("no glyph pixels inside pill (bg=#%02x%02x%02x)", bgR, bgG, bgB)
	}
	first, last := inkRowList[0], inkRowList[len(inkRowList)-1]
	above, below := first-top, bot-1-last
	t.Logf("INK rows %d..%d: above=%d below=%d (inkH=%d boxH=%d)", first, last, above, below, last-first+1, bot-top)
	switch {
	case above == below:
		t.Logf("=> CENTERED")
	case above > below:
		t.Logf("=> TEXT LOW by %d px", above-below)
	default:
		t.Logf("=> TEXT HIGH by %d px", below-above)
	}
}

// TestPillTextPaintVerticalCenter_LineHeightVariant runs the same probe against
// a `display:block; line-height:24px` pill — the classic CSS fallback for
// vertical centering — to see whether the engine centres text there.
func TestPillTextPaintVerticalCenter_LineHeightVariant(t *testing.T) {
	const W, H = 320, 40
	const htmlSrc = `<html><head><style>` + pillProbeCSS + `
.ir-pill-lh { display: block; line-height: 24px; }
</style></head><body>
<div class="input-row1"><span class="ir-pill ir-pill-model ir-pill-lh">deepseek-flash</span></div>
</body></html>`
	rv := buildPill(t, htmlSrc, W, H)
	g := collectPill(rv)
	if g.h == 0 {
		t.Fatal("no pill geometry")
	}
	t.Logf("pill(lh) top=%.2f left=%.2f w=%.2f h=%.2f center=%.2f", g.top, g.left, g.w, g.h, g.top+g.h/2)
	for i, s := range g.segs {
		t.Logf("seg[%d] X=%.2f Y=%.2f W=%.2f H=%.2f LineY=%.2f LineH=%.2f lineCenter=%.2f (delta vs pill center %.2f)",
			i, s.X, s.Y, s.Width, s.Height, s.LineY, s.LineHeight, s.Y+s.Height/2, (s.Y+s.Height/2)-(g.top+g.h/2))
	}
	canvas := graphics.NewCanvas(W, H)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: W, Height: H})
	if p := savePNG(canvas, "wbui_pill_probe_lh.png"); p != "" {
		t.Logf("PNG: %s", p)
	}
	bgR, bgG, bgB, inkRowList := scanPillInk(t, canvas, g)
	top, bot := int(g.top), int(g.top+g.h)
	t.Logf("pill bg (histogram mode) = #%02x%02x%02x", bgR, bgG, bgB)
	if len(inkRowList) == 0 {
		t.Fatalf("no glyph pixels inside pill (bg=#%02x%02x%02x)", bgR, bgG, bgB)
	}
	first, last := inkRowList[0], inkRowList[len(inkRowList)-1]
	above, below := first-top, bot-1-last
	t.Logf("INK rows %d..%d: above=%d below=%d (inkH=%d boxH=%d)", first, last, above, below, last-first+1, bot-top)
	switch {
	case above == below:
		t.Logf("=> CENTERED")
	case above > below:
		t.Logf("=> TEXT LOW by %d px", above-below)
	default:
		t.Logf("=> TEXT HIGH by %d px", below-above)
	}
}

func probeAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
