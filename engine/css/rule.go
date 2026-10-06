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

import (
	"strconv"
	"strings"
)

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
	Name      string
	Value     []Token
	Important bool
}

// String renders the declaration as "name: value; !important" if set.
//
// ★ 第 22 轮分清两个口径：String / ValueString 是**引擎内部**文本（解析器、
// 级联、渲染层读值用），url token **不带引号** —— 渲染层据此解析并加载资源。
// 浏览器口径（cssText / getPropertyValue 的文本）请用 CSSText / CSSTextValue。
func (d Declaration) String() string {
	var sb []byte
	sb = append(sb, d.Name...)
	sb = append(sb, ':', ' ')
	sb = append(sb, valueStringOf(d.Value)...)
	if d.Important {
		sb = append(sb, " !important"...)
	}
	return string(sb)
}

// ValueString renders the declaration value as a single space-separated string,
// suitable for use by the resolver when looking up the raw textual form.
func (d Declaration) ValueString() string {
	return valueStringOf(d.Value)
}

// CSSText 渲染 CSSOM 口径的声明文本（`name: value`，带 ` !important`）。
// 与 String 的差别只在**属性特化**与该值里的 url token 引号（见 CSSTextValue）。
func (d Declaration) CSSText() string {
	var sb []byte
	sb = append(sb, d.Name...)
	sb = append(sb, ':', ' ')
	sb = append(sb, d.CSSTextValue()...)
	if d.Important {
		sb = append(sb, " !important"...)
	}
	return string(sb)
}

// CSSTextValue 返回 CSSOM 口径的声明值（浏览器 cssText / getPropertyValue 的
// 文本）：url token 带双引号 + font-family 的属性特化。
//
// ★ 依据（Edge --dump-dom 实测，dev/output/wbui-audit/r22url.edge.txt）：
//
//	.a1{background-image:url(foo.png)}   → url("foo.png")
//	.a2{background-image:url("bar.png")} → url("bar.png")
//	.a3{font-family:"Probe Font",serif}  → "Probe Font", serif（含空格 → 保留引号）
//	@font-face{font-family:"ProbeFont"}  → ProbeFont（可作标识符 → 去引号）
//
// 只在 CSSOM 展示层（engine/js/bindings/domctors.go 的规则 style / cssText）
// 使用；引擎内部路径继续走 ValueString（那里 url 不带引号）。
func (d Declaration) CSSTextValue() string {
	v := valueStringOfCSSOM(d.Value)
	if strings.EqualFold(strings.TrimSpace(d.Name), "font-family") {
		v = unquoteFontFamilies(v)
	}
	return v
}

// CSSTextValueOf 把「属性名 + 原始值文本」转成 CSSOM 口径的值文本：url token 带
// 双引号 + font-family 去引号。供**声明来源是字符串**的路径使用 —— 内联 style
// （`el.style`，声明来自 style 属性原文）与 `getComputedStyle` 的值文本。
//
// ★ 依据（Edge --dump-dom 实测，dev/output/wbui-audit/r23base.edge.txt）：
//
//	style="background-image: url(foo.png)"   → getPropertyValue → url("foo.png")
//	style="background-image: url('bar.png')" → getPropertyValue → url("bar.png")
//	style="font-family: 'ProbeFont'"         → getPropertyValue → ProbeFont
//
// ★ 与 CSSTextValue 的关系：CSSTextValue 作用于**解析器产物**（DeclarationValue
// token 列表），本函数作用于**原始声明文本**（未经 token 化）；二者输出同一口径。
// 不含 url( 且非 font-family 的值原样返回（快速短路，避免给热路径加开销）。
func CSSTextValueOf(name, value string) string {
	if value == "" {
		return value
	}
	v := quoteURLTokens(value)
	if isFontFamilyName(name) {
		v = unquoteFontFamilies(v)
	}
	return v
}

// isFontFamilyName 判断属性名是否是 font-family（同时接受 kebab 与 camelCase 写法）。
func isFontFamilyName(name string) bool {
	n := strings.TrimSpace(name)
	return strings.EqualFold(n, "font-family") || strings.EqualFold(n, "fontFamily")
}

