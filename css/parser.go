// Translation of: Source/WebCore/css/parser/CSSParser.cpp
//                  Source/WebCore/css/parser/CSSParser.h
//                  Source/WebCore/css/parser/CSSSelectorParser.cpp
//                  Source/WebCore/css/parser/CSSPropertyParser.cpp
// Completeness: 70%
// Simplifications:
//   - the parser is a top-down recursive descent over the token stream rather than
//     the spec's "consume a list of rules" state machine
//   - @-rules beyond @media / @import / @font-face / @keyframes / @page / @namespace /
//     @supports are dropped (parsed-and-skipped) rather than stored
//   - CSS nesting is supported only inside style rules; nested at-rules are limited
//     to @media
//   - selectors are parsed by parseSelectorList, which supports the four combinators
//     and the pseudo-class/element forms enumerated in selector.go
//   - declaration values are stored as token slices; value interpretation (color,
//     length, calc/var) is deferred to the resolver
//   - error recovery is best-effort: a parse error in a rule skips to the matching
//     closing brace or next semicolon, mirroring the spec's "error recovery" rules
//   - the parser is stateless beyond a position cursor; CSSParserContext options are
//     folded into struct fields on the Parser

package css

import (
	"strings"
)

// Parser is the Go translation of WebCore::CSSParser. It owns a token stream
// (produced by Tokenizer) and exposes ParseStyleSheet as the top-level entry point.
// Lower-level entry points (ParseRule, ParseSelectorList, ParseDeclarationList) are
// provided for use by the CSSOM API and the resolver's inline-style path.
type Parser struct {
	tokens []Token
	pos    int
	origin Origin
}

// NewParser constructs a Parser over the tokenized input. The origin defaults to
// author; callers can override via SetOrigin before parsing.
func NewParser(input string) *Parser {
	t := NewTokenizer(input)
	return &Parser{tokens: t.Tokenize(), origin: OriginAuthor}
}

// NewParserWithOrigin constructs a Parser and assigns a cascade origin.
func NewParserWithOrigin(input string, origin Origin) *Parser {
	p := NewParser(input)
	p.origin = origin
	return p
}

// SetOrigin overrides the cascade origin for subsequently parsed rules.
func (p *Parser) SetOrigin(o Origin) { p.origin = o }

// ParseStyleSheet parses the entire input as a stylesheet, mirroring
// CSSParser::parseSheet. The returned slice is freshly allocated; parse errors in
// individual rules do not abort the parse.
func (p *Parser) ParseStyleSheet() []Rule {
	var rules []Rule
	for !p.atEnd() {
		// Skip whitespace and stray semicolons at the top level.
		p.skipWhitespaceAndSemicolons()
		if p.atEnd() {
			break
		}
		rule := p.parseQualifiedRuleOrAtRule()
		if rule != nil {
			rules = append(rules, rule)
		}
	}
	return rules
}

// ParseRule parses a single rule (used by CSSStyleSheet.insertRule). The rule may be
// either a style rule or an at-rule.
func (p *Parser) ParseRule() Rule {
	p.skipWhitespaceAndSemicolons()
	if p.atEnd() {
		return nil
	}
	r := p.parseQualifiedRuleOrAtRule()
	return r
}

// ParseSelectorList parses a comma-separated selector list, returning nil on error.
func (p *Parser) ParseSelectorList() *SelectorList {
	list, _ := p.parseSelectorList()
	return list
}

// ParseDeclarationList parses a declaration list (the body of a style rule), used
// by the CSSOM style attribute path.
func (p *Parser) ParseDeclarationList() []Declaration {
	return p.parseDeclarationsUntil(TokenRightBrace, TokenEOF)
}

// parseQualifiedRuleOrAtRule consumes one top-level rule. The next token determines
// whether it is an at-rule (TokenAtKeyword) or a qualified rule (anything else).
func (p *Parser) parseQualifiedRuleOrAtRule() Rule {
	tok := p.peek()
	if tok.Type == TokenAtKeyword {
		return p.parseAtRule()
	}
	return p.parseQualifiedRule()
}

