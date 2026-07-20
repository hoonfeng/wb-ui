// Translation of: Source/WebCore/loader/cache/MemoryCache.h
//                  Source/WebCore/loader/cache/MemoryCache.cpp
// Completeness: 50%
// Simplifications:
//   - single flat map keyed by URL (WebKit uses (url, partition, type) tuples);
//     our resource type is embedded in CachedResource.Type() but is not part of
//     the cache key, so the same URL used for different resource types will collide
//   - no CachedResourceHandle reference counting; the Go GC manages dead entries
//   - no per-type capacity limits or type-aware LRU sub-lists
//   - no prune callback / dead-resource timer; PruneDeadResources is called
//     explicitly by the loader on navigation or memory pressure
//   - no cross-origin partition separation

package page

import (
	"sync"
	"time"
)

// DefaultMemoryCacheCapacity is the default maximum cache size in bytes (10 MB),
// reduced from WebKit's 50 MB since wb-ui targets local GUI applications where
// cached resources are typically local files that can be re-read quickly.
const DefaultMemoryCacheCapacity = 10 * 1024 * 1024 // 10 MB

// MemoryCache is the Go translation of WebCore::MemoryCache. It is a global
// LRU (least-recently-used) cache for CachedResources, keyed by URL. The
// cache has a byte-based capacity limit; when that limit is exceeded the
// least-recently-used resources are evicted, starting with those that have
// no active clients and then by ascending access count.
//
// All public methods are safe for concurrent use.
type MemoryCache struct {
	// capacity is the maximum total byte size of cached resource data before
	// eviction begins, mirroring MemoryCache::capacity().
	capacity uint64

	// resources maps URLs to cached resources, mirroring the
	// MemoryCache::m_resources map.
	resources map[string]*CachedResource

	// accessList maintains the LRU ordering as a slice of URLs. The most
	// recently accessed resource is at the end; resources eligible for
	// eviction are at the front.
	accessList []string

	mu sync.RWMutex
}

// NewMemoryCache creates a MemoryCache with the given byte capacity.
// If capacity is 0, DefaultMemoryCacheCapacity (50 MB) is used.
func NewMemoryCache(capacity uint64) *MemoryCache {
	if capacity == 0 {
		capacity = DefaultMemoryCacheCapacity
	}
	return &MemoryCache{
		capacity:   capacity,
		resources:  make(map[string]*CachedResource),
		accessList: make([]string, 0),
	}
}

// Get looks up a resource by URL. On a cache hit it updates the last-accessed
// timestamp and moves the URL to the end of the LRU list (making it the most
// recently used). Returns nil on a miss.
func (mc *MemoryCache) Get(url string) *CachedResource {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	res, ok := mc.resources[url]
	if !ok {
		return nil
	}

	// Update LRU order: move url to the end of accessList.
	mc.touch(url)

	// Update the last-accessed timestamp so the eviction policy
	// has accurate data even without an external caller.
	res.SetLastAccessed(time.Now())

	return res
}

// Add inserts a resource into the cache. If a resource with the same URL
// already exists it is replaced. After insertion the cache checks whether
// the capacity has been exceeded and evicts entries as needed.
func (mc *MemoryCache) Add(resource *CachedResource) {
	if resource == nil {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	url := resource.URL()
	resource.SetLastAccessed(time.Now())

	// Insert or replace.
	mc.resources[url] = resource

	// Update LRU order: append (or move) url to the end.
	mc.touch(url)

	// Evict if we exceeded capacity.
	mc.evictIfNeeded()
}

// Remove deletes a resource from the cache by URL. No-op if the URL
// is not present.
func (mc *MemoryCache) Remove(url string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	delete(mc.resources, url)
	mc.removeFromAccessList(url)
}

// RemoveAll clears every resource from the cache.
func (mc *MemoryCache) RemoveAll() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.resources = make(map[string]*CachedResource)
	mc.accessList = make([]string, 0)
}

// Contains reports whether a resource with the given URL is in the cache.
func (mc *MemoryCache) Contains(url string) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	_, ok := mc.resources[url]
	return ok
}

// Capacity returns the current cache capacity in bytes.
func (mc *MemoryCache) Capacity() uint64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.capacity
}

