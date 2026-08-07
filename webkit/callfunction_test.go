package webkit

import (
	"testing"
)

// TestCallFunction verifies the host can actively invoke page-global JS
// functions (Go → JS): name resolution with dot paths, argument conversion,
// return value, and the `this` binding (window.fn() semantics).
func TestCallFunction(t *testing.T) {
	wv := NewWebView()
	src := `<html><body><script>
		window.add = function(a, b) { return a + b; };
		window.greet = function(name) { return "hi " + name; };
		window.state = { count: 0 };
		window.bump = function(n) { this.count = (this.count || 0) + n; return this.count; };
		window.ns = { scale: function(n) { return n * 2; } };
		window.conv = function(o) { return o.x + ":" + o.y; };
	</script></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}

	// 标量参数 + 数值返回。
	v, err := wv.CallFunction("add", 2, 3)
	if err != nil {
		t.Fatalf("CallFunction(add): %v", err)
	}
	if got := v.ToNumber(); got != 5 {
		t.Errorf("add(2,3) = %v, want 5", got)
	}

	// 字符串参数 + 字符串返回。
	v, err = wv.CallFunction("greet", "wb-ui")
	if err != nil {
		t.Fatalf("CallFunction(greet): %v", err)
	}
	if got := v.ToString(); got != "hi wb-ui" {
		t.Errorf("greet = %q, want %q", got, "hi wb-ui")
	}

	// 点路径解析（window.ns.scale 定位对象方法并调用）。
	v, err = wv.CallFunction("window.ns.scale", 4)
	if err != nil {
		t.Fatalf("CallFunction(window.ns.scale): %v", err)
	}
	if got := v.ToNumber(); got != 8 {
		t.Errorf("ns.scale(4) = %v, want 8", got)
	}
	// this 绑定为全局对象（浏览器语义 window.fn() 的 this === window），
	// 且两次调用共享全局状态（this.count 持久累加）。
	v, err = wv.CallFunction("bump", 2)
	if err != nil {
		t.Fatalf("CallFunction(bump,2): %v", err)
	}
	v, err = wv.CallFunction("bump", 3)
	if err != nil {
		t.Fatalf("CallFunction(bump,3): %v", err)
	}
	if got := v.ToNumber(); got != 5 {
		t.Errorf("bump(2)+bump(3) = %v, want 5 (this 全局持久)", got)
	}

	// map[string]any 参数（Go map → JS 对象）。
	v, err = wv.CallFunction("conv", map[string]any{"x": "a", "y": "b"})
	if err != nil {
		t.Fatalf("CallFunction(conv): %v", err)
	}
	if got := v.ToString(); got != "a:b" {
		t.Errorf("conv(map) = %q, want %q", got, "a:b")
	}

	// 不存在 / 不可调用的错误路径。
	if _, err = wv.CallFunction("noSuchFn"); err == nil {
		t.Error("CallFunction(noSuchFn) should error")
	}
	if _, err = wv.CallFunction("state.count"); err == nil {
		t.Error("CallFunction(state.count) on non-callable should error")
	}
}

// TestRenderHTML verifies the declarative Go-side UI update path:
// RenderHTML(id, html) parses the template onto the element and the document
// reflects the new children (equivalent to el.innerHTML = html).
func TestRenderHTML(t *testing.T) {
	wv := NewWebView()
	src := `<html><body><div id="root">old</div></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if err := wv.RenderHTML("root", `<ul><li>a</li><li>b</li></ul>`); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	doc := wv.MainFrame().Document()
	if doc == nil {
		t.Fatal("Document() = nil")
	}
	root := doc.GetElementById("root")
	if root == nil {
		t.Fatal("root element missing")
	}
	if got := root.GetInnerHTML(); got != "<ul><li>a</li><li>b</li></ul>" {
		t.Errorf("root.innerHTML after RenderHTML = %q", got)
	}

	// 未知 id 报错。
	if err := wv.RenderHTML("missing", "<p>x</p>"); err == nil {
		t.Error("RenderHTML on missing id should error")
	}
}
