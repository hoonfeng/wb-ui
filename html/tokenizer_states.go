// Translation of: Source/WebCore/html/parser/HTMLTokenizer.cpp (state machine body)
// Completeness: 75%
// Simplifications:
//   - processChar dispatches all 72 spec states via a switch
//   - the script end-tag speculative matching uses the temporaryBuffer directly
//   - appropriateEndTagName is auto-populated on start-tag emission as an
//     approximation of the tree-builder-driven updateStateFor flow, so the
//     tokenizer is usable standalone in tests

package html

// processChar dispatches the preprocessed character c through the current
// tokenizer state. It mutates t.token / t.state and may set t.tokenReady by
// emitting a token.
func (t *Tokenizer) processChar(c rune) {
	switch t.state {
	case stateData:
		t.dataState(c)
	case stateRCDATA:
		t.rcdataState(c)
	case stateRAWTEXT:
		t.rawtextState(c)
	case stateScriptData:
		t.scriptDataState(c)
	case statePLAINTEXT:
		t.plaintextState(c)
	case stateCharacterReferenceInData:
		t.charRefInDataState(c)
	case stateCharacterReferenceInRCDATA:
		t.charRefInRCDATAState(c)
	case stateTagOpen:
		t.tagOpenState(c)
	case stateEndTagOpen:
		t.endTagOpenState(c)
	case stateTagName:
		t.tagNameState(c)
	case stateRCDATALessThanSign:
		t.rcdataLessThanSignState(c)
	case stateRCDATAEndTagOpen:
		t.rcdataEndTagOpenState(c)
	case stateRCDATAEndTagName:
		t.rcdataEndTagNameState(c)
	case stateRAWTEXTLessThanSign:
		t.rawtextLessThanSignState(c)
	case stateRAWTEXTEndTagOpen:
		t.rawtextEndTagOpenState(c)
	case stateRAWTEXTEndTagName:
		t.rawtextEndTagNameState(c)
	case stateScriptDataLessThanSign:
		t.scriptDataLessThanSignState(c)
	case stateScriptDataEndTagOpen:
		t.scriptDataEndTagOpenState(c)
	case stateScriptDataEndTagName:
		t.scriptDataEndTagNameState(c)
	case stateScriptDataEscapeStart:
		t.scriptDataEscapeStartState(c)
	case stateScriptDataEscapeStartDash:
		t.scriptDataEscapeStartDashState(c)
	case stateScriptDataEscaped:
		t.scriptDataEscapedState(c)
	case stateScriptDataEscapedDash:
		t.scriptDataEscapedDashState(c)
	case stateScriptDataEscapedDashDash:
		t.scriptDataEscapedDashDashState(c)
	case stateScriptDataEscapedLessThanSign:
		t.scriptDataEscapedLessThanSignState(c)
	case stateScriptDataEscapedEndTagOpen:
		t.scriptDataEscapedEndTagOpenState(c)
	case stateScriptDataEscapedEndTagName:
		t.scriptDataEscapedEndTagNameState(c)
	case stateScriptDataDoubleEscapeStart:
		t.scriptDataDoubleEscapeStartState(c)
	case stateScriptDataDoubleEscaped:
		t.scriptDataDoubleEscapedState(c)
	case stateScriptDataDoubleEscapedDash:
		t.scriptDataDoubleEscapedDashState(c)
	case stateScriptDataDoubleEscapedDashDash:
		t.scriptDataDoubleEscapedDashDashState(c)
	case stateScriptDataDoubleEscapedLessThanSign:
		t.scriptDataDoubleEscapedLessThanSignState(c)
	case stateScriptDataDoubleEscapeEnd:
		t.scriptDataDoubleEscapeEndState(c)
	case stateBeforeAttributeName:
		t.beforeAttributeNameState(c)
	case stateAttributeName:
		t.attributeNameState(c)
	case stateAfterAttributeName:
		t.afterAttributeNameState(c)
	case stateBeforeAttributeValue:
		t.beforeAttributeValueState(c)
	case stateAttributeValueDoubleQuoted:
		t.attributeValueDoubleQuotedState(c)
	case stateAttributeValueSingleQuoted:
		t.attributeValueSingleQuotedState(c)
	case stateAttributeValueUnquoted:
		t.attributeValueUnquotedState(c)
	case stateCharacterReferenceInAttributeValue:
		t.charRefInAttributeValueState(c)
	case stateAfterAttributeValueQuoted:
		t.afterAttributeValueQuotedState(c)
	case stateSelfClosingStartTag:
		t.selfClosingStartTagState(c)
	case stateBogusComment:
		t.bogusCommentState(c)
	case stateMarkupDeclarationOpen:
		t.markupDeclarationOpenState(c)
	case stateCommentStart:
		t.commentStartState(c)
	case stateCommentStartDash:
		t.commentStartDashState(c)
	case stateComment:
		t.commentState(c)
	case stateCommentEndDash:
		t.commentEndDashState(c)
	case stateCommentEnd:
		t.commentEndState(c)
	case stateCommentEndBang:
		t.commentEndBangState(c)
	case stateDOCTYPE:
		t.doctypeState(c)
	case stateBeforeDOCTYPEName:
		t.beforeDOCTYPENameState(c)
	case stateDOCTYPEName:
		t.doctypeNameState(c)
	case stateAfterDOCTYPEName:
		t.afterDOCTYPENameState(c)
	case stateAfterDOCTYPEPublicKeyword:
		t.afterDOCTYPEPublicKeywordState(c)
	case stateBeforeDOCTYPEPublicIdentifier:
		t.beforeDOCTYPEPublicIdentifierState(c)
	case stateDOCTYPEPublicIdentifierDoubleQuoted:
		t.doctypePublicIdentifierDoubleQuotedState(c)
	case stateDOCTYPEPublicIdentifierSingleQuoted:
		t.doctypePublicIdentifierSingleQuotedState(c)
	case stateAfterDOCTYPEPublicIdentifier:
		t.afterDOCTYPEPublicIdentifierState(c)
	case stateBetweenDOCTYPEPublicAndSystemIdentifiers:
		t.betweenDOCTYPEPublicAndSystemIdentifiersState(c)
	case stateAfterDOCTYPESystemKeyword:
		t.afterDOCTYPESystemKeywordState(c)
	case stateBeforeDOCTYPESystemIdentifier:
		t.beforeDOCTYPESystemIdentifierState(c)
	case stateDOCTYPESystemIdentifierDoubleQuoted:
		t.doctypeSystemIdentifierDoubleQuotedState(c)
	case stateDOCTYPESystemIdentifierSingleQuoted:
		t.doctypeSystemIdentifierSingleQuotedState(c)
	case stateAfterDOCTYPESystemIdentifier:
		t.afterDOCTYPESystemIdentifierState(c)
	case stateBogusDOCTYPE:
		t.bogusDOCTYPEState(c)
	case stateCDATASection:
		t.cdataSectionState(c)
	case stateCDATASectionRightSquareBracket:
		t.cdataSectionRightSquareBracketState(c)
	case stateCDATASectionDoubleRightSquareBracket:
		t.cdataSectionDoubleRightSquareBracketState(c)
	}
}

