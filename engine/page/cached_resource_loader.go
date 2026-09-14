// Translation of: Source/WebCore/loader/cache/CachedResourceLoader.h
//                  Source/WebCore/loader/cache/CachedResourceLoader.cpp
// Completeness: 40%
// Simplifications:
//   - no preload scanner integration; all resource requests are initiated
//     synchronously from the parser (or from user code)
//   - no request priority / loading priority / request originator tracking
//   - no CORS / cross-origin access checks
//   - no content security policy / mixed content checking
//   - no resource load timings / performance entries
//   - no synchronous load mode (all loads are asynchronous)
//   - no request coalescing / duplicate request deduplication beyond the
//     MemoryCache lookup: concurrent requests for the same URL share the
//     same Pending CachedResource via AddClient

package page

import (
	"net/url"
	"strings"
	"sync"
)

// CachedResourceLoader is the Go translation of WebCore::CachedResourceLoader.
// It is the central entry point for all resource loading within a page (or
// frame). Given a URL and a resource type, it:
//
//  1. Checks the MemoryCache for an existing CachedResource.
//  2. If found and Loaded, notifies the client immediately.
//  3. If found and Pending, registers the client to receive the finish
//     notification when the in-flight load completes.
//  4. If not found, creates a new CachedResource (Pending), registers the
//     client, inserts it into the cache, and starts a ResourceHandle to
//     fetch the data asynchronously.
//
// All network responses are delivered asynchronously via the
// CachedResourceClient interface on the CachedResource.
type CachedResourceLoader struct {
	// documentURL is the URL of the owning document, used to resolve
	// relative paths in resource URLs.
	documentURL string

	// cache is the MemoryCache instance backing this loader. When nil,
	// DefaultMemoryCache is used.
	cache *MemoryCache

	// mu guards the loadInProgress flag and any metadata that may be
	// accessed from multiple goroutines (callbacks from ResourceHandle).
	mu             sync.Mutex
	loadInProgress bool

	// pendings maps URL → *CachedResource for in-flight requests that were
	// initiated by this loader but have not yet completed. This complements
	// the MemoryCache by allowing the loader to attach clients to resources
	// that are already in the cache in Pending state.
	pendings map[string]*CachedResource
}

// NewCachedResourceLoader creates a CachedResourceLoader for the given
// document URL. The documentURL is used to resolve relative resource URLs.
// The cache defaults to DefaultMemoryCache.
func NewCachedResourceLoader(documentURL string) *CachedResourceLoader {
	return &CachedResourceLoader{
		documentURL: documentURL,
		cache:       DefaultMemoryCache,
		pendings:    make(map[string]*CachedResource),
	}
}

// SetCache replaces the MemoryCache instance used by this loader.
func (crl *CachedResourceLoader) SetCache(cache *MemoryCache) {
	crl.mu.Lock()
	defer crl.mu.Unlock()
	crl.cache = cache
}

// RequestResource is the central resource request method. Given a URL and
// resource type, it returns a CachedResource (possibly Pending). The client
// is registered and will receive NotifyFinished when the resource completes
// (or fails). If a resource for this URL is already in the cache, it is
// reused; otherwise a new HTTP (or file) load is initiated.
func (crl *CachedResourceLoader) RequestResource(urlStr string, resourceType ResourceType, client CachedResourceClient) *CachedResource {
	// Resolve relative URLs against the document base URL.
	absURL := crl.resolveURL(urlStr)
	if absURL == "" {
		absURL = urlStr
	}

	// Determine which cache to use.
	cache := crl.cache
	if cache == nil {
		cache = DefaultMemoryCache
	}

	// 1. Check the cache.
	if res := cache.Get(absURL); res != nil {
		// Already loaded — notify client immediately.
		if res.Status() == CachedResourceStatusLoaded {
			if client != nil {
				client.NotifyFinished(res)
			}
			return res
		}
		// Still pending — register the client.
		if res.Status() == CachedResourceStatusPending {
			if client != nil {
				res.AddClient(client)
			}
			return res
		}
		// Error state — still notify the client so it knows the resource
		// failed.
		if client != nil {
			client.NotifyFinished(res)
		}
		return res
	}

	// 2. Not in cache — create a new Pending resource.
	res := NewCachedResource(absURL, resourceType)
	if client != nil {
		res.AddClient(client)
	}

	// Add to cache before starting the load, so that concurrent requests
	// for the same URL find it in Pending state.
	cache.Add(res)

	// Track in pendings for cleanup.
	crl.mu.Lock()
	crl.pendings[absURL] = res
	crl.mu.Unlock()

	// 3. Start the asynchronous load.
	crl.startLoad(res)

	return res
}

// LoadStylesheet is a convenience method for loading a CSS stylesheet.
// Equivalent to RequestResource(url, ResourceTypeStylesheet, client).
func (crl *CachedResourceLoader) LoadStylesheet(url string, client CachedResourceClient) *CachedResource {
	return crl.RequestResource(url, ResourceTypeStylesheet, client)
}

// LoadScript is a convenience method for loading a JavaScript file.
// Equivalent to RequestResource(url, ResourceTypeScript, client).
func (crl *CachedResourceLoader) LoadScript(url string, client CachedResourceClient) *CachedResource {
	return crl.RequestResource(url, ResourceTypeScript, client)
}

