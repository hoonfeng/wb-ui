package ui

// UI 构建层测试：基础方式（Go 构建）渲染与几何、事件回调命中、幂等事件
// 绑定、web 片段混入与混排、组件注册表双来源（native/web）、UI 库模式
// 端到端（含像素输出）。

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/webkit"
)

func newTestView(t *testing.T, mode webkit.Mode) (*webkit.WebView, *View) {
	t.Helper()
	wv := webkit.NewWebViewWithMode(mode)
	t.Cleanup(func() { wv.Destroy() })
	wv.Resize(400, 300)
	v, err := New(wv)
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return wv, v
}

func evalStr(t *testing.T, wv *webkit.WebView, script string) string {
	t.Helper()
	val, err := wv.EvalJS(script)
	if err != nil {
		t.Fatalf("EvalJS(%q): %v", script, err)
	}
	return val.ToString()
}

// rect 取元素几何（CSS 像素，经引擎的 getBoundingClientRect 几何桥）。
func rect(t *testing.T, wv *webkit.WebView, id string) (x, y, w, h float64) {
	t.Helper()
	raw := evalStr(t, wv, `(function(){var el=document.getElementById(`+strconv.Quote(id)+
		`);if(!el)return "";var r=el.getBoundingClientRect();return JSON.stringify([r.left,r.top,r.width,r.height]);})()`)
	if raw == "" {
		t.Fatalf("元素 #%s 不在文档里", id)
	}
	var arr []float64
	if err := json.Unmarshal([]byte(raw), &arr); err != nil || len(arr) != 4 {
		t.Fatalf("getBoundingClientRect(#%s) = %q", id, raw)
	}
	return arr[0], arr[1], arr[2], arr[3]
}

// click 在元素中心模拟一次按下+释放（引擎的命中测试 → 事件分发路径）。
func click(t *testing.T, wv *webkit.WebView, id string) {
	t.Helper()
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	wv.EnsureHitTestReady()
	x, y, w, h := rect(t, wv, id)
	cx, cy := x+w/2, y+h/2
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
}

// ── 基础方式构建 ─────────────────────────────────────────

// TestNativeBuildRendersWithStyles：Go 构建的节点进入引擎文档树，注入的
// 样式表生效，几何与设置一致（UI 库模式、宿主不写 HTML）。
func TestNativeBuildRendersWithStyles(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	v.Style(`body{margin:0}
		#card{width:120px;height:40px;background:#3366cc}
		#btn{width:60px;height:16px}`)
	v.Div().ID("card").Append(v.Button("点我", nil).ID("btn"))

	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	x, y, w, h := rect(t, wv, "card")
	if math.Abs(w-120) > 1 || math.Abs(h-40) > 1 {
		t.Fatalf("#card 尺寸 = %.1fx%.1f, want 120x40（样式未生效？）", w, h)
	}
	if x != 0 || y != 0 {
		t.Fatalf("#card 位置 = (%.1f,%.1f), want (0,0)", x, y)
	}
	bx, by, bw, bh := rect(t, wv, "btn")
	if bx < x || by < y {
		t.Fatalf("按钮未落在卡片内：btn=(%.1f,%.1f) card=(%.1f,%.1f)", bx, by, x, y)
	}
	if math.Abs(bw-60) > 1 || math.Abs(bh-16) > 1 {
		t.Fatalf("按钮尺寸 = %.1fx%.1f, want 60x16", bw, bh)
	}
}

// pixelRGBA 读取渲染缓冲中 (x,y) 的 RGBA（Pixels() 为 RGBA8888）。
func pixelRGBA(t *testing.T, wv *webkit.WebView, x, y int) (uint8, uint8, uint8, uint8) {
	t.Helper()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stride := wv.Width() * 4
	if len(pix) < stride*wv.Height() {
		t.Fatalf("像素缓冲长度 = %d, want ≥ %d", len(pix), stride*wv.Height())
	}
	i := y*stride + x*4
	return pix[i], pix[i+1], pix[i+2], pix[i+3]
}

