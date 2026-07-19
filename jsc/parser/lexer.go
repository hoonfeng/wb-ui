/*
 *  Copyright (C) 1999-2000 Harri Porten (porten@kde.org)
 *  Copyright (C) 2002-2023 Apple Inc. All rights reserved.
 *  Copyright (C) 2010 Zoltan Herczeg (zherczeg@inf.u-szeged.hu)
 *
 *  This library is free software; you can redistribute it and/or
 *  modify it under the terms of the GNU Library General Public
 *  License as published by the Free Software Foundation; either
 *  version 2 of the License, or (at your option) any later version.
 *
 *  This library is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 *  Library General Public License for more details.
 *
 *  You should have received a copy of the GNU Library General Public License
 *  along with this library; see the file COPYING.LIB.  If not, write to
 *  the Free Software Foundation, Inc., 51 Franklin Street, Fifth Floor,
 *  Boston, MA 02110-1301, USA.
 */

// Lexer.h — JavaScript lexical analyzer (tokenizer)

package parser

import "wb-ui/jsc/runtime"

// LexerFlags corresponds to JSC::LexerFlags
type LexerFlags uint8

const (
	LexerFlagsIgnoreReservedWords LexerFlags = 1 << 0
	LexerFlagsDontBuildStrings    LexerFlags = 1 << 1
	LexerFlagsDontBuildKeywords   LexerFlags = 1 << 2
)

// IsLexerKeyword — checks if an identifier is a keyword
func IsLexerKeyword(ident *runtime.Identifier) bool {
	// Simplified — will check against keyword list
	return false
}

// Lexer corresponds to JSC::Lexer
// Character-level tokens: string -> JSToken stream.
type Lexer struct {
	vm             *runtime.VM
	builtinMode    JSParserBuiltinMode
	scriptMode     JSParserScriptMode
	code           string
	codePtr        int
	length         int
	lineNumber     int
	lineStart      int
	m_error        bool
	m_lexErrorMessage string
	m_isReparsingFunction bool
	m_hasLineTerminatorBeforeToken bool
	m_positionBeforeLastNewline JSTextPosition
	m_buffer8      []byte
	m_buffer16     []uint16
	arena          *ParserArena
}

func NewLexer(vm *runtime.VM, builtinMode JSParserBuiltinMode, scriptMode JSParserScriptMode) *Lexer {
	return &Lexer{
		vm:          vm,
		builtinMode: builtinMode,
		scriptMode:  scriptMode,
		lineNumber:  1,
		lineStart:   0,
		m_buffer8:   make([]byte, 0, 256),
		m_buffer16:  make([]uint16, 0, 256),
	}
}

// Static helper functions
func IsWhiteSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == 0x0B || ch == 0x0C || ch == 0xA0
}

func IsLineTerminator(ch byte) bool {
	return ch == '\n' || ch == '\r'
}

func ConvertHex(c1, c2 int) byte {
	return byte(HexVal(c1)<<4 | HexVal(c2))
}

func ConvertUnicode(c1, c2, c3, c4 int) uint16 {
	return uint16(HexVal(c1)<<12 | HexVal(c2)<<8 | HexVal(c3)<<4 | HexVal(c4))
}

