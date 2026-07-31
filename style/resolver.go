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
	// mediaQueryCtx holds the current viewport/device context for media query
	// evaluation. Updated by SetMediaQueryContext / SetViewportSize.
	mediaQueryCtx css.MediaQueryContext
	// StyleSheetLoader is an optional callback for resolving @import URLs.
	// When set, encountering an @import rule in AddStyleSheet triggers a fetch
	// via this callback, the response is parsed as CSS and the resulting rules
	// are merged into the cascade. When nil, @import rules are silently skipped.
	StyleSheetLoader func(href string) (string, error)
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

// SetViewportSize updates the viewport dimensions used for media query evaluation.
// This is typically called when the FrameView is resized.
func (r *Resolver) SetViewportSize(w, h int) {
	r.mediaQueryCtx.Width = w
	r.mediaQueryCtx.Height = h
}

// SetMediaQueryContext replaces the entire media query evaluation context.
// Call this when device characteristics change (e.g. orientation, DPR).
func (r *Resolver) SetMediaQueryContext(ctx css.MediaQueryContext) {
	r.mediaQueryCtx = ctx
}

// AddStyleSheet adds a parsed stylesheet to the resolver's cascade. If the sheet
// contains @import rules and a StyleSheetLoader is configured, the imported
// stylesheets are fetched and their rules are also added. Sheets added earlier
// have lower source order than sheets added later (within the same origin).
func (r *Resolver) AddStyleSheet(sheet *css.CSSStyleSheet) {
	r.resolveImports(sheet)
	r.sheets = append(r.sheets, sheet)
	r.addKeyframesFromSheet(sheet)
}
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
// the stylesheets or DOM so subsequent calls compute fresh values.
func (r *Resolver) ClearCache() {
	r.cache = map[*dom.Element]*ComputedStyle{}
}

// resolveImports walks all rules in the sheet, and for each @import rule
// that has a non-empty Href, fetches the CSS via StyleSheetLoader, parses
// it, and adds the resulting sheet to the resolver. This recursively resolves
// @import chains up to a reasonable depth. Media-conditional imports are
// checked against the current media query context.
func (r *Resolver) resolveImports(sheet *css.CSSStyleSheet) {
	if r.StyleSheetLoader == nil {
		return
	}
	for _, rule := range sheet.Rules() {
		imp, ok := rule.(*css.ImportRule)
		if !ok {
			continue
		}
		if imp.Href == "" {
			continue
		}
		// Check media condition if present (skip if the media query does
		// not match the current context).
		if imp.Media != "" && r.mediaQueryCtx.Width > 0 {
			parsed, _ := css.ParseMediaQueryList(imp.Media)
			if len(parsed) > 0 && !css.MatchesAny(parsed, r.mediaQueryCtx) {
				continue
			}
		}
		cssText, err := r.StyleSheetLoader(imp.Href)
		if err != nil || cssText == "" {
			continue
		}
		importedSheet := css.NewCSSStyleSheetWithOwner(nil, imp.Href)
		importedSheet.SetOrigin(imp.Origin)
		p := css.NewParser(cssText)
		p.ParseStyleSheetInto(importedSheet)

		// Recursively resolve imports in the imported sheet.
		r.resolveImports(importedSheet)

		r.sheets = append(r.sheets, importedSheet)
		r.addKeyframesFromSheet(importedSheet)
	}
}