// removeFromAccessList removes all occurrences of url from the LRU list.
// Elements after the removed index are shifted left to preserve the
// relative LRU order (no swap-remove, which would corrupt the sequence).
// Caller must hold mc.mu (write lock).
func (mc *MemoryCache) removeFromAccessList(url string) {
	for i := 0; i < len(mc.accessList); i++ {
		if mc.accessList[i] == url {
			mc.accessList = append(mc.accessList[:i], mc.accessList[i+1:]...)
			i--
		}
	}
}

// SetCapacity updates the cache capacity and triggers eviction if the new
// capacity is smaller than the current size. Passing 0 resets to the default.
func (mc *MemoryCache) SetCapacity(capacity uint64) {
	if capacity == 0 {
		capacity = DefaultMemoryCacheCapacity
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.capacity = capacity
	mc.evictIfNeeded()
}

// Size returns the total byte size of all cached resource data.
func (mc *MemoryCache) Size() uint64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var total uint64
	for _, res := range mc.resources {
		total += uint64(len(res.Data()))
	}
	return total
}

// Count returns the number of resources currently in the cache.
func (mc *MemoryCache) Count() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return len(mc.resources)
}

// PruneDeadResources removes every resource that has no registered clients.
// This is called during memory pressure events or after navigation to release
// resources that are no longer referenced by any renderer.
func (mc *MemoryCache) PruneDeadResources() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for url, res := range mc.resources {
		if !res.HasClients() {
			delete(mc.resources, url)
			mc.removeFromAccessList(url)
		}
	}
}

// touch moves the given URL to the end of the LRU access list, marking it
// as the most recently used. If the URL is not yet in the list it is appended.
// Caller must hold mc.mu (write lock).
func (mc *MemoryCache) touch(url string) {
	// Remove any existing occurrence.
	mc.removeFromAccessList(url)
	// Append to the end (most recently used).
	mc.accessList = append(mc.accessList, url)
}

// evictIfNeeded removes resources from the cache (starting from the LRU end)
// until the total size is within capacity. It prefers to evict resources
// without clients first; if all remaining resources have clients, it evicts
// by ascending access count (least-referenced first).
// Caller must hold mc.mu (write lock).
func (mc *MemoryCache) evictIfNeeded() {
	for mc.computeSize() > mc.capacity && len(mc.accessList) > 0 {
		// Find the best candidate to evict: oldest (front of accessList)
		// that has no clients. If none exist, evict the oldest overall.
		idx := mc.findEvictionCandidate()
		if idx < 0 {
			break // no candidate (should not happen since len>0)
		}

		url := mc.accessList[idx]
		delete(mc.resources, url)
		mc.removeFromAccessList(url)
	}
}

// findEvictionCandidate returns the index in accessList of the resource
// to evict. It scans from the front (oldest) backward, preferring resources
// without clients. If all remaining resources have clients, it picks the one
// with the lowest access count (breaking ties by age — earlier = older).
// Returns -1 if accessList is empty.
// Caller must hold mc.mu (write lock).
func (mc *MemoryCache) findEvictionCandidate() int {
	if len(mc.accessList) == 0 {
		return -1
	}

	// First pass: look for a resource without clients (preferred eviction).
	// Scan from the front (oldest) so we evict the least-recently-used
	// resource among those without clients.
	for i, url := range mc.accessList {
		if res, ok := mc.resources[url]; ok && !res.HasClients() {
			return i
		}
	}

	// Second pass: all remaining resources have clients. Evict the one with
	// the lowest access count, breaking ties by LRU position (lower index =
	// older).
	bestIdx := 0
	bestAccess := mc.resources[mc.accessList[0]].AccessCount()
	for i := 1; i < len(mc.accessList); i++ {
		ac := mc.resources[mc.accessList[i]].AccessCount()
		if ac < bestAccess {
			bestAccess = ac
			bestIdx = i
		}
	}
	return bestIdx
}

// computeSize sums the data bytes of all cached resources.
// Caller must hold mc.mu (at least read lock).
func (mc *MemoryCache) computeSize() uint64 {
	var total uint64
	for _, res := range mc.resources {
		total += uint64(len(res.Data()))
	}
	return total
}

// DefaultMemoryCache is the global, application-wide resource cache with a
// 50 MB default capacity. All resource loading goes through this cache;
// embedders may replace it or adjust its capacity via SetCapacity.
var DefaultMemoryCache *MemoryCache

func init() {
	DefaultMemoryCache = NewMemoryCache(DefaultMemoryCacheCapacity)
}
