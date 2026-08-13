package webkit

import (
	"testing"
)

// BenchmarkEvalJS 测 LoadHTML 之后单次 EvalJS 的耗时。
// 优化前：每次 EvalJS 都重复执行 RegisterDOMBindings（幂等分支 wrapDocument
// + applyCanvas2DPatch 的 RunJS）+ injectRenderTreeBridge（重赋值 10+ 包级闭包）。
// 优化后：domBindingsInjected 置位后跳过，仅剩 ensureJSRuntime + RunJS。
func BenchmarkEvalJS(b *testing.B) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><body><div id='x'>hello</div></body></html>"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := wv.EvalJS("1 + 2"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEvalJSWithReinject 测「强制每次重新注入」的耗时（模拟优化前行为），
// 与 BenchmarkEvalJS 对比可量化去重收益。
func BenchmarkEvalJSWithReinject(b *testing.B) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><body><div id='x'>hello</div></body></html>"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 强制模拟优化前：每次 EvalJS 都重新注入。
		wv.domBindingsInjected = false
		if _, err := wv.EvalJS("1 + 2"); err != nil {
			b.Fatal(err)
		}
	}
}

// TestEvalJSDocumentSingleton 验证优化后 document 在多次 EvalJS 间是稳定单例
// （浏览器语义：document 是全局单例，不应每次 EvalJS 换成新对象）。
func TestEvalJSDocumentSingleton(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><body><div id='x'>hi</div></body></html>"); err != nil {
		t.Fatal(err)
	}
	if _, err := wv.EvalJS("window.__d1 = document;"); err != nil {
		t.Fatal(err)
	}
	rv, err := wv.EvalJS("window.__d1 === document")
	if err != nil {
		t.Fatal(err)
	}
	if !rv.ToBoolean() {
		t.Error("document 应在多次 EvalJS 间保持单例，但 === 为 false")
	}
	// 确认 DOM API 仍可用（getElementById 正常）。
	rv2, err := wv.EvalJS("document.getElementById('x').id")
	if err != nil {
		t.Fatal(err)
	}
	if rv2.ToString() != "x" {
		t.Errorf("getElementById('x').id = %q, want x", rv2.ToString())
	}
}
