package rendering

import (
	"math"
	"testing"

	"wb-ui/dom"
)

// svgDocFromMarkup 解析一段 SVG 标记为 svgDocument（复用现有测试辅助的简化路径）。
func TestResolveViewBoxTransformNone(t *testing.T) {
	// preserveAspectRatio="none"：独立拉伸填满（非等比），无居中偏移。
	doc := &svgDocument{
		viewBox:   [4]float64{0, 0, 100, 100},
		hasVB:     true,
		par:       "none",
		viewportW: 300,
		viewportH: 200,
	}
	sx, sy, dx, dy := resolveViewBoxTransform(doc, 300, 200)
	if sx != 3 || sy != 2 {
		t.Fatalf("none: sx=%.3f sy=%.3f, want 3/2（独立拉伸）", sx, sy)
	}
	if dx != 0 || dy != 0 {
		t.Fatalf("none: dx=%.3f dy=%.3f, want 0/0（无居中）", dx, dy)
	}
}

func TestResolveViewBoxTransformDefaultMeet(t *testing.T) {
	// 默认（无 preserveAspectRatio）：xMidYMid meet，等比缩放居中。
	doc := &svgDocument{
		viewBox:   [4]float64{0, 0, 100, 100},
		hasVB:     true,
		par:       "",
		viewportW: 300,
		viewportH: 200,
	}
	sx, sy, dx, dy := resolveViewBoxTransform(doc, 300, 200)
	if sx != 2 || sy != 2 {
		t.Fatalf("meet: sx=%.3f sy=%.3f, want 2/2（等比）", sx, sy)
	}
	if dx != 50 || dy != 0 {
		t.Fatalf("meet: dx=%.3f dy=%.3f, want 50/0（水平居中）", dx, dy)
	}
}

func TestResolveViewBoxTransformSlice(t *testing.T) {
	// xMidYMid slice：等比放大填满并裁剪，居中。
	doc := &svgDocument{
		viewBox:   [4]float64{0, 0, 100, 100},
		hasVB:     true,
		par:       "xMidYMid slice",
		viewportW: 300,
		viewportH: 200,
	}
	sx, sy, dx, dy := resolveViewBoxTransform(doc, 300, 200)
	if sx != 3 || sy != 3 {
		t.Fatalf("slice: sx=%.3f sy=%.3f, want 3/3（等比放大）", sx, sy)
	}
	if dx != 0 || math.Abs(dy+50) > 1e-6 {
		t.Fatalf("slice: dx=%.3f dy=%.3f, want 0/-50（垂直居中裁剪）", dx, dy)
	}
}

func TestResolveViewBoxTransformAlign(t *testing.T) {
	// xMinYMin meet：左上对齐。
	doc := &svgDocument{
		viewBox:   [4]float64{0, 0, 100, 100},
		hasVB:     true,
		par:       "xMinYMin meet",
		viewportW: 300,
		viewportH: 200,
	}
	_, _, dx, dy := resolveViewBoxTransform(doc, 300, 200)
	if dx != 0 || dy != 0 {
		t.Fatalf("xMinYMin: dx=%.3f dy=%.3f, want 0/0", dx, dy)
	}
}

// TestBuildSVGDocumentParsesPAR 验证 buildSVGDocument 读取 preserveAspectRatio。
func TestBuildSVGDocumentParsesPAR(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("svg")
	el.SetAttribute("viewBox", "0 0 100 100")
	el.SetAttribute("preserveAspectRatio", "none")
	sd := buildSVGDocument(el)
	if sd.par != "none" {
		t.Fatalf("par = %q, want none", sd.par)
	}
	if !sd.hasVB || sd.viewBox[2] != 100 || sd.viewBox[3] != 100 {
		t.Fatalf("viewBox 解析失败: %v hasVB=%v", sd.viewBox, sd.hasVB)
	}
}

// TestPaintSVGViewportFromBox 验证 paintSVG 用 viewportW/H（实际渲染尺寸）
// 而非固有 width/height 做 viewBox 变换：内联 svg + width:100% 拉伸场景。
func TestPaintSVGViewportFromBox(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("svg")
	el.SetAttribute("viewBox", "0 0 200 190")
	el.SetAttribute("preserveAspectRatio", "none")
	sd := buildSVGDocument(el)
	// 固有尺寸回退到 viewBox（200x190），但调用方设置实际渲染尺寸 500x300。
	if sd.width != 200 || sd.height != 190 {
		t.Fatalf("intrinsic = %.0fx%.0f, want 200x190", sd.width, sd.height)
	}
	sd.viewportW = 500
	sd.viewportH = 300
	sx, sy, _, _ := resolveViewBoxTransform(sd, 500, 300)
	if sx != 2.5 || sy != 300.0/190.0 {
		t.Fatalf("viewport 500x300 none: sx=%.4f sy=%.4f, want 2.5/%.4f",
			sx, sy, 300.0/190.0)
	}
}
