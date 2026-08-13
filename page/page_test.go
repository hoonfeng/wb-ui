// Translation of: tests for Source/WebCore/page/Page.cpp / LocalFrame.cpp / LocalFrameView.cpp / SettingsBase.cpp
// Completeness: 60%
// Simplifications:
//   - tests exercise the public Page/Frame/FrameView/Settings API without verifying
//     detailed layout geometry (the layout package has its own test suite)

package page

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/rendering"
)

// TestNewSettingsDefaults verifies the WebKit defaults returned by NewSettings.
func TestNewSettingsDefaults(t *testing.T) {
	s := NewSettings()
	if s.DefaultFontSize != 16 {
		t.Errorf("DefaultFontSize = %v, want 16", s.DefaultFontSize)
	}
	if s.DefaultFixedFontSize != 13 {
		t.Errorf("DefaultFixedFontSize = %v, want 13", s.DefaultFixedFontSize)
	}
	if !s.JavaScriptEnabled {
		t.Error("JavaScriptEnabled = false, want true")
	}
	if !s.LoadsImagesAutomatically {
		t.Error("LoadsImagesAutomatically = false, want true")
	}
	if !s.CSSAnimationEnabled {
		t.Error("CSSAnimationEnabled = false, want true")
	}
	if s.PluginsEnabled {
		t.Error("PluginsEnabled = true, want false")
	}
	if !s.ViewportEnabled {
		t.Error("ViewportEnabled = false, want true")
	}
	if s.MaximumRenderTreeDepth != 512 {
		t.Errorf("MaximumRenderTreeDepth = %v, want 512", s.MaximumRenderTreeDepth)
	}
}

// TestSettingsResetToConsistentState verifies that ResetToConsistentState restores
// the defaults after mutation.
func TestSettingsResetToConsistentState(t *testing.T) {
	s := NewSettings()
	s.DefaultFontSize = 99
	s.JavaScriptEnabled = false
	s.PluginsEnabled = true
	s.ResetToConsistentState()
	if s.DefaultFontSize != 16 {
		t.Errorf("DefaultFontSize = %v, want 16 after reset", s.DefaultFontSize)
	}
	if !s.JavaScriptEnabled {
		t.Error("JavaScriptEnabled = false, want true after reset")
	}
	if s.PluginsEnabled {
		t.Error("PluginsEnabled = true, want false after reset")
	}
}

// TestNewPage verifies page construction, the main frame and settings wiring.
func TestNewPage(t *testing.T) {
	s := NewSettings()
	p := NewPage(s)
	if p.MainFrame() == nil {
		t.Fatal("MainFrame() = nil")
	}
	if p.MainFrame().Page() != p {
		t.Error("MainFrame().Page() does not match the page")
	}
	if p.Settings() != s {
		t.Error("Settings() does not match the provided settings")
	}
	if !p.Visible() {
		t.Error("a new page should be visible by default")
	}
	if p.RenderView() != nil {
		t.Error("RenderView() should be nil before a document is loaded")
	}
	if p.MainFrame().View() == nil {
		t.Error("main frame should have a default FrameView")
	}
}

// TestNewPageNilSettings verifies that NewPage supplies defaults when settings is nil.
func TestNewPageNilSettings(t *testing.T) {
	p := NewPage(nil)
	if p.Settings() == nil {
		t.Fatal("Settings() = nil; expected defaults")
	}
	if p.Settings().DefaultFontSize != 16 {
		t.Errorf("DefaultFontSize = %v, want 16", p.Settings().DefaultFontSize)
	}
}

// TestPageVisible verifies SetVisible / Visible toggling.
func TestPageVisible(t *testing.T) {
	p := NewPage(nil)
	if !p.Visible() {
		t.Fatal("new page should be visible")
	}
	p.SetVisible(false)
	if p.Visible() {
		t.Error("Visible() = true after SetVisible(false)")
	}
	p.SetVisible(true)
	if !p.Visible() {
		t.Error("Visible() = false after SetVisible(true)")
	}
	// Toggling to the same value is a no-op (just ensure it does not panic).
	p.SetVisible(true)
	if !p.Visible() {
		t.Error("Visible() should remain true")
	}
}

