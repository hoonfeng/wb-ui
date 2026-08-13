package bindings

import (
	"testing"

	"wb-ui/dom"
)

func TestTemplateMoveDebug(t *testing.T) {
	doc := dom.NewDocument()
	tpl := doc.CreateElement("template")
	doc.AppendChild(tpl)
	tpl.SetInnerHTML("<svg><circle/><path/></svg>")
	t.Logf("after innerHTML: template children=%d", len(tpl.ChildNodes()))
	svg, _ := tpl.FirstChild().(*dom.Element)
	t.Logf("svg children=%d", len(svg.ChildNodes()))

	frag := doc.CreateDocumentFragment()
	for c := tpl.FirstChild(); c != nil; c = tpl.FirstChild() {
		if err := frag.AppendChild(c); err != nil {
			t.Fatalf("move to frag: %v", err)
		}
	}
	t.Logf("after content: tpl=%d frag=%d", len(tpl.ChildNodes()), len(frag.ChildNodes()))

	wrapper := frag.FirstChild()
	w, _ := wrapper.(*dom.Element)
	t.Logf("wrapper=%s children=%d", w.LocalName(), len(w.ChildNodes()))
	for i := 0; i < 5 && w.FirstChild() != nil; i++ {
		child := w.FirstChild()
		t.Logf("iter %d: wrapper.firstChild=%s(%s) parent=%v", i, child.NodeName(), child.NodeValue(), parentName(child))
		if err := frag.AppendChild(child); err != nil {
			t.Fatalf("appendChild: %v", err)
		}
		t.Logf("iter %d after: wrapper children=%d frag children=%d parent=%v",
			i, len(w.ChildNodes()), len(frag.ChildNodes()), parentName(child))
	}
}

func parentName(n dom.Node) string {
	if p := n.ParentNode(); p != nil {
		return p.NodeName()
	}
	return "<nil>"
}
