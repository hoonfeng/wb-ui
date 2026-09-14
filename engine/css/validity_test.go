// Tests for the constraint-validation pseudo-classes :valid / :invalid /
// :in-range / :out-of-range. They used to sit in the "constraint-validation
// pseudo-classes are not modeled" bucket and always returned false, even though
// the backing state (html5.ValidityState + the min/max range limits) is fully
// computed in this port. The css package must not import html5 (html5 imports
// css for its UA stylesheet), so the checker consumes an injected resolver —
// here a fake one drives the matching logic directly.

package css

import (
	"testing"

	"wb-ui/engine/dom"
)

// withFakeValidity installs a table-backed validity resolver for one test.
func withFakeValidity(t *testing.T, m map[*dom.Element]FormValidity) {
	t.Helper()
	prev := formValidityResolver
	SetFormValidityResolver(func(el *dom.Element) (FormValidity, bool) {
		fv, ok := m[el]
		return fv, ok
	})
	t.Cleanup(func() { SetFormValidityResolver(prev) })
}

func TestSelector_ValidInvalidPseudoClasses(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	validSel := mustParseOneSelector(t, ":valid")
	invalidSel := mustParseOneSelector(t, ":invalid")

	okEl := dom.NewElement(doc, "input")
	body.AppendChild(okEl)
	badEl := dom.NewElement(doc, "input")
	body.AppendChild(badEl)
	barredEl := dom.NewElement(doc, "input")
	body.AppendChild(barredEl)
	other := dom.NewElement(doc, "div")
	body.AppendChild(other)

	withFakeValidity(t, map[*dom.Element]FormValidity{
		okEl:     {WillValidate: true, Valid: true},
		badEl:    {WillValidate: true, Valid: false},
		barredEl: {WillValidate: false, Valid: false},
	})

	if !c.Match(validSel, okEl) || c.Match(invalidSel, okEl) {
		t.Error("无约束错误且是 candidate 的元素应匹配 :valid 且不匹配 :invalid")
	}
	if c.Match(validSel, badEl) || !c.Match(invalidSel, badEl) {
		t.Error("有约束错误的元素应匹配 :invalid 且不匹配 :valid")
	}
	// barred（disabled/readonly/datalist 后代……）：两者都不匹配。
	if c.Match(validSel, barredEl) || c.Match(invalidSel, barredEl) {
		t.Error("barred 元素不应匹配 :valid 也不应匹配 :invalid")
	}
	// 不参与约束校验的元素（div、output……）：两者都不匹配。
	if c.Match(validSel, other) || c.Match(invalidSel, other) {
		t.Error("非表单控件不应匹配 :valid/:invalid")
	}
}

func TestSelector_ValidInvalidWithoutResolver(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	validSel := mustParseOneSelector(t, ":valid")
	invalidSel := mustParseOneSelector(t, ":invalid")

	el := dom.NewElement(doc, "input")
	body.AppendChild(el)

	// 未注入实现（单独使用 css 包）：伪类永不匹配（保持「未建模」行为）。
	prev := formValidityResolver
	SetFormValidityResolver(nil)
	t.Cleanup(func() { SetFormValidityResolver(prev) })

	if c.Match(validSel, el) || c.Match(invalidSel, el) {
		t.Error("未注入约束校验实现时 :valid/:invalid 都不应匹配")
	}
}

