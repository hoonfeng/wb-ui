package html5

import (
	"testing"

	"wb-ui/engine/dom"
)

// newInput creates an <input> element in a fresh document and returns it wrapped
// as an HTMLInputElement. Convenience helper for tests.
func newInput(t *testing.T, attrs map[string]string) HTMLInputElement {
	t.Helper()
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	for k, v := range attrs {
		el.SetAttribute(k, v)
	}
	in, ok := ToInputElement(el)
	if !ok {
		t.Fatalf("ToInputElement returned false for <input>")
	}
	return in
}

// --- Type / Value / Checked accessors ---

func TestInput_TypeDefaultsToText(t *testing.T) {
	in := newInput(t, nil)
	if got := in.Type(); got != InputText {
		t.Fatalf("Type() = %q, want %q", got, InputText)
	}
}

func TestInput_TypeKnown(t *testing.T) {
	cases := []InputType{
		InputEmail, InputURL, InputNumber, InputRange, InputDate,
		InputTime, InputColor, InputCheckbox, InputRadio, InputPassword,
		InputTel, InputSearch, InputWeek, InputMonth, InputDateTimeLocal,
		InputFile, InputHidden, InputImage, InputSubmit, InputReset, InputButton,
	}
	for _, want := range cases {
		in := newInput(t, map[string]string{"type": string(want)})
		if got := in.Type(); got != want {
			t.Errorf("Type(%q) = %q, want %q", want, got, want)
		}
	}
}

func TestInput_TypeUnknownBecomesText(t *testing.T) {
	in := newInput(t, map[string]string{"type": "bogus"})
	if got := in.Type(); got != InputText {
		t.Fatalf("Type(bogus) = %q, want %q", got, InputText)
	}
}

func TestInput_TypeIsCaseInsensitive(t *testing.T) {
	in := newInput(t, map[string]string{"type": "EMAIL"})
	if got := in.Type(); got != InputEmail {
		t.Fatalf("Type(EMAIL) = %q, want %q", got, InputEmail)
	}
}

func TestInput_ValueAndGetSet(t *testing.T) {
	in := newInput(t, map[string]string{"value": "hello"})
	if got := in.Value(); got != "hello" {
		t.Fatalf("Value() = %q, want %q", got, "hello")
	}
	in.SetValue("world")
	if got := in.Value(); got != "world" {
		t.Fatalf("after SetValue: Value() = %q, want %q", got, "world")
	}
}

func TestInput_CheckboxValueReflectsChecked(t *testing.T) {
	in := newInput(t, map[string]string{"type": "checkbox"})
	if got := in.Value(); got != "" {
		t.Fatalf("unchecked checkbox Value() = %q, want empty", got)
	}
	in.SetChecked(true)
	if got := in.Value(); got != "on" {
		t.Fatalf("checked checkbox Value() = %q, want %q", got, "on")
	}
}

func TestInput_CheckboxValueWithExplicitValue(t *testing.T) {
	in := newInput(t, map[string]string{"type": "checkbox", "value": "agree"})
	in.SetChecked(true)
	if got := in.Value(); got != "agree" {
		t.Fatalf("checked checkbox Value() = %q, want %q", got, "agree")
	}
}

func TestInput_CheckedToggle(t *testing.T) {
	in := newInput(t, map[string]string{"type": "checkbox"})
	if in.Checked() {
		t.Fatal("new checkbox should be unchecked")
	}
	in.SetChecked(true)
	if !in.Checked() {
		t.Fatal("after SetChecked(true) should be checked")
	}
	in.SetChecked(false)
	if in.Checked() {
		t.Fatal("after SetChecked(false) should be unchecked")
	}
}

func TestInput_DisabledRequiredReadOnlyFlags(t *testing.T) {
	in := newInput(t, map[string]string{"disabled": "disabled", "required": "required", "readonly": "readonly"})
	if !in.Disabled() {
		t.Error("Disabled() = false, want true")
	}
	if !in.Required() {
		t.Error("Required() = false, want true")
	}
	if !in.ReadOnly() {
		t.Error("ReadOnly() = false, want true")
	}
}

// --- Validation: text type ---

func TestInput_TextValueMissing(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "required": "required"})
	if v := in.Validity(); !v.ValueMissing {
		t.Errorf("empty required text: ValueMissing = false, want true")
	}
	in.SetValue("hello")
	if v := in.Validity(); v.ValueMissing {
		t.Errorf("filled required text: ValueMissing = true, want false")
	}
}

