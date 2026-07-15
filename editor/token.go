// Token represents a single syntactic token produced by the Monarch
// tokenizer. Mirrors VS Code's monaco.languages.Token.
//
// A token has a range [StartIndex, EndIndex) in the source text (rune
// offsets) and a list of scopes describing what the token is (e.g.
// ["keyword", "control", "go"] for a Go control-flow keyword like "if").

package editor

import "strings"

// Token is a single syntactic token.
type Token struct {
	// StartIndex is the start position of the token in the source (rune offset).
	StartIndex int
	// EndIndex is the end position of the token (exclusive, rune offset).
	EndIndex int
	// Scopes is the list of scope names, ordered from most general to
	// most specific (e.g. ["keyword", "control", "go"]). The scopes
	// determine the highlighting style applied to this token.
	Scopes []string
}

// NewToken creates a Token with the given range and scopes.
func NewToken(start, end int, scopes ...string) Token {
	scopesCopy := make([]string, len(scopes))
	copy(scopesCopy, scopes)
	return Token{StartIndex: start, EndIndex: end, Scopes: scopesCopy}
}

// Length returns the length of the token in runes.
func (t Token) Length() int { return t.EndIndex - t.StartIndex }

// Empty reports whether the token has zero length.
func (t Token) Empty() bool { return t.StartIndex == t.EndIndex }

// HasScope reports whether the token's scopes start with the given
// dot-separated scope prefix. This follows TextMate scope matching:
// HasScope("markup.heading") matches scopes ["markup","heading","atx","md"]
// because the first two elements match "markup" and "heading".
func (t Token) HasScope(scope string) bool {
	if scope == "" {
		return true
	}
	parts := strings.Split(scope, ".")
	if len(parts) > len(t.Scopes) {
		return false
	}
	for i, p := range parts {
		if t.Scopes[i] != p {
			return false
		}
	}
	return true
}

// ScopeString returns the scopes joined by "." (e.g. "keyword.control.go").
func (t Token) ScopeString() string {
	if len(t.Scopes) == 0 {
		return ""
	}
	result := t.Scopes[0]
	for _, s := range t.Scopes[1:] {
		result += "." + s
	}
	return result
}

// TokenLine represents all tokens on a single line, plus the line's
// text and offset. Used by the highlighter to apply styles to ranges.
type TokenLine struct {
	// Tokens are the tokens on this line, in order.
	Tokens []Token
	// Text is the line's text (without the trailing newline).
	Text string
	// StartOffset is the rune offset of the start of this line in the
	// full document.
	StartOffset int
}
