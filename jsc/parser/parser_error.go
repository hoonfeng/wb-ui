// 版权所有 (C) 2013 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ParserError.h 翻译为 Go

package parser

// SyntaxErrorType 表示语法错误类型
type SyntaxErrorType uint8

const (
	SyntaxErrorNone               SyntaxErrorType = 0
	SyntaxErrorIrrecoverable      SyntaxErrorType = 1
	SyntaxErrorUnterminatedLiteral SyntaxErrorType = 2
	SyntaxErrorRecoverable        SyntaxErrorType = 3
)

// ParserErrorType 表示解析器错误类型
type ParserErrorType uint8

const (
	ParserErrorNone        ParserErrorType = 0
	ParserErrorStackOverflow ParserErrorType = 1
	ParserErrorEvalError   ParserErrorType = 2
	ParserErrorOutOfMemory ParserErrorType = 3
	ParserErrorSyntaxError ParserErrorType = 4
)

// ParserError 表示解析错误
type ParserError struct {
	Token           JSToken
	Message         string
	Line            int
	Type            ParserErrorType
	SyntaxErrorType SyntaxErrorType
}

// NewParserError 创建空的 ParserError
func NewParserError() ParserError {
	return ParserError{Type: ParserErrorNone, SyntaxErrorType: SyntaxErrorNone, Line: -1}
}

// NewParserErrorWithType 创建指定类型的 ParserError
func NewParserErrorWithType(errType ParserErrorType) ParserError {
	return ParserError{Type: errType, SyntaxErrorType: SyntaxErrorNone, Line: -1}
}

// NewParserErrorWithToken 创建带 token 的 ParserError
func NewParserErrorWithToken(errType ParserErrorType, syntaxError SyntaxErrorType, token JSToken) ParserError {
	return ParserError{Token: token, Type: errType, SyntaxErrorType: syntaxError, Line: -1}
}

// NewParserErrorDetailed 创建详细的 ParserError
func NewParserErrorDetailed(errType ParserErrorType, syntaxError SyntaxErrorType, token JSToken, msg string, line int) ParserError {
	return ParserError{Token: token, Message: msg, Line: line, Type: errType, SyntaxErrorType: syntaxError}
}

// IsValid 检查错误是否有效
func (e *ParserError) IsValid() bool {
	return e.Type != ParserErrorNone
}

// SyntaxErrorTypeVal 返回语法错误类型
func (e *ParserError) SyntaxErrorTypeVal() SyntaxErrorType {
	return e.SyntaxErrorType
}

// TokenVal 返回关联的 token
func (e *ParserError) TokenVal() JSToken {
	return e.Token
}

// MessageVal 返回错误消息
func (e *ParserError) MessageVal() string {
	return e.Message
}

// LineVal 返回行号
func (e *ParserError) LineVal() int {
	return e.Line
}

// ErrorType 返回错误类型
func (e *ParserError) ErrorType() ParserErrorType {
	return e.Type
}

// SyntaxErrorTypeName 返回语法错误类型的名称
func SyntaxErrorTypeName(typ SyntaxErrorType) string {
	switch typ {
	case SyntaxErrorNone:
		return "SyntaxErrorNone"
	case SyntaxErrorIrrecoverable:
		return "SyntaxErrorIrrecoverable"
	case SyntaxErrorUnterminatedLiteral:
		return "SyntaxErrorUnterminatedLiteral"
	case SyntaxErrorRecoverable:
		return "SyntaxErrorRecoverable"
	default:
		return "Unknown"
	}
}

// ParserErrorTypeName 返回解析错误类型的名称
func ParserErrorTypeName(typ ParserErrorType) string {
	switch typ {
	case ParserErrorNone:
		return "ErrorNone"
	case ParserErrorStackOverflow:
		return "StackOverflow"
	case ParserErrorEvalError:
		return "EvalError"
	case ParserErrorOutOfMemory:
		return "OutOfMemory"
	case ParserErrorSyntaxError:
		return "SyntaxError"
	default:
		return "Unknown"
	}
}
