// 版权所有 (C) 2010, 2013 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ParserTokens.h 翻译为 Go

package parser

import (
	"wb-ui/jsc/runtime"
)

// ===== JSTokenType 词法记号类型 =====

// Token 位域常量
const (
	UnaryOpTokenFlag                         = 1 << 8
	KeywordTokenFlag                         = 1 << 9
	BinaryOpTokenPrecedenceShift             = 10
	BinaryOpTokenAllowsInPrecedenceAdditionalShift = 4
	BinaryOpTokenPrecedenceMask              = 15 << BinaryOpTokenPrecedenceShift
	CanBeErrorTokenFlag                      = 1 << (BinaryOpTokenAllowsInPrecedenceAdditionalShift + BinaryOpTokenPrecedenceShift + 6)
	UnterminatedCanBeErrorTokenFlag          = CanBeErrorTokenFlag << 1
	RightAssociativeBinaryOpTokenFlag        = UnterminatedCanBeErrorTokenFlag << 1
)

// BINARY_OP_PRECEDENCE 宏
func binaryOpPrecedence(prec uint32) uint32 {
	return (prec << BinaryOpTokenPrecedenceShift) | (prec << (BinaryOpTokenPrecedenceShift + BinaryOpTokenAllowsInPrecedenceAdditionalShift))
}

// IN_OP_PRECEDENCE 宏
func inOpPrecedence(prec uint32) uint32 {
	return prec << (BinaryOpTokenPrecedenceShift + BinaryOpTokenAllowsInPrecedenceAdditionalShift)
}

// JSTokenType 表示词法记号类型
type JSTokenType uint32

const (
	// 关键字
	NULLTOKEN                         JSTokenType = KeywordTokenFlag
	TRUETOKEN                         JSTokenType = KeywordTokenFlag + 1
	FALSETOKEN                        JSTokenType = KeywordTokenFlag + 2
	BREAK                             JSTokenType = KeywordTokenFlag + 3
	CASE                              JSTokenType = KeywordTokenFlag + 4
	DEFAULT                           JSTokenType = KeywordTokenFlag + 5
	FOR                               JSTokenType = KeywordTokenFlag + 6
	NEW                               JSTokenType = KeywordTokenFlag + 7
	VAR                               JSTokenType = KeywordTokenFlag + 8
	CONSTTOKEN                        JSTokenType = KeywordTokenFlag + 9
	CONTINUE                          JSTokenType = KeywordTokenFlag + 10
	FUNCTION                          JSTokenType = KeywordTokenFlag + 11
	RETURN                            JSTokenType = KeywordTokenFlag + 12
	IF                                JSTokenType = KeywordTokenFlag + 13
	THISTOKEN                         JSTokenType = KeywordTokenFlag + 14
	DO                                JSTokenType = KeywordTokenFlag + 15
	WHILE                             JSTokenType = KeywordTokenFlag + 16
	SWITCH                            JSTokenType = KeywordTokenFlag + 17
	WITH                              JSTokenType = KeywordTokenFlag + 18
	RESERVED                          JSTokenType = KeywordTokenFlag + 19
	RESERVED_IF_STRICT                JSTokenType = KeywordTokenFlag + 20
	THROW                             JSTokenType = KeywordTokenFlag + 21
	TRY                               JSTokenType = KeywordTokenFlag + 22
	CATCH                             JSTokenType = KeywordTokenFlag + 23
	FINALLY                           JSTokenType = KeywordTokenFlag + 24
	DEBUGGER                          JSTokenType = KeywordTokenFlag + 25
	ELSE                              JSTokenType = KeywordTokenFlag + 26
	IMPORT                            JSTokenType = KeywordTokenFlag + 27
	EXPORT_                           JSTokenType = KeywordTokenFlag + 28
	CLASSTOKEN                        JSTokenType = KeywordTokenFlag + 29
	EXTENDS                           JSTokenType = KeywordTokenFlag + 30
	SUPER                             JSTokenType = KeywordTokenFlag + 31

	// 上下文关键字
	LET                               JSTokenType = KeywordTokenFlag + 32
	YIELD                             JSTokenType = KeywordTokenFlag + 33
	AWAIT                             JSTokenType = KeywordTokenFlag + 34
)

const (
	FirstContextualKeywordToken JSTokenType = LET
	LastContextualKeywordToken  JSTokenType = AWAIT
)

