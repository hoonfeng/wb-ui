// Translation of: Source/WebCore/style/StyleResolver.cpp
//                  Source/WebCore/style/StyleResolver.h
//                  Source/WebCore/style/PropertyCascade.cpp
//                  Source/WebCore/style/PropertyCascade.h
//                  Source/WebCore/style/ElementRuleCollector.cpp
// Completeness: 65%
// Simplifications:
//   - the resolver runs a single pass over the DOM: no invalidation, no element style
//     recalc scheduling, no ComputedStyle cache beyond per-element memoization on the
//     Resolver itself
//   - the cascade is computed by iterating all rules in source order, collecting
//     those whose selectors match the element, and then sorting the collected
//     declarations by (origin, importance, specificity, source order) — a faithful
//     but simplified version of WebKit's RuleSet + PropertyCascade machinery
//   - inline style attribute is honored as an author-origin declaration with high
//     source order but otherwise normal specificity
//   - var() is resolved by recursive substitution against the custom property map;
//     no @property registered syntax validation
//   - calc() is evaluated for simple expressions (px-only arithmetic);
//     expressions containing unresolvable % units are deferred to layout time
//   - inheritance is performed via ComputedStyle.InheritFrom against the parent
//     ComputedStyle
//   - default values come from NewComputedStyle, not from a UA stylesheet

package style

import (
	"sort"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
)

// Resolver is the Go translation of WebCore::Style::Resolver. It holds the set of
// author stylesheets (plus UA / user sheets if added) and exposes ResolveElement to
// compute a ComputedStyle for a given Element.
type Resolver struct {
	sheets  []*css.CSSStyleSheet
	checker *css.SelectorChecker
	// cache memoizes per-element ComputedStyle to make inheritance cheap. The cache
	// is keyed by element identity (pointer).
	cache map[*dom.Element]*ComputedStyle
	// keyframes stores @keyframes rules by name, for animation resolution.
	keyframes map[string]*css.KeyframesRule
}

// NewResolver constructs an empty Resolver.
func NewResolver() *Resolver {
	return &Resolver{
		checker:   css.NewSelectorChecker(),
		cache:     map[*dom.Element]*ComputedStyle{},
		keyframes: map[string]*css.KeyframesRule{},
	}
}

// AddStyleSheet adds a parsed stylesheet and collects any @keyframes rules
// from it for animation lookup.
func (r *Resolver) addKeyframesFromSheet(sheet *css.CSSStyleSheet) {
	for _, rule := range sheet.Rules() {
		if kf, ok := rule.(*css.KeyframesRule); ok {
			r.keyframes[kf.Name] = kf
		}
	}
}

// LookupKeyframes returns the @keyframes rule with the given name, or nil.
func (r *Resolver) LookupKeyframes(name string) *css.KeyframesRule {
	return r.keyframes[name]
}

// AddStyleSheet adds a parsed stylesheet to the resolver's cascade. Sheets added
// earlier have lower source order than sheets added later (within the same origin).
func (r *Resolver) AddStyleSheet(sheet *css.CSSStyleSheet) {
	r.sheets = append(r.sheets, sheet)
	r.addKeyframesFromSheet(sheet)
}

// RemoveStyleSheet removes a previously added stylesheet from the resolver.
// After removal the cache is cleared so the next ResolveElement call
// recomputes styles without the removed sheet's rules.
func (r *Resolver) RemoveStyleSheet(sheet *css.CSSStyleSheet) {
	for i, s := range r.sheets {
		if s == sheet {
			r.sheets = append(r.sheets[:i], r.sheets[i+1:]...)
			r.ClearCache()
			return
		}
	}
}

// ClearCache drops the per-element ComputedStyle cache. Call this after mutating
// the stylesheets or DOM so subsequent calls recompute fresh values.
func (r *Resolver) ClearCache() {
	r.cache = map[*dom.Element]*ComputedStyle{}
}

// collectedDecl is an intermediate structure used during cascade sorting.
type collectedDecl struct {
	decl       css.Declaration
	origin     css.Origin
	important  bool
	specificity css.Specificity
	sourceOrder int
}