// flushBufferedEndTag emits the contents of the temporary buffer (the '</' plus
// accumulated end-tag name) as character data and reconsumes the current
// character in the originating raw-text state. It is the "anything else" branch
// of the speculative end-tag states.
func (t *Tokenizer) flushBufferedEndTag(rawState tokenizerState) {
	t.token.appendToCharacterString("</")
	for _, r := range t.temporaryBuffer {
		t.token.appendToCharacter(r)
	}
	t.temporaryBuffer = t.temporaryBuffer[:0]
	t.token.Type = TokenCharacter
	t.state = rawState
}

// isAppropriateEndTag reports whether the buffered end-tag name matches the
// appropriate end tag name (the most recent start tag).
func (t *Tokenizer) isAppropriateEndTag() bool {
	return t.appropriateEndTagName != "" &&
		string(t.temporaryBuffer) == t.appropriateEndTagName
}

// --- Text states -------------------------------------------------------

func (t *Tokenizer) dataState(c rune) {
	switch {
	case c == '&':
		t.state = stateCharacterReferenceInData
	case c == '<':
		t.state = stateTagOpen
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) rcdataState(c rune) {
	switch {
	case c == '&':
		t.state = stateCharacterReferenceInRCDATA
	case c == '<':
		t.state = stateRCDATALessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) rawtextState(c rune) {
	switch {
	case c == '<':
		t.state = stateRAWTEXTLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) scriptDataState(c rune) {
	switch {
	case c == '<':
		t.state = stateScriptDataLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) plaintextState(c rune) {
	if c == 0 {
		t.token.appendToCharacter('\ufffd')
	} else {
		t.token.appendToCharacter(c)
	}
}

// --- Character reference states ----------------------------------------

func (t *Tokenizer) charRefInDataState(c rune) {
	// c is the first char after '&'. Reconsume it so processEntity reads from it.
	t.reconsume(c)
	t.processEntity(false)
	t.state = stateData
}

func (t *Tokenizer) charRefInRCDATAState(c rune) {
	// c is the first char after '&'. Reconsume it so processEntity reads from it.
	t.reconsume(c)
	t.processEntity(false)
	t.state = stateRCDATA
}

func (t *Tokenizer) charRefInAttributeValueState(c rune) {
	// c is the first char after '&'. Reconsume it so processEntity reads from it.
	t.reconsume(c)
	t.processEntity(true)
	t.state = t.charRefResumeState
}

// --- Tag open / name states --------------------------------------------

func (t *Tokenizer) tagOpenState(c rune) {
	switch {
	case c == '!':
		t.state = stateMarkupDeclarationOpen
	case c == '/':
		t.state = stateEndTagOpen
	case isASCIIAlpha(c):
		t.flushText()
		t.token.clear()
		t.token.beginStartTag(toLowerASCIIRune(c))
		t.state = stateTagName
	default:
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateData
	}
}

func (t *Tokenizer) endTagOpenState(c rune) {
	switch {
	case isASCIIAlpha(c):
		t.flushText()
		t.token.clear()
		t.token.beginEndTag(toLowerASCIIRune(c))
		t.state = stateTagName
	case c == '>':
		// missing end tag name: parse error, ignore
		t.state = stateData
	default:
		t.flushText()
		t.token.clear()
		t.token.beginComment()
		t.reconsume(c)
		t.state = stateBogusComment
	}
}

func (t *Tokenizer) tagNameState(c rune) {
	switch {
	case isWhitespace(c):
		t.state = stateBeforeAttributeName
	case c == '/':
		t.state = stateSelfClosingStartTag
	case c == '>':
		t.emitCurrentTag()
		t.state = stateData
	case isASCIIUpper(c):
		t.token.appendToName(toLowerASCIIRune(c))
	case c == 0:
		t.token.appendToName('\ufffd')
	default:
		t.token.appendToName(c)
	}
}

// emitCurrentTag handles start/end tag emission, including auto-populating
// appropriateEndTagName for start tags (approximating the tree builder's
// updateStateFor so the tokenizer is usable standalone).
func (t *Tokenizer) emitCurrentTag() {
	if t.token.Type == TokenStartTag {
		t.appropriateEndTagName = t.token.Data
	}
	t.emitCurrentToken()
}

// --- RCDATA end-tag speculative states ---------------------------------

func (t *Tokenizer) rcdataLessThanSignState(c rune) {
	if c == '/' {
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateRCDATAEndTagOpen
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateRCDATA
	}
}

func (t *Tokenizer) rcdataEndTagOpenState(c rune) {
	if isASCIIAlpha(c) {
		t.flushText()
		t.token.clear()
		t.token.beginEndTag(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer[:0], toLowerASCIIRune(c))
		t.state = stateRCDATAEndTagName
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateRCDATA
	}
}

func (t *Tokenizer) rcdataEndTagNameState(c rune) {
	switch {
	case isWhitespace(c):
		if t.isAppropriateEndTag() {
			t.state = stateBeforeAttributeName
		} else {
			t.flushBufferedEndTag(stateRCDATA)
			t.reconsume(c)
		}
	case c == '/':
		if t.isAppropriateEndTag() {
			t.state = stateSelfClosingStartTag
		} else {
			t.flushBufferedEndTag(stateRCDATA)
			t.reconsume(c)
		}
	case c == '>':
		if t.isAppropriateEndTag() {
			t.emitCurrentTag()
			t.state = stateData
		} else {
			t.flushBufferedEndTag(stateRCDATA)
			t.reconsume(c)
		}
	case isASCIIAlpha(c):
		t.token.appendToName(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	default:
		t.flushBufferedEndTag(stateRCDATA)
		t.reconsume(c)
	}
}

// --- RAWTEXT end-tag speculative states --------------------------------

func (t *Tokenizer) rawtextLessThanSignState(c rune) {
	if c == '/' {
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateRAWTEXTEndTagOpen
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateRAWTEXT
	}
}

func (t *Tokenizer) rawtextEndTagOpenState(c rune) {
	if isASCIIAlpha(c) {
		t.flushText()
		t.token.clear()
		t.token.beginEndTag(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer[:0], toLowerASCIIRune(c))
		t.state = stateRAWTEXTEndTagName
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateRAWTEXT
	}
}

func (t *Tokenizer) rawtextEndTagNameState(c rune) {
	switch {
	case isWhitespace(c):
		if t.isAppropriateEndTag() {
			t.state = stateBeforeAttributeName
		} else {
			t.flushBufferedEndTag(stateRAWTEXT)
			t.reconsume(c)
		}
	case c == '/':
		if t.isAppropriateEndTag() {
			t.state = stateSelfClosingStartTag
		} else {
			t.flushBufferedEndTag(stateRAWTEXT)
			t.reconsume(c)
		}
	case c == '>':
		if t.isAppropriateEndTag() {
			t.emitCurrentTag()
			t.state = stateData
		} else {
			t.flushBufferedEndTag(stateRAWTEXT)
			t.reconsume(c)
		}
	case isASCIIAlpha(c):
		t.token.appendToName(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	default:
		t.flushBufferedEndTag(stateRAWTEXT)
		t.reconsume(c)
	}
}

// --- Script data states ------------------------------------------------

func (t *Tokenizer) scriptDataLessThanSignState(c rune) {
	switch {
	case c == '/':
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateScriptDataEndTagOpen
	case c == '!':
		t.token.appendToCharacter('<')
		t.token.appendToCharacter('!')
		t.state = stateScriptDataEscapeStart
	default:
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateScriptData
	}
}

func (t *Tokenizer) scriptDataEndTagOpenState(c rune) {
	if isASCIIAlpha(c) {
		t.flushText()
		t.token.clear()
		t.token.beginEndTag(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer[:0], toLowerASCIIRune(c))
		t.state = stateScriptDataEndTagName
	} else {
		t.token.appendToCharacter('<')
		t.token.appendToCharacter('/')
		t.reconsume(c)
		t.state = stateScriptData
	}
}

func (t *Tokenizer) scriptDataEndTagNameState(c rune) {
	switch {
	case isWhitespace(c):
		if t.isAppropriateEndTag() {
			t.state = stateBeforeAttributeName
		} else {
			t.flushBufferedEndTagScript()
			t.reconsume(c)
		}
	case c == '/':
		if t.isAppropriateEndTag() {
			t.state = stateSelfClosingStartTag
		} else {
			t.flushBufferedEndTagScript()
			t.reconsume(c)
		}
	case c == '>':
		if t.isAppropriateEndTag() {
			t.emitCurrentTag()
			t.state = stateData
		} else {
			t.flushBufferedEndTagScript()
			t.reconsume(c)
		}
	case isASCIIAlpha(c):
		t.token.appendToName(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	default:
		t.flushBufferedEndTagScript()
		t.reconsume(c)
	}
}

// flushBufferedEndTagScript emits the buffered '</name' as script-data
// characters and reconsumes in ScriptData.
func (t *Tokenizer) flushBufferedEndTagScript() {
	// ★ 先清除 token 中残留的 end-tag name（beginEndTag 已写入 Data），
	// 否则 "span" 会残留在字符流里（script 内容中 "</span>" 被错误
	// 输出为 "span</span>"，污染 JS 字符串字面量）。
	t.token.clear()
	t.token.appendToCharacterString("</")
	for _, r := range t.temporaryBuffer {
		t.token.appendToCharacter(r)
	}
	t.temporaryBuffer = t.temporaryBuffer[:0]
	t.token.Type = TokenCharacter
	t.state = stateScriptData
}

func (t *Tokenizer) scriptDataEscapeStartState(c rune) {
	if c == '-' {
		t.token.appendToCharacter('-')
		t.state = stateScriptDataEscapeStartDash
	} else {
		t.reconsume(c)
		t.state = stateScriptData
	}
}

func (t *Tokenizer) scriptDataEscapeStartDashState(c rune) {
	if c == '-' {
		t.token.appendToCharacter('-')
		t.state = stateScriptDataEscapedDashDash
	} else {
		t.reconsume(c)
		t.state = stateScriptData
	}
}

func (t *Tokenizer) scriptDataEscapedState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
		t.state = stateScriptDataEscapedDash
	case c == '<':
		t.state = stateScriptDataEscapedLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) scriptDataEscapedDashState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
		t.state = stateScriptDataEscapedDashDash
	case c == '<':
		t.state = stateScriptDataEscapedLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
		t.state = stateScriptDataEscaped
	}
}

func (t *Tokenizer) scriptDataEscapedDashDashState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
	case c == '<':
		t.state = stateScriptDataEscapedLessThanSign
	case c == '>':
		t.token.appendToCharacter('>')
		t.state = stateScriptData
	case c == 0:
		t.token.appendToCharacter('\ufffd')
		t.state = stateScriptDataEscaped
	default:
		t.token.appendToCharacter(c)
		t.state = stateScriptDataEscaped
	}
}

func (t *Tokenizer) scriptDataEscapedLessThanSignState(c rune) {
	if c == '/' {
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateScriptDataEscapedEndTagOpen
	} else if isASCIIAlpha(c) {
		t.token.appendToCharacter('<')
		t.token.appendToCharacter(c)
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateScriptDataDoubleEscapeStart
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateScriptDataEscaped
	}
}

func (t *Tokenizer) scriptDataEscapedEndTagOpenState(c rune) {
	if isASCIIAlpha(c) {
		t.flushText()
		t.token.clear()
		t.token.beginEndTag(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer[:0], toLowerASCIIRune(c))
		t.state = stateScriptDataEscapedEndTagName
	} else {
		t.token.appendToCharacter('<')
		t.token.appendToCharacter('/')
		t.reconsume(c)
		t.state = stateScriptDataEscaped
	}
}

func (t *Tokenizer) scriptDataEscapedEndTagNameState(c rune) {
	switch {
	case isWhitespace(c):
		if t.isAppropriateEndTag() {
			t.state = stateBeforeAttributeName
		} else {
			t.flushBufferedEndTagEscaped()
			t.reconsume(c)
		}
	case c == '/':
		if t.isAppropriateEndTag() {
			t.state = stateSelfClosingStartTag
		} else {
			t.flushBufferedEndTagEscaped()
			t.reconsume(c)
		}
	case c == '>':
		if t.isAppropriateEndTag() {
			t.emitCurrentTag()
			t.state = stateData
		} else {
			t.flushBufferedEndTagEscaped()
			t.reconsume(c)
		}
	case isASCIIAlpha(c):
		t.token.appendToName(toLowerASCIIRune(c))
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	default:
		t.flushBufferedEndTagEscaped()
		t.reconsume(c)
	}
}

// flushBufferedEndTagEscaped emits the buffered '</name' as script-data
// characters and reconsumes in ScriptDataEscaped.
func (t *Tokenizer) flushBufferedEndTagEscaped() {
	// ★ 与 flushBufferedEndTagScript 相同：清除残留 end-tag name，
	// 避免 "span" 残留在转义 script 文本中。
	t.token.clear()
	t.token.appendToCharacterString("</")
	for _, r := range t.temporaryBuffer {
		t.token.appendToCharacter(r)
	}
	t.temporaryBuffer = t.temporaryBuffer[:0]
	t.token.Type = TokenCharacter
	t.state = stateScriptDataEscaped
}

func (t *Tokenizer) scriptDataDoubleEscapeStartState(c rune) {
	switch {
	case isWhitespace(c) || c == '/' || c == '>':
		if string(t.temporaryBuffer) == "script" {
			t.state = stateScriptDataDoubleEscaped
		} else {
			t.state = stateScriptDataEscaped
		}
		t.token.appendToCharacter(c)
	case isASCIIUpper(c):
		t.token.appendToCharacter(c)
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	case isASCIILower(c):
		t.token.appendToCharacter(c)
		t.temporaryBuffer = append(t.temporaryBuffer, c)
	default:
		t.reconsume(c)
		t.state = stateScriptDataEscaped
	}
}

func (t *Tokenizer) scriptDataDoubleEscapedState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
		t.state = stateScriptDataDoubleEscapedDash
	case c == '<':
		t.state = stateScriptDataDoubleEscapedLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) scriptDataDoubleEscapedDashState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
		t.state = stateScriptDataDoubleEscapedDashDash
	case c == '<':
		t.state = stateScriptDataDoubleEscapedLessThanSign
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
		t.state = stateScriptDataDoubleEscaped
	}
}

