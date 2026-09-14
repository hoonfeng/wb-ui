package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestShapeIcon: 自定义图部件图标（.ic-shape）transform:rotate(45deg) 菱形
// 设计在 transform-origin 默认值（50% 50%）修复后绕中心正确旋转。
//
// 几何：.ic-shape absolute (10,10)，border-box 14x14（border 2）→ 轮廓
// (11,11)-(23,23)。rotate 45° 绕中心 (17,17) → 菱形顶点
// (17,8.5)/(8.5,17)/(25.5,17)/(17,25.5)，边在对角方向。
// sq 6x6 absolute left:2 top:2（相对 padding box (12,12)）→ (14,14)-(20,20)
// 中心 (17,17)，旋转后小菱形中心仍 (17,17)（与外框同心）。
func TestShapeIcon(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#1c2438; }
.ic-shape { position:absolute; left:10px; top:10px; border:2px solid #5d8df0; width:14px; height:14px; border-radius:2px; transform:rotate(45deg); }
.ic-shape .sq { position:absolute; left:2px; top:2px; width:6px; height:6px; background:#5d8df0; }
`)
	htmlEl.AppendChild(styleEl)

	shape := dom.NewElement(doc, "div")
	shape.SetClassName("ic-shape")
	bodyEl.AppendChild(shape)
	sq := dom.NewElement(doc, "div")
	sq.SetClassName("sq")
	shape.AppendChild(sq)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(48, 48)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(48, 48)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 48, Height: 48})

	savePNG(canvas, "F:\\syproject\\直播挂件助手\\screenshots\\shape_icon_rotate_test.png")

	blue := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		// 菱形斜边 AA 半覆盖：alpha 足够且偏蓝即算有内容
		return int(c.A) > 80 && int(c.B) > int(c.R)+40
	}
	notBlue := func(x, y int) bool {
		return !blue(x, y)
	}

	checks := []struct {
		name string
		x, y int
		want bool // true=蓝, false=背景
	}{
		{"sq中心(17,17)", 17, 17, true},
		{"外框右上边中点(21,13)", 21, 13, true},
		{"外框左上边中点(13,13)", 13, 13, true},
		{"外框右下边中点(21,21)", 21, 21, true},
		{"外框左下边中点(13,21)", 13, 21, true},
		{"sq菱形边(19,15)", 19, 15, true},
		{"外框内-sq外(14,14)", 14, 14, false},
		{"原轮廓左上角(11,11)旋转移走", 11, 11, false},
		{"菱形顶角上方(17,5)", 17, 5, false},
	}
	for _, c := range checks {
		if c.want {
			if !blue(c.x, c.y) {
				t.Errorf("%s (%d,%d): 期望蓝色, 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
			}
		} else if !notBlue(c.x, c.y) {
			t.Errorf("%s (%d,%d): 期望非蓝(背景/透明), 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
		}
	}
	t.Log("shape 图标 rotate 菱形居中验证完成")
}