// parseAtRule consumes an at-rule. The leading at-keyword has not yet been consumed
// when this is called.
func (p *Parser) parseAtRule() Rule {
	tok := p.consume()
	name := strings.ToLower(tok.Value)
	switch name {
	case "import":
		return p.parseImportRule()
	case "media":
		return p.parseMediaRule()
	case "font-face":
		return p.parseFontFaceRule()
	case "keyframes", "-webkit-keyframes":
		return p.parseKeyframesRule(name)
	case "page":
		return p.parsePageRule()
	case "namespace":
		return p.parseNamespaceRule()
	case "supports":
		return p.parseSupportsRule()
	case "charset":
		// Drop the rest of the rule.
		p.skipUntil(TokenSemicolon)
		p.consume() // consume the semicolon if present
		return nil
	default:
		// Unknown at-rule: skip to the matching closing brace or semicolon.
		p.skipUnknownAtRule()
		return nil
	}
}

// parseImportRule parses the body of @import. The at-keyword has been consumed; the
// rest is "href [media] [supports(...)];".
func (p *Parser) parseImportRule() Rule {
	p.skipWhitespace()
	href := p.consumeStringOrURL()
	p.skipWhitespace()
	media := p.consumeUntilSemicolonOrBrace()
	rule := &ImportRule{Href: href, Media: strings.TrimSpace(media), Origin: p.origin}
	if p.peek().Type == TokenSemicolon {
		p.consume()
	}
	return rule
}

// parseMediaRule parses the body of @media: a condition string and a block of rules.
func (p *Parser) parseMediaRule() Rule {
	condition := p.consumeUntilLeftBrace()
	if p.peek().Type == TokenLeftBrace {
		p.consume()
	}
	condStr := strings.TrimSpace(condition)
	rule := &MediaRule{Condition: condStr, Origin: p.origin}
	// Attempt to parse the condition string as a media query list. Parsing errors
	// are silently ignored; the rule is still created with an empty Parsed slice so
	// it acts as if the media query always matches (backward-compatible behaviour).
	if parsed, err := ParseMediaQueryList(condStr); err == nil {
		rule.Parsed = parsed
	}
	for {
		p.skipWhitespaceAndSemicolons()
		if p.atEnd() {
			break
		}
		if p.peek().Type == TokenRightBrace {
			p.consume()
			break
		}
		nested := p.parseQualifiedRuleOrAtRule()
		if nested != nil {
			rule.Rules = append(rule.Rules, nested)
		}
	}
	return rule
}

// parseFontFaceRule parses the body of @font-face: a block of declarations.
func (p *Parser) parseFontFaceRule() Rule {
	p.skipWhitespace()
	rule := &FontFaceRule{Origin: p.origin}
	if p.peek().Type == TokenLeftBrace {
		p.consume()
		rule.Declarations = p.parseDeclarationsUntil(TokenRightBrace, TokenEOF)
	}
	if p.peek().Type == TokenRightBrace {
		p.consume()
	}
	return rule
}

// parseKeyframesRule parses the body of @keyframes: a name and a series of keyframe
// blocks ("0% { ... }", "from { ... }", "to { ... }").
func (p *Parser) parseKeyframesRule(name string) Rule {
	p.skipWhitespace()
	nameTok := p.consume()
	rule := &KeyframesRule{Name: nameTok.Value, Origin: p.origin}
	p.skipWhitespace()
	if p.peek().Type != TokenLeftBrace {
		// Malformed; bail.
		return rule
	}
	p.consume()
	for {
		p.skipWhitespaceAndSemicolons()
		if p.atEnd() || p.peek().Type == TokenRightBrace {
			if p.peek().Type == TokenRightBrace {
				p.consume()
			}
			break
		}
		// Read keyframe selector list (comma-separated percentages or "from"/"to").
		var keys []string
		for {
			p.skipWhitespace()
			tok := p.peek()
			if tok.Type == TokenIdent {
				p.consume()
				keys = append(keys, tok.Value)
			} else if tok.Type == TokenPercentage {
				p.consume()
				keys = append(keys, serializeToken(tok))
			} else if tok.Type == TokenDimension {
				p.consume()
				keys = append(keys, serializeToken(tok))
			} else {
				break
			}
			p.skipWhitespace()
			if p.peek().Type != TokenComma {
				break
			}
			p.consume()
		}
		p.skipWhitespace()
		if p.peek().Type != TokenLeftBrace {
			// Skip to the next block.
			p.skipUntil(TokenRightBrace)
			if p.peek().Type == TokenRightBrace {
				p.consume()
			}
			continue
		}
		p.consume()
		decls := p.parseDeclarationsUntil(TokenRightBrace, TokenEOF)
		if p.peek().Type == TokenRightBrace {
			p.consume()
		}
		rule.Keyframes = append(rule.Keyframes, KeyframeRule{Keys: keys, Declarations: decls})
	}
	return rule
}