// LoadImage is a convenience method for loading an image.
// Equivalent to RequestResource(url, ResourceTypeImage, client).
func (crl *CachedResourceLoader) LoadImage(url string, client CachedResourceClient) *CachedResource {
	return crl.RequestResource(url, ResourceTypeImage, client)
}

// resolveURL resolves a potentially relative URL against the loader's
// document base URL. If the URL is already absolute (has a scheme), it is
// returned as-is. Empty strings return empty.
func (crl *CachedResourceLoader) resolveURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Already absolute.
	if strings.HasPrefix(rawURL, "http://") ||
		strings.HasPrefix(rawURL, "https://") ||
		strings.HasPrefix(rawURL, "file://") ||
		strings.HasPrefix(rawURL, "data:") {
		return rawURL
	}

	// Absolute path (starts with /): resolve against the document URL's origin.
	if strings.HasPrefix(rawURL, "/") {
		if crl.documentURL == "" {
			return rawURL
		}
		base, err := url.Parse(crl.documentURL)
		if err != nil {
			return rawURL
		}
		base.Path = rawURL
		return base.String()
	}

	// Relative path: resolve against the document URL's directory.
	if crl.documentURL == "" {
		return rawURL
	}

	// If the document URL is file://, resolve by string concatenation
	// to avoid issues with url.Parse on Windows (file://C:\... is not a
	// valid RFC 8089 URL but is how ResourceHandle expects it).
	if strings.HasPrefix(crl.documentURL, "file://") {
		base := crl.documentURL
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		return base + rawURL
	}

	// For http:// and https:// URLs, use proper URL resolution.
	base, err := url.Parse(crl.documentURL)
	if err != nil {
		return rawURL
	}
	rel, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	resolved := base.ResolveReference(rel)
	return resolved.String()
}

// startLoad creates a ResourceHandle for the given CachedResource and
// initiates the asynchronous load. The loader acts as the
// ResourceHandleClient, forwarding response metadata and data to the
// CachedResource.
func (crl *CachedResourceLoader) startLoad(res *CachedResource) {
	req := NewResourceRequest(res.URL(), "GET")
	handle := NewResourceHandle(req, &resourceHandleBridge{
		loader: crl,
		resource: res,
	})
	if err := handle.Start(); err != nil {
		res.SetError(err)
		crl.cleanupPending(res.URL())
	}
}

// cleanupPending removes a URL from the pending tracking map.
func (crl *CachedResourceLoader) cleanupPending(url string) {
	crl.mu.Lock()
	delete(crl.pendings, url)
	crl.mu.Unlock()
}

// resourceHandleBridge implements ResourceHandleClient on behalf of a
// CachedResourceLoader, bridging the async network callbacks into the
// CachedResource state machine. A separate struct is used per request so
// that each in-flight load has its own isolated state.
type resourceHandleBridge struct {
	loader   *CachedResourceLoader
	resource *CachedResource
	loadedData []byte
}

// DidReceiveResponse implements ResourceHandleClient. It transfers response
// metadata (MIME type, encoding) to the CachedResource.
func (b *resourceHandleBridge) DidReceiveResponse(handle *ResourceHandle, resp *ResourceResponse) {
	if resp == nil {
		return
	}
	b.resource.SetMimeType(resp.MimeType)

	// Extract encoding from Content-Type header.
	if ct := resp.Headers.Get("Content-Type"); ct != "" {
		// Look for charset=... in the Content-Type.
		_, params := parseContentType(ct)
		if enc, ok := params["charset"]; ok {
			b.resource.SetEncoding(enc)
		}
	}
}

// DidReceiveData implements ResourceHandleClient. It accumulates the
// received data in the bridge's buffer.
func (b *resourceHandleBridge) DidReceiveData(handle *ResourceHandle, data []byte) {
	b.loadedData = append(b.loadedData, data...)
}

// DidFinishLoading implements ResourceHandleClient. It pushes the complete
// data into the CachedResource via SetData, which transitions status to
// Loaded and notifies all registered clients.
func (b *resourceHandleBridge) DidFinishLoading(handle *ResourceHandle) {
	b.resource.SetData(b.loadedData)
	b.loader.cleanupPending(b.resource.URL())
}

// DidFail implements ResourceHandleClient. It pushes the error into the
// CachedResource via SetError, which transitions status to Error and
// notifies all registered clients.
func (b *resourceHandleBridge) DidFail(handle *ResourceHandle, err error) {
	b.resource.SetError(err)
	b.loader.cleanupPending(b.resource.URL())
}

// parseContentType parses a Content-Type header value into the media type
// and a parameter map. Handles values like "text/html; charset=utf-8".
func parseContentType(contentType string) (mediaType string, params map[string]string) {
	params = make(map[string]string)

	parts := strings.SplitN(contentType, ";", 2)
	mediaType = strings.TrimSpace(strings.ToLower(parts[0]))

	if len(parts) < 2 {
		return
	}

	for _, part := range strings.Split(parts[1], ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(strings.ToLower(kv[0]))
			val := strings.Trim(strings.TrimSpace(kv[1]), "\"'")
			params[key] = val
		}
	}
	return
}