// collectedDecl is an intermediate structure used during cascade sorting.
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

	// Apply the HTML UA default display mapping before any author declarations, so
	// that block-type elements (section, article, div, p, h1-h6, …) start with
	// DisplayBlock — mirroring the browser UA stylesheet. Author declarations (CSS
	// in <style> or the style attribute) override this default via the cascade.
	applyDefaultDisplay(cs, el)

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

	// Resolve var() references in all regular properties (Properties map).
	// Custom properties have already been resolved above; now we substitute
	// var(--xxx) in property values like "background: var(--bg-primary)"
	// and re-apply them to typed fields where applicable.
	r.resolveVarInProperties(cs)

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
						// Skip selectors that consist ONLY of pseudo-elements
						// (e.g. ::selection, ::-webkit-scrollbar-thumb),
						// OR selectors that CONTAIN pseudo-elements
						// (e.g. .clearfix::after).
						// These should not apply declarations to the element itself
						// — they only apply when the resolver resolves the element
						// FOR the pseudo-element. Without this check, rules like
						// ::selection { color: #fff; } or
						// .clearfix::after { display: table; } would leak into
						// the base element's computed style.
						if isPseudoElementOnly(&sel) || hasPseudoElement(&sel) {
							continue
						}
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
			// Evaluate media queries against the current device/viewport context.
			// If the parsed query list is empty (parse error or unsupported syntax),
			// fall through to always-include to match the pre-existing behaviour.
			if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
				continue // skip rules inside non-matching @media
			}
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

	// If the declaration value is a calc() expression, store the raw tokens so the
	// layout engine can resolve them later with proper context (parent width, etc.).
	// Simple px-only calc() expressions are resolved immediately by parseLength; the
	// token storage here provides a fallback for relative-unit calc().
	if css.IsCalcValue(d.Value) {
		if cs.CalcValues == nil {
			cs.CalcValues = map[string][]css.Token{}
		}
		cs.CalcValues[name] = d.Value
	}

	if strings.HasPrefix(name, "--") {
		cs.SetCustomProperty(name, d.Value)
		if d.Important {
			cs.ImportantProperties[name] = true
		}
		return
	}
	// Store the raw value string so the var() resolution pass later
	// (resolveVarInProperties) can find and substitute var(--xxx) references.
	// Without this, Properties remains empty and CSS variables never get resolved.
	cs.SetProperty(name, valueString)

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
	case "background":
		// Shorthand: resets background-image (unless the value carries
		// gradient layers) and extracts background-color. Multiple gradient
		// layers are joined with commas (first = topmost).
		cs.BackgroundImage = ""
		var grads []string
		for _, p := range splitShorthandValue(valueString) {
			if strings.HasPrefix(p, "linear-gradient(") || strings.HasPrefix(p, "radial-gradient(") {
				grads = append(grads, p)
				continue
			}
			if c, ok := parseColor(p); ok {
				cs.BackgroundColor = c
				break
			}
		}
		if len(grads) > 0 {
			cs.BackgroundImage = strings.Join(grads, ", ")
		}
	case "font-family":
		cs.FontFamily = strings.Trim(valueString, `"'`)
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
	case "text-overflow":
		cs.TextOverflow = LookupTextOverflow(valueString)
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
	case "border-collapse":
		cs.BorderCollapse = parseBorderCollapse(valueString)
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
			// The `gap` shorthand sets both row-gap and column-gap (CSS
			// Box Alignment §gap). Grid/Flex read the longhands directly.
			cs.RowGap = l
			cs.ColumnGap = l
		}
	case "row-gap":
		if l, ok := parseLength(valueString); ok {
			cs.RowGap = l
		}
	case "column-gap":
		if l, ok := parseLength(valueString); ok {
			cs.ColumnGap = l
		}
	case "column-count":
		if v, err := strconv.Atoi(valueString); err == nil && v > 0 {
			cs.ColumnCount = v
		}
	case "column-width":
		if l, ok := parseLength(valueString); ok {
			cs.ColumnWidth = l
		}
	case "column-rule-color":
		cs.ColumnRuleColor = valueString
	case "column-rule-style":
		cs.ColumnRuleStyle = valueString
	case "column-rule-width":
		if l, ok := parseLength(valueString); ok {
			cs.ColumnRuleWidth = l
		}
	case "column-rule":
		// Shorthand: <width> <style> <color>.
		parts := strings.Fields(valueString)
		for _, part := range parts {
			if l, ok := parseLength(part); ok {
				cs.ColumnRuleWidth = l
			} else {
				// Check border-style keywords inline.
				switch part {
				case "none", "hidden", "dotted", "dashed", "solid", "double",
					"groove", "ridge", "inset", "outset":
					cs.ColumnRuleStyle = part
				default:
					cs.ColumnRuleColor = part
				}
			}
		}
	case "column-fill":
		cs.ColumnFill = valueString
	case "columns":
		// Shorthand: <column-width> || <column-count>
		parts := strings.Fields(valueString)
		for _, part := range parts {
			if l, ok := parseLength(part); ok {
				cs.ColumnWidth = l
			} else if v, err := strconv.Atoi(part); err == nil && v > 0 {
				cs.ColumnCount = v
			}
		}
	case "writing-mode":
		cs.WritingMode = valueString
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
	case "grid-area":
		// Shorthand: grid-area: <name> | <row-start> / <col-start> / <row-end> / <col-end>
		if parts := splitShorthand(valueString, "/"); len(parts) >= 1 {
			cs.GridTemplateAreas = strings.TrimSpace(parts[0])
			if len(parts) >= 2 {
				cs.GridRowStart = strings.TrimSpace(parts[1])
			}
			if len(parts) >= 3 {
				cs.GridColumnStart = strings.TrimSpace(parts[2])
			}
			if len(parts) >= 4 {
				cs.GridRowEnd = strings.TrimSpace(parts[3])
			}
			if len(parts) >= 5 {
				cs.GridColumnEnd = strings.TrimSpace(parts[4])
			}
		}
	case "grid-column":
		// Shorthand: grid-column: <start> [ / <end> ]?
		if parts := splitShorthand(valueString, "/"); len(parts) >= 1 {
			cs.GridColumnStart = strings.TrimSpace(parts[0])
			if len(parts) >= 2 {
				cs.GridColumnEnd = strings.TrimSpace(parts[1])
			}
		}
	case "grid-row":
		// Shorthand: grid-row: <start> [ / <end> ]?
		if parts := splitShorthand(valueString, "/"); len(parts) >= 1 {
			cs.GridRowStart = strings.TrimSpace(parts[0])
			if len(parts) >= 2 {
				cs.GridRowEnd = strings.TrimSpace(parts[1])
			}
		}
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
	case "transform-origin":
		cs.TransformOriginX, cs.TransformOriginY = parseTransformOrigin(valueString)
	case "transition":
		cs.Transition = valueString
		cs.TransitionProperty, cs.TransitionDuration, cs.TransitionTimingFunction, cs.TransitionDelay = parseTransitionShorthand(valueString)
	case "transition-property":
		cs.TransitionProperty = valueString
	case "transition-duration":
		cs.TransitionDuration = parseSeconds(valueString)
	case "transition-timing-function":
		cs.TransitionTimingFunction = valueString
	case "transition-delay":
		cs.TransitionDelay = parseSeconds(valueString)
	case "animation":
		cs.Animation = valueString
		cs.AnimationName, cs.AnimationDuration, cs.AnimationIterationCount, cs.AnimationDelay, cs.AnimationDirection, cs.AnimationFillMode, cs.AnimationTimingFunction = parseAnimationShorthand(valueString)
	case "animation-name":
		cs.AnimationName = valueString
	case "animation-duration":
		if v, err := strconv.ParseFloat(strings.TrimSuffix(valueString, "s"), 64); err == nil {
			cs.AnimationDuration = v
		}
	case "animation-delay":
		if v, err := strconv.ParseFloat(strings.TrimSuffix(valueString, "s"), 64); err == nil {
			cs.AnimationDelay = v
		}
	case "animation-iteration-count":
		if v, err := strconv.Atoi(valueString); err == nil {
			cs.AnimationIterationCount = v
		}
	case "animation-direction":
		cs.AnimationDirection = valueString
	case "animation-fill-mode":
		cs.AnimationFillMode = valueString
	case "animation-timing-function":
		cs.AnimationTimingFunction = valueString
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
	// Handle calc() expressions.
	if isCalcValueS(s) {
		argStr := extractCalcArgS(s)
		// Re-tokenize the calc expression for EvalCalc.
		calcTok := css.NewTokenizer(argStr)
		calcTokens := calcTok.Tokenize()
		// Strip EOF token.
		if len(calcTokens) > 0 && calcTokens[len(calcTokens)-1].Type == css.TokenEOF {
			calcTokens = calcTokens[:len(calcTokens)-1]
		}
		// Wrap with "calc(" prefix so IsCalcValue / extractCalcInner work.
		fullExpr := "calc(" + argStr + ")"
		fullTok := css.NewTokenizer(fullExpr)
		fullTokens := fullTok.Tokenize()
		if len(fullTokens) > 0 && fullTokens[len(fullTokens)-1].Type == css.TokenEOF {
			fullTokens = fullTokens[:len(fullTokens)-1]
		}
		v, err := css.EvalCalc(fullTokens, css.CalcContext{})
		if err == nil {
			return Length{Value: v, Unit: "px"}, true
		}
		// If the calc contains % (unresolvable without context), still return ok=true
		// with a marker unit so the layout engine can re-evaluate with context later.
		if strings.Contains(argStr, "%") {
			return Length{Value: 0, Unit: "calc"}, true
		}
		return Length{}, false
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
		// CSS keywords like "auto" are valid for margins.
		if s == "auto" {
			return Length{Unit: "auto"}, true
		}
		return Length{}, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return Length{}, false
	}
	return Length{Value: num, Unit: s[i:]}, true
}

