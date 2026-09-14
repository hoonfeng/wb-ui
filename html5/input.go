package html5

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"wb-ui/dom"
)

// HTMLInputElement wraps a *dom.Element whose tag is "input" and
// provides type-safe accessors for form control properties.
// Mirrors WebCore::HTMLInputElement.
type HTMLInputElement struct {
	El *dom.Element
}

// ToInputElement wraps an element as an HTMLInputElement. Returns
// false if el is nil or not an <input> element.
func ToInputElement(el *dom.Element) (HTMLInputElement, bool) {
	if el == nil || el.LocalName() != "input" {
		return HTMLInputElement{}, false
	}
	return HTMLInputElement{El: el}, true
}

// Type returns the input's type attribute, defaulting to "text".
// Mirrors HTMLInputElement::type().
func (i HTMLInputElement) Type() InputType {
	t := strings.ToLower(i.El.GetAttribute("type"))
	if t == "" {
		return InputText
	}
	if IsKnownInputType(t) {
		return InputType(t)
	}
	return InputText
}

// SetType sets the type attribute.
func (i HTMLInputElement) SetType(t InputType) {
	i.El.SetAttribute("type", string(t))
}

// Value returns the current value. For most types this is the value
// attribute; for checkbox/radio it's "on" when checked, "" when unchecked.
// Mirrors HTMLInputElement::value().
func (i HTMLInputElement) Value() string {
	switch i.Type() {
	case InputCheckbox, InputRadio:
		if i.Checked() {
			v := i.El.GetAttribute("value")
			if v == "" {
				return "on"
			}
			return v
		}
		return ""
	case InputFile:
		// File input value is read-only (the selected file path).
		return i.El.GetAttribute("value")
	default:
		v := i.El.GetAttribute("value")
		return v
	}
}

// SetValue sets the value attribute. Mirrors HTMLInputElement::setValue().
func (i HTMLInputElement) SetValue(v string) {
	i.El.SetAttribute("value", v)
}

// DefaultValue returns the value attribute as set in HTML.
func (i HTMLInputElement) DefaultValue() string {
	return i.El.GetAttribute("value")
}

// SetDefaultValue sets the value attribute.
func (i HTMLInputElement) SetDefaultValue(v string) {
	i.El.SetAttribute("value", v)
}

// Checked reports whether a checkbox/radio is checked.
// Mirrors HTMLInputElement::checked().
func (i HTMLInputElement) Checked() bool {
	return i.El.HasAttribute("checked")
}

// SetChecked sets the checked state.
func (i HTMLInputElement) SetChecked(c bool) {
	if c {
		i.El.SetAttribute("checked", "checked")
	} else {
		i.El.RemoveAttribute("checked")
	}
}

// DefaultChecked returns the default checked state from HTML.
func (i HTMLInputElement) DefaultChecked() bool {
	return i.El.HasAttribute("checked")
}

// Disabled reports whether the input is disabled.
func (i HTMLInputElement) Disabled() bool {
	return attrBool(i.El, "disabled")
}

// SetDisabled sets the disabled state.
func (i HTMLInputElement) SetDisabled(d bool) {
	if d {
		i.El.SetAttribute("disabled", "disabled")
	} else {
		i.El.RemoveAttribute("disabled")
	}
}

// Required reports whether the input is required.
func (i HTMLInputElement) Required() bool {
	return attrBool(i.El, "required")
}

// ReadOnly reports whether the input is read-only.
func (i HTMLInputElement) ReadOnly() bool {
	return attrBool(i.El, "readonly")
}

// Name returns the name attribute.
func (i HTMLInputElement) Name() string {
	return i.El.GetAttribute("name")
}

// Placeholder returns the placeholder attribute.
func (i HTMLInputElement) Placeholder() string {
	return i.El.GetAttribute("placeholder")
}

// Min returns the min attribute.
func (i HTMLInputElement) Min() string {
	return i.El.GetAttribute("min")
}

// Max returns the max attribute.
func (i HTMLInputElement) Max() string {
	return i.El.GetAttribute("max")
}

// Step returns the step attribute.
func (i HTMLInputElement) Step() string {
	return i.El.GetAttribute("step")
}

// Pattern returns the pattern attribute.
func (i HTMLInputElement) Pattern() string {
	return i.El.GetAttribute("pattern")
}

// MaxLength returns the maxlength attribute, or -1 if unset.
func (i HTMLInputElement) MaxLength() int {
	if !i.El.HasAttribute("maxlength") {
		return -1
	}
	return attrInt(i.El, "maxlength")
}

