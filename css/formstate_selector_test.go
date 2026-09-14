// Tests for the form-state pseudo-classes :default and :indeterminate
// (HTML §4.16.2 / §4.16.3). Both used to fall into the "constraint-validation
// pseudo-classes are not modeled" bucket and always returned false even though
// the state they need (checked/selected attributes, the input.indeterminate IDL
// flag, <progress> without a value) is fully available in this port.

package css

import (
	"testing"

	"wb-ui/dom"
)

func newFormStateDoc(t *testing.T) (*dom.Document, *dom.Element) {
	t.Helper()
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)
	return doc, body
}

func TestSelector_IndeterminatePseudoClass(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	sel := mustParseOneSelector(t, ":indeterminate")

	// checkbox：由 input.indeterminate（IDL，非内容属性）驱动。
	cb := dom.NewElement(doc, "input")
	cb.SetAttribute("type", "checkbox")
	body.AppendChild(cb)
	if c.Match(sel, cb) {
		t.Fatal("未设置 indeterminate 的 checkbox 不应匹配 :indeterminate")
	}
	cb.SetIndeterminate(true)
	if !c.Match(sel, cb) {
		t.Fatal("indeterminate = true 的 checkbox 应匹配 :indeterminate")
	}
	cb.SetIndeterminate(false)
	if c.Match(sel, cb) {
		t.Fatal("清零后不应再匹配 :indeterminate")
	}

	// 非 checkbox 的 input 即使带标志也不匹配（浏览器同此）。
	txt := dom.NewElement(doc, "input")
	body.AppendChild(txt)
	txt.SetIndeterminate(true)
	if c.Match(sel, txt) {
		t.Fatal("text input 不应匹配 :indeterminate")
	}

	// radio：同组全部未勾选 → 整组匹配；有任何一个 checked → 全组不匹配。
	r1 := dom.NewElement(doc, "input")
	r1.SetAttribute("type", "radio")
	r1.SetAttribute("name", "g")
	body.AppendChild(r1)
	r2 := dom.NewElement(doc, "input")
	r2.SetAttribute("type", "radio")
	r2.SetAttribute("name", "g")
	body.AppendChild(r2)
	if !c.Match(sel, r1) || !c.Match(sel, r2) {
		t.Fatal("组内无选中时两个 radio 都应匹配 :indeterminate")
	}
	r2.SetAttribute("checked", "")
	if c.Match(sel, r1) || c.Match(sel, r2) {
		t.Fatal("组内已有选中项时不应匹配 :indeterminate")
	}
	// 不同 name → 不同组。
	r3 := dom.NewElement(doc, "input")
	r3.SetAttribute("type", "radio")
	r3.SetAttribute("name", "other")
	body.AppendChild(r3)
	if !c.Match(sel, r3) {
		t.Fatal("另一组的 radio 不受 g 组选中影响")
	}
	// name 为空 → 自成一组。
	solo := dom.NewElement(doc, "input")
	solo.SetAttribute("type", "radio")
	body.AppendChild(solo)
	if !c.Match(sel, solo) {
		t.Fatal("无 name 的 radio 自成一组，应匹配 :indeterminate")
	}
	solo.SetAttribute("checked", "")
	if c.Match(sel, solo) {
		t.Fatal("自身 checked 后不应匹配 :indeterminate")
	}

	// <progress>：没有 value 属性即是 indeterminate。
	prog := dom.NewElement(doc, "progress")
	prog.SetAttribute("max", "100")
	body.AppendChild(prog)
	if !c.Match(sel, prog) {
		t.Fatal("无 value 的 progress 应匹配 :indeterminate")
	}
	prog.SetAttribute("value", "30")
	if c.Match(sel, prog) {
		t.Fatal("有 value 的 progress 不应匹配 :indeterminate")
	}

	// 无关元素。
	div := dom.NewElement(doc, "div")
	body.AppendChild(div)
	if c.Match(sel, div) {
		t.Fatal("<div> 不应匹配 :indeterminate")
	}
}

func TestSelector_DefaultPseudoClass(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	sel := mustParseOneSelector(t, ":default")

	// 已勾选的 checkbox / radio。
	cb := dom.NewElement(doc, "input")
	cb.SetAttribute("type", "checkbox")
	body.AppendChild(cb)
	if c.Match(sel, cb) {
		t.Fatal("未勾选的 checkbox 不应匹配 :default")
	}
	cb.SetAttribute("checked", "")
	if !c.Match(sel, cb) {
		t.Fatal("已勾选的 checkbox 应匹配 :default")
	}

	// 表单默认按钮：树序第一个 submit 型控件。
	form := dom.NewElement(doc, "form")
	body.AppendChild(form)
	text := dom.NewElement(doc, "input")
	form.AppendChild(text) // 非按钮控件不算
	sub1 := dom.NewElement(doc, "button") // button 缺省 type = submit
	form.AppendChild(sub1)
	sub2 := dom.NewElement(doc, "input")
	sub2.SetAttribute("type", "submit")
	form.AppendChild(sub2)
	rst := dom.NewElement(doc, "input")
	rst.SetAttribute("type", "reset")
	form.AppendChild(rst)
	plain := dom.NewElement(doc, "input")
	plain.SetAttribute("type", "button")
	form.AppendChild(plain)

	if !c.Match(sel, sub1) {
		t.Fatal("form 内第一个 submit 按钮应匹配 :default")
	}
	if c.Match(sel, sub2) {
		t.Fatal("第二个 submit 按钮不匹配 :default")
	}
	if !c.Match(sel, rst) {
		t.Fatal("form 内第一个 reset 按钮应匹配 :default（与 submit 分开计数）")
	}
	if c.Match(sel, plain) {
		t.Fatal("type=button 不匹配 :default")
	}

	// 表单外的提交按钮没有 form owner → 不是默认按钮。
	loose := dom.NewElement(doc, "input")
	loose.SetAttribute("type", "submit")
	body.AppendChild(loose)
	if c.Match(sel, loose) {
		t.Fatal("无 form owner 的 submit 按钮不应匹配 :default")
	}
	// 第二个 form 各自有自己的默认按钮。
	form2 := dom.NewElement(doc, "form")
	body.AppendChild(form2)
	sub3 := dom.NewElement(doc, "input")
	sub3.SetAttribute("type", "image") // image 亦属 submit 型
	form2.AppendChild(sub3)
	if !c.Match(sel, sub3) {
		t.Fatal("第二个 form 的 image 提交按钮应匹配 :default")
	}
	if !c.Match(sel, sub1) {
		t.Fatal("默认按钮判定按 form 独立")
	}

	// 已选中的 option。
	selEl := dom.NewElement(doc, "select")
	body.AppendChild(selEl)
	opt := dom.NewElement(doc, "option")
	selEl.AppendChild(opt)
	if c.Match(sel, opt) {
		t.Fatal("未选中的 option 不应匹配 :default")
	}
	opt.SetAttribute("selected", "")
	if !c.Match(sel, opt) {
		t.Fatal("已选中的 option 应匹配 :default")
	}
}