func TestInput_TextOptionalNoValueMissing(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text"})
	if v := in.Validity(); v.ValueMissing {
		t.Errorf("empty optional text: ValueMissing = true, want false")
	}
}

func TestInput_TextValid(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "value": "hello"})
	if v := in.Validity(); !v.Valid() {
		t.Errorf("valid text: Valid() = false, want true; state=%+v", v)
	}
}

// --- Validation: email type ---

func TestInput_EmailTypeMismatch(t *testing.T) {
	cases := []struct {
		val    string
		wantOK bool
	}{
		{"user@example.com", true},
		{"a.b+c@sub.example.org", true},
		{"not-an-email", false},
		{"@example.com", false},
		{"user@", false},
		{"", true}, // empty is not typeMismatch (handled by required)
	}
	for _, c := range cases {
		in := newInput(t, map[string]string{"type": "email", "value": c.val})
		v := in.Validity()
		if got := !v.TypeMismatch; got != c.wantOK {
			t.Errorf("email %q: TypeMismatch=%v, want %v", c.val, v.TypeMismatch, !c.wantOK)
		}
	}
}

func TestInput_EmailMultiple(t *testing.T) {
	in := newInput(t, map[string]string{"type": "email", "multiple": "multiple", "value": "a@x.com, b@y.com"})
	if v := in.Validity(); v.TypeMismatch {
		t.Errorf("valid multiple email: TypeMismatch = true, want false")
	}
	in.SetValue("a@x.com, not-email")
	if v := in.Validity(); !v.TypeMismatch {
		t.Errorf("mixed multiple email: TypeMismatch = false, want true")
	}
}

// --- Validation: url type ---

func TestInput_URLTypeMismatch(t *testing.T) {
	cases := []struct {
		val    string
		wantOK bool
	}{
		{"https://example.com", true},
		{"http://foo.org/path?q=1", true},
		{"not-a-url", false},
		{"example.com", false},
		{"", true},
	}
	for _, c := range cases {
		in := newInput(t, map[string]string{"type": "url", "value": c.val})
		v := in.Validity()
		if got := !v.TypeMismatch; got != c.wantOK {
			t.Errorf("url %q: TypeMismatch=%v, want %v", c.val, v.TypeMismatch, !c.wantOK)
		}
	}
}

// --- Validation: number / range type ---

func TestInput_NumberBadInput(t *testing.T) {
	in := newInput(t, map[string]string{"type": "number", "value": "abc"})
	if v := in.Validity(); !v.BadInput {
		t.Errorf("number abc: BadInput = false, want true")
	}
	in.SetValue("42")
	if v := in.Validity(); v.BadInput {
		t.Errorf("number 42: BadInput = true, want false")
	}
}

func TestInput_NumberRangeUnderflowOverflow(t *testing.T) {
	in := newInput(t, map[string]string{"type": "number", "min": "10", "max": "100", "value": "5"})
	if v := in.Validity(); !v.RangeUnderflow {
		t.Errorf("number 5 with min 10: RangeUnderflow = false, want true")
	}
	in.SetValue("150")
	if v := in.Validity(); !v.RangeOverflow {
		t.Errorf("number 150 with max 100: RangeOverflow = false, want true")
	}
	in.SetValue("50")
	if v := in.Validity(); !v.Valid() {
		t.Errorf("number 50 in [10,100]: Valid() = false, want true; state=%+v", v)
	}
}

func TestInput_NumberStepMismatch(t *testing.T) {
	in := newInput(t, map[string]string{"type": "number", "min": "0", "step": "10", "value": "15"})
	if v := in.Validity(); !v.StepMismatch {
		t.Errorf("number 15 with step 10: StepMismatch = false, want true")
	}
	in.SetValue("30")
	if v := in.Validity(); v.StepMismatch {
		t.Errorf("number 30 with step 10: StepMismatch = true, want false")
	}
}

func TestInput_StepAnyAllowed(t *testing.T) {
	in := newInput(t, map[string]string{"type": "number", "step": "any", "value": "13.7"})
	if v := in.Validity(); v.StepMismatch {
		t.Errorf("number with step=any: StepMismatch = true, want false")
	}
}

func TestInput_RangeDefaultNotBadInput(t *testing.T) {
	// range without value defaults to mid; empty value is not badInput.
	in := newInput(t, map[string]string{"type": "range", "min": "0", "max": "100"})
	if v := in.Validity(); v.BadInput {
		t.Errorf("empty range: BadInput = true, want false")
	}
}

