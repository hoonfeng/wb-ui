package app

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/ime"
	"wb-ui/webkit"
)

// findElementByID 在 DOM 树中按 id 找元素（Document 无 GetElementByID 时的测试辅助）。
func findElementByID(n dom.Node, id string) *dom.Element {
	if el, ok := n.(*dom.Element); ok && el.GetAttribute("id") == id {
		return el
	}
	for _, c := range n.ChildNodes() {
		if el := findElementByID(c, id); el != nil {
			return el
		}
	}
	return nil
}

// TestContentEditableCompositionWithoutSelection 钉死：手写 contenteditable
// （无 DOM Selection）的 **IME 组合输入**必须把组合文本写入 DOM，且连续
// compositionupdate（h → he → hello）不能重复累积。
//
// 这是桌面端最基本的输入链路：真实键盘字符在 Windows 上以组合事件抵达
// （实测 IME 日志 ce-compose update="hello" from=0 len=0）。修复前组合开始处
// 因「无有效选择」把 imeCompRoot 置 nil → 组合文本永不写入 DOM。
func TestContentEditableCompositionWithoutSelection(t *testing.T) {
	wv := webkit.NewWebView()
	defer wv.Destroy()
	h := NewHostForTest(wv, 400, 200)
	if err := wv.LoadHTML(`<!DOCTYPE html><html><body style="margin:0">
<div id="ed" contenteditable="true" style="width:200px;height:40px"></div></body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 6; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	doc := wv.MainFrame().Frame().Document()
	if doc == nil {
		t.Fatal("Document 为空")
	}
	el := findElementByID(doc.Body(), "ed")
	if el == nil {
		t.Fatal("未找到 #ed 元素")
	}
	h.imeFocusedEl = el
	for _, s := range []string{"h", "he", "hello"} {
		h.ApplyIMEEventsForTest([]ime.Event{{Kind: ime.EventCompositionUpdate, Composition: s}})
	}
	v, err := wv.EvalJS(`(function(){return document.getElementById("ed").textContent;})()`)
	if err != nil {
		t.Fatalf("EvalJS: %v", err)
	}
	if got := v.ToString(); got != "hello" {
		t.Fatalf("IME 组合输入未写入 DOM：got %q, want \"hello\"（连续 update 亦不得累积重复）", got)
	}
}

// TestContentEditableCharInputWithoutSelection 钉死：**手写 contenteditable**
// （只监听 input/keydown、从不调用 selection API 的元素 —— 如 gou-ide 对话输入框
// .chat-input）在真实点击聚焦后，字符输入必须真正写入 DOM。
//
// 缺陷（修复前）：引擎只在「DOM Selection 有效」（sstate.ranges 非空）时插入文本——
// CodeMirror 6 会写 selection.collapse 所以正常，而手写 contenteditable 的
// sstate.ranges 恒为空 → InsertTextAtSelection 返回 false → 字符被丢弃，只派发一个
// input 事件（实测：ev:input 触发 5 次而 DOM 文本长度 0，用户表现为「打字无反应」）。
func TestContentEditableCharInputWithoutSelection(t *testing.T) {
	wv := webkit.NewWebView()
	defer wv.Destroy()
	h := NewHostForTest(wv, 400, 200)
	if err := wv.LoadHTML(`<!DOCTYPE html><html><body style="margin:0">
<div id="ed" contenteditable="true" style="width:200px;height:40px"></div></body></html>`); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 6; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	// ★ 精确复现真实状态：点击手写 contenteditable 后引擎已设置 imeFocusedEl，
	//   但页面从不调用 selection API → sstate.ranges 为空。
	//   （不用 h.FocusElement：那条测试路径会顺带 collapse 填充 ranges，
	//    从而绕过本次要钉死的缺陷。NewHostForTest 也不注册 FocusBridge。）
	doc := wv.MainFrame().Frame().Document()
	if doc == nil {
		t.Fatal("Document 为空")
	}
	el := findElementByID(doc.Body(), "ed")
	if el == nil {
		t.Fatal("未找到 #ed 元素")
	}
	h.imeFocusedEl = el
	for _, c := range "hello" {
		h.MockKeyChar(c)
	}
	v, err := wv.EvalJS(`(function(){var e=document.getElementById("ed");return e.textContent;})()`)
	if err != nil {
		t.Fatalf("EvalJS: %v", err)
	}
	got := v.ToString()
	want := "hello"
	if got != want {
		t.Fatalf("contenteditable 字符输入未写入 DOM：\n  got  %s\n  want %s", got, want)
	}
}
