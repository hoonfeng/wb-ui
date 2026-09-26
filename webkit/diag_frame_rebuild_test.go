package webkit

import (
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
)

// TestDiagFrameRebuild 定位「宿主每帧重建渲染树」的触发源。
//
// 背景：BenchmarkIDEPageFrame（Layout+Paint）耗时 331ms/op、46MB/op、
// 191674 allocs/op，而 BenchmarkIDEPageLayout 仅 1.6ms/op、
// BenchmarkIDEPagePaintNoClear 143ms/op、6.2MB/op —— Frame 的分配量约等于
// 「重建树(38.3MB/76822 allocs) + 绘制(6.2MB/102035 allocs)」之和，且
// profile 里 RenderTreeBuilder.buildChildren 出现在 FrameView.Layout 之下
// （25.18%）。这说明「一帧」里包含了一次**全量渲染树重建**。
//
// 本测试在 Layout 前后、Paint 前后分别探测 frame.NeedsRenderTreeRebuild()，
// 判断这个重建标记是被布局、绘制还是两者之间的某处置位的：
//   - afterPaint=true  → Paint（或其回调）把树置脏 → 下帧必然全量重建
//   - afterLayout=true → 布局自身留下挂起重建（cooldown 降频推迟）
func TestDiagFrameRebuild(t *testing.T) {
	wv := newIDEPageWebView(t)
	defer wv.Destroy()
	mf := wv.Page().MainFrame()
	v := mf.View()
	fr := mf // MainFrame() 返回的即 *page.Frame
	rv := wv.RenderView()
	if v == nil || fr == nil || rv == nil {
		t.Fatal("FrameView/Frame/RenderView 为空")
	}
	c := graphics.NewCanvas(perfPhysicalW, perfPhysicalH)
	defer c.Release()
	rect := rendering.Rect{X: 0, Y: 0, Width: perfPhysicalW, Height: perfPhysicalH}

	for i := 0; i < 4; i++ {
		before := fr.NeedsRenderTreeRebuild()
		v.SetNeedsLayout(true)
		v.Layout()
		afterLayout := fr.NeedsRenderTreeRebuild()
		c.Clear(graphics.Color{})
		rendering.Paint(rv, c, rect)
		afterPaint := fr.NeedsRenderTreeRebuild()
		t.Logf("iter=%d 树脏标记: before=%v afterLayout=%v afterPaint=%v",
			i, before, afterLayout, afterPaint)
		// 回归防线：首次绘制后的加载尝试可以置脏（图片真正到达），但从
		// 第 3 轮起必须稳定 —— 若绘制后仍逐帧置脏，说明「失败重试退避」
		// 失效，宿主会退回"每帧全量重建渲染树（38.3MB/76822 allocs、
		// ≈80ms/帧）"的状态（Frame 从 106ms 恶化到 331ms）。
		if i >= 2 && afterPaint {
			t.Fatalf("iter=%d：绘制后渲染树仍被置脏 —— 每帧全量重建树回归", i)
		}
		if i >= 2 && before {
			t.Fatalf("iter=%d：进入布局前树仍为脏 —— 上帧遗留的挂起重建", i)
		}
	}
}
