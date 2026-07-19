package yarr

import "regexp"

// YarrPattern represents a compiled regex pattern.
type YarrPattern struct {
	Pattern string
	Flags   Flags
	GoRegex *regexp.Regexp

	NumSubpatterns      int
	NumNamedSubpatterns int
	NamedGroupMap       map[string]int

	HasCopiedParenSubexpressions bool
	ContainSubpatternRequiringNonGlobalMatch bool
	AnyNonCapturingGroups bool
	ContainsSimpleAtom bool
}

// NewYarrPattern creates a new YarrPattern by compiling the given regex string.
func NewYarrPattern(pattern string, flags Flags) (*YarrPattern, ErrorCode) {
	goPattern := convertJSRegexpToGo(pattern, flags)
	re, err := regexp.Compile(goPattern)
	if err != nil {
		return nil, ErrorCodePatternTooLarge
	}

	yarr := &YarrPattern{
		Pattern:                      pattern,
		Flags:                        flags,
		GoRegex:                      re,
		NumSubpatterns:               re.NumSubexp(),
		NamedGroupMap:                make(map[string]int),
		ContainSubpatternRequiringNonGlobalMatch: true,
	}

	// Extract named groups
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			yarr.NamedGroupMap[name] = i
			yarr.NumNamedSubpatterns++
		}
	}

	return yarr, ErrorCodeNoError
}

// convertJSRegexpToGo converts a JS regex pattern to Go-compatible syntax.
func convertJSRegexpToGo(pattern string, flags Flags) string {
	// Apply flags
	var prefix string
	if flags&FlagIgnoreCase != 0 {
		prefix += "(?i)"
	}
	if flags&FlagMultiline != 0 {
		prefix += "(?m)"
	}
	if flags&FlagDotAll != 0 {
		prefix += "(?s)"
	}
	return prefix + pattern
}
