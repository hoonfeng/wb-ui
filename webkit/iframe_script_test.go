package webkit

import (
	"fmt"
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/page"
	"wb-ui/engine/rendering"
)

// TestIFrameScriptExecutes 验证 iframe 子文档的 <script> 真正执行：
//   1. 子文档内联脚本改自己的 DOM（document.body 追加元素）
//   2. 脚本执行后子 Frame 渲染树重建（新元素可命中）
func TestIFrameScriptExecutes(t *testing.T) {
	wv := NewWebView()
	child := `<html><body><div id="marker">before</div><script>
		var m = document.getElementById('marker');
		m.textContent = 'after-script';
		var d = document.createElement('div');
		d.id = 'injected';
		d.textContent = 'from-script';
		document.body.appendChild(d);
		window.__subRun = true;
	</script></body></html>`
	dataURI := "data:text/html," + encDataURI(child)
	src := `<html><body><iframe id="f1" src="` + dataURI + `" width="200" height="100"></iframe></body></html>`

	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}
	subDoc := sub.Document()
	if subDoc == nil {
		t.Fatal("子文档 nil")
	}

	// 1. 脚本已执行：marker 文本被改
	t.Logf("子文档 text=%q", subDoc.TextContent())
	t.Logf("子文档 script 数=%d", len(subDoc.GetElementsByTagName("script")))
	if m := subDoc.GetElementById("marker"); m == nil {
		t.Fatalf("子文档缺 #marker（全文=%q）", subDoc.TextContent())
	} else if got := m.TextContent(); got != "after-script" {
		t.Fatalf("marker.textContent=%q, want after-script（子文档脚本未执行）", got)
	}

	// 2. 脚本创建的 #injected 在 DOM 里（ExecuteScripts 后 RebuildRenderTree）
	if el := subDoc.GetElementById("injected"); el == nil {
		t.Fatal("子文档缺 #injected（脚本 appendChild 未生效）")
	} else if got := el.TextContent(); got != "from-script" {
		t.Fatalf("injected.textContent=%q", got)
	}

	// 3. 子文档脚本的全局标记：独立 interpreter 已建立
	rt := wv.subframeInterpreter(sub)
	if rt == nil {
		t.Fatal("子 Frame 无独立 JS 环境")
	}
	v, err := rt.RunJS("window.__subRun")
	if err != nil {
		t.Fatalf("读子全局: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("子全局 __subRun 应为 true（脚本在子 interpreter 执行）")
	}
}

// TestIFrameScriptIsolated 验证子文档脚本与主文档 JS 全局环境互相隔离
// （浏览器 iframe 语义）：子脚本改自己的 window 不影响主 window。
func TestIFrameScriptIsolated(t *testing.T) {
	wv := NewWebView()
	child := `<html><body><script>window.__sub = 'sub-value';</script></body></html>`
	dataURI := "data:text/html," + encDataURI(child)
	src := `<html><body><iframe id="f1" src="` + dataURI + `"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	// 主 window 不应有 __sub（隔离）
	mainV, err := wv.JSInterpreter().RunJS("window.__sub")
	if err != nil {
		t.Fatalf("主环境读取: %v", err)
	}
	if mainV.ToString() != "" {
		t.Fatalf("主 window.__sub=%q，子脚本污染了主环境", mainV.ToString())
	}
	// 子环境有 __sub
	sub := page.IFrameFrame(wv.MainFrame().Document().GetElementById("f1"))
	if sub == nil {
		t.Fatal("子 Frame 未注册")
	}
	subV, err := wv.subframeInterpreter(sub).RunJS("window.__sub")
	if err != nil {
		t.Fatalf("子环境读取: %v", err)
	}
	if subV.ToString() != "sub-value" {
		t.Fatalf("子 window.__sub=%q", subV.ToString())
	}
}

// TestIFrameRelativeSrc 验证相对路径 src 以主文档 URL 为基准解析：
//   LoadURL("file:///D:/site/index.html") 且 iframe src="./page.html"
//   → 子 Frame 应尝试加载 file:///D:/site/page.html（文件不存在则
//   子 Frame 不注册，但解析逻辑本身不 panic）。
func TestIFrameRelativeSrc(t *testing.T) {
	wv := NewWebView()
	src := `<html><body><iframe id="f1" src="page.html" width="100" height="50"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	// 无 base URL：相对 src 无法解析，子 Frame 不创建
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	if page.IFrameFrame(iframeEl) != nil {
		t.Fatal("无 base URL 时相对 src 不应创建子 Frame")
	}
	// 有 base URL：resolveIframeSrc 正确拼接
	got := resolveIframeSrc("page.html", "file:///D:/site/index.html")
	if got != "file:///D:/site/page.html" {
		t.Fatalf("resolveIframeSrc=%q, want file:///D:/site/page.html", got)
	}
	got = resolveIframeSrc("./sub/a.html", "http://localhost:9090/panel/index.html")
	if got != "http://localhost:9090/panel/sub/a.html" {
		t.Fatalf("resolveIframeSrc(./sub/a.html)=%q", got)
	}
	// 绝对 src 原样返回
	got = resolveIframeSrc("data:text/html,x", "file:///D:/site/index.html")
	if got != "data:text/html,x" {
		t.Fatalf("resolveIframeSrc(data:)=%q", got)
	}
	got = resolveIframeSrc("http://x/y", "")
	if got != "http://x/y" {
		t.Fatalf("resolveIframeSrc(http)=%q", got)
	}
}

