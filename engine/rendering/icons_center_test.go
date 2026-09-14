package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestIconsCenter: 复现配置窗口部件卡片图标内部元素居中。
// 验证 text/list/image/camera 四图标内部元素在 border-box 语义下
// 的精确位置（*{box-sizing:border-box}，absolute 子元素相对 padding box）。
func TestIconsCenter(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#1c2438; }
.ic { position:absolute; width:24px; height:24px; }
/* text：三横线在 20x20 内容区精确居中（left:1 / 3.5 → 中心 10） */
.ic-text{left:10px;top:10px;width:20px;height:20px}
.ic-text .i1,.ic-text .i2,.ic-text .i3{position:absolute;left:1px;width:18px;height:2px;background:#5d8df0}
.ic-text .i1{top:3px}.ic-text .i2{top:9px;left:3.5px;width:13px}.ic-text .i3{top:15px}
/* list：三道杠在 16x16 padding box 居中（left:2/top:1,7,13 → 中心 8） */
.ic-list{left:50px;top:10px;width:20px;height:20px;border:2px solid #5d8df0;border-radius:3px}
.ic-list .i1,.ic-list .i2,.ic-list .i3{position:absolute;left:2px;width:12px;height:2px;background:#5d8df0}
.ic-list .i1{top:1px}.ic-list .i2{top:7px}.ic-list .i3{top:13px}
/* image：sun 左上装饰 + mnt 山形斜线（rotate 由引擎正确渲染） */
.ic-image{left:90px;top:10px;width:20px;height:20px;border:2px solid #5d8df0;border-radius:3px}
.ic-image .sun{position:absolute;left:3px;top:3px;width:4px;height:4px;border-radius:50%;background:#5d8df0}
.ic-image .mnt{position:absolute;left:4px;top:11px;width:8px;height:2px;background:#5d8df0;transform:rotate(45deg)}
/* camera：lens 水平居中（left:5 → 中心 8 = padding 中心） */
.ic-camera{left:130px;top:12px;width:20px;height:15px;border:2px solid #5d8df0;border-radius:3px}
.ic-camera .lens{position:absolute;left:5px;top:3px;width:6px;height:6px;border:2px solid #5d8df0;border-radius:50%}
`)
	htmlEl.AppendChild(styleEl)

	add := func(cls string, children ...string) {
		el := dom.NewElement(doc, "div")
		el.SetClassName(cls)
		bodyEl.AppendChild(el)
		for _, c := range children {
			ch := dom.NewElement(doc, "div")
			ch.SetClassName(c)
			el.AppendChild(ch)
		}
	}
	add("ic ic-text", "i1", "i2", "i3")
	add("ic ic-list", "i1", "i2", "i3")
	add("ic ic-image", "sun", "mnt")
	add("ic ic-camera", "lens")

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(170, 40)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(170, 40)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 170, Height: 40})

	savePNG(canvas, "F:\\syproject\\直播挂件助手\\screenshots\\icons_center_test.png")

	blue := func(x, y int) bool {
		c := canvas.PixelAt(x, y)
		return int(c.A) > 80 && int(c.B) > int(c.R)+40
	}

	// image 图标：sun 圆 (95,15)-(99,19) 中心 (97,17) 蓝；
	// mnt 斜线：8x2 水平线 (96,23)-(104,25) 绕中心 (100,24) rotate45° →
	// 斜线 (97.88,20.46)-(102.12,27.54)，中心 (100,24) 不动，原端点转走。
	checks := []struct {
		name string
		x, y int
		want bool
	}{
		{"sun 中心(97,17)", 97, 17, true},
		{"mnt 斜线中心(100,24)", 100, 24, true},
		{"mnt 斜线上端(98,21)", 98, 21, true},
		{"mnt 斜线中下(101,26)", 101, 26, true},
		{"mnt 原水平端点(96,23)已转走", 96, 23, false},
		{"mnt 原水平端点(104,25)已转走", 104, 25, false},
	}
	for _, c := range checks {
		got := blue(c.x, c.y)
		if got != c.want {
			t.Errorf("%s (%d,%d): 期望蓝=%v, 实际 %v", c.name, c.x, c.y, c.want, canvas.PixelAt(c.x, c.y))
		}
	}
	t.Log("image 图标 sun+mnt(rotate) 像素验证通过")
}