// parsePageRule parses the body of @page.
func (p *Parser) parsePageRule() Rule {
	selector := strings.TrimSpace(p.consumeUntilLeftBrace())
	if p.peek().Type == TokenLeftBrace {
		p.consume()
	}
	rule := &PageRule{Selector: selector, Origin: p.origin}
	rule.Declarations = p.parseDeclarationsUntil(TokenRightBrace, TokenEOF)
	if p.peek().Type == TokenRightBrace {
		p.consume()
	}
	return rule
}

// parseNamespaceRule parses the body of @namespace: optional prefix and a URL.
func (p *Parser) parseNamespaceRule() Rule {
	p.skipWhitespace()
	prefix := ""
	tok := p.peek()
	if tok.Type == TokenIdent {
		p.consume()
		prefix = tok.Value
		p.skipWhitespace()
	}
	uri := p.consumeStringOrURL()
	if p.peek().Type == TokenSemicolon {
		p.consume()
	}
	return &NamespaceRule{Prefix: prefix, URI: uri}
}

// parseSupportsRule parses the body of @supports: a condition and a block of rules.
func (p *Parser) parseSupportsRule() Rule {
	condition := p.consumeUntilLeftBrace()
	if p.peek().Type == TokenLeftBrace {
		p.consume()
	}
	rule := &SupportsRule{Condition: strings.TrimSpace(condition), Origin: p.origin}
	for {
		p.skipWhitespaceAndSemicolons()
		if p.atEnd() {
			break
		}
		if p.peek().Type == TokenRightBrace {
			p.consume()
			break
		}
		nested := p.parseQualifiedRuleOrAtRule()
		if nested != nil {
			rule.Rules = append(rule.Rules, nested)
		}
	}
	return rule
}

// skipUnknownAtRule skips over the body of an unrecognized at-rule.
func (p *Parser) skipUnknownAtRule() {
	// Look for either a ; (no block) or a balanced { ... } block.
	for !p.atEnd() {
		tok := p.peek()
		switch tok.Type {
		case TokenSemicolon:
			p.consume()
			return
		case TokenLeftBrace:
			p.skipUntil(TokenRightBrace)
			if p.peek().Type == TokenRightBrace {
				p.consume()
			}
			return
		case TokenEOF:
			return
		}
		p.consume()
	}
}

// parseQualifiedRule parses a style rule (selector list + declaration block). The
// leading at-keyword, if any, has been consumed by the caller.
func (p *Parser) parseQualifiedRule() Rule {
	sel, ok := p.parseSelectorList()
	if !ok {
		// Bad selector — skip to next ; or matching { ... } block.
		p.skipRuleBody()
		return nil
	}
	p.skipWhitespace()
	if p.peek().Type != TokenLeftBrace {
		p.skipRuleBody()
		return nil
	}
	p.consume() // {
	rule := &StyleRule{Selectors: sel, Origin: p.origin}
	// We parse declarations and nested rules in document order so that nested
	// style rules appear in NestedRules while declarations appear in Declarations.
	rule.Declarations, rule.NestedRules = p.parseStyleRuleBody()
	if p.peek().Type == TokenRightBrace {
		p.consume()
	}
	return rule
}

// parseStyleRuleBody parses the contents of a style rule block, returning both the
// flat declaration list and the nested rule list (CSS Nesting). The leading { has
// been consumed; this function consumes the matching }.
func (p *Parser) parseStyleRuleBody() ([]Declaration, []Rule) {
	var decls []Declaration
	var nested []Rule
	for {
		p.skipWhitespaceAndSemicolons()
		tok := p.peek()
		switch tok.Type {
		case TokenRightBrace, TokenEOF:
			return decls, nested
		case TokenAtKeyword:
			// Nested at-rule inside a style rule. We support @media only here.
			if strings.ToLower(tok.Value) == "media" {
				if r := p.parseAtRule(); r != nil {
					nested = append(nested, r)
				}
			} else {
				p.skipUnknownAtRule()
			}
		default:
			// Either a declaration or a nested style rule. We attempt a declaration
			// first; if the lookahead isn't a "ident colon ..." pattern we treat it
			// as a nested style rule selector list.
			if p.looksLikeDeclaration() {
				if d := p.parseDeclaration(); d.Name != "" {
					decls = append(decls, d)
				}
			} else {
				if r := p.parseQualifiedRule(); r != nil {
					nested = append(nested, r)
				}
			}
		}
	}
}

