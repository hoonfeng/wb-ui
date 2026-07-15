// Translation of: Source/WebCore/css/calc/CSSCalcExpressionNode.h
//                  Source/WebCore/css/calc/CSSCalcOperationNode.h
//                  Source/WebCore/css/calc/CSSCalcValueNode.h
//                  Source/WebCore/css/calc/CSSCalcValue.cpp
// Completeness: 60%
// Simplifications:
//   - no type-checking (CSS calc() restricts mixing types; this port resolves all
//     values to pixels at evaluation time using the provided context)
//   - no min() / max() / clamp() support
//   - no rounding / mod / trigonometric operations
//   - sign handling: leading '+' or '-' on a value node is treated as unary operator
//     (e.g. calc(-10px + 5px) == -5px)
package css

import (
	"fmt"
	"math"
)

// CalcContext provides the contextual dimensions needed to resolve relative units
// (%, em, rem, vw, vh) in a calc() expression to absolute pixels.
type CalcContext struct {
	ParentWidth   float64 // parent element content width (px)
	FontSize      float64 // current element font-size (px)
	RootFontSize  float64 // root element font-size (px)
	ViewportWidth  float64 // viewport width (px)
	ViewportHeight float64 // viewport height (px)
}

// EvalCalc evaluates a calc() expression embedded in a declaration value token slice.
// It searches the token slice for a TokenFunction with Value "calc", extracts the
// inner expression up to the matching TokenRightParenthesis, and evaluates it using
// the provided context. Returns the computed pixel value or 0 and an error.
//
// The function handles:
//   - arithmetic: +, -, *, / with standard precedence (* / before + -)
//   - parentheses: (expr) for grouping
//   - unary +/-
//   - units: px, %, em, rem, vw, vh (resolved via CalcContext)
//   - mixed units: calc(100% - 40px) resolves % first, then subtracts
func EvalCalc(tokens []Token, ctx CalcContext) (float64, error) {
	inner, err := extractCalcInner(tokens)
	if err != nil {
		return 0, err
	}
	if len(inner) == 0 {
		return 0, fmt.Errorf("css/calc: empty calc() expression")
	}
	// Filter out whitespace tokens.
	expr := filterWhitespace(inner)
	if len(expr) == 0 {
		return 0, fmt.Errorf("css/calc: empty expression after removing whitespace")
	}
	p := newCalcParser(expr, ctx)
	result := p.parseExpr()
	if p.err != nil {
		return 0, p.err
	}
	if p.pos < len(p.tokens) {
		return 0, fmt.Errorf("css/calc: unexpected token after expression: %v", p.tokens[p.pos])
	}
	return result, nil
}

// IsCalcValue returns true if the token slice starts with a calc() function
// (TokenFunction with value "calc" or "calc").
func IsCalcValue(value []Token) bool {
	for _, t := range value {
		switch t.Type {
		case TokenNonNewlineWhitespace, TokenNewline, TokenComment:
			continue
		case TokenFunction:
			return t.Value == "calc"
		default:
			return false
		}
	}
	return false
}

// extractCalcInner finds the calc( TokenFunction in tokens, skips the opening '('
// (already consumed by the tokenizer into TokenFunction), and returns the inner
// tokens up to the matching TokenRightParenthesis.
func extractCalcInner(tokens []Token) ([]Token, error) {
	i := 0
	// Skip whitespace and find the calc( function.
	for i < len(tokens) {
		t := tokens[i]
		if isWhitespace(t) {
			i++
			continue
		}
		if t.Type == TokenFunction && t.Value == "calc" {
			i++
			break
		}
		return nil, fmt.Errorf("css/calc: expected calc() function, got %v", t)
	}
	if i >= len(tokens) {
		return nil, fmt.Errorf("css/calc: unexpected end of tokens after calc(")
	}
	// Collect tokens until the matching closing parenthesis.
	depth := 1
	var inner []Token
	for i < len(tokens) {
		t := tokens[i]
		switch {
		case t.Type == TokenLeftParenthesis:
			depth++
			inner = append(inner, t)
		case t.Type == TokenRightParenthesis:
			depth--
			if depth == 0 {
				return inner, nil
			}
			inner = append(inner, t)
		default:
			inner = append(inner, t)
		}
		i++
	}
	return nil, fmt.Errorf("css/calc: unclosed calc()")
}

// isWhitespace reports whether the token is a whitespace or comment token.
func isWhitespace(t Token) bool {
	return t.Type == TokenNonNewlineWhitespace || t.Type == TokenNewline || t.Type == TokenComment
}

// filterWhitespace returns a new slice with whitespace tokens removed.
func filterWhitespace(tokens []Token) []Token {
	var out []Token
	for _, t := range tokens {
		if !isWhitespace(t) {
			out = append(out, t)
		}
	}
	return out
}

// --- Recursive descent expression parser for calc() inner expressions ----------
//
// Grammar (simplified):
//   expr     → term (("+" | "-") term)*
//   term     → factor (("*" | "/") factor)*
//   factor   → unary
//   unary    → ("+" | "-") unary | primary
//   primary  → NUMBER | PERCENTAGE | DIMENSION | "(" expr ")"

