package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/style"
)

// TestFlexClassBased tests flex layout using CSS class selectors
// (mimicking the real Vue app's approach) instead of inline styles.
func TestFlexClassBased(t *testing.T) {
	doc := dom.NewDocument()
	doc.SetQuirks(false)

	html := doc.CreateElement("html")
	doc.AppendChild(html)

	head := doc.CreateElement("head")
	html.AppendChild(head)

	body := doc.CreateElement("body")
	body.SetAttribute("class", "app-body")
	html.AppendChild(body)

	app := doc.CreateElement("div")
	app.SetAttribute("id", "app")
	app.SetAttribute("class", "app-root") // Vue's root element
	body.AppendChild(app)

	// Simulate Vue-rendered sidebar structure
	sidebar := doc.CreateElement("div")
	sidebar.SetAttribute("class", "sidebar-panel")
	app.AppendChild(sidebar)

	sidebarHdr := doc.CreateElement("div")
	sidebarHdr.SetAttribute("class", "panel-header")
	sidebar.AppendChild(sidebarHdr)

	hdrText := doc.CreateTextNode("EXPLORER")
	sidebarHdr.AppendChild(hdrText)

	tabBar := doc.CreateElement("div")
	tabBar.SetAttribute("class", "tab-bar")
	sidebar.AppendChild(tabBar)

	tabBtn := doc.CreateElement("div")
	tabBtn.SetAttribute("class", "tab-btn")
	tabBtnText := doc.CreateTextNode("Files")
	tabBtn.AppendChild(tabBtnText)
	tabBar.AppendChild(tabBtn)

	// CSS with classes (matching Vue's approach)
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	sheet := css.NewCSSStyleSheet()
	css.NewParser(`
		.app-body {
			margin: 0;
			padding: 0;
			font-size: 13px;
			font-family: sans-serif;
			color: #e6edf3;
			background-color: #0d1117;
		}
		.app-root {
			display: flex;
			flex-direction: row;
			width: 100%;
			height: 100%;
		}
		.sidebar-panel {
			display: flex;
			flex-direction: column;
			background-color: #0d1117;
			border-right: 1px solid #30363d;
		}
		.panel-header {
			display: flex;
			flex-direction: column;
			padding: 8px 12px;
			font-weight: 600;
		}
		.tab-bar {
			display: flex;
			flex-direction: row;
		}
		.tab-btn {
			display: flex;
			flex-direction: column;
			padding: 8px 12px;
		}
		div { box-sizing: border-box; }
	`).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	layoutRoot := layout.BuildLayoutTree(html, resolver)
	if layoutRoot == nil {
		t.Fatal("layoutRoot is nil")
	}

	state := layout.NewLayoutState(800, 600)
	bctx := &layout.BlockFormattingContext{}
	bctx.Layout(layoutRoot, state)

	// Find sidebar
	var findClass func(box *layout.LayoutBox, class string) *layout.LayoutBox
	findClass = func(box *layout.LayoutBox, class string) *layout.LayoutBox {
		if box.Element != nil && box.Element.GetAttribute("class") == class {
			return box
		}
		for _, c := range box.Children {
			if f := findClass(c, class); f != nil {
				return f
			}
		}
		return nil
	}

	appBox := findClass(layoutRoot, "app-root")
	if appBox == nil {
		// Try by id
		var findId func(box *layout.LayoutBox, id string) *layout.LayoutBox
		findId = func(box *layout.LayoutBox, id string) *layout.LayoutBox {
			if box.Element != nil && box.Element.GetAttribute("id") == id {
				return box
			}
			for _, c := range box.Children {
				if f := findId(c, id); f != nil {
					return f
				}
			}
			return nil
		}
		appBox = findId(layoutRoot, "app")
	}
	if appBox == nil {
		t.Fatal("appBox not found")
	}

	dispStr := "unknown"
	if appBox.Style != nil {
		dispStr = fmtDisplay(appBox.Style.Display)
	}
	t.Logf("appBox: w=%.0f h=%.0f disp=%s children=%d", appBox.Rect.Width, appBox.Rect.Height, dispStr, len(appBox.Children))

	for i, child := range appBox.Children {
		disp := "?"
		if child.Style != nil {
			disp = fmtDisplay(child.Style.Display)
		}
		t.Logf("  child[%d]: w=%.0f h=%.0f disp=%s type=%d text=%q",
			i, child.Rect.Width, child.Rect.Height, disp, child.Type, child.Text)

		if child.Rect.Width <= 0 && len(child.Children) > 0 {
			t.Errorf("child[%d] width is 0 but has %d children", i, len(child.Children))
		}
	}

	// Find "Files" text
	var findText func(box *layout.LayoutBox, text string) *layout.LayoutBox
	findText = func(box *layout.LayoutBox, text string) *layout.LayoutBox {
		if box.Text == text {
			return box
		}
		for _, c := range box.Children {
			if f := findText(c, text); f != nil {
				return f
			}
		}
		return nil
	}
	filesBox := findText(layoutRoot, "Files")
	if filesBox == nil {
		t.Fatal("could not find 'Files' text box")
	}
	t.Logf("'Files': TextSegments=%d", len(filesBox.TextSegments))
	for i, seg := range filesBox.TextSegments {
		t.Logf("  seg[%d]: W=%.0f H=%.0f X=%.0f Y=%.0f", i, seg.Width, seg.Height, seg.X, seg.Y)
		if seg.Width <= 0 {
			t.Error("'Files' text segment has zero width")
		}
	}
	if len(filesBox.TextSegments) == 0 {
		t.Error("'Files' has no text segments")
	}
}
