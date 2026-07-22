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
	"os"
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
	// stylesheets are silently skipped UNLESS a ResourceLoader is set. The WebView
	// sets this to a file-based loader that reads from the local filesystem.
	StyleSheetLoader func(href string) (string, error)

	// ScriptLoader is an optional callback for loading external scripts
	// referenced by <script src="..."> elements. It receives the src URL and
	// should return the JS text, or an error. When nil AND ResourceLoader is
	// also nil, external scripts are silently skipped. The WebView sets this
	// to a file-based loader that reads from the local filesystem, matching
	// the same pattern as StyleSheetLoader.
	ScriptLoader func(src string) (string, error)

	// ResourceLoader is the CachedResourceLoader used to load external resources
	// (stylesheets, scripts, images, fonts). When non-nil, extractAndAddStyles
	// uses it to load <link rel="stylesheet"> elements via the async resource
	// loading pipeline (MemoryCache + ResourceHandle), falling back to
	// StyleSheetLoader only when ResourceLoader is nil.
	ResourceLoader *CachedResourceLoader
}

// NewFrame constructs a Frame attached to the given Page. The frame is given a
// default 800x600 FrameView so that LoadHTML followed by Layout works without
// further setup, mirroring how WebKit creates a LocalFrameView for a new
// LocalFrame.
func NewFrame(page *Page) *Frame {
	f := &Frame{page: page}
	f.view = NewFrameView(f, 1280, 800)
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
	Logf("LoadHTML", "start input_size=%d", len(src))
	doc, err := html.Parse(src)
	if err != nil {
		return err
	}
	Logf("LoadHTML", "parsed tagCount=%d elementCount=%d", countTags(doc), len(doc.GetElementsByTagName("*")))
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
	Logf("SetDocument", "start hasResolver=%v", f.resolver != nil)
	f.document = doc
	if f.resolver == nil {
		f.resolver = style.NewResolver()
		f.resolver.AddStyleSheet(html5.NewUAStyleSheet())
		Logf("SetDocument", "created new resolver + UA sheet")
	} else {
		f.resolver.ClearCache()
		Logf("SetDocument", "cleared resolver cache")
	}

	f.extractAndAddStyles()
	builder := rendering.NewRenderTreeBuilder(f.resolver)
	f.renderView = builder.Build(doc)
	objCount := 0
	if f.renderView != nil {
		objCount = countRenderObjects(rendering.RenderObject(f.renderView))
	}
	Logf("SetDocument", "renderObjectCount=%d", objCount)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	Logf("SetDocument", "done")
}

// ExecuteScripts walks the document's <script> elements and executes both
// inline and external scripts via the ScriptEngine callback. Callers should
// invoke this AFTER RegisterDOMBindings so the JS runtime has access to the
// complete DOM API (document / window / Element etc.).
func (f *Frame) ExecuteScripts() {
	f.executeInlineScripts()
}