// TestNativeBuildPixels：渲染输出里出现设置的颜色（验证「Go 构建的界面
// 真的被画出来」而不只是 DOM 里有节点）。
func TestNativeBuildPixels(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	v.Style(`body{margin:0} #fill{width:400px;height:300px;background:#ff0000}`)
	v.Div().ID("fill")

	r, g, b, a := pixelRGBA(t, wv, 10, 10)
	if r < 200 || g > 60 || b > 60 || a < 200 {
		t.Fatalf("背景像素 RGBA = (%d,%d,%d,%d), want 红不透明", r, g, b, a)
	}
}

// TestBodyBackgroundPropagatesToCanvas：html/body 背景传播到画布根
// （CSS-BACKGROUNDS-3 §2.11.2，与浏览器一致）。
//
// 浏览器行为：页面只写 `body{background:#f00}` 时**整屏**都被染色（body 的
// 背景传播到画布、body 元素自身不再画）；html 与 body 都有背景时用 html 的。
// 引擎此前不传播——只有 body 盒（内容高度）那一条被染色。取视口右下角像素
// （空的 body 盒之外），只有传播生效才会是背景色。
//
// 反向验证：注释掉 renderpipeline.go 里 paintViewportBackground 的调用后，
// 两条用例都会立刻失败（像素保持透明）。
func TestBodyBackgroundPropagatesToCanvas(t *testing.T) {
	cases := []struct {
		name string
		css  string
		want [3]uint8
	}{
		{"body 背景传播到画布", `body{margin:0;background:#ff0000}`, [3]uint8{255, 0, 0}},
		{"html 背景优先于 body", `html{background:#00ff00} body{margin:0;background:#ff0000}`, [3]uint8{0, 255, 0}},
	}
	for _, tc := range cases {
		// 两种模式行为一致：传播属于渲染层，模式接线不应影响画面。
		for _, mode := range []webkit.Mode{webkit.ModeBrowser, webkit.ModeToolkit} {
			wv, v := newTestView(t, mode)
			v.Style(tc.css)
			x, y := wv.Width()-5, wv.Height()-5
			r, g, b, a := pixelRGBA(t, wv, x, y)
			if a < 200 || r != tc.want[0] || g != tc.want[1] || b != tc.want[2] {
				t.Errorf("%s（%s）：视口 (%d,%d) = rgba(%d,%d,%d,%d), want rgb%v 且不透明",
					tc.name, mode, x, y, r, g, b, a, tc.want)
			}
		}
	}
	// 两边都没有背景时画布保持透明（不能凭空引入一层底色）。
	wv, v := newTestView(t, webkit.ModeBrowser)
	v.Style(`body{margin:0}`)
	if _, _, _, a := pixelRGBA(t, wv, 10, 10); a != 0 {
		t.Errorf("html/body 都没有背景时画布应保持透明，实际 alpha=%d", a)
	}
}

// TestNativeNodeUpdates：更新语义（Text/Style/Attr/Remove/Append 移动）。
func TestNativeNodeUpdates(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	v.Style(`body{margin:0} #a,#b{height:20px}`)
	a := v.Div().ID("a").Text("旧文本")
	b := v.Div().ID("b").Text("b")

	html := evalStr(t, wv, `document.body.innerHTML`)
	if !strings.Contains(html, "旧文本") {
		t.Fatalf("初始文本未进入文档：%q", html)
	}

	a.Text("新文本")
	a.Attr("title", "提示")
	a.Style("color", "#00ff00")
	if got := evalStr(t, wv, `document.getElementById("a").textContent`); got != "新文本" {
		t.Fatalf("Text() 更新后 textContent = %q", got)
	}
	if got := evalStr(t, wv, `document.getElementById("a").getAttribute("title")`); got != "提示" {
		t.Fatalf("Attr() 后 title = %q", got)
	}

	// Append 是移动语义：把 b 放进 a
	a.Append(b)
	if got := evalStr(t, wv, `document.getElementById("b").parentNode.id`); got != "a" {
		t.Fatalf("Append 未移动节点：b.parentNode.id = %q", got)
	}
	// Remove 摘除
	b.Remove()
	if got := evalStr(t, wv, `document.getElementById("b") ? "in-doc" : "gone"`); got != "gone" {
		t.Fatalf("Remove 后节点仍在文档：%q", got)
	}
}

// ── 事件 ─────────────────────────────────────────────────

