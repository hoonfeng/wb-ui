// Translation of: Source/WebKit/WebProcess/WebPage/WebFrame.h
//                  Source/WebKit/WebProcess/WebPage/WebFrame.cpp
//                  Source/WebKit/UIProcess/API/C/WKFrame.cpp
// Completeness: 40%
// Simplifications:
//   - WebFrame is a thin wrapper over page.Frame; the WebKit WebFrame lives in the
//     WebContent process and is paired with a UIProcess-side WebFrameProxy. Both
//     halves are collapsed into this single struct.
//   - no frame tree (one frame per page); the parent/child relationships and the
//     remote-frame split are omitted
//   - no FrameLoader / policy delegates / navigation scheduler
//   - no JS WindowProxy: EvalJS lives on WebView and uses bindings.RegisterDOMBindings
//   - LoadHTML just delegates to page.Frame.LoadHTML

package webkit

import (
	"errors"
	"io"
	"os"
	"strings"

	"wb-ui/dom"
	"wb-ui/page"
	"wb-ui/rendering"
)

// WebFrame is the Go translation of WebKit::WebFrame (the WebContent-side frame
// object that wraps a WebCore::Frame and exposes the API-level frame operations).
// It holds a back-pointer to its owning WebView so callers reaching a frame from
// outside the WebView can climb back up to the top-level API.
type WebFrame struct {
	// frame is the underlying page.Frame, mirroring WebFrame::coreFrame().
	frame *page.Frame

	// webView is the owning WebView, mirroring the back-pointer kept by
	// WebFrame::page() / WebFrame::webView() (routed via WebPage).
	webView *WebView
}

// NewWebFrame constructs a WebFrame wrapping the given page.Frame for the given
// WebView, mirroring WebFrame::create with an existing core frame.
func NewWebFrame(wv *WebView, frame *page.Frame) *WebFrame {
	return &WebFrame{frame: frame, webView: wv}
}

// Frame returns the underlying page.Frame, mirroring WebFrame::coreFrame().
func (wf *WebFrame) Frame() *page.Frame { return wf.frame }

// WebView returns the owning WebView, mirroring the page/webView back-pointer on
// WebKit::WebFrame.
func (wf *WebFrame) WebView() *WebView { return wf.webView }

// LoadHTML parses an HTML source string into the underlying frame and builds the
// render tree, mirroring WebFrame::loadHTMLString (via WebFrameLoaderClient). It
// is the frame-level entry point that WebView.LoadHTML delegates to.
func (wf *WebFrame) LoadHTML(src string) error {
	if wf.frame == nil {
		return ErrNoFrame
	}
	return wf.frame.LoadHTML(src)
}

// Document returns the DOM document currently loaded in the frame, mirroring
// WebFrame::document() (which routes via coreFrame()->document()).
func (wf *WebFrame) Document() *dom.Document {
	if wf.frame == nil {
		return nil
	}
	return wf.frame.Document()
}

// RenderView returns the root of the render tree for the frame's document, mirroring
// the render view obtained via WebFrame::contentRenderer() -> view->renderRoot().
func (wf *WebFrame) RenderView() *rendering.RenderView {
	if wf.frame == nil {
		return nil
	}
	return wf.frame.RenderView()
}

// ErrNoFrame is returned when a WebFrame method is called on a frame whose
// underlying page.Frame is nil (e.g. after Close).
var ErrNoFrame = errors.New("webkit: frame is not attached")

// fetchURL is the simplified translation of WebFrameLoaderClient / Network load path.
// It supports the file:// scheme by reading the file from disk; any other scheme
// returns ErrNotImplemented because this port has no real network layer.
func fetchURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", errors.New("webkit: empty URL")
	}
	if !strings.HasPrefix(rawURL, "file://") {
		return "", ErrNotImplemented
	}
	// Accept file://localhost/path and file:///path forms; strip the authority if any.
	path := strings.TrimPrefix(rawURL, "file://")
	path = strings.TrimPrefix(path, "localhost/")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", errors.New("webkit: empty file path")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
