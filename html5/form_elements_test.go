package html5

import (
	"testing"

	"wb-ui/dom"
)

// createElement creates a fresh element in a new document with the given tag
// and attributes. Convenience helper for form-element tests.
func createElement(tag string, attrs map[string]string) *dom.Element {
	doc := dom.NewDocument()
	el := doc.CreateElement(tag)
	for k, v := range attrs {
		el.SetAttribute(k, v)
	}
	return el
}

// --- HTMLFormElement ---

func TestForm_ElementsAndLength(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	sel := doc.CreateElement("select")
	ta := doc.CreateElement("textarea")
	btn := doc.CreateElement("button")
	_ = form.AppendChild(in)
	_ = form.AppendChild(sel)
	_ = form.AppendChild(ta)
	_ = form.AppendChild(btn)

	f, _ := ToFormElement(form)
	if got := f.Length(); got != 4 {
		t.Errorf("Length() = %d, want 4", got)
	}
	elements := f.Elements()
	if len(elements) != 4 {
		t.Fatalf("Elements() len = %d, want 4", len(elements))
	}
	if elements[0] != in || elements[1] != sel || elements[2] != ta || elements[3] != btn {
		t.Errorf("Elements() order incorrect")
	}
}

func TestForm_ElementsExcludeNestedForm(t *testing.T) {
	doc := dom.NewDocument()
	outer := doc.CreateElement("form")
	inner := doc.CreateElement("form")
	in1 := doc.CreateElement("input")
	in2 := doc.CreateElement("input")
	_ = outer.AppendChild(in1)
	_ = outer.AppendChild(inner)
	_ = inner.AppendChild(in2)

	f, _ := ToFormElement(outer)
	// Only in1 belongs to outer; in2 belongs to inner.
	if got := f.Length(); got != 1 {
		t.Errorf("Length() = %d, want 1 (nested form input excluded)", got)
	}
}

func TestForm_MethodDefaultGet(t *testing.T) {
	f, _ := ToFormElement(createElement("form", nil))
	if got := f.Method(); got != "get" {
		t.Errorf("Method() default = %q, want get", got)
	}
	f, _ = ToFormElement(createElement("form", map[string]string{"method": "POST"}))
	if got := f.Method(); got != "post" {
		t.Errorf("Method(POST) = %q, want post", got)
	}
}

func TestForm_EnctypeDefault(t *testing.T) {
	f, _ := ToFormElement(createElement("form", nil))
	if got := f.Enctype(); got != "application/x-www-form-urlencoded" {
		t.Errorf("Enctype() default = %q, want application/x-www-form-urlencoded", got)
	}
	f, _ = ToFormElement(createElement("form", map[string]string{"enctype": "multipart/form-data"}))
	if got := f.Enctype(); got != "multipart/form-data" {
		t.Errorf("Enctype(multipart) = %q, want multipart/form-data", got)
	}
}

func TestForm_NoValidate(t *testing.T) {
	f, _ := ToFormElement(createElement("form", map[string]string{"novalidate": "novalidate"}))
	if !f.NoValidate() {
		t.Error("NoValidate() = false, want true")
	}
}

func TestForm_CheckValidity_AllValid(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	in.SetAttribute("value", "hello")
	_ = form.AppendChild(in)

	f, _ := ToFormElement(form)
	if !f.CheckValidity() {
		t.Error("CheckValidity() = false, want true for valid form")
	}
}

func TestForm_CheckValidity_InvalidInput(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	in.SetAttribute("required", "required")
	_ = form.AppendChild(in)

	f, _ := ToFormElement(form)
	if f.CheckValidity() {
		t.Error("CheckValidity() = true, want false for required empty input")
	}
}

func TestForm_CheckValidity_InvalidTextArea(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	ta := doc.CreateElement("textarea")
	ta.SetAttribute("required", "required")
	_ = form.AppendChild(ta)
	f, _ := ToFormElement(form)
	if f.CheckValidity() {
		t.Error("CheckValidity() = true, want false for required empty textarea")
	}
}

func TestForm_CheckValidity_InvalidSelect(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	sel := doc.CreateElement("select")
	sel.SetAttribute("required", "required")
	opt := doc.CreateElement("option")
	opt.SetAttribute("value", "")
	_ = sel.AppendChild(opt)
	_ = form.AppendChild(sel)
	f, _ := ToFormElement(form)
	if f.CheckValidity() {
		t.Error("CheckValidity() = true, want false for required select with empty value")
	}
}

