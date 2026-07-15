// Translation of: Source/WebCore/loader/cache/CachedResource.h
//                  Source/WebCore/loader/cache/CachedResource.cpp
// Completeness: 60%
// Simplifications:
//   - no CachedResourceHandle / RefPtr ownership model; Go GC manages lifecycle
//   - no preload / preconnect distinctions
//   - no request priority / loading priority / request originator tracking
//   - no redirect chain bookkeeping
//   - no resource load timing / network metrics on the base
//   - no fragment identifier stripping from the URL
//   - no decoded data / cache partition / cross-origin access control on the base
//     struct; subclasses (CachedImage, CachedFont) own decoding independently
//   - clients are stored as a flat map and cleared after the final notification,
//     matching WebKit's behaviour in CachedResource::finishLoading / error

package page

import (
	"time"
)

// CachedResourceStatus mirrors WebCore::CachedResource::Status. It tracks
// the lifecycle of a cached resource from creation through loading to a
// terminal state (loaded or error).
type CachedResourceStatus int

const (
	// CachedResourceStatusPending means the resource has been requested but
	// data has not yet arrived. This is the initial status.
	CachedResourceStatusPending CachedResourceStatus = iota
	// CachedResourceStatusLoaded means the resource data has been fully received
	// and is available via Data(). No further callbacks will fire.
	CachedResourceStatusLoaded
	// CachedResourceStatusError means the resource load failed. The error is
	// available via Error(). No further callbacks will fire.
	CachedResourceStatusError
)