// ResolveElement computes the ComputedStyle for the given element by walking the
// cascade, applying matched declarations in order, and resolving inheritance /
// custom properties. The result is cached.
func (r *Resolver) ResolveElement(el *dom.Element) *ComputedStyle {
	if cs, ok := r.cache[el]; ok {
		return cs
	}
	cs := NewComputedStyle()
	// Inherit from the parent first so that non-matched properties keep their
	// inherited values.
	if parent := parentElement(el); parent != nil {
		parentCS := r.ResolveElement(parent)
		cs.InheritFrom(parentCS)
	}

	// Collect declarations in cascade order.
	var collected []collectedDecl
	for _, sheet := range r.sheets {
		r.collectDeclarations(sheet.Rules(), sheet.Origin(), el, &collected, 0)
	}

	// Inline style attribute (high specificity, last source order in author origin).
	if style := el.GetAttribute("style"); style != "" {
		p := css.NewParserWithOrigin(style, css.OriginAuthor)
		decls := p.ParseDeclarationList()
		for _, d := range decls {
			collected = append(collected, collectedDecl{
				decl:        d,
				origin:      css.OriginAuthor,
				important:   d.Important,
				specificity: css.Specificity{A: 1, B: 0, C: 0}, // inline style specificity
				sourceOrder: 1 << 30,
			})
		}
	}

	// Sort by (origin, importance, specificity, sourceOrder). Higher origin first
	// (user > author > UA), then important > non-important, then specificity, then
	// source order.
	sort.SliceStable(collected, func(i, j int) bool {
		a, b := collected[i], collected[j]
		ai := importanceRank(a.origin, a.important)
		bj := importanceRank(b.origin, b.important)
		if ai != bj {
			return ai < bj
		}
		if c := a.specificity.Compare(b.specificity); c != 0 {
			return c < 0
		}
		return a.sourceOrder < b.sourceOrder
	})

	// Apply declarations in sorted order; later ones overwrite earlier ones.
	for _, cd := range collected {
		applyDeclaration(cs, cd.decl)
	}

	// Resolve custom properties (var()) now that the cascade is complete.
	r.resolveCustomProperties(cs)

	r.cache[el] = cs
	return cs
}

// collectDeclarations walks a rule list, recursing into @media / @supports rules,
// and appends matching declarations to collected with their cascade metadata.
func (r *Resolver) collectDeclarations(rules []css.Rule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, baseOrder int) int {
	order := baseOrder
	for _, rule := range rules {
		switch v := rule.(type) {
		case *css.StyleRule:
			if v.Selectors != nil {
				for _, sel := range v.Selectors.Selectors {
					if r.checker.Match(sel, el) {
						spec := css.SpecificityOfComplex(sel)
						for _, d := range v.Declarations {
							*collected = append(*collected, collectedDecl{
								decl:        d,
								origin:      origin,
								important:   d.Important,
								specificity: spec,
								sourceOrder: order,
							})
							order++
						}
					}
				}
			}
			// Nested rules (CSS Nesting) are matched relative to the matched
			// element, mirroring WebKit's style resolution for nested rules. For
			// simplicity we also test them against the same element; the cascade
			// order continues from the outer rule.
			if len(v.NestedRules) > 0 {
				order = r.collectDeclarations(v.NestedRules, origin, el, collected, order)
			}
		case *css.MediaRule:
			// In this simplified port we always evaluate media queries as true
			// (no viewport / device info available). Real-world cascades would
			// evaluate the media query against the document's viewport.
			order = r.collectDeclarations(v.Rules, origin, el, collected, order)
		case *css.SupportsRule:
			// Supports is treated as always-true in this port.
			order = r.collectDeclarations(v.Rules, origin, el, collected, order)
		case *css.FontFaceRule:
			// Font face rules do not contribute declarations to elements; they are
			// registered separately by the font selector.
			continue
		case *css.KeyframesRule:
			// Keyframes do not contribute declarations to the cascade.
			continue
		case *css.PageRule:
			// Page rules only apply to the page box; skipped here.
			continue
		case *css.ImportRule:
			// Imports are followed during sheet loading; the imported sheet's rules
			// have already been merged in.
			continue
		case *css.NamespaceRule:
			continue
		}
	}
	return order
}

// importanceRank returns the cascade ordering key for (origin, !important). The
// standard CSS cascade order from lowest to highest is:
//   1. UA normal
//   2. User normal
//   3. Author normal
//   4. Author important
//   5. User important
//   6. UA important
func importanceRank(origin css.Origin, important bool) int {
	switch origin {
	case css.OriginUserAgent:
		if important {
			return 6
		}
		return 1
	case css.OriginUser:
		if important {
			return 5
		}
		return 2
	case css.OriginAuthor:
		if important {
			return 4
		}
		return 3
	}
	return 0
}