// --- Validation: date / time / month / week / datetime-local ---

func TestInput_DateBadInput(t *testing.T) {
	cases := []struct {
		val    string
		wantOK bool
	}{
		{"2026-07-09", true},
		{"2026-13-01", false}, // invalid month
		{"not-a-date", false},
		{"", true}, // empty is not badInput
	}
	for _, c := range cases {
		in := newInput(t, map[string]string{"type": "date", "value": c.val})
		v := in.Validity()
		if got := !v.BadInput; got != c.wantOK {
			t.Errorf("date %q: BadInput=%v, want %v", c.val, v.BadInput, !c.wantOK)
		}
	}
}

func TestInput_TimeBadInput(t *testing.T) {
	cases := []struct {
		val    string
		wantOK bool
	}{
		{"13:45", true},
		{"13:45:30", true},
		{"25:00", false}, // invalid hour
		{"abc", false},
	}
	for _, c := range cases {
		in := newInput(t, map[string]string{"type": "time", "value": c.val})
		v := in.Validity()
		if got := !v.BadInput; got != c.wantOK {
			t.Errorf("time %q: BadInput=%v, want %v", c.val, v.BadInput, !c.wantOK)
		}
	}
}

func TestInput_MonthBadInput(t *testing.T) {
	in := newInput(t, map[string]string{"type": "month", "value": "2026-07"})
	if v := in.Validity(); v.BadInput {
		t.Errorf("month 2026-07: BadInput = true, want false")
	}
	in.SetValue("2026-13")
	if v := in.Validity(); !v.BadInput {
		t.Errorf("month 2026-13: BadInput = false, want true")
	}
}

func TestInput_WeekBadInput(t *testing.T) {
	in := newInput(t, map[string]string{"type": "week", "value": "2026-W28"})
	if v := in.Validity(); v.BadInput {
		t.Errorf("week 2026-W28: BadInput = true, want false")
	}
	in.SetValue("2026-W99")
	if v := in.Validity(); !v.BadInput {
		t.Errorf("week 2026-W99: BadInput = false, want true")
	}
}

func TestInput_DateTimeLocalBadInput(t *testing.T) {
	in := newInput(t, map[string]string{"type": "datetime-local", "value": "2026-07-09T13:45"})
	if v := in.Validity(); v.BadInput {
		t.Errorf("valid datetime-local: BadInput = true, want false")
	}
	in.SetValue("not-a-datetime")
	if v := in.Validity(); !v.BadInput {
		t.Errorf("invalid datetime-local: BadInput = false, want true")
	}
}

// --- Validation: color type ---

func TestInput_ColorBadInput(t *testing.T) {
	cases := []struct {
		val    string
		wantOK bool
	}{
		{"#ffffff", true},
		{"#000000", true},
		{"#abc", false},  // too short
		{"#gggggg", false},
		{"ffffff", false}, // missing #
	}
	for _, c := range cases {
		in := newInput(t, map[string]string{"type": "color", "value": c.val})
		v := in.Validity()
		if got := !v.BadInput; got != c.wantOK {
			t.Errorf("color %q: BadInput=%v, want %v", c.val, v.BadInput, !c.wantOK)
		}
	}
}

// --- Validation: pattern ---

func TestInput_PatternMismatch(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "pattern": "[0-9]{4}", "value": "abcd"})
	if v := in.Validity(); !v.PatternMismatch {
		t.Errorf("pattern [0-9]{4} with abcd: PatternMismatch = false, want true")
	}
	in.SetValue("1234")
	if v := in.Validity(); v.PatternMismatch {
		t.Errorf("pattern [0-9]{4} with 1234: PatternMismatch = true, want false")
	}
}

func TestInput_PatternNotCheckedWhenEmpty(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "pattern": "[0-9]{4}", "value": ""})
	if v := in.Validity(); v.PatternMismatch {
		t.Errorf("pattern with empty value: PatternMismatch = true, want false")
	}
}

// --- Validation: maxlength / minlength ---

func TestInput_TooLong(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "maxlength": "5", "value": "abcdef"})
	if v := in.Validity(); !v.TooLong {
		t.Errorf("maxlength 5 with abcdef: TooLong = false, want true")
	}
	in.SetValue("abc")
	if v := in.Validity(); v.TooLong {
		t.Errorf("maxlength 5 with abc: TooLong = true, want false")
	}
}