// TestIFrameDynamicSrcReload 验证 JS 侧修改 iframe src（el.src = x /
// setAttribute）触发子文档重载：旧子文档卸载、新子文档加载。
func TestIFrameDynamicSrcReload(t *testing.T) {
	wv := NewWebView()
	childA := "data:text/html," + encDataURI(`<html><body><div id="ver">A</div></body></html>`)
	childB := "data:text/html," + encDataURI(`<html><body><div id="ver">B</div></body></html>`)
	src := `<html><body><iframe id="f1" src="` + childA + `" width="200" height="100"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	doc := wv.MainFrame().Document()
	iframeEl := doc.GetElementById("f1")
	if iframeEl == nil {
		t.Fatal("iframe#f1 不存在")
	}
	t.Logf("src attr=%q IFrameCount=%d", iframeEl.GetAttribute("src"), page.IFrameCount())

	// 初始：子文档 A
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("子 Frame 未注册")
	}
	if v := sub.Document().GetElementById("ver"); v == nil || v.TextContent() != "A" {
		t.Fatalf("初始子文档应含 A，got %v", func() string { if v != nil { return v.TextContent() }; return "nil" }())
	}

	// 通过 setAttribute 改 src → 重载为 B
	_, err := wv.JSInterpreter().RunJS(`document.getElementById('f1').setAttribute('src', '` + childB + `')`)
	if err != nil {
		t.Fatalf("setAttribute: %v", err)
	}
	sub2 := page.IFrameFrame(iframeEl)
	if sub2 == nil {
		t.Fatal("重载后子 Frame 丢失")
	}
	if sub2 == sub {
		t.Fatal("重载应创建新子 Frame（旧 Frame 已卸载）")
	}
	if v := sub2.Document().GetElementById("ver"); v == nil || v.TextContent() != "B" {
		t.Fatalf("setAttribute 重载后应含 B，got %q", func() string { if v != nil { return v.TextContent() }; return "nil" }())
	}

	// 通过反射属性 el.src 改 → 重载为 A
	_, err = wv.JSInterpreter().RunJS(`document.getElementById('f1').src = '` + childA + `'`)
	if err != nil {
		t.Fatalf("el.src 赋值: %v", err)
	}
	sub3 := page.IFrameFrame(iframeEl)
	if sub3 == nil {
		t.Fatal("el.src 重载后子 Frame 丢失")
	}
	if v := sub3.Document().GetElementById("ver"); v == nil || v.TextContent() != "A" {
		t.Fatalf("el.src 重载后应含 A，got %q", func() string { if v != nil { return v.TextContent() }; return "nil" }())
	}

	// src 置空 → 子 Frame 卸载
	_, err = wv.JSInterpreter().RunJS(`document.getElementById('f1').setAttribute('src', '')`)
	if err != nil {
		t.Fatalf("setAttribute('') : %v", err)
	}
	if page.IFrameFrame(iframeEl) != nil {
		t.Fatal("src 置空后子 Frame 应卸载")
	}
}

// TestIFrameHitTestDive 验证 hit-test 下钻：点击 iframe 内容框内的点，
// 命中子文档元素（而非 iframe 元素本身）。
func TestIFrameHitTestDive(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="btn" style="position:absolute;left:10px;top:10px;width:80px;height:30px;background:#e00"></div></body></html>`)
	src := `<html><body style="margin:0"><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	if rv == nil {
		t.Fatal("主 RenderView nil")
	}
	// 点击 iframe 内容框内 (50, 30) → 命中子文档 #btn（在子文档 (40,20) 处
	// 因为 btn 在子文档坐标系 10,10 起 80x30，点 (50,30) 在 iframe 里对应
	// 子文档 (50,30) 也在 btn 内）
	el := rendering.HitTest(rv, 50, 30, "")
	if el == nil {
		t.Fatal("hit-test 无命中")
	}
	if el.GetId() != "btn" {
		t.Fatalf("hit-test 应命中子文档 #btn，got id=%q tag=%s", el.GetId(), el.TagName())
	}
	// 点击 iframe 外的点（仍在 html 内容区内）→ 主文档元素
	el2 := rendering.HitTest(rv, 220, 20, "")
	if el2 == nil {
		t.Fatal("iframe 外 hit-test 无命中")
	}
	if el2.LocalName() == "iframe" {
		t.Fatalf("外部点击不应命中 iframe（应为 body/html）")
	}
}

// TestIFrameScrollContainer 验证 iframe 内滚动容器查找：子文档有
// overflow:auto 容器时，主 RenderView.HitTestScrollContainer(iframe 内点)
// 返回子 Frame 的滚动容器（而非 nil）。
func TestIFrameScrollContainer(t *testing.T) {
	wv := NewWebView()
	// 子文档：overflow:auto 容器 150x60，内容超出（子块 150x200）
	child := encDataURI(`<html><body style="margin:0"><div id="scrollable" style="position:absolute;left:5px;top:5px;width:150px;height:60px;overflow:auto;background:#eee"><div style="width:140px;height:200px"></div></div></body></html>`)
	src := `<html><body style="margin:0"><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	if rv == nil {
		t.Fatal("主 RenderView nil")
	}
	// 点击子文档 #scrollable 内（iframe (50, 30) → 子文档 (50,30)，
	// 落在 scrollable(5,5,150,60) 内）
	box := rv.HitTestScrollContainer(50, 30)
	if box == nil {
		t.Fatal("iframe 内滚动容器未命中")
	}
	if el, isEl := box.Node().(*dom.Element); !isEl || el.GetId() != "scrollable" {
		t.Fatalf("滚动容器应为子文档 #scrollable，got %v", func() string { if el != nil { return el.GetId() }; return "nil" }())
	}
	// iframe 外点：滚动容器应是主文档元素（或 nil），但不是子文档的
	box2 := rv.HitTestScrollContainer(230, 130)
	if box2 != nil {
		if el, isEl := box2.Node().(*dom.Element); isEl && el.OwnerDocument() != wv.MainFrame().Document() {
			t.Fatalf("iframe 外点不应命中子文档滚动容器")
		}
	}
}

