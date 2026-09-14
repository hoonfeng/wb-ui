package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
)

// 复现配置器部件图标 .ic-video .tri（CSS 三角形技巧）的绝对定位几何：
//   .ic{position:relative;display:inline-block;width:24px;height:24px}
//   .ic-video{width:20px;height:20px;border:2px solid #5d8df0;margin-top:2px}
//   .ic-video .tri{position:absolute;left:7px;top:5px;width:0;height:0;
//                  border-left:8px solid #5d8df0;
//                  border-top:5px solid transparent;border-bottom:5px solid transparent}
// 验证 .tri 相对 .ic-video padding box 定位（left:7/top:5 语义）。
func TestTriIconAbsoluteGeom(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)
	card := dom.NewElement(doc, "div")
	card.SetClassName("card")
	card.SetAttribute("style", "width:108px; height:79px; padding:10px 8px; text-align:center;")
	bodyEl.AppendChild(card)
	picon := dom.NewElement(doc, "div")
	picon.SetClassName("picon")
	card.AppendChild(picon)
	ic := dom.NewElement(doc, "div")
	ic.SetClassName("ic ic-video")
	picon.AppendChild(ic)
	tri := dom.NewElement(doc, "div")
	tri.SetClassName("tri")
	ic.AppendChild(tri)

	css := `
* { margin: 0; padding: 0; box-sizing: border-box; }
.card { width: 108px; height: 79px; padding: 10px 8px; text-align: center; }
.picon { width: 26px; height: 26px; margin: 0 auto 4px; position: relative; }
.ic { position: relative; display: inline-block; width: 24px; height: 24px; }
.ic-video { width: 20px; height: 20px; border: 2px solid #5d8df0; border-radius: 3px; margin-top: 2px; }
.ic-video .tri { position: absolute; left: 7px; top: 5px; width: 0; height: 0;
                 border-left: 8px solid #5d8df0;
                 border-top: 5px solid transparent;
                 border-bottom: 5px solid transparent; }
`
	rv := buildAndLayout(doc, css, 200, 120)
	dumpLayoutTree(rv, 0, t)

	findByClassContains := func(root RenderObject, cls string) RenderObject {
		var result RenderObject
		var walk func(ro RenderObject)
		walk = func(ro RenderObject) {
			if result != nil {
				return
			}
			if el, ok := ro.Node().(*dom.Element); ok {
				if el.GetClassName() == cls || strings.Contains(" "+el.GetClassName()+" ", " "+cls+" ") {
					result = ro
					return
				}
			}
			for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
			}
		}
		walk(root)
		return result
	}
	icBox := findByClassContains(rv, "ic-video")
	triBox := findByClassContains(rv, "tri")
	if icBox == nil || triBox == nil {
		t.Fatal("找不到 ic-video / tri")
	}
	ix, iy, iw, ih := layoutInfo(icBox)
	tx, ty, tw, th := layoutInfo(triBox)
	t.Logf("ic-video: x=%.1f y=%.1f w=%.1f h=%.1f", ix, iy, iw, ih)
	t.Logf("tri:      x=%.1f y=%.1f w=%.1f h=%.1f", tx, ty, tw, th)

	if lb := triBox.LayoutBox(); lb != nil {
		if g := rv.LayoutState().GeometryForBox(lb); g != nil {
			t.Logf("tri geom: borderTop=%.1f borderBottom=%.1f borderLeft=%.1f contentH=%.1f",
				g.BorderTop(), g.BorderBottom(), g.BorderLeft(), g.ContentHeight())
		}
		cs := lb.Style()
		if cs != nil {
			t.Logf("tri style: BorderTopW=%v BorderBottomW=%v BorderLeftW=%v",
				cs.BorderTopWidth, cs.BorderBottomWidth, cs.BorderLeftWidth)
		}
	}

	// 期望：tri 相对 ic-video padding box（左 = ic 左 + border 2）定位。
	// left:7 → tri.x = ix + 2 + 7；top:5 → tri.y = iy + 2 + 5。
	expX := ix + 2 + 7
	expY := iy + 2 + 5
	if tx != expX || ty != expY {
		t.Errorf("tri 位置 (%v,%v), 期望 (%v,%v)（relative padding box + left/top）", tx, ty, expX, expY)
	}
	// 期望：tri 尺寸 8x10（border-left 8 + top/bottom 5）
	if tw != 8 || th != 10 {
		t.Errorf("tri 尺寸 %vx%v, 期望 8x10", tw, th)
	}
}
