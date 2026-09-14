// Translation of: Source/WTF/wtf/HashMap.h
//                  Source/WTF/wtf/HashTable.h
// Completeness: 80%
// Simplifications:
//   - backed by a Go map[K]V instead of a custom open-addressing HashTable
//   - no custom HashTranslator / HashTraits; Go's comparable constraint replaces them
//   - iterator API is omitted; use Keys()/Values()/ForEach instead

package wtf

// HashMap is the Go translation of WTF::HashMap<K, V>. WebKit's HashMap is a hash
// table mapping keys to values built on top of HashTable; the Go port uses the
// built-in map[K]V which provides the same average O(1) lookup/insert/erase with Go's
// runtime hash function. Only the API surface that downstream modules rely on is
// exposed (Set/Add/Get/GetOptional/Contains/Remove/Clear/Size/IsEmpty/Keys/Values/
// Take/Ensure/ForEach).
//
// The zero value is an empty, ready-to-use map.
//
// The distinction between Set (replace existing) and Add (no-op if present) mirrors
// WebKit's HashMap::set vs HashMap::add; the returned AddResult reports whether a new
// entry was created.
type HashMap[K comparable, V any] struct {
	m map[K]V
}

// AddResult mirrors WTF::HashMap::AddResult, exposing whether a new entry was created
// for a Set/Add operation.
type AddResult struct {
	// IsNewEntry is true when the operation inserted a brand-new key.
	IsNewEntry bool
}

// NewHashMap returns an empty HashMap.
func NewHashMap[K comparable, V any]() HashMap[K, V] {
	return HashMap[K, V]{m: make(map[K]V)}
}

// NewHashMapFrom returns a HashMap seeded with the given key/value pairs.
func NewHashMapFrom[K comparable, V any](items ...struct {
	Key   K
	Value V
}) HashMap[K, V] {
	out := NewHashMap[K, V]()
	for _, it := range items {
		out.Set(it.Key, it.Value)
	}
	return out
}

// Size returns the number of entries, mirroring HashMap::size().
func (h *HashMap[K, V]) Size() int { return len(h.m) }

// IsEmpty reports whether the map has no entries, mirroring HashMap::isEmpty().
func (h *HashMap[K, V]) IsEmpty() bool { return len(h.m) == 0 }

// Capacity returns the logical capacity hint. Go maps do not expose capacity, so this
// reports Size() to keep callers that read capacity happy without lying about an
// unknown value.
func (h *HashMap[K, V]) Capacity() int { return len(h.m) }

// ReserveInitialCapacity preallocates storage for at least keyCount entries,
// mirroring HashMap::reserveInitialCapacity.
func (h *HashMap[K, V]) ReserveInitialCapacity(keyCount int) {
	if keyCount <= 0 {
		return
	}
	if h.m == nil {
		h.m = make(map[K]V, keyCount)
		return
	}
	// Go maps grow transparently; nothing to do for an existing non-empty map.
}

// Set inserts or replaces the entry for key, mirroring HashMap::set. It returns an
// AddResult whose IsNewEntry flag is true when key was not previously present.
func (h *HashMap[K, V]) Set(key K, value V) AddResult {
	if h.m == nil {
		h.m = make(map[K]V)
	}
	_, existed := h.m[key]
	h.m[key] = value
	return AddResult{IsNewEntry: !existed}
}

// Add inserts the entry only if key is not already present, mirroring HashMap::add.
// It returns an AddResult whose IsNewEntry flag is true when a new entry was added.
// If the key already exists the stored value is left unchanged.
func (h *HashMap[K, V]) Add(key K, value V) AddResult {
	if h.m == nil {
		h.m = make(map[K]V)
	}
	if _, existed := h.m[key]; existed {
		return AddResult{IsNewEntry: false}
	}
	h.m[key] = value
	return AddResult{IsNewEntry: true}
}

// Ensure returns the value for key, creating it via the factory if absent, mirroring
// HashMap::ensure. The AddResult reports whether a new entry was created.
func (h *HashMap[K, V]) Ensure(key K, factory func() V) (V, AddResult) {
	if h.m == nil {
		h.m = make(map[K]V)
	}
	if v, existed := h.m[key]; existed {
		return v, AddResult{IsNewEntry: false}
	}
	v := factory()
	h.m[key] = v
	return v, AddResult{IsNewEntry: true}
}

// Get returns the value for key and a found flag, mirroring HashMap::get. The zero
// value of V is returned when key is absent.
func (h *HashMap[K, V]) Get(key K) (V, bool) {
	v, ok := h.m[key]
	return v, ok
}

// GetOr returns the value for key, or fallback when key is absent. It is the Go
// equivalent of HashMap::get combined with a default.
func (h *HashMap[K, V]) GetOr(key K, fallback V) V {
	if v, ok := h.m[key]; ok {
		return v
	}
	return fallback
}

// Contains reports whether key is present, mirroring HashMap::contains.
func (h *HashMap[K, V]) Contains(key K) bool {
	_, ok := h.m[key]
	return ok
}

// Remove deletes the entry for key, mirroring HashMap::remove. It returns true when
// an entry was actually removed.
func (h *HashMap[K, V]) Remove(key K) bool {
	if _, ok := h.m[key]; ok {
		delete(h.m, key)
		return true
	}
	return false
}

// RemoveIf deletes every entry for which pred returns true, mirroring
// HashMap::removeIf.
func (h *HashMap[K, V]) RemoveIf(pred func(K, V) bool) int {
	removed := 0
	for k, v := range h.m {
		if pred(k, v) {
			delete(h.m, k)
			removed++
		}
	}
	return removed
}

// Clear removes all entries, mirroring HashMap::clear().
func (h *HashMap[K, V]) Clear() {
	for k := range h.m {
		delete(h.m, k)
	}
}

// Take removes and returns the value for key, mirroring HashMap::take. The found flag
// reports whether key was present.
func (h *HashMap[K, V]) Take(key K) (V, bool) {
	if v, ok := h.m[key]; ok {
		delete(h.m, key)
		return v, true
	}
	var zero V
	return zero, false
}

// Keys returns a slice of all keys, mirroring HashMap::keys(). Order is unspecified,
// matching the upstream iterator semantics.
func (h *HashMap[K, V]) Keys() []K {
	out := make([]K, 0, len(h.m))
	for k := range h.m {
		out = append(out, k)
	}
	return out
}

// Values returns a slice of all values, mirroring HashMap::values().
func (h *HashMap[K, V]) Values() []V {
	out := make([]V, 0, len(h.m))
	for _, v := range h.m {
		out = append(out, v)
	}
	return out
}

// ForEach invokes fn for every entry. It mirrors the range-based iteration over
// HashMap in upstream code.
func (h *HashMap[K, V]) ForEach(fn func(K, V)) {
	for k, v := range h.m {
		fn(k, v)
	}
}

// SwapWith swaps the contents with other, mirroring HashMap::swap.
func (h *HashMap[K, V]) SwapWith(other *HashMap[K, V]) {
	h.m, other.m = other.m, h.m
}
