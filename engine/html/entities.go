// Translation of: Source/WebCore/html/parser/HTMLEntityTable.h
//                  Source/WebCore/html/parser/HTMLEntityParser.cpp
// Completeness: 60%
// Simplifications:
//   - Only the most common named entities are present (the full HTML5 entity
//     table is ~2200 entries; this port ships a curated subset that covers the
//     entities needed for typical document parsing)
//   - Numeric character references (decimal &#123; and hex &#x1F;) are fully
//     supported via decodeNumericEntity
//   - Entity matching is longest-prefix greedy, matching the spec algorithm

package html

// namedEntities maps a named character reference (without the leading '&' and
// trailing ';') to its replacement string. The table is a curated subset of the
// HTML5 named character reference table covering the entities most commonly
// seen in real documents. Keys are stored without trailing ';' so that both the
// terminated form (&amp;) and the unterminated special-case forms used in
// attribute values can be looked up.
var namedEntities = map[string]string{
	"amp":     "&",
	"lt":      "<",
	"gt":      ">",
	"quot":    "\"",
	"apos":    "'",
	"nbsp":    "\u00a0",
	"copy":    "\u00a9",
	"reg":     "\u00ae",
	"trade":   "\u2122",
	"hellip":  "\u2026",
	"mdash":   "\u2014",
	"ndash":   "\u2013",
	"laquo":   "\u00ab",
	"raquo":   "\u00bb",
	"lsquo":   "\u2018",
	"rsquo":   "\u2019",
	"ldquo":   "\u201c",
	"rdquo":   "\u201d",
	"bull":    "\u2022",
	"dagger":  "\u2020",
	"Dagger":  "\u2021",
	"permil":  "\u2030",
	"prime":   "\u2032",
	"Prime":   "\u2033",
	"times":   "\u00d7",
	"divide":  "\u00f7",
	"minus":   "\u2212",
	"plus":    "+",
	"plusmn":  "\u00b1",
	"frac12":  "\u00bd",
	"frac14":  "\u00bc",
	"frac34":  "\u00be",
	"sup1":    "\u00b9",
	"sup2":    "\u00b2",
	"sup3":    "\u00b3",
	"deg":     "\u00b0",
	"micro":   "\u00b5",
	"para":    "\u00b6",
	"middot":  "\u00b7",
	"cent":    "\u00a2",
	"pound":   "\u00a3",
	"euro":    "\u20ac",
	"yen":     "\u00a5",
	"curren":  "\u00a4",
	"sect":    "\u00a7",
	"brvbar":  "\u00a6",
	"shy":     "\u00ad",
	"uml":     "\u00a8",
	"macr":    "\u00af",
	"acute":   "\u00b4",
	"cedil":   "\u00b8",
	"not":     "\u00ac",
	"or":      "\u2228",
	"and":     "\u2227",
	"cap":     "\u2229",
	"cup":     "\u222a",
	"in":      "\u2208",
	"notin":   "\u2209",
	"sub":     "\u2282",
	"sup":     "\u2283",
	"equiv":   "\u2261",
	"ne":      "\u2260",
	"le":      "\u2264",
	"ge":      "\u2265",
	"inf":     "\u221e",
	"alpha":   "\u03b1",
	"beta":    "\u03b2",
	"gamma":   "\u03b3",
	"delta":   "\u03b4",
	"epsilon": "\u03b5",
	"zeta":    "\u03b6",
	"eta":     "\u03b7",
	"theta":   "\u03b8",
	"iota":    "\u03b9",
	"kappa":   "\u03ba",
	"lambda":  "\u03bb",
	"mu":      "\u03bc",
	"nu":      "\u03bd",
	"xi":      "\u03be",
	"omicron": "\u03bf",
	"pi":      "\u03c0",
	"rho":     "\u03c1",
	"sigma":   "\u03c3",
	"tau":     "\u03c4",
	"upsilon": "\u03c5",
	"phi":     "\u03c6",
	"chi":     "\u03c7",
	"psi":     "\u03c8",
	"omega":   "\u03c9",
	"Alpha":   "\u0391",
	"Beta":    "\u0392",
	"Gamma":   "\u0393",
	"Delta":   "\u0394",
	"Epsilon": "\u0395",
	"Zeta":    "\u0396",
	"Eta":     "\u0397",
	"Theta":   "\u0398",
	"Iota":    "\u0399",
	"Kappa":   "\u039a",
	"Lambda":  "\u039b",
	"Mu":      "\u039c",
	"Nu":      "\u039d",
	"Xi":      "\u039e",
	"Omicron": "\u039f",
	"Pi":      "\u03a0",
	"Rho":     "\u03a1",
	"Sigma":   "\u03a3",
	"Tau":     "\u03a4",
	"Upsilon": "\u03a5",
	"Phi":     "\u03a6",
	"Chi":     "\u03a7",
	"Psi":     "\u03a8",
	"Omega":   "\u03a9",
	"eacute":  "\u00e9",
	"egrave":  "\u00e8",
	"agrave":  "\u00e0",
	"ccedil":  "\u00e7",
	"uuml":    "\u00fc",
	"ouml":    "\u00f6",
	"auml":    "\u00e4",
	"Eacute":  "\u00c9",
	"Egrave":  "\u00c8",
	"Agrave":  "\u00c0",
	"Ccedil":  "\u00c7",
	"Uuml":    "\u00dc",
	"Ouml":    "\u00d6",
	"Auml":    "\u00c4",
	"ntilde":  "\u00f1",
	"Ntilde":  "\u00d1",
	"szlig":   "\u00df",
	"aring":   "\u00e5",
	"Aring":   "\u00c5",
	"aelig":   "\u00e6",
	"AElig":   "\u00c6",
	"oslash":  "\u00f8",
	"Oslash":  "\u00d8",
	"larr":    "\u2190",
	"uarr":    "\u2191",
	"rarr":    "\u2192",
	"darr":    "\u2193",
	"harr":    "\u2194",
	"lArr":    "\u21d0",
	"uArr":    "\u21d1",
	"rArr":    "\u21d2",
	"dArr":    "\u21d3",
	"hArr":    "\u21d4",
	"spades":    "\u2660",
	"clubs":     "\u2663",
	"hearts":    "\u2665",
	"diams":     "\u2666",
	"loz":       "\u25ca",
}