// looksLikeDeclaration reports whether the next tokens form "ident colon", the start
// of a declaration. We need this to disambiguate declarations from nested style
// rules (CSS Nesting).
func (p *Parser) looksLikeDeclaration() bool {
	if p.peek().Type != TokenIdent {
		return false
	}
	// Scan ahead skipping whitespace and comments to find the next non-whitespace.
	for i := 1; ; i++ {
		t := p.peekAt(i)
		if t.Type == TokenNonNewlineWhitespace || t.Type == TokenNewline || t.Type == TokenComment {
			continue
		}
		return t.Type == TokenColon
	}
}

// parseDeclaration parses one "name: value; !important" entry. The trailing ; is
// consumed if present.
func (p *Parser) parseDeclaration() Declaration {
	nameTok := p.consume()
	if nameTok.Type != TokenIdent {
		// Skip to the next ; or }
		p.skipUntil(TokenSemicolon, TokenRightBrace)
		if p.peek().Type == TokenSemicolon {
			p.consume()
		}
		return Declaration{}
	}
	p.skipWhitespace()
	if p.peek().Type != TokenColon {
		p.skipUntil(TokenSemicolon, TokenRightBrace)
		if p.peek().Type == TokenSemicolon {
			p.consume()
		}
		return Declaration{}
	}
	p.consume() // :
	p.skipWhitespace()
	decl := Declaration{Name: strings.ToLower(nameTok.Value)}
	// Value is everything up to ; or } or !important.
	var valueTokens []Token
	for {
		t := p.peek()
		switch t.Type {
		case TokenSemicolon:
			p.consume()
			decl.Value = valueTokens
			return decl
		case TokenRightBrace, TokenEOF:
			decl.Value = valueTokens
			return decl
		case TokenDelimiter:
			if t.Delimiter == '!' {
				// Look for "important" identifier following.
				p.consume()
				p.skipWhitespace()
				if p.peek().Type == TokenIdent && strings.EqualFold(p.peek().Value, "important") {
					p.consume()
					decl.Important = true
				}
				p.skipWhitespace()
				if p.peek().Type == TokenSemicolon {
					p.consume()
				}
				decl.Value = valueTokens
				return decl
			}
			p.consume()
			valueTokens = append(valueTokens, t)
		case TokenComment:
			p.consume() // drop comment tokens from value
		default:
			p.consume()
			valueTokens = append(valueTokens, t)
		}
	}
}

// parseDeclarationsUntil parses declarations until one of the terminator token types
// is reached. The terminator itself is not consumed.
func (p *Parser) parseDeclarationsUntil(terminators ...TokenType) []Declaration {
	var decls []Declaration
	for {
		p.skipWhitespaceAndSemicolons()
		t := p.peek()
		if t.Type == TokenEOF {
			return decls
		}
		for _, term := range terminators {
			if t.Type == term {
				return decls
			}
		}
		if t.Type == TokenAtKeyword {
			// Nested at-rules inside a declaration context are skipped.
			p.skipUnknownAtRule()
			continue
		}
		if !p.looksLikeDeclaration() {
			// Skip to the next ; to recover.
			p.skipUntil(TokenSemicolon)
			if p.peek().Type == TokenSemicolon {
				p.consume()
			}
			continue
		}
		if d := p.parseDeclaration(); d.Name != "" {
			decls = append(decls, d)
		}
	}
}

// skipRuleBody skips tokens until the matching closing brace or next top-level
// semicolon, used for error recovery.
func (p *Parser) skipRuleBody() {
	for !p.atEnd() {
		t := p.peek()
		switch t.Type {
		case TokenSemicolon:
			p.consume()
			return
		case TokenLeftBrace:
			p.skipUntil(TokenRightBrace)
			if p.peek().Type == TokenRightBrace {
				p.consume()
			}
			return
		case TokenEOF:
			return
		}
		p.consume()
	}
}