// isCalcValueS reports whether s starts with "calc(" (case-insensitive).
func isCalcValueS(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 5 && (s[:5] == "calc(" || s[:5] == "CALC(" || strings.ToLower(s[:5]) == "calc(")
}

// extractCalcArgS returns the text between "calc(" and the matching ")".
func extractCalcArgS(s string) string {
	s = strings.TrimSpace(s)
	if !isCalcValueS(s) {
		return ""
	}
	// Find the opening paren position.
	parenIdx := 4 // after "calc"
	if len(s) > parenIdx && s[parenIdx] == '(' {
		parenIdx++
	}
	// Walk to find the matching ')', tracking nested parens.
	depth := 1
	for i := parenIdx; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return s[parenIdx:i]
			}
		}
	}
	// Fallback: return everything after "calc(" up to the last ')'.
	if last := strings.LastIndex(s, ")"); last > parenIdx {
		return s[parenIdx:last]
	}
	return s[parenIdx:]
}

// parseColor parses a CSS color value. Supports #rgb / #rrggbb / #rrggbbaa / rgb() /
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
	// System colors (CSS Color 4 §6.1) used by UA stylesheets.
	switch s {
	case "-webkit-link", "linktext":
		// Default hyperlink blue (UA stylesheet link color).
		return Color{R: 0x00, G: 0x00, B: 0xEE, A: 0xFF}, true
	case "-webkit-activelink":
		// Default active-link red.
		return Color{R: 0xEE, G: 0x00, B: 0x00, A: 0xFF}, true
	case "canvas", "background":
		return Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, true
	case "canvastext", "foreground":
		return Color{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}, true
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

// splitShorthandValue splits a shorthand value on whitespace while keeping
// parenthesized groups (rgb(...), calc(...), url(...)) together, so function
// values are not torn apart into separate fields.
func splitShorthandValue(s string) []string {
	var parts []string
	depth := 0
	cur := strings.Builder{}
	for _, r := range s {
		switch r {
		case '(':
			depth++
			cur.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case ' ', '\t', '\n', '\r':
			if depth > 0 {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// parseBorderShorthand parses a border shorthand value like "1px solid #e5e7eb" and
// returns (width, style, color, ok). Components may appear in any order; missing
// components are left as their zero value.
func parseBorderShorthand(s string) (width Length, style string, color Color, ok bool) {
	parts := splitShorthandValue(s)
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
		// currentcolor resolves to the element's color property at paint time;
		// leave the zero Color so the renderer falls back to st.Color.
		if strings.EqualFold(p, "currentcolor") {
			color = Color{}
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

// resolveVarInProperties resolves var() references in all regular property
// values stored in cs.Properties. Custom properties (--xxx) have already been
// resolved by resolveCustomProperties. After substitution, known properties
// are re-applied to typed fields so the layout/paint engine can read them.
func (r *Resolver) resolveVarInProperties(cs *ComputedStyle) {
	if len(cs.CustomProperties) == 0 {
		return // nothing to substitute
	}
	for name, raw := range cs.Properties {
		if !strings.Contains(raw, "var(") {
			continue
		}
		// Tokenize the raw value and resolve var() references.
		tok := css.NewTokenizer(raw)
		tokens := tok.Tokenize()
		if len(tokens) > 0 && tokens[len(tokens)-1].Type == css.TokenEOF {
			tokens = tokens[:len(tokens)-1]
		}
		resolved := r.resolveVarInTokens(cs, tokens, map[string]bool{})
		// Convert resolved tokens back to a string.
		resolvedStr := tokensToString(resolved)
		cs.Properties[name] = resolvedStr

		// Re-apply to typed fields for known properties.
		switch name {
		case "background-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.BackgroundColor = c
			}
		case "color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.Color = c
			}
		case "font-family":
			cs.FontFamily = strings.Trim(resolvedStr, `"'`)
		case "font-size":
			if l, ok := parseLength(resolvedStr); ok {
				cs.FontSize = l
			}
		case "width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.Width = l
			}
		case "height":
			if l, ok := parseLength(resolvedStr); ok {
				cs.Height = l
			}
		case "display":
			cs.Display = LookupDisplayType(resolvedStr)
			cs.DisplaySet = true
		case "background":
			// Shorthand: resets background-image (unless gradient layers are
			// present) and extracts background-color.
			cs.BackgroundImage = ""
			var grads []string
			for _, p := range splitShorthandValue(resolvedStr) {
				if strings.HasPrefix(p, "linear-gradient(") || strings.HasPrefix(p, "radial-gradient(") {
					grads = append(grads, p)
					continue
				}
				if c, ok := parseColor(p); ok {
					cs.BackgroundColor = c
					break
				}
			}
			if len(grads) > 0 {
				cs.BackgroundImage = strings.Join(grads, ", ")
			}
		case "box-shadow":
			cs.BoxShadow = resolvedStr
		case "margin":
			// Shorthand — store as string for now
			cs.SetProperty("margin", resolvedStr)
		case "margin-top":
			if l, ok := parseLength(resolvedStr); ok {
				cs.MarginTop = l
			}
		case "margin-right":
			if l, ok := parseLength(resolvedStr); ok {
				cs.MarginRight = l
			}
		case "margin-bottom":
			if l, ok := parseLength(resolvedStr); ok {
				cs.MarginBottom = l
			}
		case "margin-left":
			if l, ok := parseLength(resolvedStr); ok {
				cs.MarginLeft = l
			}
		case "padding-top":
			if l, ok := parseLength(resolvedStr); ok {
				cs.PaddingTop = l
			}
		case "padding-right":
			if l, ok := parseLength(resolvedStr); ok {
				cs.PaddingRight = l
			}
		case "padding-bottom":
			if l, ok := parseLength(resolvedStr); ok {
				cs.PaddingBottom = l
			}
		case "padding-left":
			if l, ok := parseLength(resolvedStr); ok {
				cs.PaddingLeft = l
			}
		case "border-top-width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.BorderTopWidth = l
			}
		case "border-right-width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.BorderRightWidth = l
			}
		case "border-bottom-width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.BorderBottomWidth = l
			}
		case "border-left-width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.BorderLeftWidth = l
			}
		case "border-radius":
			if l, ok := parseLength(resolvedStr); ok {
				cs.BorderRadius = l
			}
		case "flex-basis":
			if l, ok := parseLength(resolvedStr); ok {
				cs.FlexBasis = l
			}
		case "gap":
			if l, ok := parseLength(resolvedStr); ok {
				cs.Gap = l
				// The `gap` shorthand sets both row-gap and column-gap.
				cs.RowGap = l
				cs.ColumnGap = l
			}
		case "transition":
			cs.Transition = resolvedStr
		case "border":
			if w, s, c, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderTopWidth, cs.BorderRightWidth = w, w
				cs.BorderBottomWidth, cs.BorderLeftWidth = w, w
				cs.BorderTopStyle, cs.BorderRightStyle = s, s
				cs.BorderBottomStyle, cs.BorderLeftStyle = s, s
				cs.BorderTopColor, cs.BorderRightColor = c, c
				cs.BorderBottomColor, cs.BorderLeftColor = c, c
			}
		case "border-top":
			if w, s, c, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderTopWidth, cs.BorderTopStyle, cs.BorderTopColor = w, s, c
			}
		case "border-right":
			if w, s, c, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderRightWidth, cs.BorderRightStyle, cs.BorderRightColor = w, s, c
			}
		case "border-bottom":
			if w, s, c, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderBottomWidth, cs.BorderBottomStyle, cs.BorderBottomColor = w, s, c
			}
		case "border-left":
			if w, s, c, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderLeftWidth, cs.BorderLeftStyle, cs.BorderLeftColor = w, s, c
			}
		case "border-top-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.BorderTopColor = c
			}
		case "border-right-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.BorderRightColor = c
			}
		case "border-bottom-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.BorderBottomColor = c
			}
		case "border-left-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.BorderLeftColor = c
			}
		}
	}
}

