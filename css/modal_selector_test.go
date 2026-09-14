// Tests for the :modal pseudo-class (HTML rendering / Selectors-4): it matches an
// element that is in the modal state — in this port only <dialog> opened through
// showModal().

package css

import (
	"testing"

	"wb-ui/dom"
)

func TestSelector_ModalPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)
	dlg := dom.NewElement(doc, "dialog")
	body.AppendChild(dlg)
	nonModal := dom.NewElement(doc, "dialog")
	body.AppendChild(nonModal)
	div := dom.NewElement(doc, "div")
	div.SetModalState(true) // 非 dialog 元素即使带模态状态也不匹配
	body.AppendChild(div)

	c := NewSelectorChecker()
	modal := mustParseOneSelector(t, ":modal")
	if got := modal.String(); got != ":modal" {
		t.Fatalf("ComplexSelector.String() = %q，want \":modal\"", got)
	}

	// 未打开：不匹配。
	if c.Match(modal, dlg) {
		t.Fatal("未打开的 dialog 不应匹配 :modal")
	}

	// show() 打开（非模态）：不匹配。
	nonModal.SetAttribute("open", "")
	if c.Match(modal, nonModal) {
		t.Fatal("非模态打开的 dialog 不应匹配 :modal")
	}

	// showModal() 打开：匹配。
	dlg.SetAttribute("open", "")
	dlg.SetModalState(true)
	if !c.Match(modal, dlg) {
		t.Fatal("模态打开的 dialog 应匹配 :modal")
	}

	// 移除 open 属性**不**退出模态状态（HTML §4.11.6：模态状态由 close() 或
	// 元素移除步骤清除，移除属性只让 dialog 不再显示）→ :modal 继续匹配。
	dlg.RemoveAttribute("open")
	if !c.Match(modal, dlg) {
		t.Fatal("open 属性移除后仍处于模态状态，应继续匹配 :modal")
	}
	// 模态状态被清除（close() 的「set is modal to false」）：不再匹配。
	dlg.SetModalState(false)
	if c.Match(modal, dlg) {
		t.Fatal("模态状态清除后不应匹配 :modal")
	}

	// 非 dialog 元素永不匹配。
	if c.Match(modal, div) {
		t.Fatal("<div> 不应匹配 :modal")
	}
	if c.Match(modal, body) {
		t.Fatal("<body> 不应匹配 :modal")
	}
}