const (
	OPENBRACE    JSTokenType = 0
	CLOSEBRACE   JSTokenType = 1
	OPENPAREN    JSTokenType = 2
	CLOSEPAREN   JSTokenType = 3
	OPENBRACKET  JSTokenType = 4
	CLOSEBRACKET JSTokenType = 5
	COMMA        JSTokenType = 6
	QUESTION     JSTokenType = 7
	BACKQUOTE    JSTokenType = 8
	INTEGER      JSTokenType = 9
	DOUBLE       JSTokenType = 10
	BIGINT       JSTokenType = 11
	IDENT        JSTokenType = 12
	PRIVATENAME  JSTokenType = 13
	STRING       JSTokenType = 14
	TEMPLATE     JSTokenType = 15
	REGEXP       JSTokenType = 16
	SEMICOLON    JSTokenType = 17
	COLON        JSTokenType = 18
	DOT          JSTokenType = 19
	EOFTOK       JSTokenType = 20
	EQUAL        JSTokenType = 21
	PLUSEQUAL    JSTokenType = 22
	MINUSEQUAL   JSTokenType = 23
	MULTEQUAL    JSTokenType = 24
	DIVEQUAL     JSTokenType = 25
	LSHIFTEQUAL  JSTokenType = 26
	RSHIFTEQUAL  JSTokenType = 27
	URSHIFTEQUAL JSTokenType = 28
	MODEQUAL     JSTokenType = 29
	POWEQUAL     JSTokenType = 30
	BITANDEQUAL  JSTokenType = 31
	BITXOREQUAL  JSTokenType = 32
	BITOREQUAL   JSTokenType = 33
	COALESCEEQUAL JSTokenType = 34
	OREQUAL      JSTokenType = 35
	ANDEQUAL     JSTokenType = 36
	DOTDOTDOT    JSTokenType = 37
	ARROWFUNCTION JSTokenType = 38
	QUESTIONDOT  JSTokenType = 39
)

const LastUntaggedToken JSTokenType = QUESTIONDOT

// 一元运算符标记
const (
	PLUSPLUS       JSTokenType = 0 | UnaryOpTokenFlag
	MINUSMINUS     JSTokenType = 1 | UnaryOpTokenFlag
	AUTOPLUSPLUS   JSTokenType = 2 | UnaryOpTokenFlag
	AUTOMINUSMINUS JSTokenType = 3 | UnaryOpTokenFlag
	EXCLAMATION    JSTokenType = 4 | UnaryOpTokenFlag
	TILDE          JSTokenType = 5 | UnaryOpTokenFlag
	TYPEOF         JSTokenType = 6 | UnaryOpTokenFlag | KeywordTokenFlag
	VOIDTOKEN      JSTokenType = 7 | UnaryOpTokenFlag | KeywordTokenFlag
	DELETETOKEN    JSTokenType = 8 | UnaryOpTokenFlag | KeywordTokenFlag
)

// 二元运算符标记
var (
	COALESCE     = JSTokenType(binaryOpPrecedence(1))
	OR           = JSTokenType(binaryOpPrecedence(2))
	AND          = JSTokenType(binaryOpPrecedence(3))
	BITOR        = JSTokenType(binaryOpPrecedence(4))
	BITXOR       = JSTokenType(binaryOpPrecedence(5))
	BITAND       = JSTokenType(binaryOpPrecedence(6))
	EQEQ         = JSTokenType(binaryOpPrecedence(7))
	NE           = JSTokenType(1 | binaryOpPrecedence(7))
	STREQ        = JSTokenType(2 | binaryOpPrecedence(7))
	STRNEQ       = JSTokenType(3 | binaryOpPrecedence(7))
	LT           = JSTokenType(binaryOpPrecedence(8))
	GT           = JSTokenType(1 | binaryOpPrecedence(8))
	LE           = JSTokenType(2 | binaryOpPrecedence(8))
	GE           = JSTokenType(3 | binaryOpPrecedence(8))
	INSTANCEOF   = JSTokenType(4 | binaryOpPrecedence(8) | KeywordTokenFlag)
	INTOKEN      = JSTokenType(5 | inOpPrecedence(8) | KeywordTokenFlag)
	LSHIFT       = JSTokenType(binaryOpPrecedence(9))
	RSHIFT       = JSTokenType(1 | binaryOpPrecedence(9))
	URSHIFT      = JSTokenType(2 | binaryOpPrecedence(9))
	PLUS         = JSTokenType(binaryOpPrecedence(10) | UnaryOpTokenFlag)
	MINUS        = JSTokenType(1 | binaryOpPrecedence(10) | UnaryOpTokenFlag)
	TIMES        = JSTokenType(binaryOpPrecedence(11))
	DIVIDE       = JSTokenType(1 | binaryOpPrecedence(11))
	MOD          = JSTokenType(2 | binaryOpPrecedence(11))
	POW          = JSTokenType(binaryOpPrecedence(12) | RightAssociativeBinaryOpTokenFlag)
)

