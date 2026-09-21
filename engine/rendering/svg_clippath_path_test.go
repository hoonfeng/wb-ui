// SVG clipPath 的 path 子元素回归测试。
//
// 回归要点：clipShapesToPath 曾只认 rect/circle/ellipse/polygon，*svgPath 落入
// default 被静默忽略 → `<clipPath><path d="…"/></clipPath>` 得到一个【空】Skia
// path，ClipPath(空) 把引用元素整块裁没（元素凭空消失）。修复后按
// sampleSegments 的每段子路径折算线并入同一 path。

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// buildClipPathDoc 构造：48×48 的 svg，clipPath#c 由 clipD 描述，一个铺满的
// 黑色 rect 通过 clip-path="url(#c)" 引用它。
func buildClipPathDoc(t *testing.T, clipD string) *graphics.Canvas {
	t.Helper()
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "48")
	svgEl.SetAttribute("height", "48")
	svgEl.SetAttribute("viewBox", "0 0 48 48")

	defs := doc.CreateElement("defs")
	cp := doc.CreateElement("clipPath")
	cp.SetAttribute("id", "c")
	p := doc.CreateElement("path")
	p.SetAttribute("d", clipD)
	cp.AppendChild(p)
	defs.AppendChild(cp)
	svgEl.AppendChild(defs)

	r := doc.CreateElement("rect")
	r.SetAttribute("x", "0")
	r.SetAttribute("y", "0")
	r.SetAttribute("width", "48")
	r.SetAttribute("height", "48")
	r.SetAttribute("fill", "black")
	r.SetAttribute("clip-path", "url(#c)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	canvas := graphics.NewCanvas(48, 48)
	paintSVGTo(canvas, sd, 0, 0, 48, 48, graphics.Color{})
	return canvas
}

// TestSVGClipPathWithPathShape：单子路径方块的 clipPath——方块内必须可见，
// 方块外必须透明。修复前整块内容被空 clip 裁没（两处断言都透明）。
func TestSVGClipPathWithPathShape(t *testing.T) {
	canvas := buildClipPathDoc(t, "M0 0 H24 V24 H0 Z")
	defer canvas.Release()

	if px := canvas.PixelAt(12, 12); px.A == 0 {
		t.Errorf("(12,12) 透明：path 型 clipPath 未生效（空 clip 把元素整块裁没）")
	}
	if px := canvas.PixelAt(36, 36); px.A != 0 {
		t.Errorf("(36,36) = %+v，期望透明（在 clipPath 方块之外）", px)
	}
}

// TestSVGClipPathWithMultiSubpathPath：M…zM…z 的两个方块——两处都应可见，
// 中间空隙必须透明（子路径各自独立，不能被连成一块）。
func TestSVGClipPathWithMultiSubpathPath(t *testing.T) {
	canvas := buildClipPathDoc(t, "M0 0 H20 V20 H0 Z M28 28 H48 V48 H28 Z")
	defer canvas.Release()

	if px := canvas.PixelAt(10, 10); px.A == 0 {
		t.Errorf("(10,10) 透明：clipPath 第一个子路径未生效")
	}
	if px := canvas.PixelAt(38, 38); px.A == 0 {
		t.Errorf("(38,38) 透明：clipPath 第二个子路径未生效（多子路径被当作一个子路径？）")
	}
	// 两个方块之间的空隙（x,y ∈ 20..28）既不在方块一也不在方块二里。
	if px := canvas.PixelAt(24, 24); px.A != 0 {
		t.Errorf("(24,24) = %+v，期望透明（两个 clip 子路径之间的空隙）", px)
	}
}

// TestSVGClipPathWithCurvedPath：Bezier 子路径（圆弧近似）——曲线内可见、
// 曲线外的角透明，验证折线近似没有退化成一个点或空 path。
func TestSVGClipPathWithCurvedPath(t *testing.T) {
	// 以 (24,24) 为心、半径 16 的圆（四段三次贝塞尔，k = 0.5523·r ≈ 8.84）。
	canvas := buildClipPathDoc(t,
		"M8 24 C8 15.16 15.16 8 24 8 C32.84 8 40 15.16 40 24 "+
			"C40 32.84 32.84 40 24 40 C15.16 40 8 32.84 8 24 Z")
	defer canvas.Release()

	if px := canvas.PixelAt(24, 24); px.A == 0 {
		t.Errorf("(24,24) 透明：曲线 clipPath 未生效")
	}
	if px := canvas.PixelAt(24, 12); px.A == 0 {
		t.Errorf("(24,12) 透明：曲线内部近顶部应可见（半径 16 的圆）")
	}
	// 圆心到 (2,2) 的距离 ≈ 31 > 16 → 圆外。
	if px := canvas.PixelAt(2, 2); px.A != 0 {
		t.Errorf("(2,2) = %+v，期望透明（圆外）", px)
	}
}
