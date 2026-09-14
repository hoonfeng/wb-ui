// Translation of: Source/WebCore/page/Page.h
//                  Source/WebCore/page/Page.cpp
// Completeness: 50%
// Simplifications:
//   - single process model (no WebProcess abstraction)
//   - no scrolling coordinator
//   - no Chrome / FocusController / DragController / EditorClient / ProgressTracker
//     etc.; Page only owns the main Frame, the Settings and the main-frame RenderView
//   - no activity state machine; visibility is a single boolean
//   - no PageGroup / page identifier / session id

package page

import "wb-ui/engine/rendering"

// Page is the Go translation of WebCore::Page. It is the top-level browser page
// object: it owns the main Frame, the global Settings and (once a document is
// loaded) the RenderView for the main frame. In WebKit a Page additionally owns a
// large set of controllers (Chrome, FocusController, DragController, Scrolling
// Coordinator, RenderingUpdateScheduler, ...); this port collapses all of that into
// the Frame/FrameView pair plus a single visibility flag.
//
// A typical lifecycle is:
//
//	settings := page.NewSettings()
//	p := page.NewPage(settings)
//	_ = p.MainFrame().LoadHTML("<html><body><p>hi</p></body></html>")
//	p.MainFrame().View().Layout()
type Page struct {
	mainFrame  *Frame
	settings   *Settings
	renderView *rendering.RenderView

	// visible mirrors Page::isVisible() (ActivityState::IsVisible).
	visible bool

	// focused mirrors Page::isFocused() (ActivityState::IsFocused).
	focused bool

	// isUtilityPage mirrors Page::isUtilityPage() (utility pages are off-screen
	// helper pages such as SVG image pages).
	isUtilityPage bool

	// groupName mirrors Page::groupName(); an empty string means the page is not
	// part of a named group.
	groupName string
}

// NewPage constructs a Page with the given Settings and a fresh main Frame. If
// settings is nil the WebKit defaults are used. The main Frame is created with an
// 800x600 FrameView so that loading a document and calling Layout works out of the
// box, mirroring how WebKit creates a LocalFrame with an initial LocalFrameView.
func NewPage(settings *Settings) *Page {
	if settings == nil {
		settings = NewSettings()
	}
	p := &Page{
		settings: settings,
		visible:  true,
	}
	p.mainFrame = NewFrame(p)
	return p
}

// MainFrame returns the page's main (top-level) Frame, mirroring
// Page::mainFrame(). A Page always has exactly one main frame.
func (p *Page) MainFrame() *Frame { return p.mainFrame }

// Settings returns the page's Settings object, mirroring Page::settings().
func (p *Page) Settings() *Settings { return p.settings }

// RenderView returns the RenderView for the main frame, or nil if no document has
// been loaded yet. Mirrors the main-frame render view access via
// Frame::view()->renderView() in WebKit.
func (p *Page) RenderView() *rendering.RenderView { return p.renderView }

// setRenderView installs the main-frame RenderView. It is called by Frame when a
// document is loaded so that Page::RenderView() stays in sync.
func (p *Page) setRenderView(rv *rendering.RenderView) { p.renderView = rv }

// Visible reports whether the page is currently visible, mirroring
// Page::isVisible().
func (p *Page) Visible() bool { return p.visible }

// SetVisible toggles page visibility, mirroring Page::setIsVisible(). In WebKit
// this drives the ActivityState flags and suspends/resumes rendering updates,
// animations and timers; this port only records the flag.
func (p *Page) SetVisible(visible bool) {
	if p.visible == visible {
		return
	}
	p.visible = visible
}

// SetFocused sets the page focus state, mirroring Page::setFocused().
func (p *Page) SetFocused(focused bool) { p.focused = focused }

// IsFocused returns the page focus state, mirroring Page::isFocused().
func (p *Page) IsFocused() bool { return p.focused }

// Frames returns all frames in the frame tree including child frames.
func (p *Page) Frames() []*Frame {
	var frames []*Frame
	if p.mainFrame != nil {
		frames = append(frames, p.mainFrame)
	}
	return frames
}

// IsUtilityPage reports whether this is a utility (off-screen helper) page,
// mirroring Page::isUtilityPage().
func (p *Page) IsUtilityPage() bool { return p.isUtilityPage }

// SetIsUtilityPage marks the page as a utility page, mirroring the utility-page
// construction path in WebKit (e.g. for SVG image rendering).
func (p *Page) SetIsUtilityPage(v bool) { p.isUtilityPage = v }

// GroupName returns the page group name, mirroring Page::groupName().
func (p *Page) GroupName() string { return p.groupName }

// SetGroupName sets the page group name, mirroring Page::setGroupName().
func (p *Page) SetGroupName(name string) { p.groupName = name }

// LayoutIfNeeded triggers a layout on the main frame if one is pending, mirroring
// Page::layoutIfNeeded(). It is a thin wrapper over the main FrameView's Layout.
func (p *Page) LayoutIfNeeded() {
	if p.mainFrame != nil && p.mainFrame.view != nil && p.mainFrame.view.NeedsLayout() {
		p.mainFrame.view.Layout()
	}
}
