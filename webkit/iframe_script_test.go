package webkit

import (
	"fmt"
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/page"
	"wb-ui/rendering"
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

// encDataURI 把子文档 HTML 转成安全 data URI：除 alphanumeric/空白外全部
// URL 编码。iframe src 属性值由双引号界定，HTML 里 `"marker"` 的未编码
// 双引号会截断属性值（HTML 规范行为）——之前的 ReplaceAll 只编码 <> 漏掉
// 了引号，导致 data URI 被截断、子文档解析为空。
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
