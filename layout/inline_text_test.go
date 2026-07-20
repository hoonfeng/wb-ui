package layout

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/style"
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
	// Layout to trigger inline formatting context which generates TextSegments
	Layout(root, 800, 600)
	t.Logf("After Layout:")
	var dump func(b *LayoutBox, depth int)
	dump = func(b *LayoutBox, depth int) {
		prefix := ""
		for i := 0; i < depth; i++ {
			prefix += "  "
		}
		t.Logf("%stype=%v rect=(%.0f,%.0f,%.0f,%.0f) children=%d segs=%d text=%q",
			prefix, b.Type, b.Rect.X, b.Rect.Y, b.Rect.Width, b.Rect.Height,
			len(b.Children), len(b.TextSegments), b.Text)
		for _, c := range b.Children {
			dump(c, depth+1)
		}
	}
	dump(root, 0)
	t.Logf("root type=%v children=%d", root.Type, len(root.Children))
	for i, c := range root.Children {
		t.Logf("  child[%d]: type=%v elem=%v isInline=%v isTextRun=%v",
			i, c.Type, c.Element != nil, c.IsInline(), c.IsTextRun())
		if c.Type == BoxAnonymous {
			for j, cc := range c.Children {
				t.Logf("    anon[%d]: type=%v elem=%v inline=%v textRun=%v text=%q segments=%d",
					j, cc.Type, cc.Element != nil, cc.IsInline(), cc.IsTextRun(),
					cc.Text, len(cc.TextSegments))
				if cc.Type == BoxInline {
					for k, ccc := range cc.Children {
						t.Logf("      inline[%d]: type=%v text=%q segments=%d children=%d",
							k, ccc.Type, ccc.Text, len(ccc.TextSegments), len(ccc.Children))
						for l, cccc := range ccc.Children {
							t.Logf("        sub[%d]: type=%v text=%q segments=%d",
								l, cccc.Type, cccc.Text, len(cccc.TextSegments))
						}
					}
				}
			}
		}
	}

	// Check that text runs inside span have TextSegments
	var foundText bool
	var walk func(b *LayoutBox, depth int)
	walk = func(b *LayoutBox, depth int) {
		prefix := ""
		for i := 0; i < depth; i++ {
			prefix += "  "
		}
		if b.IsTextRun() {
			if len(b.TextSegments) > 0 {
				foundText = true
				t.Logf("%sFOUND text run %q with %d segments", prefix, b.Text, len(b.TextSegments))
				for i, seg := range b.TextSegments {
					t.Logf("%s  seg[%d]: start=%d len=%d pos=(%.0f,%.0f) size=(%.0fx%.0f)",
						prefix, i, seg.Start, seg.Len, seg.X, seg.Y, seg.Width, seg.Height)
				}
			} else {
				t.Logf("%stext run %q segments=0", prefix, b.Text)
			}
		} else {
			t.Logf("%sbox type=%v text=%q children=%d segs=%d",
				prefix, b.Type, b.Text, len(b.Children), len(b.TextSegments))
		}
		for _, c := range b.Children {
			walk(c, depth+1)
		}
	}
	walk(root, 0)

	if !foundText {
		t.Error("No text run with TextSegments found")
	}
}