func TestInput_TooShort(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "minlength": "3", "value": "ab"})
	if v := in.Validity(); !v.TooShort {
		t.Errorf("minlength 3 with ab: TooShort = false, want true")
	}
	in.SetValue("abc")
	if v := in.Validity(); v.TooShort {
		t.Errorf("minlength 3 with abc: TooShort = true, want false")
	}
}

func TestInput_TooShortEmptyExempt(t *testing.T) {
	in := newInput(t, map[string]string{"type": "text", "minlength": "3", "value": ""})
	if v := in.Validity(); v.TooShort {
		t.Errorf("minlength with empty value: TooShort = true, want false (empty is exempt)")
	}
}

// --- Validation: checkbox / radio ---

func TestInput_CheckboxValueMissing(t *testing.T) {
	in := newInput(t, map[string]string{"type": "checkbox", "required": "required"})
	if v := in.Validity(); !v.ValueMissing {
		t.Errorf("required unchecked checkbox: ValueMissing = false, want true")
	}
	in.SetChecked(true)
	if v := in.Validity(); v.ValueMissing {
		t.Errorf("required checked checkbox: ValueMissing = true, want false")
	}
}

func TestInput_RadioValueMissingWithoutForm(t *testing.T) {
	// Radio not in any form with no group: relies on its own checked state.
	in := newInput(t, map[string]string{"type": "radio", "name": "grp", "required": "required"})
	if v := in.Validity(); !v.ValueMissing {
		t.Errorf("required unchecked radio: ValueMissing = false, want true")
	}
	in.SetChecked(true)
	if v := in.Validity(); v.ValueMissing {
		t.Errorf("required checked radio: ValueMissing = true, want false")
	}
}

func TestInput_RadioGroupSatisfied(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	radio1 := doc.CreateElement("input")
	radio1.SetAttribute("type", "radio")
	radio1.SetAttribute("name", "color")
	radio1.SetAttribute("required", "required")
	radio2 := doc.CreateElement("input")
	radio2.SetAttribute("type", "radio")
	radio2.SetAttribute("name", "color")
	_ = form.AppendChild(radio1)
	_ = form.AppendChild(radio2)

	// Neither checked: ValueMissing true for radio1.
	in1, _ := ToInputElement(radio1)
	if v := in1.Validity(); !v.ValueMissing {
		t.Errorf("radio group none checked: ValueMissing = false, want true")
	}
	// Check radio2: group now satisfied for radio1 too.
	radio2.SetAttribute("checked", "checked")
	if v := in1.Validity(); v.ValueMissing {
		t.Errorf("radio group sibling checked: ValueMissing = true, want false")
	}
}

// --- WillValidate / CheckValidity ---

func TestInput_WillValidate(t *testing.T) {
	// disabled never validates
	in := newInput(t, map[string]string{"type": "text", "disabled": "disabled"})
	if in.WillValidate() {
		t.Errorf("disabled input: WillValidate = true, want false")
	}
	// hidden never validates
	in = newInput(t, map[string]string{"type": "hidden"})
	if in.WillValidate() {
		t.Errorf("hidden input: WillValidate = true, want false")
	}
	// button never validates
	in = newInput(t, map[string]string{"type": "button"})
	if in.WillValidate() {
		t.Errorf("button input: WillValidate = true, want false")
	}
	// text validates
	in = newInput(t, map[string]string{"type": "text"})
	if !in.WillValidate() {
		t.Errorf("text input: WillValidate = false, want true")
	}
}

func TestInput_CheckValidity(t *testing.T) {
	// Valid input passes
	in := newInput(t, map[string]string{"type": "text", "value": "hello"})
	if !in.CheckValidity() {
		t.Errorf("valid input: CheckValidity = false, want true")
	}
	// Required empty fails
	in = newInput(t, map[string]string{"type": "text", "required": "required"})
	if in.CheckValidity() {
		t.Errorf("required empty input: CheckValidity = true, want false")
	}
}

// --- ValidationMessage ---

