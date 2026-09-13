// Translation of: Source/WebCore/html/parser/HTMLTokenizer.cpp
//                  Source/WebCore/html/parser/HTMLTokenizer.h
//                  Source/WebCore/html/parser/InputStreamPreprocessor.h
// Completeness: 75%
// Simplifications:
//   - RCData rules folded into RAWTEXT where the two differ only in entity handling
//   - SegmentedString is replaced by a position index over a decoded []rune buffer
//   - the InputStreamPreprocessor's CR/LF normalization and null replacement are
//     implemented in nextChar directly
//   - the speculative script end-tag buffering is implemented with a simple
//     temporary buffer instead of the SegmentedString rewind machinery

package html

// tokenizerState mirrors the HTMLTokenizer::State enum.
type tokenizerState uint8

// State constants, mirroring HTMLTokenizer.h State. Order follows the spec.
const (
	stateData tokenizerState = iota
	stateCharacterReferenceInData
	stateRCDATA
	stateCharacterReferenceInRCDATA
	stateRAWTEXT
	stateScriptData
	statePLAINTEXT
	stateTagOpen
	stateEndTagOpen
	stateTagName
	stateRCDATALessThanSign
	stateRCDATAEndTagOpen
	stateRCDATAEndTagName
	stateRAWTEXTLessThanSign
	stateRAWTEXTEndTagOpen
	stateRAWTEXTEndTagName
	stateScriptDataLessThanSign
	stateScriptDataEndTagOpen
	stateScriptDataEndTagName
	stateScriptDataEscapeStart
	stateScriptDataEscapeStartDash
	stateScriptDataEscaped
	stateScriptDataEscapedDash
	stateScriptDataEscapedDashDash
	stateScriptDataEscapedLessThanSign
	stateScriptDataEscapedEndTagOpen
	stateScriptDataEscapedEndTagName
	stateScriptDataDoubleEscapeStart
	stateScriptDataDoubleEscaped
	stateScriptDataDoubleEscapedDash
	stateScriptDataDoubleEscapedDashDash
	stateScriptDataDoubleEscapedLessThanSign
	stateScriptDataDoubleEscapeEnd
	stateBeforeAttributeName
	stateAttributeName
	stateAfterAttributeName
	stateBeforeAttributeValue
	stateAttributeValueDoubleQuoted
	stateAttributeValueSingleQuoted
	stateAttributeValueUnquoted
	stateCharacterReferenceInAttributeValue
	stateAfterAttributeValueQuoted
	stateSelfClosingStartTag
	stateBogusComment
	stateMarkupDeclarationOpen
	stateCommentStart
	stateCommentStartDash
	stateComment
	stateCommentEndDash
	stateCommentEnd
	stateCommentEndBang
	stateDOCTYPE
	stateBeforeDOCTYPEName
	stateDOCTYPEName
	stateAfterDOCTYPEName
	stateAfterDOCTYPEPublicKeyword
	stateBeforeDOCTYPEPublicIdentifier
	stateDOCTYPEPublicIdentifierDoubleQuoted
	stateDOCTYPEPublicIdentifierSingleQuoted
	stateAfterDOCTYPEPublicIdentifier
	stateBetweenDOCTYPEPublicAndSystemIdentifiers
	stateAfterDOCTYPESystemKeyword
	stateBeforeDOCTYPESystemIdentifier
	stateDOCTYPESystemIdentifierDoubleQuoted
	stateDOCTYPESystemIdentifierSingleQuoted
	stateAfterDOCTYPESystemIdentifier
	stateBogusDOCTYPE
	stateCDATASection
	stateCDATASectionRightSquareBracket
	stateCDATASectionDoubleRightSquareBracket
)

// Tokenizer mirrors HTMLTokenizer. It reads from a decoded []rune buffer and
// produces a stream of Token values via NextToken.
type Tokenizer struct {
	src                  []rune
	pos                  int
	state                tokenizerState
	token                Token
	emitted              *Token
	tokenReady           bool
	seenEOF              bool
	temporaryBuffer      []rune
	appropriateEndTagName string
	charRefResumeState   tokenizerState
	shouldAllowCDATA     bool
}

// NewTokenizer creates a Tokenizer that reads from src, starting in the data
// state, mirroring HTMLTokenizer::reset.
func NewTokenizer(src string) *Tokenizer {
	return &Tokenizer{
		src:   []rune(src),
		state: stateData,
	}
}

