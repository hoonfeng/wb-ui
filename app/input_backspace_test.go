package app

import (
	"testing"

	"wb-ui/webkit"
)

// TestInputBackspaceIntoDOM 钉死 (e) 退格键必须真正改写 DOM 文本：
// 在 contenteditable 中键入 "hello" 后退格 3 次，textContent 必须为 "he"。
// 断言可量化：textContent 字符串 + 长度。
func TestInputBackspaceIntoDOM(t *testing.T) {
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
	// ★ 测试隔离：sstate.ranges 是 bindings 包级单例，会残留同包其它测试
	//   （TestTextEditCtrl* 的 Ctrl+A 全选等）写入的旧 range —— 不清空会让
	//   SelectionInsideElement 误判、退格走错路径（全量跑 FAIL / 单独跑 PASS 的差异源）。
	if _, err := wv.EvalJS(`(function(){ var s = window.getSelection();
    if (s && s.removeAllRanges) { s.removeAllRanges(); } return 'cleared'; })()`); err != nil {
		t.Fatalf("清理残留选区失败: %v", err)
	}
	h.imeFocusedEl = el
	for _, c := range "hello" {
		h.MockKeyChar(c)
	}
	textOf := func() string {
		v, err := wv.EvalJS(`(function(){ return document.getElementById("ed").textContent; })()`)
		if err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		return v.ToString()
	}
	typed := textOf()
	t.Logf("键入后 textContent=%q (len=%d)", typed, len(typed))
	if typed != "hello" {
		t.Fatalf("键盘输入未写入 DOM：got %q, want \"hello\"", typed)
	}
	for i := 0; i < 3; i++ {
		h.deleteFocusedChar(false)
	}
	got := textOf()
	t.Logf("退格 3 次后 textContent=%q (len=%d)", got, len(got))
	if got != "he" {
		t.Fatalf("退格未改写 DOM：got %q (len=%d), want \"he\" (len=2)", got, len(got))
	}
}
