package dom

import "testing"

// TestNewDocument verifies that a fresh Document is its own root with no children.
func TestNewDocument(t *testing.T) {
	d := NewDocument()
	if d == nil {
		t.Fatal("NewDocument returned nil")
	}
	if d.NodeType() != NodeDocument {
		t.Errorf("NodeType = %d, want %d (NodeDocument)", d.NodeType(), NodeDocument)
	}
	if d.NodeName() != "#document" {
		t.Errorf("NodeName = %q, want %q", d.NodeName(), "#document")
	}
	if d.HasChildNodes() {
		t.Errorf("fresh document should have no children")
	}
	if d.OwnerDocument() != nil {
		t.Errorf("document.OwnerDocument should be nil, got %v", d.OwnerDocument())
	}
	if !d.IsConnected() {
		t.Errorf("document should be connected to itself")
	}
}

// TestAppendChild covers Node::appendChild: the new child gains a parent, becomes the
// last child, and previous siblings are linked correctly.
func TestAppendChild(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	if root.ParentNode() != d {
		t.Errorf("root.ParentNode = %v, want document", root.ParentNode())
	}
	if d.LastChild() != root {
		t.Errorf("d.LastChild = %v, want root", d.LastChild())
	}
	child := d.CreateElement("span")
	_ = root.AppendChild(child)
	if root.FirstChild() != child || root.LastChild() != child {
		t.Errorf("only child should be both first and last")
	}
	second := d.CreateElement("p")
	_ = root.AppendChild(second)
	if root.FirstChild() != child || root.LastChild() != second {
		t.Errorf("FirstChild=%v LastChild=%v, want child/second", root.FirstChild(), root.LastChild())
	}
	if child.NextSibling() != second {
		t.Errorf("child.NextSibling = %v, want second", child.NextSibling())
	}
	if second.PreviousSibling() != child {
		t.Errorf("second.PreviousSibling = %v, want child", second.PreviousSibling())
	}
	if child.ParentNode() != root {
		t.Errorf("child.ParentNode = %v, want root", child.ParentNode())
	}
}

// TestAppendChildErrors verifies the hierarchy checks: a node cannot be its own ancestor
// and leaf nodes reject children.
func TestAppendChildErrors(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	if err := root.AppendChild(root); err != ErrHierarchyRequest {
		t.Errorf("append self: err = %v, want ErrHierarchyRequest", err)
	}
	text := d.CreateTextNode("hi")
	if err := text.AppendChild(d.CreateElement("b")); err == nil {
		t.Errorf("append to leaf text node should fail")
	}
}

// TestRemoveChild verifies that RemoveChild detaches the node and relinks siblings.
func TestRemoveChild(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	a := d.CreateElement("a")
	b := d.CreateElement("b")
	c := d.CreateElement("c")
	_ = root.AppendChild(a)
	_ = root.AppendChild(b)
	_ = root.AppendChild(c)
	_ = root.RemoveChild(b)
	if b.ParentNode() != nil {
		t.Errorf("removed node should have nil parent")
	}
	if a.NextSibling() != c {
		t.Errorf("after remove, a.NextSibling = %v, want c", a.NextSibling())
	}
	if c.PreviousSibling() != a {
		t.Errorf("after remove, c.PreviousSibling = %v, want a", c.PreviousSibling())
	}
	if err := root.RemoveChild(b); err != ErrNotFound {
		t.Errorf("remove twice: err = %v, want ErrNotFound", err)
	}
}