func HexVal(c int) int {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func (lex *Lexer) SetCode(code SourceCode, arena *ParserArena) {
	lex.code = code.View()
	lex.codePtr = 0
	lex.length = len(lex.code)
	lex.arena = arena
	lex.lineNumber = code.FirstLine().OneBasedInt()
	lex.m_error = false
	lex.m_lexErrorMessage = ""
	lex.m_hasLineTerminatorBeforeToken = false
}

func (lex *Lexer) SetIsReparsingFunction()    { lex.m_isReparsingFunction = true }
func (lex *Lexer) IsReparsingFunction() bool  { return lex.m_isReparsingFunction }
func (lex *Lexer) LineNumber() int            { return lex.lineNumber }
func (lex *Lexer) CurrentOffset() int         { return lex.codePtr }
func (lex *Lexer) CurrentLineStartOffset() int { return lex.lineStart }

func (lex *Lexer) CurrentPosition() JSTextPosition {
	return NewJSTextPosition(lex.lineNumber, lex.codePtr, lex.lineStart)
}

func (lex *Lexer) PositionBeforeLastNewline() JSTextPosition {
	return lex.m_positionBeforeLastNewline
}

func (lex *Lexer) HasLineTerminatorBeforeToken() bool { return lex.m_hasLineTerminatorBeforeToken }
func (lex *Lexer) SawError() bool                      { return lex.m_error }
func (lex *Lexer) SetSawError(saw bool)                { lex.m_error = saw }
func (lex *Lexer) GetErrorMessage() string             { return lex.m_lexErrorMessage }
func (lex *Lexer) SetErrorMessage(msg string)           { lex.m_lexErrorMessage = msg }
func (lex *Lexer) SourceURLDirective() string           { return "" }
func (lex *Lexer) SourceMappingURLDirective() string    { return "" }

func (lex *Lexer) Clear() {
	lex.code = ""
	lex.codePtr = 0
	lex.length = 0
	lex.m_buffer8 = lex.m_buffer8[:0]
	lex.m_buffer16 = lex.m_buffer16[:0]
}

func (lex *Lexer) NextTokenIsColon() bool {
	i := lex.codePtr
	for i < lex.length {
		ch := lex.code[i]
		if ch == ':' {
			return true
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			i++
			continue
		}
		return false
	}
	return false
}

// Lex — main lex function, returns next token type
func (lex *Lexer) Lex(token *JSToken, flags OptionSet[LexerFlags], strictMode bool) JSTokenType {
	if lex.codePtr >= lex.length {
		token.Type = EOFTOK
		token.StartPosition = lex.CurrentPosition()
		token.EndPosition = lex.CurrentPosition()
		return EOFTOK
	}

	// Skip whitespace and line terminators
	lex.m_hasLineTerminatorBeforeToken = false
	for lex.codePtr < lex.length {
		ch := lex.code[lex.codePtr]
		if IsWhiteSpace(ch) {
			lex.codePtr++
			continue
		}
		if IsLineTerminator(ch) {
			lex.m_hasLineTerminatorBeforeToken = true
			lex.m_positionBeforeLastNewline = lex.CurrentPosition()
			lex.lineNumber++
			if ch == '\r' && lex.codePtr+1 < lex.length && lex.code[lex.codePtr+1] == '\n' {
				lex.codePtr++
			}
			lex.codePtr++
			lex.lineStart = lex.codePtr
			continue
		}
		break
	}

	if lex.codePtr >= lex.length {
		token.Type = EOFTOK
		return EOFTOK
	}

	token.StartPosition = lex.CurrentPosition()
	ch := lex.code[lex.codePtr]

	// Simple single-character tokens
	switch ch {
	case '{':
		lex.codePtr++
		token.Type = OPENBRACE
	case '}':
		lex.codePtr++
		token.Type = CLOSEBRACE
	case '(':
		lex.codePtr++
		token.Type = OPENPAREN
	case ')':
		lex.codePtr++
		token.Type = CLOSEPAREN
	case '[':
		lex.codePtr++
		token.Type = OPENBRACKET
	case ']':
		lex.codePtr++
		token.Type = CLOSEBRACKET
	case ',':
		lex.codePtr++
		token.Type = COMMA
	case ';':
		lex.codePtr++
		token.Type = SEMICOLON
	case ':':
		lex.codePtr++
		token.Type = COLON
	case '?':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '?' {
			lex.codePtr++
			token.Type = COALESCE
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '.' {
			lex.codePtr++
			token.Type = QUESTIONDOT
		} else {
			token.Type = QUESTION
		}
	case '~':
		lex.codePtr++
		token.Type = TILDE
	case '`':
		lex.codePtr++
		token.Type = BACKQUOTE
	case '.':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '.' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '.' {
				lex.codePtr++
				token.Type = DOTDOTDOT
			}
		} else {
			token.Type = DOT
		}
	default:
		// More complex tokens
		token.Type = lex.scanComplexToken(ch, strictMode)
	}

	token.EndPosition = lex.CurrentPosition()
	return token.Type
}

// scanComplexToken handles identifiers, numbers, strings, operators
func (lex *Lexer) scanComplexToken(ch byte, strictMode bool) JSTokenType {
	switch {
	case ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == '$':
		return lex.scanIdentifier()
	case ch >= '0' && ch <= '9':
		return lex.scanNumber()
	case ch == '"' || ch == '\'':
		return lex.scanString(ch)
	case ch == '/':
		return lex.scanSlash()
	case ch == '+':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '+' {
			lex.codePtr++
			return PLUSPLUS
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return PLUSEQUAL
		}
		return PLUS
	case ch == '-':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '-' {
			lex.codePtr++
			return MINUSMINUS
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return MINUSEQUAL
		}
		return MINUS
	case ch == '*':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return MULTEQUAL
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '*' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return POWEQUAL
			}
			return POW
		}
		return TIMES
	case ch == '%':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return MODEQUAL
		}
		return MOD
	case ch == '=':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return STREQ
			}
			return EQEQ
		}
		return EQUAL
	case ch == '!':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return STRNEQ
			}
			return NE
		}
		return EXCLAMATION
	case ch == '<':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return LE
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '<' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return LSHIFTEQUAL
			}
			return LSHIFT
		}
		return LT
	case ch == '>':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return GE
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '>' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '>' {
				lex.codePtr++
				if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
					lex.codePtr++
					return URSHIFTEQUAL
				}
				return URSHIFT
			}
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return RSHIFTEQUAL
			}
			return RSHIFT
		}
		return GT
	case ch == '&':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '&' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return ANDEQUAL
			}
			return AND
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return BITANDEQUAL
		}
		return BITAND
	case ch == '|':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '|' {
			lex.codePtr++
			if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
				lex.codePtr++
				return OREQUAL
			}
			return OR
		} else if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return BITOREQUAL
		}
		return BITOR
	case ch == '^':
		lex.codePtr++
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '=' {
			lex.codePtr++
			return BITXOREQUAL
		}
		return BITXOR
	default:
		lex.codePtr++
		return ERRORTOK
	}
}

