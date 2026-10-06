package webkit

// A2-③：宿主 worker 交付播放帧 → 引擎**自己**置脏重绘。
//
// 与 async_repaint_test.go（图片那条通知）同构，但走的是视频帧通道：异步抽帧的字节在
// worker goroutine 上回来，按需渲染的宿主（app.Host 里 `rv.IsDirty()` 为假即跳过
// Paint）不会自己发现帧缓存里多了一帧——没有 onAsyncVideoFrame 这条接线，播放画面就
// 要等**下一次别的重绘理由**（鼠标移动、脚本改 DOM）才更新，看起来是卡的。
//
// 反向验证：注释掉 NewWebViewWithMode 里 AddVideoFrameReadyListener 的接线（或
// onAsyncVideoFrame 里的置脏），本测试立刻失败。

import (
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// TestVideoFrameDeliveryMarksFrameDirty 直接驱动渲染层的帧通道（不依赖 ffmpeg 与真实
// 播放）：预取 → 宿主交付一帧 → 断言视图已被引擎自己置脏。
func TestVideoFrameDeliveryMarksFrameDirty(t *testing.T) {
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadHTML(`<!DOCTYPE html><html><body><div style="width:10px;height:10px"></div></body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 缺失")
	}
	// 稳定一帧 + 清零脏标记：否则「帧到位后置脏」可能来自其它脏源（布局/样式重扫），
	// 断言就抓不住本回归。
	rv.ClearDirty()
	if rv.IsDirty() {
		t.Fatal("前置条件失败：清零脏标记后视图仍为脏（有其它脏源在持续置脏）")
	}

	// 装一个手工控制的异步帧源：交付时机握在测试手里。
	deliver := make(chan func([]byte, bool), 1)
	rendering.SetVideoAsyncFrameSource(func(_ string, _ float64, d func([]byte, bool)) {
		deliver <- d
	})
	defer rendering.SetVideoAsyncFrameSource(nil)

	rendering.PrefetchVideoFrame("clip.mp4", 1.0)
	var d func([]byte, bool)
	select {
	case d = <-deliver:
	case <-time.After(2 * time.Second):
		t.Fatal("异步帧源未被调用（预取请求没有下发到宿主）")
	}
	d(redPNG4x4(t), true)

	// ★ 核心断言：引擎自己置脏——宿主什么都没做。
	if !rv.IsDirty() {
		t.Error("帧交付后视图未置脏：按需渲染的宿主会一直跳过 Paint（播放画面不更新）")
	}
}