// tokensToString converts a token slice back to a CSS value string.
func tokensToString(tokens []css.Token) string {
	var sb strings.Builder
	for i, t := range tokens {
		if i > 0 && t.Type != css.TokenComma && t.Type != css.TokenRightParenthesis {
			prev := tokens[i-1]
			if prev.Type != css.TokenLeftParenthesis && prev.Type != css.TokenComma {
				sb.WriteByte(' ')
			}
		}
		switch t.Type {
		case css.TokenIdent, css.TokenFunction, css.TokenAtKeyword,
			css.TokenURL, css.TokenBadURL, css.TokenString, css.TokenBadString:
			sb.WriteString(t.Value)
			if t.Type == css.TokenFunction {
				sb.WriteByte('(')
			}
		case css.TokenHash:
			sb.WriteByte('#')
			sb.WriteString(t.Value)
		case css.TokenNumber, css.TokenPercentage, css.TokenDimension:
			s := strconv.FormatFloat(t.Numeric, 'f', -1, 64)
			if t.Unit != "" {
				s += t.Unit
			}
			if t.Type == css.TokenPercentage {
				s += "%"
			}
			sb.WriteString(s)
		case css.TokenDelimiter:
			sb.WriteRune(t.Delimiter)
		case css.TokenComma:
			sb.WriteString(", ")
		case css.TokenLeftParenthesis:
			sb.WriteByte('(')
		case css.TokenRightParenthesis:
			sb.WriteByte(')')
		case css.TokenNonNewlineWhitespace, css.TokenNewline:
			sb.WriteByte(' ')
		default:
			sb.WriteString(t.Value)
		}
	}
	return strings.TrimSpace(sb.String())
}