func (t *Tokenizer) scriptDataDoubleEscapedDashDashState(c rune) {
	switch {
	case c == '-':
		t.token.appendToCharacter('-')
	case c == '<':
		t.state = stateScriptDataDoubleEscapedLessThanSign
	case c == '>':
		t.token.appendToCharacter('>')
		t.state = stateScriptData
	case c == 0:
		t.token.appendToCharacter('\ufffd')
		t.state = stateScriptDataDoubleEscaped
	default:
		t.token.appendToCharacter(c)
		t.state = stateScriptDataDoubleEscaped
	}
}

func (t *Tokenizer) scriptDataDoubleEscapedLessThanSignState(c rune) {
	if c == '/' {
		t.temporaryBuffer = t.temporaryBuffer[:0]
		t.state = stateScriptDataDoubleEscapeEnd
	} else {
		t.token.appendToCharacter('<')
		t.reconsume(c)
		t.state = stateScriptDataDoubleEscaped
	}
}

func (t *Tokenizer) scriptDataDoubleEscapeEndState(c rune) {
	switch {
	case isWhitespace(c) || c == '/' || c == '>':
		if string(t.temporaryBuffer) == "script" {
			t.state = stateScriptDataEscaped
		} else {
			t.state = stateScriptDataDoubleEscaped
		}
		t.token.appendToCharacter(c)
	case isASCIIUpper(c):
		t.token.appendToCharacter(c)
		t.temporaryBuffer = append(t.temporaryBuffer, toLowerASCIIRune(c))
	case isASCIILower(c):
		t.token.appendToCharacter(c)
		t.temporaryBuffer = append(t.temporaryBuffer, c)
	default:
		t.reconsume(c)
		t.state = stateScriptDataDoubleEscaped
	}
}

