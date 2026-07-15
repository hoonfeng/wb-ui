package dom

import "testing"

// buildRangeTree builds a document with the structure:
//
//	<div id="root">
//	  <p id="p1">Hello</p>
//	  <span id="s1">World</span>
//	  <p id="p2">Tail</p>
//	</div>
//
// and returns the document, root div and the three child elements.
func buildRangeTree(t *testing.T) (*Document, *Element, *Element, *Element, *Element) {
	t.Helper()
	d := NewDocument()
	root := d.CreateElement("div")
	root.SetId("root")
	_ = d.AppendChild(root)
	p1 := d.CreateElement("p")
	p1.SetId("p1")
	_ = p1.AppendChild(d.CreateTextNode("Hello"))
	_ = root.AppendChild(p1)
	s1 := d.CreateElement("span")
	s1.SetId("s1")
	_ = s1.AppendChild(d.CreateTextNode("World"))
	_ = root.AppendChild(s1)
	p2 := d.CreateElement("p")
	p2.SetId("p2")
	_ = p2.AppendChild(d.CreateTextNode("Tail"))
	_ = root.AppendChild(p2)
	return d, root, p1, s1, p2
}

// TestNewRange verifies the default range is collapsed at the document root.
func TestNewRange(t *testing.T) {
	d := NewDocument()
	r := NewRange(d)
	if !r.Collapsed() {
		t.Errorf("fresh range should be collapsed")
	}
	if r.StartContainer() != d || r.EndContainer() != d {
		t.Errorf("fresh range boundary containers should be the document")
	}
	if r.StartOffset() != 0 || r.EndOffset() != 0 {
		t.Errorf("fresh range offsets should be 0")
	}
}

// TestSetStartSetEnd covers setting boundary points and the auto-collapse behaviour.
func TestSetStartSetEnd(t *testing.T) {
	d, root, p1, s1, _ := buildRangeTree(t)
	r := NewRange(d)
	if err := r.SetStart(root, 1); err != nil {
		t.Fatalf("SetStart error: %v", err)
	}
	if err := r.SetEnd(root, 3); err != nil {
		t.Fatalf("SetEnd error: %v", err)
	}
	if r.StartContainer() != root || r.StartOffset() != 1 {
		t.Errorf("start = (%v, %d), want (root, 1)", r.StartContainer(), r.StartOffset())
	}
	if r.EndContainer() != root || r.EndOffset() != 3 {
		t.Errorf("end = (%v, %d), want (root, 3)", r.EndContainer(), r.EndOffset())
	}
	if r.Collapsed() {
		t.Errorf("range should not be collapsed")
	}
	// Common ancestor of (root, 1) and (root, 3) is root.
	if r.CommonAncestorContainer() != root {
		t.Errorf("common ancestor = %v, want root", r.CommonAncestorContainer())
	}
	// SetStart within the existing range should not collapse the end: (s1, 0) lies
	// before (root, 3) in document order so the range stays well-formed.
	if err := r.SetStart(s1, 0); err != nil {
		t.Fatalf("SetStart within range: %v", err)
	}
	if r.EndContainer() != root || r.EndOffset() != 3 {
		t.Errorf("after setStart within range, end = (%v, %d), want (root, 3)", r.EndContainer(), r.EndOffset())
	}
	// SetEnd before the start should collapse the start to the new end: (p1, 0) precedes
	// (s1, 0) so the start is pulled back to (p1, 0).
	if err := r.SetEnd(p1, 0); err != nil {
		t.Fatalf("SetEnd before start: %v", err)
	}
	if r.StartContainer() != p1 || r.StartOffset() != 0 {
		t.Errorf("after setEnd before start, start = (%v, %d), want (p1, 0)", r.StartContainer(), r.StartOffset())
	}
}

// TestSetStartInvalidOffset verifies that out-of-range offsets return IndexSizeError.
func TestSetStartInvalidOffset(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	// root has 3 children, so offset 4 is invalid.
	if err := r.SetStart(root, 4); err != ErrIndexSize {
		t.Errorf("SetStart(root, 4) err = %v, want ErrIndexSize", err)
	}
	// negative offset is invalid.
	if err := r.SetStart(root, -1); err != ErrIndexSize {
		t.Errorf("SetStart(root, -1) err = %v, want ErrIndexSize", err)
	}
}

// TestCollapse covers Range.Collapse in both directions.
func TestCollapse(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 1)
	_ = r.SetEnd(root, 3)
	r.Collapse(true)
	if r.EndOffset() != r.StartOffset() {
		t.Errorf("after collapse to start, end offset = %d, want %d", r.EndOffset(), r.StartOffset())
	}
	_ = r.SetStart(root, 1)
	_ = r.SetEnd(root, 3)
	r.Collapse(false)
	if r.StartOffset() != r.EndOffset() {
		t.Errorf("after collapse to end, start offset = %d, want %d", r.StartOffset(), r.EndOffset())
	}
}

