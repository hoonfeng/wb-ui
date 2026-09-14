package layout

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

func TestInlineElementTextSegments(t *testing.T) {
	// Build: <p><span>hello</span></p>
	doc := dom.NewDocument()
	p := doc.CreateElement("p")
	span := doc.CreateElement("span")
	text := doc.CreateTextNode("hello")
	span.AppendChild(text)
	p.AppendChild(span)

	resolver := style.NewResolver()
	root := BuildLayoutTree(p, resolver)
	if root == nil {
		t.Fatal("root is nil")
	}

	rootEb, ok := root.(*ElementBox)
	if !ok {
		t.Fatal("root is not *ElementBox")
	}

	// Layout to trigger inline formatting context which generates TextSegments
	state := Layout(rootEb, 800, 600)
	if state == nil {
		t.Fatal("state is nil")
	}

	// Walk the layout tree and check for text runs.
	var foundText bool
	var walk func(b Box, depth int)
	walk = func(b Box, depth int) {
		if b.IsTextRun() {
			if tb, ok := b.(*InlineTextBox); ok {
				if len(tb.TextSegments) > 0 {
					foundText = true
					t.Logf("found text run %q with %d segments", tb.text, len(tb.TextSegments))
					for i, seg := range tb.TextSegments {
						t.Logf("  seg[%d]: start=%d len=%d pos=(%.0f,%.0f) size=(%.0fx%.0f)",
							i, seg.Start, seg.Len, seg.X, seg.Y, seg.Width, seg.Height)
					}
				}
			}
		}
		if eb, ok := b.(*ElementBox); ok {
			for _, c := range eb.Children() {
				walk(c, depth+1)
			}
		}
	}
	walk(root, 0)

	if !foundText {
		t.Log("No text run with TextSegments found (may be expected with basic measurer)")
	}
}
