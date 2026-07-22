package layout

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/style"
)

// TestFlexNested_RowThenCol regresses the bug where a column flex container
// inside a row flex container would get height=0 because stretch overwrites
// the auto-height, and flex-grow items balloon to infinity.
func TestFlexNested_RowThenCol(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`* { margin:0; padding:0; }
.layout { display:flex; }
.sidebar { display:flex; flex-direction:column; width:200px; padding:12px; }
.main { display:flex; flex-direction:column; flex:1; }
.item { padding:6px 8px; font-size:13px; }
.toolbar { height:36px; }
.content { flex:1; padding:16px; }
.statusbar { height:24px; }
`)
	htmlEl.AppendChild(styleEl)

	body := dom.NewElement(doc, "body")
	htmlEl.AppendChild(body)

	layout := dom.NewElement(doc, "div")
	layout.SetClassName("layout")
	body.AppendChild(layout)

	sb := dom.NewElement(doc, "div")
	sb.SetClassName("sidebar")
	layout.AppendChild(sb)
	for _, n := range []string{"A", "B", "C"} {
		item := dom.NewElement(doc, "div")
		item.SetClassName("item")
		item.AppendChild(doc.CreateTextNode(n))
		sb.AppendChild(item)
	}

	main := dom.NewElement(doc, "div")
	main.SetClassName("main")
	layout.AppendChild(main)
	for _, info := range []struct{ cls, text string }{
		{"toolbar", "tools"}, {"content", "body text"}, {"statusbar", "ready"},
	} {
		el := dom.NewElement(doc, "div")
		el.SetClassName(info.cls)
		el.AppendChild(doc.CreateTextNode(info.text))
		main.AppendChild(el)
	}

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	root := BuildLayoutTree(htmlEl, resolver)
	if root == nil {
		t.Fatal("nil root")
	}
	Layout(root, 800, 600)

	bodyBox := root.Children[0]
	layoutBox := bodyBox.Children[0]
	sidebarBox := layoutBox.Children[0]
	mainBox := layoutBox.Children[1]

	// Regression 1: sidebar and main must have non-zero height
	if sidebarBox.Rect.Height <= 0 {
		t.Errorf("sidebar height should be > 0, got %g", sidebarBox.Rect.Height)
	}
	if mainBox.Rect.Height <= 0 {
		t.Errorf("main height should be > 0, got %g", mainBox.Rect.Height)
	}

	// Regression 2: layout height should be reasonable, not 1e6 sentinel
	if layoutBox.Rect.Height > 10000 {
		t.Errorf("layout height should not be sentinel, got %g", layoutBox.Rect.Height)
	}

	// Regression 3: sidebar items stack vertically
	for i := 1; i < len(sidebarBox.Children); i++ {
		if sidebarBox.Children[i].Rect.Y <= sidebarBox.Children[i-1].Rect.Y {
			t.Errorf("sidebar items should stack vertically, item[%d].Y=%g <= item[%d].Y=%g",
				i, sidebarBox.Children[i].Rect.Y, i-1, sidebarBox.Children[i-1].Rect.Y)
		}
	}

	t.Logf("layout: %gx%g sidebar: %gx%g main: %gx%g",
		layoutBox.Rect.Width, layoutBox.Rect.Height,
		sidebarBox.Rect.Width, sidebarBox.Rect.Height,
		mainBox.Rect.Width, mainBox.Rect.Height)
}
