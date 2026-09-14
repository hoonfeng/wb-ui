package webkit

// location.hash / fragment 同文档导航的真实回归（HTML §7.4.2 "navigate to a
// fragment"）：这类导航**不重新加载文档**，只做三件事——更新 URL 的 fragment、
// 滚动到锚点、在 fragment 真的变化时派发 hashchange；历史条目仍按 push 追加，
// 遍历（back/forward）回到对应 fragment 时 popstate 与 hashchange 都派发。
//
// 引擎此前：`location.hash` 只有 getter（赋值静默丢弃），因此靠 hash 做锚点
// 跳转/单页路由的页面全部失效（URL 不变、不滚动、无事件）。

import (
	"bytes"
	"strings"
	"testing"
)

const fragmentFixtureBase = "http://example.com/page.html"

// 布局：#top 高 1500 → #bottom 在 y=1500（高 200）→ <a name="named"> 在
// y=1700 → #tail 高 900（内容总高 2600，视口 400x300 可滚）。
const fragmentFixtureHTML = `<!DOCTYPE html><html><head><meta charset="utf-8">` +
	`<style>body{margin:0}#bottom:target{background:#ff0000}</style></head><body>` +
	`<div id="top" style="height:1500px">TOP</div>` +
	`<div id="bottom" style="height:200px">BOTTOM</div>` +
	`<a name="named"></a>` +
	`<div id="tail" style="height:900px">TAIL</div>` +
	`<script>` +
	`window.__nav = [];` +
	`window.addEventListener("hashchange", function (e) {` +
	`  window.__nav.push("hashchange|" + e.type + "|" + e.oldURL + "|" + e.newURL);` +
	`});` +
	`window.addEventListener("popstate", function (e) {` +
	`  window.__nav.push("popstate|" + e.type + "|" + document.URL);` +
	`});` +
	`</script></body></html>`

func fragmentLoad(t *testing.T) *WebView {
	t.Helper()
	wv := modeWebView(t, ModeBrowser)
	if err := wv.LoadHTMLWithBaseURL(fragmentFixtureHTML, fragmentFixtureBase); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	return wv
}

func fragmentMustEval(t *testing.T, wv *WebView, script string) {
	t.Helper()
	if _, err := wv.EvalJS(script); err != nil {
		t.Fatalf("EvalJS(%q): %v", script, err)
	}
}

// fragmentNavEvents 返回 window.__nav 里记录的事件（hashchange / popstate）。
func fragmentNavEvents(t *testing.T, wv *WebView) []string {
	t.Helper()
	raw := evalNavStr(t, wv, `window.__nav.join("\n")`)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// fragmentScrollY 返回主框架的页面级滚动偏移（宿主内省用的那一份）。
func fragmentScrollY(t *testing.T, wv *WebView) int {
	t.Helper()
	view := wv.page.MainFrame().View()
	if view == nil {
		t.Fatal("主框架没有 FrameView")
	}
	return view.ScrollY()
}

// fragmentRenderScrollY 返回渲染真正使用的页面级滚动偏移（rendering.RenderView）。
func fragmentRenderScrollY(t *testing.T, wv *WebView) float64 {
	t.Helper()
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("主框架没有 RenderView")
	}
	_, y := rv.ScrollOffset()
	return y
}

// fragmentPixels 返回当前渲染输出（用于「画面真的动了」的像素级断言）。
func fragmentPixels(t *testing.T, wv *WebView) []byte {
	t.Helper()
	px, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return px
}

// fragmentPixelAt 返回**文档坐标** (x,y) 处的像素：渲染前扣除当前页面级滚动
// 偏移，因此无论页面滚到哪里，取到的都是该文档位置画在视口里的颜色。
func fragmentPixelAt(t *testing.T, wv *WebView, x, y int) (r, g, b, a uint8) {
	t.Helper()
	pix := fragmentPixels(t, wv)
	return pixelAt(pix, wv.Width(), x, y-fragmentScrollY(t, wv))
}