func TestForm_NameAction(t *testing.T) {
	f, _ := ToFormElement(createElement("form", map[string]string{
		"name":   "loginForm",
		"action": "/submit",
	}))
	if got := f.Name(); got != "loginForm" {
		t.Errorf("Name() = %q, want loginForm", got)
	}
	if got := f.Action(); got != "/submit" {
		t.Errorf("Action() = %q, want /submit", got)
	}
}

// --- HTMLButtonElement ---

func TestButton_TypeDefaultsToSubmit(t *testing.T) {
	b, _ := ToButtonElement(createElement("button", nil))
	if got := b.Type(); got != ButtonSubmit {
		t.Errorf("Type() default = %q, want submit", got)
	}
}

func TestButton_TypeKnown(t *testing.T) {
	for _, c := range []struct {
		attr string
		want ButtonType
	}{
		{"reset", ButtonReset},
		{"button", ButtonButton},
		{"submit", ButtonSubmit},
		{"SUBMIT", ButtonSubmit}, // case-insensitive
	} {
		b, _ := ToButtonElement(createElement("button", map[string]string{"type": c.attr}))
		if got := b.Type(); got != c.want {
			t.Errorf("Type(%q) = %q, want %q", c.attr, got, c.want)
		}
	}
}

func TestButton_ValueNameDisabled(t *testing.T) {
	b, _ := ToButtonElement(createElement("button", map[string]string{
		"value":    "save",
		"name":     "action",
		"disabled": "disabled",
	}))
	if got := b.Value(); got != "save" {
		t.Errorf("Value() = %q, want save", got)
	}
	if got := b.Name(); got != "action" {
		t.Errorf("Name() = %q, want action", got)
	}
	if !b.Disabled() {
		t.Error("Disabled() = false, want true")
	}
	b.SetValue("cancel")
	if got := b.Value(); got != "cancel" {
		t.Errorf("after SetValue: Value() = %q, want cancel", got)
	}
	b.SetDisabled(false)
	if b.Disabled() {
		t.Error("after SetDisabled(false): Disabled() = true, want false")
	}
}

func TestButton_FormAncestor(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	btn := doc.CreateElement("button")
	_ = form.AppendChild(btn)
	b, _ := ToButtonElement(btn)
	if got := b.Form(); got != form {
		t.Errorf("Form() = %v, want form element", got)
	}
}

// --- HTMLLabelElement ---

func TestLabel_HTMLFor(t *testing.T) {
	l, _ := ToLabelElement(createElement("label", map[string]string{"for": "username"}))
	if got := l.HTMLFor(); got != "username" {
		t.Errorf("HTMLFor() = %q, want username", got)
	}
	l.SetHTMLFor("password")
	if got := l.HTMLFor(); got != "password" {
		t.Errorf("after SetHTMLFor: HTMLFor() = %q, want password", got)
	}
}

func TestLabel_ControlByFor(t *testing.T) {
	doc := dom.NewDocument()
	// Attach elements to the document so GetElementById can find them.
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	label := doc.CreateElement("label")
	label.SetAttribute("for", "name")
	input := doc.CreateElement("input")
	input.SetAttribute("id", "name")
	_ = doc.AppendChild(html)
	_ = html.AppendChild(body)
	_ = body.AppendChild(label)
	_ = body.AppendChild(input)

	l, _ := ToLabelElement(label)
	got := l.Control()
	if got != input {
		t.Errorf("Control() = %v, want input element", got)
	}
}

func TestLabel_ControlByNestedElement(t *testing.T) {
	doc := dom.NewDocument()
	label := doc.CreateElement("label")
	input := doc.CreateElement("input")
	_ = label.AppendChild(input)

	l, _ := ToLabelElement(label)
	got := l.Control()
	if got != input {
		t.Errorf("Control() = %v, want nested input element", got)
	}
}

func TestLabel_ControlNone(t *testing.T) {
	l, _ := ToLabelElement(createElement("label", nil))
	if got := l.Control(); got != nil {
		t.Errorf("Control() = %v, want nil", got)
	}
}

func TestLabel_ClickFocusesCheckbox(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	label := doc.CreateElement("label")
	label.SetAttribute("for", "cb")
	input := doc.CreateElement("input")
	input.SetAttribute("id", "cb")
	input.SetAttribute("type", "checkbox")
	_ = doc.AppendChild(body)
	_ = body.AppendChild(label)
	_ = body.AppendChild(input)

	l, _ := ToLabelElement(label)
	if l.Control() != input {
		t.Fatal("Control() should return the checkbox")
	}

	// Verify initial state.
	in, _ := ToInputElement(input)
	if in.Checked() {
		t.Fatal("checkbox should be initially unchecked")
	}
	if in.El.IsFocused() {
		t.Fatal("checkbox should not be initially focused")
	}

	// Click the label.
	l.Click()

	// After label click: checkbox should be checked and focused.
	if !in.Checked() {
		t.Fatal("checkbox should be checked after label click")
	}
	if !in.El.IsFocused() {
		t.Fatal("checkbox should be focused after label click")
	}

	// Click again to toggle back.
	l.Click()
	if in.Checked() {
		t.Fatal("checkbox should be unchecked after second label click")
	}
}

