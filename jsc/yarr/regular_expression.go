package yarr

import "regexp"

// RegularExpression wraps Go's regexp.Regexp for JSC compatibility.
type RegularExpression struct {
	pattern         string
	flags           Flags
	goRegex         *regexp.Regexp
	matched         bool
	matchStart      int
	matchEnd        int
}

// NewRegularExpression creates a new RegularExpression.
func NewRegularExpression(pattern string, flags Flags) (*RegularExpression, ErrorCode) {
	goPattern := convertJSRegexpToGo(pattern, flags)
	re, err := regexp.Compile(goPattern)
	if err != nil {
		return nil, ErrorCodePatternTooLarge
	}
	return &RegularExpression{
		pattern: pattern,
		flags:   flags,
		goRegex: re,
	}, ErrorCodeNoError
}

// Match checks if the pattern matches the given string.
func (r *RegularExpression) Match(str string) bool {
	r.matched = r.goRegex.MatchString(str)
	if r.matched {
		loc := r.goRegex.FindStringIndex(str)
		if loc != nil {
			r.matchStart = loc[0]
			r.matchEnd = loc[1]
		}
	}
	return r.matched
}

// Search performs a search and returns the match position.
func (r *RegularExpression) Search(str string) (int, int) {
	loc := r.goRegex.FindStringIndex(str)
	if loc != nil {
		r.matched = true
		r.matchStart = loc[0]
		r.matchEnd = loc[1]
		return loc[0], loc[1]
	}
	r.matched = false
	return -1, -1
}

// Matched returns whether the last match succeeded.
func (r *RegularExpression) Matched() bool {
	return r.matched
}

// MatchStart returns the start position of the last match.
func (r *RegularExpression) MatchStart() int {
	return r.matchStart
}

// MatchEnd returns the end position of the last match.
func (r *RegularExpression) MatchEnd() int {
	return r.matchEnd
}

// Pattern returns the original pattern string.
func (r *RegularExpression) Pattern() string {
	return r.pattern
}

// Flags returns the regex flags.
func (r *RegularExpression) Flags() Flags {
	return r.flags
}
