// 复现 ConvSidebar.vue 的 ctx-bar 结构（用户 QQ20260804-193558 截图）：
//   .ctx-bar { height:12px; border:1px solid; border-radius:6px; overflow:hidden; background:#081919 }
//   .ctx-bar-fill { height:100%; width:3px; border-radius:6px; background:红 }
// 走真实布局（buildAndLayout）验证：
//   1. fill 顶部是否偏移 borderTop（应为 bar.top + 1）
//   2. fill 高度 100% 是否参照 content 高度（应为 10px）
//   3. 渲染后 fill 顶边 AA 是否与 bar 圆弧顶重叠
package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

func TestCtxBarRealLayout(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)
	wrap := dom.NewElement(doc, "div")
	wrap.SetAttribute("style", "width:240px; height:40px; padding:10px;")
	bodyEl.AppendChild(wrap)

	bar := dom.NewElement(doc, "div")
	bar.SetClassName("ctx-bar")
	wrap.AppendChild(bar)

	fill := dom.NewElement(doc, "div")
	fill.SetClassName("ctx-bar-fill")
	bar.AppendChild(fill)

	css := `
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: 100%; height: 100%; }
.ctx-bar {
  height: 12px;
  background: #081919;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #3B3B3B;
}
.ctx-bar-fill {
  height: 100%;
  width: 3px;
  border-radius: 6px;
  background: #c0303a;
}
`
	rv := buildAndLayout(doc, css, 240, 40)
	dumpLayoutTree(rv, 0, t)

	barBox := findByClass(rv, "ctx-bar")
	fillBox := findByClass(rv, "ctx-bar-fill")
	if barBox == nil || fillBox == nil {
		t.Fatal("找不到 ctx-bar / ctx-bar-fill")
	}
	bx, by, bw, bh := layoutInfo(barBox)
	fx, fy, fw, fh := layoutInfo(fillBox)
	t.Logf("ctx-bar: x=%.1f y=%.1f w=%.1f h=%.1f", bx, by, bw, bh)
	t.Logf("ctx-bar-fill: x=%.1f y=%.1f w=%.1f h=%.1f", fx, fy, fw, fh)

	// 期望：fill 顶部 = bar 顶部 + borderTop(1px)
	expectFillTop := by + 1
	if fy != expectFillTop {
		t.Errorf("fill 顶部 y=%.1f, 期望 %.1f (bar 顶部 + borderTop 1px) — 缺 borderTop 偏移！", fy, expectFillTop)
	}
	// 期望：fill 高度 100% = content 高 = 12 - 2*border = 10
	expectFillH := bh - 2
	if fh != expectFillH {
		t.Errorf("fill 高度 h=%.1f, 期望 %.1f (content 高)", fh, expectFillH)
	}

	// 渲染并检查 fill 顶边 AA 像素
	canvas := graphics.NewCanvas(240, 40)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 240, Height: 40})

	// fill 列（x = fx + 1 内容中）y 剖面
	t.Logf("--- fill 列 x=%.0f y 剖面 ---", fx)
	for y := int(by) - 2; y <= int(by)+int(bh)+2; y++ {
		p := canvas.PixelAt(int(fx)+1, y)
		if p.R > 0 || p.G > 0 || p.B > 0 {
			t.Logf("(%.0f,%d) = R%d G%d B%d", fx+1, y, p.R, p.G, p.B)
		}
	}
}