// TestLocationHashNavigatesToAnchorInDocument：`location.hash = "#x"` 只改 URL 的
// fragment + 滚动到锚点 + 派发 hashchange —— 文档内容不被替换；历史条目按
// 浏览器语义追加。反向验证：把 hash 的 setter 摘掉（回到「只有 getter」）后，
// 本测试立即失败（URL 不变、不滚动、无事件）。
func TestLocationHashNavigatesToAnchorInDocument(t *testing.T) {
	wv := fragmentLoad(t)
	if got := evalNavStr(t, wv, `location.hash`); got != "" {
		t.Fatalf("初始 location.hash = %q, want \"\"", got)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "1" {
		t.Fatalf("初始 history.length = %q, want 1", got)
	}
	before := fragmentPixels(t, wv)

	fragmentMustEval(t, wv, `location.hash = "#bottom"`)
	if got, want := evalNavStr(t, wv, `document.URL`), fragmentFixtureBase+"#bottom"; got != want {
		t.Errorf("document.URL = %q, want %q", got, want)
	}
	if got := evalNavStr(t, wv, `location.hash`); got != "#bottom" {
		t.Errorf("location.hash = %q, want #bottom", got)
	}
	if got := evalNavStr(t, wv, `document.getElementById("top").textContent`); got != "TOP" {
		t.Errorf("文档被重新加载了（内容丢失）: %q", got)
	}
	// 同文档导航在浏览器里同样产生历史条目。
	if got := evalNavStr(t, wv, `history.length`); got != "2" {
		t.Errorf("hash 导航后 history.length = %q, want 2", got)
	}
	if y := fragmentScrollY(t, wv); y < 1400 || y > 1600 {
		t.Errorf("hash 导航后 ScrollY = %d, want ≈1500（滚动到 #bottom）", y)
	}
	if y := fragmentRenderScrollY(t, wv); y < 1400 || y > 1600 {
		t.Errorf("hash 导航后渲染用 ScrollY = %v, want ≈1500", y)
	}
	// 渲染层验证：滚动必须真的改变画面（渲染管线按 RenderView 的偏移平移
	// canvas）。「状态变了但画面没动」这条断言会失败。
	if after := fragmentPixels(t, wv); bytes.Equal(before, after) {
		t.Errorf("hash 导航后渲染输出与滚动前完全相同：画面没有滚动")
	}
	// `:target` 语义：URL fragment 变化必须让依赖 URL 的选择器重新匹配
	// （`#bottom:target{background:#ff0000}` 是纯 CSS 的 hash 路由写法）。
	if r, g, b, a := fragmentPixelAt(t, wv, 200, 1600); !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("#bottom:target 背景像素 = rgba(%d,%d,%d,%d), want 红", r, g, b, a)
	}
	ev := fragmentNavEvents(t, wv)
	if len(ev) != 1 {
		t.Fatalf("hashchange 事件 = %v, want 恰好一条", ev)
	}
	if !strings.HasPrefix(ev[0], "hashchange|hashchange|") {
		t.Errorf("事件缺少 type=hashchange：%q", ev[0])
	}
	if want := fragmentFixtureBase + "|" + fragmentFixtureBase + "#bottom"; !strings.HasSuffix(ev[0], want) {
		t.Errorf("hashchange oldURL/newURL = %q, want 后缀 %q", ev[0], want)
	}

	// 赋相同的 hash：不追加条目、不派发事件（浏览器语义），但 URL 保持正确。
	fragmentMustEval(t, wv, `location.hash = "#bottom"`)
	if got := evalNavStr(t, wv, `history.length`); got != "2" {
		t.Errorf("重复赋相同 hash 后 history.length = %q, want 2（不追加条目）", got)
	}
	if ev := fragmentNavEvents(t, wv); len(ev) != 1 {
		t.Errorf("重复赋相同 hash 派发了事件: %v", ev)
	}

	// 裸片段名（不带 "#"）等价于带 "#" 的写法。
	fragmentMustEval(t, wv, `location.hash = "named"`)
	if got := evalNavStr(t, wv, `location.hash`); got != "#named" {
		t.Errorf("location.hash（裸名赋值后）= %q, want #named", got)
	}
	if y := fragmentScrollY(t, wv); y < 1600 || y > 1900 {
		t.Errorf("named 锚点 ScrollY = %d, want ≈1700（<a name> 命名锚点）", y)
	}

	// 清除 fragment：URL 去掉 "#…" 且滚回文档顶部。
	fragmentMustEval(t, wv, `location.hash = ""`)
	if got, want := evalNavStr(t, wv, `document.URL`), fragmentFixtureBase; got != want {
		t.Errorf("清除 hash 后 document.URL = %q, want %q", got, want)
	}
	if y := fragmentScrollY(t, wv); y != 0 {
		t.Errorf("清除 hash 后 ScrollY = %d, want 0（滚回文档顶部）", y)
	}
	if r, g, b, _ := fragmentPixelAt(t, wv, 200, 100); r > 200 && g < 80 && b < 80 {
		t.Errorf("清除 hash 后 :target 仍然匹配 #bottom：像素 = rgb(%d,%d,%d)", r, g, b)
	}
	if ev := fragmentNavEvents(t, wv); len(ev) != 3 {
		t.Errorf("事件序列 = %v, want 3 条（#bottom→#named→清除）", ev)
	}

	// 没有匹配的锚点：hashchange 仍派发（fragment 变了），滚动回顶部。
	fragmentMustEval(t, wv, `location.hash = "#missing-anchor"`)
	if y := fragmentScrollY(t, wv); y != 0 {
		t.Errorf("无匹配锚点时 ScrollY = %d, want 0", y)
	}
	if ev := fragmentNavEvents(t, wv); len(ev) != 4 {
		t.Errorf("无匹配锚点时事件 = %v, want 4 条（hashchange 仍派发）", ev)
	}

	// location.href = "#x" 走同一条同文档路径（不换文档、内容保留）。
	fragmentMustEval(t, wv, `location.href = "#bottom"`)
	if got := evalNavStr(t, wv, `document.URL`); got != fragmentFixtureBase+"#bottom" {
		t.Errorf("href 赋 fragment 后 document.URL = %q, want %q", got, fragmentFixtureBase+"#bottom")
	}
	if got := evalNavStr(t, wv, `document.getElementById("bottom").textContent`); got != "BOTTOM" {
		t.Errorf("href 赋 fragment 后文档内容 = %q, want BOTTOM（没有重新加载）", got)
	}
}

