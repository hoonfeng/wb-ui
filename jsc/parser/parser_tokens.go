/*
 * Copyright (C) 2010, 2013 Apple Inc. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. AND ITS CONTRIBUTORS ``AS IS''
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
 * THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL APPLE INC. OR ITS CONTRIBUTORS
 * BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
 * THE POSSIBILITY OF SUCH DAMAGE.
 */

// ParserTokens.h — Token types, token data structures, and related constants

package parser

// JSTokenType corresponds to JSC::JSTokenType
type JSTokenType uint32

// Token flags
const (
	UnaryOpTokenFlag                      JSTokenType = 1 << 8
	KeywordTokenFlag                      JSTokenType = 1 << 9
	BinaryOpTokenPrecedenceShift          JSTokenType = 10
	BinaryOpTokenAllowsInPrecedenceAdditionalShift JSTokenType = 4
	BinaryOpTokenPrecedenceMask           JSTokenType = 15 << BinaryOpTokenPrecedenceShift
	CanBeErrorTokenFlag                   JSTokenType = 1 << (BinaryOpTokenAllowsInPrecedenceAdditionalShift + BinaryOpTokenPrecedenceShift + 6)
	UnterminatedCanBeErrorTokenFlag       JSTokenType = CanBeErrorTokenFlag << 1
	RightAssociativeBinaryOpTokenFlag     JSTokenType = UnterminatedCanBeErrorTokenFlag << 1
)

// Helper macro for binary op precedence
func binaryOpPrecedence(prec JSTokenType) JSTokenType {
	return (prec << BinaryOpTokenPrecedenceShift) | (prec << (BinaryOpTokenPrecedenceShift + BinaryOpTokenAllowsInPrecedenceAdditionalShift))
}

func inOpPrecedence(prec JSTokenType) JSTokenType {
	return prec << (BinaryOpTokenPrecedenceShift + BinaryOpTokenAllowsInPrecedenceAdditionalShift)
}

const (
	NULLTOKEN   JSTokenType = KeywordTokenFlag
	TRUETOKEN   JSTokenType = iota + NULLTOKEN + 1
	FALSETOKEN
	BREAK
	CASE
	DEFAULT
	FOR
	NEW
	VAR
	CONSTTOKEN
	CONTINUE
	FUNCTION
	RETURN
	IF
	THISTOKEN
	DO
	WHILE
	SWITCH
	WITH
	RESERVED
	RESERVED_IF_STRICT
	THROW
	TRY
	CATCH
	FINALLY
	DEBUGGER
	ELSE
	IMPORT
	EXPORT_
	CLASSTOKEN
	EXTENDS
	SUPER

	// Contextual keywords
	LET  JSTokenType = iota
	YIELD
	AWAIT

	FirstContextualKeywordToken JSTokenType = LET
	LastContextualKeywordToken  JSTokenType = AWAIT
)

const (
	OPENBRACE   JSTokenType = 0
	CLOSEBRACE  JSTokenType = iota
	OPENPAREN
	CLOSEPAREN
	OPENBRACKET
	CLOSEBRACKET
	COMMA
	QUESTION
	BACKQUOTE
	INTEGER
	DOUBLE
	BIGINT
	IDENT
	PRIVATENAME
	STRING
	TEMPLATE
	REGEXP
	SEMICOLON
	COLON
	DOT
	EOFTOK
	EQUAL
	PLUSEQUAL
	MINUSEQUAL
	MULTEQUAL
	DIVEQUAL
	LSHIFTEQUAL
	RSHIFTEQUAL
	URSHIFTEQUAL
	MODEQUAL
	POWEQUAL
	BITANDEQUAL
	BITXOREQUAL
	BITOREQUAL
	COALESCEEQUAL
	OREQUAL
	ANDEQUAL
	DOTDOTDOT
	ARROWFUNCTION
	QUESTIONDOT
	LastUntaggedToken
)

// Tagged tokens (unary operators)
const (
	PLUSPLUS     JSTokenType = 0 | UnaryOpTokenFlag
	MINUSMINUS   JSTokenType = 1 | UnaryOpTokenFlag
	AUTOPLUSPLUS JSTokenType = 2 | UnaryOpTokenFlag
	AUTOMINUSMINUS JSTokenType = 3 | UnaryOpTokenFlag
	EXCLAMATION  JSTokenType = 4 | UnaryOpTokenFlag
	TILDE        JSTokenType = 5 | UnaryOpTokenFlag
	TYPEOF       JSTokenType = 6 | UnaryOpTokenFlag | KeywordTokenFlag
	VOIDTOKEN    JSTokenType = 7 | UnaryOpTokenFlag | KeywordTokenFlag
	DELETETOKEN  JSTokenType = 8 | UnaryOpTokenFlag | KeywordTokenFlag
)