// applyDefaultDisplay sets the default display property based on the HTML tag name,
// mirroring the browser's UA stylesheet default display mapping. This runs before
// any author declarations so that CSS rules can override the default.
func applyDefaultDisplay(cs *ComputedStyle, el *dom.Element) {
	tag := strings.ToLower(el.LocalName())
	switch tag {
	// Block elements
	case "html", "body", "main", "section", "article", "nav", "aside",
		"header", "footer", "hgroup", "figure", "figcaption",
		"div", "p", "hr", "pre", "blockquote", "address",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "dl", "dt", "dd",
		"table", "caption", "colgroup", "col", "tbody", "thead", "tfoot", "tr",
		"fieldset", "legend", "form", "details", "summary", "dialog":
		cs.Display = DisplayBlock
		cs.DisplaySet = true
	// Inline-block replaced elements
	case "button", "input", "textarea", "select", "meter", "progress",
		"img", "canvas", "video", "audio", "object", "embed":
		cs.Display = DisplayInlineBlock
		cs.DisplaySet = true
	// Table-cell
	case "td", "th":
		cs.Display = DisplayTableCell
		cs.DisplaySet = true
	// List-item
	case "li":
		cs.Display = DisplayListItem
		cs.DisplaySet = true
	// Inline (default from NewComputedStyle, no-op)
	default:
		// keep DisplayInline from NewComputedStyle
	}
}