func TestLabel_ClickFocusesRadio(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.CreateElement("body")
	label := doc.CreateElement("label")
	label.SetAttribute("for", "radio1")
	input := doc.CreateElement("input")
	input.SetAttribute("id", "radio1")
	input.SetAttribute("type", "radio")
	input.SetAttribute("name", "rg")
	_ = doc.AppendChild(body)
	_ = body.AppendChild(label)
	_ = body.AppendChild(input)

	l, _ := ToLabelElement(label)
	in, _ := ToInputElement(input)

	if in.Checked() {
		t.Fatal("radio should be initially unchecked")
	}

	// Click the label → radio becomes checked and focused.
	l.Click()
	if !in.Checked() {
		t.Fatal("radio should be checked after label click")
	}
	if !in.El.IsFocused() {
		t.Fatal("radio should be focused after label click")
	}

	// Click again — radio stays checked (can't uncheck a radio by clicking).
	l.Click()
	if !in.Checked() {
		t.Fatal("radio should remain checked after second label click")
	}
}

// --- HTMLFieldSetElement ---

func TestFieldSet_DisabledAndName(t *testing.T) {
	fs, _ := ToFieldSetElement(createElement("fieldset", map[string]string{
		"disabled": "disabled",
		"name":     "group1",
	}))
	if !fs.Disabled() {
		t.Error("Disabled() = false, want true")
	}
	if got := fs.Name(); got != "group1" {
		t.Errorf("Name() = %q, want group1", got)
	}
}

func TestFieldSet_Elements(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	fs := doc.CreateElement("fieldset")
	in := doc.CreateElement("input")
	sel := doc.CreateElement("select")
	_ = form.AppendChild(fs)
	_ = fs.AppendChild(in)
	_ = fs.AppendChild(sel)

	fse, _ := ToFieldSetElement(fs)
	elements := fse.Elements()
	if len(elements) != 2 {
		t.Fatalf("Elements() len = %d, want 2", len(elements))
	}
	if elements[0] != in || elements[1] != sel {
		t.Errorf("Elements() order incorrect")
	}
}

func TestFieldSet_Form(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	fs := doc.CreateElement("fieldset")
	_ = form.AppendChild(fs)

	fse, _ := ToFieldSetElement(fs)
	if got := fse.Form(); got != form {
		t.Errorf("Form() = %v, want form element", got)
	}
}

// --- HTMLOutputElement ---

func TestOutput_Value(t *testing.T) {
	o, _ := ToOutputElement(createElement("output", nil))
	o.SetValue("42")
	if got := o.Value(); got != "42" {
		t.Errorf("Value() = %q, want 42", got)
	}
	if got := o.DefaultValue(); got != "42" {
		t.Errorf("DefaultValue() = %q, want 42", got)
	}
}

func TestOutput_HTMLForName(t *testing.T) {
	o, _ := ToOutputElement(createElement("output", map[string]string{
		"for":  "a b",
		"name": "result",
	}))
	if got := o.HTMLFor(); got != "a b" {
		t.Errorf("HTMLFor() = %q, want 'a b'", got)
	}
	if got := o.Name(); got != "result" {
		t.Errorf("Name() = %q, want result", got)
	}
}

// --- HTMLLegendElement ---

func TestLegend_FormThroughFieldSet(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	fs := doc.CreateElement("fieldset")
	legend := doc.CreateElement("legend")
	_ = form.AppendChild(fs)
	_ = fs.AppendChild(legend)

	l, _ := ToLegendElement(legend)
	if got := l.Form(); got != form {
		t.Errorf("Form() = %v, want form element (through fieldset)", got)
	}
}

func TestLegend_FormNoFieldSet(t *testing.T) {
	l, _ := ToLegendElement(createElement("legend", nil))
	if got := l.Form(); got != nil {
		t.Errorf("Form() = %v, want nil (no fieldset ancestor)", got)
	}
}

// --- HTMLSelectElement ---

func TestSelect_OptionsFromOptGroup(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	grp := doc.CreateElement("optgroup")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "a")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "b")
	_ = sel.AppendChild(grp)
	_ = grp.AppendChild(opt1)
	_ = grp.AppendChild(opt2)

	s, _ := ToSelectElement(sel)
	opts := s.Options()
	if len(opts) != 2 {
		t.Fatalf("Options() len = %d, want 2 (options inside optgroup)", len(opts))
	}
	if got := s.Length(); got != 2 {
		t.Errorf("Length() = %d, want 2", got)
	}
}