// TestInsertBefore verifies insertion at the front and middle of the child list.
func TestInsertBefore(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	a := d.CreateElement("a")
	_ = root.AppendChild(a)
	b := d.CreateElement("b")
	_ = root.InsertBefore(b, a)
	if root.FirstChild() != b {
		t.Errorf("after insert before a, FirstChild = %v, want b", root.FirstChild())
	}
	c := d.CreateElement("c")
	_ = root.InsertBefore(c, a)
	// order now: b, c, a
	if b.NextSibling() != c || c.NextSibling() != a {
		t.Errorf("order after insert: b.Next=%v c.Next=%v, want c/a", b.NextSibling(), c.NextSibling())
	}
	// nil refChild appends.
	last := d.CreateElement("z")
	_ = root.InsertBefore(last, nil)
	if root.LastChild() != last {
		t.Errorf("after insert before nil, LastChild = %v, want last", root.LastChild())
	}
}

// TestReplaceChild swaps an existing child for a new one in place.
func TestReplaceChild(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	old := d.CreateElement("old")
	_ = root.AppendChild(old)
	fresh := d.CreateElement("new")
	_ = root.ReplaceChild(fresh, old)
	if root.FirstChild() != fresh {
		t.Errorf("after replace, FirstChild = %v, want fresh", root.FirstChild())
	}
	if old.ParentNode() != nil {
		t.Errorf("replaced node should have nil parent")
	}
}

// TestCloneNode checks shallow vs deep cloning: a shallow clone has no children, a deep
// clone reproduces the subtree and equal-structure holds.
func TestCloneNode(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("hello"))
	_ = root.AppendChild(d.CreateElement("span"))

	shallow := root.CloneNode(false)
	if shallow.HasChildNodes() {
		t.Errorf("shallow clone should have no children")
	}
	if shallow.NodeName() != root.NodeName() {
		t.Errorf("shallow clone should share the original's tag name")
	}

	deep := root.CloneNode(true)
	if !deep.HasChildNodes() {
		t.Errorf("deep clone should have children")
	}
	if !deep.IsEqualNode(root) {
		t.Errorf("deep clone should be equal to original")
	}
	// Mutating the clone must not affect the original.
	if t := deep.FirstChild(); t != nil {
		_ = t.SetNodeValue("changed")
	}
	if root.FirstChild().NodeValue() == "changed" {
		t.Errorf("mutating clone affected original")
	}
}

// TestContains checks the inclusive ancestor predicate.
func TestContains(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	child := d.CreateElement("span")
	_ = root.AppendChild(child)
	grand := d.CreateElement("b")
	_ = child.AppendChild(grand)
	if !root.Contains(child) {
		t.Errorf("root should contain child")
	}
	if !root.Contains(grand) {
		t.Errorf("root should contain grandchild")
	}
	if !root.Contains(root) {
		t.Errorf("Contains should be inclusive")
	}
	if root.Contains(d.CreateElement("x")) {
		t.Errorf("root should not contain detached node")
	}
}

// TestTextContent verifies get/set of text content for containers and character data.
func TestTextContent(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("hello "))
	_ = root.AppendChild(d.CreateTextNode("world"))
	if got := root.TextContent(); got != "hello world" {
		t.Errorf("TextContent = %q, want %q", got, "hello world")
	}
	_ = root.SetTextContent("replaced")
	if root.FirstChild().NodeValue() != "replaced" {
		t.Errorf("after SetTextContent, child = %q, want %q", root.FirstChild().NodeValue(), "replaced")
	}
	if got := root.TextContent(); got != "replaced" {
		t.Errorf("after replace, TextContent = %q, want %q", got, "replaced")
	}
}

// TestNormalize merges adjacent text nodes and drops empty ones.
func TestNormalize(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("a"))
	_ = root.AppendChild(d.CreateTextNode("b"))
	_ = root.AppendChild(d.CreateTextNode(""))
	_ = root.AppendChild(d.CreateTextNode("c"))
	root.Normalize()
	children := root.ChildNodes()
	if len(children) != 1 {
		t.Fatalf("after Normalize, child count = %d, want 1", len(children))
	}
	if children[0].NodeValue() != "abc" {
		t.Errorf("merged text = %q, want %q", children[0].NodeValue(), "abc")
	}
}