// --- Attribute states --------------------------------------------------

func (t *Tokenizer) beforeAttributeNameState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '/' || c == '>':
		t.reconsume(c)
		t.state = stateAfterAttributeName
	case c == '=':
		// parse error: start an attribute with '=' in the name
		t.token.beginAttribute()
		t.token.appendToAttributeName(c)
		t.state = stateAttributeName
	default:
		t.token.beginAttribute()
		t.reconsume(c)
		t.state = stateAttributeName
	}
}

func (t *Tokenizer) attributeNameState(c rune) {
	switch {
	case isWhitespace(c) || c == '/' || c == '>':
		t.reconsume(c)
		t.state = stateAfterAttributeName
	case c == '=':
		t.state = stateBeforeAttributeValue
	case isASCIIUpper(c):
		t.token.appendToAttributeName(toLowerASCIIRune(c))
	case c == 0:
		t.token.appendToAttributeName('\ufffd')
	case c == '"', c == '\'', c == '<':
		t.token.appendToAttributeName(c)
	default:
		t.token.appendToAttributeName(c)
	}
}

func (t *Tokenizer) afterAttributeNameState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '/':
		t.state = stateSelfClosingStartTag
	case c == '=':
		t.state = stateBeforeAttributeValue
	case c == '>':
		t.emitCurrentTag()
		t.state = stateData
	default:
		t.token.beginAttribute()
		t.reconsume(c)
		t.state = stateAttributeName
	}
}

