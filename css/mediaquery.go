// Translation of: Source/WebCore/css/MediaQuery.h
//                  Source/WebCore/css/MediaQuery.cpp
//                  Source/WebCore/css/MediaQueryEvaluator.h
//                  Source/WebCore/css/MediaQueryEvaluator.cpp
// Completeness: 50%
// Simplifications:
//   - no MediaQuerySet / MediaQueryParser intermediate layer; a single top-down
//     parser produces []MediaQuery directly
//   - CSS Level 4 range syntax (width >= 400px) is not supported; only the
//     Level 3 feature:value form
//   - resolution, scan, grid, update, overflow-block, overflow-inline, scripting
//     features are not evaluated
//   - prefers-reduced-motion, prefers-contrast, forced-colors are not evaluated
package css

import (
	"fmt"
	"strconv"
	"strings"
)

// MediaQueryContext provides the device and viewport information needed to
// evaluate media queries.
type MediaQueryContext struct {
	Width             int     // viewport width in CSS pixels
	Height            int     // viewport height in CSS pixels
	DeviceWidth       int     // device width in CSS pixels
	DeviceHeight      int     // device height in CSS pixels
	DevicePixelRatio  float64 // device-pixel-ratio (1.0 for standard, 2.0 for Retina)
	Orientation       string  // "portrait" or "landscape"
	PrefersColorScheme string // "light" or "dark"
	Hover             string  // "hover", "none", "on-demand"
	AnyHover          string  // "hover", "none", "on-demand"
	Pointer           string  // "fine", "coarse", "none"
	AnyPointer        string  // "fine", "coarse", "none"
}

// MediaQuery is the Go translation of WebCore::MediaQuery. It represents a single
// media query in a comma-separated list.
type MediaQuery struct {
	Qualifier string         // "only", "not", or "" (absent)
	MediaType string         // "all", "screen", "print", "speech", or "" (omitted)
	Features  []MediaFeature
}

// MediaFeature is a single feature inside a media query, e.g. (max-width: 768px)
// or (orientation: landscape).
type MediaFeature struct {
	Name  string // e.g. "max-width", "min-width", "orientation", "prefers-color-scheme"
	Value string // e.g. "768px", "landscape", "" (for boolean features)
}

// String renders the media query back to CSS text.
func (mq MediaQuery) String() string {
	var parts []string
	if mq.Qualifier != "" {
		parts = append(parts, mq.Qualifier)
	}
	if mq.MediaType != "" {
		parts = append(parts, mq.MediaType)
	}
	for _, f := range mq.Features {
		if f.Value != "" {
			parts = append(parts, "("+f.Name+": "+f.Value+")")
		} else {
			parts = append(parts, "("+f.Name+")")
		}
	}
	return strings.Join(parts, " and ")
}

// Matches evaluates the media query against the given device context. Returns true
// if the query matches (i.e. the @media block should apply).
func (mq MediaQuery) Matches(ctx MediaQueryContext) bool {
	// Evaluate media type.
	typeMatch := mq.evaluateMediaType(ctx)
	if !typeMatch {
		return false
	}

	// Evaluate features.
	for _, f := range mq.Features {
		if !evaluateFeature(f, ctx) {
			return false
		}
	}
	return true
}

// evaluateMediaType returns true if the media type matches in the given context,
// taking into account the "not" qualifier.
func (mq MediaQuery) evaluateMediaType(ctx MediaQueryContext) bool {
	mt := strings.ToLower(mq.MediaType)
	typeMatch := true

	switch mt {
	case "", "all":
		// Empty media type or "all" always matches the type part.
		typeMatch = true
	case "screen":
		typeMatch = true // screen is the primary output medium for web browsers
	case "print":
		typeMatch = false // print is not supported
	case "speech":
		typeMatch = false // speech is not supported
	default:
		// Unknown media types are treated as "not all".
		typeMatch = false
	}

	if mq.Qualifier == "not" {
		return !typeMatch
	}
	return typeMatch
}