// TestIFrameScrollTargetRoute 验证滚动事件路由到子 Frame（s1）：
// ScrollTargetAt 应返回子文档滚动容器 + **子 RenderView**（偏移表属主）。
// 主 rv.BoxScrollOffset(子box) 查不到，滚轮必须写子 rv 才生效。
func TestIFrameScrollTargetRoute(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="scrollable" style="position:absolute;left:5px;top:5px;width:150px;height:60px;overflow:auto;background:#eee"><div style="width:140px;height:200px"></div></div></body></html>`)
	src := `<html><body style="margin:0"><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	// iframe 内点 (50,30)：目标应为子文档 #scrollable + 子 RenderView
	tgt := rv.ScrollTargetAt(50, 30)
	if tgt.Box == nil {
		t.Fatal("iframe 内滚动目标未命中")
	}
	if el, isEl := tgt.Box.Node().(*dom.Element); !isEl || el.GetId() != "scrollable" {
		t.Fatalf("目标 box 应为子文档 #scrollable")
	}
	if tgt.RV == rv {
		t.Fatal("iframe 内滚动目标 RV 应为子 RenderView（非主 rv）——偏移表在子 rv")
	}
	// 子 rv 的偏移读写生效：写偏移后子 rv 能读回（主 rv 读不到）
	subRV := tgt.RV
	subRV.SetBoxScrollOffset(tgt.Box, 0, 30)
	sx, sy := subRV.BoxScrollOffset(tgt.Box)
	if sy != 30 {
		t.Fatalf("子 rv 读回偏移 sy=%v, want 30", sy)
	}
	if sx != 0 {
		t.Fatalf("sx=%v", sx)
	}
	// 主 rv 查不到子 box 的偏移（证明必须用子 rv 路由）
	msx, msy := rv.BoxScrollOffset(tgt.Box)
	if msx != 0 || msy != 0 {
		t.Fatalf("主 rv 不应持有子 box 偏移（got %v,%v）", msx, msy)
	}

	// iframe 外点：目标 RV 是主 rv（不误路由）
	tgt2 := rv.ScrollTargetAt(230, 130)
	if tgt2.Box != nil && tgt2.RV != rv {
		t.Fatalf("iframe 外点不应路由到子 rv")
	}
}