// scanIdentifier reads an identifier token
func (lex *Lexer) scanIdentifier() JSTokenType {
	startIdx := lex.codePtr
	for lex.codePtr < lex.length {
		ch := lex.code[lex.codePtr]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '$' {
			lex.codePtr++
		} else {
			break
		}
	}
	ident := lex.code[startIdx:lex.codePtr]
	return lex.identToToken(ident)
}

// identToToken maps identifier string to token type
func (lex *Lexer) identToToken(ident string) JSTokenType {
	switch ident {
	case "null":
		return NULLTOKEN
	case "true":
		return TRUETOKEN
	case "false":
		return FALSETOKEN
	case "break":
		return BREAK
	case "case":
		return CASE
	case "default":
		return DEFAULT
	case "for":
		return FOR
	case "new":
		return NEW
	case "var":
		return VAR
	case "const":
		return CONSTTOKEN
	case "continue":
		return CONTINUE
	case "function":
		return FUNCTION
	case "return":
		return RETURN
	case "if":
		return IF
	case "this":
		return THISTOKEN
	case "do":
		return DO
	case "while":
		return WHILE
	case "switch":
		return SWITCH
	case "with":
		return WITH
	case "throw":
		return THROW
	case "try":
		return TRY
	case "catch":
		return CATCH
	case "finally":
		return FINALLY
	case "debugger":
		return DEBUGGER
	case "else":
		return ELSE
	case "import":
		return IMPORT
	case "export":
		return EXPORT_
	case "class":
		return CLASSTOKEN
	case "extends":
		return EXTENDS
	case "super":
		return SUPER
	case "typeof":
		return TYPEOF
	case "void":
		return VOIDTOKEN
	case "delete":
		return DELETETOKEN
	case "instanceof":
		return INSTANCEOF
	case "in":
		return INTOKEN
	case "let":
		return LET
	case "yield":
		return YIELD
	case "await":
		return AWAIT
	default:
		return IDENT
	}
}

// scanNumber reads a numeric literal
func (lex *Lexer) scanNumber() JSTokenType {
	// Simplified number scanning
	startNum := lex.codePtr
	_ = startNum
	isFloat := false

	if lex.codePtr < lex.length && lex.code[lex.codePtr] == '0' {
		lex.codePtr++
		if lex.codePtr < lex.length {
			switch lex.code[lex.codePtr] {
			case 'x', 'X':
				return lex.scanHexNumber()
			case 'o', 'O':
				return lex.scanOctalNumber()
			case 'b', 'B':
				return lex.scanBinaryNumber()
			case '.':
				lex.codePtr++
				isFloat = true
				for lex.codePtr < lex.length && lex.code[lex.codePtr] >= '0' && lex.code[lex.codePtr] <= '9' {
					lex.codePtr++
				}
			}
		}
	} else {
		for lex.codePtr < lex.length && lex.code[lex.codePtr] >= '0' && lex.code[lex.codePtr] <= '9' {
			lex.codePtr++
		}
		if lex.codePtr < lex.length && lex.code[lex.codePtr] == '.' {
			lex.codePtr++
			isFloat = true
			for lex.codePtr < lex.length && lex.code[lex.codePtr] >= '0' && lex.code[lex.codePtr] <= '9' {
				lex.codePtr++
			}
		}
	}

	// Exponent
	if lex.codePtr < lex.length && (lex.code[lex.codePtr] == 'e' || lex.code[lex.codePtr] == 'E') {
		isFloat = true
		lex.codePtr++
		if lex.codePtr < lex.length && (lex.code[lex.codePtr] == '+' || lex.code[lex.codePtr] == '-') {
			lex.codePtr++
		}
		for lex.codePtr < lex.length && lex.code[lex.codePtr] >= '0' && lex.code[lex.codePtr] <= '9' {
			lex.codePtr++
		}
	}

	if isFloat {
		return DOUBLE
	}
	return INTEGER
}

func (lex *Lexer) scanHexNumber() JSTokenType {
	lex.codePtr++ // skip x/X
	for lex.codePtr < lex.length {
		ch := lex.code[lex.codePtr]
		if ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F' {
			lex.codePtr++
		} else {
			break
		}
	}
	return INTEGER
}

