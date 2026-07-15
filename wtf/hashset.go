// Translation of: Source/WTF/wtf/HashSet.h
//                  Source/WTF/wtf/HashTable.h
// Completeness: 80%
// Simplifications:
//   - backed by a Go map[T]struct{} instead of a custom open-addressing HashTable
//   - no custom HashTranslator / HashTraits; Go's comparable constraint replaces them
//   - iterator API is omitted; use Values()/ForEach instead

package wtf

// HashSet is the Go translation of WTF::HashSet<T>. WebKit's HashSet is a hash table
// of unique values built on top of HashTable; the Go port uses the built-in
// map[T]struct{} which provides the same average O(1) insert/contains/erase with Go's
// runtime hash function. Only the API surface that downstream modules rely on is
// exposed (Add/Contains/Remove/Clear/Size/IsEmpty/Take/ForEach and the set-algebra
// helpers).
//
// The zero value is an empty, ready-to-use set.
type HashSet[T comparable] struct {
	m map[T]struct{}
}

// NewHashSet returns an empty HashSet.
func NewHashSet[T comparable]() HashSet[T] {
	return HashSet[T]{m: make(map[T]struct{})}
}

// NewHashSetFrom returns a HashSet seeded with the given values, mirroring the
// initializer-list constructor of WebKit's HashSet.
func NewHashSetFrom[T comparable](items ...T) HashSet[T] {
	out := NewHashSet[T]()
	for _, it := range items {
		out.Add(it)
	}
	return out
}

// Size returns the number of entries, mirroring HashSet::size().
func (s *HashSet[T]) Size() int { return len(s.m) }

// Len is an alias for Size matching Go container convention.
func (s *HashSet[T]) Len() int { return len(s.m) }

// IsEmpty reports whether the set has no entries, mirroring HashSet::isEmpty().
func (s *HashSet[T]) IsEmpty() bool { return len(s.m) == 0 }

// ReserveInitialCapacity preallocates storage for at least keyCount entries,
// mirroring HashSet::reserveInitialCapacity.
func (s *HashSet[T]) ReserveInitialCapacity(keyCount int) {
	if keyCount <= 0 {
		return
	}
	if s.m == nil {
		s.m = make(map[T]struct{}, keyCount)
	}
}

// Add inserts value if not already present, mirroring HashSet::add. It returns an
// AddResult whose IsNewEntry flag is true when a new entry was added.
func (s *HashSet[T]) Add(value T) AddResult {
	if s.m == nil {
		s.m = make(map[T]struct{})
	}
	if _, existed := s.m[value]; existed {
		return AddResult{IsNewEntry: false}
	}
	s.m[value] = struct{}{}
	return AddResult{IsNewEntry: true}
}

// Contains reports whether value is present, mirroring HashSet::contains.
func (s *HashSet[T]) Contains(value T) bool {
	_, ok := s.m[value]
	return ok
}

// Remove deletes value, mirroring HashSet::remove. It returns true when an entry was
// actually removed.
func (s *HashSet[T]) Remove(value T) bool {
	if _, ok := s.m[value]; ok {
		delete(s.m, value)
		return true
	}
	return false
}

// RemoveIf deletes every entry for which pred returns true, mirroring
// HashSet::removeIf. It returns the number of removed entries.
func (s *HashSet[T]) RemoveIf(pred func(T) bool) int {
	removed := 0
	for v := range s.m {
		if pred(v) {
			delete(s.m, v)
			removed++
		}
	}
	return removed
}

// Clear removes all entries, mirroring HashSet::clear().
func (s *HashSet[T]) Clear() {
	for v := range s.m {
		delete(s.m, v)
	}
}

// Take removes and returns an arbitrary value, mirroring HashSet::takeAny(). The
// found flag reports whether the set was non-empty.
func (s *HashSet[T]) Take() (T, bool) {
	for v := range s.m {
		delete(s.m, v)
		return v, true
	}
	var zero T
	return zero, false
}

// Values returns a slice of all values, mirroring the iterator range over a HashSet.
func (s *HashSet[T]) Values() []T {
	out := make([]T, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

// ForEach invokes fn for every value.
func (s *HashSet[T]) ForEach(fn func(T)) {
	for v := range s.m {
		fn(v)
	}
}

// SwapWith swaps the contents with other, mirroring HashSet::swap.
func (s *HashSet[T]) SwapWith(other *HashSet[T]) {
	s.m, other.m = other.m, s.m
}

// UnionWith returns a new set containing elements present in either set, mirroring
// HashSet::unionWith.
func (s *HashSet[T]) UnionWith(other HashSet[T]) HashSet[T] {
	out := NewHashSet[T]()
	for v := range s.m {
		out.m[v] = struct{}{}
	}
	for v := range other.m {
		out.m[v] = struct{}{}
	}
	return out
}

// IntersectionWith returns a new set containing elements present in both sets,
// mirroring HashSet::intersectionWith.
func (s *HashSet[T]) IntersectionWith(other HashSet[T]) HashSet[T] {
	out := NewHashSet[T]()
	for v := range s.m {
		if _, ok := other.m[v]; ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// DifferenceWith returns a new set containing elements present in this set but not
// in other, mirroring HashSet::differenceWith.
func (s *HashSet[T]) DifferenceWith(other HashSet[T]) HashSet[T] {
	out := NewHashSet[T]()
	for v := range s.m {
		if _, ok := other.m[v]; !ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// SymmetricDifferenceWith returns a new set containing elements present in exactly
// one of the two sets, mirroring HashSet::symmetricDifferenceWith.
func (s *HashSet[T]) SymmetricDifferenceWith(other HashSet[T]) HashSet[T] {
	out := NewHashSet[T]()
	for v := range s.m {
		if _, ok := other.m[v]; !ok {
			out.m[v] = struct{}{}
		}
	}
	for v := range other.m {
		if _, ok := s.m[v]; !ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// FormIntersection removes elements not present in other, mirroring
// HashSet::formIntersection.
func (s *HashSet[T]) FormIntersection(other HashSet[T]) {
	for v := range s.m {
		if _, ok := other.m[v]; !ok {
			delete(s.m, v)
		}
	}
}

// FormDifference removes elements present in other, mirroring
// HashSet::formDifference.
func (s *HashSet[T]) FormDifference(other HashSet[T]) {
	for v := range other.m {
		delete(s.m, v)
	}
}
