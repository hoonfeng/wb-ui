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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
)

// indexedRule pairs a StyleRule with its declaration-stream position inside the
// sheet (count of declarations in ALL rules — including @media bodies — that
// precede it). Source order for the cascade must follow the CSS source, not the
// bucket traversal order, or same-specificity rules would resolve in the wrong
// direction.
type indexedRule struct {
	rule  *css.StyleRule
	order int
}

// ruleBucket is a per-stylesheet rule index built by class/id/tag of the
// rightmost compound selector. Candidate lookup for an element only touches
// the buckets its own tag/id/classes hit, plus the universal bucket — instead
// of scanning every rule of every sheet (O(element × rules) → O(candidates)).
type ruleBucket struct {
	universal []indexedRule // rules with no indexable key (must always match)
	byKey     map[string][]indexedRule
}

// Resolver is the Go translation of WebCore::Style::Resolver. It holds the set of
// author stylesheets (plus UA / user sheets if added) and exposes ResolveElement to
// compute a ComputedStyle for a given Element.
type Resolver struct {
	sheets  []*css.CSSStyleSheet
	checker *css.SelectorChecker
	// cache memoizes per-element ComputedStyle to make inheritance cheap.
	// Keyed by element identity + 属性版本号：class/type/checked 等属性变化
	// 后 AttrVersion 递增，旧缓存自动失效（浏览器 attribute 变化触发 style
	// recalc 的等价物）。此前只按元素指针缓存，class 切换（Vue :class）在
	// style 指纹不变时不清缓存 → 选中/悬停样式残留、多个高亮并存。
	cache map[*dom.Element]cachedStyle
	// sheetIndex holds the per-sheet rule index (nil until first use / rebuild).
	sheetIndex map[*css.CSSStyleSheet]*ruleBucket
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

// cachedStyle 是带版本号的 per-element 计算样式缓存项。
// ver 记录解析时的 AttrVersion，属性变化后自动失效。
type cachedStyle struct {
	cs  *ComputedStyle
	ver uint64
}

// SelectionColors returns the ::selection pseudo-element background and
// foreground colors from the stylesheets, if any rule declares them. Mirrors
// StyleResolver::pseudoStyleForElement for ::selection. The background is
// used by PaintSelection; the foreground by text painting of selected runs.
// Colors are style.Color (the resolver's own type); the renderer converts.
func (r *Resolver) SelectionColors() (bg, fg Color, ok bool) {
	var walk func(rules []css.Rule)
	walk = func(rules []css.Rule) {
		for _, rule := range rules {
			switch r2 := rule.(type) {
			case *css.StyleRule:
				if hasSelectionSelector(r2.Selectors) {
					for _, d := range r2.Declarations {
						switch d.Name {
						case "background-color":
							if c, cok := parseColor(d.ValueString()); cok {
								bg = c
							}
						case "color":
							if c, cok := parseColor(d.ValueString()); cok {
								fg = c
							}
						}
					}
				}
				if len(r2.NestedRules) > 0 {
					walk(r2.NestedRules)
				}
			case *css.MediaRule:
				walk(r2.Rules)
			}
		}
	}
	for _, sheet := range r.sheets {
		walk(sheet.Rules())
	}
	return bg, fg, bg.A != 0 || fg.A != 0
}

// hasSelectionSelector reports whether any complex selector in the list ends
// with the ::selection pseudo-element (::selection matches any element).
func hasSelectionSelector(list *css.SelectorList) bool {
	if list == nil {
		return false
	}
	for _, cs := range list.Selectors {
		if len(cs.Compounds) == 0 {
			continue
		}
		last := cs.Compounds[len(cs.Compounds)-1]
		for _, s := range last.Selectors {
			if s.Match == css.MatchPseudoElement && s.PseudoElem == css.PseudoElementSelection {
				return true
			}
		}
	}
	return false
}

// NewResolver constructs an empty Resolver.
func NewResolver() *Resolver {
	return &Resolver{
		checker:    css.NewSelectorChecker(),
		cache:      map[*dom.Element]cachedStyle{},
		keyframes:  map[string]*css.KeyframesRule{},
		sheetIndex: map[*css.CSSStyleSheet]*ruleBucket{},
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
			delete(r.sheetIndex, sheet)
			return
		}
	}
}

// ClearCache drops the per-element ComputedStyle cache. Call this after mutating
// the stylesheets or DOM so subsequent calls compute fresh values.
func (r *Resolver) ClearCache() {
	r.cache = map[*dom.Element]cachedStyle{}
}