// applyDeclaration applies a single CSS declaration to a ComputedStyle. Properties
// that the resolver knows how to interpret are set on typed fields; the rest are
// stored in the Properties map for later retrieval.
func applyDeclaration(cs *ComputedStyle, d css.Declaration) {
	name := strings.ToLower(d.Name)
	valueString := d.ValueString()
	if strings.HasPrefix(name, "--") {
		// CSS custom property; store as raw tokens.
		cs.SetCustomProperty(name, d.Value)
		if d.Important {
			cs.ImportantProperties[name] = true
		}
		return
	}
	switch name {
	case "display":
		cs.Display = LookupDisplayType(valueString)
		cs.DisplaySet = true
	case "position":
		cs.Position = LookupPositionType(valueString)
	case "color":
		if c, ok := parseColor(valueString); ok {
			cs.Color = c
		}
	case "background-color":
		if c, ok := parseColor(valueString); ok {
			cs.BackgroundColor = c
		}
	case "font-family":
		cs.FontFamily = valueString
	case "font-size":
		if l, ok := parseLength(valueString); ok {
			cs.FontSize = l
		}
	case "font-weight":
		cs.FontWeight = valueString
	case "font-style":
		cs.FontStyle = valueString
	case "line-height":
		if l, ok := parseLength(valueString); ok {
			cs.LineHeight = l
		}
	case "letter-spacing":
		if l, ok := parseLength(valueString); ok {
			cs.LetterSpacing = l
		}
	case "word-spacing":
		if l, ok := parseLength(valueString); ok {
			cs.WordSpacing = l
		}
	case "text-indent":
		if l, ok := parseLength(valueString); ok {
			cs.TextIndent = l
		}
	case "text-align":
		cs.TextAlign = parseTextAlign(valueString)
	case "text-decoration":
		cs.TextDecoration = valueString
	case "text-transform":
		cs.TextTransform = valueString
	case "white-space":
		cs.WhiteSpace = LookupWhiteSpace(valueString)
	case "direction":
		cs.Direction = valueString
	case "unicode-bidi":
		cs.UnicodeBidi = valueString
	case "width":
		if l, ok := parseLength(valueString); ok {
			cs.Width = l
		}
	case "height":
		if l, ok := parseLength(valueString); ok {
			cs.Height = l
		}
	case "min-width":
		if l, ok := parseLength(valueString); ok {
			cs.MinWidth = l
		}
	case "min-height":
		if l, ok := parseLength(valueString); ok {
			cs.MinHeight = l
		}
	case "max-width":
		if l, ok := parseLength(valueString); ok {
			cs.MaxWidth = l
		}
	case "max-height":
		if l, ok := parseLength(valueString); ok {
			cs.MaxHeight = l
		}
	case "margin":
		top, right, bottom, left := parseEdgeShorthand(valueString)
		cs.MarginTop = top
		cs.MarginRight = right
		cs.MarginBottom = bottom
		cs.MarginLeft = left
	case "margin-top":
		if l, ok := parseLength(valueString); ok {
			cs.MarginTop = l
		}
	case "margin-right":
		if l, ok := parseLength(valueString); ok {
			cs.MarginRight = l
		}
	case "margin-bottom":
		if l, ok := parseLength(valueString); ok {
			cs.MarginBottom = l
		}
	case "margin-left":
		if l, ok := parseLength(valueString); ok {
			cs.MarginLeft = l
		}
	case "padding":
		top, right, bottom, left := parseEdgeShorthand(valueString)
		cs.PaddingTop = top
		cs.PaddingRight = right
		cs.PaddingBottom = bottom
		cs.PaddingLeft = left
	case "padding-top":
		if l, ok := parseLength(valueString); ok {
			cs.PaddingTop = l
		}
	case "padding-right":
		if l, ok := parseLength(valueString); ok {
			cs.PaddingRight = l
		}
	case "padding-bottom":
		if l, ok := parseLength(valueString); ok {
			cs.PaddingBottom = l
		}
	case "padding-left":
		if l, ok := parseLength(valueString); ok {
			cs.PaddingLeft = l
		}
	case "border":
		w, s, c, ok := parseBorderShorthand(valueString)
		if ok {
			cs.BorderTopWidth = w
			cs.BorderRightWidth = w
			cs.BorderBottomWidth = w
			cs.BorderLeftWidth = w
			cs.BorderTopStyle = s
			cs.BorderRightStyle = s
			cs.BorderBottomStyle = s
			cs.BorderLeftStyle = s
			cs.BorderTopColor = c
			cs.BorderRightColor = c
			cs.BorderBottomColor = c
			cs.BorderLeftColor = c
		}
	case "border-top":
		if w, s, c, ok := parseBorderShorthand(valueString); ok {
			cs.BorderTopWidth = w
			cs.BorderTopStyle = s
			cs.BorderTopColor = c
		}
	case "border-right":
		if w, s, c, ok := parseBorderShorthand(valueString); ok {
			cs.BorderRightWidth = w
			cs.BorderRightStyle = s
			cs.BorderRightColor = c
		}
	case "border-bottom":
		if w, s, c, ok := parseBorderShorthand(valueString); ok {
			cs.BorderBottomWidth = w
			cs.BorderBottomStyle = s
			cs.BorderBottomColor = c
		}
	case "border-left":
		if w, s, c, ok := parseBorderShorthand(valueString); ok {
			cs.BorderLeftWidth = w
			cs.BorderLeftStyle = s
			cs.BorderLeftColor = c
		}
	case "border-top-width":
		if l, ok := parseLength(valueString); ok {
			cs.BorderTopWidth = l
		}
	case "border-right-width":
		if l, ok := parseLength(valueString); ok {
			cs.BorderRightWidth = l
		}
	case "border-bottom-width":
		if l, ok := parseLength(valueString); ok {
			cs.BorderBottomWidth = l
		}
	case "border-left-width":
		if l, ok := parseLength(valueString); ok {
			cs.BorderLeftWidth = l
		}
	case "border-top-color":
		if c, ok := parseColor(valueString); ok {
			cs.BorderTopColor = c
		}
	case "border-right-color":
		if c, ok := parseColor(valueString); ok {
			cs.BorderRightColor = c
		}
	case "border-bottom-color":
		if c, ok := parseColor(valueString); ok {
			cs.BorderBottomColor = c
		}
	case "border-left-color":
		if c, ok := parseColor(valueString); ok {
			cs.BorderLeftColor = c
		}
	case "border-top-style":
		cs.BorderTopStyle = valueString
	case "border-right-style":
		cs.BorderRightStyle = valueString
	case "border-bottom-style":
		cs.BorderBottomStyle = valueString
	case "border-left-style":
		cs.BorderLeftStyle = valueString
	case "border-radius":
		if l, ok := parseLength(valueString); ok {
			cs.BorderRadius = l
		}
	case "box-sizing":
		cs.BoxSizing = valueString
	case "visibility":
		cs.Visibility = valueString
	case "opacity":
		if v, err := strconv.ParseFloat(valueString, 64); err == nil {
			cs.Opacity = v
		}
	case "z-index":
		if v, err := strconv.Atoi(valueString); err == nil {
			cs.ZIndex = v
		}
	case "flex":
		// CSS flex shorthand: <grow> <shrink> <basis>. Single number => grow,
		// basis 0%; single length => grow 1, basis length; none/auto/initial keywords.
		grow, shrink, basis := parseFlexShorthand(valueString)
		cs.FlexGrow = grow
		cs.FlexShrink = shrink
		cs.FlexBasis = basis
	case "flex-direction":
		cs.FlexDirection = valueString
	case "flex-wrap":
		cs.FlexWrap = valueString
	case "justify-content":
		cs.JustifyContent = valueString
	case "align-items":
		cs.AlignItems = valueString
	case "align-content":
		cs.AlignContent = valueString
	case "align-self":
		cs.AlignSelf = valueString
	case "justify-self":
		cs.JustifySelf = valueString
	case "flex-basis":
		if l, ok := parseLength(valueString); ok {
			cs.FlexBasis = l
		}
	case "flex-grow":
		if v, err := strconv.ParseFloat(valueString, 64); err == nil {
			cs.FlexGrow = v
		}
	case "flex-shrink":
		if v, err := strconv.ParseFloat(valueString, 64); err == nil {
			cs.FlexShrink = v
		}
	case "order":
		if v, err := strconv.Atoi(valueString); err == nil {
			cs.Order = v
		}
	case "gap":
		if l, ok := parseLength(valueString); ok {
			cs.Gap = l
		}
	case "row-gap":
		if l, ok := parseLength(valueString); ok {
			cs.RowGap = l
		}
	case "column-gap":
		if l, ok := parseLength(valueString); ok {
			cs.ColumnGap = l
		}
	case "grid-template-columns":
		cs.GridTemplateColumns = valueString
	case "grid-template-rows":
		cs.GridTemplateRows = valueString
	case "grid-template-areas":
		cs.GridTemplateAreas = valueString
	case "grid-auto-flow":
		cs.GridAutoFlow = valueString
	case "grid-auto-columns":
		cs.GridAutoColumns = valueString
	case "grid-auto-rows":
		cs.GridAutoRows = valueString
	case "grid-row-start":
		cs.GridRowStart = valueString
	case "grid-row-end":
		cs.GridRowEnd = valueString
	case "grid-column-start":
		cs.GridColumnStart = valueString
	case "grid-column-end":
		cs.GridColumnEnd = valueString
	case "list-style-type":
		cs.ListStyleType = valueString
	case "list-style-position":
		cs.ListStylePosition = valueString
	case "list-style-image":
		cs.ListStyleImage = valueString
	case "vertical-align":
		cs.VerticalAlign = valueString
	case "content":
		cs.Content = valueString
	case "cursor":
		cs.Cursor = valueString
	case "user-select":
		cs.UserSelect = valueString
	case "pointer-events":
		cs.PointerEvents = valueString
	case "box-shadow":
		cs.BoxShadow = valueString
	case "text-shadow":
		cs.TextShadow = valueString
	case "transform":
		cs.Transform = valueString
	case "transition":
		cs.Transition = valueString
	case "animation":
		cs.Animation = valueString
		cs.AnimationName, cs.AnimationDuration, cs.AnimationIterationCount = parseAnimationShorthand(valueString)
	case "filter":
		cs.Filter = valueString
	case "backdrop-filter":
		cs.BackdropFilter = valueString
	case "background-image":
		cs.BackgroundImage = valueString
	case "background-repeat":
		cs.BackgroundRepeat = valueString
	case "background-position":
		cs.BackgroundPosition = valueString
	case "background-size":
		cs.BackgroundSize = valueString
	case "background-attachment":
		cs.BackgroundAttachment = valueString
	case "background-clip":
		cs.BackgroundClip = valueString
	case "background-origin":
		cs.BackgroundOrigin = valueString
	case "float":
		cs.Float = valueString
	case "clear":
		cs.Clear = valueString
	case "overflow-x":
		cs.OverflowX = LookupOverflow(valueString)
	case "overflow-y":
		cs.OverflowY = LookupOverflow(valueString)
	case "overflow":
		cs.OverflowX = LookupOverflow(valueString)
		cs.OverflowY = LookupOverflow(valueString)
	default:
		// Unknown / shorthand property: store as raw string.
		cs.SetProperty(name, valueString)
	}
	if d.Important {
		cs.ImportantProperties[name] = true
	}
}

