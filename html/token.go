// Translation of: Source/WebCore/html/parser/HTMLToken.h
// Completeness: 80%
// Simplifications:
//   - char16_t data vectors collapsed to Go string (UTF-8)
//   - Attribute name/value are plain strings
//   - DoctypeData uses strings for public/system identifiers
//   - 8-bit data tracking is omitted (Go strings carry their own encoding)

package html

// TokenType mirrors HTMLToken::Type.
type TokenType uint8

// Token type constants, mirroring HTMLToken::Type.
const (
	TokenUninitialized TokenType = iota
	TokenDoctype
	TokenStartTag
	TokenEndTag
	TokenComment
	TokenCharacter
	TokenEOF
)

// Attribute mirrors HTMLToken::Attribute: a name/value pair. Names are stored
// lower-cased by the tokenizer (matching the spec's attribute-name normalization),
// values keep their original case.
type Attribute struct {
	Name  string
	Value string
}

// DoctypeData mirrors WebCore::DoctypeData.
type DoctypeData struct {
	PublicIdentifier  string
	SystemIdentifier  string
	HasPublicID       bool
	HasSystemID       bool
	ForceQuirks       bool
}

// Token mirrors WebCore::HTMLToken. It is the unit of data passed from the
// tokenizer to the tree builder. The Data field holds the tag name (for
// StartTag/EndTag/DOCTYPE), comment text (for Comment) or character data (for
// Character).
type Token struct {
	Type         TokenType
	Data         string
	Attributes   []Attribute
	SelfClosing  bool
	DoctypeData  *DoctypeData
}

// clear resets the token to the uninitialized state, mirroring HTMLToken::clear().
func (t *Token) clear() {
	t.Type = TokenUninitialized
	t.Data = ""
	t.Attributes = nil
	t.SelfClosing = false
	t.DoctypeData = nil
}

// makeEndOfFile turns the token into an EOF token, mirroring makeEndOfFile().
func (t *Token) makeEndOfFile() {
	t.Type = TokenEOF
}

// beginDOCTYPE starts a DOCTYPE token, mirroring beginDOCTYPE().
func (t *Token) beginDOCTYPE() {
	t.Type = TokenDoctype
	t.DoctypeData = &DoctypeData{}
}

// appendToName appends a character to the token's name, mirroring appendToName().
func (t *Token) appendToName(r rune) {
	t.Data += string(r)
}

// setForceQuirks marks the current DOCTYPE token as forcing quirks mode.
func (t *Token) setForceQuirks() {
	if t.DoctypeData != nil {
		t.DoctypeData.ForceQuirks = true
	}
}

// beginStartTag starts a StartTag token with the first character of the tag name.
func (t *Token) beginStartTag(r rune) {
	t.Type = TokenStartTag
	t.SelfClosing = false
	t.Attributes = nil
	t.Data = string(r)
}

// beginEndTag starts an EndTag token with the first character of the tag name.
func (t *Token) beginEndTag(r rune) {
	t.Type = TokenEndTag
	t.SelfClosing = false
	t.Attributes = nil
	t.Data = string(r)
}

// beginAttribute starts a new attribute on the current tag token.
func (t *Token) beginAttribute() {
	t.Attributes = append(t.Attributes, Attribute{})
}

// appendToAttributeName appends r to the current attribute's name.
func (t *Token) appendToAttributeName(r rune) {
	if len(t.Attributes) == 0 {
		return
	}
	t.Attributes[len(t.Attributes)-1].Name += string(r)
}

// appendToAttributeValue appends r to the current attribute's value.
func (t *Token) appendToAttributeValue(r rune) {
	if len(t.Attributes) == 0 {
		return
	}
	t.Attributes[len(t.Attributes)-1].Value += string(r)
}

// appendToAttributeValueString appends s to the current attribute's value.
func (t *Token) appendToAttributeValueString(s string) {
	if len(t.Attributes) == 0 {
		return
	}
	t.Attributes[len(t.Attributes)-1].Value += s
}

// appendToCharacter starts or extends a Character token with r.
func (t *Token) appendToCharacter(r rune) {
	t.Type = TokenCharacter
	t.Data += string(r)
}

// appendToCharacterString starts or extends a Character token with s.
func (t *Token) appendToCharacterString(s string) {
	t.Type = TokenCharacter
	t.Data += s
}

// beginComment starts a Comment token.
func (t *Token) beginComment() {
	t.Type = TokenComment
}

// appendToComment appends r to the comment text.
func (t *Token) appendToComment(r rune) {
	t.Data += string(r)
}

// attrValue returns the value of the named attribute on a tag token, or "".
func (t *Token) attrValue(name string) string {
	for _, a := range t.Attributes {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

// hasAttribute reports whether the tag token carries the named attribute.
func (t *Token) hasAttribute(name string) bool {
	for _, a := range t.Attributes {
		if a.Name == name {
			return true
		}
	}
	return false
}
