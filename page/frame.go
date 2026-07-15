// Translation of: Source/WebCore/page/Frame.h
//                  Source/WebCore/page/LocalFrame.h
//                  Source/WebCore/page/LocalFrame.cpp
// Completeness: 85%
// Simplifications:
//   - no frame tree (single frame per page); the Local/Remote frame split is omitted
//   - no NavigationScheduler / FrameLoaderClient / WindowProxy
//   - no owner element / sandbox flags / opener relationship
//   - LoadHTML drives HTML parse -> Document -> style resolve -> RenderTree build
//     directly, with no incremental parsing or script execution

package page

import (
	"errors"
	"fmt"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/rendering"
	"wb-ui/style"
)

// Frame is the Go translation of WebCore::LocalFrame. A Frame is the rendering
// context for a single Document: it owns the document, the style resolver and the
// RenderView built from that document, plus the FrameView that manages the viewport
// and layout. In WebKit a Frame is part of a frame tree and is associated with a
// loader, a window proxy and an owner element; this port models a single
// standalone frame attached to a Page.
type Frame struct {
	page *Page

	// document is the currently-loaded DOM Document, mirroring
	// LocalFrame::document().
	document *dom.Document

	// renderView is the root of the render tree built from document, mirroring
	// the render view reached via LocalFrame::contentRenderer() / FrameView.
	renderView *rendering.RenderView

	// view is the FrameView managing the viewport and layout, mirroring
	// LocalFrame::view().
	view *FrameView

	// resolver resolves ComputedStyles for the document's elements. It is reused
	// across render-tree rebuilds so that added style sheets persist.
	resolver *style.Resolver

	// styleSheets tracks the CSSStyleSheets extracted from <style> elements
	// in the current document. They are removed from the resolver before a
	// fresh extraction on RebuildRenderTree, preventing stale rules from
	// accumulating when style content changes dynamically.
	styleSheets []*css.CSSStyleSheet

	// ScriptEngine is an optional callback for executing JavaScript. When set,
	// executeInlineScripts calls it for each inline <script> element found in
	// the document. The WebView sets this to its EvalJS method so that inline
	// scripts run after the document is parsed and styles are extracted.
	ScriptEngine func(code string) error

	// StyleSheetLoader is an optional callback for loading external stylesheets
	// referenced by <link rel="stylesheet" href="..."> elements. It receives the
	// href URL and should return the CSS text, or an error. When nil, external
	// stylesheets are silently skipped. The WebView sets this to a file-based
	// loader that reads from the local filesystem.
	StyleSheetLoader func(href string) (string, error)
}

// NewFrame constructs a Frame attached to the given Page. The frame is given a
// default 800x600 FrameView so that LoadHTML followed by Layout works without
// further setup, mirroring how WebKit creates a LocalFrameView for a new
// LocalFrame.
func NewFrame(page *Page) *Frame {
	f := &Frame{page: page}
	f.view = NewFrameView(f, 800, 600)
	return f
}

// Page returns the owning Page, mirroring Frame::page().
func (f *Frame) Page() *Page { return f.page }

// Document returns the currently-loaded Document, mirroring
// LocalFrame::document(). It returns nil until LoadHTML or SetDocument is called.
func (f *Frame) Document() *dom.Document { return f.document }

// RenderView returns the root render object for the frame's document, mirroring
// the render view obtained via FrameView::renderView() in WebKit.
func (f *Frame) RenderView() *rendering.RenderView { return f.renderView }

// View returns the FrameView, mirroring LocalFrame::view().
func (f *Frame) View() *FrameView { return f.view }

// Resolver returns the style resolver used to build this frame's render tree.
// Callers may add style sheets to it before (re)loading a document so that the
// next render-tree build takes them into account.
func (f *Frame) Resolver() *style.Resolver { return f.resolver }