// parseTextAlign maps a CSS keyword to TextAlignType.
func parseTextAlign(s string) TextAlignType {
	switch strings.ToLower(s) {
	case "left":
		return TextAlignLeft
	case "right":
		return TextAlignRight
	case "center":
		return TextAlignCenter
	case "justify":
		return TextAlignJustify
	case "end":
		return TextAlignEnd
	}
	return TextAlignStart
}

// parseLength parses a CSS length value like "12px" or "1.5em" or "auto". The
// returned Length has Value and Unit set; "auto" returns a length with Unit="auto".
// Non-length strings (e.g. "solid", "#e5e7eb", "red") return ok=false so callers
// like parseBorderShorthand can fall back to interpreting them as a style keyword
// or color.
//
// When the value is a calc() expression, parseLength attempts to evaluate it via
// EvalCalc. If the expression can be fully resolved without context (e.g. "calc(40px + 8px)"),
// it returns a px Length with the computed value. If it contains % and no context is
// available, it returns ok=false so the caller can try an alternate interpretation.
func parseLength(s string) (Length, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Length{}, false
	}

	// Handle calc() expressions.
	if isCalcValue(s) {
		arg := extractCalcArg(s)
		v, ok := EvalCalc(arg, 0) // context=0: % cannot be resolved here
		if ok {
			return Length{Value: v, Unit: "px"}, true
		}
		// If the calc contains % (unresolvable without context), still return ok=true
		// with a marker unit so the layout engine can re-evaluate with context later.
		if strings.Contains(arg, "%") {
			return Length{Value: 0, Unit: "calc"}, true
		}
		return Length{}, false
	}

	if s == "auto" || s == "none" || s == "inherit" || s == "initial" {
		return Length{Unit: s}, true
	}
	// Find the boundary between number and unit.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		// No leading number: not a length (e.g. "solid", "#e5e7eb", "red").
		return Length{}, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return Length{}, false
	}
	return Length{Value: num, Unit: s[i:]}, true
}

