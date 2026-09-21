package rendering

import (
	"fmt"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// renderSVGPath paints a single <path> (viewBox == canvas size, scale 1) and
// returns the canvas. Callers must Release it.
func renderSVGPath(t *testing.T, w, h int, d string, attrs map[string]string) *graphics.Canvas {
	return renderSVGPathFull(t, w, h, d, attrs, nil)
}

// renderSVGPathFull is renderSVGPath with extra attributes on the <svg> root
// (to exercise inheritance of SVG presentation attributes).
func renderSVGPathFull(t *testing.T, w, h int, d string, attrs, svgAttrs map[string]string) *graphics.Canvas {
	t.Helper()
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", fmt.Sprint(w))
	svgEl.SetAttribute("height", fmt.Sprint(h))
	svgEl.SetAttribute("viewBox", fmt.Sprintf("0 0 %d %d", w, h))
	for k, v := range svgAttrs {
		svgEl.SetAttribute(k, v)
	}
	p := doc.CreateElement("path")
	p.SetAttribute("d", d)
	p.SetAttribute("fill", "none")
	p.SetAttribute("stroke", "black")
	p.SetAttribute("stroke-width", "1")
	for k, v := range attrs {
		p.SetAttribute(k, v)
	}
	svgEl.AppendChild(p)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	canvas := graphics.NewCanvas(w, h)
	paintSVGTo(canvas, sd, 0, 0, float64(w), float64(h), graphics.Color{})
	return canvas
}

// TestSVGPathSubpathsStaySeparate: several subpaths packed into one "d"
// (M…zM…z — the standard shape of Lucide/Feather icons) must stay separate.
// A flattened point list made the renderer connect them with phantom lines:
// for the 4-square grid below the stray segment (12,8)→(2,2) ran straight
// through the interiors of squares 1/2/3.
func TestSVGPathSubpathsStaySeparate(t *testing.T) {
	canvas := renderSVGPath(t, 20, 20,
		"M2 2h6v6h-6zM12 2h6v6h-6zM2 12h6v6h-6zM12 12h6v6h-6z",
		map[string]string{"stroke-width": "1.5"})
	defer canvas.Release()

	// Sample points sit well inside the squares / the gap, away from any
	// border pixel (the border is 1.5 wide, so ±0.75 around the edges).
	for _, pt := range [][2]int{{5, 5}, {15, 5}, {5, 15}, {15, 15}, {10, 10}, {10, 7}} {
		if px := canvas.PixelAt(pt[0], pt[1]); px.A != 0 {
			t.Fatalf("方块内部 (%d,%d) = %+v, 期望透明（子路径被幻觉连线连接）", pt[0], pt[1], px)
		}
	}
	// The four squares must still be drawn (border pixels present).
	for _, pt := range [][2]int{{5, 2}, {2, 5}, {16, 2}, {15, 12}} {
		if px := canvas.PixelAt(pt[0], pt[1]); px.A == 0 {
			t.Fatalf("方块边框 (%d,%d) 缺失，期望有像素", pt[0], pt[1])
		}
	}
}

// TestSVGPathCloseUsesOwnSubpathStart: "z" closes the subpath it belongs to,
// not the first point of the whole "d". Closing to the path-wide first point
// drew a long diagonal from (18,18) back to (2,2).
func TestSVGPathCloseUsesOwnSubpathStart(t *testing.T) {
	canvas := renderSVGPath(t, 20, 20, "M2 2h6v6zM12 12h6v6z",
		map[string]string{"stroke-width": "1.5"})
	defer canvas.Release()

	if px := canvas.PixelAt(10, 10); px.A != 0 {
		t.Fatalf("(10,10) = %+v, 期望透明（z 闭合到了整条 path 的首点）", px)
	}
	// Both triangles' own closing edges must be there.
	if px := canvas.PixelAt(5, 5); px.A == 0 {
		t.Fatal("第一个三角形的斜边 (5,5) 缺失")
	}
	if px := canvas.PixelAt(15, 15); px.A == 0 {
		t.Fatal("第二个三角形的斜边 (15,15) 缺失")
	}
}

// TestSVGStrokeLineJoinIsHonoured: stroke-linejoin must reach the rasterizer.
// Shape: an L (M4 12H16V22) with stroke-width 8 → the outer corner sits in the
// square x∈[16,20], y∈[8,12]. A miter join fills the sharp corner (so (19,8)
// is painted); round/bevel do not reach it.
func TestSVGStrokeLineJoinIsHonoured(t *testing.T) {
	const d = "M4 12H16V22"
	probe := [2]int{19, 8}

	miter := renderSVGPath(t, 24, 24, d, map[string]string{"stroke-width": "8", "stroke-linejoin": "miter"})
	defer miter.Release()
	roundJoin := renderSVGPath(t, 24, 24, d, map[string]string{"stroke-width": "8", "stroke-linejoin": "round"})
	defer roundJoin.Release()
	bevel := renderSVGPath(t, 24, 24, d, map[string]string{"stroke-width": "8", "stroke-linejoin": "bevel"})
	defer bevel.Release()

	pm := miter.PixelAt(probe[0], probe[1])
	pr := roundJoin.PixelAt(probe[0], probe[1])
	pb := bevel.PixelAt(probe[0], probe[1])
	t.Logf("外角 (%d,%d): miter=%+v round=%+v bevel=%+v", probe[0], probe[1], pm, pr, pb)

	if pm.A == 0 {
		t.Fatalf("miter 外角 (%d,%d) 应有像素，实际透明", probe[0], probe[1])
	}
	if pr.A != 0 {
		t.Errorf("round 外角 (%d,%d) 不应有像素（圆角切掉了尖角），实际 = %+v —— stroke-linejoin=round 未生效", probe[0], probe[1], pr)
	}
	if pb.A != 0 {
		t.Errorf("bevel 外角 (%d,%d) 不应有像素（斜切），实际 = %+v —— stroke-linejoin=bevel 未生效", probe[0], probe[1], pb)
	}
}

// TestSVGStrokeLineJoinInheritedFromRoot: stroke-linejoin declared once on the
// <svg> root must be inherited by child <path> elements, the way stroke /
// stroke-width / fill already are. Icon sprites typically declare the join on
// the root <svg>, so a broken inheritance would silently fall back to miter.
func TestSVGStrokeLineJoinInheritedFromRoot(t *testing.T) {
	const d = "M4 12H16V22"
	probe := [2]int{19, 8}

	rootRound := renderSVGPathFull(t, 24, 24, d,
		map[string]string{"stroke-width": "8"},
		map[string]string{"stroke-linejoin": "round"})
	defer rootRound.Release()
	rootMiter := renderSVGPathFull(t, 24, 24, d,
		map[string]string{"stroke-width": "8"},
		map[string]string{"stroke-linejoin": "miter"})
	defer rootMiter.Release()

	pr := rootRound.PixelAt(probe[0], probe[1])
	pm := rootMiter.PixelAt(probe[0], probe[1])
	t.Logf("根继承 外角 (%d,%d): round=%+v miter=%+v", probe[0], probe[1], pr, pm)
	if pm.A == 0 {
		t.Fatal("miter（设在 <svg> 根）外角应有像素，实际透明")
	}
	if pr.A != 0 {
		t.Errorf("round（设在 <svg> 根）外角不应有像素，实际 = %+v —— stroke-linejoin 未从 <svg> 根继承", pr)
	}
}

// TestSVGStrokeDashArrayInheritedFromRoot: stroke-dasharray is an inheritable
// presentation attribute, so declaring it once on the <svg> root must dash the
// child <path> strokes.
func TestSVGStrokeDashArrayInheritedFromRoot(t *testing.T) {
	canvas := renderSVGPathFull(t, 48, 24, "M2 12H46",
		map[string]string{"stroke-width": "4"},
		map[string]string{"stroke-dasharray": "6 6"})
	defer canvas.Release()

	// dash 6 / gap 6 measured from the path start (x=2): painted 2..8,
	// blank 8..14, painted 14..20, blank 20..26, painted 26..32 ...
	if px := canvas.PixelAt(5, 12); px.A == 0 {
		t.Error("(5,12) 应有像素（第 1 段虚线）")
	}
	if px := canvas.PixelAt(11, 12); px.A != 0 {
		t.Errorf("(11,12) = %+v，期望透明（虚线间隙）—— stroke-dasharray 未从 <svg> 根继承", px)
	}
	if px := canvas.PixelAt(17, 12); px.A == 0 {
		t.Error("(17,12) 应有像素（第 3 段虚线）")
	}
}