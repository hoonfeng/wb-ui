package rendering

// AboutModal logo 的 SVG「圆角方块」回归测试。
//
// 缺陷：`svgFilledShape.paint` 的渐变填充分支在画出渐变后【直接 return】，
// 跳过了随后的描边绘制 ⇒「渐变填充 + 描边」的图形整条描边消失；同时
// `paintShapeGradient` 按直角矩形铺渐变（不理会 rx/ry）。两者叠加使
// logo 的 `<rect rx='96' ry='96' fill='url(#bgGrad)' stroke='#1a3a4a'
// stroke-width='2'>` 只剩一层与弹窗底色几乎同色的渐变——用户可见现象为
// 「logo 没有圆角」（圆角轮廓那 2px 描边完全不画，渐变色还溢出到圆角外）。
//
// 修复：渐变分支不再 return（改为「渐变替代 fill + 立即还原变换」，描边照画），
// 且 rx/ry>0 时先设圆角裁剪再铺渐变。

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// buildGradientRoundRectSVG 构造与 AboutModal logo 同构的最小 SVG：
// 圆角矩形 + 渐变填充 + 实色描边（缩放为 100x100 便于逐像素断言）。
func buildGradientRoundRectSVG(t *testing.T) *svgDocument {
	t.Helper()
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "100")

	defs := doc.CreateElement("defs")
	grad := doc.CreateElement("linearGradient")
	grad.SetAttribute("id", "bgGrad")
	grad.SetAttribute("x1", "0")
	grad.SetAttribute("y1", "0")
	grad.SetAttribute("x2", "1")
	grad.SetAttribute("y2", "1")
	s1 := doc.CreateElement("stop")
	s1.SetAttribute("offset", "0%")
	s1.SetAttribute("stop-color", "#0000ff")
	s2 := doc.CreateElement("stop")
	s2.SetAttribute("offset", "100%")
	s2.SetAttribute("stop-color", "#00ff00")
	grad.AppendChild(s1)
	grad.AppendChild(s2)
	defs.AppendChild(grad)

	rect := doc.CreateElement("rect")
	rect.SetAttribute("x", "10")
	rect.SetAttribute("y", "10")
	rect.SetAttribute("width", "80")
	rect.SetAttribute("height", "80")
	rect.SetAttribute("rx", "30")
	rect.SetAttribute("ry", "30")
	rect.SetAttribute("fill", "url(#bgGrad)")
	rect.SetAttribute("stroke", "#ff0000")
	rect.SetAttribute("stroke-width", "2")

	svgEl.AppendChild(defs)
	svgEl.AppendChild(rect)
	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	return sd
}

func TestSVGGradientRectKeepsRoundedCornersAndStroke(t *testing.T) {
	sd := buildGradientRoundRectSVG(t)
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// ① 中心：渐变填充已铺（不透明，且是蓝→绿渐变中的一色）。
	if px := canvas.PixelAt(50, 50); px.A == 0 {
		t.Fatalf("center (50,50) = %+v, want opaque gradient fill", px)
	}
	// ② 左上角圆角【之外】(12,12)：必须透明。
	//    rx=30、rect(10,10)-(90,90) ⇒ 左上角弧心 (40,40)，(12,12) 距弧心 39.6 > 30。
	//    修复前渐变按直角矩形铺色，(12,12) 是不透明的渐变。
	if px := canvas.PixelAt(12, 12); px.A != 0 {
		t.Fatalf("corner (12,12) = %+v, want transparent (渐变须被 rx 圆角裁剪)", px)
	}
	// ③ 上边中点 (50,10)：描边必须可见（红）。修复前渐变分支 return 跳过描边。
	if px := canvas.PixelAt(50, 10); !(px.R > 200 && px.G < 80 && px.B < 80) {
		t.Fatalf("top edge (50,10) = %+v, want red stroke (渐变分支不得跳过描边)", px)
	}
}
