package page

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/rendering"
)

// TestRebuildKeepsBoxScrollOffset is the regression test for "content jumps
// out of view as soon as I type": RebuildRenderTree builds a brand-new
// render tree (new RenderBox objects, empty scroll-offset map), so every
// keystroke previously reset vertical scroll to 0 and auto-scroll then
// yanked the content to the caret row. RestoreScrollOffsetsFrom must carry
// the offsets across the rebuild by DOM node.
func TestRebuildKeepsBoxScrollOffset(t *testing.T) {
	f := &Frame{}
	doc := mustParse(t, `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}#sc{overflow:auto;width:200px;height:100px;position:relative}.c{height:400px}</style></head><body><div id="sc"><div class="c"></div></div></body></html>`)
	f.SetDocument(doc)
	if f.renderView == nil {
		t.Fatal("no render view after SetDocument")
	}
	rv := f.renderView
	sc := findBox(t, rv, "sc")
	if sc == nil {
		// Dump the tree so a future breakage is diagnosable.
		var walk func(o rendering.RenderObject, ind string)
		walk = func(o rendering.RenderObject, ind string) {
			if o == nil {
				return
			}
			name := o.RenderName()
			if el, ok := o.Node().(*dom.Element); ok {
				name += " id=" + el.GetAttribute("id")
			}
			t.Logf("%s%s", ind, name)
			for c := o.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c, ind+"  ")
			}
		}
		walk(rendering.RenderObject(rv), "")
		t.Fatal("scroll container not found")
	}
	rv.SetBoxScrollOffset(sc, 12, 137)

	// Rebuild exactly like every keystroke / DOM mutation does.
	f.RebuildRenderTree()
	rv2 := f.renderView
	if rv2 == nil {
		t.Fatal("no render view after rebuild")
	}
	if rv2 == rv {
		t.Fatal("rebuild reused the same RenderView — test is meaningless")
	}
	sc2 := findBox(t, rv2, "sc")
	if sc2 == nil {
		t.Fatal("scroll container not found after rebuild")
	}
	sx, sy := rv2.BoxScrollOffset(sc2)
	if sx != 12 || sy != 137 {
		t.Fatalf("after rebuild scroll=(%v,%v), want (12,137) — RebuildRenderTree must preserve per-box scroll offsets", sx, sy)
	}
}

// findBox locates the first RenderBox whose DOM element has the given id.
func findBox(t *testing.T, rv *rendering.RenderView, id string) *rendering.RenderBox {
	t.Helper()
	var found *rendering.RenderBox
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if found != nil {
			return
		}
		if el, ok := o.Node().(*dom.Element); ok && el.GetAttribute("id") == id {
			if b := asRenderBox(o); b != nil {
				found = b
				return
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	return found
}

func mustParse(t *testing.T, htmlStr string) *dom.Document {
	t.Helper()
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}