type calcParser struct {
	tokens []Token
	pos    int
	ctx    CalcContext
	err    error
}

func newCalcParser(tokens []Token, ctx CalcContext) *calcParser {
	return &calcParser{tokens: tokens, pos: 0, ctx: ctx}
}

// peek returns the current token without consuming it.
func (p *calcParser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

// consume advances and returns the current token.
func (p *calcParser) consume() Token {
	if p.err != nil {
		return Token{Type: TokenEOF}
	}
	if p.pos >= len(p.tokens) {
		p.err = fmt.Errorf("css/calc: unexpected end of expression")
		return Token{Type: TokenEOF}
	}
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// parseExpr handles expr → term (("+" | "-") term)*
func (p *calcParser) parseExpr() float64 {
	if p.err != nil {
		return 0
	}
	left := p.parseTerm()
	for {
		t := p.peek()
		if t.Type == TokenDelimiter && (t.Delimiter == '+' || t.Delimiter == '-') {
			p.consume()
			right := p.parseTerm()
			if t.Delimiter == '+' {
				left += right
			} else {
				left -= right
			}
		} else {
			break
		}
	}
	return left
}

// parseTerm handles term → factor (("*" | "/") factor)*
func (p *calcParser) parseTerm() float64 {
	if p.err != nil {
		return 0
	}
	left := p.parseFactor()
	for {
		t := p.peek()
		if t.Type == TokenDelimiter && (t.Delimiter == '*' || t.Delimiter == '/') {
			p.consume()
			right := p.parseFactor()
			if t.Delimiter == '*' {
				left *= right
			} else {
				if right == 0 {
					p.err = fmt.Errorf("css/calc: division by zero")
					return 0
				}
				left /= right
			}
		} else {
			break
		}
	}
	return left
}

// parseFactor handles factor → unary
func (p *calcParser) parseFactor() float64 {
	return p.parseUnary()
}

// parseUnary handles unary → ("+" | "-") unary | primary
func (p *calcParser) parseUnary() float64 {
	if p.err != nil {
		return 0
	}
	t := p.peek()
	if t.Type == TokenDelimiter && (t.Delimiter == '+' || t.Delimiter == '-') {
		p.consume()
		val := p.parseUnary()
		if t.Delimiter == '-' {
			return -val
		}
		return val
	}
	return p.parsePrimary()
}

// parsePrimary handles primary → NUMBER | PERCENTAGE | DIMENSION | "(" expr ")"
func (p *calcParser) parsePrimary() float64 {
	if p.err != nil {
		return 0
	}
	t := p.consume()
	switch t.Type {
	case TokenNumber:
		return t.Numeric
	case TokenPercentage:
		return p.resolveUnit(t.Numeric, "%")
	case TokenDimension:
		return p.resolveUnit(t.Numeric, t.Unit)
	case TokenLeftParenthesis:
		val := p.parseExpr()
		if p.err != nil {
			return val
		}
		close := p.consume()
		if close.Type != TokenRightParenthesis {
			p.err = fmt.Errorf("css/calc: expected ')' after sub-expression")
			return 0
		}
		return val
	case TokenFunction:
		// Nested calc() — this shouldn't normally happen since the CSS spec
		// treats nested calc() as equivalent to outer calc(), but handle it
		// gracefully by evaluating recursively.
		if t.Value == "calc" {
			// The tokens for the inner calc() include the function token.
			// We need to reconstruct from current position backward.
			// Instead, support by creating a mini-slice from current pos - 1.
			// But actually let's keep it simple: reconstruct tokens from position-1
			p.err = fmt.Errorf("css/calc: nested calc() not supported")
			return 0
		}
		p.err = fmt.Errorf("css/calc: unexpected function %s() in expression", t.Value)
		return 0
	default:
		p.err = fmt.Errorf("css/calc: unexpected token %v in expression", t)
		return 0
	}
}

// resolveUnit converts a numeric value with a CSS unit to pixels using the context.
// Supported units: px, %, em, rem, vw, vh.
func (p *calcParser) resolveUnit(value float64, unit string) float64 {
	switch unit {
	case "px":
		return value
	case "%":
		return value / 100.0 * p.ctx.ParentWidth
	case "em":
		return value * p.ctx.FontSize
	case "rem":
		return value * p.ctx.RootFontSize
	case "vw":
		return value / 100.0 * p.ctx.ViewportWidth
	case "vh":
		return value / 100.0 * p.ctx.ViewportHeight
	case "vmin":
		return value / 100.0 * math.Min(p.ctx.ViewportWidth, p.ctx.ViewportHeight)
	case "vmax":
		return value / 100.0 * math.Max(p.ctx.ViewportWidth, p.ctx.ViewportHeight)
	case "pt":
		return value * 1.3333333333 // 1pt = 1/72in ≈ 1.333px at 96dpi
	case "cm":
		return value * 37.795275591 // 1cm ≈ 37.8px at 96dpi
	case "mm":
		return value * 3.7795275591 // 1mm ≈ 3.78px at 96dpi
	case "in":
		return value * 96.0 // 1in = 96px at 96dpi
	default:
		// Unknown unit: return raw value and let caller handle it.
		return value
	}
}
