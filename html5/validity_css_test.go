// 端到端接线测试：html5 的真实表单元素状态（required/pattern/min/max/
// readonly/disabled）经 init 注入的 resolver 影响 css 选择器对
// :valid / :invalid / :in-range / :out-of-range 的匹配结果。

package html5

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
)

func validityTestDoc(t *testing.T) *dom.Element {
	t.Helper()
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	if err := doc.AppendChild(html); err != nil {
		t.Fatalf("AppendChild(html): %v", err)
	}
	body := dom.NewElement(doc, "body")
	if err := html.AppendChild(body); err != nil {
		t.Fatalf("AppendChild(body): %v", err)
	}
	return body
}

func parseOneSelector(t *testing.T, src string) css.ComplexSelector {
	t.Helper()
	list := css.NewParser(src).ParseSelectorList()
	if list == nil || len(list.Selectors) != 1 {
		t.Fatalf("解析 %q 失败", src)
	}
	return list.Selectors[0]
}

func TestValidityPseudoClasses_EndToEnd(t *testing.T) {
	body := validityTestDoc(t)
	c := css.NewSelectorChecker()
	valid := parseOneSelector(t, ":valid")
	invalid := parseOneSelector(t, ":invalid")
	inRange := parseOneSelector(t, ":in-range")
	outOfRange := parseOneSelector(t, ":out-of-range")

	newInput := func(attrs map[string]string) *dom.Element {
		el := dom.NewElement(body.OwnerDocument(), "input")
		for k, v := range attrs {
			el.SetAttribute(k, v)
		}
		if err := body.AppendChild(el); err != nil {
			t.Fatalf("AppendChild(input): %v", err)
		}
		return el
	}

	// required：空 → :invalid；填入值 → :valid。
	req := newInput(map[string]string{"type": "text", "required": "required"})
	if c.Match(valid, req) || !c.Match(invalid, req) {
		t.Fatal("required 且为空：应匹配 :invalid 且不匹配 :valid")
	}
	req.SetAttribute("value", "hello")
	if !c.Match(valid, req) || c.Match(invalid, req) {
		t.Fatal("required 且有值：应匹配 :valid")
	}

	// pattern：不匹配 → :invalid。
	pat := newInput(map[string]string{"type": "text", "pattern": `\d{3}`, "value": "abc"})
	if !c.Match(invalid, pat) {
		t.Fatal("pattern 不匹配：应匹配 :invalid")
	}
	pat.SetAttribute("value", "123")
	if !c.Match(valid, pat) {
		t.Fatal("pattern 匹配：应匹配 :valid")
	}

	// type=email：格式错误 → :invalid。
	em := newInput(map[string]string{"type": "email", "value": "not-an-email"})
	if !c.Match(invalid, em) {
		t.Fatal("email 格式错误：应匹配 :invalid")
	}
	em.SetAttribute("value", "user@example.com")
	if !c.Match(valid, em) {
		t.Fatal("email 合法：应匹配 :valid")
	}

	// min/max：无范围限制的元素 :in-range/:out-of-range 都不匹配；
	// 越界 → :out-of-range（同时 :invalid），范围内的值 → :in-range。
	noRange := newInput(map[string]string{"type": "text", "value": "abc"})
	if c.Match(inRange, noRange) || c.Match(outOfRange, noRange) {
		t.Fatal("text 的 min/max 不构成范围限制：:in-range/:out-of-range 都不应匹配")
	}
	num := newInput(map[string]string{"type": "number", "min": "5", "max": "10", "value": "3"})
	if !c.Match(outOfRange, num) || c.Match(inRange, num) {
		t.Fatal("number 低于 min：应匹配 :out-of-range")
	}
	if !c.Match(invalid, num) {
		t.Fatal("number 低于 min：也应匹配 :invalid")
	}
	num.SetAttribute("value", "7")
	if !c.Match(inRange, num) || c.Match(outOfRange, num) {
		t.Fatal("number 在范围内：应匹配 :in-range")
	}
	if !c.Match(valid, num) {
		t.Fatal("number 在范围内：也应匹配 :valid")
	}

	// readonly（barred）：既非 :valid 也非 :invalid。
	ro := newInput(map[string]string{"type": "text", "required": "required", "readonly": "readonly"})
	if c.Match(valid, ro) || c.Match(invalid, ro) {
		t.Fatal("readonly 元素不参与约束校验：:valid/:invalid 都不应匹配")
	}
	// disabled 同理。
	dis := newInput(map[string]string{"type": "text", "required": "required", "disabled": "disabled"})
	if c.Match(valid, dis) || c.Match(invalid, dis) {
		t.Fatal("disabled 元素不参与约束校验：:valid/:invalid 都不应匹配")
	}

	// 空值但有范围限制：不越界（不匹配 :out-of-range），匹配 :in-range。
	empty := newInput(map[string]string{"type": "number", "min": "5"})
	if c.Match(outOfRange, empty) || !c.Match(inRange, empty) {
		t.Fatal("空值不越界：应匹配 :in-range 且不匹配 :out-of-range")
	}

	// step mismatch 属于 :invalid，但不算 :out-of-range。注意 step base 的优先
	// 次序是 min → value 内容属性 → 0（HTML §4.10.5.3.8 / MDN step）：有 min
	// 时 3 相对 base 0 不是 2 的整数倍 → mismatch。
	step := newInput(map[string]string{"type": "number", "min": "0", "step": "2", "value": "3"})
	if !c.Match(invalid, step) {
		t.Fatal("step mismatch：应匹配 :invalid")
	}
	if c.Match(outOfRange, step) {
		t.Fatal("step mismatch 不是 underflow/overflow：不应匹配 :out-of-range")
	}
	// 没有 min 时 step base 就是 value 内容属性本身（13 与自身同余）→ 不 mismatch。
	stepOnValueBase := newInput(map[string]string{"type": "number", "step": "2", "value": "13"})
	if c.Match(invalid, stepOnValueBase) {
		t.Fatal("step base 取 value 属性时 13 是 2 的整数倍相位：不应匹配 :invalid")
	}

	// select / textarea 也参与（required）。
	sel := dom.NewElement(body.OwnerDocument(), "select")
	sel.SetAttribute("required", "required")
	_ = sel.SetInnerHTML(`<option value=""></option><option value="a">a</option>`)
	if err := body.AppendChild(sel); err != nil {
		t.Fatalf("AppendChild(select): %v", err)
	}
	if c.Match(valid, sel) || !c.Match(invalid, sel) {
		t.Fatal("required 且未选择：应匹配 :invalid")
	}
	ta := dom.NewElement(body.OwnerDocument(), "textarea")
	ta.SetAttribute("required", "required")
	if err := body.AppendChild(ta); err != nil {
		t.Fatalf("AppendChild(textarea): %v", err)
	}
	if !c.Match(invalid, ta) {
		t.Fatal("required 空 textarea：应匹配 :invalid")
	}
}