// TestSelectNode covers Range.SelectNode.
func TestSelectNode(t *testing.T) {
	d, root, p1, _, _ := buildRangeTree(t)
	r := NewRange(d)
	if err := r.SelectNode(p1); err != nil {
		t.Fatalf("SelectNode error: %v", err)
	}
	if r.StartContainer() != root || r.StartOffset() != 0 {
		t.Errorf("after SelectNode start = (%v, %d), want (root, 0)", r.StartContainer(), r.StartOffset())
	}
	if r.EndOffset() != 1 {
		t.Errorf("after SelectNode end offset = %d, want 1", r.EndOffset())
	}
}

// TestSelectNodeContents covers Range.SelectNodeContents.
func TestSelectNodeContents(t *testing.T) {
	d, _, p1, _, _ := buildRangeTree(t)
	r := NewRange(d)
	if err := r.SelectNodeContents(p1); err != nil {
		t.Fatalf("SelectNodeContents error: %v", err)
	}
	if r.StartContainer() != p1 || r.EndContainer() != p1 {
		t.Errorf("after SelectNodeContents, containers should be p1")
	}
	if r.StartOffset() != 0 || r.EndOffset() != 1 {
		t.Errorf("offsets = (%d, %d), want (0, 1)", r.StartOffset(), r.EndOffset())
	}
}

// TestCompareBoundaryPoints covers Range.CompareBoundaryPoints for all four how values.
func TestCompareBoundaryPoints(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r1 := NewRange(d)
	_ = r1.SetStart(root, 1)
	_ = r1.SetEnd(root, 2)
	r2 := NewRange(d)
	_ = r2.SetStart(root, 0)
	_ = r2.SetEnd(root, 3)
	cases := []struct {
		how  CompareHow
		want int
	}{
		{StartToStart, 1},  // r1.start(1) > r2.start(0)
		{StartToEnd, -1},   // r1.start(1) < r2.end(3)
		{EndToEnd, -1},     // r1.end(2) < r2.end(3)
		{EndToStart, 1},    // r1.end(2) > r2.start(0)
	}
	for _, c := range cases {
		got, err := r1.CompareBoundaryPoints(c.how, r2)
		if err != nil {
			t.Fatalf("CompareBoundaryPoints(%d) error: %v", c.how, err)
		}
		if got != c.want {
			t.Errorf("CompareBoundaryPoints(%d) = %d, want %d", c.how, got, c.want)
		}
	}
}

// TestCloneRange verifies that CloneRange returns an independent copy.
func TestCloneRange(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 1)
	_ = r.SetEnd(root, 3)
	cp := r.CloneRange()
	_ = cp.SetStart(root, 0)
	if r.StartOffset() != 1 {
		t.Errorf("mutating clone affected original: start = %d, want 1", r.StartOffset())
	}
}

// TestDeleteContents covers Range.DeleteContents on a container range.
func TestDeleteContents(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 1) // before span
	_ = r.SetEnd(root, 3)   // after p2
	if err := r.DeleteContents(); err != nil {
		t.Fatalf("DeleteContents error: %v", err)
	}
	children := root.ChildNodes()
	if len(children) != 1 {
		t.Fatalf("after delete, root has %d children, want 1", len(children))
	}
	if localName(children[0]) != "p" {
		t.Errorf("remaining child = %q, want %q", localName(children[0]), "p")
	}
}

// TestExtractContents covers Range.ExtractContents: the contents are removed from the
// document and returned in a fragment.
func TestExtractContents(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 0)
	_ = r.SetEnd(root, 2)
	frag, err := r.ExtractContents()
	if err != nil {
		t.Fatalf("ExtractContents error: %v", err)
	}
	children := frag.ChildNodes()
	if len(children) != 2 {
		t.Fatalf("fragment has %d children, want 2", len(children))
	}
	// Original should now have only one child (p2).
	rootChildren := root.ChildNodes()
	if len(rootChildren) != 1 {
		t.Errorf("after extract, root has %d children, want 1", len(rootChildren))
	}
	if localName(rootChildren[0]) != "p" {
		t.Errorf("remaining child = %q, want %q", localName(rootChildren[0]), "p")
	}
}

// TestCloneContents covers Range.CloneContents: the document is unchanged.
func TestCloneContents(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 0)
	_ = r.SetEnd(root, 2)
	frag, err := r.CloneContents()
	if err != nil {
		t.Fatalf("CloneContents error: %v", err)
	}
	children := frag.ChildNodes()
	if len(children) != 2 {
		t.Fatalf("fragment has %d children, want 2", len(children))
	}
	if root.ChildNodes()[0] == children[0] {
		t.Errorf("cloned child should be a copy, not the original")
	}
}