// parentElement returns the parent element of el, or nil if the parent is not an
// element (e.g. the document).
// isPseudoElementOnly reports whether the complex selector consists of nothing
// but pseudo-element simple selectors (e.g. ::selection, ::-webkit-scrollbar-thumb).
// Such selectors should NOT apply their declarations to the base element — they
// only apply when the resolver resolves the element FOR the pseudo-element.
func isPseudoElementOnly(sel *css.ComplexSelector) bool {
	if sel == nil || len(sel.Compounds) == 0 {
		return false
	}
	// Check the LAST compound (the one actually matched against the element)
	last := sel.Compounds[len(sel.Compounds)-1]
	if len(last.Selectors) == 0 {
		return false
	}
	for _, s := range last.Selectors {
		if s.Match != css.MatchPseudoElement {
			return false
		}
	}
	return true
}

// hasPseudoElement reports whether any compound in the complex selector contains
// a pseudo-element simple selector (e.g. ::after, ::before, ::selection).
// Selectors with pseudo-elements should not apply their declarations to the base
// element — they only apply when the resolver resolves for the pseudo-element.
func hasPseudoElement(sel *css.ComplexSelector) bool {
	if sel == nil {
		return false
	}
	for _, comp := range sel.Compounds {
		for _, s := range comp.Selectors {
			if s.Match == css.MatchPseudoElement {
				return true
			}
		}
	}
	return false
}

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

