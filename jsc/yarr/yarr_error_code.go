package yarr

// ErrorCode represents regex parsing/compilation error codes.
type ErrorCode int32

const (
	ErrorCodeNoError                           ErrorCode = 0
	ErrorCodePatternTooLarge                   ErrorCode = 1
	ErrorCodeQuantifierOutOfOrder              ErrorCode = 2
	ErrorCodeQuantifierWithoutAtom             ErrorCode = 3
	ErrorCodeQuantifierTooLarge                ErrorCode = 4
	ErrorCodeMissingParentheses                ErrorCode = 5
	ErrorCodeParenthesesUnmatched              ErrorCode = 6
	ErrorCodeParenthesesTypeInvalid            ErrorCode = 7
	ErrorCodeCharacterClassUnmatched           ErrorCode = 8
	ErrorCodeCharacterClassOutOfOrder          ErrorCode = 9
	ErrorCodeCharacterClassRangeInvalid        ErrorCode = 10
	ErrorCodeCharacterClassRangeOutOfOrder     ErrorCode = 11
	ErrorCodeEscapeUnterminated                ErrorCode = 12
	ErrorCodeInvalidUnicodeEscape              ErrorCode = 13
	ErrorCodeInvalidBackreference              ErrorCode = 14
	ErrorCodeInvalidGroupHeader                ErrorCode = 15
	ErrorCodeInvalidGroupName                  ErrorCode = 16
	ErrorCodeInvalidGroupNameSyntax            ErrorCode = 17
	ErrorCodeInvalidFlags                      ErrorCode = 18
	ErrorCodeTooManyCapturingGroups            ErrorCode = 19
	ErrorCodeTooManyDisjunctions               ErrorCode = 20
	ErrorCodeTooManyAlternatives               ErrorCode = 21
	ErrorCodeTooManyLookaheadAssertions        ErrorCode = 22
	ErrorCodeTooManyLookbehindAssertions       ErrorCode = 23
	ErrorCodeTooManyCharacterClassNodes        ErrorCode = 24
	ErrorCodeTooManyCharacterClassRanges       ErrorCode = 25
	ErrorCodeUnicodePropertyUnescaped          ErrorCode = 26
	ErrorCodeInvalidQuantifierWithinLookbehind ErrorCode = 27
	ErrorCodeTooManyCaptureGroups              ErrorCode = 28
	ErrorCodeNotEnoughMemory                   ErrorCode = 29
	ErrorCodeCharacterClassEscapeInvalid       ErrorCode = 30
	ErrorCodeInvalidQuantifiableAssertion      ErrorCode = 31
)

// ErrorMessage returns the human-readable error message for an error code.
func (e ErrorCode) ErrorMessage() string {
	switch e {
	case ErrorCodeNoError:
		return "no error"
	case ErrorCodePatternTooLarge:
		return "regular expression too large"
	case ErrorCodeQuantifierOutOfOrder:
		return "numbers out of order in quantifier"
	case ErrorCodeQuantifierWithoutAtom:
		return "nothing to repeat"
	case ErrorCodeQuantifierTooLarge:
		return "quantifier too large"
	case ErrorCodeMissingParentheses:
		return "missing parenthesis"
	case ErrorCodeParenthesesUnmatched:
		return "unmatched parentheses"
	case ErrorCodeParenthesesTypeInvalid:
		return "invalid parentheses type"
	case ErrorCodeCharacterClassUnmatched:
		return "unmatched character class"
	case ErrorCodeCharacterClassOutOfOrder:
		return "range out of order in character class"
	case ErrorCodeCharacterClassRangeInvalid:
		return "invalid range in character class"
	case ErrorCodeCharacterClassRangeOutOfOrder:
		return "range out of order in character class"
	case ErrorCodeEscapeUnterminated:
		return "unterminated escape sequence"
	case ErrorCodeInvalidUnicodeEscape:
		return "invalid Unicode escape"
	case ErrorCodeInvalidBackreference:
		return "invalid backreference"
	case ErrorCodeInvalidGroupHeader:
		return "invalid group header"
	case ErrorCodeInvalidGroupName:
		return "invalid group name"
	case ErrorCodeInvalidGroupNameSyntax:
		return "invalid group name syntax"
	case ErrorCodeInvalidFlags:
		return "invalid flags"
	case ErrorCodeTooManyCapturingGroups:
		return "too many capturing groups"
	case ErrorCodeTooManyDisjunctions:
		return "too many disjunctions"
	case ErrorCodeTooManyAlternatives:
		return "too many alternatives"
	case ErrorCodeTooManyLookaheadAssertions:
		return "too many lookahead assertions"
	case ErrorCodeTooManyLookbehindAssertions:
		return "too many lookbehind assertions"
	case ErrorCodeTooManyCharacterClassNodes:
		return "too many character class nodes"
	case ErrorCodeTooManyCharacterClassRanges:
		return "too many character class ranges"
	case ErrorCodeUnicodePropertyUnescaped:
		return "invalid Unicode property escape"
	case ErrorCodeInvalidQuantifierWithinLookbehind:
		return "invalid quantifier in lookbehind"
	case ErrorCodeTooManyCaptureGroups:
		return "too many capture groups"
	case ErrorCodeNotEnoughMemory:
		return "out of memory"
	case ErrorCodeCharacterClassEscapeInvalid:
		return "invalid character class escape"
	case ErrorCodeInvalidQuantifiableAssertion:
		return "invalid quantifiable assertion"
	default:
		return "unknown error"
	}
}