func (t *Tokenizer) beforeAttributeValueState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '"':
		t.state = stateAttributeValueDoubleQuoted
	case c == '\'':
		t.state = stateAttributeValueSingleQuoted
	case c == '>':
		// parse error: missing attribute value
		t.emitCurrentTag()
		t.state = stateData
	default:
		t.reconsume(c)
		t.state = stateAttributeValueUnquoted
	}
}

func (t *Tokenizer) attributeValueDoubleQuotedState(c rune) {
	switch {
	case c == '"':
		t.state = stateAfterAttributeValueQuoted
	case c == '&':
		t.charRefResumeState = stateAttributeValueDoubleQuoted
		t.state = stateCharacterReferenceInAttributeValue
	case c == 0:
		t.token.appendToAttributeValue('\ufffd')
	default:
		t.token.appendToAttributeValue(c)
	}
}

func (t *Tokenizer) attributeValueSingleQuotedState(c rune) {
	switch {
	case c == '\'':
		t.state = stateAfterAttributeValueQuoted
	case c == '&':
		t.charRefResumeState = stateAttributeValueSingleQuoted
		t.state = stateCharacterReferenceInAttributeValue
	case c == 0:
		t.token.appendToAttributeValue('\ufffd')
	default:
		t.token.appendToAttributeValue(c)
	}
}

