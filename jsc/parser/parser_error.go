// ParserError.h — Parser error types and error object creation

package parser

// ParserError corresponds to JSC::ParserError
type ParserError struct {
	token           JSToken
	message         string
	line            int
	errType         ParserErrorType
	syntaxErrorType ParserSyntaxErrorType
}

// ParserSyntaxErrorType corresponds to JSC::ParserError::SyntaxErrorType
type ParserSyntaxErrorType uint8

const (
	ParserSyntaxErrorNone                ParserSyntaxErrorType = 0
	ParserSyntaxErrorIrrecoverable       ParserSyntaxErrorType = 1
	ParserSyntaxErrorUnterminatedLiteral ParserSyntaxErrorType = 2
	ParserSyntaxErrorRecoverable         ParserSyntaxErrorType = 3
)

// ParserErrorType corresponds to JSC::ParserError::ErrorType
type ParserErrorType uint8

const (
	ParserErrorNone         ParserErrorType = 0
	ParserErrorStackOverflow ParserErrorType = 1
	ParserErrorEvalError    ParserErrorType = 2
	ParserErrorOutOfMemory  ParserErrorType = 3
	ParserErrorSyntaxError  ParserErrorType = 4
)

func NewParserError() ParserError {
	return ParserError{errType: ParserErrorNone, syntaxErrorType: ParserSyntaxErrorNone}
}

func NewParserErrorType(errType ParserErrorType) ParserError {
	return ParserError{errType: errType, syntaxErrorType: ParserSyntaxErrorNone}
}

func NewParserErrorFull(errType ParserErrorType, syntaxErrorType ParserSyntaxErrorType, token JSToken) ParserError {
	return ParserError{token: token, errType: errType, syntaxErrorType: syntaxErrorType}
}

func NewParserErrorWithMessage(errType ParserErrorType, syntaxErrorType ParserSyntaxErrorType, token JSToken, msg string, line int) ParserError {
	return ParserError{token: token, message: msg, line: line, errType: errType, syntaxErrorType: syntaxErrorType}
}

func (e ParserError) IsValid() bool                             { return e.errType != ParserErrorNone }
func (e ParserError) SyntaxErrorType() ParserSyntaxErrorType    { return e.syntaxErrorType }
func (e ParserError) Token() JSToken                            { return e.token }
func (e ParserError) Message() string                           { return e.message }
func (e ParserError) Line() int                                 { return e.line }
func (e ParserError) Type() ParserErrorType                     { return e.errType }

func (e ParserError) ToErrorObject(globalObject *JSGlobalObject, source SourceCode, overrideLineNumber int) interface{} {
	switch e.errType {
	case ParserErrorNone:
		return nil
	case ParserErrorSyntaxError:
		return "SyntaxError: " + e.message
	case ParserErrorEvalError:
		return "EvalError: " + e.message
	case ParserErrorStackOverflow:
		return "RangeError: Maximum call stack size exceeded"
	case ParserErrorOutOfMemory:
		return "Error: Out of memory"
	}
	return nil
}
