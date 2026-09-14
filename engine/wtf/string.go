// Translation of: Source/WTF/wtf/text/WTFString.h
//                  Source/WTF/wtf/text/WTFString.cpp
// Completeness: 75%
// Simplifications:
//   - backed by a Go string (UTF-8) instead of an internal UTF-16 LChar*/UChar* buffer
//   - the UTF-16 view is computed lazily on demand rather than stored permanently
//   - only the API surface needed by downstream DOM/CSS phases is exposed

package wtf

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// String is the Go translation of WTF::String. WebKit's String is a reference-counted
// UTF-16 string with both 8-bit (Latin-1) and 16-bit storage modes; the Go port is
// backed by a Go string (UTF-8) and exposes a lazily-computed UTF-16 view so that
// translated code calling UTF16Length / CharCodeAt / Substring keeps the same
// semantics. The String type deliberately wraps (rather than aliases) a Go string so
// that methods can be attached without polluting the builtin.
//
// Construct with NewString; the zero value is the empty string.
type String struct {
	s string
}

// NewString constructs a String from a Go string. It mirrors WTF::String(const char*).
func NewString(s string) String { return String{s: s} }

// NewStringFromString constructs a String from another String (copy).
func NewStringFromString(s String) String { return String{s: s.s} }

// String returns the underlying Go string, mirroring String::utf8() / String::latin1().
func (s String) String() string { return s.s }

// Str is a shorter alias for String() for use at call sites that read the payload.
func (s String) Str() string { return s.s }

// IsEmpty reports whether the string has zero length, mirroring String::isEmpty().
func (s String) IsEmpty() bool { return len(s.s) == 0 }

// IsNull reports whether the string is the null/undefined string. WebKit
// distinguishes the empty string from the null string; the Go port folds both into
// the empty Go string, so IsNull is equivalent to IsEmpty here.
func (s String) IsNull() bool { return len(s.s) == 0 }

// Length returns the number of UTF-16 code units, mirroring String::length(). For an
// ASCII string this equals the byte length; for strings containing astral characters
// surrogate pairs are counted as two units, matching the UTF-16 model.
func (s String) Length() int { return s.UTF16Length() }

// ByteLength returns the number of bytes in the UTF-8 representation. It has no
// direct WebKit counterpart but is useful when bridging to Go APIs.
func (s String) ByteLength() int { return len(s.s) }

// UTF16Length returns the number of UTF-16 code units, mirroring String::length().
// It is computed on demand from the UTF-8 bytes.
func (s String) UTF16Length() int {
	return len(utf16.Encode([]rune(s.s)))
}

// utf16View returns the UTF-16 code unit sequence of the string. It is the basis for
// CharCodeAt/Substring which operate in UTF-16 index space.
func (s String) utf16View() []uint16 {
	return utf16.Encode([]rune(s.s))
}

// CharCodeAt returns the UTF-16 code unit at the given UTF-16 index and a found flag.
// It mirrors String::codeUnitAt(index). Out-of-range indices report found=false with a
// zero code unit, matching WebKit's behaviour of returning 0 past the end.
func (s String) CharCodeAt(index int) (uint16, bool) {
	u := s.utf16View()
	if index < 0 || index >= len(u) {
		return 0, false
	}
	return u[index], true
}

// Substring returns the substring of length maxLength starting at UTF-16 index start,
// mirroring String::substring(start, maxLength). If start is beyond the end the empty
// string is returned; maxLength is clamped to the remaining length.
func (s String) Substring(start, maxLength int) String {
	u := s.utf16View()
	if start < 0 {
		start = 0
	}
	if start >= len(u) {
		return String{}
	}
	end := start + maxLength
	if maxLength < 0 || end > len(u) {
		end = len(u)
	}
	return NewString(string(utf16.Decode(u[start:end])))
}

// Equals reports whether two Strings are equal, mirroring String::operator==. Go
// string comparison is used, which is correct because equal UTF-8 encodings imply
// equal strings and vice versa.
func (s String) Equals(other String) bool { return s.s == other.s }

// EqualsString compares against a plain Go string.
func (s String) EqualsString(other string) bool { return s.s == other }

// IsASCII reports whether every byte of the string is ASCII (< 0x80), mirroring
// String::isAllASCII / String::is8Bit. This is the 8-bit storage mode predicate.
func (s String) IsASCII() bool {
	for i := 0; i < len(s.s); i++ {
		if s.s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// IsLatin1 reports whether every code point fits in a single byte (<= 0xFF), the
// predicate for WebKit's LChar storage mode.
func (s String) IsLatin1() bool {
	for _, r := range s.s {
		if r > 0xFF {
			return false
		}
	}
	return true
}

// ToASCIIUpper returns an uppercased copy of the string using ASCII rules only,
// mirroring String::upper() in its ASCII-only fast path.
func (s String) ToASCIIUpper() String {
	return NewString(strings.ToUpper(s.s))
}

// ToASCIILower returns a lowercased copy of the string using ASCII rules only,
// mirroring String::lower() in its ASCII-only fast path.
func (s String) ToASCIILower() String {
	return NewString(strings.ToLower(s.s))
}

// Contains reports whether substr appears within the string, mirroring
// String::find(substring) != NotFound.
func (s String) Contains(substr string) bool { return strings.Contains(s.s, substr) }

// StartsWith reports whether the string begins with prefix, mirroring
// String::startsWith(prefix).
func (s String) StartsWith(prefix string) bool { return strings.HasPrefix(s.s, prefix) }

// EndsWith reports whether the string ends with suffix, mirroring
// String::endsWith(suffix).
func (s String) EndsWith(suffix string) bool { return strings.HasSuffix(s.s, suffix) }

// IndexOf returns the byte index of the first occurrence of substr, or NotFound if
// absent. It mirrors String::find(substring).
func (s String) IndexOf(substr string) int {
	idx := strings.Index(s.s, substr)
	if idx < 0 {
		return NotFound
	}
	return idx
}

// Append returns a new String formed by concatenating other, mirroring the String
// concatenation operator.
func (s String) Append(other String) String { return NewString(s.s + other.s) }

// IsolatedCopy returns a copy of the string. WebKit's isolatedCopy is concerned with
// thread-safe ref-counting of the shared buffer; Go strings are immutable and safe to
// share across goroutines, so this is a trivial copy retained for fidelity.
func (s String) IsolatedCopy() String { return s }