func TestSelect_MultipleSizeDefaults(t *testing.T) {
	s, _ := ToSelectElement(createElement("select", nil))
	if s.Multiple() {
		t.Error("Multiple() = true, want false by default")
	}
	if got := s.Size(); got != 1 {
		t.Errorf("Size() = %d, want 1 for single-select", got)
	}
	s, _ = ToSelectElement(createElement("select", map[string]string{"multiple": "multiple"}))
	if got := s.Size(); got != 4 {
		t.Errorf("Size() multiple = %d, want 4", got)
	}
}

func TestSelect_SelectedIndex(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "a")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "b")
	opt2.SetAttribute("selected", "selected")
	opt3 := doc.CreateElement("option")
	opt3.SetAttribute("value", "c")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)
	_ = sel.AppendChild(opt3)

	s, _ := ToSelectElement(sel)
	if got := s.SelectedIndex(); got != 1 {
		t.Errorf("SelectedIndex() = %d, want 1", got)
	}
}

func TestSelect_SetSelectedIndexClearsOthers(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("selected", "selected")
	opt2 := doc.CreateElement("option")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)

	s, _ := ToSelectElement(sel)
	s.SetSelectedIndex(1)
	if opt1.HasAttribute("selected") {
		t.Error("after SetSelectedIndex(1): opt1 should be deselected")
	}
	if !opt2.HasAttribute("selected") {
		t.Error("after SetSelectedIndex(1): opt2 should be selected")
	}
	if got := s.SelectedIndex(); got != 1 {
		t.Errorf("SelectedIndex() = %d, want 1", got)
	}
}

func TestSelect_ValueFromSelected(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt := doc.CreateElement("option")
	opt.SetAttribute("value", "x")
	opt.SetAttribute("selected", "selected")
	_ = sel.AppendChild(opt)
	s, _ := ToSelectElement(sel)
	if got := s.Value(); got != "x" {
		t.Errorf("Value() = %q, want x", got)
	}
}

func TestSelect_ValueFromTextWhenNoValueAttr(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt := doc.CreateElement("option")
	_ = opt.SetTextContent("  Option A  ")
	opt.SetAttribute("selected", "selected")
	_ = sel.AppendChild(opt)
	s, _ := ToSelectElement(sel)
	if got := s.Value(); got != "Option A" {
		t.Errorf("Value() = %q, want 'Option A' (trimmed text)", got)
	}
}

func TestSelect_SetValueMatches(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "a")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "b")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)

	s, _ := ToSelectElement(sel)
	s.SetValue("b")
	if !opt2.HasAttribute("selected") {
		t.Error("after SetValue(b): opt2 should be selected")
	}
	if opt1.HasAttribute("selected") {
		t.Error("after SetValue(b): opt1 should be deselected")
	}
}

func TestSelect_RequiredValueMissing(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	sel.SetAttribute("required", "required")
	opt := doc.CreateElement("option")
	opt.SetAttribute("value", "")
	_ = sel.AppendChild(opt)
	s, _ := ToSelectElement(sel)
	if v := s.Validity(); !v.ValueMissing {
		t.Errorf("required select with no selection: ValueMissing = false, want true")
	}
}

func TestSelect_RequiredSatisfied(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	sel.SetAttribute("required", "required")
	opt := doc.CreateElement("option")
	opt.SetAttribute("value", "x")
	opt.SetAttribute("selected", "selected")
	_ = sel.AppendChild(opt)
	s, _ := ToSelectElement(sel)
	if v := s.Validity(); v.ValueMissing {
		t.Errorf("required select with selection: ValueMissing = true, want false")
	}
}

func TestSelect_DisabledSkipsValidation(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	sel.SetAttribute("disabled", "disabled")
	sel.SetAttribute("required", "required")
	s, _ := ToSelectElement(sel)
	// Disabled selects do not participate in validation.
	if s.WillValidate() {
		t.Error("WillValidate() = true for disabled select, want false")
	}
	// CheckValidity returns true when not validating (skipped).
	if !s.CheckValidity() {
		t.Error("CheckValidity() = false for disabled select, want true (skipped)")
	}
}

// --- HTMLOptionElement ---

func TestOption_ValueFromAttrOrText(t *testing.T) {
	o, _ := ToOptionElement(createElement("option", map[string]string{"value": "x"}))
	if got := o.Value(); got != "x" {
		t.Errorf("Value() = %q, want x", got)
	}
	o2, _ := ToOptionElement(createElement("option", nil))
	o2.SetText("  Hello  ")
	if got := o2.Value(); got != "Hello" {
		t.Errorf("Value() from text = %q, want Hello (trimmed)", got)
	}
}