// TestIFrameFixedHitTestDive 验证 fixed 定位 iframe 下钻（s2）：
// 主文档 position:fixed 的 iframe（悬浮嵌入面板），点击内容应命中
// 子文档元素（hitTestFixedInner 下钻路径）。
func TestIFrameFixedHitTestDive(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="fbtn" style="position:absolute;left:10px;top:10px;width:80px;height:30px;background:#07c"></div></body></html>`)
	src := `<html><body style="margin:0"><div id="page-content" style="width:600px;height:400px;background:#ccc">main</div><iframe id="f1" src="data:text/html,` + child + `" style="position:fixed;left:10px;top:10px;width:200px;height:100px;border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(640, 480)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	// 点击 fixed iframe 内容 (50,30)（子文档坐标 50,30，落在 #fbtn 10,10 80x30 内）
	el := rendering.HitTest(rv, 50, 30, "")
	if el == nil {
		t.Fatal("fixed iframe 内 hit-test 无命中")
	}
	if el.GetId() != "fbtn" {
		t.Fatalf("fixed iframe 下钻应命中子文档 #fbtn，got id=%q tag=%s", el.GetId(), el.TagName())
	}
	// 点击 fixed iframe 外（主文档内容区 (300,200)）→ 主文档元素，不误入子文档
	el2 := rendering.HitTest(rv, 300, 200, "")
	if el2 == nil {
		t.Fatal("主文档 hit-test 无命中")
	}
	if el2.OwnerDocument() != wv.MainFrame().Document() {
		t.Fatalf("fixed iframe 外点击应命中主文档元素（got 子文档元素 id=%q）", el2.GetId())
	}
}

// TestIFrameTextSelectionDive 验证文本选择下钻（s3）：点击 iframe 内
// 子文档文本，HitTestText 应返回子文档的 RenderText 位置（而非无效）。
func TestIFrameTextSelectionDive(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="txt" style="position:absolute;left:5px;top:5px;font-size:16px">hello-iframe-text</div></body></html>`)
	src := `<html><body style="margin:0"><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	// 点击子文档文本区域（iframe (40,20) → 子文档 (40,20)，落在
	// #txt(5,5) 起的文本行内）
	pos := rendering.HitTestText(rv, 40, 20)
	if !pos.IsValid() {
		t.Fatal("iframe 内文本 hit-test 无命中（应下钻子文档文本）")
	}
	// 命中的 RenderText 属于子文档（其 OwnerDocument 非主文档）
	if pos.RT == nil {
		t.Fatal("pos.RT nil")
	}
	subDoc := page.IFrameFrame(wv.MainFrame().Document().GetElementById("f1")).Document()
	if od := pos.RT.Node().OwnerDocument(); od != subDoc {
		t.Fatalf("命中的 RenderText 应属于子文档")
	}
	// 点击 iframe 外空白（无文本处）→ 无效位置（不误命中）
	pos2 := rendering.HitTestText(rv, 230, 130)
	if pos2.IsValid() {
		t.Fatalf("iframe 外空白处不应命中文本（got RT=%p）", pos2.RT)
	}
}