// SetView installs a new FrameView, mirroring LocalFrame::setView(). The previous
// view is replaced; the new view's frame pointer is set to this frame.
func (f *Frame) SetView(v *FrameView) {
	f.view = v
	if v != nil {
		v.frame = f
	}
}

// LoadHTML parses an HTML source string, creates a Document and builds the render
// tree for it. It is the Go translation of the WebKit load pipeline
// (FrameLoader::load -> HTMLDocumentParser -> Document -> attachRenderTree) for the
// common case of loading an inline HTML string. After it returns, Document() and
// RenderView() are populated and the frame's view is marked as needing layout.
func (f *Frame) LoadHTML(src string) error {
	if src == "" {
		return errors.New("page: empty HTML input")
	}
	doc, err := html.Parse(src)
	if err != nil {
		return err
	}
	f.SetDocument(doc)
	return nil
}

// SetDocument installs a Document on the frame and (re)builds the render tree,
// mirroring the document-attach step of the load pipeline
// (FrameLoader::clear / setDocument -> FrameView::setContents). It creates a style
// resolver if none exists yet, extracts CSS from <style> elements, and constructs
// the render tree via rendering.RenderTreeBuilder. The Page's main-frame RenderView
// is kept in sync.
func (f *Frame) SetDocument(doc *dom.Document) {
	f.document = doc
	if f.resolver == nil {
		f.resolver = style.NewResolver()
		// Inject the UA default stylesheet for HTML form controls (mirrors
		// WebCore/css/html.css). It must be added before author sheets so
		// the cascade gives author styles higher priority.
		f.resolver.AddStyleSheet(html5.NewUAStyleSheet())
	} else {
		f.resolver.ClearCache()
	}

	// Extract CSS from <style> elements in the document and add them to the
	// resolver. This mirrors the WebKit path where StyleEngine collects inline
	// stylesheets from <style> elements after the HTML parser emits them.
	f.extractAndAddStyles()

	// Execute inline <script> elements found in the document.
	// This runs before the render tree is built so that scripts can mutate
	// the DOM before the first paint, matching browser behavior.
	f.executeInlineScripts()

	builder := rendering.NewRenderTreeBuilder(f.resolver)
	f.renderView = builder.Build(doc)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	// A new document invalidates layout.
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
}

// RebuildRenderTree reconstructs the render tree from the current DOM without
// re-parsing stylesheets. This mirrors the style-recalc + attach phase in
// WebKit (Document::resolveStyle -> Style::TreeResolver::createRenderTree) that
// fires after DOM mutations outside of parsing. Embedders should call this
// after mutating the DOM (SetTextContent / SetAttribute / AppendChild) so the
// next layout pass sees an up-to-date render tree. It also clears the style
// resolver cache so new/changed inline styles take effect.
func (f *Frame) RebuildRenderTree() {
	if f.document == nil {
		return
	}
	if f.resolver != nil {
		// Re-extract styles from <style> elements before rebuilding. This
		// catches dynamically added/modified <style> elements whose CSS text
		// may have changed since the last render-tree build.
		f.extractAndAddStyles()
		f.resolver.ClearCache()
	}
	builder := rendering.NewRenderTreeBuilder(f.resolver)
	f.renderView = builder.Build(f.document)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
}

// NeedsLayout reports whether the frame's view requires a layout pass, mirroring