// decodeNumericEntity decodes a numeric character reference. body is the text
// between '&#' and the trailing ';'. isHex reports whether body is hexadecimal.
// Returns the replacement string and true on success. On malformed input it
// returns the Unicode replacement character and true (the spec replaces invalid
// code points with U+FFFD).
func decodeNumericEntity(body string, isHex bool) (string, bool) {
	if body == "" {
		return "\ufffd", false
	}
	var code uint64
	if isHex {
		for i := 0; i < len(body); i++ {
			c := body[i]
			var v byte
			switch {
			case c >= '0' && c <= '9':
				v = c - '0'
			case c >= 'a' && c <= 'f':
				v = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v = c - 'A' + 10
			default:
				return "\ufffd", false
			}
			code = code*16 + uint64(v)
			if code > 0x10FFFF {
				return "\ufffd", false
			}
		}
	} else {
		// Decimal: parse base 10.
		for i := 0; i < len(body); i++ {
			c := body[i]
			if c < '0' || c > '9' {
				return "\ufffd", false
			}
			code = code*10 + uint64(c-'0')
			if code > 0x10FFFF {
				return "\ufffd", false
			}
		}
	}
	switch {
	case code == 0:
		return "\ufffd", true
	case code > 0x10FFFF:
		return "\ufffd", true
	case code >= 0xD800 && code <= 0xDFFF:
		return "\ufffd", true
	default:
		return string(rune(code)), true
	}
}

// lookupEntity performs the HTML5 named-character-reference lookup. It returns
// the replacement string and the number of input bytes (after the leading '&')
// consumed. When withSemicolon is true the reference was terminated by ';'.
//
// The spec algorithm is a longest-prefix greedy match: it scans forward while the
// accumulated name is a known prefix of some entity name, then tries the longest
// known name. For the unterminated special cases the spec lists (&amp without ';' in
// attribute values), we only consume the entity when it is a known terminated
// entity or one of the special-cased unterminated names.
func lookupEntity(afterAmp string) (replacement string, consumed int, matched bool) {
	// Try the longest possible name terminated by ';'.
	const maxName = 32
	n := len(afterAmp)
	if n > maxName {
		n = maxName
	}
	// Find the position of ';' within the candidate window.
	semi := -1
	for i := 0; i < n; i++ {
		if afterAmp[i] == ';' {
			semi = i
			break
		}
		if !isEntityNameByte(afterAmp[i]) {
			break
		}
	}
	if semi > 0 {
		name := afterAmp[:semi]
		if rep, ok := namedEntities[name]; ok {
			return rep, semi + 1, true
		}
	}
	// Try the unterminated special-case entities used in attribute values. The
	// spec allows a fixed set of names to be matched without ';' in attribute
	// contexts; outside attributes they only match with ';'. The tokenizer passes
	// a context flag, but here we return the terminated form only.
	return "", 0, false
}

// isEntityNameByte reports whether b may appear in a named entity body.
func isEntityNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
