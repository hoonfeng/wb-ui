// Translation of: Source/WebCore/css/StyleRule.h
//                  Source/WebCore/css/StyleRule.cpp
//                  Source/WebCore/css/CSSRule.h
//                  Source/WebCore/css/StyleRuleType.h
// Completeness: 75%
// Simplifications:
//   - the C++ hierarchy (StyleRuleBase -> StyleRule / StyleRuleMedia / StyleRuleImport
//     / StyleRuleFontFace / StyleRuleKeyframes / StyleRuleNamespace / StyleRulePage)
//     is flattened to a Go interface with concrete structs per rule kind
//   - the @page rule, @namespace rule and @import rule model only the fields needed
//     for parsing; CSSOM mutation is omitted
//   - StyleProperties is represented as a slice of Declaration rather than the
//     WebKit StyleProperties heap-allocated form
//   - parent stylesheet tracking is via a back-pointer set by the parser; the CSSRule
//     parent chain is not modeled

package css

import "strconv"

// RuleType mirrors StyleRuleType in StyleRuleType.h.
type RuleType int

const (
	RuleUnknown RuleType = iota
	RuleStyle
	RuleImport
	RuleMedia
	RuleFontFace
	RulePage
	RuleKeyframes
	RuleKeyframe
	RuleNamespace
	RuleCharset
	RuleSupports
	RuleLayerBlock
	RuleLayerStatement
	RuleContainer
	RuleScope
	RuleStartingStyle
	RuleCounterStyle
)

// Rule is the Go translation of StyleRuleBase. Every concrete rule type satisfies this
// interface so the parser can stash heterogeneous rules in a single slice and the
// resolver can dispatch on type.
type Rule interface {
	Type() RuleType
}

// Declaration is a CSS property declaration "name: value", mirroring the
// CSSPropertySourceData concept. The Value is stored as a token slice so that the
// resolver can interpret it lazily (e.g. resolve var() or evaluate calc()).
type Declaration struct {
	Name       string
	Value      []Token
	Important  bool
}

// String renders the declaration as "name: value; !important" if set.
func (d Declaration) String() string {
	var sb []byte
	sb = append(sb, d.Name...)
	sb = append(sb, ':', ' ')
	for i, tok := range d.Value {
		if i > 0 {
			sb = append(sb, ' ')
		}
		sb = append(sb, serializeToken(tok)...)
	}
	if d.Important {
		sb = append(sb, " !important"...)
	}
	return string(sb)
}

// ValueString renders the declaration value as a single space-separated string,
// suitable for use by the resolver when looking up the raw textual form.
func (d Declaration) ValueString() string {
	var sb []byte
	for i, tok := range d.Value {
		if i > 0 {
			sb = append(sb, ' ')
		}
		sb = append(sb, serializeToken(tok)...)
	}
	return string(sb)
}

// serializeToken renders a single token back to CSS text. This is a minimal helper
// sufficient for diagnostics and the resolver's value-string lookup.
func serializeToken(t Token) string {
	switch t.Type {
	case TokenIdent, TokenAtKeyword:
		return t.Value
	case TokenString:
		return "\"" + t.Value + "\""
	case TokenFunction:
		return t.Value + "("
	case TokenHash:
		return "#" + t.Value
	case TokenNumber:
		return floatToString(t.Numeric)
	case TokenPercentage:
		return floatToString(t.Numeric) + "%"
	case TokenDimension:
		return floatToString(t.Numeric) + t.Unit
	case TokenDelimiter:
		return string(t.Delimiter)
	case TokenComma:
		return ","
	case TokenColon:
		return ":"
	case TokenSemicolon:
		return ";"
	case TokenLeftBrace:
		return "{"
	case TokenRightBrace:
		return "}"
	case TokenLeftParenthesis:
		return "("
	case TokenRightParenthesis:
		return ")"
	case TokenLeftBracket:
		return "["
	case TokenRightBracket:
		return "]"
	}
	return ""
}

// floatToString renders a numeric value as CSS text (without trailing zeros).
func floatToString(v float64) string {
	// Try integer first.
	if v == float64(int64(v)) {
		return intToString(int(v))
	}
	return formatFloat(v)
}

// formatFloat renders a floating-point value using strconv to avoid reimplementing
// float formatting. Mirrors WTF::NumberToString.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// StyleRule is the Go translation of StyleRule: a selector list plus a list of
// declarations. This is the most common rule kind ("selector { prop: value; }").
type StyleRule struct {
	Selectors    *SelectorList
	Declarations []Declaration
	// NestedRules holds rules nested inside this style rule (CSS Nesting), in order.
	NestedRules []Rule
	Origin      Origin
}

// Type implements Rule.
func (*StyleRule) Type() RuleType { return RuleStyle }

// MediaRule is the Go translation of StyleRuleMedia: a media query plus a list of
// nested rules.
type MediaRule struct {
	Condition string
	Parsed    []MediaQuery // parsed media query list, set by the parser
	Rules     []Rule
	Origin    Origin
}

// Type implements Rule.
func (*MediaRule) Type() RuleType { return RuleMedia }

// ImportRule is the Go translation of StyleRuleImport: an href plus a media query.
type ImportRule struct {
	Href      string
	Media     string
	Supports  string
	Origin    Origin
}

// Type implements Rule.
func (*ImportRule) Type() RuleType { return RuleImport }

// FontFaceRule is the Go translation of StyleRuleFontFace.
type FontFaceRule struct {
	Declarations []Declaration
	Origin       Origin
}

// Type implements Rule.
func (*FontFaceRule) Type() RuleType { return RuleFontFace }

// KeyframesRule is the Go translation of StyleRuleKeyframes. Each Keyframe holds a
// list of selector keys (e.g. "0%", "50%", "from", "to") plus declarations.
type KeyframesRule struct {
	Name     string
	Keyframes []KeyframeRule
	Origin   Origin
}

// Type implements Rule.
func (*KeyframesRule) Type() RuleType { return RuleKeyframes }

// KeyframeRule is one keyframe inside a @keyframes rule.
type KeyframeRule struct {
	Keys         []string
	Declarations []Declaration
}

// PageRule is the Go translation of StyleRulePage.
type PageRule struct {
	Selector     string
	Declarations []Declaration
	Origin       Origin
}

// Type implements Rule.
func (*PageRule) Type() RuleType { return RulePage }

// NamespaceRule is the Go translation of StyleRuleNamespace.
type NamespaceRule struct {
	Prefix string
	URI    string
}

// Type implements Rule.
func (*NamespaceRule) Type() RuleType { return RuleNamespace }

// SupportsRule is the Go translation of StyleRuleSupports.
type SupportsRule struct {
	Condition string
	Rules     []Rule
	Origin    Origin
}

// Type implements Rule.
func (*SupportsRule) Type() RuleType { return RuleSupports }

// Origin mirrors CSSParserMode / CascadeOrigin: where a rule came from. The cascade
// orders rules first by origin and importance, then by specificity, then by order.
type Origin int

const (
	OriginUserAgent Origin = iota
	OriginUser
	OriginAuthor
)

// String returns the origin name for diagnostics.
func (o Origin) String() string {
	switch o {
	case OriginUserAgent:
		return "user-agent"
	case OriginUser:
		return "user"
	case OriginAuthor:
		return "author"
	}
	return "unknown"
}
