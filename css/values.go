// CSS shorthand value grammars that are shared between the style resolver and
// the CSS.supports() feature-detection API, so that "the engine accepts this
// syntax" and "supports() says the syntax is valid" can never drift apart.

package css

import "strings"

// FlexFlow is the parsed result of a `flex-flow` shorthand value.
type FlexFlow struct {
	Direction string // row | row-reverse | column | column-reverse
	Wrap      string // nowrap | wrap | wrap-reverse
}

// ParseFlexFlow parses the `flex-flow: <flex-direction> || <flex-wrap>`
// shorthand (CSS Flexible Box §5.2). Both components are optional and may come
// in either order; an omitted component resets to its initial value (row /
// nowrap). ok=false means the whole declaration is invalid and must be dropped
// — a value mixing two directions or two wrap keywords ("row column",
// "nowrap wrap-reverse") or using an unknown keyword ("none") must NOT be
// partially applied.
func ParseFlexFlow(value string) (FlexFlow, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 || len(fields) > 2 {
		return FlexFlow{}, false
	}
	var out FlexFlow
	for _, f := range fields {
		switch strings.ToLower(f) {
		case "row", "row-reverse", "column", "column-reverse":
			if out.Direction != "" {
				return FlexFlow{}, false
			}
			out.Direction = strings.ToLower(f)
		case "nowrap", "wrap", "wrap-reverse":
			if out.Wrap != "" {
				return FlexFlow{}, false
			}
			out.Wrap = strings.ToLower(f)
		default:
			return FlexFlow{}, false
		}
	}
	if out.Direction == "" {
		out.Direction = "row"
	}
	if out.Wrap == "" {
		out.Wrap = "nowrap"
	}
	return out, true
}
