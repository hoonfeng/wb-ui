package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestShapeIcon: 自定义图部件图标（.ic-shape）去掉 transform:rotate 后，
// 外框 20x20(border2) 与内部 sq 10x10 精确居中。
// 容器 .ic 24x24 放在 (10,10)，.ic-shape margin-top:2 → (12,12)-(32,32)，
// sq left:3/top:3 相对 padding box → 绝对 (17,17)-(27,27)，中心 (22,22)=外框中心。
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
.ic { position:absolute; left:10px; top:10px; width:24px; height:24px; }
.ic-shape { border:2px solid #5d8df0; width:20px; height:20px; margin-top:2px; border-radius:3px; }
.ic-shape .sq { position:absolute; left:3px; top:3px; width:10px; height:10px; background:#5d8df0; border-radius:2px; }
`)
	htmlEl.AppendChild(styleEl)

	shape := dom.NewElement(doc, "div")
	shape.SetClassName("ic ic-shape")
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

	savePNG(canvas, "F:\\syproject\\直播挂件助手\\screenshots\\shape_icon_test.png")

	blue := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		return int(c.R) > 60 && int(c.G) > 110 && int(c.B) > 200
	}
	dark := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		return int(c.R) < 60 && int(c.G) < 80 && int(c.B) < 110
	}

	checks := []struct {
		name string
		x, y int
		want bool // true=蓝, false=背景
	}{
		{"外框顶边", 22, 13, true},
		{"外框左边", 11, 22, true},
		{"外框右边", 29, 22, true},
		{"外框底边", 22, 31, true},
		{"sq中心", 22, 22, true},
		{"sq外-外框内(左)", 14, 22, false},
		{"sq外-外框内(上)", 20, 15, false},
		{"外框外(右下)", 34, 34, false},
	}
	for _, c := range checks {
		got := blue(c.x, c.y)
		if c.want && !got {
			t.Errorf("%s (%d,%d): 期望蓝色, 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
		}
		if !c.want && !dark(c.x, c.y) {
			t.Errorf("%s (%d,%d): 期望背景深色, 实际 %v", c.name, c.x, c.y, canvas.PixelAt(c.x, c.y))
		}
	}
	t.Log("shape 图标渲染检查完成")
}