func TestInput_ValidationMessage(t *testing.T) {
	cases := []struct {
		name    string
		attrs   map[string]string
		wantMsg string
	}{
		{"valueMissing", map[string]string{"type": "text", "required": "required"}, "Please fill out this field."},
		{"typeMismatch", map[string]string{"type": "email", "value": "bad"}, "Please enter a valid value."},
		{"patternMismatch", map[string]string{"type": "text", "pattern": "[0-9]+", "value": "abc"}, "Please match the requested format."},
		{"rangeUnderflow", map[string]string{"type": "number", "min": "10", "value": "5"}, "Value is too low."},
		{"rangeOverflow", map[string]string{"type": "number", "max": "100", "value": "150"}, "Value is too high."},
		{"tooLong", map[string]string{"type": "text", "maxlength": "3", "value": "abcd"}, "Please shorten this text."},
		{"tooShort", map[string]string{"type": "text", "minlength": "5", "value": "ab"}, "Please lengthen this text."},
		{"valid", map[string]string{"type": "text", "value": "ok"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newInput(t, c.attrs)
			v := in.Validity()
			got := v.ValidationMessage(in.El)
			if got != c.wantMsg {
				t.Errorf("ValidationMessage() = %q, want %q", got, c.wantMsg)
			}
		})
	}
}

// --- ToInputElement rejects non-input elements ---

func TestToInputElement_RejectsNonInput(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	if _, ok := ToInputElement(div); ok {
		t.Errorf("ToInputElement(div) = true, want false")
	}
	if _, ok := ToInputElement(nil); ok {
		t.Errorf("ToInputElement(nil) = true, want false")
	}
}

// --- SetType ---

func TestInput_SetType(t *testing.T) {
	in := newInput(t, nil)
	in.SetType(InputEmail)
	if got := in.Type(); got != InputEmail {
		t.Fatalf("after SetType(Email): Type() = %q, want %q", got, InputEmail)
	}
}

// --- Min/Max/Step/Pattern/MaxLength/MinLength accessors ---

func TestInput_AttributeAccessors(t *testing.T) {
	in := newInput(t, map[string]string{
		"min":       "10",
		"max":       "100",
		"step":      "5",
		"pattern":   "[0-9]+",
		"maxlength": "8",
		"minlength": "2",
		"name":      "age",
		"placeholder": "Enter age",
	})
	if got := in.Min(); got != "10" {
		t.Errorf("Min() = %q, want 10", got)
	}
	if got := in.Max(); got != "100" {
		t.Errorf("Max() = %q, want 100", got)
	}
	if got := in.Step(); got != "5" {
		t.Errorf("Step() = %q, want 5", got)
	}
	if got := in.Pattern(); got != "[0-9]+" {
		t.Errorf("Pattern() = %q, want [0-9]+", got)
	}
	if got := in.MaxLength(); got != 8 {
		t.Errorf("MaxLength() = %d, want 8", got)
	}
	if got := in.MinLength(); got != 2 {
		t.Errorf("MinLength() = %d, want 2", got)
	}
	if got := in.Name(); got != "age" {
		t.Errorf("Name() = %q, want age", got)
	}
	if got := in.Placeholder(); got != "Enter age" {
		t.Errorf("Placeholder() = %q, want Enter age", got)
	}
}

func TestInput_MaxLengthMinLengthUnset(t *testing.T) {
	in := newInput(t, nil)
	if got := in.MaxLength(); got != -1 {
		t.Errorf("unset MaxLength() = %d, want -1", got)
	}
	if got := in.MinLength(); got != -1 {
		t.Errorf("unset MinLength() = %d, want -1", got)
	}
}

// --- Form ancestor lookup ---

func TestFindFormAncestor(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	div := doc.CreateElement("div")
	input := doc.CreateElement("input")
	_ = form.AppendChild(div)
	_ = div.AppendChild(input)

	in, _ := ToInputElement(input)
	if got := in.Form(); got == nil {
		t.Errorf("Form() = nil, want form element")
	} else if got != form {
		t.Errorf("Form() = %p, want %p", got, form)
	}
}

func TestFindFormAncestor_NoForm(t *testing.T) {
	doc := dom.NewDocument()
	input := doc.CreateElement("input")
	in, _ := ToInputElement(input)
	if got := in.Form(); got != nil {
		t.Errorf("Form() = %v, want nil", got)
	}
}

// --- Autofocus / Multiple ---

func TestInput_AutofocusMultiple(t *testing.T) {
	in := newInput(t, map[string]string{"autofocus": "autofocus", "multiple": "multiple"})
	if !in.Autofocus() {
		t.Error("Autofocus() = false, want true")
	}
	if !in.Multiple() {
		t.Error("Multiple() = false, want true")
	}
}

// --- Datalist / list attribute ---

