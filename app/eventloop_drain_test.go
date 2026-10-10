package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"wb-ui/webkit"
)

// pageWithFutureTimers 返回一个页面：带 click 监听器 + **未到期**的
// setTimeout/setInterval（模拟真实框架页面：Vue 的 nextTick 兜底 setTimeout、
// 轮询/心跳、CM6/xterm 自调度 —— 未到期定时器几乎总是存在）。
func pageWithFutureTimers(t *testing.T) (*webkit.WebView, *Host) {
	t.Helper()
	wv := webkit.NewWebView()
	t.Cleanup(wv.Destroy)
	wv.Resize(400, 300)
	if err := wv.LoadHTML(`<!doctype html><html><body>
<div id="b" style="width:100px;height:50px;background:#000"></div>
<script>
window.__clicks = 0;
document.getElementById('b').addEventListener('click', function () {
  window.__clicks++;
  document.getElementById('b').style.background = '#f00';
});
setTimeout(function () { window.__late = 1; }, 30000);
setInterval(function () { window.__ticks = (window.__ticks || 0) + 1; }, 30000);
</script>
</body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	return wv, NewHostForTest(wv, 400, 300)
}

// TestDrainEventLoopDoesNotBlockOnFutureTimers 回归「引擎在触发事件执行时绘制
// 就会停止」：
//
// 宿主在派发事件（click / contextmenu / mouseup）后要**同步**驱动事件循环，
// 让 handler 里的 async 链落地。旧实现用 EventLoop.PendingTasks()==0 作退出
// 条件，但 PendingTasks 把**未到期**的定时器也算 pending ⇒ 页面只要有一个未到期
// 定时器，判定恒为 false，循环只能等满 120~180ms 超时退出；这段超时里主循环
// 既不布局也不绘制 —— 用户看到的就是「一触发事件，画面就停止更新」。
//
// 修复后 drainEventLoop 用 HasReadyTasks（只算可立即执行的任务），未到期定时器
// 不再阻塞：本用例断言 drain 立即返回，且远定时器**没有**被执行。
func TestDrainEventLoopDoesNotBlockOnFutureTimers(t *testing.T) {
	wv, h := pageWithFutureTimers(t)

	interp := wv.JSInterpreter()
	if interp == nil {
		t.Fatal("JSInterpreter = nil")
	}
	el := interp.GetEventLoop()
	if el == nil {
		t.Fatal("GetEventLoop = nil")
	}
	// 前提：页面脚本已执行，两个未到期定时器都在队列里。
	if el.PendingTasks() == 0 {
		t.Fatal("PendingTasks = 0：未到期定时器必须计入 PendingTasks（这正是旧判定等满超时的根源）")
	}
	if el.HasReadyTasks() {
		t.Fatal("HasReadyTasks = true：未到期的 30s 定时器不该算作「可立即执行」")
	}

	start := time.Now()
	h.drainEventLoop(180 * time.Millisecond)
	elapsed := time.Since(start)
	if elapsed > 60*time.Millisecond {
		t.Fatalf("drainEventLoop 耗时 %v：未到期定时器不该阻塞本帧（旧判定会等满 180ms，期间不布局不绘制 = 画面冻结）", elapsed)
	}

	// 远定时器不得被提前执行（浏览器语义：不阻塞当前帧，交给后续帧按时间推进）。
	v, err := interp.RunJS("typeof window.__late")
	if err != nil {
		t.Fatalf("RunJS: %v", err)
	}
	if v.ToString() != "undefined" {
		t.Fatalf("未到期的远定时器被执行了：typeof window.__late = %q", v.ToString())
	}
}

// TestEventDispatchKeepsDirtyAfterDrain 回归的**另一半**：事件处理器执行后，
// 它引发的渲染树变更必须仍然标脏 —— 否则宿主下一帧的 needPaint 判定为 false，
// 会直接跳过绘制，画面停在事件之前的状态（同样表现为「触发事件后绘制停止」）。
func TestEventDispatchKeepsDirtyAfterDrain(t *testing.T) {
	wv, h := pageWithFutureTimers(t)

	// 先渲染一帧：消费首帧脏标记（Paint 内部 ClearDirty）。
	wv.EnsureLayout()
	pix0, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rv := wv.RenderView(); rv != nil && rv.IsDirty() {
		t.Fatal("首帧渲染后仍为脏：基线不成立（无法验证事件后的脏标记）")
	}
	if n := countPixel(pix0, 0xff, 0x00, 0x00); n != 0 {
		t.Fatalf("基线不该有红色像素，实际 %d 个", n)
	}

	// #b 的真实视口中心（body 默认 margin 让元素不在原点，不能硬编码坐标）。
	v, err := wv.EvalJS(`(function(){
		var el = document.getElementById('b');
		if (!el) return '';
		var r = el.getBoundingClientRect();
		return (r.left + r.width/2) + ',' + (r.top + r.height/2);
	})()`)
	if err != nil {
		t.Fatalf("EvalJS 取 #b 中心: %v", err)
	}
	var cx, cy float64
	if _, err := fmt.Sscanf(strings.TrimSpace(v.ToString()), "%f,%f", &cx, &cy); err != nil {
		t.Fatalf("解析 #b 中心失败（%q）：%v", v.ToString(), err)
	}

	// 真实鼠标路径派发 click（命中 <div id=b> 中心），处理器修改背景色。
	wv.HandleMouseMove(cx, cy)
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)

	// 宿主侧：事件后同步驱动事件循环（drain）。这一步不得吞掉脏标记。
	start := time.Now()
	h.drainEventLoop(180 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 60*time.Millisecond {
		t.Fatalf("drainEventLoop 耗时 %v：事件后的同步驱动不该等满超时", elapsed)
	}
	if got, err := wv.EvalJS("String(window.__clicks)"); err != nil {
		t.Fatalf("EvalJS 读 __clicks: %v", err)
	} else if got.ToString() != "1" {
		t.Fatalf("click 处理器未执行：window.__clicks = %s，want 1", got.ToString())
	}

	wv.EnsureLayout()
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView = nil")
	}
	if !rv.IsDirty() {
		t.Fatal("事件处理器修改 DOM 后渲染树未标脏：宿主下一帧会跳过绘制（触发事件后画面停止更新）")
	}

	// 且这次绘制必须真的把新颜色画出来。
	pix1, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if n := countPixel(pix1, 0xff, 0x00, 0x00); n == 0 {
		t.Fatal("事件后首帧画布无红色像素：事件处理器的样式变更未被绘制")
	}
}

// countPixel 统计画布中等于给定 RGB 的像素个数（忽略 alpha）。
func countPixel(pix []byte, r, g, b byte) int {
	n := 0
	for i := 0; i+3 < len(pix); i += 4 {
		if pix[i] == r && pix[i+1] == g && pix[i+2] == b {
			n++
		}
	}
	return n
}