// parseColor parses a CSS color value. Supports #rgb / #rrggbb / #rrggbbaa / rgb() /
// rgba() / hsl() / hsla() / named colors.
func parseColor(s string) (Color, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Color{}, false
	}
	if s == "transparent" {
		return Color{A: 0}, true
	}
	if s == "inherit" || s == "currentcolor" || s == "initial" {
		// The caller should handle inheritance; we treat these as "no value".
		return Color{}, false
	}
	if s[0] == '#' {
		return parseHexColor(s)
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		return parseRGB(s[4:len(s)-1], false)
	}
	if strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")") {
		return parseRGB(s[5:len(s)-1], true)
	}
	if strings.HasPrefix(s, "hsl(") && strings.HasSuffix(s, ")") {
		return parseHSL(s[4:len(s)-1], false)
	}
	if strings.HasPrefix(s, "hsla(") && strings.HasSuffix(s, ")") {
		return parseHSL(s[5:len(s)-1], true)
	}
	if c, ok := namedColor(s); ok {
		return c, true
	}
	return Color{}, false
}

// parseHexColor parses #rgb / #rgba / #rrggbb / #rrggbbaa.
func parseHexColor(s string) (Color, bool) {
	if len(s) == 4 {
		// #rgb
		r := hexDigit(s[1])
		g := hexDigit(s[2])
		b := hexDigit(s[3])
		return Color{R: r * 17, G: g * 17, B: b * 17, A: 0xFF}, true
	}
	if len(s) == 5 {
		// #rgba
		r := hexDigit(s[1])
		g := hexDigit(s[2])
		b := hexDigit(s[3])
		a := hexDigit(s[4])
		return Color{R: r * 17, G: g * 17, B: b * 17, A: a * 17}, true
	}
	if len(s) == 7 {
		// #rrggbb
		r := hexDigit(s[1])*16 + hexDigit(s[2])
		g := hexDigit(s[3])*16 + hexDigit(s[4])
		b := hexDigit(s[5])*16 + hexDigit(s[6])
		return Color{R: r, G: g, B: b, A: 0xFF}, true
	}
	if len(s) == 9 {
		// #rrggbbaa
		r := hexDigit(s[1])*16 + hexDigit(s[2])
		g := hexDigit(s[3])*16 + hexDigit(s[4])
		b := hexDigit(s[5])*16 + hexDigit(s[6])
		a := hexDigit(s[7])*16 + hexDigit(s[8])
		return Color{R: r, G: g, B: b, A: a}, true
	}
	return Color{}, false
}

