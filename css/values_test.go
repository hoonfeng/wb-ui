package css

import "testing"

// TestParseFlexFlow covers the `flex-flow: <flex-direction> || <flex-wrap>`
// grammar: both components are optional and order-independent, an omitted
// component resets to its initial value, and an invalid value must be rejected
// as a whole (never partially applied).
func TestParseFlexFlow(t *testing.T) {
	valid := map[string]FlexFlow{
		"column":                    {Direction: "column", Wrap: "nowrap"},
		"wrap":                      {Direction: "row", Wrap: "wrap"},
		"column wrap":               {Direction: "column", Wrap: "wrap"},
		"wrap column":               {Direction: "column", Wrap: "wrap"},
		"row nowrap":                {Direction: "row", Wrap: "nowrap"},
		"COLUMN":                    {Direction: "column", Wrap: "nowrap"},
		"  wrap   column-reverse  ": {Direction: "column-reverse", Wrap: "wrap"},
	}
	for in, want := range valid {
		got, ok := ParseFlexFlow(in)
		if !ok {
			t.Errorf("ParseFlexFlow(%q): ok=false, want valid %+v", in, want)
			continue
		}
		if got != want {
			t.Errorf("ParseFlexFlow(%q) = %+v, want %+v", in, got, want)
		}
	}

	invalid := []string{
		"row column",          // two directions
		"nowrap wrap-reverse", // two wrap keywords
		"none",                // unknown keyword
		"",                    // empty
		"row wrap nowrap",     // three components
		"column-wrap",         // not a keyword
		"column, wrap",        // comma is not allowed in this shorthand
		"wrap wrp",            // one bad keyword among good ones
	}
	for _, in := range invalid {
		if got, ok := ParseFlexFlow(in); ok {
			t.Errorf("ParseFlexFlow(%q) = %+v, ok=true; want invalid", in, got)
		}
	}
}
