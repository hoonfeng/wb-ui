package html5

import (
	"testing"

	"wb-ui/engine/dom"
)

// ─── 候选资格（barred from constraint validation，HTML §4.10.21.1）─────
//
// barred 的条件散落在各元素定义里：disabled；input/textarea 的 readonly
// （只对支持 readonly 的类型生效）；input type=hidden/reset/button；
// button type=reset/button；<datalist> 后代。

func TestConstraintValidity_BarredInputs(t *testing.T) {
	cases := []struct {
		label string
		attrs map[string]string
		want  bool
	}{
		{"text", map[string]string{"type": "text"}, true},
		{"text disabled", map[string]string{"type": "text", "disabled": "disabled"}, false},
		{"text readonly", map[string]string{"type": "text", "readonly": "readonly"}, false},
		{"email readonly", map[string]string{"type": "email", "readonly": "readonly"}, false},
		{"number readonly", map[string]string{"type": "number", "readonly": "readonly"}, false},
		// readonly 对不支持它的类型没有意义 → 仍然是候选。
		{"checkbox readonly", map[string]string{"type": "checkbox", "readonly": "readonly"}, true},
		{"range readonly", map[string]string{"type": "range", "readonly": "readonly"}, true},
		{"file readonly", map[string]string{"type": "file", "readonly": "readonly"}, true},
		{"hidden", map[string]string{"type": "hidden"}, false},
		{"reset", map[string]string{"type": "reset"}, false},
		{"button", map[string]string{"type": "button"}, false},
		{"submit", map[string]string{"type": "submit"}, true},
		{"image", map[string]string{"type": "image"}, true},
	}
	for _, c := range cases {
		in, _ := ToInputElement(createElement("input", c.attrs))
		if got := in.WillValidate(); got != c.want {
			t.Errorf("input %s WillValidate() = %v, want %v", c.label, got, c.want)
		}
	}
}

func TestConstraintValidity_BarredMisc(t *testing.T) {
	// <datalist> 后代：元素只是选项来源，不是用户输入控件。
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	_ = dl.AppendChild(in)
	dlInput, _ := ToInputElement(in)
	if dlInput.WillValidate() {
		t.Error("datalist 内的 input WillValidate() = true, want false")
	}

	// textarea: readonly / disabled 都排除。
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{"readonly": "readonly"}))
	if ta.WillValidate() {
		t.Error("readonly textarea WillValidate() = true, want false")
	}
	ta2, _ := ToTextAreaElement(createElement("textarea", nil))
	if !ta2.WillValidate() {
		t.Error("普通 textarea WillValidate() = false, want true")
	}

	// select: 没有 readonly 属性，只有 disabled 排除。
	sel, _ := ToSelectElement(createElement("select", map[string]string{"readonly": "readonly"}))
	if !sel.WillValidate() {
		t.Error("select 不支持 readonly；WillValidate() = false, want true")
	}
	sel2, _ := ToSelectElement(createElement("select", map[string]string{"disabled": "disabled"}))
	if sel2.WillValidate() {
		t.Error("disabled select WillValidate() = true, want false")
	}

	// button: type=submit（含默认）是候选，reset/button 排除。
	btnCases := []struct {
		attrs map[string]string
		want  bool
	}{
		{nil, true},
		{map[string]string{"type": "submit"}, true},
		{map[string]string{"type": "reset"}, false},
		{map[string]string{"type": "button"}, false},
		{map[string]string{"disabled": "disabled"}, false},
	}
	for _, c := range btnCases {
		b, _ := ToButtonElement(createElement("button", c.attrs))
		if got := b.WillValidate(); got != c.want {
			t.Errorf("button %v WillValidate() = %v, want %v", c.attrs, got, c.want)
		}
	}
}

func TestConstraintValidity_NonFormControl(t *testing.T) {
	for _, tag := range []string{"div", "span", "output", "fieldset", "progress"} {
		if _, ok := ConstraintValidity(createElement(tag, nil)); ok {
			t.Errorf("<%s> ConstraintValidity ok = true, want false", tag)
		}
	}
	if _, ok := ConstraintValidity(nil); ok {
		t.Error("ConstraintValidity(nil) ok = true, want false")
	}
}

// ─── 范围限制与越界（:in-range / :out-of-range 的判定源）─────────────

