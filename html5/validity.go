// Package html5 provides HTML5 form element types that mirror WebKit's
// HTMLFormElement / HTMLInputElement / HTMLSelectElement / etc.
// These types wrap *dom.Element and provide type-safe accessors for
// form-specific attributes (value, checked, type, etc.) and validation
// logic (ValidityState, constraint validation API).
//
// The types are not embedded in the DOM package to avoid bloating
// dom.Element with HTML-specific methods. Instead, they are lightweight
// wrappers that read and write element attributes, mirroring WebKit's
// HTMLInputElement::value() / setValue() etc.
package html5

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wb-ui/dom"
)

// ValidityState mirrors the WebIDL ValidityState interface. Each field
// corresponds to a constraint that can fail; the Valid field is true
// when all constraints pass.
type ValidityState struct {
	ValueMissing     bool // required field is empty
	TypeMismatch     bool // wrong format for type (email/url/date)
	PatternMismatch  bool // does not match pattern attribute
	TooLong          bool // value exceeds maxlength
	TooShort         bool // value is shorter than minlength
	RangeUnderflow   bool // value < min
	RangeOverflow    bool // value > max
	StepMismatch     bool // value does not match step
	BadInput         bool // value cannot be parsed (e.g. non-numeric in number)
	CustomError      bool // setCustomValidity was called with non-empty message
}

// Valid returns true when all constraints pass.
func (v ValidityState) Valid() bool {
	return !v.ValueMissing && !v.TypeMismatch && !v.PatternMismatch &&
		!v.TooLong && !v.TooShort && !v.RangeUnderflow &&
		!v.RangeOverflow && !v.StepMismatch && !v.BadInput &&
		!v.CustomError
}

// ValidationMessage returns the first failing constraint's message,
// or "" if valid. Mirrors the validationMessage IDL attribute.
// The owningEl parameter is the element that owns this ValidityState;
// it is used to look up per-element custom error messages set via
// SetCustomValidity.
func (v ValidityState) ValidationMessage(owningEl *dom.Element) string {
	if v.CustomError {
		if owningEl != nil {
			if msg, ok := customErrorMessages[owningEl]; ok && msg != "" {
				return msg
			}
		}
		return "Please provide a valid value."
	}
	if v.ValueMissing {
		return "Please fill out this field."
	}
	if v.TypeMismatch {
		return "Please enter a valid value."
	}
	if v.PatternMismatch {
		return "Please match the requested format."
	}
	if v.TooLong {
		return "Please shorten this text."
	}
	if v.TooShort {
		return "Please lengthen this text."
	}
	if v.RangeUnderflow {
		return "Value is too low."
	}
	if v.RangeOverflow {
		return "Value is too high."
	}
	if v.StepMismatch {
		return "Please enter a valid value."
	}
	if v.BadInput {
		return "Please enter a valid value."
	}
	return ""
}

// customErrorMessages stores custom error messages set via
// SetCustomValidity. Keyed by element pointer so each element can have its
// own custom error message, matching browser behavior.
var customErrorMessages = map[*dom.Element]string{}

// --- Regex patterns for type validation ---

var (
	emailRe = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	urlRe   = regexp.MustCompile(`^https?://[^\s<>"{}|\\^` + "`" + `[\]]+$`)
)

// --- Utility helpers ---

// hasAttr reports whether el has the attribute and its value is not empty.
func hasAttr(el *dom.Element, name string) bool {
	v := el.GetAttribute(name)
	return v != ""
}

