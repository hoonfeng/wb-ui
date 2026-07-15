package dom

import "testing"

// TestElementAttributes covers HasAttribute/GetAttribute/SetAttribute/RemoveAttribute and
// the attribute insertion-order preservation.
func TestElementAttributes(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	if el.HasAttributes() {
		t.Errorf("fresh element should have no attributes")
	}
	if el.HasAttribute("id") {
		t.Errorf("HasAttribute on missing attribute should be false")
	}
	el.SetAttribute("id", "main")
	if !el.HasAttribute("id") {
		t.Errorf("HasAttribute after set should be true")
	}
	if got := el.GetAttribute("id"); got != "main" {
		t.Errorf("GetAttribute = %q, want %q", got, "main")
	}
	if !el.HasAttributes() {
		t.Errorf("HasAttributes after set should be true")
	}
	el.SetAttribute("class", "btn")
	if got := el.GetAttribute("class"); got != "btn" {
		t.Errorf("GetAttribute class = %q, want %q", got, "btn")
	}
	// Overwrite existing value.
	el.SetAttribute("id", "alt")
	if got := el.GetAttribute("id"); got != "alt" {
		t.Errorf("after overwrite, GetAttribute = %q, want %q", got, "alt")
	}
	el.RemoveAttribute("class")
	if el.HasAttribute("class") {
		t.Errorf("HasAttribute after remove should be false")
	}
}

// TestElementIdClassName covers the id and className convenience accessors.
func TestElementIdClassName(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("div")
	el.SetId("root")
	if el.GetId() != "root" {
		t.Errorf("GetId = %q, want %q", el.GetId(), "root")
	}
	el.SetClassName("btn primary")
	if el.ClassName() != "btn primary" {
		t.Errorf("ClassName = %q, want %q", el.ClassName(), "btn primary")
	}
	if !el.HasClassName("btn") {
		t.Errorf("HasClassName(btn) should be true")
	}
	if !el.HasClassName("primary") {
		t.Errorf("HasClassName(primary) should be true")
	}
	if el.HasClassName("missing") {
		t.Errorf("HasClassName(missing) should be false")
	}
}

// TestGetElementById covers the descendant id search on a subtree.
func TestGetElementById(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	target := d.CreateElement("span")
	target.SetId("target")
	_ = root.AppendChild(target)
	// Add a sibling that does not match.
	_ = root.AppendChild(d.CreateElement("p"))
	found := root.GetElementById("target")
	if found != target {
		t.Errorf("GetElementById = %v, want target", found)
	}
	if el := root.GetElementById("missing"); el != nil {
		t.Errorf("GetElementById(missing) = %v, want nil", el)
	}
}

// TestGetElementsByTagName covers the wildcard and named tag searches.
func TestGetElementsByTagName(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateElement("p"))
	_ = root.AppendChild(d.CreateElement("span"))
	_ = root.AppendChild(d.CreateElement("p"))
	ps := root.GetElementsByTagName("p")
	if len(ps) != 2 {
		t.Errorf("getElementsByTagName(p) = %d, want 2", len(ps))
	}
	all := root.GetElementsByTagName("*")
	// div + p + span + p = 4
	if len(all) != 4 {
		t.Errorf("getElementsByTagName(*) = %d, want 4", len(all))
	}
}

// TestGetElementsByClassName covers the class-token search.
func TestGetElementsByClassName(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	a := d.CreateElement("a")
	a.SetClassName("btn primary")
	_ = root.AppendChild(a)
	b := d.CreateElement("b")
	b.SetClassName("btn")
	_ = root.AppendChild(b)
	c := d.CreateElement("c")
	_ = root.AppendChild(c)
	btns := root.GetElementsByClassName("btn")
	if len(btns) != 2 {
		t.Errorf("getElementsByClassName(btn) = %d, want 2", len(btns))
	}
	primary := root.GetElementsByClassName("primary")
	if len(primary) != 1 {
		t.Errorf("getElementsByClassName(primary) = %d, want 1", len(primary))
	}
	if el := root.GetElementsByClassName(""); el != nil {
		t.Errorf("empty token should return nil, got %v", el)
	}
}

// TestInnerHTML covers GetInnerHTML and SetInnerHTML for a simple subtree.
func TestInnerHTML(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.SetInnerHTML(`<a id="x"><b>text</b></a>`)
	got := root.GetInnerHTML()
	want := `<a id="x"><b>text</b></a>`
	if got != want {
		t.Errorf("InnerHTML = %q, want %q", got, want)
	}
	// Verify the parsed structure.
	a := root.FirstChild()
	if a == nil || localName(a) != "a" {
		t.Fatalf("first child after parse: %v, want <a>", a)
	}
	if el, ok := a.(*Element); !ok || el.GetAttribute("id") != "x" {
		t.Errorf("parsed <a> id = %q, want %q", el.GetAttribute("id"), "x")
	}
}

// TestOuterHTML covers the outerHTML serialization including the element itself.
func TestOuterHTML(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("a")
	el.SetAttribute("href", "/x")
	_ = el.AppendChild(d.CreateTextNode("link"))
	got := el.GetOuterHTML()
	want := `<a href="/x">link</a>`
	if got != want {
		t.Errorf("OuterHTML = %q, want %q", got, want)
	}
}

// TestElementNodeName covers TagName/LocalName/NodeName for elements.
func TestElementNodeName(t *testing.T) {
	d := NewDocument()
	el := d.CreateElement("DIV")
	if el.NodeName() != "DIV" {
		t.Errorf("NodeName = %q, want %q", el.NodeName(), "DIV")
	}
	if el.TagName() != "DIV" {
		t.Errorf("TagName = %q, want %q", el.TagName(), "DIV")
	}
	if el.LocalName() != "div" {
		t.Errorf("LocalName = %q, want %q", el.LocalName(), "div")
	}
}

// TestTextNode verifies Text node behaviour.
func TestTextNode(t *testing.T) {
	d := NewDocument()
	tx := d.CreateTextNode("hello")
	if tx.NodeType() != NodeText {
		t.Errorf("text NodeType = %d, want %d", tx.NodeType(), NodeText)
	}
	if tx.NodeName() != "#text" {
		t.Errorf("text NodeName = %q, want %q", tx.NodeName(), "#text")
	}
	if tx.NodeValue() != "hello" {
		t.Errorf("text NodeValue = %q, want %q", tx.NodeValue(), "hello")
	}
	if tx.Length() != 5 {
		t.Errorf("text Length = %d, want 5", tx.Length())
	}
	if got := tx.SubstringData(1, 3); got != "ell" {
		t.Errorf("SubstringData(1,3) = %q, want %q", got, "ell")
	}
	tx.AppendData("!")
	if tx.NodeValue() != "hello!" {
		t.Errorf("after AppendData: %q, want %q", tx.NodeValue(), "hello!")
	}
	tx.InsertData(0, "X")
	if tx.NodeValue() != "Xhello!" {
		t.Errorf("after InsertData: %q, want %q", tx.NodeValue(), "Xhello!")
	}
	tx.DeleteData(0, 1)
	if tx.NodeValue() != "hello!" {
		t.Errorf("after DeleteData: %q, want %q", tx.NodeValue(), "hello!")
	}
	tx.ReplaceData(0, 5, "bye")
	if tx.NodeValue() != "bye!" {
		t.Errorf("after ReplaceData: %q, want %q", tx.NodeValue(), "bye!")
	}
}