// 错误标记
const (
	ERRORTOK                                JSTokenType = CanBeErrorTokenFlag
	UNTERMINATED_IDENTIFIER_ESCAPE_ERRORTOK JSTokenType = CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_IDENTIFIER_ESCAPE_ERRORTOK      JSTokenType = 1 | CanBeErrorTokenFlag
	UNTERMINATED_IDENTIFIER_UNICODE_ESCAPE_ERRORTOK JSTokenType = 2 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_IDENTIFIER_UNICODE_ESCAPE_ERRORTOK     JSTokenType = 3 | CanBeErrorTokenFlag
	UNTERMINATED_MULTILINE_COMMENT_ERRORTOK         JSTokenType = 4 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_NUMERIC_LITERAL_ERRORTOK           JSTokenType = 5 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_OCTAL_NUMBER_ERRORTOK              JSTokenType = 6 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_NUMERIC_LITERAL_ERRORTOK                JSTokenType = 7 | CanBeErrorTokenFlag
	UNTERMINATED_STRING_LITERAL_ERRORTOK            JSTokenType = 8 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_STRING_LITERAL_ERRORTOK                 JSTokenType = 9 | CanBeErrorTokenFlag
	INVALID_PRIVATE_NAME_ERRORTOK                   JSTokenType = 10 | CanBeErrorTokenFlag
	UNTERMINATED_HEX_NUMBER_ERRORTOK                JSTokenType = 11 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_BINARY_NUMBER_ERRORTOK             JSTokenType = 12 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_TEMPLATE_LITERAL_ERRORTOK          JSTokenType = 13 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	UNTERMINATED_REGEXP_LITERAL_ERRORTOK            JSTokenType = 14 | CanBeErrorTokenFlag | UnterminatedCanBeErrorTokenFlag
	INVALID_TEMPLATE_LITERAL_ERRORTOK               JSTokenType = 15 | CanBeErrorTokenFlag
	ESCAPED_KEYWORD                                 JSTokenType = 16 | CanBeErrorTokenFlag
	INVALID_UNICODE_ENCODING_ERRORTOK               JSTokenType = 17 | CanBeErrorTokenFlag
	INVALID_IDENTIFIER_UNICODE_ERRORTOK             JSTokenType = 18 | CanBeErrorTokenFlag
)

// ===== JSTextPosition 扩展方法 =====

// Column 返回列号
func (p JSTextPosition) Column() int {
	return p.Offset - p.LineStartOffset
}

// CheckConsistency 检查位置的一致性
func (p JSTextPosition) CheckConsistency() {
	// 断言已禁用
}

// PositionAdd 返回偏移增加后的新位置
func (p JSTextPosition) PositionAdd(adjustment int) JSTextPosition {
	return NewJSTextPosition(p.Line, p.Offset+adjustment, p.LineStartOffset)
}

// ===== JSTokenData 词法记号数据 =====

// JSTokenData 表示词法记号的数据（C++ union 的 Go 等效）
type JSTokenData struct {
	// 模板字符串数据
	Cooked   *runtime.Identifier
	Raw      *runtime.Identifier
	IsTail   bool

	// 位置数据
	Line            uint32
	Offset          uint32
	LineStartOffset uint32

	// 数值
	DoubleValue float64

	// 标识符
	Ident   *runtime.Identifier
	Escaped bool

	// BigInt
	BigIntString *runtime.Identifier
	Radix        uint8

	// 正则表达式
	Pattern *runtime.Identifier
	Flags   *runtime.Identifier
}

// ===== JSTokenLocation 扩展 =====

// JSTokenLocation 已定义在 nodes.go 中

// ===== JSToken =====

// JSToken 表示一个词法记号
type JSToken struct {
	Type          JSTokenType
	Data          JSTokenData
	StartPosition JSTextPosition
	EndPosition   JSTextPosition
}

// Location 返回记号的位置
func (t *JSToken) Location() JSTokenLocation {
	return JSTokenLocation{
		Line:            t.StartPosition.Line,
		LineStartOffset: t.StartPosition.LineStartOffset,
		StartOffset:     t.StartPosition.Offset,
		EndOffset:       t.EndPosition.Offset,
	}
}

// ===== 辅助函数 =====

// IsUpdateOp 判断是否是自增/自减操作符
func IsUpdateOp(token JSTokenType) bool {
	return token >= PLUSPLUS && token <= AUTOMINUSMINUS
}

// IsUnaryOp 判断是否是一元操作符
func IsUnaryOp(token JSTokenType) bool {
	return token&UnaryOpTokenFlag != 0
}