// TestRangeToString covers Range.ToString: it concatenates text inside the range.
func TestRangeToString(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 0)
	_ = r.SetEnd(root, 3)
	got := r.ToString()
	want := "HelloWorldTail"
	if got != want {
		t.Errorf("ToString = %q, want %q", got, want)
	}
}

// TestRangeToStringPartialText covers a range that starts mid-text and ends mid-text.
func TestRangeToStringPartialText(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("hello world"))
	r := NewRange(d)
	text := root.FirstChild()
	_ = r.SetStart(text, 2)  // "llo ..."
	_ = r.SetEnd(text, 7)    // "...world"[2..7] => "llo w"
	got := r.ToString()
	want := "llo w"
	if got != want {
		t.Errorf("ToString = %q, want %q", got, want)
	}
}

// TestIsPointInRange covers Range.IsPointInRange.
func TestIsPointInRange(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 1)
	_ = r.SetEnd(root, 3)
	cases := []struct {
		node Node
		off  int
		want bool
	}{
		{root, 0, false}, // before
		{root, 1, true},  // at start
		{root, 2, true},  // in middle
		{root, 3, true},  // at end
		{root, 4, false}, // out of bounds (also returns false via error)
	}
	for _, c := range cases {
		got, _ := r.IsPointInRange(c.node, c.off)
		if got != c.want {
			t.Errorf("IsPointInRange(root, %d) = %v, want %v", c.off, got, c.want)
		}
	}
}

// TestIntersectsNode covers Range.IntersectsNode.
func TestIntersectsNode(t *testing.T) {
	d, root, _, s1, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 0)
	_ = r.SetEnd(root, 1) // covers only p1
	if r.IntersectsNode(s1) {
		t.Errorf("range covering p1 should not intersect s1")
	}
	_ = r.SetEnd(root, 2) // now covers p1 and s1
	if !r.IntersectsNode(s1) {
		t.Errorf("range covering p1+s1 should intersect s1")
	}
}

// TestInsertNode covers Range.InsertNode into a container.
func TestInsertNode(t *testing.T) {
	d, root, _, _, _ := buildRangeTree(t)
	r := NewRange(d)
	_ = r.SetStart(root, 1) // before s1
	ins := d.CreateElement("ins")
	if err := r.InsertNode(ins); err != nil {
		t.Fatalf("InsertNode error: %v", err)
	}
	children := root.ChildNodes()
	if len(children) != 4 {
		t.Fatalf("after insert, root has %d children, want 4", len(children))
	}
	if localName(children[1]) != "ins" {
		t.Errorf("inserted child at index 1 = %q, want %q", localName(children[1]), "ins")
	}
}

// TestSetStartBeforeAfter covers the SetStartBefore/SetStartAfter/SetEndBefore/SetEndAfter
// helpers.
func TestSetStartBeforeAfter(t *testing.T) {
	d, root, _, s1, _ := buildRangeTree(t)
	r := NewRange(d)
	if err := r.SetStartBefore(s1); err != nil {
		t.Fatalf("SetStartBefore error: %v", err)
	}
	if r.StartOffset() != 1 {
		t.Errorf("after SetStartBefore(s1), offset = %d, want 1", r.StartOffset())
	}
	if err := r.SetEndAfter(s1); err != nil {
		t.Fatalf("SetEndAfter error: %v", err)
	}
	if r.EndOffset() != 2 {
		t.Errorf("after SetEndAfter(s1), offset = %d, want 2", r.EndOffset())
	}
	if r.StartContainer() != root || r.EndContainer() != root {
		t.Errorf("containers should be root")
	}
}

// TestRangeExtractTextOnly covers extract on a single text node.
func TestRangeExtractTextOnly(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("hello"))
	r := NewRange(d)
	text := root.FirstChild()
	_ = r.SetStart(text, 1)
	_ = r.SetEnd(text, 4)
	frag, err := r.ExtractContents()
	if err != nil {
		t.Fatalf("ExtractContents error: %v", err)
	}
	got := frag.FirstChild().NodeValue()
	want := "ell"
	if got != want {
		t.Errorf("extracted text = %q, want %q", got, want)
	}
	if text.NodeValue() != "ho" {
		t.Errorf("remaining text = %q, want %q", text.NodeValue(), "ho")
	}
}

// TestRangeDeleteTextOnly covers delete on a single text node.
func TestRangeDeleteTextOnly(t *testing.T) {
	d := NewDocument()
	root := d.CreateElement("div")
	_ = d.AppendChild(root)
	_ = root.AppendChild(d.CreateTextNode("hello world"))
	r := NewRange(d)
	text := root.FirstChild()
	_ = r.SetStart(text, 6)
	_ = r.SetEnd(text, 11)
	if err := r.DeleteContents(); err != nil {
		t.Fatalf("DeleteContents error: %v", err)
	}
	if text.NodeValue() != "hello " {
		t.Errorf("after delete, text = %q, want %q", text.NodeValue(), "hello ")
	}
}