// TestFrameLoadHTMLEndToEnd is the end-to-end test: parsing an HTML string builds a
// Document and a RenderView on the frame (and propagates the render view to the page).
func TestFrameLoadHTMLEndToEnd(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	src := "<html><head><title>t</title></head><body><div>hello</div></body></html>"
	if err := mf.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	doc := mf.Document()
	if doc == nil {
		t.Fatal("Document() = nil after LoadHTML")
	}
	root := doc.DocumentElement()
	if root == nil || root.LocalName() != "html" {
		t.Fatalf("documentElement = %v, want <html>", root)
	}
	if rv := mf.RenderView(); rv == nil {
		t.Fatal("RenderView() = nil after LoadHTML")
	} else if !rv.IsRenderView() {
		t.Error("RenderView() did not report IsRenderView")
	}
	// The page's RenderView should be in sync with the main frame's.
	if p.RenderView() != mf.RenderView() {
		t.Error("Page.RenderView() != MainFrame.RenderView()")
	}
	// The resolver should have been created and be reusable.
	if mf.Resolver() == nil {
		t.Error("Resolver() = nil after LoadHTML")
	}
	// Loading a new document should not panic and should refresh the resolver cache.
	if err := mf.LoadHTML("<html><body><p>again</p></body></html>"); err != nil {
		t.Fatalf("second LoadHTML failed: %v", err)
	}
}

// TestFrameLoadHTMLErrors verifies that empty input is rejected.
func TestFrameLoadHTMLErrors(t *testing.T) {
	p := NewPage(nil)
	if err := p.MainFrame().LoadHTML(""); err == nil {
		t.Error("LoadHTML(\"\") = nil, want error")
	}
}

// TestFrameSetDocument verifies that SetDocument builds a render tree from a
// pre-parsed document.
func TestFrameSetDocument(t *testing.T) {
	doc, err := html.Parse("<html><body><span>x</span></body></html>")
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}
	p := NewPage(nil)
	mf := p.MainFrame()
	mf.SetDocument(doc)
	if mf.Document() != doc {
		t.Error("Document() does not match the document passed to SetDocument")
	}
	if mf.RenderView() == nil {
		t.Error("RenderView() = nil after SetDocument")
	}
}

// TestFrameViewLayout verifies that loading a document marks the view as needing
// layout and that Layout() clears the flag.
func TestFrameViewLayout(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	view := mf.View()
	if view == nil {
		t.Fatal("View() = nil")
	}
	// A brand-new view is marked as needing layout.
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false on a new view")
	}
	if err := mf.LoadHTML("<html><body><p>hi</p></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	// Loading a document should mark layout as pending.
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after LoadHTML")
	}
	// The frame-level NeedsLayout mirrors the view.
	if !mf.NeedsLayout() {
		t.Error("Frame.NeedsLayout() = false after LoadHTML")
	}
	view.Layout()
	if view.NeedsLayout() {
		t.Error("NeedsLayout() = true after Layout()")
	}
}

// TestFrameViewResize verifies that resizing marks layout as pending.
func TestFrameViewResize(t *testing.T) {
	f := NewFrame(nil)
	view := f.View()
	view.SetNeedsLayout(false) // start clean
	view.SetWidth(1024)
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after SetWidth")
	}
	if view.Width() != 1024 {
		t.Errorf("Width() = %v, want 1024", view.Width())
	}
	view.SetNeedsLayout(false)
	view.SetHeight(768)
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after SetHeight")
	}
	if view.Height() != 768 {
		t.Errorf("Height() = %v, want 768", view.Height())
	}
	// Setting the same value should not dirty the view.
	view.SetNeedsLayout(false)
	view.SetWidth(1024)
	view.SetHeight(768)
	if view.NeedsLayout() {
		t.Error("NeedsLayout() = true after no-op resize")
	}
	// SetSize should dirty when one dimension changes.
	view.SetNeedsLayout(false)
	view.SetSize(1280, 768)
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after SetSize change")
	}
	if view.Width() != 1280 {
		t.Errorf("Width() = %v, want 1280", view.Width())
	}
}