// attrInt returns the integer value of an attribute, or 0 if absent/invalid.
func attrInt(el *dom.Element, name string) int {
	v := el.GetAttribute(name)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

// attrFloat returns the float value of an attribute, or 0 if absent/invalid.
func attrFloat(el *dom.Element, name string) float64 {
	v := el.GetAttribute(name)
	if v == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// attrBool returns true if the attribute is present (HTML boolean attribute).
func attrBool(el *dom.Element, name string) bool {
	return el.HasAttribute(name)
}

// textContent returns the concatenated text of an element's descendants.
func textContent(el *dom.Element) string {
	var sb strings.Builder
	var walk func(dom.Node)
	walk = func(n dom.Node) {
		if t, ok := n.(*dom.Text); ok {
			sb.WriteString(t.Data())
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(el)
	return sb.String()
}

// --- Input type constants ---

// InputType enumerates the 22 HTML input type values.
type InputType string

const (
	InputText           InputType = "text"
	InputPassword       InputType = "password"
	InputCheckbox       InputType = "checkbox"
	InputRadio          InputType = "radio"
	InputSubmit         InputType = "submit"
	InputReset          InputType = "reset"
	InputFile           InputType = "file"
	InputHidden         InputType = "hidden"
	InputImage          InputType = "image"
	InputButton         InputType = "button"
	InputSearch         InputType = "search"
	InputEmail          InputType = "email"
	InputURL            InputType = "url"
	InputTel            InputType = "tel"
	InputNumber         InputType = "number"
	InputRange          InputType = "range"
	InputDate           InputType = "date"
	InputTime           InputType = "time"
	InputColor          InputType = "color"
	InputDateTimeLocal  InputType = "datetime-local"
	InputMonth          InputType = "month"
	InputWeek           InputType = "week"
)

// allInputTypes lists all 22 recognized input types.
var allInputTypes = []InputType{
	InputText, InputPassword, InputCheckbox, InputRadio,
	InputSubmit, InputReset, InputFile, InputHidden,
	InputImage, InputButton, InputSearch, InputEmail,
	InputURL, InputTel, InputNumber, InputRange,
	InputDate, InputTime, InputColor, InputDateTimeLocal,
	InputMonth, InputWeek,
}

// IsKnownInputType reports whether typ is one of the 22 recognized types.
func IsKnownInputType(typ string) bool {
	for _, t := range allInputTypes {
		if string(t) == strings.ToLower(typ) {
			return true
		}
	}
	return false
}

// --- Date/time parsing helpers ---

// parseDate parses a yyyy-mm-dd date string. Returns zero time on failure.
func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseTime parses a hh:mm or hh:mm:ss time string. Returns zero on failure.
func parseTimeStr(s string) (time.Time, bool) {
	formats := []string{"15:04:05", "15:04"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseMonth parses a yyyy-mm string.
func parseMonth(s string) (year, month int, ok bool) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return 0, 0, false
	}
	return t.Year(), int(t.Month()), true
}

// parseWeek parses a yyyy-Www string (ISO 8601 week, HTML 的 valid week string）。
//
// ⚠️ 不能用 time.Parse("2006-W02", s) 解析：Go 的时间布局里 "02" 是「月中的
// 第几天」，布局 "2006-W02" 中的 W 只是字面量，没有「ISO 周年 + 周号」的解析
// 支持。用它会把 "1970-W03" 解析成 1970-01-03（1 月的第 3 天），因此
// 1970-W01…W04 会全部落到同一周、任意年的第 N 周都被算成第 N 天——min/max
// 与 step 的周比较因此失真（此处修复；周号必须自己按 ISO 规则校验与换算）。
func parseWeek(s string) (year, week int, ok bool) {
	// 规范：value 必须匹配「四位以上的年 + "-W" + 两位周号」。
	parts := strings.SplitN(s, "-W", 2)
	if len(parts) != 2 || len(parts[0]) < 4 || len(parts[1]) != 2 {
		return 0, 0, false
	}
	y, errY := strconv.Atoi(parts[0])
	w, errW := strconv.Atoi(parts[1])
	if errY != nil || errW != nil || y < 1 || w < 1 || w > 53 {
		return 0, 0, false
	}
	// 第 53 周不是每年都有（ISO：该年含 53 周 ⟺ 1 月 1 日是周四，或闰年的
	// 1 月 1 日是周三）——不校验会让 "2026-W53" 之类的值被当成合法输入。
	if weeksInISOYear(y) < w {
		return 0, 0, false
	}
	return y, w, true
}

// weeksInISOYear 返回 ISO 周年 y 包含的周数（52 或 53）。
func weeksInISOYear(y int) int {
	jan1 := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
	wd := int(jan1.Weekday()) // Sunday=0
	if wd == 0 {
		wd = 7
	}
	leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
	if wd == 4 || (wd == 3 && leap) {
		return 53
	}
	return 52
}

// parseDateTimeLocal parses a yyyy-mm-ddThh:mm or yyyy-mm-ddThh:mm:ss string.
func parseDateTimeLocal(s string) (time.Time, bool) {
	formats := []string{"2006-01-02T15:04:05", "2006-01-02T15:04"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseURL parses a URL string.
func parseURL(s string) (*url.URL, bool) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Allow protocol-relative URLs.
		if strings.HasPrefix(s, "//") {
			u2, err2 := url.Parse("http:" + s)
			if err2 == nil && u2.Host != "" {
				return u2, true
			}
		}
		return nil, false
	}
	return u, true
}