// MinLength returns the minlength attribute, or -1 if unset.
func (i HTMLInputElement) MinLength() int {
	if !i.El.HasAttribute("minlength") {
		return -1
	}
	return attrInt(i.El, "minlength")
}

// Form returns the enclosing form element, or nil if not in a form.
func (i HTMLInputElement) Form() *dom.Element {
	return FindFormAncestor(i.El)
}

// Autofocus reports whether the input should autofocus.
func (i HTMLInputElement) Autofocus() bool {
	return attrBool(i.El, "autofocus")
}

// Multiple reports whether multiple values are allowed (file/email).
func (i HTMLInputElement) Multiple() bool {
	return attrBool(i.El, "multiple")
}

// List returns the associated <datalist> element, or nil if the input's
// "list" attribute references a non-existent datalist. Mirrors
// HTMLInputElement::list().
func (i HTMLInputElement) List() *HTMLDataListElement {
	listID := i.El.GetAttribute("list")
	if listID == "" {
		return nil
	}
	doc := i.El.OwnerDocument()
	if doc == nil {
		return nil
	}
	dl := doc.GetElementById(listID)
	if dl == nil || dl.LocalName() != "datalist" {
		return nil
	}
	dle, ok := ToDataListElement(dl)
	if !ok {
		return nil
	}
	return &dle
}

// AcceptedLabels returns the list of label strings from the datalist options
// whose value starts with the given prefix. If the input has no associated
// datalist, returns nil. This is the primary API for autocomplete UI.
func (i HTMLInputElement) AcceptedLabels(prefix string) []string {
	dl := i.List()
	if dl == nil {
		return nil
	}
	return dl.SuggestionsFor(prefix)
}

// --- Validation ---

// Validity returns the ValidityState for this input.
// Mirrors HTMLInputElement::validity().
func (i HTMLInputElement) Validity() ValidityState {
	v := ValidityState{}
	t := i.Type()
	val := i.Value()

	// required check (valueMissing)
	if i.Required() {
		switch t {
		case InputCheckbox:
			v.ValueMissing = !i.Checked()
		case InputRadio:
			// For radio, check if any radio in the same group is checked.
			v.ValueMissing = !radioGroupChecked(i)
		default:
			v.ValueMissing = val == ""
		}
	}

	// type-specific validation (typeMismatch / badInput)
	switch t {
	case InputEmail:
		if val != "" {
			emails := []string{val}
			if i.Multiple() {
				emails = strings.Split(val, ",")
				for j := range emails {
					emails[j] = strings.TrimSpace(emails[j])
				}
			}
			for _, e := range emails {
				if !emailRe.MatchString(e) {
					v.TypeMismatch = true
					break
				}
			}
		}
	case InputURL:
		if val != "" {
			if _, ok := parseURL(val); !ok {
				v.TypeMismatch = true
			}
		}
	case InputNumber, InputRange:
		if val != "" {
			if _, err := parseFloat(val); err != nil {
				v.BadInput = true
			}
		}
	case InputDate, InputMonth, InputWeek, InputTime, InputDateTimeLocal:
		if val != "" {
			if !validateDateTimeInput(t, val) {
				v.BadInput = true
			}
		}
	case InputColor:
		if val != "" {
			if !validateColor(val) {
				v.BadInput = true
			}
		}
	}

	// pattern check
	if i.Pattern() != "" && val != "" {
		pat := i.Pattern()
		if !strings.HasPrefix(pat, "^") {
			pat = "^" + pat
		}
		if !strings.HasSuffix(pat, "$") {
			pat = pat + "$"
		}
		re, err := compilePattern(pat)
		if err == nil && !re.MatchString(val) {
			v.PatternMismatch = true
		}
	}

	// maxlength / minlength
	maxLen := i.MaxLength()
	if maxLen >= 0 && len(val) > maxLen {
		v.TooLong = true
	}
	minLen := i.MinLength()
	if minLen >= 0 && len(val) < minLen && val != "" {
		v.TooShort = true
	}

	// 范围（min/max）：对 number/range 与日期类类型都生效——inputRangeState
	// 按输入类型解析后比较（空值/不可解析值不越界）。
	if _, under, over := inputRangeState(i); under {
		v.RangeUnderflow = true
	} else if over {
		v.RangeOverflow = true
	}

	// step mismatch：所有支持 step 的类型都校验（number / range 与
	// date / month / week / time / datetime-local），日期类类型需要按类型换算
	// 单位与 step base——见 step.go（含「step 缺失时用 default step」这条
	// 常被忽略的规范行为：time 默认只允许整分钟、number 默认只允许整数）。
	if StepMismatch(i) {
		v.StepMismatch = true
	}

	// 自定义错误消息（setCustomValidity 的非空消息）：与 select/textarea 的
	// Validity() 一致。此前 input 漏了这一项——setCustomValidity("...") 之后
	// validity.customError 仍为 false、checkValidity() 仍返回 true。
	if msg, ok := customErrorMessages[i.El]; ok && msg != "" {
		v.CustomError = true
	}

	return v
}