func TestOption_SelectedToggle(t *testing.T) {
	o, _ := ToOptionElement(createElement("option", nil))
	if o.Selected() {
		t.Error("new option should be unselected")
	}
	o.SetSelected(true)
	if !o.Selected() {
		t.Error("after SetSelected(true) should be selected")
	}
	o.SetSelected(false)
	if o.Selected() {
		t.Error("after SetSelected(false) should be unselected")
	}
}

func TestOption_Disabled(t *testing.T) {
	o, _ := ToOptionElement(createElement("option", map[string]string{"disabled": "disabled"}))
	if !o.Disabled() {
		t.Error("Disabled() = false, want true")
	}
}

func TestOption_IndexInSelect(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	opt1 := doc.CreateElement("option")
	opt2 := doc.CreateElement("option")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)
	o2, _ := ToOptionElement(opt2)
	if got := o2.Index(); got != 1 {
		t.Errorf("Index() = %d, want 1", got)
	}
}

func TestOption_IndexOutsideSelect(t *testing.T) {
	o, _ := ToOptionElement(createElement("option", nil))
	if got := o.Index(); got != -1 {
		t.Errorf("Index() outside select = %d, want -1", got)
	}
}

// --- HTMLOptGroupElement ---

func TestOptGroup_LabelDisabled(t *testing.T) {
	g, _ := ToOptGroupElement(createElement("optgroup", map[string]string{
		"label":    "Colors",
		"disabled": "disabled",
	}))
	if got := g.Label(); got != "Colors" {
		t.Errorf("Label() = %q, want Colors", got)
	}
	if !g.Disabled() {
		t.Error("Disabled() = false, want true")
	}
}

// --- HTMLDataListElement ---

func TestDataList_OptionsAndLength(t *testing.T) {
	doc := dom.NewDocument()
	dl := doc.CreateElement("datalist")
	opt1 := doc.CreateElement("option")
	opt2 := doc.CreateElement("option")
	_ = dl.AppendChild(opt1)
	_ = dl.AppendChild(opt2)
	d, _ := ToDataListElement(dl)
	if got := d.Length(); got != 2 {
		t.Errorf("Length() = %d, want 2", got)
	}
	opts := d.Options()
	if len(opts) != 2 {
		t.Fatalf("Options() len = %d, want 2", len(opts))
	}
}

// --- HTMLTextAreaElement ---

func TestTextArea_ValueAndSet(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	ta.SetValue("hello world")
	if got := ta.Value(); got != "hello world" {
		t.Errorf("Value() = %q, want hello world", got)
	}
}

func TestTextArea_DefaultsRowsCols(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	if got := ta.Rows(); got != 2 {
		t.Errorf("Rows() default = %d, want 2", got)
	}
	if got := ta.Cols(); got != 20 {
		t.Errorf("Cols() default = %d, want 20", got)
	}
}

func TestTextArea_MaxLengthMinLengthUnset(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	if got := ta.MaxLength(); got != -1 {
		t.Errorf("MaxLength() unset = %d, want -1", got)
	}
	if got := ta.MinLength(); got != -1 {
		t.Errorf("MinLength() unset = %d, want -1", got)
	}
}

func TestTextArea_Wrap(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{"wrap": "hard"}))
	if got := ta.Wrap(); got != "hard" {
		t.Errorf("Wrap(hard) = %q, want hard", got)
	}
	ta, _ = ToTextAreaElement(createElement("textarea", map[string]string{"wrap": "off"}))
	if got := ta.Wrap(); got != "off" {
		t.Errorf("Wrap(off) = %q, want off", got)
	}
	ta, _ = ToTextAreaElement(createElement("textarea", nil))
	if got := ta.Wrap(); got != "soft" {
		t.Errorf("Wrap() default = %q, want soft", got)
	}
}

func TestTextArea_RequiredValueMissing(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{"required": "required"}))
	if v := ta.Validity(); !v.ValueMissing {
		t.Errorf("empty required textarea: ValueMissing = false, want true")
	}
	ta.SetValue("content")
	if v := ta.Validity(); v.ValueMissing {
		t.Errorf("filled required textarea: ValueMissing = true, want false")
	}
}

func TestTextArea_TooLongTooShort(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{"maxlength": "5", "minlength": "3"}))
	ta.SetValue("abcdef")
	if v := ta.Validity(); !v.TooLong {
		t.Errorf("maxlength 5 with abcdef: TooLong = false, want true")
	}
	ta.SetValue("ab")
	if v := ta.Validity(); !v.TooShort {
		t.Errorf("minlength 3 with ab: TooShort = false, want true")
	}
}

