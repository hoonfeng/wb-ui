// Translation of: tests for Source/WebKit/WebView.cpp (the Go WebView entry point)
//                  Source/WebKit/UIProcess/win/WebView.cpp (the Win port test surface)
// Completeness: 50%
// Simplifications:
//   - tests exercise the public WebView API against the real page/rendering/jsc
//     packages; there is no IPC stub or mock process

package webkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewWebView verifies the construction invariants of NewWebView: a Page, a
// main WebFrame, default Settings and an 800x600 viewport are wired up ready to
// load HTML.
func TestNewWebView(t *testing.T) {
	wv := NewWebView()
	if wv.Page() == nil {
		t.Fatal("Page() = nil")
	}
	if wv.MainFrame() == nil {
		t.Fatal("MainFrame() = nil")
	}
	if wv.MainFrame().WebView() != wv {
		t.Error("MainFrame().WebView() does not match the parent WebView")
	}
	if wv.MainFrame().Frame() != wv.Page().MainFrame() {
		t.Error("MainFrame().Frame() does not match Page().MainFrame()")
	}
	if wv.Settings() == nil {
		t.Fatal("Settings() = nil")
	}
	if !wv.Settings().JavaScriptEnabled {
		t.Error("JavaScriptEnabled should be true by default")
	}
	if wv.Width() != DefaultWebViewWidth || wv.Height() != DefaultWebViewHeight {
		t.Errorf("viewport = %dx%d, want %dx%d",
			wv.Width(), wv.Height(), DefaultWebViewWidth, DefaultWebViewHeight)
	}
}

// TestWebViewLoadHTMLAndDocument verifies that LoadHTML populates the main frame's
// document and render view, mirroring the WebFrame load pipeline.
func TestWebViewLoadHTMLAndDocument(t *testing.T) {
	wv := NewWebView()
	src := "<html><head><title>t</title></head><body><div>hello</div></body></html>"
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	doc := wv.MainFrame().Document()
	if doc == nil {
		t.Fatal("Document() = nil after LoadHTML")
	}
	if doc.Title() != "t" {
		t.Errorf("Document().Title() = %q, want %q", doc.Title(), "t")
	}
	if wv.MainFrame().RenderView() == nil {
		t.Error("RenderView() = nil after LoadHTML")
	}
	if wv.Page().RenderView() != wv.MainFrame().RenderView() {
		t.Error("Page.RenderView() not in sync with MainFrame.RenderView()")
	}
}

