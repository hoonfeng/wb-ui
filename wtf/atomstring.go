// Translation of: Source/WTF/wtf/text/AtomString.h
//                  Source/WTF/wtf/text/AtomString.cpp
// Completeness: 80%
// Simplifications:
//   - backed by a sync.Map intern table instead of WebKit's custom StringHasher+HashTable
//   - equality is a single pointer comparison on the interned *string rather than a hash
//   - no 8-bit/16-bit storage mode distinction

package wtf

import "sync"

// atomTable is the global intern table mapping a string value to a stable *string.
// AtomString instances hold a pointer into this table so that two AtomStrings created
// from the same value share the same pointer and can be compared by pointer equality
// in O(1). This mirrors WebKit's AtomStringImpl singleton-per-value model.
var atomTable sync.Map // map[string]*string

// AtomString is the Go translation of WTF::AtomString. In WebKit AtomString is a
// reference to a unique, interned string: every distinct value has exactly one
// underlying AtomStringImpl in a global table, so equality reduces to a pointer
// comparison. The Go port uses a sync.Map keyed by the string value and stores a
// stable *string; two AtomStrings built from the same value share the same pointer and
// thus compare equal without hashing the contents again.
//
// Construct with NewAtomString; the zero value is the null atom (empty string).
type AtomString struct {
	p *string
}

// NewAtomString interns value and returns an AtomString referring to it. Repeated
// calls with the same value return AtomStrings that share the same underlying pointer
// and therefore compare equal in O(1).
func NewAtomString(value string) AtomString {
	if v, ok := atomTable.Load(value); ok {
		return AtomString{p: v.(*string)}
	}
	// Make a stable heap copy so the pointer is independent of the caller's backing
	// array. LoadOrStore handles the race where another goroutine interned the same
	// value concurrently.
	cp := value
	actual, _ := atomTable.LoadOrStore(value, &cp)
	return AtomString{p: actual.(*string)}
}

// NewAtomStringFromString interns the payload of a String and returns an AtomString.
func NewAtomStringFromString(s String) AtomString { return NewAtomString(s.s) }

// String returns the interned string value, mirroring AtomString::string(). The null
// atom returns the empty string.
func (a AtomString) String() string {
	if a.p == nil {
		return ""
	}
	return *a.p
}

// Str is a shorter alias for String().
func (a AtomString) Str() string { return a.String() }

// IsNull reports whether the atom is the null atom, mirroring AtomString::isNull().
func (a AtomString) IsNull() bool { return a.p == nil }

// IsEmpty reports whether the atom holds the empty string, mirroring
// AtomString::isEmpty().
func (a AtomString) IsEmpty() bool { return a.p == nil || *a.p == "" }

// Equals reports whether two atoms refer to the same interned value, mirroring
// AtomString::operator==. Because both operands must have been interned, the
// comparison is a single pointer compare.
func (a AtomString) Equals(other AtomString) bool { return a.p == other.p }

// EqualsString reports whether the atom refers to the interned form of value. It
// interns value on demand so it is correct but slower than Equals for repeated use.
func (a AtomString) EqualsString(value string) bool {
	return a.Equals(NewAtomString(value))
}

// Impl returns the underlying interned pointer, exposed so that downstream code can
// use atoms as hash map keys in the same way WebKit uses AtomStringImpl*. The pointer
// is stable for the lifetime of the process.
func (a AtomString) Impl() *string { return a.p }