func (t *Tokenizer) attributeValueUnquotedState(c rune) {
	switch {
	case isWhitespace(c):
		t.state = stateBeforeAttributeName
	case c == '&':
		t.charRefResumeState = stateAttributeValueUnquoted
		t.state = stateCharacterReferenceInAttributeValue
	case c == '>':
		t.emitCurrentTag()
		t.state = stateData
	case c == 0:
		t.token.appendToAttributeValue('\ufffd')
	case c == '"', c == '\'', c == '<', c == '=', c == '`':
		t.token.appendToAttributeValue(c)
	default:
		t.token.appendToAttributeValue(c)
	}
}

func (t *Tokenizer) afterAttributeValueQuotedState(c rune) {
	switch {
	case isWhitespace(c):
		t.state = stateBeforeAttributeName
	case c == '/':
		t.state = stateSelfClosingStartTag
	case c == '>':
		t.emitCurrentTag()
		t.state = stateData
	default:
		t.reconsume(c)
		t.state = stateBeforeAttributeName
	}
}

func (t *Tokenizer) selfClosingStartTagState(c rune) {
	switch {
	case c == '>':
		t.token.SelfClosing = true
		t.emitCurrentTag()
		t.state = stateData
	case isWhitespace(c), c == '/':
		// parse error, treat as before-attribute-name / reconsume
		t.reconsume(c)
		t.state = stateBeforeAttributeName
	default:
		t.reconsume(c)
		t.state = stateBeforeAttributeName
	}
}

// --- Comment / markup declaration --------------------------------------

func (t *Tokenizer) bogusCommentState(c rune) {
	if c == '>' {
		t.emitCurrentToken()
		t.state = stateData
	} else if c == 0 {
		t.token.appendToComment('\ufffd')
	} else {
		t.token.appendToComment(c)
	}
}

func (t *Tokenizer) markupDeclarationOpenState(c rune) {
	// Two dashes => comment. 'DOCTYPE' (case-insensitive) => doctype. '[CDATA['
	// (when allowed) => CDATA section. Otherwise comment.
	if c == '-' {
		next, ok := t.peek()
		if ok && next == '-' {
			t.pos++
			t.flushText()
			t.token.clear()
			t.token.beginComment()
			t.state = stateCommentStart
			return
		}
		// Single dash: treat as comment.
		t.flushText()
		t.token.clear()
		t.token.beginComment()
		t.reconsume(c)
		t.state = stateComment
		return
	}
	if isASCIIUpper(c) || isASCIILower(c) {
		if t.matchDoctype(c) {
			t.flushText()
			t.token.clear()
			t.token.beginDOCTYPE()
			t.state = stateBeforeDOCTYPEName
			return
		}
		if t.shouldAllowCDATA && t.matchCDATA(c) {
			t.flushText()
			t.token.clear()
			t.state = stateCDATASection
			return
		}
	}
	// Anything else: comment.
	t.flushText()
	t.token.clear()
	t.token.beginComment()
	t.reconsume(c)
	t.state = stateComment
}