// TestIFrameScrollbarHitTestDive 验证 iframe 内滚动条命中下钻（s1）：
// 点击子文档滚动容器右侧的滚动条区域，HitTestScrollbar 应命中子文档
// 滚动条，且返回的 RV 是子 Frame 的 RenderView（偏移表存子 rv——主 rv
// 查不到子 box 偏移，写偏移必须用子 rv）。
func TestIFrameScrollbarHitTestDive(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="sc" style="position:absolute;left:0;top:0;width:200px;height:60px;overflow:auto"><div style="height:300px">scroll-content</div></div></body></html>`)
	src := `<html><body style="margin:0"><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}
	subRV := sub.RenderView()
	if subRV == nil {
		t.Fatal("子 Frame RenderView nil")
	}
	subDoc := sub.Document()
	scEl := subDoc.GetElementById("sc")
	if scEl == nil {
		t.Fatal("子文档缺 #sc")
	}
	// 子 rv 布局后找 #sc 的 RenderBox（子文档坐标）
	if sub.NeedsLayout() {
		sub.LayoutNow()
	}
	scBox := subRV.FindRenderBoxForNode(scEl)
	if scBox == nil {
		t.Fatal("子文档 #sc 无 RenderBox")
	}
	m := rendering.VerticalScrollbarMetrics(subRV, scBox)
	if !m.OK {
		t.Fatal("子文档 #sc 应可滚动（高 300 内容 vs 60 视口）")
	}
	// 滚动条区域：子坐标 = 主坐标（iframe 在主 (0,0) 且 border 0）
	// x 在 box 右侧滚动条内（标准滚动条宽 ~15px，取右侧 4px 处），
	// y 在 track 中部（避开箭头区）。
	scPB := scBox.PaddingBoxRect()
	trackY := scPB.Y + 30
	hitX := scPB.X + scPB.Width - 4

	// 主 rv 命中子文档滚动条 → RV 应为子 rv
	hit := rendering.HitTestScrollbar(rv, hitX, trackY)
	if hit == nil {
		t.Fatalf("HitTestScrollbar(%v,%v) 未命中（应命中子文档滚动条）", hitX, trackY)
	}
	if hit.RV != subRV {
		t.Fatalf("滚动条命中 RV=%p, want 子 rv=%p（子文档滚动条偏移表在子 rv）", hit.RV, subRV)
	}
	if hit.Box != scBox {
		t.Fatalf("滚动条命中 Box 不是子文档 #sc")
	}
	if !hit.IsVThumb && !hit.IsVTrack {
		t.Fatalf("应命中垂直 thumb 或 track（got VThumb=%v VTrack=%v）", hit.IsVThumb, hit.IsVTrack)
	}
	// 偏移读写验证：主 rv 查不到子 box 偏移（返回 0），子 rv 写后能读回
	if sx, sy := rv.BoxScrollOffset(scBox); sx != 0 || sy != 0 {
		t.Fatalf("主 rv 不应持有子 box 偏移（got %v,%v）", sx, sy)
	}
	subRV.SetBoxScrollOffset(scBox, 0, 50)
	if _, sy := subRV.BoxScrollOffset(scBox); sy != 50 {
		t.Fatalf("子 rv 写偏移后读回 sy=%v, want 50", sy)
	}
}

// collectTestTexts 遍历渲染树收集所有 RenderText（测试辅助）。
func collectTestTexts(o rendering.RenderObject, list *[]*rendering.RenderText) {
	if o == nil {
		return
	}
	if rt, ok := o.(*rendering.RenderText); ok {
		*list = append(*list, rt)
		return
	}
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		collectTestTexts(c, list)
	}
}

