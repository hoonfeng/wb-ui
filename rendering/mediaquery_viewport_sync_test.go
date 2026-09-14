// 尺寸媒体查询（@media (min-width/max-width/min-height/max-height)）必须按真实
// 视口求值：style.Resolver 的媒体查询上下文曾是零值 0×0，于是 min-* 恒不匹配、
// max-* 恒匹配——所有尺寸媒体查询都落在错误分支上（`@media (min-width:600px)`
// 的页面永远看到桌面样式，`@media (max-width:950px)` 的页面永远看到移动样式）。
//
// 本测试是 dev/suites/cssprobe/fixtures/viewport-consistency.html 前两项检查的单测
// 对应物（第三项要页面脚本读 innerWidth/visualViewport，探针与单测都不执行脚本）。
// 同一页面用两个视口渲染形成交叉验证：既验证命中，也验证不命中。
package rendering_test

import (
	"testing"

	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

// mediaViewportHTML：两个 200x80 探针，默认红 #c92a2a；
// #width-probe 在 max-width:950px 命中时转绿 #087f5b，
// #height-probe 在 min-height:900px 命中时转蓝 #1971c2。
const mediaViewportHTML = `<!doctype html>
<style>
  html, body { margin: 0; }
  .probe { position: absolute; left: 10px; width: 200px; height: 80px; background: #c92a2a; }
  #width-probe { top: 10px; }
  #height-probe { top: 110px; }
  @media (max-width: 950px) { #width-probe { background: #087f5b; } }
  @media (min-height: 900px) { #height-probe { background: #1971c2; } }
</style>
<div id="width-probe" class="probe"></div>
<div id="height-probe" class="probe"></div>`

// renderMediaViewport 走与 dev/suites/cssprobe 相同的链路：视口尺寸在 Build（首次
// 样式解析）之前写进 resolver，Build 后再由 RenderView 同步一次。真实链路中
// 前者是 page.Frame.syncMediaQueryViewport（渲染树重建前调用），后者是
// RenderView.SetViewportSize。
func renderMediaViewport(t *testing.T, w, h int) *graphics.Canvas {
	t.Helper()
	doc, err := html.Parse(mediaViewportHTML)
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	resolver.SetViewportSize(w, h)
	if root := doc.DocumentElement(); root != nil {
		extractStylesTest(root, resolver)
	}
	rv := rendering.NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("render tree build failed")
	}
	rv.SetResolver(resolver)
	rv.SetViewportSize(float64(w), float64(h))
	rv.Layout(layout.NewLayoutState(float64(w), float64(h)))

	canvas := graphics.NewCanvas(w, h)
	t.Cleanup(canvas.Release)
	canvas.Clear(graphics.Color{R: 255, G: 255, B: 255, A: 255})
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})
	return canvas
}

func TestMediaQueryViewportSync(t *testing.T) {
	red := graphics.Color{R: 0xc9, G: 0x2a, B: 0x2a, A: 0xff}
	green := graphics.Color{R: 0x08, G: 0x7f, B: 0x5b, A: 0xff}
	blue := graphics.Color{R: 0x19, G: 0x71, B: 0xc2, A: 0xff}

	cases := []struct {
		name                  string
		w, h                  int
		wantWidth, wantHeight graphics.Color
	}{
		{"900x1000 matches max-width:950 and min-height:900", 900, 1000, green, blue},
		{"1200x800 matches neither", 1200, 800, red, red},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canvas := renderMediaViewport(t, tc.w, tc.h)
			// 探针中心：(10+100, 10+40) = (110,50)；(10+100, 110+40) = (110,150)
			if got := canvas.PixelAt(110, 50); got != tc.wantWidth {
				t.Errorf("@media (max-width:950px) at %dx%d: #width-probe = %+v, want %+v",
					tc.w, tc.h, got, tc.wantWidth)
			}
			if got := canvas.PixelAt(110, 150); got != tc.wantHeight {
				t.Errorf("@media (min-height:900px) at %dx%d: #height-probe = %+v, want %+v",
					tc.w, tc.h, got, tc.wantHeight)
			}
		})
	}

	// 反向对照：不同步视口时媒体上下文是零值 0×0 —— max-width:950px 恒命中
	// （0 ≤ 950）、min-height:900px 恒不匹配（0 < 900），两个探针的颜色组合与
	// 上面两种视口都不同。这证明上面的断言确实由「视口同步」驱动：一旦同步
	// 链路失效，本子测试会立即捕捉到（而不是上面两项恰好也通过）。
	t.Run("no sync leaves the media context at 0x0", func(t *testing.T) {
		canvas := renderMediaViewportNoSync(t)
		if got := canvas.PixelAt(110, 50); got != green {
			t.Errorf("without viewport sync: #width-probe = %+v, want %+v (0 <= 950 matches)",
				got, green)
		}
		if got := canvas.PixelAt(110, 150); got != red {
			t.Errorf("without viewport sync: #height-probe = %+v, want %+v (0 < 900 does not match)",
				got, red)
		}
	})
}

// renderMediaViewportNoSync 渲染同一页面，但**不**把视口尺寸同步给样式解析器
// （resolver 不预设、RenderView 不设尺寸），复现同步前媒体上下文为 0×0 的状态。
// 画布仍取 900x1000，保证两个探针落在可视区域内可读像素。
func renderMediaViewportNoSync(t *testing.T) *graphics.Canvas {
	t.Helper()
	doc, err := html.Parse(mediaViewportHTML)
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	if root := doc.DocumentElement(); root != nil {
		extractStylesTest(root, resolver)
	}
	rv := rendering.NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("render tree build failed")
	}
	rv.SetResolver(resolver)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(900, 1000)
	t.Cleanup(canvas.Release)
	canvas.Clear(graphics.Color{R: 255, G: 255, B: 255, A: 255})
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 900, Height: 1000})
	return canvas
}