// skipUntil consumes tokens until a token of one of the given types is the next
// token (without consuming it).
func (p *Parser) skipUntil(kinds ...TokenType) {
	depth := 0
	for !p.atEnd() {
		t := p.peek()
		if t.Type == TokenLeftBrace || t.Type == TokenLeftParenthesis || t.Type == TokenLeftBracket {
			depth++
		}
		if t.Type == TokenRightBrace || t.Type == TokenRightParenthesis || t.Type == TokenRightBracket {
			if depth == 0 {
				// Don't consume the unmatched close brace.
				for _, k := range kinds {
					if t.Type == k {
						return
					}
				}
				return
			}
			depth--
		}
		for _, k := range kinds {
			if t.Type == k && depth == 0 {
				return
			}
		}
		p.consume()
	}
}

// consumeUntilLeftBrace consumes tokens until a { is the next token (without
// consuming the brace) and returns the consumed tokens serialized as a string.
func (p *Parser) consumeUntilLeftBrace() string {
	var sb strings.Builder
	for !p.atEnd() {
		t := p.peek()
		if t.Type == TokenLeftBrace {
			return sb.String()
		}
		p.consume()
		if t.Type == TokenSemicolon {
			return sb.String()
		}
		sb.WriteString(serializeToken(t))
		sb.WriteByte(' ')
	}
	return sb.String()
}

// consumeUntilSemicolonOrBrace consumes tokens until ; or { is the next token and
// returns the consumed tokens serialized as a string.
func (p *Parser) consumeUntilSemicolonOrBrace() string {
	var sb strings.Builder
	for !p.atEnd() {
		t := p.peek()
		if t.Type == TokenSemicolon || t.Type == TokenLeftBrace {
			return sb.String()
		}
		p.consume()
		sb.WriteString(serializeToken(t))
		sb.WriteByte(' ')
	}
	return sb.String()
}

// consumeStringOrURL consumes a single string or url() token and returns its value.
// If the next token is not a string or url, this consumes one token and returns its
// value (best-effort).
func (p *Parser) consumeStringOrURL() string {
	t := p.peek()
	switch t.Type {
	case TokenString:
		p.consume()
		return t.Value
	case TokenURL:
		p.consume()
		return t.Value
	case TokenFunction:
		// url(...) with whitespace; consume the function body.
		p.consume()
		var sb strings.Builder
		for !p.atEnd() {
			tok := p.peek()
			if tok.Type == TokenRightParenthesis {
				p.consume()
				break
			}
			if tok.Type == TokenEOF {
				break
			}
			p.consume()
			sb.WriteString(serializeToken(tok))
		}
		return strings.TrimSpace(sb.String())
	}
	p.consume()
	return t.Value
}

// --- Selector parsing -------------------------------------------------------

// parseSelectorList parses a comma-separated list of complex selectors, stopping at
// the next { (which is not consumed). Returns the list and whether parsing
// succeeded.
func (p *Parser) parseSelectorList() (*SelectorList, bool) {
	list := &SelectorList{}
	for {
		p.skipWhitespace()
		sel, ok := p.parseComplexSelector()
		if !ok {
			return nil, false
		}
		list.Selectors = append(list.Selectors, sel)
		p.skipWhitespace()
		t := p.peek()
		if t.Type == TokenComma {
			p.consume()
			continue
		}
		if t.Type == TokenLeftBrace || t.Type == TokenEOF {
			return list, true
		}
		// Unexpected token; bail.
		return list, false
	}
}

// parseComplexSelector parses a single complex selector with combinators.
func (p *Parser) parseComplexSelector() (ComplexSelector, bool) {
	var cs ComplexSelector
	comp, ok := p.parseCompoundSelector()
	if !ok {
		return cs, false
	}
	cs.Compounds = append(cs.Compounds, comp)
	for {
		// Determine the combinator: explicit (>, +, ~) or implicit descendant
		// (whitespace). The loop consumes whitespace and comments first to detect
		// an explicit combinator.
		hadWhitespace := false
		for {
			t := p.peek()
			if t.Type == TokenNonNewlineWhitespace || t.Type == TokenNewline {
				p.consume()
				hadWhitespace = true
				continue
			}
			if t.Type == TokenComment {
				p.consume()
				continue
			}
			break
		}
		t := p.peek()
		if t.Type == TokenComma || t.Type == TokenLeftBrace || t.Type == TokenEOF ||
			t.Type == TokenRightBrace || t.Type == TokenRightParenthesis {
			return cs, true
		}
		var rel Relation
		switch t.Type {
		case TokenDelimiter:
			switch t.Delimiter {
			case '>':
				p.consume()
				rel = RelationChild
			case '+':
				p.consume()
				rel = RelationDirectAdjacent
			case '~':
				p.consume()
				rel = RelationIndirectAdjacent
			default:
				// Unknown combinator — bail.
				return cs, false
			}
		default:
			if !hadWhitespace {
				// No combinator, no whitespace; we shouldn't get here normally.
				return cs, true
			}
			rel = RelationDescendant
		}
		// Skip whitespace after an explicit combinator.
		if rel != RelationDescendant {
			for {
				t := p.peek()
				if t.Type == TokenNonNewlineWhitespace || t.Type == TokenNewline {
					p.consume()
					continue
				}
				if t.Type == TokenComment {
					p.consume()
					continue
				}
				break
			}
		}
		comp, ok := p.parseCompoundSelector()
		if !ok {
			return cs, false
		}
		comp.Relation = rel
		cs.Compounds = append(cs.Compounds, comp)
	}
}

