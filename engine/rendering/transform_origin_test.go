package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestTransformOriginDefault: CSS 规范默认 transform-origin 为 50% 50%
// （盒子中心）。此前 DefaultNonInheritedData 未设默认值 → Length{}（Unit=""）
// → resolveTransformOrigin 返回 -1 → rotate 绕盒子左上角旋转 → 图形"飘"右上角。
//
// 验证：40x40 红方块放 (20,20)，transform:rotate(45deg) 且不写 transform-origin。
// 修复后绕中心 (40,40) 旋转 → 中心不动、四角在菱形顶点 (40,11.7)/(11.7,40)/
// (68.3,40)/(40,68.3)，原左上角 (20,20) 区域变背景。
func TestTransformOriginDefault(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#1c2438; }
.box { position:absolute; left:20px; top:20px; width:40px; height:40px; background:#ff0000; transform:rotate(45deg); }
`)
	htmlEl.AppendChild(styleEl)

	box := dom.NewElement(doc, "div")
	box.SetClassName("box")
	bodyEl.AppendChild(box)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(90, 90)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(90, 90)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 90, Height: 90})

	savePNG(canvas, "transform_origin_test.png")

	red := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		return int(c.R) > 150 && int(c.G) < 80 && int(c.B) < 80
	}
	dark := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		return int(c.R) < 60 && int(c.G) < 80 && int(c.B) < 110
	}

	checks := []struct {
		name string
		x, y int
		want bool // true=红(方块), false=背景
	}{
		{"中心不动(40,40)", 40, 40, true},
		{"顶角(40,12)", 40, 12, true},
		{"左角(12,40)", 12, 40, true},
		{"右边界内侧(66,40)", 66, 40, true},
		{"底边界内侧(40,66)", 40, 66, true},
		{"菱形边中点(54,26)", 54, 26, true},
		{"原左上角(20,20)已移走", 20, 20, false},
		{"原右上角(60,20)已移走", 60, 20, false},
	}
	for _, c := range checks {
		if c.want {
			if !red(c.x, c.y) {
				t.Errorf("%s (%d,%d): 期望红色(方块), 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
			}
		} else if !dark(c.x, c.y) {
			t.Errorf("%s (%d,%d): 期望背景深色, 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
		}
	}
	t.Log("默认 transform-origin 50% 50% 验证完成")
}
