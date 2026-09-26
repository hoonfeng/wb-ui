package webkit

import (
	"fmt"
	"os"
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
)

// idePageFixture 是真实 gou-ide 前端页面的「渲染后 DOM」快照（1613 元素
// / 346KB），由桌面壳 headless 执行 `document.documentElement.outerHTML`
// dump 得到。它是性能基准的输入：用真实规模 + 真实样式表测布局/绘制成本，
// 而不是用合成小页面自欺欺人。
const idePageFixture = "../dev/fixtures/perf/ide-page.html"

// viewportW/H 与真实壳一致（1600x1000 CSS px；本机 DPR 1.25 → 物理 2000x1250）。
const (
	perfViewportW = 1600
	perfViewportH = 1000
	perfPhysicalW = 2000
	perfPhysicalH = 1250
)

func newIDEPageWebView(tb testing.TB) *WebView {
	tb.Helper()
	src, err := os.ReadFile(idePageFixture)
	if err != nil {
		tb.Skipf("缺真实页面夹具 %s：%v", idePageFixture, err)
	}
	wv := NewWebView()
	wv.Resize(perfViewportW, perfViewportH)
	if err := wv.LoadHTML(string(src)); err != nil {
		tb.Fatalf("LoadHTML 失败：%v", err)
	}
	if v := wv.Page().MainFrame().View(); v != nil {
		v.SetNeedsLayout(true)
		v.Layout()
	}
	return wv
}

// --- 规模统计 -----------------------------------------------------------------

// TestIDEPageScale 打印真实页面的渲染树规模（节点/文本/层），作为所有
// 性能数字的分母——脱离规模谈耗时没有意义。
func TestIDEPageScale(t *testing.T) {
	wv := newIDEPageWebView(t)
	defer wv.Destroy()
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 为空")
	}
	objs, boxes, texts, inlines := 0, 0, 0, 0
	var walk func(o rendering.RenderObject, depth int)
	maxDepth := 0
	walk = func(o rendering.RenderObject, depth int) {
		if o == nil {
			return
		}
		objs++
		if depth > maxDepth {
			maxDepth = depth
		}
		switch o.(type) {
		case *rendering.RenderBox:
			boxes++
		case *rendering.RenderText:
			texts++
		case *rendering.RenderInline:
			inlines++
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, depth+1)
		}
	}
	walk(rv, 0)

	// 渲染层统计
	layers, composited := 0, 0
	var walkLayers func(l *rendering.RenderLayer)
	walkLayers = func(l *rendering.RenderLayer) {
		if l == nil {
			return
		}
		layers++
		if l.IsComposited() {
			composited++
		}
		for c := l.FirstChild(); c != nil; c = c.NextSibling() {
			walkLayers(c)
		}
	}
	if root := rv.RootLayer(); root != nil {
		walkLayers(root)
	}

	fmt.Printf("[scale] 渲染对象=%d box=%d text=%d inline=%d 深度=%d 层=%d(合成=%d)\n",
		objs, boxes, texts, inlines, maxDepth, layers, composited)
	if fv := wv.Page().MainFrame().View(); fv != nil {
		fmt.Printf("[scale] contentSize=%dx%d viewport=%dx%d\n",
			fv.ContentWidth(), fv.ContentHeight(), perfViewportW, perfViewportH)
	}
}

// --- 分环节基准 ---------------------------------------------------------------

// BenchmarkIDEPageLayout 全量布局一遍（FrameView.Layout 的 rv.Layout(nil) 路径）。
func BenchmarkIDEPageLayout(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	v := wv.Page().MainFrame().View()
	if v == nil {
		b.Fatal("FrameView 为空")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.SetNeedsLayout(true)
		v.Layout()
	}
}

// BenchmarkIDEPageRebuildTree 渲染树全量重建（DOM 变更/悬停触发的路径）。
func BenchmarkIDEPageRebuildTree(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wv.RebuildRenderTree()
	}
}

// BenchmarkIDEPagePaint 全视口绘制一帧（物理分辨率 2000x1250，含 Clear）。
func BenchmarkIDEPagePaint(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	rv := wv.RenderView()
	if rv == nil {
		b.Fatal("RenderView 为空")
	}
	c := graphics.NewCanvas(perfPhysicalW, perfPhysicalH)
	defer c.Release()
	rect := rendering.Rect{X: 0, Y: 0, Width: perfPhysicalW, Height: perfPhysicalH}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Clear(graphics.Color{})
		rendering.Paint(rv, c, rect)
	}
}

// BenchmarkIDEPagePaintNoClear 只测 Paint（不含 Clear），用于拆分 Clear 成本。
func BenchmarkIDEPagePaintNoClear(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	rv := wv.RenderView()
	c := graphics.NewCanvas(perfPhysicalW, perfPhysicalH)
	defer c.Release()
	rect := rendering.Rect{X: 0, Y: 0, Width: perfPhysicalW, Height: perfPhysicalH}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rendering.Paint(rv, c, rect)
	}
}

// BenchmarkIDEPagePaintNewCanvas 每帧新建 Canvas（复现宿主路径：每帧
// NewCanvasFromSurface 新实例 → fontCache 空 map → 字形/字体对象重建）。
// 与 BenchmarkIDEPagePaint（复用 Canvas）对比，量化字体缓存失效的代价。
func BenchmarkIDEPagePaintNewCanvas(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	rv := wv.RenderView()
	rect := rendering.Rect{X: 0, Y: 0, Width: perfPhysicalW, Height: perfPhysicalH}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := graphics.NewCanvas(perfPhysicalW, perfPhysicalH)
		c.Clear(graphics.Color{})
		rendering.Paint(rv, c, rect)
		c.Release()
	}
}

// BenchmarkIDEPageFrame 一次「完整帧」：布局 + 绘制（模拟宿主渲染循环里
// 需要重绘的帧）。这是与浏览器帧预算（16.7ms）直接可比的数字。
func BenchmarkIDEPageFrame(b *testing.B) {
	wv := newIDEPageWebView(b)
	defer wv.Destroy()
	v := wv.Page().MainFrame().View()
	c := graphics.NewCanvas(perfPhysicalW, perfPhysicalH)
	defer c.Release()
	rect := rendering.Rect{X: 0, Y: 0, Width: perfPhysicalW, Height: perfPhysicalH}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.SetNeedsLayout(true)
		v.Layout()
		rv := wv.RenderView()
		c.Clear(graphics.Color{})
		rendering.Paint(rv, c, rect)
	}
}
