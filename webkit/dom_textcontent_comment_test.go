package webkit

import "testing"

// TestContainerTextContentExcludesComments 钉死 DOM 规范行为：
// 容器的 textContent 只拼接 **Text 节点后代**的 data，注释(Comment)与处理指令
// (ProcessingInstruction) **不参与**拼接；而注释节点自身的 textContent 仍是其 data。
//
// 缺陷背景（2026-09-25 定位）：TextContent 曾把注释数据一并拼入 → 
// renderformcontrol.go 的 paintButtonText 用 el.TextContent() 当按钮文字绘制 →
// gou-ide 图标按钮（<button><svg><!-- Folder -->…</svg></button>）把注释原文
// 画在图标上（界面出现 "Folder"/"Chevron Down (Rotated chevron-right)" 等文字污染）。
func TestContainerTextContentExcludesComments(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	if err := wv.LoadHTML(`<!DOCTYPE html><html><body>
<div id="d">a<!-- SECRET -->b</div>
<div id="e">x</div>
</body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}

	if got := dynEval(t, wv, `document.getElementById("d").textContent`); got != "ab" {
		t.Fatalf("容器 textContent 含注释：got %q, want %q", got, "ab")
	}
	// 注释节点自身的 textContent 仍是其 data（Node.textContent 语义）。
	if got := dynEval(t, wv, `document.getElementById("d").childNodes[1].textContent`); got != " SECRET " {
		t.Fatalf("注释自身 textContent 错误：got %q, want %q", got, " SECRET ")
	}
	// 嵌套注释同样不参与。
	if err := wv.LoadHTML(`<!DOCTYPE html><html><body><div id="f"><span>y<!-- C -->z</span></div></body></html>`); err != nil {
		t.Fatalf("LoadHTML2: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render2: %v", err)
		}
	}
	if got := dynEval(t, wv, `document.getElementById("f").textContent`); got != "yz" {
		t.Fatalf("嵌套注释参与了容器 textContent：got %q, want %q", got, "yz")
	}
}

// TestButtonWithIconCommentPaintsNoText：按钮内 svg 图标的注释不得被
// paintButtonText 当作按钮文字绘制（像素级验证 —— 带注释与不带注释必须完全一致）。
func TestButtonWithIconCommentPaintsNoText(t *testing.T) {
	page := func(mid string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<button style="width:120px;height:40px;background:#ffffff;border:0;padding:0"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#000000" stroke-width="2">` + mid + `<path d="M6 9 12 15 18 9"/></svg></button>
</body></html>`
	}
	a := renderPagePixels(t, page(`<!-- Chevron Down (Rotated chevron-right) -->`), 160, 60)
	b := renderPagePixels(t, page(``), 160, 60)
	if !pixelsEqual(a, b) {
		t.Fatalf("按钮内 svg 注释被绘制成按钮文字（paintButtonText 读 TextContent 含注释）")
	}
}