// matchDoctype reports whether the upcoming input spells DOCTYPE (case-
// insensitively), starting with the already-consumed c, and consumes it.
func (t *Tokenizer) matchDoctype(first rune) bool {
	want := "doctype"
	if toLowerASCIIRune(first) != 'd' {
		return false
	}
	for i := 1; i < len(want); i++ {
		c, ok := t.peek()
		if !ok || toLowerASCIIRune(c) != rune(want[i]) {
			return false
		}
		t.pos++
	}
	return true
}

// matchCDATA reports whether the upcoming input spells [CDATA[ starting with c.
func (t *Tokenizer) matchCDATA(first rune) bool {
	if first != '[' {
		return false
	}
	want := "CDATA["
	for i := 0; i < len(want); i++ {
		c, ok := t.peek()
		if !ok || c != rune(want[i]) {
			return false
		}
		t.pos++
	}
	return true
}

func (t *Tokenizer) commentStartState(c rune) {
	switch {
	case c == '-':
		t.state = stateCommentStartDash
	case c == '>':
		// parse error: abrupt closing
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.reconsume(c)
		t.state = stateComment
	}
}

func (t *Tokenizer) commentStartDashState(c rune) {
	switch {
	case c == '-':
		t.state = stateCommentEnd
	case c == '>':
		// parse error
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.reconsume(c)
		t.state = stateComment
	}
}

func (t *Tokenizer) commentState(c rune) {
	switch {
	case c == '-':
		t.state = stateCommentEndDash
	case c == 0:
		t.token.appendToComment('\ufffd')
	case c == '<':
		t.token.appendToComment('<')
	default:
		t.token.appendToComment(c)
	}
}

func (t *Tokenizer) commentEndDashState(c rune) {
	switch {
	case c == '-':
		t.state = stateCommentEnd
	default:
		t.reconsume(c)
		t.state = stateComment
	}
}

func (t *Tokenizer) commentEndState(c rune) {
	switch {
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	case c == '!':
		t.state = stateCommentEndBang
	case c == '-':
		t.token.appendToComment('-')
	default:
		t.reconsume(c)
		t.state = stateComment
	}
}

func (t *Tokenizer) commentEndBangState(c rune) {
	switch {
	case c == '-':
		t.token.appendToComment('-')
		t.state = stateCommentEnd
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.reconsume(c)
		t.state = stateComment
	}
}

// --- DOCTYPE states ----------------------------------------------------

func (t *Tokenizer) doctypeState(c rune) {
	if isWhitespace(c) {
		t.state = stateBeforeDOCTYPEName
	} else if c == '>' {
		t.reconsume(c)
		t.state = stateBeforeDOCTYPEName
	} else {
		// parse error: force quirks
		t.token.setForceQuirks()
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) beforeDOCTYPENameState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == 0:
		t.token.appendToName('\ufffd')
		t.state = stateDOCTYPEName
	case c == '>':
		t.token.setForceQuirks()
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.appendToName(toLowerASCIIRune(c))
		t.state = stateDOCTYPEName
	}
}

func (t *Tokenizer) doctypeNameState(c rune) {
	switch {
	case isWhitespace(c):
		t.state = stateAfterDOCTYPEName
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	case c == 0:
		t.token.appendToName('\ufffd')
	default:
		t.token.appendToName(toLowerASCIIRune(c))
	}
}

func (t *Tokenizer) afterDOCTYPENameState(c rune) {
	if isWhitespace(c) {
		t.state = stateBetweenDOCTYPEPublicAndSystemIdentifiers
		return
	}
	if c == '>' {
		t.emitCurrentToken()
		t.state = stateData
		return
	}
	// Peek for PUBLIC or SYSTEM keyword (case-insensitive).
	if t.matchKeyword("ublic", c, 'p') {
		t.state = stateAfterDOCTYPEPublicKeyword
		return
	}
	if t.matchKeyword("ystem", c, 's') {
		t.state = stateAfterDOCTYPESystemKeyword
		return
	}
	t.token.setForceQuirks()
	t.state = stateBogusDOCTYPE
}

// matchKeyword reports whether the upcoming input spells suffix (case-
// insensitively) given that the first letter's lower-case form is firstLower.
// It consumes the matched characters on success. suffix must be lower-case.
func (t *Tokenizer) matchKeyword(suffix string, first rune, firstLower byte) bool {
	if toLowerASCIIRune(first) != rune(firstLower) {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c, ok := t.peek()
		if !ok || toLowerASCIIRune(c) != rune(suffix[i]) {
			return false
		}
		t.pos++
	}
	return true
}