// splitShorthand splits a CSS shorthand value by the given separator and returns the
// resulting parts with balanced parentheses handling (so "rgb(...)/ 2" is correctly
// split into two parts only at the unquoted, unparenthesized separator).
func splitShorthand(value, sep string) []string {
	depth := 0
	var parts []string
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && string(value[i]) == sep {
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	parts = append(parts, value[start:])
	return parts
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

// parseAnimationShorthand parses the CSS animation shorthand into its
// component values. It handles name, duration, timing-function, delay,
// iteration-count, direction, and fill-mode in any order.
//
// Recognised keywords:
//
//	duration/time:       0.5s, 500ms, 2s
//	iteration-count:     3, infinite
//	timing-function:     linear, ease, ease-in, ease-out, ease-in-out
//	direction:           normal, reverse, alternate, alternate-reverse
//	fill-mode:           none, forwards, backwards, both
//	name:                any other identifier (first unrecognised token is treated as the name)
func parseAnimationShorthand(s string) (name string, duration float64, iterationCount int, delay float64, direction string, fillMode string, timingFunction string) {
	iterationCount = 1 // default per spec
	parts := strings.Fields(s)
	for _, p := range parts {
		pl := strings.ToLower(p)
		switch {
		case pl == "infinite":
			iterationCount = 0
		case pl == "none":
			// "none" as an animation name is reserved (no animation).
			if name == "" {
				name = "none"
			}
		case pl == "linear", pl == "ease", pl == "ease-in", pl == "ease-out", pl == "ease-in-out":
			timingFunction = pl
		case pl == "normal", pl == "reverse", pl == "alternate", pl == "alternate-reverse":
			direction = pl
		case pl == "forwards", pl == "backwards", pl == "both":
			fillMode = pl
		case strings.HasSuffix(pl, "ms"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(pl, "ms"), 64); err == nil {
				v /= 1000
				if duration == 0 {
					duration = v
				} else if delay == 0 {
					delay = v
				}
			} else if name == "" {
				name = p
			}
		case strings.HasSuffix(pl, "s"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(pl, "s"), 64); err == nil {
				if duration == 0 {
					duration = v
				} else if delay == 0 {
					delay = v
				}
			} else if name == "" {
				name = p
			}
		default:
			if n, err := strconv.Atoi(pl); err == nil {
				iterationCount = n
			} else if name == "" {
				name = p
			}
		}
	}
	if duration == 0 {
		duration = 0 // 0s = no animation duration
	}
	return
}

// parseTransitionShorthand parses the CSS transition shorthand:
//   transition: <property> <duration> <timing-function> <delay>
// Examples: "all 0.3s ease", "opacity 0.2s", "transform 0.5s ease-in-out"
// parseTransformOrigin parses "transform-origin: <x> <y>" where each value is
// a percentage, length, or keyword (left/center/right, top/middle/bottom).
// Defaults to 50% 50% (CSS 2.1 §11.1.2).
func parseTransformOrigin(s string) (x, y Length) {
	x = Length{Value: 50, Unit: "%"}
	y = Length{Value: 50, Unit: "%"}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return
	}
	parseAxis := func(v string) Length {
		switch v {
		case "left", "top":
			return Length{Value: 0, Unit: "%"}
		case "center", "middle":
			return Length{Value: 50, Unit: "%"}
		case "right", "bottom":
			return Length{Value: 100, Unit: "%"}
		}
		if strings.HasSuffix(v, "%") {
			if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
				return Length{Value: n, Unit: "%"}
			}
			return Length{Value: 50, Unit: "%"}
		}
		if l, ok := parseLength(v); ok {
			return l
		}
		return Length{Value: 50, Unit: "%"}
	}
	x = parseAxis(parts[0])
	if len(parts) >= 2 {
		y = parseAxis(parts[1])
	} else if parts[0] == "left" || parts[0] == "right" {
		y = Length{Value: 50, Unit: "%"}
	} else if parts[0] == "top" || parts[0] == "bottom" {
		x = Length{Value: 50, Unit: "%"}
	}
	return
}