func TestConstraintValidity_RangeState(t *testing.T) {
	cases := []struct {
		label        string
		attrs        map[string]string
		wantLimited  bool
		wantOutRange bool
	}{
		{"number min only, below", map[string]string{"type": "number", "min": "5", "value": "3"}, true, true},
		{"number min only, above", map[string]string{"type": "number", "min": "5", "value": "7"}, true, false},
		{"number min only, empty value", map[string]string{"type": "number", "min": "5"}, true, false},
		{"number max only, above", map[string]string{"type": "number", "max": "5", "value": "7"}, true, true},
		{"number no min/max", map[string]string{"type": "number", "value": "7"}, false, false},
		{"number unparsable min", map[string]string{"type": "number", "min": "abc", "value": "7"}, false, false},
		// min/max 对 text 类型没有意义 → 不构成范围限制。
		{"text min/max", map[string]string{"type": "text", "min": "5", "max": "9", "value": "3"}, false, false},
		// step mismatch 是 :invalid 的成因，但不算 out-of-range。
		{"number step mismatch", map[string]string{"type": "number", "step": "2", "value": "3"}, false, false},
		// 日期类类型：min/max 按类型解析后比较。
		{"date below min", map[string]string{"type": "date", "min": "2024-01-10", "value": "2024-01-05"}, true, true},
		{"date above max", map[string]string{"type": "date", "max": "2024-01-10", "value": "2025-01-05"}, true, true},
		{"date inside", map[string]string{"type": "date", "min": "2024-01-01", "max": "2024-12-31", "value": "2024-06-01"}, true, false},
		{"month below min", map[string]string{"type": "month", "min": "2024-05", "value": "2024-03"}, true, true},
		{"time below min", map[string]string{"type": "time", "min": "10:00", "value": "09:00"}, true, true},
		{"datetime-local below min", map[string]string{"type": "datetime-local", "min": "2024-01-10T10:00", "value": "2024-01-10T09:00"}, true, true},
		{"week below min", map[string]string{"type": "week", "min": "2024-W10", "value": "2024-W02"}, true, true},
		{"week equal min", map[string]string{"type": "week", "min": "2024-W02", "value": "2024-W02"}, true, false},
	}
	for _, c := range cases {
		el := createElement("input", c.attrs)
		st, ok := ConstraintValidity(el)
		if !ok {
			t.Fatalf("%s: ConstraintValidity ok = false, want true", c.label)
		}
		if st.RangeLimited != c.wantLimited {
			t.Errorf("%s: RangeLimited = %v, want %v", c.label, st.RangeLimited, c.wantLimited)
		}
		if st.OutOfRange != c.wantOutRange {
			t.Errorf("%s: OutOfRange = %v, want %v", c.label, st.OutOfRange, c.wantOutRange)
		}
	}
}

// TestConstraintValidity_ValidFlag 覆盖 Valid 与 WillValidate 的组合关系：
// 同是「required 且为空」，candidate 元素校验失败（CheckValidity 为 false），
// 而 barred 元素（readonly）不参与校验（CheckValidity 恒为 true）。CSS 的
// :valid/:invalid 由 WillValidate 把关——barred 元素两者都不匹配。
func TestConstraintValidity_ValidFlag(t *testing.T) {
	// 空的 required input（candidate）：约束失败。
	in, _ := ToInputElement(createElement("input", map[string]string{"type": "text", "required": "required"}))
	st, ok := ConstraintValidity(in.El)
	if !ok {
		t.Fatal("ConstraintValidity(required input) ok = false")
	}
	if !st.WillValidate {
		t.Error("required input WillValidate = false, want true")
	}
	if st.Valid {
		t.Error("required 且为空的 input Valid = true, want false")
	}
	if in.CheckValidity() {
		t.Error("required 且为空：CheckValidity() = true, want false")
	}
	if in.Validity().Valid() {
		t.Error("required 且为空：Validity().Valid() = true, want false")
	}

	// 同样内容但 readonly（barred）：不参与校验。
	ro, _ := ToInputElement(createElement("input", map[string]string{
		"type": "text", "required": "required", "readonly": "readonly",
	}))
	st2, _ := ConstraintValidity(ro.El)
	if st2.WillValidate {
		t.Error("readonly input WillValidate = true, want false")
	}
	if !ro.CheckValidity() {
		t.Error("barred 元素 CheckValidity() = false, want true")
	}
}

// ─── input 的 customError（此前漏检的 bug）──────────────────────────