// Invalidate drops the cached ComputedStyle for a single element so the next
// ResolveElement recomputes it (e.g. after a dynamic pseudo-class change like
// :hover). Used by the host's hover fast-path to avoid a full rebuild.
func (r *Resolver) Invalidate(el *dom.Element) {
	if r.cache != nil {
		delete(r.cache, el)
	}
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
type collectedDecl struct {
	decl        css.Declaration
	origin      css.Origin
	important   bool
	specificity css.Specificity
	sourceOrder int
	selector    string // matched rule selector text (diag only)
	sbKind      int    // scrollbar pseudo kind: 0=::-webkit-scrollbar, 1=::-webkit-scrollbar-thumb, -1=none
	// scope is the tree-scope depth of the stylesheet the declaration came from
	// (0 = document tree, 1 = shadow tree hosted directly in the document, 2 = a
	// nested shadow tree, …). Per CSS Scoping Level 1 §3.3, for normal declarations
	// a deeper scope outranks a shallower one (shadow rules beat document rules on
	// the same element); for !important the order reverses.
	scope int
}

// keyStyleProp lists the layout-critical properties whose cascade history the
// WB_DIAG=style probe logs (to surface wrong-override bugs).
var keyStyleProp = map[string]bool{
	"display": true, "position": true, "width": true, "height": true,
	"min-width": true, "max-width": true, "min-height": true, "max-height": true,
	"flex": true, "flex-grow": true, "flex-shrink": true, "flex-basis": true,
	"flex-direction": true, "flex-wrap": true, "justify-content": true,
	"align-items": true, "align-self": true, "gap": true,
	"margin": true, "margin-top": true, "margin-right": true, "margin-bottom": true, "margin-left": true,
	"padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true, "padding-left": true,
	"border": true, "border-width": true, "border-top-width": true, "border-right-width": true,
	"border-bottom-width": true, "border-left-width": true,
	"overflow": true, "overflow-x": true, "overflow-y": true,
	"box-sizing": true, "grid-template-columns": true, "grid-template-rows": true,
	"grid-template-areas": true, "grid-column": true, "grid-row": true,
	"top": true, "right": true, "bottom": true, "left": true,
	"background": true, "background-color": true, "color": true,
	"font-size": true, "line-height": true, "white-space": true,
	"text-overflow": true, "z-index": true, "opacity": true, "float": true,
}

// ResolveElement computes the ComputedStyle for the given element by walking the
// cascade, applying matched declarations in order, and resolving inheritance /
// custom properties. The result is cached.
func (r *Resolver) ResolveElement(el *dom.Element) *ComputedStyle {
	if c, ok := r.cache[el]; ok && c.ver == el.AttrVersion() {
		return c.cs
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

	// Collect declarations in cascade order (indexed: only candidate rules
	// whose rightmost compound mentions the element's tag/id/class are matched,
	// instead of scanning every rule of every sheet).
	var collected []collectedDecl
	for _, sheet := range r.sheets {
		r.collectSheetDeclarations(sheet, el, &collected)
		// Cross-shadow-boundary cascade origins (CSS Scoping Level 1): rules that
		// target an element on the *other* side of a shadow boundary than their own
		// stylesheet. :host rules style the shadow host; ::slotted rules style
		// slot-assigned light-DOM nodes; ::part rules style shadow-tree elements
		// from an outer scope.
		r.collectShadowHostDeclarations(sheet, el, &collected)
		r.collectSlottedDeclarations(sheet, el, &collected)
		r.collectPartDeclarations(sheet, el, &collected)
	}

	// ::-webkit-scrollbar / ::-webkit-scrollbar-thumb rules (WebKit/Blink
	// scrollbar styling) apply to the element's scrollbar palette, not the
	// element itself — collect them separately and map onto raw properties
	// the painter reads (width/height/track/thumb color + radius). Uses the
	// rule index (candidate buckets) like the main cascade.
	var sbDecls []collectedDecl
	for _, sheet := range r.sheets {
		bkt := r.sheetIndex[sheet]
		if bkt == nil {
			bkt = buildRuleBucket(sheet)
			r.sheetIndex[sheet] = bkt
		}
		order := 0
		for _, ir := range bkt.candidates(el) {
			order = r.collectScrollbarFromRule(ir.rule, sheet.Origin(), el, &sbDecls, order)
		}
		for _, rule := range sheet.Rules() {
			switch v := rule.(type) {
			case *css.MediaRule:
				if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
					continue
				}
				order = r.collectScrollbarDeclarations(v.Rules, sheet.Origin(), el, &sbDecls, order)
			case *css.SupportsRule:
				order = r.collectScrollbarDeclarations(v.Rules, sheet.Origin(), el, &sbDecls, order)
			}
		}
	}
	applyScrollbarDeclarations(cs, sbDecls)

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
				scope:       elScopeDepth(el),
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
		// Tree-scope depth: a deeper scope outranks a shallower one for normal
		// declarations (shadow rules beat document rules on the same element);
		// !important reverses the scope order (outer scope wins).
		if a.scope != b.scope {
			if a.important {
				return a.scope > b.scope
			}
			return a.scope < b.scope
		}
		if c := a.specificity.Compare(b.specificity); c != 0 {
			return c < 0
		}
		return a.sourceOrder < b.sourceOrder
	})
	// Apply declarations in sorted order; later ones overwrite earlier ones.
	diag := DiagEnabled("style")
	if diag {
		// Track how many distinct rules touched each key layout property so
		// probes can spot cascade overrides (the "wrong value wins" class of
		// bugs) — a single element where width/height/flex/overflow is set by
		// several rules with different values is a prime suspect.
		elTag := el.LocalName()
		elCls := el.GetAttribute("class")
		elID := el.GetAttribute("id")
		elName := elTag
		if elID != "" {
			elName += "#" + elID
		}
		if elCls != "" {
			elName += "." + strings.Fields(elCls)[0]
		}
		for _, cd := range collected {
			pn := strings.ToLower(cd.decl.Name)
			if keyStyleProp[pn] {
				Diagf("style", "%s: %s = %q  via %s (origin=%d imp=%v spec=%d.%d.%d)",
					elName, pn, cd.decl.ValueString(), cd.selector, cd.origin, cd.important,
					cd.specificity.A, cd.specificity.B, cd.specificity.C)
			}
		}
	}
	// ★ 两阶段应用（cascade 顺序）：
	//   阶段 1：先按级联顺序应用自定义属性（--xxx），再 resolveCustomProperties
	//           展开嵌套 var()，使 resolveVarInTokens 引用到最终级联值。
	//   阶段 2：普通属性按级联顺序逐个应用；应用前先把值里的 var() 展开。
	//           这保证了声明顺序被保留（如 `border` 后跟 `border-left`）。
	//           旧的 resolveVarInProperties 遍历 Go map（迭代顺序随机）逐个
	//           re-apply 含 var() 的属性：`border` 可能在 `border-left`
	//           之后处理，把 `border-left: 3px solid var(--accent)` 静默
	//           覆盖回 `border` 的 1px 灰色——折叠摘要条左侧蓝色竖条
	//           （border-left: 3px var(--accent)）随机消失的根因。
	for _, cd := range collected {
		if strings.HasPrefix(cd.decl.Name, "--") {
			applyDeclaration(cs, cd.decl)
		}
	}
	r.resolveCustomProperties(cs)

	for _, cd := range collected {
		if strings.HasPrefix(cd.decl.Name, "--") {
			continue
		}
		d := cd.decl
		if declContainsVar(d.Value) {
			d.Value = r.resolveVarInTokens(cs, d.Value, map[string]bool{})
		}
		applyDeclaration(cs, d)
	}

	// 兜底：展开剩余未展开的 var()（例如 applyScrollbarDeclarations 写入
	// Properties 的 -webkit-scrollbar-* 属性）。此时普通属性（border 系列
	// 等）已在阶段 2 按级联顺序展开为无 var() 的最终值，不再进入该循环，
	// 因此不会重演「map 顺序随机重放 border/border-left」的顺序 bug。
	r.resolveVarInProperties(cs)

	r.cache[el] = cachedStyle{cs: cs, ver: el.AttrVersion()}
	return cs
}

// declContainsVar reports whether the declaration value references a var()
// function at any token position (case-insensitive).
func declContainsVar(value []css.Token) bool {
	for _, t := range value {
		if t.Type == css.TokenFunction && strings.EqualFold(t.Value, "var") {
			return true
		}
	}
	return false
}

// ResolvePseudoElement computes the ComputedStyle for a ::before/::after
// pseudo-element of el, mirroring WebKit's pseudo-element style resolution
// (ElementRuleCollector + StyleResolver for pseudo-elements).
//
// Returns (style, content, ok): ok=false when no rule targets the
// pseudo-element (no box should be generated). The style inherits from the
// host element's computed style first, then applies matching declarations
// (e.g. `.switch .track::after { width:12px; ... }`).
func (r *Resolver) ResolvePseudoElement(el *dom.Element, pe css.PseudoElement) (*ComputedStyle, string, bool) {
	var collected []collectedDecl
	found := false
	for _, sheet := range r.sheets {
		bkt := r.sheetIndex[sheet]
		if bkt == nil {
			bkt = buildRuleBucket(sheet)
			r.sheetIndex[sheet] = bkt
		}
		order := 0
		scope := sheetScopeDepth(sheet)
		for _, ir := range bkt.candidates(el) {
			order = r.collectPseudoDeclarationsFromRule(ir.rule, sheet.Origin(), el, pe, &collected, order, scope)
		}
		// @media / @supports bodies are scanned fully (rare).
		for _, rule := range sheet.Rules() {
			switch v := rule.(type) {
			case *css.MediaRule:
				if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
					continue
				}
				order = r.collectPseudoDeclarations(v.Rules, sheet.Origin(), el, pe, &collected, order, scope)
			case *css.SupportsRule:
				order = r.collectPseudoDeclarations(v.Rules, sheet.Origin(), el, pe, &collected, order, scope)
			}
		}
		if !found && len(collected) > 0 {
			found = true
		}
	}
	if !found {
		return nil, "", false
	}

	cs := NewComputedStyle()
	// 伪元素从宿主元素继承（WebKit：伪元素继承宿主 computed style）。
	hostCS := r.ResolveElement(el)
	cs.InheritFrom(hostCS)

	// 排序并应用声明（与 ResolveElement 相同的级联逻辑）。
	sort.SliceStable(collected, func(i, j int) bool {
		a, b := collected[i], collected[j]
		ai := importanceRank(a.origin, a.important)
		bj := importanceRank(b.origin, b.important)
		if ai != bj {
			return ai < bj
		}
		if a.scope != b.scope {
			if a.important {
				return a.scope > b.scope
			}
			return a.scope < b.scope
		}
		if c := a.specificity.Compare(b.specificity); c != 0 {
			return c < 0
		}
		return a.sourceOrder < b.sourceOrder
	})
	for _, cd := range collected {
		if strings.HasPrefix(cd.decl.Name, "--") {
			applyDeclaration(cs, cd.decl)
		}
	}
	r.resolveCustomProperties(cs)

	for _, cd := range collected {
		if strings.HasPrefix(cd.decl.Name, "--") {
			continue
		}
		d := cd.decl
		if declContainsVar(d.Value) {
			d.Value = r.resolveVarInTokens(cs, d.Value, map[string]bool{})
		}
		applyDeclaration(cs, d)
	}

	// 兜底：展开剩余未展开的 var()（如 scrollbar 属性），见 ResolveElement 注释。
	r.resolveVarInProperties(cs)

	content := cs.GetProperty("content")
	return cs, content, true
}