// Binary operator tokens — use helper functions for precedence
var (
	COALESCE JSTokenType = 0 | binaryOpPrecedence(1)
	OR       JSTokenType = 0 | binaryOpPrecedence(2)
	AND      JSTokenType = 0 | binaryOpPrecedence(3)
	BITOR    JSTokenType = 0 | binaryOpPrecedence(4)
	BITXOR   JSTokenType = 0 | binaryOpPrecedence(5)
	BITAND   JSTokenType = 0 | binaryOpPrecedence(6)
	EQEQ     JSTokenType = 0 | binaryOpPrecedence(7)
	NE       JSTokenType = 1 | binaryOpPrecedence(7)
	STREQ    JSTokenType = 2 | binaryOpPrecedence(7)
	STRNEQ   JSTokenType = 3 | binaryOpPrecedence(7)
	LT       JSTokenType = 0 | binaryOpPrecedence(8)
	GT       JSTokenType = 1 | binaryOpPrecedence(8)
	LE       JSTokenType = 2 | binaryOpPrecedence(8)
	GE       JSTokenType = 3 | binaryOpPrecedence(8)
	INSTANCEOF JSTokenType = 4 | binaryOpPrecedence(8) | KeywordTokenFlag
	INTOKEN  JSTokenType = 5 | inOpPrecedence(8) | KeywordTokenFlag
	LSHIFT   JSTokenType = 0 | binaryOpPrecedence(9)
	RSHIFT   JSTokenType = 1 | binaryOpPrecedence(9)
	URSHIFT  JSTokenType = 2 | binaryOpPrecedence(9)
	PLUS     JSTokenType = 0 | binaryOpPrecedence(10) | UnaryOpTokenFlag
	MINUS    JSTokenType = 1 | binaryOpPrecedence(10) | UnaryOpTokenFlag
	TIMES    JSTokenType = 0 | binaryOpPrecedence(11)
	DIVIDE   JSTokenType = 1 | binaryOpPrecedence(11)
	MOD      JSTokenType = 2 | binaryOpPrecedence(11)
	POW      JSTokenType = 0 | binaryOpPrecedence(12) | RightAssociativeBinaryOpTokenFlag
)

// Error tokens
const (
	ERRORTOK                               JSTokenType = 0 | CanBeErrorTokenFlag
	UNTERMINATED_IDENTIFIER_ESCAPE_ERRORTOK JSTokenType = 0 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_IDENTIFIER_ESCAPE_ERRORTOK     JSTokenType = 1 | CanBeErrorTokenFlag
	UNTERMINATED_IDENTIFIER_UNICODE_ESCAPE_ERRORTOK JSTokenType = 2 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_IDENTIFIER_UNICODE_ESCAPE_ERRORTOK JSTokenType = 3 | CanBeErrorTokenFlag
	UNTERMINATED_MULTILINE_COMMENT_ERRORTOK JSTokenType = 4 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_NUMERIC_LITERAL_ERRORTOK  JSTokenType = 5 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_OCTAL_NUMBER_ERRORTOK    JSTokenType = 6 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_NUMERIC_LITERAL_ERRORTOK      JSTokenType = 7 | CanBeErrorTokenFlag
	UNTERMINATED_STRING_LITERAL_ERRORTOK  JSTokenType = 8 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_STRING_LITERAL_ERRORTOK       JSTokenType = 9 | CanBeErrorTokenFlag
	INVALID_PRIVATE_NAME_ERRORTOK         JSTokenType = 10 | CanBeErrorTokenFlag
	UNTERMINATED_HEX_NUMBER_ERRORTOK      JSTokenType = 11 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_BINARY_NUMBER_ERRORTOK   JSTokenType = 12 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_TEMPLATE_LITERAL_ERRORTOK JSTokenType = 13 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_REGEXP_LITERAL_ERRORTOK  JSTokenType = 14 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_TEMPLATE_LITERAL_ERRORTOK     JSTokenType = 15 | CanBeErrorTokenFlag
	ESCAPED_KEYWORD                        JSTokenType = 16 | CanBeErrorTokenFlag
	INVALID_UNICODE_ENCODING_ERRORTOK     JSTokenType = 17 | CanBeErrorTokenFlag
	INVALID_IDENTIFIER_UNICODE_ERRORTOK   JSTokenType = 18 | CanBeErrorTokenFlag
)

// JSTextPosition corresponds to JSC::JSTextPosition
type JSTextPosition struct {
	Line           int
	Offset         int
	LineStartOffset int
}

func NewJSTextPosition(line, offset, lineStartOffset int) JSTextPosition {
	return JSTextPosition{Line: line, Offset: offset, LineStartOffset: lineStartOffset}
}

func (p JSTextPosition) Column() int { return p.Offset - p.LineStartOffset }

func (p JSTextPosition) Add(adjustment int) JSTextPosition {
	return JSTextPosition{Line: p.Line, Offset: p.Offset + adjustment, LineStartOffset: p.LineStartOffset}
}

// JSTokenData corresponds to JSC::JSTokenData (union)
type JSTokenData struct {
	// Template literal
	Cooked  *string
	Raw     *string
	IsTail  bool

	// Numeric
	DoubleValue float64

	// Identifier
	Ident   *string
	Escaped bool

	// BigInt
	BigIntString *string
	Radix        uint8

	// RegExp
	Pattern *string
	Flags   *string

	// Location (for error tokens)
	Line            uint32
	Offset          uint32
	LineStartOffset uint32
}

// JSTokenLocation corresponds to JSC::JSTokenLocation
type JSTokenLocation struct {
	Line            int
	LineStartOffset uint32
	StartOffset     uint32
	EndOffset       uint32
}

// JSToken corresponds to JSC::JSToken
type JSToken struct {
	Type          JSTokenType
	Data          JSTokenData
	StartPosition JSTextPosition
	EndPosition   JSTextPosition
}

func NewJSToken() JSToken {
	return JSToken{Type: ERRORTOK}
}

func (t JSToken) Location() JSTokenLocation {
	return JSTokenLocation{
		Line:            t.StartPosition.Line,
		LineStartOffset: uint32(t.StartPosition.LineStartOffset),
		StartOffset:     uint32(t.StartPosition.Offset),
		EndOffset:       uint32(t.EndPosition.Offset),
	}
}

// IsUpdateOp — checks if token is update operator (++ or --)
func IsUpdateOp(token JSTokenType) bool {
	return token >= PLUSPLUS && token <= AUTOMINUSMINUS
}

// IsUnaryOp — checks if token has unary operator flag
func IsUnaryOp(token JSTokenType) bool {
	return token&UnaryOpTokenFlag != 0
}