// hexDigit converts a hex character to its numeric value.
func hexDigit(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// parseFlexShorthand expands the CSS `flex` shorthand into (grow, shrink, basis),
// mirroring the CSS Flexbox spec §7. The flex shorthand accepts:
//   - none            => 0 0 auto
//   - auto            => 1 1 auto
//   - initial         => 0 1 auto  (the spec initial value)
//   - <number>        => <number> 1 0%     (e.g. "flex: 1" => grow=1, shrink=1, basis=0%)
//   - <length>|%      => 1 1 <length>      (e.g. "flex: 100px")
//   - <n> <n>         => <n> <n> 0%
//   - <n> <n> <basis> => <n> <n> <basis>
// Numeric values map to grow then shrink; the first non-numeric, non-keyword token is
// the basis. The default basis for the numeric form is 0% (per spec).
func parseFlexShorthand(s string) (grow, shrink float64, basis Length) {
	parts := strings.Fields(s)
	// Initial value: 0 1 auto.
	grow, shrink, basis = 0, 1, Length{Unit: "auto"}
	if len(parts) == 0 {
		return
	}
	switch strings.ToLower(parts[0]) {
	case "none":
		return 0, 0, Length{Unit: "auto"}
	case "auto":
		return 1, 1, Length{Unit: "auto"}
	case "initial":
		return 0, 1, Length{Unit: "auto"}
	}
	// Numeric / length form: default basis is 0%.
	basis = Length{Value: 0, Unit: "%"}
	numCount := 0
	for _, p := range parts {
		if n, err := strconv.ParseFloat(p, 64); err == nil {
			if numCount == 0 {
				grow = n
			} else if numCount == 1 {
				shrink = n
			}
			numCount++
			continue
		}
		if l, ok := parseLength(p); ok {
			basis = l
			continue
		}
		if strings.EqualFold(p, "auto") {
			basis = Length{Unit: "auto"}
		}
	}
	return grow, shrink, basis
}

// parseEdgeShorthand parses a 1-to-4 value edge shorthand like "padding: 10px 20px"
// or "margin: 1 2 3 4" and returns (top, right, bottom, left). Mirrors the CSS
// "Edge value shorthand" expansion rules.
func parseEdgeShorthand(s string) (top, right, bottom, left Length) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return
	}
	switch len(parts) {
	case 1:
		if l, ok := parseLength(parts[0]); ok {
			top, right, bottom, left = l, l, l, l
		}
	case 2:
		v, ok := parseLength(parts[0])
		h, ok2 := parseLength(parts[1])
		if ok && ok2 {
			top, bottom = v, v
			right, left = h, h
		}
	case 3:
		t, ok1 := parseLength(parts[0])
		h, ok2 := parseLength(parts[1])
		b, ok3 := parseLength(parts[2])
		if ok1 && ok2 && ok3 {
			top, right, bottom, left = t, h, b, h
		}
	default:
		t, ok1 := parseLength(parts[0])
		r, ok2 := parseLength(parts[1])
		b, ok3 := parseLength(parts[2])
		l, ok4 := parseLength(parts[3])
		if ok1 && ok2 && ok3 && ok4 {
			top, right, bottom, left = t, r, b, l
		}
	}
	return
}