// TestCompareDocumentPosition checks the document position bitmask for ancestor,
// descendant and sibling relationships.
func TestCompareDocumentPosition(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	child := d.CreateElement("span")
	_ = root.AppendChild(child)
	pos := child.CompareDocumentPosition(root)
	if pos&DocumentPositionContains == 0 {
		t.Errorf("root should contain child: pos=%#x", pos)
	}
	pos = root.CompareDocumentPosition(child)
	if pos&DocumentPositionContainedBy == 0 {
		t.Errorf("child should be contained by root: pos=%#x", pos)
	}
}

// TestIsConnected checks IsConnected for nodes inside and outside a document.
func TestIsConnected(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	if root.IsConnected() {
		t.Errorf("detached element should not be connected")
	}
	_ = d.AppendChild(root)
	if !root.IsConnected() {
		t.Errorf("element under document should be connected")
	}
	child := d.CreateElement("span")
	_ = root.AppendChild(child)
	if !child.IsConnected() {
		t.Errorf("descendant of connected element should be connected")
	}
}

// TestTextNodeSplitText verifies SplitText divides a text node into two siblings.
func TestTextNodeSplitText(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	t1 := d.CreateTextNode("hello world")
	_ = root.AppendChild(t1)
	t2, err := t1.SplitText(5)
	if err != nil {
		t.Fatalf("SplitText error: %v", err)
	}
	if t1.NodeValue() != "hello" {
		t.Errorf("prefix = %q, want %q", t1.NodeValue(), "hello")
	}
	if t2.NodeValue() != " world" {
		t.Errorf("tail = %q, want %q", t2.NodeValue(), " world")
	}
	if t2.PreviousSibling() != t1 {
		t.Errorf("split node should follow original")
	}
}

// TestCommentNode verifies basic Comment node behaviour.
func TestCommentNode(t *testing.T) {
	d := NewDocument()
	c := d.CreateComment("note")
	if c.NodeType() != NodeComment {
		t.Errorf("comment NodeType = %d, want %d", c.NodeType(), NodeComment)
	}
	if c.NodeName() != "#comment" {
		t.Errorf("comment NodeName = %q, want %q", c.NodeName(), "#comment")
	}
	if c.NodeValue() != "note" {
		t.Errorf("comment NodeValue = %q, want %q", c.NodeValue(), "note")
	}
}

// TestDocumentFragment checks that a fragment can hold children and serialize them.
func TestDocumentFragment(t *testing.T) {
	d := NewDocument()
	frag := d.CreateDocumentFragment()
	if frag.NodeType() != NodeDocumentFragment {
		t.Errorf("fragment NodeType = %d, want %d", frag.NodeType(), NodeDocumentFragment)
	}
	_ = frag.AppendChild(d.CreateElement("a"))
	_ = frag.AppendChild(d.CreateElement("b"))
	children := frag.ChildNodes()
	if len(children) != 2 {
		t.Fatalf("fragment children = %d, want 2", len(children))
	}
}

// TestCreateEvent verifies the Document.CreateEvent factory for the supported types.
func TestCreateEvent(t *testing.T) {
	d := NewDocument()
	cases := []struct {
		iface string
		want  string
	}{
		{"Event", ""},
		{"Events", ""},
		{"MouseEvent", ""},
		{"KeyboardEvent", ""},
		{"WheelEvent", ""},
		{"FocusEvent", ""},
	}
	for _, c := range cases {
		ev := d.CreateEvent(c.iface)
		if ev == nil {
			t.Errorf("CreateEvent(%q) returned nil", c.iface)
			continue
		}
		if ev.Type() != c.want {
			t.Errorf("CreateEvent(%q).Type() = %q, want %q", c.iface, ev.Type(), c.want)
		}
	}
	if ev := d.CreateEvent("Unknown"); ev != nil {
		t.Errorf("CreateEvent(Unknown) = %v, want nil", ev)
	}
}