// TestWebViewRenderEndToEnd is the end-to-end pipeline test: LoadHTML → Layout →
// Paint → RGBA pixel buffer.
func TestWebViewRenderEndToEnd(t *testing.T) {
	wv := NewWebView()
	wv.Resize(40, 30)
	src := `<html><body style="background-color: rgb(255, 0, 0);"><p>hi</p></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	pixels, err := wv.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	expectedLen := 40 * 30 * 4
	if len(pixels) != expectedLen {
		t.Fatalf("pixel buffer length = %d, want %d", len(pixels), expectedLen)
	}
	if wv.MainFrame().RenderView() == nil {
		t.Error("RenderView() = nil after Render")
	}
	view := wv.Page().MainFrame().View()
	if view.NeedsLayout() {
		t.Error("NeedsLayout() = true after Render, want false (layout should have run)")
	}
}

// TestWebViewRenderNoDocument verifies that Render returns ErrNoDocument before any
// HTML has been loaded.
func TestWebViewRenderNoDocument(t *testing.T) {
	wv := NewWebView()
	_, err := wv.Render()
	if err != ErrNoDocument {
		t.Errorf("Render before load err = %v, want ErrNoDocument", err)
	}
}

// TestWebViewResize verifies that Resize updates the viewport dimensions and marks
// the main frame's view as needing layout.
func TestWebViewResize(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><body><p>x</p></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	view := wv.Page().MainFrame().View()
	if view == nil {
		t.Fatal("MainFrame().View() = nil")
	}
	view.SetNeedsLayout(false)
	wv.Resize(320, 240)
	if wv.Width() != 320 || wv.Height() != 240 {
		t.Errorf("viewport after resize = %dx%d, want 320x240", wv.Width(), wv.Height())
	}
	if view.Width() != 320 || view.Height() != 240 {
		t.Errorf("FrameView size after resize = %dx%d, want 320x240", view.Width(), view.Height())
	}
	if !view.NeedsLayout() {
		t.Error("NeedsLayout() = false after Resize, want true")
	}
	wv.Resize(-5, -5)
	if wv.Width() != 0 || wv.Height() != 0 {
		t.Errorf("viewport after negative resize = %dx%d, want 0x0", wv.Width(), wv.Height())
	}
}

// TestWebViewEvalJS verifies that EvalJS evaluates a script against the loaded
// document via the bindings package.
func TestWebViewEvalJS(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><head><title>hello</title></head><body><div id=\"x\">y</div></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	if _, err := wv.EvalJS(`console.log(document.title);`); err != nil {
		t.Fatalf("EvalJS title failed: %v", err)
	}
	if got := strings.TrimSpace(wv.ConsoleOutput()); got != "hello" {
		t.Errorf("document.title output = %q, want %q", got, "hello")
	}
	wv.ResetConsole()
	if _, err := wv.EvalJS(`document.getElementById("x").id = "changed";`); err != nil {
		t.Fatalf("EvalJS mutate failed: %v", err)
	}
	if _, err := wv.EvalJS(`console.log(document.getElementById("changed").tagName);`); err != nil {
		t.Fatalf("EvalJS read-back failed: %v", err)
	}
	if got := strings.TrimSpace(wv.ConsoleOutput()); got != "DIV" {
		t.Errorf("tagName after rename = %q, want DIV", got)
	}
}

// TestWebViewEvalJSDisabled verifies that EvalJS returns ErrJavaScriptDisabled
// when Settings.JavaScriptEnabled is false.
func TestWebViewEvalJSDisabled(t *testing.T) {
	wv := NewWebView()
	wv.Settings().JavaScriptEnabled = false
	defer func() { wv.Settings().JavaScriptEnabled = true }()
	if err := wv.LoadHTML("<html><body><p>x</p></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	v, err := wv.EvalJS(`1 + 2`)
	if err != ErrJavaScriptDisabled {
		t.Errorf("EvalJS err = %v, want ErrJavaScriptDisabled", err)
	}
	if !v.IsUndefined() {
		t.Errorf("EvalJS return value = %v, want undefined", v)
	}
}

// TestWebViewEvalJSNoDocument verifies that EvalJS still runs (and produces console
// output) when no document is loaded.
func TestWebViewEvalJSNoDocument(t *testing.T) {
	wv := NewWebView()
	if _, err := wv.EvalJS(`console.log(1 + 2);`); err != nil {
		t.Fatalf("EvalJS failed: %v", err)
	}
	if got := strings.TrimSpace(wv.ConsoleOutput()); got != "3" {
		t.Errorf("1+2 console output = %q, want %q", got, "3")
	}
}

// TestWebViewLoadURLHTTP verifies that http:// URLs are processed through the
// network layer (will fail with a network error rather than ErrNotImplemented).
func TestWebViewLoadURLHTTP(t *testing.T) {
	wv := NewWebView()
	err := wv.LoadURL("http://localhost:19765/nonexistent")
	if err == nil {
		t.Fatal("LoadURL(http) expected error, got nil")
	}
	if err == ErrNotImplemented {
		t.Fatal("LoadURL(http) returned ErrNotImplemented — HTTP path not active")
	}
	t.Logf("LoadURL(http) correctly returned network error: %v", err)
}

// TestWebViewLoadURLHTTPS verifies that https:// URLs are processed through the
// network layer (will fail with a network error rather than ErrNotImplemented).
func TestWebViewLoadURLHTTPS(t *testing.T) {
	wv := NewWebView()
	err := wv.LoadURL("https://localhost:19766/nonexistent")
	if err == nil {
		t.Fatal("LoadURL(https) expected error, got nil")
	}
	if err == ErrNotImplemented {
		t.Fatal("LoadURL(https) returned ErrNotImplemented — HTTPS path not active")
	}
	t.Logf("LoadURL(https) correctly returned network error: %v", err)
}

// TestWebViewLoadURLFile verifies that file:// URLs are loaded from disk.
func TestWebViewLoadURLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	src := "<html><head><title>file</title></head><body><p>from disk</p></body></html>"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	wv := NewWebView()
	if err := wv.LoadURL("file:///" + path); err != nil {
		t.Fatalf("LoadURL(file://) failed: %v", err)
	}
	if doc := wv.MainFrame().Document(); doc == nil {
		t.Fatal("Document() = nil after LoadURL(file://)")
	} else if doc.Title() != "file" {
		t.Errorf("Document().Title() = %q, want %q", doc.Title(), "file")
	}
}

// TestWebViewLoadURLDataURI verifies that data: URIs are loaded correctly.
func TestWebViewLoadURLDataURI(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadURL("data:text/html,<h1>Hello</h1>"); err != nil {
		t.Fatalf("LoadURL(data: text) failed: %v", err)
	}
	doc := wv.MainFrame().Document()
	if doc == nil {
		t.Fatal("Document() = nil after data: URI")
	}
	body := doc.Body()
	if body == nil {
		t.Fatal("Body() = nil after data: URI")
	}
	t.Logf("data: URI loaded OK: body has %d children", len(body.ChildNodes()))
	if err := wv.LoadURL("data:"); err == nil {
		t.Error("LoadURL(data:) expected error for empty data URI")
	}
}

// TestWebViewLoadURLEmpty verifies that an empty URL returns an error.
func TestWebViewLoadURLEmpty(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadURL(""); err == nil {
		t.Error("LoadURL('') expected error")
	}
}

// TestWebViewLoadURLUnsupportedScheme verifies that unsupported URL schemes
// return a descriptive error.
func TestWebViewLoadURLUnsupportedScheme(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadURL("ftp://files.example.com/data.html"); err == nil {
		t.Error("LoadURL(ftp://) expected error")
	} else if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("LoadURL(ftp://) error = %q, want 'unsupported URL scheme'", err)
	}
}

// TestWebViewRenderAfterResize verifies that Render after a Resize uses the new
// viewport dimensions for the output buffer.
func TestWebViewRenderAfterResize(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><body style=\"background-color: black;\"><p>x</p></body></html>"); err != nil {
		t.Fatalf("LoadHTML failed: %v", err)
	}
	wv.Resize(7, 5)
	pixels, err := wv.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(pixels) != 7*5*4 {
		t.Errorf("pixel buffer length = %d, want %d", len(pixels), 7*5*4)
	}
}

func TestWebViewConsoleOutputEmpty(t *testing.T) {
	wv := NewWebView()
	if got := wv.ConsoleOutput(); got != "" {
		t.Errorf("ConsoleOutput() before any JS = %q, want empty", got)
	}
}

func TestWebViewMultipleLoadHTML(t *testing.T) {
	wv := NewWebView()
	if err := wv.LoadHTML("<html><head><title>First</title></head><body><p>a</p></body></html>"); err != nil {
		t.Fatalf("first LoadHTML failed: %v", err)
	}
	doc1 := wv.MainFrame().Document()
	if doc1.Title() != "First" {
		t.Errorf("title after first load = %q, want %q", doc1.Title(), "First")
	}
	if err := wv.LoadHTML("<html><head><title>Second</title></head><body><p>b</p></body></html>"); err != nil {
		t.Fatalf("second LoadHTML failed: %v", err)
	}
	doc2 := wv.MainFrame().Document()
	if doc2.Title() != "Second" {
		t.Errorf("title after second load = %q, want %q", doc2.Title(), "Second")
	}
}

func TestWebViewResizeBeforeLoad(t *testing.T) {
	wv := NewWebView()
	wv.Resize(640, 480)
	if wv.Width() != 640 || wv.Height() != 480 {
		t.Errorf("resize before load = %dx%d, want 640x480", wv.Width(), wv.Height())
	}
}