// parseBorderShorthand parses a border shorthand value like "1px solid #e5e7eb" and
// returns (width, style, color, ok). Components may appear in any order; missing
// components are left as their zero value.
func parseBorderShorthand(s string) (width Length, style string, color Color, ok bool) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return
	}
	ok = true
	for _, p := range parts {
		// Try length first (e.g. "1px", "2em").
		if l, lOK := parseLength(p); lOK {
			width = l
			continue
		}
		// Try color (e.g. "#e5e7eb", "red", "rgb(...)").
		if c, cOK := parseColor(p); cOK {
			color = c
			continue
		}
		// Otherwise treat as style keyword (solid, dashed, dotted, none, double, ...).
		switch strings.ToLower(p) {
		case "none", "hidden", "dotted", "dashed", "solid", "double",
			"groove", "ridge", "inset", "outset":
			style = strings.ToLower(p)
		}
	}
	return
}

// parseRGB parses the comma- or space-separated arguments of rgb()/rgba().
func parseRGB(args string, _ bool) (Color, bool) {
	parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) < 3 {
		return Color{}, false
	}
	r := parseComponent(parts[0])
	g := parseComponent(parts[1])
	b := parseComponent(parts[2])
	a := uint8(0xFF)
	if len(parts) >= 4 {
		a = parseAlpha(parts[3])
	}
	return Color{R: r, G: g, B: b, A: a}, true
}

// parseComponent parses a single RGB component (0-255 or 0%-100%).
func parseComponent(s string) uint8 {
	s = strings.TrimSuffix(s, "%")
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return uint8(v * 255 / 100)
	}
	v, _ := strconv.ParseFloat(s, 64)
	if v > 255 {
		v = 255
	}
	if v < 0 {
		v = 0
	}
	return uint8(v)
}

// parseAlpha parses an alpha component (0.0-1.0).
func parseAlpha(s string) uint8 {
	s = strings.TrimSuffix(s, "%")
	v, _ := strconv.ParseFloat(s, 64)
	if strings.HasSuffix(s, "%") {
		v = v * 255 / 100
	}
	if v > 255 {
		v = 255
	}
	if v < 0 {
		v = 0
	}
	return uint8(v)
}

// parseHSL parses hsl()/hsla() arguments. The hue is in degrees (0-360), saturation
// and lightness are percentages.
func parseHSL(args string, _ bool) (Color, bool) {
	parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) < 3 {
		return Color{}, false
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	s, _ := strconv.ParseFloat(strings.TrimSuffix(parts[1], "%"), 64)
	l, _ := strconv.ParseFloat(strings.TrimSuffix(parts[2], "%"), 64)
	a := 1.0
	if len(parts) >= 4 {
		a, _ = strconv.ParseFloat(strings.TrimSuffix(parts[3], "%"), 64)
	}
	r, g, b := hslToRGB(h, s/100, l/100)
	return Color{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: uint8(a * 255),
	}, true
}

