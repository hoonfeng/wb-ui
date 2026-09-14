package html5

import (
	"strconv"
	"strings"
	"time"

	"wb-ui/css"
	"wb-ui/dom"
)

// init 把本包的约束校验实现注入 css 包的选择器匹配器
// （:valid / :invalid / :in-range / :out-of-range）。放在 init 而不是由调用方
// 显式装配，是为了让任何导入 html5 的程序（引擎、诊断工具、测试）自动接线
// ——漏调用会让这些伪类静默地永不匹配，属于难以发现的失效。
//
// 依赖方向：html5 → css（本包已为 UA 样式表导入 css）；css 侧只持有
// FormValidity 这个纯数据结构，不反向依赖 html5。
func init() {
	css.SetFormValidityResolver(func(el *dom.Element) (css.FormValidity, bool) {
		st, ok := ConstraintValidity(el)
		if !ok {
			return css.FormValidity{}, false
		}
		return css.FormValidity{
			WillValidate: st.WillValidate,
			Valid:        st.Valid,
			RangeLimited: st.RangeLimited,
			OutOfRange:   st.OutOfRange,
		}, true
	})
}

// ─── 约束校验：候选资格与范围限制（HTML §4.10.21）─────────────────────
//
// 本文件把「元素能否参与约束校验」（barred from constraint validation）与
// 「是否越界」（suffering from an underflow / overflow）两个判定集中起来，
// 供三处消费：
//
//  1. IDL：willValidate / validity / checkValidity / reportValidity；
//  2. CSS：:valid / :invalid / :in-range / :out-of-range（css 包通过宿主注入
//     的 FormValidityResolver 间接消费本文件的 ConstraintValidity）；
//  3. 表单提交时的交互校验（HTMLFormElement.RequestSubmit）。
//
// 规范依据：
//   - 一个 submittable element 只有未被 barred 时才是「candidate for
//     constraint validation」（§4.10.21.1 Definitions）；被排除的元素
//     willValidate 为 false，且既非 :valid 也非 :invalid。
//   - barred 的具体条件（散落在各元素的定义里）：
//     · 元素 disabled（§4.10.19.5）；
//     · input/textarea 为 readonly（§4.10.5.3.6 / §4.10.7；readonly 只对
//       支持该属性的输入类型有意义，checkbox/radio/range/file/hidden/
//       color/submit/image/reset/button 等类型不受影响——MDN readonly）；
//     · input 的 type 为 hidden/reset/button（MDN HTMLInputElement.
//       willValidate：type 为 hidden、reset、button 之一）；
//     · button 的 type 为 reset/button（§4.10.6，submit 状态不排除）；
//     · 元素有 datalist 祖先；
//     · 其余元素（output/fieldset/progress/普通元素）不参与约束校验。

// FormControlValidity 汇总一个表单控件的约束校验状态。
type FormControlValidity struct {
	// WillValidate 为 false 表示元素被排除在约束校验之外（barred）——
	// 此时 CSS 的 :valid 与 :invalid 都不匹配，checkValidity() 恒为 true。
	WillValidate bool

	// Valid 表示元素没有违反任何约束（barred 时无意义；调用方应先看
	// WillValidate）。
	Valid bool

	// RangeLimited 表示元素「有范围限制」——HTML §4.10.21.1 中
	// :in-range / :out-of-range 只匹配有范围限制的元素：min/max 属性存在、
	// 能解析、且元素类型支持范围（number/range/date/month/week/time/
	// datetime-local）。没有范围限制的元素两者都不匹配（MDN :in-range
	// "only applies to elements that have (and can take) a range limitation"）。
	RangeLimited bool

	// OutOfRange 表示元素 suffering from an underflow 或 overflow。注意
	// step mismatch 不计入（HTML 规范中 :out-of-range 只看 underflow 与
	// overflow 两种违规，step mismatch 只影响 :invalid）。
	OutOfRange bool
}

// ConstraintValidity 返回 el 的约束校验状态；ok=false 表示该元素不参与约束
// 校验（普通元素、output/fieldset 等），调用方应按「既非 valid 也非 invalid、
// willValidate 为 false」处理。
func ConstraintValidity(el *dom.Element) (FormControlValidity, bool) {
	if el == nil {
		return FormControlValidity{}, false
	}
	switch el.LocalName() {
	case "input":
		in, ok := ToInputElement(el)
		if !ok {
			return FormControlValidity{}, false
		}
		st := FormControlValidity{WillValidate: in.WillValidate()}
		v := in.Validity()
		st.Valid = v.Valid()
		limited, under, over := inputRangeState(in)
		st.RangeLimited = limited
		st.OutOfRange = under || over
		return st, true
	case "select":
		sel, ok := ToSelectElement(el)
		if !ok {
			return FormControlValidity{}, false
		}
		st := FormControlValidity{WillValidate: sel.WillValidate()}
		st.Valid = sel.Validity().Valid()
		return st, true
	case "textarea":
		ta, ok := ToTextAreaElement(el)
		if !ok {
			return FormControlValidity{}, false
		}
		st := FormControlValidity{WillValidate: ta.WillValidate()}
		st.Valid = ta.Validity().Valid()
		return st, true
	case "button":
		btn, ok := ToButtonElement(el)
		if !ok {
			return FormControlValidity{}, false
		}
		st := FormControlValidity{WillValidate: btn.WillValidate()}
		st.Valid = btn.Validity().Valid()
		return st, true
	}
	return FormControlValidity{}, false
}

// ─── 候选资格（barred from constraint validation）────────────────────