func TestInput_CustomValidityAffectsValidity(t *testing.T) {
	in, _ := ToInputElement(createElement("input", map[string]string{"type": "text"}))
	if !in.CheckValidity() {
		t.Fatal("无约束的 input CheckValidity() = false, want true")
	}
	in.SetCustomValidity("名字已被占用")
	v := in.Validity()
	if !v.CustomError {
		t.Error("setCustomValidity 后 validity.customError = false, want true")
	}
	if v.Valid() {
		t.Error("setCustomValidity 后 validity.valid = true, want false")
	}
	if in.CheckValidity() {
		t.Error("setCustomValidity 后 CheckValidity() = true, want false")
	}
	if got := v.ValidationMessage(in.El); got != "名字已被占用" {
		t.Errorf("validationMessage = %q, want 自定义消息", got)
	}
	st, _ := ConstraintValidity(in.El)
	if st.Valid {
		t.Error("setCustomValidity 后 ConstraintValidity.Valid = true, want false")
	}
	// 清空消息 → 恢复有效。
	in.SetCustomValidity("")
	if !in.CheckValidity() {
		t.Error("清空自定义消息后 CheckValidity() = false, want true")
	}
}

func TestButton_ConstraintValidation(t *testing.T) {
	// submit 按钮：可以 setCustomValidity 变无效（从而阻止提交）。
	b, _ := ToButtonElement(createElement("button", map[string]string{"type": "submit"}))
	if !b.WillValidate() {
		t.Fatal("submit button WillValidate() = false, want true")
	}
	if !b.CheckValidity() {
		t.Fatal("新 submit button CheckValidity() = false, want true")
	}
	b.SetCustomValidity("按钮不可用")
	if b.Validity().Valid() {
		t.Error("setCustomValidity 后 button validity.valid = true, want false")
	}
	if b.CheckValidity() {
		t.Error("setCustomValidity 后 button CheckValidity() = true, want false")
	}
	// 普通（type=button）按钮被 barred：checkValidity 恒真。
	b2, _ := ToButtonElement(createElement("button", map[string]string{"type": "button"}))
	b2.SetCustomValidity("忽略")
	if !b2.CheckValidity() {
		t.Error("barred 按钮 CheckValidity() = false, want true")
	}
}

// ─── 日期类类型的 min/max 进入 ValidityState ────────────────────────

func TestInput_DateRangeValidityState(t *testing.T) {
	in, _ := ToInputElement(createElement("input", map[string]string{
		"type": "date", "min": "2024-01-10", "value": "2024-01-05",
	}))
	v := in.Validity()
	if !v.RangeUnderflow {
		t.Error("date 值早于 min：RangeUnderflow = false, want true")
	}
	if v.Valid() {
		t.Error("date 值早于 min：valid = true, want false")
	}
	in2, _ := ToInputElement(createElement("input", map[string]string{
		"type": "date", "min": "2024-01-10", "value": "2024-02-05",
	}))
	if in2.Validity().RangeUnderflow {
		t.Error("date 值晚于 min：RangeUnderflow = true, want false")
	}
	if !in2.CheckValidity() {
		t.Error("date 值晚于 min：CheckValidity() = false, want true")
	}
}

// TestCompilePatternCache 覆盖 pattern 校验的正则缓存：同一 pattern 反复校验
// 结果稳定（缓存命中与首次编译一致），非法 pattern 被忽略而不是报错。
func TestCompilePatternCache(t *testing.T) {
	in, _ := ToInputElement(createElement("input", map[string]string{
		"type": "text", "pattern": `[a-z]{3}`, "value": "ab",
	}))
	for i := 0; i < 3; i++ {
		if !in.Validity().PatternMismatch {
			t.Fatalf("第 %d 次：不匹配的值应报 PatternMismatch", i+1)
		}
	}
	in.SetValue("abc")
	for i := 0; i < 3; i++ {
		if in.Validity().PatternMismatch {
			t.Fatalf("第 %d 次：匹配的值不应报 PatternMismatch", i+1)
		}
	}
	// 非法正则：HTML 规定该 pattern 被忽略（不是让控件无效，也不该 panic）。
	bad, _ := ToInputElement(createElement("input", map[string]string{
		"type": "text", "pattern": `[`, "value": "abc",
	}))
	if bad.Validity().PatternMismatch {
		t.Error("非法 pattern 应被忽略，不应报 PatternMismatch")
	}
	if !bad.CheckValidity() {
		t.Error("非法 pattern 不应使控件无效")
	}
}