// evaluateFeature evaluates a single media feature against the context.
func evaluateFeature(f MediaFeature, ctx MediaQueryContext) bool {
	name := strings.ToLower(f.Name)
	value := strings.TrimSpace(f.Value)

	switch {
	case name == "width":
		return comparePx(value, ctx.Width, false, false)
	case name == "min-width":
		return comparePx(value, ctx.Width, true, false)
	case name == "max-width":
		return comparePx(value, ctx.Width, false, true)
	case name == "height":
		return comparePx(value, ctx.Height, false, false)
	case name == "min-height":
		return comparePx(value, ctx.Height, true, false)
	case name == "max-height":
		return comparePx(value, ctx.Height, false, true)
	case name == "device-width":
		return comparePx(value, ctx.DeviceWidth, false, false)
	case name == "min-device-width":
		return comparePx(value, ctx.DeviceWidth, true, false)
	case name == "max-device-width":
		return comparePx(value, ctx.DeviceWidth, false, true)
	case name == "device-height":
		return comparePx(value, ctx.DeviceHeight, false, false)
	case name == "min-device-height":
		return comparePx(value, ctx.DeviceHeight, true, false)
	case name == "max-device-height":
		return comparePx(value, ctx.DeviceHeight, false, true)
	case name == "orientation":
		return value == "" || strings.EqualFold(value, ctx.Orientation)
	case name == "device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, false, false)
	case name == "min-device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, true, false)
	case name == "max-device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, false, true)
	case name == "-webkit-device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, false, false)
	case name == "min--webkit-device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, true, false)
	case name == "max--webkit-device-pixel-ratio":
		return compareFloat(value, ctx.DevicePixelRatio, false, true)
	case name == "prefers-color-scheme":
		return value == "" || strings.EqualFold(value, ctx.PrefersColorScheme)
	case name == "hover":
		return value == "" || strings.EqualFold(value, ctx.Hover)
	case name == "any-hover":
		return value == "" || strings.EqualFold(value, ctx.AnyHover)
	case name == "pointer":
		return value == "" || strings.EqualFold(value, ctx.Pointer)
	case name == "any-pointer":
		return value == "" || strings.EqualFold(value, ctx.AnyPointer)
	case name == "color":
		// Boolean feature: always true for typical displays (color depth > 0).
		return value == "" || true
	case name == "monochrome":
		// Boolean feature: false for typical color displays.
		return false
	case name == "color-index":
		return value == "" || true
	default:
		// Unknown features are treated as matching (optimistic).
		return true
	}
}

// comparePx compares a CSS pixel value string (e.g. "768px") against an actual
// pixel dimension. min/max control whether it's a >= or <= comparison.
func comparePx(value string, actual int, min, max bool) bool {
	if value == "" {
		return true
	}
	v := parseCSSPx(value)
	if min {
		return actual >= v
	}
	if max {
		return actual <= v
	}
	return actual == v
}

// compareFloat compares a numeric value string against a float.
func compareFloat(value string, actual float64, min, max bool) bool {
	if value == "" {
		return true
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}
	if min {
		return actual >= v
	}
	if max {
		return actual <= v
	}
	return actual == v
}

// parseCSSPx parses a CSS pixel length like "768px" to an int.
func parseCSSPx(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(v)
}

// ParseMediaQueryList parses a comma-separated list of media queries from a string.
// The input is expected to be the condition part of an @media rule, e.g.
//
//	"screen and (max-width: 768px)"
//	"print, screen and (min-width: 600px)"
//	"(prefers-color-scheme: dark)"
func ParseMediaQueryList(input string) ([]MediaQuery, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []MediaQuery{{MediaType: "all"}}, nil
	}
	var result []MediaQuery
	// Split by commas (outside parentheses).
	parts := splitMediaList(input)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mq, err := parseSingleMediaQuery(part)
		if err != nil {
			return nil, fmt.Errorf("css/mediaquery: %v in %q", err, part)
		}
		result = append(result, mq)
	}
	if len(result) == 0 {
		result = append(result, MediaQuery{MediaType: "all"})
	}
	return result, nil
}