// setDataState switches the tokenizer to the data state (normal text).
func (t *Tokenizer) setDataState()       { t.state = stateData }
func (t *Tokenizer) setRCDATAState()     { t.state = stateRCDATA }
func (t *Tokenizer) setRAWTEXTState()    { t.state = stateRAWTEXT }
func (t *Tokenizer) setScriptDataState() { t.state = stateScriptData }
func (t *Tokenizer) setPLAINTEXTState()  { t.state = statePLAINTEXT }

// NextToken consumes input until a complete token is ready, mirroring
// HTMLTokenizer::nextToken. It returns nil only at end of stream after EOF has
// already been emitted.
func (t *Tokenizer) NextToken() *Token {
	if t.tokenReady && t.emitted != nil {
		return t.takeEmitted()
	}
	for {
		if t.tokenReady {
			return t.takeEmitted()
		}
		c, ok := t.nextChar()
		if !ok {
			t.handleEOF()
			if t.tokenReady {
				return t.takeEmitted()
			}
			continue
		}
		t.processChar(c)
	}
}

// takeEmitted returns the ready token and resets the ready flag.
// ★ 发出前 flush 构建缓冲（属性值/文本/注释按 O(总长) 一次性拼装）。
func (t *Tokenizer) takeEmitted() *Token {
	out := t.emitted
	if out != nil {
		out.flushBuffers()
	}
	t.emitted = nil
	t.tokenReady = false
	return out
}

// emitToken emits tok and arranges NextToken to return it.
func (t *Tokenizer) emitToken(tok Token) {
	cp := tok
	t.emitted = &cp
	t.tokenReady = true
}

// emitCurrentToken emits the in-progress token and clears it.
func (t *Tokenizer) emitCurrentToken() {
	t.emitToken(t.token)
	t.token.clear()
}

// flushText emits any accumulated character data as a Character token. It is
// called before the tokenizer begins building a tag/comment/doctype token, so
// that text preceding a tag is not lost when the shared token is cleared.
func (t *Tokenizer) flushText() {
	if t.token.Type == TokenCharacter && (t.token.Data != "" || len(t.token.dataBuf) > 0) {
		t.emitCurrentToken()
	}
}

// resumeInDataState emits the current token and switches to the data state.
func (t *Tokenizer) resumeInDataState() {
	t.emitCurrentToken()
	t.state = stateData
}

// nextChar returns the next input character with CR/LF normalization applied,
// mirroring InputStreamPreprocessor::nextInput. CR followed by LF collapses to a
// single LF; a lone CR becomes LF. EOF is reported as (0, false).
func (t *Tokenizer) nextChar() (rune, bool) {
	if t.pos >= len(t.src) {
		return 0, false
	}
	c := t.src[t.pos]
	t.pos++
	if c == '\r' {
		if t.pos < len(t.src) && t.src[t.pos] == '\n' {
			t.pos++
		}
		return '\n', true
	}
	return c, true
}

// peek returns the next input character without consuming it (after CR/LF
// normalization). EOF returns (0, false).
func (t *Tokenizer) peek() (rune, bool) {
	if t.pos >= len(t.src) {
		return 0, false
	}
	c := t.src[t.pos]
	if c == '\r' {
		c = '\n'
	}
	return c, true
}

// reconsume pushes c back so the next nextChar returns it.
func (t *Tokenizer) reconsume(c rune) {
	if t.pos > 0 {
		t.pos--
	}
}