func parseTransitionShorthand(s string) (prop string, duration float64, timing string, delay float64) {	if s == "" || s == "none" {
		return "all", 0, "ease", 0
	}
	// Defaults
	prop = "all"
	duration = 0
	timing = "ease"
	delay = 0

	parts := strings.Fields(s)
	for _, p := range parts {
		pl := strings.ToLower(p)
		switch {
		case pl == "all" || pl == "none" || pl == "opacity" || pl == "transform" ||
			pl == "color" || pl == "background-color" || pl == "width" || pl == "height" ||
			pl == "left" || pl == "top" || pl == "right" || pl == "bottom":
			prop = pl
		case pl == "linear" || pl == "ease" || pl == "ease-in" ||
			pl == "ease-out" || pl == "ease-in-out":
			timing = pl
		case strings.HasSuffix(pl, "s"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(pl, "s"), 64); err == nil {
				if duration == 0 {
					duration = v
				} else if delay == 0 {
					delay = v
				}
			}
		case strings.HasSuffix(pl, "ms"):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(pl, "ms"), 64); err == nil {
				v /= 1000 // convert ms to s
				if duration == 0 {
					duration = v
				} else if delay == 0 {
					delay = v
				}
			}
		}
	}
	return
}

// parseSeconds parses a CSS time value like "0.3s" or "150ms" into seconds.
func parseSeconds(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "ms") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64)
		if err != nil {
			return 0
		}
		return v / 1000
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64)
	if err != nil {
		return 0
	}
	return v
}