// parseCompoundSelector parses a sequence of simple selectors with no combinator.
func (p *Parser) parseCompoundSelector() (CompoundSelector, bool) {
	var comp CompoundSelector
	comp.Relation = RelationSubselector
	for {
		t := p.peek()
		switch t.Type {
		case TokenIdent:
			p.consume()
			comp.Selectors = append(comp.Selectors, SimpleSelector{Match: MatchTag, Value: t.Value})
		case TokenHash:
			p.consume()
			comp.Selectors = append(comp.Selectors, SimpleSelector{
				Match: MatchID,
				Value: t.Value,
			})
		case TokenDelimiter:
			switch t.Delimiter {
			case '.':
				p.consume()
				name := p.peek()
				if name.Type != TokenIdent {
					return comp, false
				}
				p.consume()
				comp.Selectors = append(comp.Selectors, SimpleSelector{Match: MatchClass, Value: name.Value})
			case '*':
				p.consume()
				// Universal selector; we model it as a tag with name "*".
				comp.Selectors = append(comp.Selectors, SimpleSelector{Match: MatchTag, Value: "*"})
			case '|':
				// Namespace prefix; consume and ignore in this simplified port.
				p.consume()
				// Following must be an ident or *.
				if next := p.peek(); next.Type == TokenIdent || (next.Type == TokenDelimiter && next.Delimiter == '*') {
					p.consume()
				}
			default:
				if len(comp.Selectors) == 0 {
					return comp, false
				}
				return comp, true
			}
		case TokenColon:
			p.consume()
			ss, ok := p.parsePseudo()
			if !ok {
				return comp, false
			}
			comp.Selectors = append(comp.Selectors, ss)
		case TokenLeftBracket:
			p.consume()
			ss, ok := p.parseAttributeSelector()
			if !ok {
				return comp, false
			}
			comp.Selectors = append(comp.Selectors, ss)
		default:
			if len(comp.Selectors) == 0 {
				return comp, false
			}
			return comp, true
		}
	}
}

