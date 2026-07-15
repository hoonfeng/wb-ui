// Translation of: Source/WebCore/platform/network/ResourceHandle.h
//                  Source/WebCore/platform/network/ResourceHandle.cpp
//                  Source/WebCore/platform/network/ResourceHandleClient.h
// Completeness: 45%
// Simplifications:
//   - single goroutine-per-request model; WebKit delegates to a platform run loop
//     (CFNetwork / curl multi-handle / soup session) for async I/O
//   - no authentication challenge / credential storage
//   - no redirect handling (follows redirects automatically via net/http)
//   - no caching layer integration (the caller is responsible for populating
//     the MemoryCache via CachedResourceLoader)
//   - no synchronous load mode / loading priority / throttling
//   - no platform-specific subclasses; the Go net/http Client is used on all
//     platforms, with file:// URLs handled locally via os.ReadFile

package page

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// resourceChunkSize is the size of each data chunk delivered via
// DidReceiveData, matching a typical network buffer size (32 KB).
const resourceChunkSize = 32 * 1024

// ResourceHandleClient is the Go translation of WebCore::ResourceHandleClient.
// An object that wishes to receive progress and completion callbacks from a
// ResourceHandle implements this interface. All methods are called from the
// handle's background goroutine; the caller is responsible for forwarding
// results to the appropriate thread (typically via channels or the main loop).
type ResourceHandleClient interface {
	// DidReceiveResponse is called once when the HTTP response headers have
	// been received. The response parameter contains the status code, headers
	// and MIME type.
	DidReceiveResponse(handle *ResourceHandle, response *ResourceResponse)

	// DidReceiveData is called zero or more times as chunks of the response
	// body arrive. data is a slice of bytes valid only during the call;
	// clients that need it beyond the callback must copy it.
	DidReceiveData(handle *ResourceHandle, data []byte)

	// DidFinishLoading is called when the entire response body has been
	// delivered. No further callbacks will be made for this handle.
	DidFinishLoading(handle *ResourceHandle)

	// DidFail is called when the request encounters an error (network error,
	// timeout, cancellation, or file I/O error). No further callbacks will
	// be made for this handle.
	DidFail(handle *ResourceHandle, err error)
}

// ResourceHandle is the Go translation of WebCore::ResourceHandle. It
// represents a single HTTP (or file://) request in flight. Callers create a
// handle with NewResourceHandle, optionally configure it, then call Start()
// to begin the asynchronous load. Progress is reported via the
// ResourceHandleClient interface.
//
// Each handle may be started at most once. Cancellation is supported via
// Cancel(), which aborts the underlying HTTP request.
type ResourceHandle struct {
	// request is the original ResourceRequest, captured at creation time.
	request *ResourceRequest

	// client receives progress and completion callbacks. May be nil; in that
	// case the handle still performs the request but discards all data and
	// results.
	client ResourceHandleClient

	// response accumulates the response metadata received from the server.
	// It is populated during DidReceiveResponse and remains available after
	// the handle completes via the Response() accessor.
	response *ResourceResponse

	// loadedData accumulates the full response body if the client does not
	// consume individual DidReceiveData chunks. It is used by Response()
	// to return the complete payload.
	loadedData []byte

	// cancelChan is closed to signal that the request should be cancelled.
	cancelChan chan struct{}

	// done gates terminal callbacks (DidFinishLoading / DidFail) so they
	// are dispatched exactly once, even in edge cases where the HTTP
	// response and a cancellation race.
	doneOnce sync.Once

	// done indicates that the handle has reached a terminal state (finished,
	// failed or cancelled). Used internally to skip redundant callbacks.
	done bool
}

// NewResourceHandle creates a ResourceHandle for the given request and
// client. The handle is not started until Start() is called. The client
// may be nil if the caller only needs to inspect the final response via
// Response() after a synchronous-style wait.
func NewResourceHandle(request *ResourceRequest, client ResourceHandleClient) *ResourceHandle {
	return &ResourceHandle{
		request:    request,
		client:     client,
		cancelChan: make(chan struct{}),
	}
}

// Start begins the asynchronous load. It launches a background goroutine
// that performs the HTTP request (or reads a local file for file:// URLs).
// Progress is reported via the configured ResourceHandleClient.
//
// Start returns an error only if the request could not be initiated (e.g.
// an invalid URL). Network-level errors are reported via DidFail.
func (rh *ResourceHandle) Start() error {
	if rh.request == nil {
		return fmt.Errorf("resource handle: request is nil")
	}
	if rh.done {
		return fmt.Errorf("resource handle: already completed")
	}

	url := rh.request.URL

	// Handle file:// URLs synchronously in a goroutine.
	if strings.HasPrefix(url, "file://") {
		go rh.doFileLoad(url)
		return nil
	}

	// For all other schemes, use net/http.
	go rh.doHTTPLoad()
	return nil
}