func (t *Tokenizer) afterDOCTYPEPublicKeywordState(c rune) {
	if isWhitespace(c) {
		t.state = stateBeforeDOCTYPEPublicIdentifier
	} else if c == '"' {
		t.token.DoctypeData.HasPublicID = true
		t.state = stateDOCTYPEPublicIdentifierDoubleQuoted
	} else if c == '\'' {
		t.token.DoctypeData.HasPublicID = true
		t.state = stateDOCTYPEPublicIdentifierSingleQuoted
	} else {
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) beforeDOCTYPEPublicIdentifierState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '"':
		t.token.DoctypeData.HasPublicID = true
		t.state = stateDOCTYPEPublicIdentifierDoubleQuoted
	case c == '\'':
		t.token.DoctypeData.HasPublicID = true
		t.state = stateDOCTYPEPublicIdentifierSingleQuoted
	case c == '>':
		t.token.setForceQuirks()
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) doctypePublicIdentifierDoubleQuotedState(c rune) {
	switch {
	case c == '"':
		t.state = stateAfterDOCTYPEPublicIdentifier
	case c == 0:
		t.token.DoctypeData.PublicIdentifier += "\ufffd"
	default:
		t.token.DoctypeData.PublicIdentifier += string(c)
	}
}

func (t *Tokenizer) doctypePublicIdentifierSingleQuotedState(c rune) {
	switch {
	case c == '\'':
		t.state = stateAfterDOCTYPEPublicIdentifier
	case c == 0:
		t.token.DoctypeData.PublicIdentifier += "\ufffd"
	default:
		t.token.DoctypeData.PublicIdentifier += string(c)
	}
}

func (t *Tokenizer) afterDOCTYPEPublicIdentifierState(c rune) {
	switch {
	case isWhitespace(c):
		t.state = stateBetweenDOCTYPEPublicAndSystemIdentifiers
	case c == '"':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierDoubleQuoted
	case c == '\'':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierSingleQuoted
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) betweenDOCTYPEPublicAndSystemIdentifiersState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '"':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierDoubleQuoted
	case c == '\'':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierSingleQuoted
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) afterDOCTYPESystemKeywordState(c rune) {
	if isWhitespace(c) {
		t.state = stateBeforeDOCTYPESystemIdentifier
	} else if c == '"' {
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierDoubleQuoted
	} else if c == '\'' {
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierSingleQuoted
	} else {
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) beforeDOCTYPESystemIdentifierState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '"':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierDoubleQuoted
	case c == '\'':
		t.token.DoctypeData.HasSystemID = true
		t.state = stateDOCTYPESystemIdentifierSingleQuoted
	case c == '>':
		t.token.setForceQuirks()
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) doctypeSystemIdentifierDoubleQuotedState(c rune) {
	switch {
	case c == '"':
		t.state = stateAfterDOCTYPESystemIdentifier
	case c == 0:
		t.token.DoctypeData.SystemIdentifier += "\ufffd"
	default:
		t.token.DoctypeData.SystemIdentifier += string(c)
	}
}

func (t *Tokenizer) doctypeSystemIdentifierSingleQuotedState(c rune) {
	switch {
	case c == '\'':
		t.state = stateAfterDOCTYPESystemIdentifier
	case c == 0:
		t.token.DoctypeData.SystemIdentifier += "\ufffd"
	default:
		t.token.DoctypeData.SystemIdentifier += string(c)
	}
}

func (t *Tokenizer) afterDOCTYPESystemIdentifierState(c rune) {
	switch {
	case isWhitespace(c):
		// ignore
	case c == '>':
		t.emitCurrentToken()
		t.state = stateData
	default:
		t.token.setForceQuirks()
		t.reconsume(c)
		t.state = stateBogusDOCTYPE
	}
}

func (t *Tokenizer) bogusDOCTYPEState(c rune) {
	if c == '>' {
		t.emitCurrentToken()
		t.state = stateData
	}
	// all other characters ignored
}

// --- CDATA section states ----------------------------------------------

func (t *Tokenizer) cdataSectionState(c rune) {
	switch {
	case c == ']':
		t.state = stateCDATASectionRightSquareBracket
	case c == 0:
		t.token.appendToCharacter('\ufffd')
	default:
		t.token.appendToCharacter(c)
	}
}

func (t *Tokenizer) cdataSectionRightSquareBracketState(c rune) {
	if c == ']' {
		t.state = stateCDATASectionDoubleRightSquareBracket
	} else {
		t.token.appendToCharacter(']')
		t.reconsume(c)
		t.state = stateCDATASection
	}
}

func (t *Tokenizer) cdataSectionDoubleRightSquareBracketState(c rune) {
	if c == '>' {
		t.emitCurrentToken()
		t.state = stateData
	} else {
		t.token.appendToCharacter(']')
		t.token.appendToCharacter(']')
		t.reconsume(c)
		t.state = stateCDATASection
	}
}