// String returns the human-readable name of the status.
func (s CachedResourceStatus) String() string {
	switch s {
	case CachedResourceStatusPending:
		return "Pending"
	case CachedResourceStatusLoaded:
		return "Loaded"
	case CachedResourceStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// CachedResourceClient is the Go translation of WebCore::CachedResourceClient.
// An object that wishes to be notified when a CachedResource finishes loading
// or fails implements this interface and registers itself via AddClient.
type CachedResourceClient interface {
	// NotifyFinished is called when the resource transitions to Loaded or Error
	// status (once, after SetData or SetError). The resource pointer is valid
	// for the duration of the call but may be destroyed afterward; clients
	// should not retain it beyond NotifyFinished.
	NotifyFinished(resource *CachedResource)
}

// CachedResource is the Go translation of WebCore::CachedResource. It is the
// base class for all cacheable resources (CSS stylesheets, scripts, images,
// fonts, raw data, SVG documents). Each CachedResource is uniquely identified
// by its URL and ResourceType; the MemoryCache uses (url, type) as the key.
//
// Lifecycle:
//
//	NewCachedResource(url, type)   status = Pending, lastAccessed = now
//	    →  AddClient(client)        one or more consumers register interest
//	    →  SetData(data)            status = Loaded, notify all clients, clear clients
//	    →  SetError(err)            status = Error, notify all clients, clear clients
//	    →  (after clients are gone, resource sits in MemoryCache until evicted)
type CachedResource struct {
	url          string
	resourceType ResourceType
	status       CachedResourceStatus
	data         []byte
	err          error
	textEncoding string
	mimeType     string
	accessCount  int
	clients      map[CachedResourceClient]bool
	lastAccessed time.Time
}

// NewCachedResource creates a CachedResource in Pending status for the given
// URL and type. The last-accessed timestamp is set to the current time so that
// freshly-created resources sort correctly in the MemoryCache LRU list.
func NewCachedResource(url string, resourceType ResourceType) *CachedResource {
	return &CachedResource{
		url:          url,
		resourceType: resourceType,
		status:       CachedResourceStatusPending,
		clients:      make(map[CachedResourceClient]bool),
		lastAccessed: time.Now(),
	}
}

// URL returns the resource URL, mirroring CachedResource::url().
func (cr *CachedResource) URL() string { return cr.url }

// Type returns the resource type, mirroring CachedResource::type().
func (cr *CachedResource) Type() ResourceType { return cr.resourceType }

// Status returns the current load status, mirroring CachedResource::status().
func (cr *CachedResource) Status() CachedResourceStatus { return cr.status }

// Data returns the loaded resource data (nil when status is not Loaded),
// mirroring CachedResource::data().
func (cr *CachedResource) Data() []byte { return cr.data }

// Error returns the load error (nil when status is not Error),
// mirroring CachedResource::error().
func (cr *CachedResource) Error() error { return cr.err }

// TextEncoding returns the text encoding extracted from the resource's
// Content-Type header, mirroring CachedResource::encoding().
func (cr *CachedResource) TextEncoding() string { return cr.textEncoding }

// MimeType returns the MIME type string, mirroring CachedResource::mimeType().
func (cr *CachedResource) MimeType() string { return cr.mimeType }

// AccessCount returns the number of active references to this resource,
// mirroring CachedResource::accessCount().
func (cr *CachedResource) AccessCount() int { return cr.accessCount }

// IncrementAccessCount bumps the access count, mirroring
// CachedResource::incrementAccessCount().
func (cr *CachedResource) IncrementAccessCount() { cr.accessCount++ }

// DecrementAccessCount reduces the access count (but never below zero),
// mirroring CachedResource::decrementAccessCount().
func (cr *CachedResource) DecrementAccessCount() {
	if cr.accessCount > 0 {
		cr.accessCount--
	}
}

// LastAccessed returns the time this resource was last accessed (created,
// or most recently touched by the MemoryCache), mirroring the sort key used
// in WebKit's MemoryCache::LRUList.
func (cr *CachedResource) LastAccessed() time.Time { return cr.lastAccessed }

// SetLastAccessed updates the last-accessed timestamp, mirroring the update
// the MemoryCache performs on cache hits.
func (cr *CachedResource) SetLastAccessed(t time.Time) { cr.lastAccessed = t }

// SetEncoding sets the text encoding, mirroring CachedResource::setEncoding().
func (cr *CachedResource) SetEncoding(encoding string) { cr.textEncoding = encoding }

// SetMimeType sets the MIME type, mirroring CachedResource::setMimeType().
func (cr *CachedResource) SetMimeType(mimeType string) { cr.mimeType = mimeType }

// Encoding is an alias for TextEncoding, matching the WebKit accessor name.
func (cr *CachedResource) Encoding() string { return cr.textEncoding }

// SetData stores the loaded data, transitions the status to Loaded and
// notifies all registered clients via NotifyFinished. After notification the
// client list is cleared (clients are expected to have consumed the resource
// and do not need a second notification). Mirroring the final part of
// CachedResource::finishLoading().
func (cr *CachedResource) SetData(data []byte) {
	cr.data = data
	cr.status = CachedResourceStatusLoaded
	cr.notifyClients()
}

// SetError records the load error, transitions the status to Error and
// notifies all registered clients via NotifyFinished. After notification the
// client list is cleared. Mirroring CachedResource::error().
func (cr *CachedResource) SetError(err error) {
	cr.err = err
	cr.status = CachedResourceStatusError
	cr.notifyClients()
}

// HasClients reports whether at least one client is registered, mirroring
// CachedResource::hasClients().
func (cr *CachedResource) HasClients() bool {
	return len(cr.clients) > 0
}

// AddClient registers a client for notification when the resource finishes
// loading or errors. If the resource has already reached a terminal status
// (Loaded or Error) the client is notified immediately and is not stored.
// Mirroring CachedResource::addClient().
func (cr *CachedResource) AddClient(client CachedResourceClient) {
	if client == nil {
		return
	}
	// If the resource is already in a terminal state, notify immediately.
	if cr.status != CachedResourceStatusPending {
		client.NotifyFinished(cr)
		return
	}
	cr.clients[client] = true
}

// RemoveClient unregisters a client so it will not receive the finish
// notification. Mirroring CachedResource::removeClient().
func (cr *CachedResource) RemoveClient(client CachedResourceClient) {
	delete(cr.clients, client)
}

// notifyClients iterates all registered clients, calls NotifyFinished on
// each, then clears the client map. This matches WebKit behaviour where
// clients are cleared after the final notification in finishLoading/error.
func (cr *CachedResource) notifyClients() {
	// Snapshot the map so that a client which calls RemoveClient during
	// its own NotifyFinished does not affect iteration of other clients.
	for client := range cr.clients {
		client.NotifyFinished(cr)
	}
	cr.clients = make(map[CachedResourceClient]bool)
}