// Cancel aborts an in-flight request. It is safe to call multiple times
// and safe to call after the handle has already completed.
func (rh *ResourceHandle) Cancel() {
	rh.doneOnce.Do(func() {
		rh.done = true
		close(rh.cancelChan)
	})
}

// Request returns the original ResourceRequest, mirroring
// ResourceHandle::firstRequest().
func (rh *ResourceHandle) Request() *ResourceRequest {
	return rh.request
}

// Response returns the ResourceResponse accumulated during the load, or
// nil if no response has been received yet.
func (rh *ResourceHandle) Response() *ResourceResponse {
	return rh.response
}

// doHTTPLoad executes the HTTP request on the current goroutine. It builds
// a context from cancelChan for cancellation support, sends the request via
// net/http.Client, and drives the ResourceHandleClient callbacks.
func (rh *ResourceHandle) doHTTPLoad() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Monitor the cancel channel in a separate goroutine so that
	// Cancel() from another goroutine interrupts the context.
	go func() {
		select {
		case <-rh.cancelChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	req, err := http.NewRequestWithContext(ctx, rh.request.Method, rh.request.URL, rh.request.Body)
	if err != nil {
		rh.fail(err)
		return
	}
	// Copy headers from the ResourceRequest.
	for key, values := range rh.request.Headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// Check if the error was due to cancellation.
		select {
		case <-rh.cancelChan:
			rh.fail(fmt.Errorf("resource handle: cancelled"))
		default:
			rh.fail(fmt.Errorf("resource handle: %w", err))
		}
		return
	}
	defer resp.Body.Close()

	// Build the ResourceResponse from the HTTP response.
	response := NewResourceResponse()
	response.URL = resp.Request.URL.String()
	response.StatusCode = resp.StatusCode
	response.StatusText = strings.TrimPrefix(resp.Status, fmt.Sprintf("%d ", resp.StatusCode))
	response.Headers = resp.Header
	response.ContentLength = resp.ContentLength

	// Extract MIME type from Content-Type.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		response.SetContentType(ct)
	}

	rh.response = response

	// Notify the client that headers are available.
	if rh.client != nil {
		rh.client.DidReceiveResponse(rh, response)
	}

	// Read the body in chunks and deliver via DidReceiveData.
	buf := make([]byte, resourceChunkSize)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			rh.loadedData = append(rh.loadedData, chunk...)
			if rh.client != nil {
				rh.client.DidReceiveData(rh, chunk)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			rh.fail(fmt.Errorf("resource handle: read error: %w", readErr))
			return
		}
	}

	// Success.
	rh.finish()
}

// doFileLoad reads a local file for file:// URLs and drives the client
// callbacks as if it were an HTTP response.
func (rh *ResourceHandle) doFileLoad(url string) {
	// Strip the file:// prefix to get the local path.
	path := strings.TrimPrefix(url, "file://")

	data, err := os.ReadFile(path)
	if err != nil {
		rh.fail(fmt.Errorf("resource handle: file error: %w", err))
		return
	}

	// Build a synthetic response.
	response := NewResourceResponse()
	response.URL = url
	response.StatusCode = 200
	response.StatusText = "OK"
	response.ContentLength = int64(len(data))

	// Guess MIME type from extension.
	if mime := guessMIMEType(path); mime != "" {
		response.SetContentType(mime)
	}

	rh.response = response

	if rh.client != nil {
		rh.client.DidReceiveResponse(rh, response)
	}

	// Deliver the file content in chunks.
	offset := 0
	for offset < len(data) {
		end := offset + resourceChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		rh.loadedData = append(rh.loadedData, chunk...)
		if rh.client != nil {
			rh.client.DidReceiveData(rh, chunk)
		}
		offset = end
	}

	rh.finish()
}

// finish dispatches DidFinishLoading exactly once. It is called when the
// entire response body has been successfully delivered.
func (rh *ResourceHandle) finish() {
	rh.doneOnce.Do(func() {
		rh.done = true
		if rh.client != nil {
			rh.client.DidFinishLoading(rh)
		}
	})
}

// fail dispatches DidFail exactly once. It is called when a network, file
// I/O or cancellation error occurs.
func (rh *ResourceHandle) fail(err error) {
	rh.doneOnce.Do(func() {
		rh.done = true
		if rh.client != nil {
			rh.client.DidFail(rh, err)
		}
	})
}

// guessMIMEType returns a MIME type for common file extensions, used when
// loading local files where no Content-Type header is available.
func guessMIMEType(path string) string {
	path = strings.ToLower(path)
	switch {
	case strings.HasSuffix(path, ".html"), strings.HasSuffix(path, ".htm"):
		return "text/html"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".js"):
		return "text/javascript"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".svg"), strings.HasSuffix(path, ".svgz"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".woff"):
		return "font/woff"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(path, ".ttf"):
		return "font/ttf"
	case strings.HasSuffix(path, ".otf"):
		return "font/otf"
	case strings.HasSuffix(path, ".xml"):
		return "application/xml"
	case strings.HasSuffix(path, ".pdf"):
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