// parsePseudo parses a pseudo-class or pseudo-element following the leading colon(s).
func (p *Parser) parsePseudo() (SimpleSelector, bool) {
	colons := 1
	if p.peek().Type == TokenColon {
		p.consume()
		colons = 2
	}
	nameTok := p.peek()
	if nameTok.Type != TokenIdent && nameTok.Type != TokenFunction {
		return SimpleSelector{}, false
	}
	// TokenFunction means the tokenizer already consumed the trailing '(' (e.g.
	// "not("); for TokenIdent the pseudo may still be functional if a '(' follows.
	isFunctional := nameTok.Type == TokenFunction
	name := nameTok.Value
	p.consume()
	if !isFunctional && p.peek().Type == TokenLeftParenthesis {
		isFunctional = true
		p.consume() // consume the '('
	}
	match, pc, pe := ParsePseudoElementOrClass(colons, name)
	ss := SimpleSelector{Match: match, PseudoClass: pc, PseudoElem: pe}
	if !isFunctional {
		return ss, true
	}
	// Capture the raw tokens up to the matching ).
	argTokens := p.consumeParenBody()
	if argTokens == nil {
		return SimpleSelector{}, false
	}
	if p.peek().Type == TokenRightParenthesis {
		p.consume()
	}
	// For :is / :where / :not / :has, parse the argument as a selector list.
	if pc == PseudoClassIs || pc == PseudoClassWhere || pc == PseudoClassNot || pc == PseudoClassHas {
		argParser := Parser{tokens: append(argTokens, Token{Type: TokenEOF})}
		list, ok := argParser.parseSelectorList()
		if !ok {
			return SimpleSelector{}, false
		}
		ss.SelectorList = list
		return ss, true
	}
	// For :nth-* the argument is an An+B expression.
	if pc == PseudoClassNthChild || pc == PseudoClassNthLastChild ||
		pc == PseudoClassNthOfType || pc == PseudoClassNthLastOfType {
		a, b := parseAnB(argTokens)
		ss.NthA = a
		ss.NthB = b
		// Optional "of S" clause: parse the trailing selector list after the of
		// keyword if present.
		var ofTokens []Token
		inOf := false
		for _, t := range argTokens {
			if inOf {
				ofTokens = append(ofTokens, t)
				continue
			}
			if t.Type == TokenIdent && strings.EqualFold(t.Value, "of") {
				inOf = true
			}
		}
		if inOf {
			ofParser := Parser{tokens: append(ofTokens, Token{Type: TokenEOF})}
			if list, ok := ofParser.parseSelectorList(); ok {
				ss.SelectorList = list
			}
		}
		return ss, true
	}
	// For :lang the argument is a comma-separated list of strings.
	if pc == PseudoClassLang {
		var langs []string
		for _, t := range argTokens {
			if t.Type == TokenString || t.Type == TokenIdent {
				langs = append(langs, t.Value)
			}
		}
		ss.StringList = langs
		return ss, true
	}
	// For :dir the argument is an identifier.
	for _, t := range argTokens {
		if t.Type == TokenIdent {
			ss.Argument = t.Value
			break
		}
	}
	return ss, true
}

// consumeParenBody consumes tokens up to the matching ) and returns them. The
// leading ( has already been consumed; the matching ) is left in place.
func (p *Parser) consumeParenBody() []Token {
	var out []Token
	depth := 0
	for !p.atEnd() {
		t := p.peek()
		if t.Type == TokenLeftParenthesis {
			depth++
			p.consume()
			out = append(out, t)
			continue
		}
		if t.Type == TokenRightParenthesis {
			if depth == 0 {
				return out
			}
			depth--
			p.consume()
			out = append(out, t)
			continue
		}
		if t.Type == TokenEOF {
			return nil
		}
		p.consume()
		out = append(out, t)
	}
	return out
}

// parseAttributeSelector parses the contents of [attr=value] after the [ has been
// consumed. The trailing ] is consumed.
func (p *Parser) parseAttributeSelector() (SimpleSelector, bool) {
	p.skipWhitespace()
	nameTok := p.peek()
	if nameTok.Type != TokenIdent {
		return SimpleSelector{}, false
	}
	p.consume()
	attr := nameTok.Value
	p.skipWhitespace()
	ss := SimpleSelector{Attribute: attr, Match: MatchSet}
	t := p.peek()
	if t.Type == TokenRightBracket {
		p.consume()
		return ss, true
	}
	// Operator: one of = ~= |= ^= $= *=. The tokenizer emits the combined forms
	// (~=, |=, ^=, $=, *=) as their own token types (TokenIncludeMatch etc.), so we
	// handle both the single-delimiter form and the combined-token form here.
	var matchType Match
	switch t.Type {
	case TokenDelimiter:
		switch t.Delimiter {
		case '=':
			p.consume()
			matchType = MatchExact
		default:
			return SimpleSelector{}, false
		}
	case TokenIncludeMatch: // ~=
		p.consume()
		matchType = MatchList
	case TokenDashMatch: // |=
		p.consume()
		matchType = MatchHyphen
	case TokenPrefixMatch: // ^=
		p.consume()
		matchType = MatchBegin
	case TokenSuffixMatch: // $=
		p.consume()
		matchType = MatchEnd
	case TokenSubstringMatch: // *=
		p.consume()
		matchType = MatchContain
	default:
		return SimpleSelector{}, false
	}
	p.skipWhitespace()
	// Value is a string or ident.
	v := p.peek()
	if v.Type != TokenString && v.Type != TokenIdent {
		return SimpleSelector{}, false
	}
	p.consume()
	ss.Value = v.Value
	ss.Match = matchType
	p.skipWhitespace()
	// Optional case-sensitivity flag: 'i' or 's'.
	if flag := p.peek(); flag.Type == TokenIdent {
		switch strings.ToLower(flag.Value) {
		case "i":
			ss.AttrMatch = AttributeMatchCaseInsensitive
			p.consume()
			p.skipWhitespace()
		case "s":
			ss.AttrMatch = AttributeMatchCaseSensitive
			p.consume()
			p.skipWhitespace()
		}
	}
	if p.peek().Type != TokenRightBracket {
		return SimpleSelector{}, false
	}
	p.consume()
	return ss, true
}