func TestInput_ListNoAttr(t *testing.T) {
	in := newInput(t, nil)
	if dl := in.List(); dl != nil {
		t.Fatal("List() = non-nil, want nil when no list attribute")
	}
}

func TestInput_ListNonExistent(t *testing.T) {
	in := newInput(t, map[string]string{"list": "nonexistent"})
	if dl := in.List(); dl != nil {
		t.Fatal("List() = non-nil, want nil when datalist id does not exist")
	}
}

func TestInput_ListRefersToDatalist(t *testing.T) {
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	dl.SetAttribute("id", "colors")
	doc.CreateElement("body").AppendChild(dl)

	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "red")
	dl.AppendChild(opt1)
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "green")
	dl.AppendChild(opt2)
	opt3 := doc.CreateElement("option")
	opt3.SetAttribute("value", "blue")
	dl.AppendChild(opt3)

	// Add dummy root so CreateElement attaches to document
	root := doc.CreateElement("html")
	root.AppendChild(dl)
	doc.AppendChild(root)

	input := doc.CreateElement("input")
	input.SetAttribute("list", "colors")
	in, ok := ToInputElement(input)
	if !ok {
		t.Fatal("ToInputElement failed")
	}

	dlElem := in.List()
	if dlElem == nil {
		t.Fatal("List() = nil, want non-nil datalist")
	}
	if dlElem.Length() != 3 {
		t.Fatalf("Length() = %d, want 3", dlElem.Length())
	}
}

func TestInput_ListRefersToNonDatalist(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.SetAttribute("id", "not-datalist")
	root := doc.CreateElement("html")
	root.AppendChild(div)
	doc.AppendChild(root)

	input := doc.CreateElement("input")
	input.SetAttribute("list", "not-datalist")
	in, ok := ToInputElement(input)
	if !ok {
		t.Fatal("ToInputElement failed")
	}
	if dl := in.List(); dl != nil {
		t.Fatal("List() returned non-nil for non-datalist element")
	}
}

func TestDatalist_SuggestionsFor(t *testing.T) {
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	for _, v := range []string{"Apple", "Apricot", "Banana", "Blueberry", "Cherry"} {
		opt := doc.CreateElement("option")
		opt.SetAttribute("value", v)
		dl.AppendChild(opt)
	}
	dle, _ := ToDataListElement(dl)

	tests := []struct {
		prefix string
		want   int
	}{
		{"", 0},
		{"A", 2},  // Apple, Apricot
		{"Ap", 2}, // Apple, Apricot
		{"B", 2},  // Banana, Blueberry
		{"C", 1},  // Cherry
		{"X", 0},
		{"a", 2}, // case-insensitive
	}
	for _, tt := range tests {
		got := dle.SuggestionsFor(tt.prefix)
		if len(got) != tt.want {
			t.Errorf("SuggestionsFor(%q) = %d results, want %d: %v", tt.prefix, len(got), tt.want, got)
		}
	}
}

func TestDatalist_SuggestionsWithLabel(t *testing.T) {
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	for _, pair := range [][2]string{
		{"US", "United States"},
		{"CA", "Canada"},
		{"MX", "Mexico"},
	} {
		opt := doc.CreateElement("option")
		opt.SetAttribute("value", pair[0])
		opt.SetAttribute("label", pair[1])
		dl.AppendChild(opt)
	}
	dle, _ := ToDataListElement(dl)

	got := dle.SuggestionsFor("U")
	if len(got) != 1 {
		t.Fatalf("SuggestionsFor('U') = %d results, want 1: %v", len(got), got)
	}
	if got[0] != "United States" {
		t.Errorf("label = %q, want 'United States'", got[0])
	}
}

func TestInput_AcceptedLabels(t *testing.T) {
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	dl.SetAttribute("id", "fruits")
	for _, v := range []string{"Apple", "Apricot", "Avocado", "Banana"} {
		opt := doc.CreateElement("option")
		opt.SetAttribute("value", v)
		dl.AppendChild(opt)
	}
	root := doc.CreateElement("html")
	root.AppendChild(dl)
	doc.AppendChild(root)

	input := doc.CreateElement("input")
	input.SetAttribute("list", "fruits")
	in, _ := ToInputElement(input)

	labels := in.AcceptedLabels("Ap")
	if len(labels) != 2 {
		t.Fatalf("AcceptedLabels('Ap') = %d results, want 2: %v", len(labels), labels)
	}
}