// extractAndAddStyles finds all <style> elements in the current document,
// parses their CSS text, and adds the resulting stylesheets to the style
// resolver. Previously extracted stylesheets are removed first so that
// dynamically changed style content is reflected correctly.
func (f *Frame) extractAndAddStyles() {
	if f.document == nil || f.resolver == nil {
		return
	}

	// Remove previously extracted dynamic stylesheets to prevent stale rules
	// from accumulating.
	for _, sheet := range f.styleSheets {
		f.resolver.RemoveStyleSheet(sheet)
	}
	f.styleSheets = nil

	// Find all <style> elements and add their content as new sheets.
	styleElements := f.document.GetElementsByTagName("style")
	for _, styleEl := range styleElements {
		cssText := styleEl.TextContent()
		if strings.TrimSpace(cssText) == "" {
			continue
		}
		sheet := css.NewCSSStyleSheetWithOwner(styleEl, "")
		p := css.NewParser(cssText)
		p.ParseStyleSheetInto(sheet)
		f.resolver.AddStyleSheet(sheet)
		f.styleSheets = append(f.styleSheets, sheet)
	}

	// Process <link rel="stylesheet"> elements: resolve href via the
	// StyleSheetLoader callback and add the parsed CSS to the resolver.
	linkElements := f.document.GetElementsByTagName("link")
	for _, linkEl := range linkElements {
		l, ok := html5.ToLinkElement(linkEl)
		if !ok || !l.IsStyleSheet() {
			continue
		}
		href := l.Href()
		if href == "" {
			continue
		}

		// Use the custom loader if available; otherwise skip external sheets.
		if f.StyleSheetLoader == nil {
			continue
		}
		cssText, err := f.StyleSheetLoader(href)
		if err != nil || strings.TrimSpace(cssText) == "" {
			continue
		}
		sheet := css.NewCSSStyleSheetWithOwner(linkEl, href)
		p := css.NewParser(cssText)
		p.ParseStyleSheetInto(sheet)
		f.resolver.AddStyleSheet(sheet)
		f.styleSheets = append(f.styleSheets, sheet)
	}
}

// executeInlineScripts finds all inline <script> elements (no src attribute,
// type="text/javascript" or no type) and executes them via the ScriptEngine
// callback (if set). External scripts and non-JS types are skipped.
// This mirrors the HTML parsing step where scripts are encountered and
// executed in order (though this port does not block parsing on script
// execution).
func (f *Frame) executeInlineScripts() {
	if f.document == nil || f.ScriptEngine == nil {
		return
	}
	scriptElements := f.document.GetElementsByTagName("script")
	for _, el := range scriptElements {
		s, ok := html5.ToScriptElement(el)
		if !ok {
			continue
		}
		// Skip external scripts (need a network fetch; not yet implemented).
		if s.Src() != "" {
			continue
		}
		// Only execute standard JavaScript; skip modules and other types.
		if s.Type() != "text/javascript" {
			continue
		}
		code := s.Text()
		if strings.TrimSpace(code) == "" {
			continue
		}
		// Execute via the ScriptEngine callback (set by WebView).
		if err := f.ScriptEngine(code); err != nil {
			// Log the error but continue executing remaining scripts,
			// matching browser behavior where one script failure does
			// not block subsequent scripts.
			logError("script execution failed: %v", err)
		}
	}
}

// NeedsLayout reports whether the frame's view requires a layout pass, mirroring
// the per-frame layout-pending check in WebKit (FrameView::needsLayout()).
func (f *Frame) NeedsLayout() bool {
	return f.view != nil && f.view.NeedsLayout()
}

// SetNeedsLayout marks the frame's view as needing (or not needing) a layout
// pass, mirroring FrameView::setNeedsLayout(). Embedders should call this
// after mutating the DOM outside of a style recalc so the next EnsureLayout
// rebuilds/relayouts the render tree.
func (f *Frame) SetNeedsLayout(needs bool) {
	if f.view != nil {
		f.view.SetNeedsLayout(needs)
	}
}

// logError is a placeholder for production error logging.
// In a real deployment, replace this with a proper logging framework
// (e.g., zap, log/slog) or a dedicated package-level logger instance.
func logError(format string, args ...interface{}) {
	// In production: logger.Errorf(format, args...)
	_ = fmt.Sprintf(format, args...)
}
// Layout triggers a layout on the frame's view, mirroring the Frame-level
// layout entry point (LocalFrameView::layout() reached via Frame::view()). It is a
// no-op when no view or render view is present.
func (f *Frame) Layout() {
	if f.view != nil {
		f.view.Layout()
	}
}