// TestIFrameSelectionHighlightCrossFrame 验证跨 iframe 文本选择的渲染
// 高亮（s2）：选择锚在主文档、终点在 iframe 子文档（或反之）时，主 rv
// 绘制输出主文档部分的高亮矩形、子 rv 绘制输出子文档部分的高亮矩形。
func TestIFrameSelectionHighlightCrossFrame(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="s1" style="font-size:16px">sub-one-text</div></body></html>`)
	src := `<html><body style="margin:0"><div id="m1" style="font-size:16px">main-one-text</div><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0"></iframe><div id="m2" style="font-size:16px">main-two-text</div></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 160)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}
	subRV := sub.RenderView()
	// 收集主/子文档文本
	var mainTexts, subTexts []*rendering.RenderText
	collectTestTexts(rendering.RenderObject(rv), &mainTexts)
	if subRV != nil {
		collectTestTexts(rendering.RenderObject(subRV), &subTexts)
	}
	if len(mainTexts) == 0 || len(subTexts) == 0 {
		t.Fatalf("主/子文档文本缺失（main=%d sub=%d）", len(mainTexts), len(subTexts))
	}
	// 选区：主文档第一个文本 → 子文档第一个文本（跨 iframe）
	rendering.CurrentSelection = &rendering.Selection{
		Start: rendering.TextPosition{RT: mainTexts[0], Offset: 0},
		End:   rendering.TextPosition{RT: subTexts[0], Offset: subTexts[0].Length()},
	}
	defer func() { rendering.CurrentSelection = nil }()

	// 走完整 Paint 入口（主 Frame 构建跨 Frame 全局文本列表）
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// 主 rv：应输出主文档部分的高亮（main-one-text 全选 + main-two-text 全选）
	mainRects := rendering.SelectionRects(rv)
	if len(mainRects) == 0 {
		t.Fatal("跨 iframe 选区：主 rv 应输出主文档部分的高亮矩形")
	}
	// 子 rv：应输出子文档部分的高亮
	subRects := rendering.SelectionRects(subRV)
	if len(subRects) == 0 {
		t.Fatal("跨 iframe 选区：子 rv 应输出子文档部分的高亮矩形")
	}
	// 命中验证：主文档文本应在选区中，子文档文本也应在选区中
	if !rendering.IsOffsetInSelection(rv, mainTexts[0], 0) {
		t.Fatal("主文档文本应被跨 Frame 选区覆盖")
	}
	if !rendering.IsOffsetInSelection(subRV, subTexts[0], 0) {
		t.Fatal("子文档文本应被跨 Frame 选区覆盖")
	}
	// 反向选区（子 → 主）也应正确
	rendering.CurrentSelection = &rendering.Selection{
		Start: rendering.TextPosition{RT: subTexts[0], Offset: 0},
		End:   rendering.TextPosition{RT: mainTexts[len(mainTexts)-1], Offset: 1},
	}
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render(反向): %v", err)
	}
	if len(rendering.SelectionRects(rv)) == 0 || len(rendering.SelectionRects(subRV)) == 0 {
		t.Fatal("反向跨 Frame 选区：主/子 rv 都应输出高亮")
	}
}

// TestIFrameCursorHoverDive 验证子 Frame 滚动条 hover 高亮的光标下钻
// （t2）：光标在主坐标 (cx,cy) 落在 iframe 内容框内时，SetCursorPosRecursive
// 应把换算后的子坐标设置到子 rv（子文档滚动条 hover 读子 rv cursor）；
// 光标移出 iframe 后子 rv 光标应被清除（大负数），避免 hover 残留。
func TestIFrameCursorHoverDive(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="s1" style="font-size:16px">sub-text</div></body></html>`)
	src := `<html><body style="margin:0"><div id="m1" style="font-size:16px">main</div><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0;display:block;margin:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 160)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}
	subRV := sub.RenderView()
	if subRV == nil {
		t.Fatal("子 rv 为 nil")
	}
	// 找到 iframe box 的绝对位置（主文档坐标）
	frameBox := rv.FindRenderBoxForNode(iframeEl)
	if frameBox == nil {
		t.Fatal("iframe box 未渲染")
	}
	absX := frameBox.AbsoluteX()
	absY := frameBox.AbsoluteY()

	// 光标在 iframe 内部（内容框左上 10,10）→ 子 rv 光标应被设置（换算坐标）
	insideX := absX + 10
	insideY := absY + 10
	rendering.SetCursorPosRecursive(rv, insideX, insideY)
	sx, sy := subRV.CursorPos()
	if sx < 5 || sx > 15 || sy < 5 || sy > 15 {
		t.Fatalf("光标在 iframe 内：子 rv cursor=(%.0f,%.0f), want ≈(10,10)", sx, sy)
	}

	// 光标移出 iframe（负坐标区域）→ 子 rv 光标被清除（大负数）
	rendering.SetCursorPosRecursive(rv, -5, -5)
	sx2, sy2 := subRV.CursorPos()
	if sx2 > -1e8 || sy2 > -1e8 {
		t.Fatalf("光标移出 iframe：子 rv cursor=(%.0f,%.0f), want 清除(<-1e8)", sx2, sy2)
	}
	// 主 rv 光标仍记录原始坐标（不受子 Frame 传播影响）
	mx, my := rv.CursorPos()
	if mx != -5 || my != -5 {
		t.Fatalf("主 rv cursor=(%.0f,%.0f), want (-5,-5)", mx, my)
	}
}

