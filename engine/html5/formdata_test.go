package html5

import (
	"testing"

	"wb-ui/engine/dom"
)

// --- DOMFormData basics ---

func TestFormData_AppendGetHas(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("a", "1")
	fd.Append("b", "2")
	if got := fd.Get("a"); got != "1" {
		t.Errorf("Get(a) = %q, want 1", got)
	}
	if got := fd.Get("b"); got != "2" {
		t.Errorf("Get(b) = %q, want 2", got)
	}
	if !fd.Has("a") {
		t.Error("Has(a) = false, want true")
	}
	if fd.Has("missing") {
		t.Error("Has(missing) = true, want false")
	}
	if got := fd.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
}

func TestFormData_GetMissingReturnsEmpty(t *testing.T) {
	fd := NewDOMFormData()
	if got := fd.Get("x"); got != "" {
		t.Errorf("Get(missing) = %q, want empty", got)
	}
}

func TestFormData_AppendMultiple(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("k", "1")
	fd.Append("k", "2")
	fd.Append("k", "3")
	if got := fd.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
	all := fd.All("k")
	if len(all) != 3 || all[0] != "1" || all[1] != "2" || all[2] != "3" {
		t.Errorf("All(k) = %v, want [1 2 3]", all)
	}
	if got := fd.Get("k"); got != "1" {
		t.Errorf("Get(k) (first) = %q, want 1", got)
	}
}

func TestFormData_SetReplaces(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("k", "1")
	fd.Append("k", "2")
	fd.Append("other", "x")
	fd.Set("k", "new")
	if got := fd.Len(); got != 2 {
		t.Errorf("Len() after Set = %d, want 2 (one k + one other)", got)
	}
	if got := fd.Get("k"); got != "new" {
		t.Errorf("Get(k) after Set = %q, want new", got)
	}
	all := fd.All("k")
	if len(all) != 1 || all[0] != "new" {
		t.Errorf("All(k) after Set = %v, want [new]", all)
	}
}

func TestFormData_SetNew(t *testing.T) {
	fd := NewDOMFormData()
	fd.Set("k", "v")
	if got := fd.Get("k"); got != "v" {
		t.Errorf("Get(k) after Set(new) = %q, want v", got)
	}
}

func TestFormData_Delete(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("a", "1")
	fd.Append("b", "2")
	fd.Append("a", "3")
	fd.Delete("a")
	if fd.Has("a") {
		t.Error("Has(a) after Delete = true, want false")
	}
	if !fd.Has("b") {
		t.Error("Has(b) after Delete(a) = false, want true (should keep b)")
	}
	if got := fd.Len(); got != 1 {
		t.Errorf("Len() after Delete = %d, want 1", got)
	}
}

func TestFormData_EntriesKeysValues(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("a", "1")
	fd.Append("b", "2")
	fd.Append("a", "3")
	entries := fd.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries() len = %d, want 3", len(entries))
	}
	if entries[0].Name != "a" || entries[0].Value != "1" {
		t.Errorf("Entries()[0] = %+v, want {a 1}", entries[0])
	}
	keys := fd.Keys()
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "a" {
		t.Errorf("Keys() = %v, want [a b a]", keys)
	}
	values := fd.Values()
	if len(values) != 3 || values[0] != "1" || values[1] != "2" || values[2] != "3" {
		t.Errorf("Values() = %v, want [1 2 3]", values)
	}
}

// --- DOMFormData from form ---

func TestFormData_FromFormTextAndSelect(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("name", "user")
	in.SetAttribute("value", "alice")
	sel := doc.CreateElement("select")
	sel.SetAttribute("name", "color")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "red")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "blue")
	opt2.SetAttribute("selected", "selected")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)
	_ = form.AppendChild(in)
	_ = form.AppendChild(sel)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if got := fd.Get("user"); got != "alice" {
		t.Errorf("Get(user) = %q, want alice", got)
	}
	if got := fd.Get("color"); got != "blue" {
		t.Errorf("Get(color) = %q, want blue (selected)", got)
	}
}

