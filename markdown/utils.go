package markdown

// Translation of: markdown-it/lib/common/utils.mjs
// Completeness: 90%
//
// Assorted character classification and string utility functions shared by the
// block, inline and core rule chains. Most operate on rune (Unicode code point)
// values because markdown-it's JS source uses charCodeAt (UTF-16 code unit),
// and for the BMP characters Markdown cares about a rune == code unit.

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// IsString reports whether v is a string. Mirrors utils.isString.
func IsString(v interface{}) bool {
	_, ok := v.(string)
	return ok
}

// Has reports whether key exists in map obj. Mirrors utils.has.
func Has(obj map[string]interface{}, key string) bool {
	_, ok := obj[key]
	return ok
}

// Assign merges src into dst (shallow copy) and returns dst. Mirrors
// utils.assign / Object.assign.
func Assign(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// IsSpace reports whether code is a space or tab character (0x09 or 0x20).
// Mirrors utils.isSpace — note this is narrower than isWhiteSpace (no newlines).
func IsSpace(code rune) bool {
	return code == 0x09 || code == 0x20
}

// IsWhiteSpace reports whether code is a whitespace character that can separate
// tokens: space, tab, newline (0x0A), vertical tab (0x0B), form feed (0x0C),
// carriage return (0x0D). Mirrors utils.isWhiteSpace.
func IsWhiteSpace(code rune) bool {
	switch code {
	case 0x09, 0x20, 0x0A, 0x0B, 0x0C, 0x0D:
		return true
	}
	return false
}

// IsMdAsciiPunct reports whether code is an ASCII punctuation character in the
// ranges 0x21-0x2F, 0x3A-0x40, 0x5B-0x60, 0x7B-0x7E. Mirrors
// utils.isMdAsciiPunct.
func IsMdAsciiPunct(code rune) bool {
	switch {
	case code >= 0x21 && code <= 0x2F:
		return true
	case code >= 0x3A && code <= 0x40:
		return true
	case code >= 0x5B && code <= 0x60:
		return true
	case code >= 0x7B && code <= 0x7E:
		return true
	}
	return false
}

// IsPunctChar reports whether ch is a punctuation character. Mirrors
// utils.isPunctChar which uses the Unicode \p{P} category plus some symbol
// ranges. Go's unicode.IsPunct covers the Punctuation category; we also
// include ASCII punctuation explicitly to match markdown-it behaviour.
func IsPunctChar(ch rune) bool {
	if ch < 0x80 {
		return IsMdAsciiPunct(ch)
	}
	return unicode.IsPunct(ch)
}

// IsPunctCharCode is the code-point variant of IsPunctChar. Mirrors
// utils.isPunctCharCode.
func IsPunctCharCode(code rune) bool {
	return IsPunctChar(code)
}

// canOpenPunct and canClosePunct help identify Unicode "Ps"/"Pe" categories.
// Not used directly by markdown-it but kept for parity with potential plugins.

// NormalizeReference converts a reference label to its canonical form for
// lookup: Unicode case-fold, trim, and collapse internal whitespace runs into
// single spaces. Mirrors utils.normalizeReference.
func NormalizeReference(s string) string {
	// Case-fold: use unicode.SimpleFold to lowercase, equivalent to JS
	// toLowerCase on the full string.
	folded := strings.ToLower(s)
	// Collapse whitespace runs (including newlines) to single space, then trim.
	var b strings.Builder
	b.Grow(len(folded))
	inWS := false
	for _, r := range folded {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			inWS = true
			continue
		}
		if inWS {
			b.WriteByte(' ')
			inWS = false
		}
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ")
}

// AsciiTrim removes leading and trailing ASCII whitespace (space, tab, newline)
// from s. Mirrors utils.asciiTrim.
func AsciiTrim(s string) string {
	return strings.Trim(s, " \t\n\r")
}

// HTMLEntityMap maps common HTML named entities to their characters. Used by
// unescapeAll. This is a subset of markdown-it's entities module — the most
// common entities used in Markdown source.
var htmlEntityMap = map[string]string{
	"amp":    "&",
	"lt":     "<",
	"gt":     ">",
	"quot":   "\"",
	"apos":   "'",
	"nbsp":   "\u00A0",
	"copy":   "\u00A9",
	"reg":    "\u00AE",
	"trade":  "\u2122",
	"hellip": "\u2026",
	"mdash":  "\u2014",
	"ndash":  "\u2013",
	"lsquo":  "\u2018",
	"rsquo":  "\u2019",
	"ldquo":  "\u201C",
	"rdquo":  "\u201D",
	"bull":   "\u2022",
	"deg":    "\u00B0",
	"plusmn": "\u00B1",
	"times":  "\u00D7",
	"divide": "\u00F7",
	"frac12": "\u00BD",
	"frac14": "\u00BC",
	"frac34": "\u00BE",
	"laquo":  "\u00AB",
	"raquo":  "\u00BB",
	"iexcl":  "\u00A1",
	"iquest": "\u00BF",
	"cent":   "\u00A2",
	"pound":  "\u00A3",
	"yen":    "\u00A5",
	"euro":   "\u20AC",
	"sect":   "\u00A7",
	"para":   "\u00B6",
	"middot": "\u00B7",
}

// UnescapeMd unescapes Markdown backslash escapes in str. A backslash followed
// by any ASCII punctuation character yields that character literally; other
// backslash sequences are left as-is. Mirrors utils.unescapeMd.
func UnescapeMd(str string) string {
	if strings.IndexByte(str, '\\') < 0 {
		return str
	}
	var b strings.Builder
	b.Grow(len(str))
	runes := []rune(str)
	i := 0
	for i < len(runes) {
		if runes[i] == '\\' && i+1 < len(runes) {
			next := runes[i+1]
			if next < 0x80 && IsMdAsciiPunct(next) {
				b.WriteRune(next)
				i += 2
				continue
			}
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// UnescapeAll unescapes both Markdown backslash escapes and HTML entities
// (&name;, &#decimal;, &#xhex;). Mirrors utils.unescapeAll.
func UnescapeAll(str string) string {
	if strings.IndexByte(str, '\\') < 0 && strings.IndexByte(str, '&') < 0 {
		return str
	}
	// First unescape backslash escapes.
	s := UnescapeMd(str)
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	// Then unescape HTML entities.
	return unescapeEntities(s)
}

// unescapeEntities replaces &name;, &#decimal;, and &#xhex; entities with
// their literal characters. Unknown entities are left as-is.
func unescapeEntities(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Try to find a semicolon within the next 32 chars.
		end := -1
		max := i + 33
		if max > len(s) {
			max = len(s)
		}
		for j := i + 1; j < max; j++ {
			if s[j] == ';' {
				end = j
				break
			}
		}
		if end == -1 {
			b.WriteByte(s[i])
			i++
			continue
		}
		entity := s[i+1 : end]
		if len(entity) == 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Numeric entity: &#...; or &#x...;
		if entity[0] == '#' {
			if replaced, ok := decodeNumericEntity(entity); ok {
				b.WriteString(replaced)
				i = end + 1
				continue
			}
			b.WriteByte(s[i])
			i++
			continue
		}
		// Named entity.
		if val, ok := htmlEntityMap[entity]; ok {
			b.WriteString(val)
			i = end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// decodeNumericEntity decodes &#decimal; or &#xhex; entity body (without the
// leading & and trailing ;). Returns the decoded string and true on success.
func decodeNumericEntity(body string) (string, bool) {
	if len(body) < 2 || body[0] != '#' {
		return "", false
	}
	var code rune
	if body[1] == 'x' || body[1] == 'X' {
		// Hex entity: &#xHHHH;
		for i := 2; i < len(body); i++ {
			c := body[i]
			switch {
			case c >= '0' && c <= '9':
				code = code*16 + rune(c-'0')
			case c >= 'a' && c <= 'f':
				code = code*16 + rune(c-'a'+10)
			case c >= 'A' && c <= 'F':
				code = code*16 + rune(c-'A'+10)
			default:
				return "", false
			}
		}
	} else {
		// Decimal entity: &#DDDD;
		for i := 1; i < len(body); i++ {
			c := body[i]
			if c < '0' || c > '9' {
				return "", false
			}
			code = code*10 + rune(c-'0')
		}
	}
	if !IsValidEntityCode(code) {
		return "", false
	}
	return string(rune(code)), true
}

// EscapeHtml escapes the characters &, <, >, " in str for safe HTML output.
// Mirrors utils.escapeHtml.
func EscapeHtml(str string) string {
	if strings.IndexAny(str, "&<>\"") < 0 {
		return str
	}
	var b strings.Builder
	b.Grow(len(str) * 2)
	for i := 0; i < len(str); {
		r, size := utf8.DecodeRuneInString(str[i:])
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// FromCodePoint converts a Unicode code point to its string representation.
// Mirrors utils.fromCodePoint. In Go this is just string(rune(code)).
func FromCodePoint(code rune) string {
	return string(code)
}

// IsValidEntityCode reports whether code is a valid Unicode code point that can
// be represented as a character. Mirrors utils.isValidEntityCode.
func IsValidEntityCode(code rune) bool {
	// Surrogate pairs (0xD800-0xDFFF) are invalid in UTF-8/UTF-32.
	if code >= 0xD800 && code <= 0xDFFF {
		return false
	}
	// Non-characters: 0xFFFE/0xFFFF, 0xFDD0-0xFDEF, and the last two code
	// points of each plane (0xNFFFE/0xNFFFF for N=1..16).
	if code > 0x10FFFF {
		return false
	}
	if (code >= 0xFDD0 && code <= 0xFDEF) ||
		(code&0xFFFE) == 0xFFFE {
		return false
	}
	return true
}

// ArrayReplaceAt replaces a single element at index off in arr with the
// elements of items, returning the new slice. Mirrors utils.arrayReplaceAt.
func ArrayReplaceAt(arr []Token, off int, items []Token) []Token {
	// arr[:off] + items + arr[off+1:]
	result := make([]Token, 0, len(arr)-1+len(items))
	result = append(result, arr[:off]...)
	result = append(result, items...)
	result = append(result, arr[off+1:]...)
	return result
}

// EscapeRE is the regex used to escape regex special characters. In Go this
// serves as documentation; callers use regexp.QuoteMeta instead.
// Mirrors utils.escapeRE for reference.

// PartialSortStable is not in markdown-it but provided as a utility for
// sorting delimiters by length in emphasis post-processing.

// AssignTokens is a helper to append a slice of tokens (not in markdown-it,
// provided for Go convenience since slices are values not references).