// TestFrameLayoutWithEmptyView verifies that Layout does not panic when no render
// view is present (e.g. before a document is loaded).
func TestFrameLayoutNoRenderView(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	// No document loaded yet: Layout should be a safe no-op.
	mf.Layout()
	view := mf.View()
	view.SetNeedsLayout(false)
	view.Layout()
	if view.NeedsLayout() {
		t.Error("NeedsLayout() = true after Layout() with no render view")
	}
}

// TestPageLayoutIfNeeded verifies that Page.LayoutIfNeeded delegates to the main
// frame view and clears the pending flag.
func TestPageLayoutIfNeeded(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	if err := mf.LoadHTML("<html><body><div>ok</div></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	if !p.MainFrame().NeedsLayout() {
		t.Fatal("expected layout to be pending after LoadHTML")
	}
	p.LayoutIfNeeded()
	if p.MainFrame().NeedsLayout() {
		t.Error("layout still pending after LayoutIfNeeded")
	}
	// A second LayoutIfNeeded with nothing pending is a no-op.
	p.LayoutIfNeeded()
}

// TestPageUtilityAndGroup verifies the utility-page flag and group name accessors.
func TestPageUtilityAndGroup(t *testing.T) {
	p := NewPage(nil)
	if p.IsUtilityPage() {
		t.Error("a normal page should not be a utility page")
	}
	p.SetIsUtilityPage(true)
	if !p.IsUtilityPage() {
		t.Error("IsUtilityPage = false after SetIsUtilityPage(true)")
	}
	if p.GroupName() != "" {
		t.Errorf("GroupName() = %q, want empty", p.GroupName())
	}
	p.SetGroupName("test-group")
	if p.GroupName() != "test-group" {
		t.Errorf("GroupName() = %q, want %q", p.GroupName(), "test-group")
	}
}

// TestFrameSetView verifies replacing the frame view keeps the back reference in sync.
func TestFrameSetView(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	orig := mf.View()
	newView := NewFrameView(mf, 320, 240)
	mf.SetView(newView)
	if mf.View() != newView {
		t.Fatal("View() did not update after SetView")
	}
	if newView.Frame() != mf {
		t.Error("new view's Frame() does not point back to the frame")
	}
	_ = orig
}

// TestFrameApplyTextChange verifies the incremental text-update path (C1): after a DOM
// Text node's data changes, Frame.ApplyTextChange re-syncs the RenderText and layout
// InlineTextBox, marks the view as needing layout, and a relayout produces the new text
// (without a full render-tree rebuild).
func TestFrameApplyTextChange(t *testing.T) {
	p := NewPage(nil)
	mf := p.MainFrame()
	if err := mf.LoadHTML("<html><body><p>hello</p></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	view := mf.View()
	view.Layout()

	// Locate the "hello" text node.
	var textNode *dom.Text
	dom.WalkComposedTree(mf.Document(), func(n dom.Node) {
		if tn, ok := n.(*dom.Text); ok && tn.Data() == "hello" {
			textNode = tn
		}
	})
	if textNode == nil {
		t.Fatal("text node not found")
	}

	rv := mf.RenderView()
	ro := rv.FindRenderObjectForNode(textNode)
	rt, ok := ro.(*rendering.RenderText)
	if !ok {
		t.Fatal("RenderText not found for text node")
	}
	if rt.Text() != "hello" {
		t.Fatalf("initial text = %q, want hello", rt.Text())
	}

	textNode.SetData("world")
	if !mf.ApplyTextChange(textNode) {
		t.Fatal("ApplyTextChange returned false")
	}
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after ApplyTextChange")
	}
	view.Layout()

	if rt.Text() != "world" {
		t.Fatalf("text after relayout = %q, want world", rt.Text())
	}
	if len(rt.Segments()) == 0 {
		t.Error("segments empty after relayout")
	}
}