// collectPseudoDeclarationsFromRule matches one StyleRule's selectors whose
// pseudo-element equals pe, appending matched declarations. Shared by the
// indexed and full-scan paths.
func (r *Resolver) collectPseudoDeclarationsFromRule(v *css.StyleRule, origin css.Origin, el *dom.Element, pe css.PseudoElement, collected *[]collectedDecl, order, scope int) int {
	if v.Selectors != nil {
		for _, sel := range v.Selectors.Selectors {
			if pseudoElementOf(&sel) != pe {
				continue
			}
			if !r.checker.Match(sel, el) {
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
					scope:       scope,
				})
				order++
			}
		}
	}
	if len(v.NestedRules) > 0 {
		order = r.collectPseudoDeclarations(v.NestedRules, origin, el, pe, collected, order, scope)
	}
	return order
}

// collectPseudoDeclarations 收集匹配「el + 伪元素 pe」的规则声明。
// 与 collectDeclarations 的区别：不跳过含伪元素的选择器，而是要求选择器的
// 伪元素恰好等于 pe（`X::after` 在解析 X 的 ::after 时收集）。
func (r *Resolver) collectPseudoDeclarations(rules []css.Rule, origin css.Origin, el *dom.Element, pe css.PseudoElement, collected *[]collectedDecl, baseOrder, scope int) int {
	order := baseOrder
	for _, rule := range rules {
		switch v := rule.(type) {
		case *css.StyleRule:
			order = r.collectPseudoDeclarationsFromRule(v, origin, el, pe, collected, order, scope)
		case *css.MediaRule:
			order = r.collectPseudoDeclarations(v.Rules, origin, el, pe, collected, order, scope)
		case *css.SupportsRule:
			order = r.collectPseudoDeclarations(v.Rules, origin, el, pe, collected, order, scope)
		}
	}
	return order
}

// pseudoElementOf 返回复杂选择器末尾的伪元素（若无则 PseudoElementUnknown）。
// 浏览器语义：伪元素必须位于选择器最后。
func pseudoElementOf(sel *css.ComplexSelector) css.PseudoElement {
	if sel == nil || len(sel.Compounds) == 0 {
		return css.PseudoElementUnknown
	}
	last := sel.Compounds[len(sel.Compounds)-1]
	if len(last.Selectors) == 0 {
		return css.PseudoElementUnknown
	}
	s := last.Selectors[len(last.Selectors)-1]
	if s.Match == css.MatchPseudoElement {
		return s.PseudoElem
	}
	return css.PseudoElementUnknown
}

// ─── rule index (fast candidate lookup) ───────────────────

// complexSelectorKeys returns the index keys for one complex selector: the
// tag/id/class of its RIGHTMOST compound. A selector whose rightmost compound
// has none of those (pure :hover, attribute selector, :not(...), pseudo-element,
// universal) returns nil — such selectors match elements regardless of their
// tag/class, so their rule must live in the universal bucket.
func complexSelectorKeys(cs css.ComplexSelector) map[string]bool {
	if len(cs.Compounds) == 0 {
		return nil
	}
	last := cs.Compounds[len(cs.Compounds)-1]
	keys := map[string]bool{}
	for _, s := range last.Selectors {
		switch s.Match {
		case css.MatchTag:
			if s.Value == "*" {
				return nil // universal selector matches any element
			}
			keys[s.Value] = true
		case css.MatchID:
			keys["#"+s.Value] = true
		case css.MatchClass:
			keys["."+s.Value] = true
		case css.MatchPseudoClass:
			if s.SelectorList != nil {
				return nil // :not(...) / :is(...) / :has(...) — be conservative
			}
			// :hover / :first-child etc. carry no key by themselves; if the
			// same compound also has a class (e.g. .a:hover) that key applies.
		case css.MatchSet, css.MatchExact, css.MatchList, css.MatchHyphen,
			css.MatchBegin, css.MatchEnd, css.MatchContain:
			// Attribute selectors (very common in Vue scoped CSS:
			// `.msg-item[data-v-xxxx]`) match by element attribute, which the
			// checker evaluates — they don't contribute an index key but must
			// NOT disable the class/id/tag key of the same compound.
		default:
			// pseudo-element selectors — no safe key
			return nil
		}
	}
	return keys
}

// selectorListKeys returns the union of keys across a selector list, or nil
// when ANY selector is keyless — a single keyless selector (e.g. `a:hover`'s
// sibling `*`) makes the whole rule match every element, so the rule must be
// matched unconditionally (universal bucket) for correctness.
func selectorListKeys(list *css.SelectorList) []string {
	if list == nil {
		return nil
	}
	all := map[string]bool{}
	for _, cs := range list.Selectors {
		keys := complexSelectorKeys(cs)
		if len(keys) == 0 {
			return nil
		}
		for k := range keys {
			all[k] = true
		}
	}
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	return out
}

// buildRuleBucket indexes the top-level StyleRules of a sheet, recording each
// rule's declaration-stream position so cascade source order stays correct.
// Rules inside @media/@supports stay out of the index (handled by full
// recursion) but still consume stream positions. Rules whose selector list
// contains any keyless selector go to the universal bucket.
func buildRuleBucket(sheet *css.CSSStyleSheet) *ruleBucket {
	b := &ruleBucket{byKey: map[string][]indexedRule{}}
	declCount := 0
	var countDecls func(rules []css.Rule)
	countDecls = func(rules []css.Rule) {
		for _, rule := range rules {
			if sr, ok := rule.(*css.StyleRule); ok {
				declCount += len(sr.Declarations)
			}
		}
	}
	for _, rule := range sheet.Rules() {
		sr, ok := rule.(*css.StyleRule)
		if !ok {
			countDecls([]css.Rule{rule})
			continue
		}
		ir := indexedRule{rule: sr, order: declCount}
		declCount += len(sr.Declarations)
		keys := selectorListKeys(sr.Selectors)
		if len(keys) == 0 {
			b.universal = append(b.universal, ir)
			continue
		}
		for _, k := range keys {
			b.byKey[k] = append(b.byKey[k], ir)
		}
	}
	return b
}

// candidates returns the candidate rules for an element: union of the buckets
// for its tag / id / each class, plus the universal bucket, deduped. Keeps the
// order of first encounter (universal first, then tag/id/class buckets).
func (b *ruleBucket) candidates(el *dom.Element) []indexedRule {
	seen := make(map[*css.StyleRule]bool)
	out := make([]indexedRule, 0, 16)
	add := func(rs []indexedRule) {
		for _, ir := range rs {
			if !seen[ir.rule] {
				seen[ir.rule] = true
				out = append(out, ir)
			}
		}
	}
	add(b.universal)
	if el != nil {
		if t := el.LocalName(); t != "" {
			add(b.byKey[t])
		}
		if id := el.GetAttribute("id"); id != "" {
			add(b.byKey["#"+id])
		}
		if cn := el.ClassName(); cn != "" {
			for _, c := range strings.Fields(cn) {
				add(b.byKey["."+c])
			}
		}
	}
	return out
}