// splitMediaList splits a media query list by commas, respecting parentheses.
func splitMediaList(input string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range input {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, input[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, input[start:])
	return parts
}

// parseSingleMediaQuery parses a single media query string.
func parseSingleMediaQuery(input string) (MediaQuery, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return MediaQuery{MediaType: "all"}, nil
	}

	// Tokenize by whitespace and parentheses.
	tokens := tokenizeMQ(input)
	if len(tokens) == 0 {
		return MediaQuery{MediaType: "all"}, nil
	}

	var mq MediaQuery
	pos := 0

	// Check optional qualifier.
	if pos < len(tokens) && (tokens[pos] == "only" || tokens[pos] == "not") {
		mq.Qualifier = tokens[pos]
		pos++
	}

	// Check for media type (all, screen, print, speech, or an identifier).
	if pos < len(tokens) {
		tok := tokens[pos]
		if isMediaType(tok) {
			mq.MediaType = tok
			pos++
		} else if tok != "and" && !strings.HasPrefix(tok, "(") {
			// Not a recognized media type and not a feature; treat as unknown.
			mq.MediaType = tok
			pos++
		}
	}

	// Parse "and" feature clauses.
	for pos < len(tokens) {
		tok := tokens[pos]
		if tok == "and" {
			pos++
			continue
		}
		if strings.HasPrefix(tok, "(") {
			f, err := parseMQFeature(tok)
			if err != nil {
				return mq, err
			}
			mq.Features = append(mq.Features, f)
			pos++
			continue
		}
		// Unexpected token.
		return mq, fmt.Errorf("unexpected token %q", tok)
	}

	return mq, nil
}

// isMediaType returns true if the token is a known CSS media type.
func isMediaType(s string) bool {
	switch strings.ToLower(s) {
	case "all", "screen", "print", "speech", "braille", "embossed",
		"handheld", "projection", "tty", "tv":
		return true
	}
	return false
}
// tokenizeMQ splits a media query string into tokens: identifiers, "and", "only",
func parseMQFeature(tok string) (MediaFeature, error) {
	inner := strings.TrimSpace(tok)
	inner = strings.TrimPrefix(inner, "(")
	inner = strings.TrimSuffix(inner, ")")
	inner = strings.TrimSpace(inner)

	if inner == "" {
		return MediaFeature{}, fmt.Errorf("empty feature in %q", tok)
	}

	// Check for "name: value" form.
	colonPos := strings.Index(inner, ":")
	if colonPos >= 0 {
		name := strings.TrimSpace(inner[:colonPos])
		value := strings.TrimSpace(inner[colonPos+1:])
		return MediaFeature{Name: name, Value: value}, nil
	}

	// Boolean feature form: "(feature-name)".
	return MediaFeature{Name: inner, Value: ""}, nil
}

// tokenizeMQ splits a media query string into tokens: identifiers, "and", "only",
// "not", and parenthesized groups.
func tokenizeMQ(input string) []string {
	var tokens []string
	i := 0
	for i < len(input) {
		// Skip whitespace.
		for i < len(input) && (input[i] == ' ' || input[i] == '\t' || input[i] == '\n') {
			i++
		}
		if i >= len(input) {
			break
		}

		// Parenthesized group.
		if input[i] == '(' {
			depth := 0
			start := i
			for i < len(input) {
				if input[i] == '(' {
					depth++
				} else if input[i] == ')' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
			tokens = append(tokens, input[start:i])
			continue
		}

		// Identifier or keyword.
		start := i
		for i < len(input) && input[i] != ' ' && input[i] != '\t' && input[i] != '\n' && input[i] != '(' && input[i] != ')' {
			i++
		}
		if i > start {
			tokens = append(tokens, strings.ToLower(input[start:i]))
		}
	}
	return tokens
}

// MatchesAny evaluates a list of media queries (comma-separated). Returns true if
// any single query in the list matches the context.
func MatchesAny(queries []MediaQuery, ctx MediaQueryContext) bool {
	if len(queries) == 0 {
		return true // empty list = always match
	}
	for _, q := range queries {
		if q.Matches(ctx) {
			return true
		}
	}
	return false
}
