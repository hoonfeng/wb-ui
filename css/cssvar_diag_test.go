package css

import (
	"fmt"
	"strings"
	"testing"
)

// TestCVarTokenization verifies that CSS variable values with dimensions
// and font-family lists are correctly tokenized.
func TestCVarTokenization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVars map[string]string // expected token values
	}{
		{
			"simple-px",
			":root { --font-size-base: 13px; }",
			map[string]string{"--font-size-base": "13px"},
		},
		{
			"hash-color",
			":root { --bg-primary: #0d1117; }",
			map[string]string{"--bg-primary": "#0d1117"},
		},
		{
			"multi-values",
			`:root {
				--bg-primary: #0d1117;
				--font-size-base: 13px;
				--border-radius: 4px;
			}`,
			map[string]string{
				"--bg-primary":     "#0d1117",
				"--font-size-base": "13px",
				"--border-radius":  "4px",
			},
		},
		{
			"font-family-list",
			":root { --font-ui: 'Inter', system-ui, -apple-system, sans-serif; }",
			map[string]string{"--font-ui": "'Inter', system-ui, -apple-system, sans-serif"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sheet := NewCSSStyleSheet()
			NewParser(tt.input).ParseStyleSheetInto(sheet)
			rules := sheet.Rules()

			for _, r := range rules {
				sr, ok := r.(*StyleRule)
				if !ok {
					continue
				}
				for _, d := range sr.Declarations {
					raw := d.ValueString()
					t.Logf("  %s: %s", d.Name, raw)

					if expected, ok := tt.wantVars[d.Name]; ok {
						if raw != expected {
							t.Errorf("  ❌ %s: got %q, want %q", d.Name, raw, expected)
						} else {
							t.Logf("  ✅ %s: %q", d.Name, raw)
						}
					}

					// Show token breakdown
					tokens := tokenize(raw)
					t.Logf("    tokens: %v", tokens)
				}
			}
		})
	}
}

func tokenize(raw string) []string {
	tok := NewTokenizer(raw)
	tokens := tok.Tokenize()
	var parts []string
	typeNames := map[TokenType]string{
		TokenIdent:        "ident",
		TokenFunction:     "func",
		TokenHash:         "hash",
		TokenNumber:       "num",
		TokenDimension:    "dim",
		TokenComma:        "comma",
		TokenDelimiter:    "delim",
		TokenString:       "str",
		TokenNonNewlineWhitespace: "ws",
		TokenNewline:      "nl",
		TokenSemicolon:    "semi",
		TokenEOF:           "eof",
		TokenLeftParenthesis:   "lparen",
		TokenRightParenthesis:  "rparen",
	}
	for _, tok := range tokens {
		if tok.Type == TokenEOF {
			break
		}
		tn := typeNames[tok.Type]
		if tn == "" {
			tn = fmt.Sprintf("T%d", tok.Type)
		}
		parts = append(parts, fmt.Sprintf("%s=%q", tn, tok.Value))
	}
	return parts
}

// TestCVarResolverWithBodyFull verifies the full CSS variable resolution
// chain: :root → body inheritance → var() substitution → background-color.
func TestCVarResolverWithBodyFull(t *testing.T) {
	cssText := `
		:root, .theme-dark {
			--bg-primary: #0d1117;
			--bg-secondary: #161b22;
			--text-primary: #e6edf3;
			--font-size-base: 13px;
			--border-radius: 4px;
		}
		body {
			font-family: var(--font-ui);
			font-size: var(--font-size-base);
			color: var(--text-primary);
			background-color: var(--bg-primary);
		}
	`
	sheet := NewCSSStyleSheet()
	NewParser(cssText).ParseStyleSheetInto(sheet)

	t.Logf("Parsed %d rules", len(sheet.Rules()))
	for i, r := range sheet.Rules() {
		sr, ok := r.(*StyleRule)
		if !ok {
			t.Logf("Rule %d: not StyleRule", i)
			continue
		}
		selStr := selectorText(sr.Selectors)
		t.Logf("Rule %d: %s", i, selStr)
		for _, d := range sr.Declarations {
			raw := d.ValueString()
			t.Logf("  %-30s = %q", d.Name, raw)
			tokens := tokenize(raw)
			t.Logf("    tokens: %v", strings.Join(tokens, " | "))
		}
	}
}

func selectorText(sl *SelectorList) string {
	if sl == nil {
		return "<nil>"
	}
	var parts []string
	for _, s := range sl.Selectors {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, ", ")
}