// TestDragSelectionPlainTextLiveUpdate 验证普通文本拖拽选区的实时更新
// 链路（t1）：模拟 Host 的 Press → CursorMove → Release 状态机——
// ① Press 在文本上：HitTestText 命中 → CurrentSelection 建立（Active=true，
// 拖动中高亮跟随）；
// ② CursorMove 移动：End 实时扩展（终点可落在 iframe 子文档——跨 Frame）；
// ③ Release：Active=false（停止跟随，选区保留）。
func TestDragSelectionPlainTextLiveUpdate(t *testing.T) {
	wv := NewWebView()
	child := encDataURI(`<html><body style="margin:0"><div id="s1" style="font-size:16px">sub-one sub-two</div></body></html>`)
	src := `<html><body style="margin:0"><div id="m1" style="font-size:16px">main-one main-two</div><iframe id="f1" src="data:text/html,` + child + `" width="200" height="100" style="border:0;display:block"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 160)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	iframeEl := wv.MainFrame().Document().GetElementById("f1")
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}
	subRV := sub.RenderView()
	if subRV == nil {
		t.Fatal("子 rv 为 nil")
	}
	// 子 rv 先布局（iframe 子 Frame 独立 FrameView，主 rv 布局不覆盖）。
	if sub.NeedsLayout() {
		sub.LayoutNow()
	}
	var subTexts []*rendering.RenderText
	collectTestTexts(rendering.RenderObject(subRV), &subTexts)
	if len(subTexts) == 0 {
		t.Fatal("子文档无文本")
	}

	// ── ① Press：主文档文本上按下（Host 普通文本分支同款逻辑）──
	var mainTexts []*rendering.RenderText
	collectTestTexts(rendering.RenderObject(rv), &mainTexts)
	if len(mainTexts) == 0 {
		t.Fatal("主文档无文本")
	}
	seg0 := mainTexts[0].Segments()[0]
	anchorX := seg0.X + 2
	anchorY := seg0.Y + seg0.Height/2
	press := rendering.HitTestText(rv, anchorX, anchorY)
	if !press.IsValid() {
		t.Fatal("Press 应命中主文档文本")
	}
	rendering.CurrentSelection = &rendering.Selection{
		Start:  press,
		End:    press,
		Active: true, // 拖动中：高亮跟随
	}
	// Press 时 start==end（零宽选区）→ 无高亮矩形、无选中文本（浏览器
	// 语义：按下未拖动仅放置 caret）。选区已建立（Active=true）——
	// 拖动开始后 End 扩展才产生高亮。
	if rendering.CurrentSelection == nil || !rendering.CurrentSelection.Active {
		t.Fatal("Press 后：CurrentSelection 应建立且 Active=true（拖动中跟随）")
	}

	// ── ② CursorMove：向 iframe 子文档拖动（跨 Frame 终点）──
	// 子文档文本的绝对坐标 = 子坐标 + iframe 内容框偏移。
	frameBox := rv.FindRenderBoxForNode(iframeEl)
	if frameBox == nil {
		t.Fatal("iframe box 未渲染")
	}
	subSeg := subTexts[0].Segments()[0]
	subAbsX := subSeg.X + frameBox.AbsoluteX() + 5
	subAbsY := subSeg.Y + frameBox.AbsoluteY() + 5
	moved := rendering.HitTestText(rv, subAbsX, subAbsY)
	if !moved.IsValid() {
		t.Fatal("CursorMove 终点应命中子文档文本（跨 Frame）")
	}
	rendering.CurrentSelection.End = moved
	// 绘制一帧：Paint 入口构建 globalSelTexts（跨 Frame 全局树序），
	// 与真实 Host 的 MarkRenderTreeDirty → 下一帧绘制一致。
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !rendering.IsOffsetInSelection(subRV, subTexts[0], 0) {
		t.Fatal("跨 Frame 拖动中：子文档文本应在选区中（高亮跟随）")
	}

	// ── ③ Release：Active=false，选区保留 ──
	rendering.CurrentSelection.Active = false
	if len(rendering.SelectionRects(rv)) == 0 || len(rendering.SelectionRects(subRV)) == 0 {
		t.Fatal("Release 后：主/子 rv 应保留各自高亮段")
	}
}

// TestHoverClearedAfterRebuild 验证「无操作时 agent 输出也影响渲染」的
// 修复（t3）：鼠标悬停元素后 SetHovered(true)，随后 agent 输出触发渲染
// 树重建——重建后清除 hover（鼠标移出窗口 / 重建期间），:hover 样式必须
// 不再应用。之前 SetHovered 不 bump attrVersion，resolver 缓存不失效，
// 重建后 :hover 样式残留（无交互画面却出现 hover 高亮/按钮变色）。
func TestHoverClearedAfterRebuild(t *testing.T) {
	wv := NewWebView()
	src := `<html><head><style>
		.btn { color: #222; }
		.btn:hover { color: #ff0000; }
	</style></head><body style="margin:0"><button id="b1" class="btn" style="width:120px;height:40px">Hover Me</button></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(200, 120)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	rv := wv.MainFrame().RenderView()
	btn := wv.MainFrame().Document().GetElementById("b1")
	if btn == nil {
		t.Fatal("button not found")
	}
	// 找到 button 的 render box，读取计算样式
	ro := rv.FindRenderBoxForNode(btn)
	if ro == nil || ro.Style() == nil {
		t.Fatal("button render box 无样式")
	}
	// 初始：无 hover → #222（非红）
	if c := ro.Style().Color; c.R == 255 {
		t.Fatalf("初始 color=%v want 非红（#222）", c)
	}

	// 悬停：:hover 生效（真实流程：SetHovered → MarkRenderTreeDirty →
	// 下帧 RebuildRenderTree 重新解析样式）
	btn.SetHovered(true)
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	rv = wv.RenderView()
	if ro2 := rv.FindRenderBoxForNode(btn); ro2 != nil && ro2.Style() != nil {
		if c := ro2.Style().Color; c.R != 255 {
			t.Fatalf("hovered 时 color=%v want red", c)
		}
	}

	// 模拟 agent 输出 → 渲染树重建（新 RenderBox，resolver 重新解析）
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	rv2 := wv.RenderView()
	btn2 := wv.MainFrame().Document().GetElementById("b1")
	ro3 := rv2.FindRenderBoxForNode(btn2)
	if ro3 == nil || ro3.Style() == nil {
		t.Fatal("重建后 button 无样式")
	}
	// 重建后仍悬停（agent 输出不应清除 hover）→ 红色保留
	if c := ro3.Style().Color; c.R != 255 {
		t.Fatalf("重建后（仍悬停）color=%v want red（agent 输出不应清除 hover）", c)
	}

	// 鼠标移出窗口：SetHovered(false) → 缓存必须失效 → :hover 不再应用
	btn2.SetHovered(false)
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	rv3 := wv.RenderView()
	ro4 := rv3.FindRenderBoxForNode(btn2)
	if ro4 == nil || ro4.Style() == nil {
		t.Fatal("清除 hover 后 button 无样式")
	}
	if c := ro4.Style().Color; c.R == 255 {
		t.Fatalf("清除 hover 后 color=%v want 非红（:hover 残留！）", c)
	}
}

func encDataURI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == ' ', c == '\t', c == '\n', c == '\r':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
