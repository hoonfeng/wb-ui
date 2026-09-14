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

	// ScriptEngine is an optional callback invoked by the HTML parser when
	// an inline <script> element is encountered. It is set by WebView.LoadHTML.
	ScriptEngine func(code string) error
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

// fetchURL 取 URL 的内容（data: / http(s): / file:）。
func fetchURL(rawURL string) (string, error) {
	body, _, err := fetchURLWithFinalURL(rawURL)
	return body, err
}

// fetchURLWithFinalURL 同 fetchURL，但额外返回**最终 URL**：HTTP 重定向后
// 它是响应实际所在的地址。
//
// 为什么需要它：浏览器里文档的 base URL 是**重定向之后的**地址，相对引用
// 据此解析。真实站点「http → https」「补尾斜杠」「跳到 /index.html」都是
// 常态——只用请求 URL 当基准，`http://host` 被跳转到 `https://host/` 后，
// 页面里的 `style.css` 会被解析回 `http://host/style.css`（错的地址）。
//
// 非 HTTP（data:/file:）与无重定向时，最终 URL 等于入参。
func fetchURLWithFinalURL(rawURL string) (body, finalURL string, err error) {
	r, err := fetchResource(rawURL)
	return r.content, r.finalURL, err
}

// resourceResponse 是一次外部取内容的完整结果：内容 + 取内容协议层的元数据。
//
// 为什么要元数据：① MIME 检查——`<link rel=stylesheet>` / `<script src>` 在
// 响应带 `X-Content-Type-Options: nosniff` 时按类型**拒绝**（与浏览器一致）；
// ② 缓存策略——`Cache-Control: no-store/no-cache`、`Pragma: no-cache` 不缓存。
// 取内容层只**报告**这些头，怎么用交给各用途的消费端（浏览器同样把 MIME 检查
// 放在消费端：同一个响应可能是 CSS、JS 或 JSON）。
type resourceResponse struct {
	content     string
	finalURL    string
	contentType string // 原始 Content-Type 头（没有时为空串）
	nosniff     bool   // X-Content-Type-Options: nosniff
	noStore     bool   // Cache-Control: no-store/no-cache 或 Pragma: no-cache
}

// fetchResource 取 URL 的内容与元数据（data: / http(s): / file:）。
func fetchResource(rawURL string) (resourceResponse, error) {
	if rawURL == "" {
		return resourceResponse{}, errEmptyURL
	}
	// data: URI — inline data.
	if strings.HasPrefix(rawURL, "data:") {
		body, err := fetchDataURI(rawURL)
		return resourceResponse{content: body, finalURL: rawURL, contentType: dataURIMediaType(rawURL)}, err
	}
	// http:// or https:// — network request.
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return fetchHTTPResource(rawURL)
	}
	// file:// — local filesystem（没有响应头：Content-Type 留空，消费端按
	// 「无类型」宽松处理，与浏览器对 file 的推断近似）。
	if strings.HasPrefix(rawURL, "file://") {
		body, err := fetchFile(rawURL)
		return resourceResponse{content: body, finalURL: rawURL}, err
	}
	return resourceResponse{finalURL: rawURL}, fmt.Errorf("webkit: unsupported URL scheme: %s", rawURL)
}

// fetchHTTP 取 http(s) 内容（只返回内容与最终 URL）。
func fetchHTTP(rawURL string) (string, string, error) {
	r, err := fetchHTTPResource(rawURL)
	return r.content, r.finalURL, err
}

// fetchHTTPResource performs an HTTP GET request with a 30-second timeout,
// follows redirects, limits the response body to 10 MB, and returns the
// content together with the response metadata (Content-Type / nosniff /
// Cache-Control) that the resource consumers need.
func fetchHTTPResource(rawURL string) (resourceResponse, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		// CheckRedirect follows redirects (default behaviour, up to 10).
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return resourceResponse{finalURL: rawURL}, fmt.Errorf("webkit: GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resourceResponse{finalURL: rawURL}, fmt.Errorf("webkit: GET %s returned %s", rawURL, resp.Status)
	}

	res := resourceResponse{
		finalURL:    rawURL,
		contentType: resp.Header.Get("Content-Type"),
	}
	// 重定向后的最终 URL（无重定向时等于 rawURL）：调用方用它当文档基地址。
	if resp.Request != nil && resp.Request.URL != nil {
		res.finalURL = resp.Request.URL.String()
	}
	res.nosniff = strings.EqualFold(strings.TrimSpace(resp.Header.Get("X-Content-Type-Options")), "nosniff")
	cc := strings.ToLower(resp.Header.Get("Cache-Control"))
	pragma := strings.ToLower(resp.Header.Get("Pragma"))
	res.noStore = strings.Contains(cc, "no-store") || strings.Contains(cc, "no-cache") ||
		strings.Contains(pragma, "no-cache")

	// Content-Type：只记录提示，**不**阻断。
	//
	// 这里原先返回 error（注释写的是 "warn but still return the content"，
	// 实现却返回了空串）——fetch 层并不知道调用方的用途：同一个响应可能是
	// <link rel=stylesheet> 的 text/css、<script src> 的 application/
	// javascript 或 fetch() 的 application/json。按「非 HTML 即失败」判断
	// 会让真实网络下的外部 CSS/JS **全部**加载失败（相对 URL 修好后立刻
	// 暴露：服务器收到了请求、内容也拿到了，样式却不生效）。
	// 浏览器把 MIME 检查放在各用途的消费端（nosniff 下 <script> 拒非 JS MIME
	// 等）——那一层现在由 loadExternalResource 按用途实现（见 resource_cache.go），
	// 这里内容照常返回。
	if ct := res.contentType; ct != "" && !isHTMLCompatible(parseMediaType(ct)) {
		page.Logf("fetchHTTP", "%s: Content-Type %q（非 HTML，内容照常返回）", rawURL, ct)
	}

	// Read with size limit.
	limitedReader := io.LimitReader(resp.Body, maxBodySize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return res, fmt.Errorf("webkit: reading %s: %w", rawURL, err)
	}
	if int64(len(body)) > maxBodySize {
		return res, fmt.Errorf("webkit: %s: %w", rawURL, errOversizedResponse)
	}

	res.content = string(body)
	return res, nil
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

// dataURIMediaType 返回 data: URL 的媒体类型（RFC 2397），缺省按规范是
// `text/plain;charset=US-ASCII`。
func dataURIMediaType(uri string) string {
	rest := strings.TrimPrefix(uri, "data:")
	if i := strings.IndexByte(rest, ','); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, ";base64")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "text/plain;charset=US-ASCII"
	}
	return rest
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
