// Translation of: Source/WebCore/css/CSSCalcValue.cpp
//                  Source/WebCore/css/CSSCalcExpressionNode.cpp
// Completeness: 40%
// Simplifications:
//   - only supports +, -, *, / operators between px and % values
//   - no min()/max()/clamp() support
//   - no nested calc() evaluation
//   - no type checking (unit mismatch returns false)
//   - contextValue is the containing-block dimension for % resolution
//   - calc(100% - 40px) with contextWidth=500 returns 500*100/100 - 40 = 460

package style

import (
	"strconv"
	"strings"
)

// EvalCalc evaluates a calc() expression string (without the "calc(" prefix)
// and returns the computed pixel value. contextValue is the containing-block
// dimension used for resolving % units. Returns (0, false) when the expression
// cannot be evaluated.
//
// Supported syntax:
//   - calc(100% - 40px)
//   - calc(50% + 10px)
//   - calc(200px * 2)
//   - calc(100px / 2)
func EvalCalc(expr string, contextValue float64) (float64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	return evalSum(expr, contextValue)
}

// evalSum parses and evaluates a calc sum expression: term ( [+-] term )*
func evalSum(expr string, ctx float64) (float64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}

	// Find the first +/- outside parentheses.
	depth := 0
	opIdx := -1
	op := byte(0)
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			continue
		}
		if depth == 0 && (ch == '+' || ch == '-') && i > 0 {
			opIdx = i
			op = ch
			break
		}
	}

	if opIdx < 0 {
		// No top-level operator: evaluate as product.
		return evalProduct(expr, ctx)
	}

	left := strings.TrimSpace(expr[:opIdx])
	right := strings.TrimSpace(expr[opIdx+1:])

	lv, lok := evalSum(left, ctx)
	rv, rok := evalSum(right, ctx)
	if !lok || !rok {
		return 0, false
	}

	switch op {
	case '+':
		return lv + rv, true
	case '-':
		return lv - rv, true
	}
	return 0, false
}

// evalProduct parses and evaluates a calc product expression: value ( [*/] value )*
func evalProduct(expr string, ctx float64) (float64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}

	depth := 0
	opIdx := -1
	op := byte(0)
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			continue
		}
		if depth == 0 && (ch == '*' || ch == '/') && i > 0 {
			opIdx = i
			op = ch
			break
		}
	}

	if opIdx < 0 {
		// No operator: evaluate as a single value.
		return evalValue(expr, ctx)
	}

	left := strings.TrimSpace(expr[:opIdx])
	right := strings.TrimSpace(expr[opIdx+1:])

	lv, lok := evalValue(left, ctx)
	rv, rok := evalValue(right, ctx)
	if !lok || !rok {
		return 0, false
	}

	switch op {
	case '*':
		return lv * rv, true
	case '/':
		if rv == 0 {
			return 0, false
		}
		return lv / rv, true
	}
	return 0, false
}

// evalValue parses a single calc value: a number with unit or a parenthesized expression.
func evalValue(expr string, ctx float64) (float64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}

	// Parenthesized sub-expression.
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evalSum(expr[1:len(expr)-1], ctx)
	}

	// Parse number + unit.
	return resolveCalcLength(expr, ctx)
}

// resolveCalcLength parses a CSS length with unit for calc() evaluation.
// Supported units: px, %, em (uses defaultFontSize as reference).
func resolveCalcLength(s string, ctx float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	// Find the boundary between number and unit.
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, false
	}

	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}

	unit := strings.TrimSpace(s[i:])
	switch unit {
	case "", "px":
		return num, true
	case "%":
		if ctx <= 0 {
			return 0, false
		}
		return num * ctx / 100, true
	case "em":
		// Use a fixed 16px fallback when font-size context is unknown.
		return num * 16, true
	}
	// Unknown unit.
	return 0, false
}

// isCalcValue reports whether a CSS value string is or contains a calc() expression.
func isCalcValue(s string) bool {
	return strings.Contains(strings.ToLower(s), "calc(")
}

// extractCalcArg extracts the argument inside calc(...) from a value string.
// For "calc(100% - 40px)" it returns "100% - 40px".
func extractCalcArg(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	start := strings.Index(lower, "calc(")
	if start < 0 {
		return s
	}
	// Move past "calc("
	start += 5
	// Find matching closing paren.
	depth := 1
	i := start
	for i < len(s) && depth > 0 {
		ch := s[i]
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[start:i])
			}
		}
		i++
	}
	return strings.TrimSpace(s[start:])
}
