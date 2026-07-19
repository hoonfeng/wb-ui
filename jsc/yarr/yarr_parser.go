package yarr

// YarrParser parses a JS regex pattern string into a YarrPattern.
type YarrParser struct {
	pattern string
	pos     int
	flags   Flags
}

// NewYarrParser creates a new parser for the given pattern and flags.
func NewYarrParser(pattern string, flags Flags) *YarrParser {
	return &YarrParser{pattern: pattern, flags: flags}
}

// Parse parses the pattern and returns a YarrPattern.
func (p *YarrParser) Parse() (*YarrPattern, ErrorCode) {
	return NewYarrPattern(p.pattern, p.flags)
}