// hslToRGB converts HSL (0-360, 0-1, 0-1) to RGB (0-1, 0-1, 0-1).
func hslToRGB(h, s, l float64) (float64, float64, float64) {
	h = h / 360
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return hueToRGB(p, q, h+1.0/3), hueToRGB(p, q, h), hueToRGB(p, q, h-1.0/3)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// namedColor returns the RGB value of a CSS named color. The set here covers the
// basic 16 plus the common HTML colors.
func namedColor(name string) (Color, bool) {
	switch strings.ToLower(name) {
	case "black":
		return Color{0, 0, 0, 0xFF}, true
	case "white":
		return Color{255, 255, 255, 0xFF}, true
	case "red":
		return Color{255, 0, 0, 0xFF}, true
	case "green", "lime":
		return Color{0, 255, 0, 0xFF}, true
	case "blue":
		return Color{0, 0, 255, 0xFF}, true
	case "yellow":
		return Color{255, 255, 0, 0xFF}, true
	case "cyan", "aqua":
		return Color{0, 255, 255, 0xFF}, true
	case "magenta", "fuchsia":
		return Color{255, 0, 255, 0xFF}, true
	case "silver":
		return Color{192, 192, 192, 0xFF}, true
	case "gray", "grey":
		return Color{128, 128, 128, 0xFF}, true
	case "maroon":
		return Color{128, 0, 0, 0xFF}, true
	case "olive":
		return Color{128, 128, 0, 0xFF}, true
	case "navy":
		return Color{0, 0, 128, 0xFF}, true
	case "teal":
		return Color{0, 128, 128, 0xFF}, true
	case "purple":
		return Color{128, 0, 128, 0xFF}, true
	case "orange":
		return Color{255, 165, 0, 0xFF}, true
	case "pink":
		return Color{255, 192, 203, 0xFF}, true
	case "brown":
		return Color{165, 42, 42, 0xFF}, true
	}
	return Color{}, false
}

// resolveCustomProperties iterates over the custom property map and resolves
// var() references. The substitution is performed by walking the token slice and
// splicing the referenced property's tokens in place.
func (r *Resolver) resolveCustomProperties(cs *ComputedStyle) {
	for name, tokens := range cs.CustomProperties {
		cs.CustomProperties[name] = r.resolveVarInTokens(cs, tokens, map[string]bool{})
	}
}

// resolveVarInTokens returns the token slice with var() references resolved.
func (r *Resolver) resolveVarInTokens(cs *ComputedStyle, tokens []css.Token, seen map[string]bool) []css.Token {
	if seen == nil {
		seen = map[string]bool{}
	}
	var out []css.Token
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Type == css.TokenFunction && strings.EqualFold(t.Value, "var") {
			// Look for the closing right parenthesis.
			depth := 0
			j := i + 1
			var args []css.Token
			for ; j < len(tokens); j++ {
				tj := tokens[j]
				if tj.Type == css.TokenLeftParenthesis {
					depth++
				} else if tj.Type == css.TokenRightParenthesis {
					if depth == 0 {
						break
					}
					depth--
				}
				args = append(args, tj)
			}
			// Split args at the first comma into the variable name and the fallback.
			var name string
			var fallback []css.Token
			commaIdx := -1
			for k, at := range args {
				if at.Type == css.TokenComma {
					commaIdx = k
					break
				}
			}
			if commaIdx == -1 {
				name = identValue(args)
			} else {
				name = identValue(args[:commaIdx])
				fallback = args[commaIdx+1:]
			}
			name = strings.ToLower(name)
			if seen[name] {
				// Cycle; use the fallback (or nothing).
				if fallback != nil {
					out = append(out, fallback...)
				}
				i = j
				continue
			}
			if val, ok := cs.CustomProperties[name]; ok && len(val) > 0 {
				seen[name] = true
				out = append(out, r.resolveVarInTokens(cs, val, seen)...)
				delete(seen, name)
			} else if fallback != nil {
				out = append(out, r.resolveVarInTokens(cs, fallback, seen)...)
			}
			i = j
			continue
		}
		out = append(out, t)
	}
	return out
}

// identValue returns the value of the first identifier token in the slice.
func identValue(tokens []css.Token) string {
	for _, t := range tokens {
		if t.Type == css.TokenIdent {
			return t.Value
		}
		if t.Type == css.TokenHash {
			return t.Value
		}
	}
	return ""
}

// parentElement returns the parent element of el, or nil if the parent is not an
// element (e.g. the document).
func parentElement(el *dom.Element) *dom.Element {
	p := el.ParentNode()
	if p == nil {
		return nil
	}
	if pe, ok := p.(*dom.Element); ok {
		return pe
	}
	return nil
}

// ResolveDocument walks the document tree starting at the root element and resolves
// each element's ComputedStyle, returning a map keyed by element. This is a
// convenience entry point; per-element ResolveElement calls suffice in most cases.
func (r *Resolver) ResolveDocument(doc *dom.Document) map[*dom.Element]*ComputedStyle {
	out := map[*dom.Element]*ComputedStyle{}
	root := doc.DocumentElement()
	if root == nil {
		return out
	}
	var walk func(el *dom.Element)
	walk = func(el *dom.Element) {
		out[el] = r.ResolveElement(el)
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if child, ok := c.(*dom.Element); ok {
				walk(child)
			}
		}
	}
	walk(root)
	return out
}

// parseAnimationShorthand parses the CSS animation shorthand into its name,
// duration (seconds), and iteration-count (0 = infinite). Only the common
// form "name duration iteration-count" is handled; timing-function / delay
// are ignored for simplicity.
func parseAnimationShorthand(s string) (name string, duration float64, iterationCount int) {
	parts := strings.Fields(s)
	for _, p := range parts {
		p = strings.ToLower(p)
		switch {
		case p == "infinite":
			iterationCount = 0
		case strings.HasSuffix(p, "s") && !strings.HasSuffix(p, "ms"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(p, "s"), 64); err == nil {
				duration = v
			}
		case strings.HasSuffix(p, "ms"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(p, "ms"), 64); err == nil {
				duration = v / 1000
			}
		case strings.HasPrefix(p, "infinite"):
			iterationCount = 0
		default:
			if n, err := strconv.Atoi(p); err == nil {
				iterationCount = n
			} else if name == "" {
				name = p
			}
		}
	}
	return
}