// parseAnB parses the An+B expression from a token slice. Supports:
//   - integer (e.g. "3" → 0n+3)
//   - "odd" → 2n+1
//   - "even" → 2n+0
//   - "n", "-n", "+n"
//   - "An+B" with optional sign and B
func parseAnB(tokens []Token) (int, int) {
	// Concatenate identifiers, numbers, dimensions, and "+/-"/"n" into a single
	// string and parse that with the standard algorithm.
	var sb strings.Builder
	for _, t := range tokens {
		switch t.Type {
		case TokenIdent:
			sb.WriteString(t.Value)
		case TokenNumber:
			sb.WriteString(floatToString(t.Numeric))
		case TokenDimension:
			sb.WriteString(floatToString(t.Numeric))
			sb.WriteString(t.Unit)
		case TokenDelimiter:
			sb.WriteRune(t.Delimiter)
		case TokenPercentage:
			sb.WriteString(floatToString(t.Numeric))
		}
	}
	expr := strings.ToLower(strings.TrimSpace(sb.String()))
	if expr == "" {
		return 0, 0
	}
	if expr == "odd" {
		return 2, 1
	}
	if expr == "even" {
		return 2, 0
	}
	// Find the n.
	idx := strings.Index(expr, "n")
	if idx == -1 {
		// Pure number.
		b := atoi(expr)
		return 0, b
	}
	aStr := strings.TrimSpace(expr[:idx])
	a := 1
	switch aStr {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		a = atoi(aStr)
	}
	bStr := strings.TrimSpace(expr[idx+1:])
	if bStr == "" {
		return a, 0
	}
	b := 0
	if strings.HasPrefix(bStr, "+") {
		b = atoi(bStr[1:])
	} else if strings.HasPrefix(bStr, "-") {
		b = -atoi(bStr[1:])
	} else {
		b = atoi(bStr)
	}
	return a, b
}

// atoi parses a (possibly signed) integer. Returns 0 on failure.
func atoi(s string) int {
	if s == "" {
		return 0
	}
	neg := false
	i := 0
	if s[0] == '+' {
		i++
	} else if s[0] == '-' {
		neg = true
		i++
	}
	n := 0
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// --- Token cursor primitives ------------------------------------------------

// peek returns the next token without advancing.
func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

// peekAt returns the token at offset off from the current position. EOF beyond end.
func (p *Parser) peekAt(off int) Token {
	idx := p.pos + off
	if idx < 0 || idx >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[idx]
}

// consume returns the next token and advances the cursor.
func (p *Parser) consume() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// atEnd reports whether the cursor is past the end of the token stream or has
// reached the EOF sentinel token (which Tokenize always appends as the last token).
func (p *Parser) atEnd() bool {
	if p.pos >= len(p.tokens) {
		return true
	}
	return p.tokens[p.pos].Type == TokenEOF
}

// skipWhitespaceAndSemicolons consumes whitespace, comments, and stray semicolons.
func (p *Parser) skipWhitespaceAndSemicolons() {
	for {
		t := p.peek()
		switch t.Type {
		case TokenNonNewlineWhitespace, TokenNewline, TokenComment, TokenSemicolon:
			p.consume()
			continue
		}
		return
	}
}

// skipWhitespace consumes whitespace and comment tokens.
func (p *Parser) skipWhitespace() {
	for {
		t := p.peek()
		switch t.Type {
		case TokenNonNewlineWhitespace, TokenNewline, TokenComment:
			p.consume()
			continue
		}
		return
	}
}

// ParseStyleSheetInto runs ParseStyleSheet and appends the resulting rules to sheet.
func (p *Parser) ParseStyleSheetInto(sheet *CSSStyleSheet) {
	for _, r := range p.ParseStyleSheet() {
		sheet.AppendRule(r)
	}
}