func TestSelector_InRangeOutOfRangePseudoClasses(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	inSel := mustParseOneSelector(t, ":in-range")
	outSel := mustParseOneSelector(t, ":out-of-range")

	inEl := dom.NewElement(doc, "input")
	body.AppendChild(inEl)
	outEl := dom.NewElement(doc, "input")
	body.AppendChild(outEl)
	unlimitedEl := dom.NewElement(doc, "input")
	body.AppendChild(unlimitedEl)
	barredEl := dom.NewElement(doc, "input")
	body.AppendChild(barredEl)

	withFakeValidity(t, map[*dom.Element]FormValidity{
		inEl:        {WillValidate: true, Valid: true, RangeLimited: true, OutOfRange: false},
		outEl:       {WillValidate: true, Valid: false, RangeLimited: true, OutOfRange: true},
		unlimitedEl: {WillValidate: true, Valid: true, RangeLimited: false},
		barredEl:    {WillValidate: false, RangeLimited: true, OutOfRange: true},
	})

	if !c.Match(inSel, inEl) || c.Match(outSel, inEl) {
		t.Error("有范围限制且未越界 → 应匹配 :in-range")
	}
	if c.Match(inSel, outEl) || !c.Match(outSel, outEl) {
		t.Error("越界 → 应匹配 :out-of-range")
	}
	// 没有范围限制（无 min/max，或类型不支持）：两者都不匹配。
	if c.Match(inSel, unlimitedEl) || c.Match(outSel, unlimitedEl) {
		t.Error("无范围限制的元素 :in-range/:out-of-range 都不应匹配")
	}
	// barred 元素即使 min/max 越界也不匹配（不参与约束校验）。
	if c.Match(inSel, barredEl) || c.Match(outSel, barredEl) {
		t.Error("barred 元素 :in-range/:out-of-range 都不应匹配")
	}
}

// TestSelector_UserValidUserInvalidPseudoClasses 覆盖 :user-valid /
// :user-invalid：与 :valid/:invalid 的唯一差别是要求 user validity（「用户
// 交互过」）为 true——用户没交互过时两者都不匹配（否则页面一打开就把所有
// 必填项标红）。
func TestSelector_UserValidUserInvalidPseudoClasses(t *testing.T) {
	doc, body := newFormStateDoc(t)
	c := NewSelectorChecker()
	uv := mustParseOneSelector(t, ":user-valid")
	ui := mustParseOneSelector(t, ":user-invalid")

	untouched := dom.NewElement(doc, "input")
	body.AppendChild(untouched)
	touchedValid := dom.NewElement(doc, "input")
	body.AppendChild(touchedValid)
	touchedInvalid := dom.NewElement(doc, "input")
	body.AppendChild(touchedInvalid)
	barred := dom.NewElement(doc, "input")
	body.AppendChild(barred)

	withFakeValidity(t, map[*dom.Element]FormValidity{
		untouched:      {WillValidate: true, Valid: false, UserInteracted: false},
		touchedValid:   {WillValidate: true, Valid: true, UserInteracted: true},
		touchedInvalid: {WillValidate: true, Valid: false, UserInteracted: true},
		barred:         {WillValidate: false, Valid: true, UserInteracted: true},
	})

	if c.Match(uv, untouched) || c.Match(ui, untouched) {
		t.Error("未交互的元素 :user-valid/:user-invalid 都不应匹配")
	}
	if !c.Match(uv, touchedValid) || c.Match(ui, touchedValid) {
		t.Error("交互过且有效的元素应匹配 :user-valid")
	}
	if c.Match(uv, touchedInvalid) || !c.Match(ui, touchedInvalid) {
		t.Error("交互过且无效的元素应匹配 :user-invalid")
	}
	if c.Match(uv, barred) || c.Match(ui, barred) {
		t.Error("barred 元素 :user-valid/:user-invalid 都不应匹配")
	}
}

// TestPseudoClass_EnumNameRoundTrip 覆盖「枚举 ↔ 名称 ↔ 解析」三向一致性：
// 每个枚举都必须有名称、名称能解析回原枚举（防止新增枚举时漏改名称表或解析
// 表——上一批 PseudoElementName 漏掉 -webkit-scrollbar* 就是这类漂移）。
func TestPseudoClass_EnumNameRoundTrip(t *testing.T) {
	for pc := PseudoClassUnknown + 1; pc <= PseudoClassUserInvalid; pc++ {
		name := PseudoClassName(pc)
		if name == "" {
			t.Errorf("PseudoClass(%d) 没有名称：PseudoClassName 漏了这个枚举", pc)
			continue
		}
		if got := LookupPseudoClass(name); got != pc {
			t.Errorf("LookupPseudoClass(%q) = %d, want %d", name, got, pc)
		}
	}
	if name := PseudoClassName(PseudoClassUnknown); name != "" {
		t.Errorf("PseudoClassName(Unknown) = %q, want \"\"", name)
	}
}