// quoteURLTokens 把值文本里**引号外**的 url(...) 统一序列化为 url("...")
// （CSSOM 口径：Chromium 恒以双引号序列化 url token；已带引号的统一为双引号）。
// 字符串字面量内部的 "url(" 不受影响（整段原样复制）。
func quoteURLTokens(v string) string {
	if !strings.Contains(strings.ToLower(v), "url(") {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 8)
	for i := 0; i < len(v); {
		c := v[i]
		if c == '"' || c == '\'' {
			// 字符串字面量：整段复制（含其转义序列）。
			q := c
			b.WriteByte(c)
			i++
			for i < len(v) {
				b.WriteByte(v[i])
				if v[i] == '\\' && i+1 < len(v) {
					i++
					b.WriteByte(v[i])
					i++
					continue
				}
				if v[i] == q {
					i++
					break
				}
				i++
			}
			continue
		}
		if hasURLPrefixAt(v, i) {
			start := i + 4
			j := start
			for j < len(v) && v[j] != ')' {
				j++
			}
			if j < len(v) {
				inner := strings.TrimSpace(v[start:j])
				if len(inner) >= 2 && (inner[0] == '"' && inner[len(inner)-1] == '"' ||
					inner[0] == '\'' && inner[len(inner)-1] == '\'') {
					inner = inner[1 : len(inner)-1]
				}
				b.WriteString("url(\"")
				b.WriteString(strings.ReplaceAll(inner, "\"", "\\\""))
				b.WriteString("\")")
				i = j + 1
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// hasURLPrefixAt 判断 v[i:] 是否以 `url(` 开头（大小写不敏感），且其前一个字符
// 不是标识符字符 —— 后者避免把 `myurl(` 这类自定义标识符误当 url token。
func hasURLPrefixAt(v string, i int) bool {
	if i+4 > len(v) || !strings.EqualFold(v[i:i+4], "url(") {
		return false
	}
	if i == 0 {
		return true
	}
	prev := v[i-1]
	switch {
	case prev == '-' || prev == '_':
		return false
	case prev >= 'a' && prev <= 'z', prev >= 'A' && prev <= 'Z', prev >= '0' && prev <= '9':
		return false
	}
	return true
}

// unquoteFontFamilies 实现 font-family 的 CSSOM 序列化特化：字体名若可写作
// CSS 标识符（不含空格等），序列化时**省略引号**。
//
// ★ Edge 基线（dev/output/wbui-audit/r22url.edge.txt）：
//
//	.a3{font-family:"Probe Font",serif} → font-family: "Probe Font", serif;（含空格 → 保留引号）
//	.a4{font-family:ProbeFont}          → font-family: ProbeFont;（本就是标识符）
//	@font-face{font-family:"ProbeFont"} → font-family: ProbeFont;（可作标识符 → 去引号）
func unquoteFontFamilies(v string) string {
	parts := splitTopLevelCommas(v)
	for i, part := range parts {
		p := strings.TrimSpace(part)
		if len(p) >= 2 && (p[0] == '"' && p[len(p)-1] == '"' || p[0] == '\'' && p[len(p)-1] == '\'') {
			if inner := p[1 : len(p)-1]; isCSSIdentLike(inner) {
				parts[i] = inner
				continue
			}
		}
		parts[i] = p
	}
	return strings.Join(parts, ", ")
}

// splitTopLevelCommas 按**引号外**的逗号切分（字体名内部可能含逗号）。
func splitTopLevelCommas(v string) []string {
	var out []string
	var b strings.Builder
	var quote byte
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			b.WriteByte(c)
		case c == ',':
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(out, b.String())
}

// isCSSIdentLike 判断 s 是否可写作 CSS 标识符（近似判定：首字符非数字，其余为
// 字母 / 数字 / 连字符 / 下划线 / 非 ASCII）。用于 font-family 的去引号特化。
func isCSSIdentLike(s string) bool {
	if s == "" || s == "-" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			continue
		}
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '-':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// valueStringOf serializes a token slice into CSSOM-style text: whitespace and
// comment tokens are dropped, the inter-token separator follows browser rules
// (see appendValueSeparator) — rgb(0,128,0) round-trips as "rgb(0, 128, 0)".
func valueStringOf(toks []Token) string { return valueStringOfMode(toks, false) }

// valueStringOfCSSOM 与 valueStringOf 相同，只有 url token 按 CSSOM 口径带双引号
// （见 Declaration.CSSTextValue）。
func valueStringOfCSSOM(toks []Token) string { return valueStringOfMode(toks, true) }

func valueStringOfMode(toks []Token, cssomURL bool) string {
	var sb []byte
	for i, tok := range toks {
		if isValueSeparatorToken(tok) {
			continue
		}
		if len(sb) > 0 {
			appendValueSeparator(&sb, toks[i-1], tok)
		}
		if cssomURL {
			sb = append(sb, serializeTokenCSSOM(tok)...)
		} else {
			sb = append(sb, serializeToken(tok)...)
		}
	}
	return string(sb)
}

// isValueSeparatorToken reports whether the token is pure whitespace/comment —
// tokenizer noise that must not appear in a serialized value (browsers drop it;
// only the single inter-token separator remains).
func isValueSeparatorToken(t Token) bool {
	switch t.Type {
	case TokenNonNewlineWhitespace, TokenNewline, TokenComment:
		return true
	}
	return false
}

// appendValueSeparator writes the CSSOM component-value separator between two
// consecutive tokens, matching how browsers serialize computed values:
//   - before a comma → NO space:   rgb(0, 128, 0)
//   - after a function token "rgb(" → NO space:  rgb(0, 128, 0)
//   - after a comma → ONE space:                  rgb(0, 128, 0)
//   - otherwise → ONE space:                      1px solid red
//
// Without this, <style>background: rgb(0,128,0)</style> round-trips through the
// token stream as "rgb( 0 , 128 , 0 )" — getComputedStyle().backgroundColor
// differs from Chrome's "rgb(0, 128, 0)" and string comparisons fail.
func appendValueSeparator(sb *[]byte, prev, cur Token) {
	if cur.Type == TokenComma || cur.Type == TokenRightParenthesis {
		// "0," / "0)" — no space before the comma or closing paren.
		return
	}
	switch prev.Type {
	case TokenFunction:
		// "rgb(" already ends with "("; no separator.
		return
	default:
		// "0, 128" / "1px solid red"
		*sb = append(*sb, ' ')
	}
}

// serializeTokenCSSOM 与 serializeToken 相同，但 url token 按 CSSOM 口径带双引号
// （tokenizer 已剥掉原文的引号 —— 带引号与不带引号两种写法都产出 Value 不含引号
// 的 TokenURL）。基线：dev/output/wbui-audit/r22url.edge.txt。仅供 CSSTextValue 用。
func serializeTokenCSSOM(t Token) string {
	if t.Type == TokenURL {
		return "url(\"" + t.Value + "\")"
	}
	return serializeToken(t)
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
	case TokenURL:
		return "url(" + t.Value + ")"
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
	Href     string
	Media    string
	Supports string
	Origin   Origin
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
	Name      string
	Keyframes []KeyframeRule
	Origin    Origin
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
