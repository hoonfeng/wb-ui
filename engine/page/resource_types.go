// Translation of: Source/WebCore/platform/network/ResourceRequest.h
//                  Source/WebCore/platform/network/ResourceRequestBase.h
//                  Source/WebCore/platform/network/ResourceResponse.h
//                  Source/WebCore/platform/network/ResourceResponseBase.h
// Completeness: 40%
// Simplifications:
//   - no platform-specific subclasses (CFNetwork / curl / soup backends); the Go
//     structs hold all fields directly and are used uniformly across all platforms
//   - no cache policy / requester type / redirect count / priority / timeout fields
//     on ResourceRequest beyond the basics (URL, method, headers, body)
//   - no ResourceResponseBase base class; ResourceResponse is a single flat struct
//   - no source/tainting/cache-state on ResourceResponse
//   - ResourceType is a plain Go enum (not a bitmask) for the common resource kinds

package page

import (
	"io"
	"net/http"
	"strings"
)

// ResourceType identifies the kind of resource being loaded, mirroring
// WebCore::CachedResource::Type. This determines which CachedResource subclass
// is created and how the data is processed (CSS vs script vs image vs font).
type ResourceType int

const (
	// ResourceTypeMainResource is the top-level HTML document load.
	ResourceTypeMainResource ResourceType = iota
	// ResourceTypeStylesheet is a CSS stylesheet (<link rel="stylesheet"> or @import).
	ResourceTypeStylesheet
	// ResourceTypeScript is a JavaScript file (<script src>).
	ResourceTypeScript
	// ResourceTypeImage is an image (<img src>, background-image: url(), etc.).
	ResourceTypeImage
	// ResourceTypeFont is a font file (@font-face src: url()).
	ResourceTypeFont
	// ResourceTypeRaw is a raw resource (fetch(), XHR).
	ResourceTypeRaw
	// ResourceTypeSVGDocument is an SVG document loaded as an image or embedded.
	ResourceTypeSVGDocument
)

// String returns the human-readable name of the resource type, matching WebKit's
// CachedResource::typeName() convention.
func (t ResourceType) String() string {
	switch t {
	case ResourceTypeMainResource:
		return "MainResource"
	case ResourceTypeStylesheet:
		return "Stylesheet"
	case ResourceTypeScript:
		return "Script"
	case ResourceTypeImage:
		return "Image"
	case ResourceTypeFont:
		return "Font"
	case ResourceTypeRaw:
		return "Raw"
	case ResourceTypeSVGDocument:
		return "SVGDocument"
	default:
		return "Unknown"
	}
}

// ResourceRequest is the Go translation of WebCore::ResourceRequest (which inherits
// from ResourceRequestBase). It represents an HTTP request to be sent to the network
// layer: the URL to fetch, the HTTP method, request headers and an optional body.
type ResourceRequest struct {
	// URL is the absolute URL being requested, mirroring ResourceRequestBase::url().
	URL string
	// Method is the HTTP method (GET, POST, HEAD, etc.), defaulting to "GET".
	// Mirrors ResourceRequestBase::httpMethod().
	Method string
	// Headers holds the HTTP request headers, mirroring
	// ResourceRequestBase::httpHeaderFields().
	Headers http.Header
	// Body is an optional reader for the request body (POST / PUT payloads),
	// mirroring ResourceRequestBase::httpBody(). May be nil.
	Body io.Reader
}

// NewResourceRequest creates a ResourceRequest for the given URL and HTTP method.
// If method is empty it defaults to "GET". The headers map is initialised.
func NewResourceRequest(url, method string) *ResourceRequest {
	if method == "" {
		method = "GET"
	}
	return &ResourceRequest{
		URL:     url,
		Method:  method,
		Headers: make(http.Header),
	}
}

// SetHeader sets a request header, mirroring ResourceRequestBase::setHTTPHeaderField().
func (r *ResourceRequest) SetHeader(name, value string) {
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}
	r.Headers.Set(name, value)
}

// GetHeader returns the value of a request header, mirroring
// ResourceRequestBase::httpHeaderField(). Returns "" when the header is not set.
func (r *ResourceRequest) GetHeader(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers.Get(name)
}

// ResourceResponse is the Go translation of WebCore::ResourceResponse (which inherits
// from ResourceResponseBase). It represents the response received from the network
// after issuing a ResourceRequest: the HTTP status code, response headers, content
// length and MIME type.
type ResourceResponse struct {
	// URL is the final URL after any redirects, mirroring
	// ResourceResponseBase::url(). May differ from the request URL.
	URL string
	// StatusCode is the HTTP response status code (200, 304, 404, etc.),
	// mirroring ResourceResponseBase::httpStatusCode(). 0 means no HTTP response.
	StatusCode int
	// StatusText is the HTTP status text ("OK", "Not Found", etc.),
	// mirroring ResourceResponseBase::httpStatusText().
	StatusText string
	// Headers holds the HTTP response headers, mirroring
	// ResourceResponseBase::httpHeaderFields().
	Headers http.Header
	// ContentLength is the value of the Content-Length header, or -1 if unknown.
	// Mirrors ResourceResponseBase::expectedContentLength().
	ContentLength int64
	// MimeType is the MIME type extracted from the Content-Type header,
	// mirroring ResourceResponseBase::mimeType(). Lowercased, without parameters.
	MimeType string
}

// NewResourceResponse creates an empty ResourceResponse. Headers is initialised;
// ContentLength defaults to -1 (unknown).
func NewResourceResponse() *ResourceResponse {
	return &ResourceResponse{
		Headers:       make(http.Header),
		ContentLength: -1,
	}
}

// SetContentType parses the Content-Type header value and sets both the header
// and the MimeType field. The MimeType is the media type portion (e.g. "text/html"
// from "text/html; charset=utf-8"), mirroring ResourceResponseBase::setMimeType().
func (r *ResourceResponse) SetContentType(contentType string) {
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}
	r.Headers.Set("Content-Type", contentType)

	// Extract the media type (before ';').
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		r.MimeType = strings.TrimSpace(strings.ToLower(contentType[:idx]))
	} else {
		r.MimeType = strings.TrimSpace(strings.ToLower(contentType))
	}
}

// IsHTTP reports whether this response came from an actual HTTP server as
// opposed to a file:// or data: URI. It returns true when StatusCode is non-zero,
// mirroring ResourceResponseBase::isHTTP().
func (r *ResourceResponse) IsHTTP() bool {
	return r.StatusCode > 0
}