// TestHistoryTraverseBetweenFragments：同文档历史遍历——back() 回到上一个
// fragment：不重新加载文档、URL 与滚动位置恢复、popstate 与 hashchange 都派发、
// 条目数不增长。
func TestHistoryTraverseBetweenFragments(t *testing.T) {
	wv := fragmentLoad(t)
	fragmentMustEval(t, wv, `location.hash = "#bottom"`)
	fragmentMustEval(t, wv, `location.hash = "#named"`)
	if got := evalNavStr(t, wv, `history.length`); got != "3" {
		t.Fatalf("history.length = %q, want 3（初始文档 + 两个 fragment 条目）", got)
	}
	fragmentMustEval(t, wv, `window.__nav = []`)

	// back() → 同文档遍历回 #bottom。
	fragmentMustEval(t, wv, `history.back()`)
	if got, want := evalNavStr(t, wv, `document.URL`), fragmentFixtureBase+"#bottom"; got != want {
		t.Errorf("back() 后 document.URL = %q, want %q", got, want)
	}
	if got := evalNavStr(t, wv, `location.hash`); got != "#bottom" {
		t.Errorf("back() 后 location.hash = %q, want #bottom", got)
	}
	if got := evalNavStr(t, wv, `document.getElementById("bottom").textContent`); got != "BOTTOM" {
		t.Errorf("back() 重新加载了文档: %q", got)
	}
	if y := fragmentScrollY(t, wv); y < 1400 || y > 1600 {
		t.Errorf("back() 后 ScrollY = %d, want ≈1500（滚动位置随条目恢复）", y)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "3" {
		t.Errorf("back() 后 history.length = %q, want 3（遍历不追加条目）", got)
	}
	ev := fragmentNavEvents(t, wv)
	if len(ev) != 2 {
		t.Fatalf("back() 事件 = %v, want popstate + hashchange", ev)
	}
	// 浏览器顺序：先 popstate（历史指针已移动），再 hashchange（fragment 变化）。
	if !strings.HasPrefix(ev[0], "popstate|") {
		t.Errorf("第一个事件 = %q, want popstate", ev[0])
	}
	if !strings.HasPrefix(ev[1], "hashchange|hashchange|") {
		t.Errorf("第二个事件 = %q, want hashchange", ev[1])
	}

	// forward() → 回到 #named。
	fragmentMustEval(t, wv, `history.forward()`)
	if got := evalNavStr(t, wv, `location.hash`); got != "#named" {
		t.Errorf("forward() 后 location.hash = %q, want #named", got)
	}
	if y := fragmentScrollY(t, wv); y < 1600 || y > 1900 {
		t.Errorf("forward() 后 ScrollY = %d, want ≈1700", y)
	}
	if got := evalNavStr(t, wv, `history.length`); got != "3" {
		t.Errorf("forward() 后 history.length = %q, want 3", got)
	}
}

// TestToolkitModeRejectsFragmentNavigation：UI 库模式下 fragment 导航同样被模式
// 门禁拒绝（不滚动、不改 URL、不派发 hashchange），宿主能通过回调感知。
func TestToolkitModeRejectsFragmentNavigation(t *testing.T) {
	wv := modeWebView(t, ModeToolkit)
	var blocked []string
	wv.SetOnNavigationBlocked(func(url string) { blocked = append(blocked, url) })
	html := `<!DOCTYPE html><html><head><meta charset="utf-8"><style>body{margin:0}</style></head>` +
		`<body><div id="top" style="height:1500px">TOP</div><div id="bottom">BOTTOM</div>` +
		`<script>window.__hashCalls = 0;` +
		`window.addEventListener("hashchange", function () { window.__hashCalls++; });</script></body></html>`
	if err := wv.LoadHTMLWithBaseURL(html, "app://ui/page.html"); err != nil {
		t.Fatalf("LoadHTMLWithBaseURL: %v", err)
	}
	fragmentMustEval(t, wv, `location.hash = "#bottom"`)
	if got := evalNavStr(t, wv, `document.URL`); got != "app://ui/page.html" {
		t.Errorf("UI 库模式下 URL 被改了: %q", got)
	}
	if y := fragmentScrollY(t, wv); y != 0 {
		t.Errorf("UI 库模式下发生了滚动: ScrollY=%d", y)
	}
	if got := evalNavStr(t, wv, `window.__hashCalls`); got != "0" {
		t.Errorf("UI 库模式下派发了 hashchange: 次数=%q", got)
	}
	if len(blocked) != 1 || blocked[0] != "#bottom" {
		t.Errorf("导航被拒回调 = %v, want 一条 #bottom", blocked)
	}
}