// WillValidate 报告该 input 是否参与约束校验（HTML §4.10.21.1 的
// "candidate for constraint validation"）。被排除的情形：disabled、
// readonly（仅对支持 readonly 的输入类型）、type 为 hidden/reset/button、
// 位于 <datalist> 内——见 constraint.go 的 barredInput。
func (i HTMLInputElement) WillValidate() bool {
	return !barredInput(i)
}

// CheckValidity returns true if the input's value satisfies all
// constraints. Mirrors HTMLInputElement::checkValidity().
func (i HTMLInputElement) CheckValidity() bool {
	if !i.WillValidate() {
		return true
	}
	return i.Validity().Valid()
}

// SetCustomValidity sets a custom error message. If message is non-empty,
// the element is invalid with a customError.
func (i HTMLInputElement) SetCustomValidity(message string) {
	customErrorMessages[i.El] = message
}

// radioGroupChecked reports whether any radio input with the same name
// in the same form is checked.
func radioGroupChecked(input HTMLInputElement) bool {
	name := input.Name()
	if name == "" {
		return input.Checked()
	}
	form := input.Form()
	if form == nil {
		return input.Checked()
	}
	// Walk form descendants looking for radio inputs with same name.
	var found bool
	walkElements(form, func(el *dom.Element) {
		if el.LocalName() != "input" {
			return
		}
		t := strings.ToLower(el.GetAttribute("type"))
		if t != "radio" {
			return
		}
		if el.GetAttribute("name") != name {
			return
		}
		if el.HasAttribute("checked") {
			found = true
		}
	})
	return found
}

// walkElements traverses the DOM tree and calls fn for every Element.
func walkElements(node dom.Node, fn func(*dom.Element)) {
	if el, ok := node.(*dom.Element); ok {
		fn(el)
	}
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		walkElements(c, fn)
	}
}

// parseFloat wraps strconv.ParseFloat for cleaner error handling.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// validateDateTimeInput checks if a date/time string is valid for the type.
func validateDateTimeInput(t InputType, val string) bool {
	switch t {
	case InputDate:
		_, ok := parseDate(val)
		return ok
	case InputTime:
		_, ok := parseTimeStr(val)
		return ok
	case InputMonth:
		_, _, ok := parseMonth(val)
		return ok
	case InputWeek:
		_, _, ok := parseWeek(val)
		return ok
	case InputDateTimeLocal:
		_, ok := parseDateTimeLocal(val)
		return ok
	}
	return true
}

// validateColor checks if a color value is a valid #rrggbb hex.
func validateColor(val string) bool {
	if len(val) != 7 || val[0] != '#' {
		return false
	}
	for _, c := range val[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// patternCache 缓存已编译的 pattern 正则。
//
// 为什么要缓存：pattern 校验现在会被样式层按元素反复调用（css 的 :valid /
// :invalid / :user-invalid 通过注入的 FormValidityResolver 查询约束状态），
// 而 regexp.Compile 每次都要重新解析并建 NFA——样式重算频繁时这明显偏高。
// 页面里的 pattern 数量是个位数，缓存无内存压力；同时缓存编译错误（同一个
// 非法 pattern 也会被反复问），避免每次重复解析失败路径。
var (
	patternCacheMu sync.Mutex
	patternCache   = map[string]patternEntry{}
)

type patternEntry struct {
	re  *regexp.Regexp
	err error
}

// compilePattern wraps regexp.Compile for the pattern validation, with a cache.
func compilePattern(pat string) (*regexp.Regexp, error) {
	patternCacheMu.Lock()
	entry, ok := patternCache[pat]
	patternCacheMu.Unlock()
	if ok {
		return entry.re, entry.err
	}
	re, err := regexp.Compile(pat)
	patternCacheMu.Lock()
	patternCache[pat] = patternEntry{re: re, err: err}
	patternCacheMu.Unlock()
	return re, err
}

// FindFormAncestor returns the nearest <form> ancestor of el, or nil.
func FindFormAncestor(el *dom.Element) *dom.Element {
	for p := el.ParentNode(); p != nil; p = p.ParentNode() {
		if e, ok := p.(*dom.Element); ok && e.LocalName() == "form" {
			return e
		}
	}
	return nil
}