// RebuildRenderTree reconstructs the render tree from the current DOM without
// re-parsing stylesheets. This mirrors the style-recalc + attach phase in
// WebKit (Document::resolveStyle -> Style::TreeResolver::createRenderTree) that
// fires after DOM mutations outside of parsing. Embedders should call this
// after mutating the DOM (SetTextContent / SetAttribute / AppendChild) so the
// next layout pass sees an up-to-date render tree. It also clears the style
// resolver cache so new/changed inline styles take effect.
func (f *Frame) RebuildRenderTree() {
	Logf("RebuildRenderTree", "start hasDocument=%v hasResolver=%v", f.document != nil, f.resolver != nil)
	if f.document == nil {
		Logf("RebuildRenderTree", "skip: no document")
		return
	}
	if f.resolver != nil {
		f.extractAndAddStyles()
		f.resolver.ClearCache()
	}
	builder := rendering.NewRenderTreeBuilder(f.resolver)
	f.renderView = builder.Build(f.document)
	objCount := 0
	if f.renderView != nil {
		objCount = countRenderObjects(rendering.RenderObject(f.renderView))
	}
	Logf("RebuildRenderTree", "renderObjectCount=%d", objCount)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	Logf("RebuildRenderTree", "done")
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

// extractAndAddStyles finds all <style> elements in the current document,
// parses their CSS text, and adds the resulting stylesheets to the style
// resolver. Previously extracted stylesheets are removed first so that
// dynamically changed style content is reflected correctly.
func (f *Frame) extractAndAddStyles() {
	Logf("extractAndAddStyles", "start docOk=%v resolverOk=%v", f.document != nil, f.resolver != nil)
	if f.document == nil || f.resolver == nil {
		Logf("extractAndAddStyles", "skip: no doc/resolver")
		return
	}

	removed := len(f.styleSheets)
	for _, sheet := range f.styleSheets {
		f.resolver.RemoveStyleSheet(sheet)
	}
	f.styleSheets = nil
	Logf("extractAndAddStyles", "removedPrevSheets=%d", removed)

	styleElements := f.document.GetElementsByTagName("style")
	Logf("extractAndAddStyles", "styleElementCount=%d", len(styleElements))
	for i, styleEl := range styleElements {
		cssText := styleEl.TextContent()
		if strings.TrimSpace(cssText) == "" {
			Logf("extractAndAddStyles", "style[%d]: empty, skip", i)
			continue
		}
		sheet := css.NewCSSStyleSheetWithOwner(styleEl, "")
		p := css.NewParser(cssText)
		p.ParseStyleSheetInto(sheet)
		rules := sheet.Rules()
		f.resolver.AddStyleSheet(sheet)
		f.styleSheets = append(f.styleSheets, sheet)
		Logf("extractAndAddStyles", "style[%d]: cssLen=%d rules=%d", i, len(cssText), len(rules))
	}

	linkElements := f.document.GetElementsByTagName("link")
	linkCount := 0
	for _, linkEl := range linkElements {
		l, ok := html5.ToLinkElement(linkEl)
		if !ok || !l.IsStyleSheet() {
			continue
		}
		href := l.Href()
		if href == "" {
			continue
		}
		linkCount++
		Logf("extractAndAddStyles", "link[%d]: href=%q", linkCount, href)
		if f.ResourceLoader != nil {
			f.ResourceLoader.LoadStylesheet(href, &frameStyleSheetClient{
				frame: f,
				owner: linkEl,
				href:  href,
			})
			Logf("extractAndAddStyles", "link[%d]: async load queued", linkCount)
		} else if f.StyleSheetLoader != nil {
			cssText, err := f.StyleSheetLoader(href)
			if err != nil || strings.TrimSpace(cssText) == "" {
				Logf("extractAndAddStyles", "link[%d]: load failed err=%v", linkCount, err)
				continue
			}
			sheet := css.NewCSSStyleSheetWithOwner(linkEl, href)
			p := css.NewParser(cssText)
			p.ParseStyleSheetInto(sheet)
			f.resolver.AddStyleSheet(sheet)
			f.styleSheets = append(f.styleSheets, sheet)
			Logf("extractAndAddStyles", "link[%d]: sync loaded len=%d", linkCount, len(cssText))
		}
	}
	Logf("extractAndAddStyles", "done: totalStyleSheets=%d linkCount=%d", len(f.styleSheets), linkCount)
}

// frameStyleSheetClient implements CachedResourceClient to handle the
// asynchronous delivery of an externally loaded stylesheet.
type frameStyleSheetClient struct {
	frame *Frame
	owner dom.Node
	href  string
}

func (c *frameStyleSheetClient) NotifyFinished(resource *CachedResource) {
	if resource.Status() != CachedResourceStatusLoaded {
		Logf("extractAndAddStyles", "async: href=%q status=%d (not loaded)", c.href, resource.Status())
		return
	}
	data := resource.Data()
	if len(data) == 0 {
		Logf("extractAndAddStyles", "async: href=%q empty data", c.href)
		return
	}
	cssText := string(data)
	if strings.TrimSpace(cssText) == "" {
		Logf("extractAndAddStyles", "async: href=%q whitespace only", c.href)
		return
	}

	sheet := css.NewCSSStyleSheetWithOwner(c.owner, c.href)
	p := css.NewParser(cssText)
	p.ParseStyleSheetInto(sheet)
	rules := sheet.Rules()
	c.frame.resolver.AddStyleSheet(sheet)
	c.frame.styleSheets = append(c.frame.styleSheets, sheet)
	Logf("extractAndAddStyles", "async: href=%q loaded len=%d rules=%d", c.href, len(cssText), len(rules))

	if c.frame.renderView != nil {
		c.frame.view.SetNeedsLayout(true)
		Logf("extractAndAddStyles", "async: setNeedsLayout")
	}
}

// executeInlineScripts finds all <script> elements and executes them.
// External scripts are loaded via ScriptLoader or ResourceLoader.
// Inline scripts are executed directly — NO try/catch wrapper (real errors
// propagate to the frame's error log).
func (f *Frame) executeInlineScripts() {
	if f.document == nil || f.ScriptEngine == nil {
		return
	}
	scriptElements := f.document.GetElementsByTagName("script")
	Logf("ScriptLoad", "start: scripts=%d", len(scriptElements))
	for i, el := range scriptElements {
		s, ok := html5.ToScriptElement(el)
		if !ok {
			continue
		}
		t := s.Type()
		if t != "" && t != "text/javascript" && t != "module" {
			Logf("ScriptLoad", "[%d] skip: type=%q", i, t)
			continue
		}

		if src := s.Src(); src != "" {
			Logf("ScriptLoad", "[%d] external: src=%q type=%q", i, src, t)
			if f.ResourceLoader != nil {
				f.ResourceLoader.LoadScript(src, &frameScriptClient{
					frame: f,
					src:   src,
				})
				Logf("ScriptLoad", "[%d] async queued", i)
				continue
			}
			if f.ScriptLoader != nil {
				code, err := f.ScriptLoader(src)
				if err != nil {
					Logf("ScriptLoad", "[%d] LOAD FAIL: %v", i, err)
					continue
				}
				if strings.TrimSpace(code) == "" {
					Logf("ScriptLoad", "[%d] empty content", i)
					continue
				}
				Logf("ScriptLoad", "[%d] exec: len=%d", i, len(code))
				// Execute directly — no try/catch wrapper.
				if err := f.ScriptEngine(code); err != nil {
					Logf("ScriptLoad", "[%d] EXEC FAIL: %v", i, err)
				} else {
					Logf("ScriptLoad", "[%d] OK", i)
				}
				continue
			}
			Logf("ScriptLoad", "[%d] no loader, skip", i)
			continue
		}

		// Inline script: execute directly.
		code := s.Text()
		if strings.TrimSpace(code) == "" {
			Logf("ScriptLoad", "[%d] inline empty", i)
			continue
		}
		Logf("ScriptLoad", "[%d] inline: len=%d", i, len(code))
		if err := f.ScriptEngine(code); err != nil {
			Logf("ScriptLoad", "[%d] INLINE FAIL: %v", i, err)
		} else {
			Logf("ScriptLoad", "[%d] inline OK", i)
		}
	}
	Logf("ScriptLoad", "done")
}

// frameScriptClient implements CachedResourceClient for async script loading.
type frameScriptClient struct {
	frame *Frame
	src   string
}

func (c *frameScriptClient) NotifyFinished(resource *CachedResource) {
	if resource.Status() != CachedResourceStatusLoaded {
		Logf("ScriptLoad", "async FAIL: src=%q status=%d", c.src, resource.Status())
		return
	}
	data := resource.Data()
	if len(data) == 0 {
		Logf("ScriptLoad", "async empty: src=%q", c.src)
		return
	}
	code := string(data)
	if strings.TrimSpace(code) == "" {
		Logf("ScriptLoad", "async whitespace: src=%q", c.src)
		return
	}
	Logf("ScriptLoad", "async exec: src=%q len=%d", c.src, len(code))
	if err := c.frame.ScriptEngine(code); err != nil {
		Logf("ScriptLoad", "async EXEC FAIL: src=%q %v", c.src, err)
	} else {
		Logf("ScriptLoad", "async OK: src=%q", c.src)
	}
}

// logError logs script/resource errors (legacy; prefer Logf).
func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[page] "+format+"\n", args...)
}

// Layout triggers a layout on the frame's view.
func (f *Frame) Layout() {
	if f.view != nil {
		f.view.Layout()
	}
}

// ─── Verbose logging helpers ─────────────────────────────

func countTags(doc *dom.Document) int {
	count := 0
	walk := func(n dom.Node) {
		if _, ok := n.(*dom.Element); ok {
			count++
		}
	}
	walkNode(doc, walk)
	return count
}

func countRenderObjects(ro rendering.RenderObject) int {
	count := 0
	var walkRO func(r rendering.RenderObject)
	walkRO = func(r rendering.RenderObject) {
		count++
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			walkRO(c)
		}
	}
	walkRO(ro)
	return count
}

func walkNode(n dom.Node, fn func(dom.Node)) {
	fn(n)
	switch v := n.(type) {
	case *dom.Document:
		if el := v.DocumentElement(); el != nil {
			walkNode(el, fn)
		}
		if b := v.Head(); b != nil {
			walkNode(b, fn)
		}
		if b := v.Body(); b != nil {
			walkNode(b, fn)
		}
	case *dom.Element:
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			walkNode(c, fn)
		}
	}
}

func trimForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