// isWhitespace reports whether c is HTML whitespace, per the spec (ASCII tab/LF/
// FF/CR/space).
func isWhitespace(c rune) bool {
	switch c {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// isASCIIUpper / isASCIILower mirror the ASCII letter classification used by the
// tokenizer (lower-casing of tag names is done here).
func isASCIIUpper(c rune) bool { return c >= 'A' && c <= 'Z' }
func isASCIILower(c rune) bool { return c >= 'a' && c <= 'z' }
func isASCIIAlpha(c rune) bool { return isASCIIUpper(c) || isASCIILower(c) }
func isASCIIDigit(c rune) bool { return c >= '0' && c <= '9' }
func isASCIIAlnum(c rune) bool { return isASCIIAlpha(c) || isASCIIDigit(c) }

// toLowerASCIIRune lower-cases an ASCII letter rune.
func toLowerASCIIRune(c rune) rune {
	if isASCIIUpper(c) {
		return c + ('a' - 'A')
	}
	return c
}

// toLowerASCII lower-cases an ASCII string.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// isHexDigit reports whether b is a hexadecimal digit.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// handleEOF mirrors the EOF handling in HTMLTokenizer. If there is buffered
// character data it is emitted first; otherwise an EOF token is emitted. After
// the first call seenEOF prevents re-entry.
func (t *Tokenizer) handleEOF() {
	if t.seenEOF {
		t.tokenReady = false
		return
	}
	switch t.state {
	case stateData, stateRCDATA, stateRAWTEXT, stateScriptData, statePLAINTEXT,
		stateCDATASection, stateCDATASectionRightSquareBracket,
		stateCDATASectionDoubleRightSquareBracket:
		if t.token.Type == TokenCharacter && (t.token.Data != "" || len(t.token.dataBuf) > 0) {
			t.emitCurrentToken()
			return
		}
	}
	switch t.state {
		case stateComment, stateCommentStart, stateCommentStartDash,
			stateCommentEndDash, stateCommentEnd, stateCommentEndBang,
			stateBogusComment:
			if t.token.Type == TokenComment {
				t.emitCurrentToken()
				return
			}
	}
	t.seenEOF = true
	t.token.clear()
	t.token.makeEndOfFile()
	t.emitCurrentToken()
}

// processEntity consumes a character reference starting after the '&' and emits
// its replacement into the current token. When inAttribute is true the decoded
// text is appended to the current attribute's value (mirroring the HTML spec's
// "character reference in attribute value" state); otherwise it is appended to
// the character data. The inAttribute flag also selects the attribute-value
// entity rules (which allow a few unterminated names).
func (t *Tokenizer) processEntity(inAttribute bool) {
	// t.src[t.pos] is the char after '&'.
	if t.pos < len(t.src) && t.src[t.pos] == '#' {
		t.pos++
		isHex := false
		if t.pos < len(t.src) && (t.src[t.pos] == 'x' || t.src[t.pos] == 'X') {
			t.pos++
			isHex = true
		}
		var body []byte
		for t.pos < len(t.src) {
			c := t.src[t.pos]
			if c == ';' {
				t.pos++
				break
			}
			if isHex && isHexDigit(byte(c)) {
				body = append(body, byte(c))
				t.pos++
			} else if !isHex && c >= '0' && c <= '9' {
				body = append(body, byte(c))
				t.pos++
			} else {
				break
			}
		}
		rep, _ := decodeNumericEntity(string(body), isHex)
		t.appendEntityResult(rep, inAttribute)
		return
	}
	// Named entity: try terminated form first.
	start := t.pos
	for t.pos < len(t.src) && isEntityNameByte(byte(t.src[t.pos])) {
		t.pos++
	}
	if t.pos < len(t.src) && t.src[t.pos] == ';' {
		name := string(t.src[start:t.pos])
		t.pos++
		if rep, ok := namedEntities[name]; ok {
			t.appendEntityResult(rep, inAttribute)
			return
		}
		// Unknown terminated entity: emit '&' literally and reconsume the rest.
		t.pos = start
		t.appendEntityAmp(inAttribute)
		return
	}
	// Unterminated: only a few names are matched in attribute context; otherwise
	// treat '&' literally.
	candidate := ""
	if t.pos > start {
		candidate = string(t.src[start:t.pos])
	}
	if inAttribute {
		if rep, ok := unterminatedAttrEntity(candidate); ok {
			t.appendEntityResult(rep, inAttribute)
			return
		}
	}
	t.pos = start
	t.appendEntityAmp(inAttribute)
}

// appendEntityResult appends decoded entity text either to the current
// attribute value (inAttribute) or to the character data.
func (t *Tokenizer) appendEntityResult(rep string, inAttribute bool) {
	if inAttribute {
		t.token.appendToAttributeValueString(rep)
	} else {
		t.token.appendToCharacterString(rep)
	}
}

// appendEntityAmp appends a literal '&' either to the current attribute value
// (inAttribute) or to the character data.
func (t *Tokenizer) appendEntityAmp(inAttribute bool) {
	if inAttribute {
		t.token.appendToAttributeValue('&')
	} else {
		t.token.appendToCharacter('&')
	}
}

// unterminatedAttrEntity returns the replacement for a named entity that the
// spec allows to match without a trailing ';' in attribute values. Outside
// attribute context these names only match with the semicolon.
func unterminatedAttrEntity(name string) (string, bool) {
	switch name {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "quot":
		return "\"", true
	case "apos":
		return "'", true
	}
	return "", false
}