// collectSheetDeclarations collects cascade declarations for el from one sheet
// using the rule index. Non-indexed containers (@media / @supports) fall back
// to the original full scan; the stream order continues from the indexed rules
// so the cascade's source-order comparison stays faithful to the CSS source.
func (r *Resolver) collectSheetDeclarations(sheet *css.CSSStyleSheet, el *dom.Element, collected *[]collectedDecl) {
	// ★ 样式作用域隔离（CSS Scoping）：UA sheet 全局作用；author sheet 只在相同
	// 的 scoping root 内作用——文档级 author sheet 不匹配 shadow tree 内元素，
	// shadow tree 内 author sheet 也不匹配文档/其它 shadow tree 的元素。这防止
	// shadow 内的 <style> 泄漏到全局、也防止全局样式穿透进 shadow tree。
	if sheet.Origin() != css.OriginUserAgent {
		if sheetScopingRoot(sheet) != dom.ContainingShadowRoot(el) {
			return
		}
	}
	bkt := r.sheetIndex[sheet]
	if bkt == nil {
		bkt = buildRuleBucket(sheet)
		r.sheetIndex[sheet] = bkt
	}
	origin := sheet.Origin()
	scope := sheetScopeDepth(sheet)
	cands := bkt.candidates(el)
	// Collect in CSS source order (not bucket order) so the cascade's
	// source-order comparison matches the full scan exactly; sourceOrder is
	// the matched-declaration counter (same as the original implementation).
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].order < cands[j].order })
	order := 0
	for _, ir := range cands {
		order = r.collectFromStyleRule(ir.rule, origin, el, collected, order, scope)
	}
	// @media / @supports rule bodies are scanned fully (their inner StyleRules
	// are not in the index — they are few); their stream positions continue
	// after the top-level matched rules.
	for _, rule := range sheet.Rules() {
		switch v := rule.(type) {
		case *css.MediaRule:
			if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
				continue
			}
			order = r.collectDeclarations(v.Rules, origin, el, collected, order, scope)
		case *css.SupportsRule:
			order = r.collectDeclarations(v.Rules, origin, el, collected, order, scope)
		}
	}
}

// collectFromStyleRule matches one StyleRule's selectors against el and appends
// matched declarations; recurses into nested rules (CSS nesting) afterwards.
// This is the per-rule body shared by the indexed and full-scan paths.
func (r *Resolver) collectFromStyleRule(v *css.StyleRule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, order, scope int) int {
	if v.Selectors != nil {
		for _, sel := range v.Selectors.Selectors {
			if r.checker.Match(sel, el) {
				// Skip selectors that consist ONLY of pseudo-elements
				// (e.g. ::selection, ::-webkit-scrollbar-thumb),
				// OR selectors that CONTAIN pseudo-elements
				// (e.g. .clearfix::after).
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
						selector:    sel.String(),
						scope:       scope,
					})
					order++
				}
			}
		}
	}
	if len(v.NestedRules) > 0 {
		order = r.collectDeclarations(v.NestedRules, origin, el, collected, order, scope)
	}
	return order
}

// collectDeclarations walks a rule list, recursing into @media / @supports rules,
// and appends matching declarations to collected with their cascade metadata.
// Used for non-indexed containers (media/supports bodies) and nested rules.
func (r *Resolver) collectDeclarations(rules []css.Rule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, baseOrder, scope int) int {
	order := baseOrder
	for _, rule := range rules {
		switch v := rule.(type) {
		case *css.StyleRule:
			order = r.collectFromStyleRule(v, origin, el, collected, order, scope)
		case *css.MediaRule:
			// Evaluate media queries against the current device/viewport context.
			// If the parsed query list is empty (parse error or unsupported syntax),
			// fall through to always-include to match the pre-existing behaviour.
			if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
				continue // skip rules inside non-matching @media
			}
			order = r.collectDeclarations(v.Rules, origin, el, collected, order, scope)
		case *css.SupportsRule:
			// Supports is treated as always-true in this port.
			order = r.collectDeclarations(v.Rules, origin, el, collected, order, scope)
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

// ─── cross-shadow-boundary cascade origins (CSS Scoping Level 1) ───

// collectScopedFromRules walks a rule list and appends declarations whose selectors
// satisfy the given predicate (match) AND match el via the checker. It is the shared
// traversal for :host / ::slotted / ::part routing — these rules target an element on
// the far side of a shadow boundary from their stylesheet, so they bypass the normal
// scoping-root isolation (and the hasPseudoElement skip) that collectSheetDeclarations
// applies.
func (r *Resolver) collectScopedFromRules(rules []css.Rule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, baseOrder, scope int, match func(*css.ComplexSelector) bool) int {
	order := baseOrder
	for _, rule := range rules {
		switch v := rule.(type) {
		case *css.StyleRule:
			if v.Selectors != nil {
				for _, sel := range v.Selectors.Selectors {
					if !match(&sel) {
						continue
					}
					if !r.checker.Match(sel, el) {
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
							selector:    sel.String(),
							scope:       scope,
						})
						order++
					}
				}
			}
			if len(v.NestedRules) > 0 {
				order = r.collectScopedFromRules(v.NestedRules, origin, el, collected, order, scope, match)
			}
		case *css.MediaRule:
			if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
				continue
			}
			order = r.collectScopedFromRules(v.Rules, origin, el, collected, order, scope, match)
		case *css.SupportsRule:
			order = r.collectScopedFromRules(v.Rules, origin, el, collected, order, scope, match)
		}
	}
	return order
}

// matchHostSelector reports whether the complex selector contains a :host or
// :host-context pseudo-class.
func matchHostSelector(sel *css.ComplexSelector) bool {
	if sel == nil {
		return false
	}
	for _, comp := range sel.Compounds {
		for _, s := range comp.Selectors {
			if s.Match == css.MatchPseudoClass &&
				(s.PseudoClass == css.PseudoClassHost || s.PseudoClass == css.PseudoClassHostContext) {
				return true
			}
		}
	}
	return false
}

// matchSlottedSelector reports whether the complex selector contains a ::slotted
// pseudo-element.
func matchSlottedSelector(sel *css.ComplexSelector) bool {
	if sel == nil {
		return false
	}
	for _, comp := range sel.Compounds {
		for _, s := range comp.Selectors {
			if s.Match == css.MatchPseudoElement && s.PseudoElem == css.PseudoElementSlotted {
				return true
			}
		}
	}
	return false
}

// matchPartSelector reports whether the complex selector contains a ::part
// pseudo-element.
func matchPartSelector(sel *css.ComplexSelector) bool {
	if sel == nil {
		return false
	}
	for _, comp := range sel.Compounds {
		for _, s := range comp.Selectors {
			if s.Match == css.MatchPseudoElement && s.PseudoElem == css.PseudoElementPart {
				return true
			}
		}
	}
	return false
}

// collectShadowHostDeclarations routes :host / :host-context rules to the shadow host.
// A sheet inside el's shadow tree (scoping root's host == el) contributes its :host
// rules to el. The scope is the shadow tree's depth, so :host rules outrank the host's
// own document-tree rules (normal declarations).
func (r *Resolver) collectShadowHostDeclarations(sheet *css.CSSStyleSheet, el *dom.Element, collected *[]collectedDecl) {
	if sheet.Origin() == css.OriginUserAgent || !el.HasShadowRoot() {
		return
	}
	sr := sheetScopingRoot(sheet)
	if sr == nil || sr.Host() != el {
		return
	}
	scope := sr.TreeScopeDepth()
	order := 0
	order = r.collectScopedFromRules(sheet.Rules(), sheet.Origin(), el, collected, order, scope, matchHostSelector)
}

// collectSlottedDeclarations routes ::slotted rules to slot-assigned light-DOM nodes.
// When el is a light-DOM child of a shadow host assigned to a slot, the host's shadow
// tree's sheets contribute their ::slotted rules to el (scope = shadow depth).
func (r *Resolver) collectSlottedDeclarations(sheet *css.CSSStyleSheet, el *dom.Element, collected *[]collectedDecl) {
	if sheet.Origin() == css.OriginUserAgent {
		return
	}
	if el.AssignedSlot() == nil {
		return
	}
	parent := el.ParentNode()
	host, ok := parent.(*dom.Element)
	if !ok {
		return
	}
	sr := sheetScopingRoot(sheet)
	if sr == nil || sr.Host() != host {
		return
	}
	scope := sr.TreeScopeDepth()
	order := 0
	order = r.collectScopedFromRules(sheet.Rules(), sheet.Origin(), el, collected, order, scope, matchSlottedSelector)
}