// TestNativeButtonClickCallback：Go 回调经引擎事件分发被调用（命中测试 →
// click → 监听器），与页面 JS 监听器同一条路径。
func TestNativeButtonClickCallback(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	clicks := 0
	v.Style(`body{margin:0} #b{display:block;width:100px;height:40px}`)
	v.Button("点我", func(dom.Event) { clicks++ }).ID("b")

	click(t, wv, "b")
	if clicks != 1 {
		t.Fatalf("click 回调次数 = %d, want 1", clicks)
	}

	// 点按钮之外（body 空白处）不应触发
	wv.HandleMouseButton(300, 250, 0, 0)
	wv.HandleMouseButton(300, 250, 0, 1)
	if clicks != 1 {
		t.Fatalf("点击按钮外触发了回调：clicks = %d", clicks)
	}
}

// TestNodeOnReplacesHandler：同一事件重复 On 替换旧处理器（不累积重复回调）。
func TestNodeOnReplacesHandler(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	first, second := 0, 0
	v.Style(`body{margin:0} #b{display:block;width:80px;height:30px}`)
	btn := v.Button("x", nil).ID("b")
	btn.On("click", func(dom.Event) { first++ })
	btn.On("click", func(dom.Event) { second++ })

	click(t, wv, "b")
	if first != 0 {
		t.Errorf("被替换的处理器仍被调用：first = %d", first)
	}
	if second != 1 {
		t.Errorf("最新处理器调用次数 = %d, want 1", second)
	}
}

// ── web 方式混入 ─────────────────────────────────────────

// TestWebFragmentMixedWithNative：web 片段与 Go 构建的节点在同一棵树里，
// 按文档顺序参与布局；两边都能互相查询。
func TestWebFragmentMixedWithNative(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	v.Style(`body{margin:0} #native1{height:20px} #web1{height:20px}`)
	v.Div().ID("native1").Text("native")
	v.Web(`<div id="web1">web</div>`)

	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if wv.Document().GetElementById("web1") == nil {
		t.Fatal("web 片段节点未接入文档")
	}
	_, y1, _, h1 := rect(t, wv, "native1")
	_, y2, _, _ := rect(t, wv, "web1")
	if y2 < y1+h1-1 {
		t.Fatalf("web 片段未排在 native 节点之后：y1=%.1f h1=%.1f y2=%.1f", y1, h1, y2)
	}

	// Node.Web 返回承载容器，容器内可继续查询（Go 侧可继续操作 web 片段）
	holder := v.Web(`<div id="web2">web2</div>`)
	if holder == nil {
		t.Fatal("Node.Web 返回 nil")
	}
	if holder.Element().GetElementById("web2") == nil {
		t.Fatal("web 片段容器内查不到片段节点")
	}
	// 反向：native 节点可以挂进 web 片段里
	inside := v.Web(`<div id="host"></div>`)
	if inside.Element().GetElementById("host") == nil {
		t.Fatal("片段容器结构异常")
	}
	nativeChild := v.Div().ID("into-web").Text("go")
	host := &Node{view: v, el: inside.Element().GetElementById("host")}
	host.Append(nativeChild)
	if got := evalStr(t, wv, `document.getElementById("into-web").parentNode.id`); got != "host" {
		t.Fatalf("Go 节点未挂进 web 片段：parentNode.id = %q", got)
	}
}

// ── 组件注册表（基础 vs web 双来源）──────────────────────