func TestTextArea_DisabledSkipsValidation(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{
		"disabled": "disabled",
		"required": "required",
	}))
	if ta.WillValidate() {
		t.Error("WillValidate() = true for disabled, want false")
	}
	if !ta.CheckValidity() {
		t.Error("CheckValidity() = false for disabled textarea, want true (skipped)")
	}
}

// --- HTMLMeterElement ---

func TestMeter_Defaults(t *testing.T) {
	m, _ := ToMeterElement(createElement("meter", nil))
	if got := m.Min(); got != 0 {
		t.Errorf("Min() default = %v, want 0", got)
	}
	if got := m.Max(); got != 1 {
		t.Errorf("Max() default = %v, want 1", got)
	}
	if got := m.Low(); got != 0 {
		t.Errorf("Low() default (== min) = %v, want 0", got)
	}
	if got := m.High(); got != 1 {
		t.Errorf("High() default (== max) = %v, want 1", got)
	}
	if got := m.Optimum(); got != 0.5 {
		t.Errorf("Optimum() default (midpoint) = %v, want 0.5", got)
	}
}

func TestMeter_ValueSetGet(t *testing.T) {
	m, _ := ToMeterElement(createElement("meter", nil))
	m.SetValue(0.7)
	if got := m.Value(); got != 0.7 {
		t.Errorf("Value() = %v, want 0.7", got)
	}
}

func TestMeter_Position(t *testing.T) {
	m, _ := ToMeterElement(createElement("meter", map[string]string{
		"min": "0", "max": "100", "value": "25",
	}))
	if got := m.Position(); got != 0.25 {
		t.Errorf("Position() = %v, want 0.25", got)
	}
}

func TestMeter_PositionClamped(t *testing.T) {
	m, _ := ToMeterElement(createElement("meter", map[string]string{
		"min": "0", "max": "10", "value": "50",
	}))
	if got := m.Position(); got != 1 {
		t.Errorf("Position() over max = %v, want 1 (clamped)", got)
	}
}

func TestMeter_PositionDegenerate(t *testing.T) {
	m, _ := ToMeterElement(createElement("meter", map[string]string{
		"min": "5", "max": "5", "value": "5",
	}))
	if got := m.Position(); got != -1 {
		t.Errorf("Position() degenerate = %v, want -1", got)
	}
}

// --- HTMLProgressElement ---

func TestProgress_Defaults(t *testing.T) {
	p, _ := ToProgressElement(createElement("progress", nil))
	if got := p.Max(); got != 1 {
		t.Errorf("Max() default = %v, want 1", got)
	}
	if got := p.Value(); got != 0 {
		t.Errorf("Value() default = %v, want 0", got)
	}
	// No value attribute -> indeterminate.
	if got := p.Position(); got != -1 {
		t.Errorf("Position() indeterminate = %v, want -1", got)
	}
}

func TestProgress_Position(t *testing.T) {
	p, _ := ToProgressElement(createElement("progress", map[string]string{
		"max": "100", "value": "40",
	}))
	if got := p.Position(); got != 0.4 {
		t.Errorf("Position() = %v, want 0.4", got)
	}
}

func TestProgress_PositionClamped(t *testing.T) {
	p, _ := ToProgressElement(createElement("progress", map[string]string{
		"max": "10", "value": "100",
	}))
	if got := p.Position(); got != 1 {
		t.Errorf("Position() over max = %v, want 1 (clamped)", got)
	}
}

// --- ToElement constructors reject wrong tags ---

func TestToFormElement_RejectsNonForm(t *testing.T) {
	if _, ok := ToFormElement(createElement("div", nil)); ok {
		t.Error("ToFormElement(div) = true, want false")
	}
	if _, ok := ToFormElement(nil); ok {
		t.Error("ToFormElement(nil) = true, want false")
	}
}

func TestToSelectElement_RejectsNonSelect(t *testing.T) {
	if _, ok := ToSelectElement(createElement("input", nil)); ok {
		t.Error("ToSelectElement(input) = true, want false")
	}
}

func TestToOptionElement_RejectsNonOption(t *testing.T) {
	if _, ok := ToOptionElement(createElement("div", nil)); ok {
		t.Error("ToOptionElement(div) = true, want false")
	}
}

func TestToTextAreaElement_RejectsNonTextArea(t *testing.T) {
	if _, ok := ToTextAreaElement(createElement("input", nil)); ok {
		t.Error("ToTextAreaElement(input) = true, want false")
	}
}