// collectPartDeclarations routes ::part rules from an outer scope to shadow-tree
// elements. When el lives inside a shadow tree and carries a `part` attribute, sheets
// from a shallower scope (document tree, or an ancestor shadow tree) contribute their
// ::part rules to el. The declaration scope stays the *sheet's* scope, so a document-
// level ::part rule (scope 0) loses to the element's own shadow rules (scope N) for
// normal declarations.
func (r *Resolver) collectPartDeclarations(sheet *css.CSSStyleSheet, el *dom.Element, collected *[]collectedDecl) {
	if sheet.Origin() == css.OriginUserAgent {
		return
	}
	if len(el.PartNames()) == 0 {
		return
	}
	elSR := dom.ContainingShadowRoot(el)
	if elSR == nil {
		return
	}
	sr := sheetScopingRoot(sheet)
	if sr == elSR {
		// The sheet lives in the same shadow tree as el: ::part only styles elements
		// from an *outer* scope.
		return
	}
	scope := sheetScopeDepth(sheet)
	order := 0
	order = r.collectScopedFromRules(sheet.Rules(), sheet.Origin(), el, collected, order, scope, matchPartSelector)
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
	case "inset":
		// Shorthand for top/right/bottom/left (CSS Positioned Layout).
		top, right, bottom, left := parseEdgeShorthand(valueString)
		cs.SetProperty("top", top.String())
		cs.SetProperty("right", right.String())
		cs.SetProperty("bottom", bottom.String())
		cs.SetProperty("left", left.String())
	case "color":
		if c, ok := parseColor(valueString); ok {
			cs.Color = c
		}
	case "background-color":
		if c, ok := parseColor(valueString); ok {
			cs.BackgroundColor = c
		}
	case "background":
		// Shorthand: per CSS Backgrounds-3, resets ALL sub-properties to
		// their initial values, then applies the specified ones. In
		// particular a value carrying no color (e.g. "background: none")
		// resets background-color to transparent — otherwise the UA button
		// face (#f0f0f0 gradient/color) leaks through styled buttons.
		cs.BackgroundColor = Color{}
		cs.BackgroundImage = ""
		cs.BackgroundPosition = ""
		cs.BackgroundSize = ""
		cs.BackgroundRepeat = ""
		var grads []string
		for _, p := range splitShorthandValue(valueString) {
			if strings.HasPrefix(p, "linear-gradient(") || strings.HasPrefix(p, "radial-gradient(") || strings.HasPrefix(p, "url(") {
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
			pos, size := parseBgLayerPosSize(splitShorthandValue(valueString))
			if pos != "" {
				cs.BackgroundPosition = pos
			}
			if size != "" {
				cs.BackgroundSize = size
			}
		}
	case "font":
		// ★ font 简写（浏览器标准）：
		//   font: [style] [variant] [weight] [stretch]? size[/line-height] family
		// 此前走 default 存 raw string → 组件内联 `font: 13px/1.4 monospace`
		// 完全不生效（继承默认字体）。展开到各子属性。
		if st, v, wt, sz, lh, fam, ok := parseFontShorthand(valueString); ok {
			if fam != "" {
				cs.FontFamily = strings.Trim(fam, `"'`)
			}
			if sz != "" {
				if l, ok2 := parseLength(sz); ok2 {
					cs.FontSize = l
				}
			}
			if wt != "" {
				cs.FontWeight = wt
			}
			if st != "" {
				cs.FontStyle = st
			}
			if v != "" {
				cs.FontVariant = v
			}
			if lh != "" {
				if strings.EqualFold(strings.TrimSpace(lh), "normal") {
					cs.LineHeight = Length{Value: 0, Unit: "normal"}
				} else if l, ok2 := parseLength(lh); ok2 {
					cs.LineHeight = l
				}
			}
		}
	case "font-family":
		if valueString == "inherit" {
			// UA stylesheet uses font-family: inherit for form controls;
			// the inherited value was already copied from the parent in
			// ResolveElement via InheritFrom — keep it (do not overwrite
			// with the literal keyword).
			break
		}
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
		if strings.EqualFold(strings.TrimSpace(valueString), "normal") {
			// ★ 浏览器语义：line-height: normal = 字体度量行高
			// （ascent+descent+lineGap），非 1.2×font-size。xterm 的
			// .xterm-rows / .xterm-char-measure-element 用 normal，
			// 之前落默认 1.2 → 行高 16px vs 浏览器 15px。用 Unit
			// "normal" 标记，由 cssLineHeight 返回 0（走字体度量）。
			cs.LineHeight = Length{Value: 0, Unit: "normal"}
		} else if l, ok := parseLength(valueString); ok {
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
	case "word-break":
		cs.WordBreak = valueString
		cs.SetProperty("word-break", valueString)
	case "overflow-wrap", "word-wrap":
		cs.OverflowWrap = valueString
		cs.SetProperty("overflow-wrap", valueString)
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
		w, s, c, cset, ok := parseBorderShorthand(valueString)
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
			cs.BorderTopColorSet = cset
			cs.BorderRightColorSet = cset
			cs.BorderBottomColorSet = cset
			cs.BorderLeftColorSet = cset
		}
	case "outline":
		if w, s, c, ok := parseOutlineShorthand(valueString); ok {
			cs.OutlineWidth = w
			cs.OutlineStyle = s
			cs.OutlineColor = c
			cs.OutlineSet = true
		} else if strings.TrimSpace(valueString) == "none" {
			cs.OutlineWidth = Length{Value: 0, Unit: "px"}
			cs.OutlineStyle = "none"
			cs.OutlineSet = true
		}
	case "outline-width":
		if l, ok := parseLength(valueString); ok {
			cs.OutlineWidth = l
			cs.OutlineSet = true
		}
	case "outline-style":
		s := strings.TrimSpace(valueString)
		if s != "" {
			cs.OutlineStyle = s
			cs.OutlineSet = true
		}
	case "outline-color":
		if c, ok := parseColor(valueString); ok {
			cs.OutlineColor = c
			cs.OutlineSet = true
		}
	case "border-top":
		if w, s, c, cset, ok := parseBorderShorthand(valueString); ok {
			cs.BorderTopWidth = w
			cs.BorderTopStyle = s
			cs.BorderTopColor = c
			cs.BorderTopColorSet = cset
		}
	case "border-right":
		if w, s, c, cset, ok := parseBorderShorthand(valueString); ok {
			cs.BorderRightWidth = w
			cs.BorderRightStyle = s
			cs.BorderRightColor = c
			cs.BorderRightColorSet = cset
		}
	case "border-bottom":
		if w, s, c, cset, ok := parseBorderShorthand(valueString); ok {
			cs.BorderBottomWidth = w
			cs.BorderBottomStyle = s
			cs.BorderBottomColor = c
			cs.BorderBottomColorSet = cset
		}
	case "border-left":
		if w, s, c, cset, ok := parseBorderShorthand(valueString); ok {
			cs.BorderLeftWidth = w
			cs.BorderLeftStyle = s
			cs.BorderLeftColor = c
			cs.BorderLeftColorSet = cset
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
	case "border-color":
		// Shorthand: 1-4 colors → top/right/bottom/left (CSS box model §8.5).
		if colors, ok := parseBorderColorShorthand(valueString); ok {
			switch len(colors) {
			case 1:
				cs.BorderTopColor, cs.BorderRightColor = colors[0], colors[0]
				cs.BorderBottomColor, cs.BorderLeftColor = colors[0], colors[0]
				cs.BorderTopColorSet, cs.BorderRightColorSet = true, true
				cs.BorderBottomColorSet, cs.BorderLeftColorSet = true, true
			case 2:
				cs.BorderTopColor, cs.BorderBottomColor = colors[0], colors[1]
				cs.BorderRightColor, cs.BorderLeftColor = colors[0], colors[1]
				cs.BorderTopColorSet, cs.BorderBottomColorSet = true, true
				cs.BorderRightColorSet, cs.BorderLeftColorSet = true, true
			case 3:
				cs.BorderTopColor = colors[0]
				cs.BorderRightColor, cs.BorderLeftColor = colors[1], colors[1]
				cs.BorderBottomColor = colors[2]
				cs.BorderTopColorSet, cs.BorderBottomColorSet = true, true
				cs.BorderRightColorSet, cs.BorderLeftColorSet = true, true
			case 4:
				cs.BorderTopColor, cs.BorderRightColor = colors[0], colors[1]
				cs.BorderBottomColor, cs.BorderLeftColor = colors[2], colors[3]
				cs.BorderTopColorSet, cs.BorderRightColorSet = true, true
				cs.BorderBottomColorSet, cs.BorderLeftColorSet = true, true
			}
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
			cs.StaticOpacity = v
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
			// A single bare word is a named grid area; store it in Properties
			// so the grid formatter can place the element into the matching
			// cell of the container's grid-template-areas. (Do NOT write it
			// into GridTemplateAreas — that field belongs to the container.)
			cs.SetProperty("grid-area", strings.TrimSpace(parts[0]))
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

// parseBorderColorShorthand parses the border-color shorthand (1-4 colors).
func parseBorderColorShorthand(s string) ([]Color, bool) {
	// ★ 括号感知分割（splitShorthandValue）：rgba()/hsl() 函数内部有空格
	// （ValueString 序列化逗号后加空格，如 "rgba(74, 128, 232, 0.35)"），
	// strings.Fields 会把它拆成多段 → parseColor("rgba(74,") 失败 →
	// border-color:rgba(...) 简写整体不生效（.wbox.off 半透明边框色
	// 未覆盖 .wbox 的 border 简写色的根因）。
	parts := splitShorthandValue(s)
	if len(parts) == 0 || len(parts) > 4 {
		return nil, false
	}
	colors := make([]Color, 0, len(parts))
	for _, p := range parts {
		c, ok := parseColor(p)
		if !ok {
			return nil, false
		}
		colors = append(colors, c)
	}
	return colors, true
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
	// Handle math functions: calc(), min(), max(), clamp().
	if name, full, ok := mathFuncInfoS(s); ok {
		expr := full
		if name == "calc" {
			// calc 保持既有语义：CalcExpr 存内部表达式。
			expr = extractCalcArgS(s)
		}
		// ★ 含相对单位（%、em、rem、vw、vh、vmin、vmax）的表达式无法在
		// style resolve 阶段求值——此时不知道包含块尺寸 / font-size /
		// viewport。用空 CalcContext 求值会把 % 解析为 0（ParentWidth=0），
		// 导致 calc(100% - 40px) 错误地 = -40px（现代 SPA 侧边栏/编辑器
		// 布局大量使用该模式，宽度会被压成 0）。保留 calc 标记 + 原始
		// 表达式，让布局引擎带真实 context 重新求值（resolveLength 的
		// "calc" case）。
		if css.CalcHasRelativeUnit(expr) {
			return Length{Unit: "calc", CalcExpr: expr}, true
		}
		// 纯绝对单位（px/pt/cm/mm/in）：无需 context，立即求值。
		v, err := css.EvalCalcString(expr, css.CalcContext{})
		if err == nil {
			return Length{Value: v, Unit: "px"}, true
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
	unit := s[i:]
	if unit == "" && num == 0 {
		// CSS: 无单位的 0 是合法长度，等价 0px（如 .tri 的 width:0;height:0
		// 边框三角形技巧）。若不归一化为 px，resolveLengthAuto 会把显式
		// 0 误判为 auto（shrink-to-fit），三角尺寸被 border 撑大。
		unit = "px"
	}
	return Length{Value: num, Unit: unit}, true
}

// mathFuncInfoS detects a CSS math function prefix (calc/min/max/clamp) and
// returns its lowercased name plus the full balanced function expression
// (e.g. "min(100%, 600px)"), with ok=true. Used by parseLength to treat
// min()/max()/clamp() the same as calc().
func mathFuncInfoS(s string) (name, full string, ok bool) {
	s = strings.TrimSpace(s)
	for _, n := range []string{"calc", "min", "max", "clamp"} {
		if len(s) < len(n)+1 || !strings.EqualFold(s[:len(n)], n) || s[len(n)] != '(' {
			continue
		}
		depth := 0
		for i := len(n); i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return n, s[:i+1], true
				}
			}
		}
		return n, s, true
	}
	return "", "", false
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

// fontSizeTokenRe 匹配 font 简写里的字号 token：13px、13px/1.4、12pt/1.5em 等。
var fontSizeTokenRe = regexp.MustCompile(`^([0-9]*\.?[0-9]+(?:px|em|rem|pt|%|vh|vw|vmin|vmax))(?:/([0-9]*\.?[0-9]*(?:px|em|rem|pt|%)?))?$`)

// parseFontShorthand 解析 CSS font 简写（浏览器标准）：
//
//	font: [ <font-style> || <font-variant> || <font-weight> || <font-stretch> ]?
//	      <font-size> [ / <line-height> ]? <font-family>
//
// 返回展开的 (style, variant, weight, size, lineHeight, family)。family 可含
// 空格（如 "Times New Roman"），取 size 后的剩余部分；关键字按标准归类。
func parseFontShorthand(s string) (style, variant, weight, size, lineHeight, family string, ok bool) {
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return
	}
	// 1) 找 size token（可能含 /line-height 或后随独立 /lh token）
	sizeIdx := -1
	for i, tok := range tokens {
		if m := fontSizeTokenRe.FindStringSubmatch(tok); m != nil {
			sizeIdx = i
			size = m[1]
			if m[2] != "" {
				lineHeight = m[2]
			}
			break
		}
	}
	if sizeIdx < 0 {
		return
	}
	// 2) size 前：style / variant / weight / stretch 关键字
	for _, tok := range tokens[:sizeIdx] {
		switch strings.ToLower(tok) {
		case "italic", "oblique":
			style = strings.ToLower(tok)
		case "small-caps":
			variant = "small-caps"
		case "bold", "bolder", "lighter":
			weight = strings.ToLower(tok)
		case "normal":
			// normal 既可能是 font-style 也可能是 font-weight，取缺省
			if style == "" {
				style = "normal"
			}
		default:
			if n, err := strconv.Atoi(tok); err == nil && n >= 100 && n <= 900 && n%100 == 0 {
				weight = tok
			}
		}
	}
	// 3) size 后：独立 /lh token 或 family
	rest := tokens[sizeIdx+1:]
	if lineHeight == "" && len(rest) > 0 && strings.HasPrefix(rest[0], "/") {
		lineHeight = strings.TrimPrefix(rest[0], "/")
		rest = rest[1:]
	}
	family = strings.Join(rest, " ")
	ok = true
	return
}

// parseEdgeShorthand parses a 1-to-4 value edge shorthand like "padding: 10px 20px"
// or "margin: 1 2 3 4" and returns (top, right, bottom, left). Mirrors the CSS
// "Edge value shorthand" expansion rules.
func parseEdgeShorthand(s string) (top, right, bottom, left Length) {	parts := strings.Fields(s)
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

// parseBgLayerPosSize extracts the first background layer's position and
// size from a shorthand's whitespace-split parts, e.g.
//
//	["linear-gradient(...)", "0", "0/70px", "70px", "no-repeat"]
//	  → pos "0 0", size "70px 70px"
//
// The CSS tokenizer serializes "/" as its own part, so the slash may also
// appear standalone: ["0", "0", "/", "70px", "70px"]. Both shapes are
// handled. Returns "" for missing components.
func parseBgLayerPosSize(parts []string) (pos, size string) {
	isImage := func(s string) bool {
		return strings.HasPrefix(s, "linear-gradient(") || strings.HasPrefix(s, "radial-gradient(") || strings.HasPrefix(s, "url(")
	}
	isRepeat := func(s string) bool {
		switch s {
		case "repeat", "no-repeat", "repeat-x", "repeat-y", "space", "round":
			return true
		}
		return false
	}
	isValue := func(s string) bool { return s != "" && s != "/" && !isImage(s) && !isRepeat(s) }
	for i, p := range parts {
		j := strings.IndexByte(p, '/')
		if j < 0 {
			continue
		}
		var posVals, sizeVals []string
		if p == "/" {
			// Standalone slash: position = parts[i-2], parts[i-1];
			// size = parts[i+1], parts[i+2].
			if i >= 2 && isValue(parts[i-2]) {
				posVals = append(posVals, parts[i-2])
			}
			if i >= 1 && isValue(parts[i-1]) {
				posVals = append(posVals, parts[i-1])
			}
			if i+1 < len(parts) && isValue(parts[i+1]) {
				sizeVals = append(sizeVals, parts[i+1])
			}
			if i+2 < len(parts) && isValue(parts[i+2]) {
				sizeVals = append(sizeVals, parts[i+2])
			}
		} else {
			// Merged shape "0/70px": position = parts[i-1] + left,
			// size = right + parts[i+1].
			left := strings.TrimSpace(p[:j])
			right := strings.TrimSpace(p[j+1:])
			if i >= 1 && isValue(parts[i-1]) {
				posVals = append(posVals, parts[i-1])
			}
			if left != "" {
				posVals = append(posVals, left)
			}
			if right != "" {
				sizeVals = append(sizeVals, right)
			}
			if i+1 < len(parts) && isValue(parts[i+1]) {
				sizeVals = append(sizeVals, parts[i+1])
			}
		}
		if len(posVals) > 0 {
			pos = strings.Join(posVals, " ")
		}
		if len(sizeVals) > 0 {
			size = strings.Join(sizeVals, " ")
		}
		return pos, size
	}
	return "", ""
}

// parseBorderShorthand parses a border shorthand value like "1px solid #e5e7eb" and
// returns (width, style, color, ok). Components may appear in any order; missing
// components are left as their zero value.
func parseBorderShorthand(s string) (width Length, style string, color Color, colorSet bool, ok bool) {
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
		// Try color (e.g. "#e5e7eb", "red", "rgb(...)", "transparent").
		if c, cOK := parseColor(p); cOK {
			color = c
			colorSet = true
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

// parseOutlineShorthand 解析 outline 简写（与 border 简写语法相同：
// width style color 任意顺序）。outline 默认样式为 none。
func parseOutlineShorthand(s string) (width Length, style string, color Color, ok bool) {
	w, st, c, _, bok := parseBorderShorthand(s)
	if !bok {
		return
	}
	if st == "" {
		st = "solid" // 浏览器默认 outline-style: none；显式 width 时常用 solid
	}
	return w, st, c, true
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
	isPct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	v, _ := strconv.ParseFloat(s, 64)
	if isPct {
		// "50%" → 127
		v = v * 255 / 100
	} else if v <= 1 {
		// 0-1 float (rgba(…, 0.15)) → 0-255
		v = v * 255
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
			// Find the matching closing right parenthesis. The tokenizer
			// does NOT emit a TokenLeftParenthesis for function calls — the
			// opening paren is implied by TokenFunction (consumeIdentLikeToken
			// consumes the '(' and returns a single TokenFunction). Nested
			// functions (e.g. var(--a, var(--b)) or var(--a, rgb(1,2,3)))
			// therefore need depth counted per TokenFunction, not per
			// TokenLeftParenthesis: with the old depth-0 logic the FIRST
			// ')' — the inner var()'s closing paren — broke the scan, the
			// fallback kept a dangling ')' in its tokens, and the resolved
			// value gained a trailing TokenRightParenthesis that made
			// parseBorderShorthand/parseColor fail (border-bottom came out
			// transparent for `var(--border-subtle, var(--border-color))`).
			depth := 0
			j := i + 1
			var args []css.Token
			for ; j < len(tokens); j++ {
				tj := tokens[j]
				if tj.Type == css.TokenFunction {
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
	// 即使没有任何自定义属性也要解析 var()——未定义变量的 fallback 值
	// （如 var(--undefined, blue)）仍应替换。只有属性值里完全没有 var(
	// 才跳过。
	hasVar := false
	for _, raw := range cs.Properties {
		if strings.Contains(raw, "var(") {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return
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
			if resolvedStr != "inherit" {
				cs.FontFamily = strings.Trim(resolvedStr, `"'`)
			}
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
				if strings.HasPrefix(p, "linear-gradient(") || strings.HasPrefix(p, "radial-gradient(") || strings.HasPrefix(p, "url(") {
					grads = append(grads, p)
					continue				}
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
		case "border-color":
			if colors, ok := parseBorderColorShorthand(resolvedStr); ok {
				switch len(colors) {
				case 1:
					cs.BorderTopColor, cs.BorderRightColor = colors[0], colors[0]
					cs.BorderBottomColor, cs.BorderLeftColor = colors[0], colors[0]
					cs.BorderTopColorSet, cs.BorderRightColorSet = true, true
					cs.BorderBottomColorSet, cs.BorderLeftColorSet = true, true
				case 2:
					cs.BorderTopColor, cs.BorderBottomColor = colors[0], colors[1]
					cs.BorderRightColor, cs.BorderLeftColor = colors[0], colors[1]
					cs.BorderTopColorSet, cs.BorderBottomColorSet = true, true
					cs.BorderRightColorSet, cs.BorderLeftColorSet = true, true
				case 3:
					cs.BorderTopColor = colors[0]
					cs.BorderRightColor, cs.BorderLeftColor = colors[1], colors[1]
					cs.BorderBottomColor = colors[2]
					cs.BorderTopColorSet, cs.BorderBottomColorSet = true, true
					cs.BorderRightColorSet, cs.BorderLeftColorSet = true, true
				case 4:
					cs.BorderTopColor, cs.BorderRightColor = colors[0], colors[1]
					cs.BorderBottomColor, cs.BorderLeftColor = colors[2], colors[3]
					cs.BorderTopColorSet, cs.BorderRightColorSet = true, true
					cs.BorderBottomColorSet, cs.BorderLeftColorSet = true, true
				}
			}
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
		case "border":
			if w, s, c, cset, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderTopWidth, cs.BorderRightWidth = w, w
				cs.BorderBottomWidth, cs.BorderLeftWidth = w, w
				cs.BorderTopStyle, cs.BorderRightStyle = s, s
				cs.BorderBottomStyle, cs.BorderLeftStyle = s, s
				// 颜色：仅当没有更具体的 border-color / border-*-color
				// 声明时由简写设置。Properties 遍历是 map 顺序（随机），
				// 若简写后处理会把已解析的 accent 色覆盖回默认灰。
				// 注意：必须查 Properties map 本身（GetProperty 对
				// border-*-color 有 typed fallback，恒非空）。
				_, hasBColor := cs.Properties["border-color"]
				_, hasBTop := cs.Properties["border-top-color"]
				_, hasBRight := cs.Properties["border-right-color"]
				_, hasBBottom := cs.Properties["border-bottom-color"]
				_, hasBLeft := cs.Properties["border-left-color"]
				if !hasBColor && !hasBTop && !hasBRight && !hasBBottom && !hasBLeft {
					cs.BorderTopColor, cs.BorderRightColor = c, c
					cs.BorderBottomColor, cs.BorderLeftColor = c, c
					cs.BorderTopColorSet, cs.BorderRightColorSet = cset, cset
					cs.BorderBottomColorSet, cs.BorderLeftColorSet = cset, cset
				}
			}
		case "border-top":
			if w, s, c, cset, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderTopWidth, cs.BorderTopStyle, cs.BorderTopColor = w, s, c
				cs.BorderTopColorSet = cset
			}
		case "border-right":
			if w, s, c, cset, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderRightWidth, cs.BorderRightStyle, cs.BorderRightColor = w, s, c
				cs.BorderRightColorSet = cset
			}
		case "border-bottom":
			if w, s, c, cset, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderBottomWidth, cs.BorderBottomStyle, cs.BorderBottomColor = w, s, c
				cs.BorderBottomColorSet = cset
			}
		case "border-left":
			if w, s, c, cset, ok := parseBorderShorthand(resolvedStr); ok {
				cs.BorderLeftWidth, cs.BorderLeftStyle, cs.BorderLeftColor = w, s, c
				cs.BorderLeftColorSet = cset
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
		case "outline":
			if w, s, c, ok := parseOutlineShorthand(resolvedStr); ok {
				cs.OutlineWidth, cs.OutlineStyle, cs.OutlineColor = w, s, c
				cs.OutlineSet = true
			} else if strings.TrimSpace(resolvedStr) == "none" {
				cs.OutlineWidth = Length{Value: 0, Unit: "px"}
				cs.OutlineStyle = "none"
				cs.OutlineSet = true
			}
		case "outline-color":
			if c, ok := parseColor(resolvedStr); ok {
				cs.OutlineColor = c
				cs.OutlineSet = true
			}
		case "outline-width":
			if l, ok := parseLength(resolvedStr); ok {
				cs.OutlineWidth = l
				cs.OutlineSet = true
			}
		case "outline-style":
			s := strings.TrimSpace(resolvedStr)
			if s != "" {
				cs.OutlineStyle = s
				cs.OutlineSet = true
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
// webkitScrollbarPseudo returns which ::-webkit-scrollbar pseudo-element the
// complex selector's LAST compound targets: 1=scrollbar-thumb, 2=scrollbar-track,
// 0=scrollbar, -1=none (not a webkit scrollbar selector).
func webkitScrollbarPseudo(sel *css.ComplexSelector) int {
	if sel == nil || len(sel.Compounds) == 0 {
		return -1
	}
	last := sel.Compounds[len(sel.Compounds)-1]
	for _, s := range last.Selectors {
		if s.Match != css.MatchPseudoElement {
			continue
		}
		switch s.PseudoElem {
		case css.PseudoElementWebkitScrollbar:
			return 0
		case css.PseudoElementWebkitScrollbarThumb:
			return 1
		case css.PseudoElementWebkitScrollbarTrack:
			return 2
		}
	}
	return -1
}

// collectScrollbarDeclarations gathers declarations from rules whose selector
// targets ::-webkit-scrollbar / ::-webkit-scrollbar-thumb on el. These are
// stored separately (applied to the scrollbar palette, never to the element's
// own computed style — width:4px on a scrollbar must not shrink the element).
// collectScrollbarFromRule matches one StyleRule whose selector is a
// ::-webkit-scrollbar pseudo, appending matched declarations.
func (r *Resolver) collectScrollbarFromRule(v *css.StyleRule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, order int) int {
	if v.Selectors != nil {
		for _, sel := range v.Selectors.Selectors {
			if k := webkitScrollbarPseudo(&sel); k >= 0 && r.checker.Match(sel, el) {
				spec := css.SpecificityOfComplex(sel)
				for _, d := range v.Declarations {
					*collected = append(*collected, collectedDecl{
						decl:        d,
						origin:      origin,
						important:   d.Important,
						specificity: spec,
						sourceOrder: order,
						selector:    sel.String(),
						sbKind:      k,
					})
					order++
				}
			}
		}
	}
	if len(v.NestedRules) > 0 {
		order = r.collectScrollbarDeclarations(v.NestedRules, origin, el, collected, order)
	}
	return order
}

func (r *Resolver) collectScrollbarDeclarations(rules []css.Rule, origin css.Origin, el *dom.Element, collected *[]collectedDecl, baseOrder int) int {
	order := baseOrder
	for _, rule := range rules {
		switch v := rule.(type) {
		case *css.StyleRule:
			order = r.collectScrollbarFromRule(v, origin, el, collected, order)
		case *css.MediaRule:
			if len(v.Parsed) > 0 && !css.MatchesAny(v.Parsed, r.mediaQueryCtx) {
				continue
			}
			order = r.collectScrollbarDeclarations(v.Rules, origin, el, collected, order)
		case *css.SupportsRule:
			order = r.collectScrollbarDeclarations(v.Rules, origin, el, collected, order)
		}
	}
	return order
}

// applyScrollbarDeclarations maps ::-webkit-scrollbar declarations onto the
// element's scrollbar palette (raw properties read by the painter):
//
//	::-webkit-scrollbar { width → -webkit-scrollbar-width, background → track color }
//	::-webkit-scrollbar-thumb { background → -webkit-scrollbar-thumb-color,
//	                            border-radius → -webkit-scrollbar-thumb-radius }
func applyScrollbarDeclarations(cs *ComputedStyle, decls []collectedDecl) {
	if len(decls) == 0 {
		return
	}
	// Sort by cascade (origin/importance/specificity/sourceOrder) so later
	// declarations overwrite earlier ones like the normal cascade.
	sort.SliceStable(decls, func(i, j int) bool {
		a, b := decls[i], decls[j]
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
	for _, cd := range decls {
		pn := strings.ToLower(cd.decl.Name)
		val := cd.decl.ValueString()
		if DiagEnabled("scrollbar") {
			Diagf("scrollbar", "  DECL sel=%q kind=%d prop=%s val=%q", cd.selector, cd.sbKind, pn, val)
		}
		switch pn {
		case "width":
			cs.SetProperty("-webkit-scrollbar-width", val)
		case "height":
			cs.SetProperty("-webkit-scrollbar-height", val)
		case "background", "background-color":
			switch cd.sbKind {
			case 1:
				cs.SetProperty("-webkit-scrollbar-thumb-color", val)
			case 2:
				cs.SetProperty("-webkit-scrollbar-track-color", val)
			default:
				cs.SetProperty("-webkit-scrollbar-track-color", val)
			}
		case "border-radius":
			if cd.sbKind == 1 {
				cs.SetProperty("-webkit-scrollbar-thumb-radius", val)
			}
		case "scrollbar-width":
			cs.SetProperty("scrollbar-width", val)
		case "scrollbar-color":
			cs.SetProperty("scrollbar-color", val)
		}
	}
}

// webkitScrollbarPseudoFromSelector is retained for compatibility (not used by
// the cascade path, which records sbKind at collection time).


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
	// A shadow tree's top-level elements inherit from the shadow host (CSS Scoping:
	// inheritance crosses the shadow boundary from host to shadow tree). The raw
	// ParentNode of such an element is the ShadowRoot (a DocumentFragment), not an
	// Element, so it would otherwise break the inheritance chain.
	if sr, ok := p.(*dom.ShadowRoot); ok {
		return sr.Host()
	}
	return nil
}

// sheetScopingRoot returns the shadow root an author stylesheet is scoped to, derived
// from its owner node's position in the tree. Document-level sheets (owner nil, or an
// owner outside any shadow tree) return nil. User-agent sheets are never scoped and
// the caller short-circuits before calling this.
func sheetScopingRoot(sheet *css.CSSStyleSheet) *dom.ShadowRoot {
	owner := sheet.OwnerNode()
	if owner == nil {
		return nil
	}
	return dom.ContainingShadowRoot(owner)
}

// sheetScopeDepth returns the tree-scope depth of a stylesheet: 0 for a document-level
// sheet (or UA sheet), N for a sheet inside an N-deep shadow tree. This is the "scope"
// dimension of the cascade (CSS Scoping Level 1 §3.3).
func sheetScopeDepth(sheet *css.CSSStyleSheet) int {
	if sheet.Origin() == css.OriginUserAgent {
		return 0
	}
	sr := sheetScopingRoot(sheet)
	if sr == nil {
		return 0
	}
	return sr.TreeScopeDepth()
}

// elScopeDepth returns the tree-scope depth of an element (0 in the document tree, N
// inside an N-deep shadow tree). Inline style declarations carry this scope so a
// shadow-tree element's inline style outranks a document-level ::part rule.
func elScopeDepth(el *dom.Element) int {
	sr := dom.ContainingShadowRoot(el)
	if sr == nil {
		return 0
	}
	return sr.TreeScopeDepth()
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
		case pl == "step-start", pl == "step-end", strings.HasPrefix(pl, "steps("), strings.HasPrefix(pl, "cubic-bezier("):
			// 函数形式 timing-function（CM6 光标闪烁用 steps(1)）：
			// 必须识别为 timingFunction 而非动画名——否则 AnimationName
			// 被错误设为 "steps(1)"，KeyframesLookup 找不到 cm-blink →
			// 光标 opacity 动画完全不驱动（「光标不闪」根因之一）。
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