func TestRegistryDualSource(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)
	reg := v.Registry()

	if err := reg.RegisterNative("badge", func(view *View, p Props) *Node {
		return view.El("span").Class("badge").Text(p.String("text"))
	}); err != nil {
		t.Fatalf("RegisterNative: %v", err)
	}
	if err := reg.RegisterWeb("chip", func(p Props) string {
		return `<span class="chip" data-len="` + strconv.Itoa(p.Int("len")) + `">` + p.String("text") + `</span>`
	}); err != nil {
		t.Fatalf("RegisterWeb: %v", err)
	}

	if s, ok := reg.Source("badge"); !ok || s != SourceNative {
		t.Fatalf("badge 来源 = %v/%v, want native", s, ok)
	}
	if s, ok := reg.Source("chip"); !ok || s != SourceWeb {
		t.Fatalf("chip 来源 = %v/%v, want web", s, ok)
	}
	if names := reg.Names(); len(names) != 2 || names[0] != "badge" || names[1] != "chip" {
		t.Fatalf("Names() = %v", names)
	}

	// 定义校验
	if err := reg.Register("both", Component{Native: func(*View, Props) *Node { return nil }, Web: func(Props) string { return "" }}); !errors.Is(err, ErrComponentDef) {
		t.Errorf("Native+Web 同时设置：err = %v, want ErrComponentDef", err)
	}
	if err := reg.Register("none", Component{}); !errors.Is(err, ErrComponentDef) {
		t.Errorf("Native/Web 都未设置：err = %v, want ErrComponentDef", err)
	}
	if err := reg.Register("   ", Component{Native: func(*View, Props) *Node { return nil }}); !errors.Is(err, ErrComponentName) {
		t.Errorf("空名：err = %v, want ErrComponentName", err)
	}

	// 挂载：两种来源都进入同一棵树
	v.Style(`body{margin:0} .badge,.chip{display:block;height:18px}`)
	badgeNode, err := v.Mount("badge", Props{"text": "基础组件"})
	if err != nil {
		t.Fatalf("Mount(badge): %v", err)
	}
	// 基础方式：返回组件实例本身（span）；web 方式：返回承载片段的容器。
	if got := badgeNode.Element().TagName(); got != "SPAN" {
		t.Errorf("native 组件返回节点 = %s, want SPAN", got)
	}
	chipNode, err := v.Mount("chip", Props{"text": "web组件", "len": 7})
	if err != nil {
		t.Fatalf("Mount(chip): %v", err)
	}
	if got := chipNode.Element().TagName(); got != "DIV" {
		t.Errorf("web 组件返回节点 = %s, want DIV（片段承载容器）", got)
	}
	if _, err := v.Mount("missing", nil); !errors.Is(err, ErrComponentNotFound) {
		t.Errorf("未注册组件：err = %v, want ErrComponentNotFound", err)
	}

	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := evalStr(t, wv, `document.body.innerHTML`)
	for _, want := range []string{"基础组件", "web组件", `data-len="7"`} {
		if !strings.Contains(html, want) {
			t.Errorf("渲染出的 HTML 缺少 %q：%s", want, html)
		}
	}
}

// ── UI 库模式端到端 ──────────────────────────────────────

// TestToolkitModeEndToEnd：模式（无浏览器专属能力）+ 基础构建 + web 片段 +
// 桥路由数据 + 像素输出，一次性串起来。
func TestToolkitModeEndToEnd(t *testing.T) {
	wv, v := newTestView(t, webkit.ModeToolkit)

	// 1. 浏览器专属能力被裁剪（UI 库语义）
	for _, name := range []string{"Worker", "WebSocket", "XMLHttpRequest"} {
		if got := evalStr(t, wv, `typeof `+name); got != "undefined" {
			t.Errorf("UI 库模式下 typeof %s = %q, want undefined", name, got)
		}
	}
	// 2. 基础方式构建界面
	v.Style(`body{margin:0}
		#bg{width:400px;height:300px;background:#00ff00}
		#panel{width:50px;height:50px;background:#0000ff}`)
	v.Div().ID("bg").Append(v.Div().ID("panel"))
	// 3. web 方式混入
	v.Web(`<div id="web-slot">web</div>`)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 4. 两条路的产物都在同一棵树里
	if wv.Document().GetElementById("panel") == nil || wv.Document().GetElementById("web-slot") == nil {
		t.Fatal("native/web 节点应同时存在于文档")
	}
	// 5. 像素：panel 区域是蓝色（native 构建的盒子被画出来）
	r, g, b, _ := pixelRGBA(t, wv, 25, 25)
	if b < 200 || r > 60 || g > 60 {
		t.Fatalf("panel 像素 RGB = (%d,%d,%d), want 蓝", r, g, b)
	}
	// 6. panel 之外的区域是铺满容器 #bg 的绿色
	r2, g2, b2, _ := pixelRGBA(t, wv, 300, 200)
	if g2 < 200 || r2 > 60 || b2 > 60 {
		t.Fatalf("#bg 像素 RGB = (%d,%d,%d), want 绿", r2, g2, b2)
	}
}