func TestFormData_FromFormCheckboxRadioSkippedUnlessChecked(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	cb1 := doc.CreateElement("input")
	cb1.SetAttribute("type", "checkbox")
	cb1.SetAttribute("name", "agree")
	cb1.SetAttribute("value", "yes")
	cb2 := doc.CreateElement("input")
	cb2.SetAttribute("type", "checkbox")
	cb2.SetAttribute("name", "news")
	cb2.SetAttribute("value", "subscribe")
	cb2.SetAttribute("checked", "checked")
	radio := doc.CreateElement("input")
	radio.SetAttribute("type", "radio")
	radio.SetAttribute("name", "gender")
	radio.SetAttribute("value", "female")
	_ = form.AppendChild(cb1)
	_ = form.AppendChild(cb2)
	_ = form.AppendChild(radio)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if fd.Has("agree") {
		t.Error("unchecked checkbox agree should be skipped")
	}
	if !fd.Has("news") {
		t.Error("checked checkbox news should be included")
	}
	if got := fd.Get("news"); got != "subscribe" {
		t.Errorf("Get(news) = %q, want subscribe", got)
	}
	if fd.Has("gender") {
		t.Error("unchecked radio should be skipped")
	}
}

func TestFormData_FromFormCheckboxDefaultValueOn(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	cb := doc.CreateElement("input")
	cb.SetAttribute("type", "checkbox")
	cb.SetAttribute("name", "agree")
	cb.SetAttribute("checked", "checked")
	_ = form.AppendChild(cb)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if got := fd.Get("agree"); got != "on" {
		t.Errorf("Get(agree) with no value attr = %q, want on", got)
	}
}

func TestFormData_FromFormSkipsSubmitResetButton(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	submit := doc.CreateElement("input")
	submit.SetAttribute("type", "submit")
	submit.SetAttribute("name", "submitBtn")
	submit.SetAttribute("value", "Send")
	reset := doc.CreateElement("input")
	reset.SetAttribute("type", "reset")
	reset.SetAttribute("name", "resetBtn")
	text := doc.CreateElement("input")
	text.SetAttribute("type", "text")
	text.SetAttribute("name", "q")
	text.SetAttribute("value", "hello")
	_ = form.AppendChild(submit)
	_ = form.AppendChild(reset)
	_ = form.AppendChild(text)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if fd.Has("submitBtn") {
		t.Error("submit input should be skipped")
	}
	if fd.Has("resetBtn") {
		t.Error("reset input should be skipped")
	}
	if !fd.Has("q") {
		t.Error("text input should be included")
	}
}

func TestFormData_FromFormDisabledSkipped(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("name", "disabled")
	in.SetAttribute("value", "x")
	in.SetAttribute("disabled", "disabled")
	_ = form.AppendChild(in)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if fd.Has("disabled") {
		t.Error("disabled input should be skipped")
	}
}

func TestFormData_FromFormSkipsUnnamed(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("value", "no name")
	_ = form.AppendChild(in)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if fd.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (no name attr)", fd.Len())
	}
}

func TestFormData_FromFormTextArea(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	ta := doc.CreateElement("textarea")
	ta.SetAttribute("name", "comment")
	_ = ta.SetTextContent("Hello world")
	_ = form.AppendChild(ta)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	if got := fd.Get("comment"); got != "Hello world" {
		t.Errorf("Get(comment) = %q, want Hello world", got)
	}
}

func TestFormData_FromFormSelectMultiple(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	sel := doc.CreateElement("select")
	sel.SetAttribute("name", "tags")
	sel.SetAttribute("multiple", "multiple")
	opt1 := doc.CreateElement("option")
	opt1.SetAttribute("value", "a")
	opt1.SetAttribute("selected", "selected")
	opt2 := doc.CreateElement("option")
	opt2.SetAttribute("value", "b")
	opt3 := doc.CreateElement("option")
	opt3.SetAttribute("value", "c")
	opt3.SetAttribute("selected", "selected")
	_ = sel.AppendChild(opt1)
	_ = sel.AppendChild(opt2)
	_ = sel.AppendChild(opt3)
	_ = form.AppendChild(sel)

	f, _ := ToFormElement(form)
	fd := NewDOMFormDataFromForm(f)
	all := fd.All("tags")
	if len(all) != 2 || all[0] != "a" || all[1] != "c" {
		t.Errorf("All(tags) = %v, want [a c] (both selected)", all)
	}
}