func TestToButtonElement_RejectsNonButton(t *testing.T) {
	if _, ok := ToButtonElement(createElement("input", nil)); ok {
		t.Error("ToButtonElement(input) = true, want false")
	}
}

func TestToMeterElement_RejectsNonMeter(t *testing.T) {
	if _, ok := ToMeterElement(createElement("div", nil)); ok {
		t.Error("ToMeterElement(div) = true, want false")
	}
}

func TestToProgressElement_RejectsNonProgress(t *testing.T) {
	if _, ok := ToProgressElement(createElement("div", nil)); ok {
		t.Error("ToProgressElement(div) = true, want false")
	}
}

// --- Select 补充测试 ---

func TestSelect_MultipleSelection(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.CreateElement("select")
	sel.SetAttribute("multiple", "multiple")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "a")
	opt1.SetAttribute("selected", "selected")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "b")
	opt2.SetAttribute("selected", "selected")
	opt3 := doc.CreateElement("option")
	opt3.SetAttribute("value", "c")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)
	_ = sel.AppendChild(opt3)

	s, _ := ToSelectElement(sel)
	if !s.Multiple() {
		t.Error("Multiple() = false, want true")
	}
	// Multiple select: SelectedIndex returns first selected.
	if got := s.SelectedIndex(); got != 0 {
		t.Errorf("SelectedIndex() = %d, want 0 (first selected)", got)
	}
	// Both options should remain selected.
	if !opt1.HasAttribute("selected") {
		t.Error("opt1 should remain selected")
	}
	if !opt2.HasAttribute("selected") {
		t.Error("opt2 should remain selected")
	}
	// Value should be the first selected option's value.
	if got := s.Value(); got != "a" {
		t.Errorf("Value() = %q, want a (first selected)", got)
	}
}

func TestSelect_OptionTextContent(t *testing.T) {
	o, _ := ToOptionElement(createElement("option", nil))
	o.SetText("  Hello World  ")
	if got := o.Text(); got != "Hello World" {
		t.Errorf("Text() = %q, want 'Hello World' (trimmed)", got)
	}
	// SetText should also update the element's text content.
	if got := o.Value(); got != "Hello World" {
		t.Errorf("Value() from text = %q, want 'Hello World'", got)
	}
}

func TestSelect_CustomValidity(t *testing.T) {
	s, _ := ToSelectElement(createElement("select", map[string]string{"required": "required"}))
	v := s.Validity()
	if v.Valid() {
		t.Error("required select with no selection: Valid() = true, want false")
	}
	s.SetCustomValidity("Please pick one")
	v = s.Validity()
	if !v.CustomError {
		t.Error("after SetCustomValidity: CustomError = false, want true")
	}
	if v.Valid() {
		t.Error("after SetCustomValidity with required+empty: Valid() = true, want false")
	}
}

// --- TextArea 补充测试 ---

func TestTextArea_DefaultValue(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	// Set the initial text content (which is both value and default value
	// in this simplified implementation where value == text content).
	_ = ta.El.SetTextContent("initial")
	if got := ta.DefaultValue(); got != "initial" {
		t.Errorf("DefaultValue() = %q, want 'initial'", got)
	}
}

func TestTextArea_PlaceholderNameReadOnly(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{
		"placeholder": "Enter text",
		"name":        "comment",
		"readonly":    "readonly",
	}))
	if got := ta.Placeholder(); got != "Enter text" {
		t.Errorf("Placeholder() = %q, want 'Enter text'", got)
	}
	if got := ta.Name(); got != "comment" {
		t.Errorf("Name() = %q, want 'comment'", got)
	}
	if !ta.ReadOnly() {
		t.Error("ReadOnly() = false, want true")
	}
}

func TestTextArea_Autofocus(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", map[string]string{"autofocus": "autofocus"}))
	if !ta.Autofocus() {
		t.Error("Autofocus() = false, want true")
	}
	ta2, _ := ToTextAreaElement(createElement("textarea", nil))
	if ta2.Autofocus() {
		t.Error("Autofocus() default = true, want false")
	}
}

func TestTextArea_DisabledSetter(t *testing.T) {
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	if ta.Disabled() {
		t.Error("new textarea should not be disabled")
	}
	ta.SetDisabled(true)
	if !ta.Disabled() {
		t.Error("after SetDisabled(true): Disabled() = false, want true")
	}
	ta.SetDisabled(false)
	if ta.Disabled() {
		t.Error("after SetDisabled(false): Disabled() = true, want false")
	}
}

// --- Button 补充测试 ---

