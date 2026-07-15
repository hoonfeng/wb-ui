// Translation of: Source/WebKit/WebProcess/WebPage/WebFrame.h
//                  Source/WebKit/WebProcess/WebPage/WebFrame.cpp
//                  Source/WebKit/UIProcess/API/C/WKFrame.cpp
// Completeness: 45%
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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// fetchURL loads a resource by URL and returns its content as a string. It supports
// the following schemes:
//
//   - http:// / https://  — HTTP GET with 30s timeout, 10MB body limit, follows
//     redirects. Non-2xx status codes return an error.
//   - file://             — reads from the local filesystem. Windows paths like
//     file:///C:/path/file.html are handled correctly.
//   - data:               — inline data URIs per RFC 2397 (data:text/html,...).
//
// Content-Type checking is lenient: non-HTML content types produce a warning logged
// via the returned error but the content is still returned.
//
// Max response body: 10MB.
var (
	errOversizedResponse = errors.New("webkit: response body exceeds 10MB limit")
	errEmptyURL          = errors.New("webkit: empty URL")
	maxBodySize          int64 = 10 * 1024 * 1024 // 10 MB
)

func fetchURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", errEmptyURL
	}

	// data: URI — inline data.
	if strings.HasPrefix(rawURL, "data:") {
		return fetchDataURI(rawURL)
	}

	// http:// or https:// — network request.
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return fetchHTTP(rawURL)
	}

	// file:// — local filesystem.
	if strings.HasPrefix(rawURL, "file://") {
		return fetchFile(rawURL)
	}

	return "", fmt.Errorf("webkit: unsupported URL scheme: %s", rawURL)
}

// fetchHTTP performs an HTTP GET request with a 30-second timeout, follows
// redirects, limits the response body to 10 MB, and checks Content-Type.
func fetchHTTP(rawURL string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		// CheckRedirect follows redirects (default behaviour, up to 10).
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("webkit: GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("webkit: GET %s returned %s", rawURL, resp.Status)
	}

	// Check Content-Type (lenient: warn but still return the content).
	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		ctBase := parseMediaType(ct)
		if !isHTMLCompatible(ctBase) {
			// Non-HTML content — warn but continue.
			return "", fmt.Errorf("webkit: GET %s: unexpected Content-Type %q (content returned anyway)", rawURL, ct)
		}
	}

	// Read with size limit.
	limitedReader := io.LimitReader(resp.Body, maxBodySize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("webkit: reading %s: %w", rawURL, err)
	}
	if int64(len(body)) > maxBodySize {
		return "", fmt.Errorf("webkit: %s: %w", rawURL, errOversizedResponse)
	}

	return string(body), nil
}

// fetchFile reads a file:// URL from the local filesystem. It handles the
// platform-specific path conversion, including Windows paths like
// file:///C:/path/file.html → C:\path\file.html.
func fetchFile(rawURL string) (string, error) {
	// Parse as a proper URL to handle various forms correctly.
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("webkit: parsing file URL %s: %w", rawURL, err)
	}

	// Get the file path from the URL. url.Parse handles the conversion:
	//   file:///path/to/file  →  /path/to/file  (Unix)
	//   file:///C:/path/file  →  /C:/path/file  (Windows, leading slash)
	//   file://localhost/path →  /path (Host == "localhost")
	filePath := u.Path

	// On Windows, strip the leading slash from paths like /C:/... → C:/...
	if len(filePath) >= 3 && filePath[0] == '/' && filePath[2] == ':' {
		filePath = filePath[1:] // "/C:/path" → "C:/path"
	}

	// Clean the path using the OS path separator.
	filePath = filepath.FromSlash(filePath)

	if filePath == "" {
		return "", fmt.Errorf("webkit: empty file path in %s", rawURL)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("webkit: opening %s: %w", rawURL, err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("webkit: reading %s: %w", rawURL, err)
	}
	return string(b), nil
}

// fetchDataURI parses a data: URI (RFC 2397) and returns the decoded content.
// Supported form: data:[<mediatype>][;base64],<data>
func fetchDataURI(rawURL string) (string, error) {
	// Strip the "data:" prefix.
	rest := rawURL[5:]
	if rest == "" {
		return "", fmt.Errorf("webkit: empty data URI")
	}

	// Split at the first comma to separate the header from the data.
	commaIdx := strings.IndexByte(rest, ',')
	if commaIdx < 0 {
		return "", fmt.Errorf("webkit: data URI missing comma separator")
	}

	header := rest[:commaIdx]
	data := rest[commaIdx+1:]

	// Check for base64 encoding.
	isBase64 := strings.HasSuffix(header, ";base64")

	// Extract media type (default is text/plain;charset=US-ASCII per RFC 2397).
	mediaType := header
	if isBase64 {
		mediaType = strings.TrimSuffix(header, ";base64")
	}
	if mediaType == "" {
		mediaType = "text/plain;charset=US-ASCII"
	}

	_ = mediaType // media type is informational; we return the decoded data.

	if isBase64 {
		// Decode base64.
		decoded, err := decodeBase64(data)
		if err != nil {
			return "", fmt.Errorf("webkit: decoding base64 data URI: %w", err)
		}
		return string(decoded), nil
	}

	// URL-decode the percent-encoded data (RFC 2397 says the data is
	// URL-encoded unless base64).
	decoded, err := url.QueryUnescape(data)
	if err != nil {
		return "", fmt.Errorf("webkit: decoding data URI: %w", err)
	}
	return decoded, nil
}

// parseMediaType extracts the base media type from a Content-Type header value,
// stripping parameters like charset=utf-8.
func parseMediaType(ct string) string {
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		return strings.TrimSpace(strings.ToLower(ct[:idx]))
	}
	return strings.TrimSpace(strings.ToLower(ct))
}

// isHTMLCompatible returns true if the media type is HTML-compatible
// (text/html, text/plain, application/xhtml+xml, etc.).
func isHTMLCompatible(mediaType string) bool {
	switch mediaType {
	case "text/html", "text/plain", "application/xhtml+xml",
		"application/xml", "text/xml", "text/xhtml",
		"text/htm", "application/html":
		return true
	}
	return false
}

// decodeBase64 decodes a base64-encoded string. It handles both standard
// and URL-safe base64 with optional padding.
func decodeBase64(s string) ([]byte, error) {
	// Try standard base64 first.
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	// Fall back to URL-safe base64 (no padding variant).
	decoded, err = base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	// Try standard with padding relaxed.
	decoded, err = base64.RawStdEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("webkit: base64 decode error (tried 3 variants)")
}