func (lex *Lexer) scanOctalNumber() JSTokenType {
	lex.codePtr++ // skip o/O
	for lex.codePtr < lex.length && lex.code[lex.codePtr] >= '0' && lex.code[lex.codePtr] <= '7' {
		lex.codePtr++
	}
	return INTEGER
}

func (lex *Lexer) scanBinaryNumber() JSTokenType {
	lex.codePtr++ // skip b/B
	for lex.codePtr < lex.length && (lex.code[lex.codePtr] == '0' || lex.code[lex.codePtr] == '1') {
		lex.codePtr++
	}
	return INTEGER
}

// scanString reads a string literal
func (lex *Lexer) scanString(quote byte) JSTokenType {
	lex.codePtr++ // skip opening quote
	lex.m_buffer8 = lex.m_buffer8[:0]

	for lex.codePtr < lex.length {
		ch := lex.code[lex.codePtr]
		if ch == quote {
			lex.codePtr++ // skip closing quote
			return STRING
		}
		if ch == '\\' {
			lex.codePtr++
			if lex.codePtr >= lex.length {
				return UNTERMINATED_STRING_LITERAL_ERRORTOK
			}
			esc := lex.code[lex.codePtr]
			switch esc {
			case 'n':
				lex.m_buffer8 = append(lex.m_buffer8, '\n')
			case 't':
				lex.m_buffer8 = append(lex.m_buffer8, '\t')
			case 'r':
				lex.m_buffer8 = append(lex.m_buffer8, '\r')
			case 'b':
				lex.m_buffer8 = append(lex.m_buffer8, '\b')
			case 'f':
				lex.m_buffer8 = append(lex.m_buffer8, '\f')
			case '0':
				lex.m_buffer8 = append(lex.m_buffer8, 0)
			case 'x':
				if lex.codePtr+2 < lex.length {
					lex.codePtr++
					c1 := int(lex.code[lex.codePtr])
					lex.codePtr++
					c2 := int(lex.code[lex.codePtr])
					lex.m_buffer8 = append(lex.m_buffer8, ConvertHex(c1, c2))
				}
			default:
				lex.m_buffer8 = append(lex.m_buffer8, esc)
			}
			lex.codePtr++
		} else {
			if IsLineTerminator(ch) {
				return UNTERMINATED_STRING_LITERAL_ERRORTOK
			}
			lex.m_buffer8 = append(lex.m_buffer8, ch)
			lex.codePtr++
		}
	}

	return UNTERMINATED_STRING_LITERAL_ERRORTOK
}

// scanSlash handles / and // and /* and /regexp/
func (lex *Lexer) scanSlash() JSTokenType {
	lex.codePtr++
	if lex.codePtr >= lex.length {
		return DIVIDE
	}
	switch lex.code[lex.codePtr] {
	case '/':
		// Line comment
		lex.codePtr++
		for lex.codePtr < lex.length && !IsLineTerminator(lex.code[lex.codePtr]) {
			lex.codePtr++
		}
		// Continue scanning
		return lex.Lex(&JSToken{}, OptionSet[LexerFlags]{}, false)
	case '*':
		// Block comment
		lex.codePtr++
		for lex.codePtr+1 < lex.length {
			if lex.code[lex.codePtr] == '*' && lex.code[lex.codePtr+1] == '/' {
				lex.codePtr += 2
				return lex.Lex(&JSToken{}, OptionSet[LexerFlags]{}, false)
			}
			if IsLineTerminator(lex.code[lex.codePtr]) {
				lex.lineNumber++
				lex.lineStart = lex.codePtr + 1
			}
			lex.codePtr++
		}
		return ERRORTOK
	case '=':
		lex.codePtr++
		return DIVEQUAL
	default:
		// Could be regexp, but simplified to just divide
		return DIVIDE
	}
}

// ScanRegExp — scan a regexp literal
func (lex *Lexer) ScanRegExp(token *JSToken, patternPrefix byte) JSTokenType {
	return REGEXP
}

// ScanTemplateString — scan a template literal
func (lex *Lexer) ScanTemplateString(token *JSToken, rawStringsBuildMode RawStringsBuildMode) JSTokenType {
	return TEMPLATE
}

type RawStringsBuildMode int

const (
	RawStringsBuildModeBuildRawStrings   RawStringsBuildMode = 0
	RawStringsBuildModeDontBuildRawStrings RawStringsBuildMode = 1
)

// OptionSet — lightweight option set type
type OptionSet[T ~uint8 | ~uint16 | ~uint32] struct {
	value T
}

func NewOptionSet[T ~uint8 | ~uint16 | ~uint32](flags T) OptionSet[T] {
	return OptionSet[T]{value: flags}
}

func (o OptionSet[T]) Has(flag T) bool {
	return o.value&flag != 0
}