// hasDataListAncestor 报告元素是否有 <datalist> 祖先（被 datalist 包裹的
// 元素不参与约束校验，它只是选项来源而不是用户输入控件）。
func hasDataListAncestor(el *dom.Element) bool {
	for p := el.ParentNode(); p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "datalist" {
			return true
		}
	}
	return false
}

// readonlySupportingInputType 报告某 input 类型是否支持 readonly 属性。
// 只有支持 readonly 的类型才会因为 readonly 被排除在约束校验之外：
// range/color/checkbox/radio/file/hidden/submit/image/reset/button 等类型的
// readonly 属性没有意义，不影响校验（MDN readonly 的「supported by textual
// form controls」清单）。
func readonlySupportingInputType(t InputType) bool {
	switch t {
	case InputText, InputSearch, InputTel, InputURL, InputEmail, InputPassword,
		InputDate, InputMonth, InputWeek, InputTime, InputDateTimeLocal, InputNumber:
		return true
	}
	return false
}

// barredInput 报告 input 是否被排除在约束校验之外。
func barredInput(in HTMLInputElement) bool {
	if in.Disabled() {
		return true
	}
	switch in.Type() {
	case InputHidden, InputReset, InputButton:
		return true
	}
	if in.ReadOnly() && readonlySupportingInputType(in.Type()) {
		return true
	}
	return hasDataListAncestor(in.El)
}

// barredTextArea 报告 textarea 是否被排除在约束校验之外（readonly 生效）。
func barredTextArea(t HTMLTextAreaElement) bool {
	return t.Disabled() || t.ReadOnly()
}

// barredSelect 报告 select 是否被排除在约束校验之外（select 不支持 readonly）。
func barredSelect(s HTMLSelectElement) bool {
	return s.Disabled()
}

// barredButton 报告 button 是否被排除在约束校验之外：type 为 reset/button
// 时排除，submit（含默认）时是候选（可以 setCustomValidity 让它无效，
// 从而阻止表单提交）。
func barredButton(b HTMLButtonElement) bool {
	if b.Disabled() {
		return true
	}
	switch b.Type() {
	case ButtonReset, ButtonButton:
		return true
	}
	return false
}

// ─── 范围限制与越界判定（:in-range / :out-of-range）──────────────────

// rangeLimitedInputType 报告该 input 类型是否「能取范围限制」（min/max
// 对它有定义）。text/email 等类型上的 min/max 属性没有意义。
func rangeLimitedInputType(t InputType) bool {
	switch t {
	case InputNumber, InputRange, InputDate, InputMonth, InputWeek, InputTime,
		InputDateTimeLocal:
		return true
	}
	return false
}

// inputRangeState 返回 (是否有范围限制, underflow, overflow)。只有当元素类型
// 支持范围、min/max 至少一个存在且能按类型解析成功时才有范围限制；空值
// 既不 underflow 也不 overflow（规范只在整个 value 可解析时比较）。
func inputRangeState(in HTMLInputElement) (limited, underflow, overflow bool) {
	t := in.Type()
	if !rangeLimitedInputType(t) {
		return false, false, false
	}
	minStr, maxStr := strings.TrimSpace(in.Min()), strings.TrimSpace(in.Max())
	if minStr == "" && maxStr == "" {
		return false, false, false
	}
	if t == InputNumber || t == InputRange {
		minV, okMin := parseOptionalFloat(minStr)
		maxV, okMax := parseOptionalFloat(maxStr)
		if !okMin && !okMax {
			return false, false, false
		}
		num, ok := parseOptionalFloat(strings.TrimSpace(in.Value()))
		if !ok || strings.TrimSpace(in.Value()) == "" {
			// 有范围限制但值为空/不可解析：不越界（但 :in-range 仍匹配——
			// 它只看「没有 underflow/overflow」）。
			return true, false, false
		}
		return true, okMin && num < minV, okMax && num > maxV
	}
	minT, okMin := parseDateTimeValue(t, minStr)
	maxT, okMax := parseDateTimeValue(t, maxStr)
	if !okMin && !okMax {
		return false, false, false
	}
	valStr := strings.TrimSpace(in.Value())
	if valStr == "" {
		return true, false, false
	}
	valT, ok := parseDateTimeValue(t, valStr)
	if !ok {
		return true, false, false
	}
	return true, okMin && valT.Before(minT), okMax && valT.After(maxT)
}

// parseOptionalFloat 解析可选的数值属性（"" 与 "any" 视为不存在）。
func parseOptionalFloat(s string) (float64, bool) {
	if s == "" || s == "any" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseDateTimeValue 按输入类型解析日期/时间字符串（min/max 与 value 共用）。
func parseDateTimeValue(t InputType, s string) (time.Time, bool) {
	switch t {
	case InputDate:
		return parseDate(s)
	case InputMonth:
		y, m, ok := parseMonth(s)
		if !ok {
			return time.Time{}, false
		}
		return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC), true
	case InputWeek:
		y, w, ok := parseWeek(s)
		if !ok {
			return time.Time{}, false
		}
		// ISO 8601：第 1 周是「包含 1 月 4 日的那一周」，其周一是基准日。
		jan4 := time.Date(y, time.January, 4, 0, 0, 0, 0, time.UTC)
		wd := int(jan4.Weekday()) // Sunday=0
		if wd == 0 {
			wd = 7
		}
		week1Monday := jan4.AddDate(0, 0, -(wd - 1))
		return week1Monday.AddDate(0, 0, 7*(w-1)), true
	case InputTime:
		return parseTimeStr(s)
	case InputDateTimeLocal:
		return parseDateTimeLocal(s)
	}
	return time.Time{}, false
}