// --- Encode ---

func TestFormData_Encode(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("a", "1")
	fd.Append("b", "hello")
	got := fd.Encode()
	want := "a=1&b=hello"
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestFormData_EncodeSpecialChars(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("q", "hello world")
	fd.Append("email", "a@b.com")
	got := fd.Encode()
	// space -> +, @ -> %40, . stays.
	want := "q=hello+world&email=a%40b.com"
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestFormData_EncodeNonASCII(t *testing.T) {
	fd := NewDOMFormData()
	fd.Append("name", "中文")
	got := fd.Encode()
	// 中 = E4 B8 AD, 文 = E6 96 87
	want := "name=%E4%B8%AD%E6%96%87"
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

func TestFormData_EncodeEmpty(t *testing.T) {
	fd := NewDOMFormData()
	if got := fd.Encode(); got != "" {
		t.Errorf("Encode() empty = %q, want empty", got)
	}
}

// --- FormSubmit / SerializeForm helpers ---

func TestFormSubmit(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("name", "q")
	in.SetAttribute("value", "test")
	_ = form.AppendChild(in)
	f, _ := ToFormElement(form)
	if got := FormSubmit(f); got != "q=test" {
		t.Errorf("FormSubmit() = %q, want q=test", got)
	}
}

func TestSerializeForm_RejectsNonForm(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	if got := SerializeForm(div); got != "" {
		t.Errorf("SerializeForm(div) = %q, want empty", got)
	}
}

// --- End-to-end: validation + submission ---

func TestForm_CompleteValidationAndSubmission(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	form.SetAttribute("method", "post")
	form.SetAttribute("action", "/login")

	// username (required text)
	user := doc.CreateElement("input")
	user.SetAttribute("type", "text")
	user.SetAttribute("name", "username")
	user.SetAttribute("required", "required")
	user.SetAttribute("value", "alice")

	// email (required email)
	email := doc.CreateElement("input")
	email.SetAttribute("type", "email")
	email.SetAttribute("name", "email")
	email.SetAttribute("required", "required")
	email.SetAttribute("value", "alice@example.com")

	// age (number with range)
	age := doc.CreateElement("input")
	age.SetAttribute("type", "number")
	age.SetAttribute("name", "age")
	age.SetAttribute("min", "18")
	age.SetAttribute("max", "120")
	age.SetAttribute("value", "30")

	// newsletter checkbox (unchecked)
	news := doc.CreateElement("input")
	news.SetAttribute("type", "checkbox")
	news.SetAttribute("name", "newsletter")
	news.SetAttribute("value", "yes")

	_ = form.AppendChild(user)
	_ = form.AppendChild(email)
	_ = form.AppendChild(age)
	_ = form.AppendChild(news)

	f, _ := ToFormElement(form)

	// Form should be valid.
	if !f.CheckValidity() {
		t.Error("CheckValidity() = false, want true for valid form")
	}

	// Submission should include username, email, age; skip unchecked checkbox.
	fd := NewDOMFormDataFromForm(f)
	if got := fd.Get("username"); got != "alice" {
		t.Errorf("Get(username) = %q, want alice", got)
	}
	if got := fd.Get("email"); got != "alice@example.com" {
		t.Errorf("Get(email) = %q", got)
	}
	if got := fd.Get("age"); got != "30" {
		t.Errorf("Get(age) = %q, want 30", got)
	}
	if fd.Has("newsletter") {
		t.Error("unchecked newsletter should not be in FormData")
	}

	// Make email invalid and re-check.
	email.SetAttribute("value", "not-an-email")
	if f.CheckValidity() {
		t.Error("CheckValidity() = true, want false for invalid email")
	}
}

func TestForm_InvalidEmailBlocksSubmission(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	email := doc.CreateElement("input")
	email.SetAttribute("type", "email")
	email.SetAttribute("name", "email")
	email.SetAttribute("required", "required")
	email.SetAttribute("value", "bad-email")
	_ = form.AppendChild(email)
	f, _ := ToFormElement(form)
	if f.CheckValidity() {
		t.Error("CheckValidity() = true for invalid required email, want false")
	}
}