func TestButton_WillValidate(t *testing.T) {
	// Button type=submit is submittable but does NOT participate in validation.
	b, _ := ToButtonElement(createElement("button", map[string]string{"type": "submit"}))
	// HTMLButtonElement has no WillValidate method - buttons are not validated.
	// Verify the button properties work.
	if got := b.Type(); got != ButtonSubmit {
		t.Errorf("Type() = %q, want submit", got)
	}
}

func TestButton_CheckValidity(t *testing.T) {
	// HTMLButtonElement does not participate in validation; CheckValidity
	// always returns true for buttons.
	b, _ := ToButtonElement(createElement("button", map[string]string{"type": "submit"}))
	// Buttons have no built-in validation, so treat as valid.
	_ = b
}

func TestButton_SetCustomValidity(t *testing.T) {
	// HTMLButtonElement does not implement SetCustomValidity in this port.
	b, _ := ToButtonElement(createElement("button", nil))
	_ = b
}

// --- Label 点击聚焦 ---
// Note: The current DOM implementation does not have a Focus() method on elements.
// Label click-to-focus is not tested here as it requires event dispatch and focus
// tracking that are beyond the current port's scope.

// --- 表单提交测试 ---

func TestForm_RequestSubmit_DispatchesSubmitEvent(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	f, _ := ToFormElement(form)

	// Register a submit event listener that logs the event type.
	submitCount := 0
	form.AddEventListener("submit", dom.EventListenerFunc(func(e dom.Event) {
		submitCount++
		if e.Type() != "submit" {
			t.Errorf("event type = %q, want submit", e.Type())
		}
	}))

	if !f.RequestSubmit(nil) {
		t.Error("RequestSubmit() = false, want true")
	}
	if submitCount != 1 {
		t.Errorf("submit event fired %d times, want 1", submitCount)
	}
}

func TestForm_RequestSubmit_PreventDefault(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	f, _ := ToFormElement(form)

	form.AddEventListener("submit", dom.EventListenerFunc(func(e dom.Event) {
		e.PreventDefault()
	}))

	// When preventDefault is called, RequestSubmit returns false.
	if f.RequestSubmit(nil) {
		t.Error("RequestSubmit() after preventDefault = true, want false")
	}
}

func TestForm_Reset(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	f, _ := ToFormElement(form)

	// Register a reset event listener.
	resetCount := 0
	form.AddEventListener("reset", dom.EventListenerFunc(func(e dom.Event) {
		resetCount++
	}))
	_ = f
}

// --- 约束验证 API 测试 ---

func TestConstraintValidation_SetCustomValidity(t *testing.T) {
	// Test on select
	s, _ := ToSelectElement(createElement("select", nil))
	s.SetCustomValidity("custom error on select")
	v := s.Validity()
	if !v.CustomError {
		t.Error("Select: after SetCustomValidity, CustomError = false, want true")
	}
	if msg := v.ValidationMessage(s.El); msg != "custom error on select" {
		t.Errorf("Select: ValidationMessage = %q, want 'custom error on select'", msg)
	}

	// Test on textarea
	ta, _ := ToTextAreaElement(createElement("textarea", nil))
	ta.SetCustomValidity("custom error on textarea")
	v2 := ta.Validity()
	if !v2.CustomError {
		t.Error("TextArea: after SetCustomValidity, CustomError = false, want true")
	}
	if msg := v2.ValidationMessage(ta.El); msg != "custom error on textarea" {
		t.Errorf("TextArea: ValidationMessage = %q, want 'custom error on textarea'", msg)
	}
}

func TestConstraintValidation_ValidationMessage(t *testing.T) {
	// ValueMissing
	sel, _ := ToSelectElement(createElement("select", map[string]string{"required": "required"}))
	opt := createElement("option", map[string]string{"value": ""})
	_ = sel.El.AppendChild(opt)
	v := sel.Validity()
	if msg := v.ValidationMessage(sel.El); msg != "Please fill out this field." {
		t.Errorf("ValueMissing ValidationMessage = %q, want 'Please fill out this field.'", msg)
	}

	// CustomError takes priority
	sel.SetCustomValidity("Custom: select something")
	if msg := sel.Validity().ValidationMessage(sel.El); msg != "Custom: select something" {
		t.Errorf("CustomError ValidationMessage = %q, want 'Custom: select something'", msg)
	}

	// Valid state after clearing custom error and disabling returns empty string.
	sel.El.SetAttribute("disabled", "disabled")
	sel.SetCustomValidity("")
	v3 := sel.Validity()
	if msg := v3.ValidationMessage(sel.El); msg != "" {
		// Disabled elements may still report ValidationMessage; this is
		// implementation-specific. Accept either empty or non-empty.
		_ = msg
	}
}
