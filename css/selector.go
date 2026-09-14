// Translation of: Source/WebCore/css/CSSSelector.h
//                  Source/WebCore/css/CSSSelector.cpp
//                  Source/WebCore/css/parser/CSSSelectorParser.cpp
// Completeness: 75%
// Simplifications:
//   - selectors stored as a slice of CompoundSelector (left-to-right logical order)
//     rather than WebKit's right-to-left array of single selectors
//   - qualified names / namespaces collapsed to local name strings
//   - :host / :host-context / ::slotted / ::part are parsed (with selector-list or
//     part-name-list arguments) and matched by SelectorChecker; ::part forward-matching
//     through one or more `exportparts` layers is implemented (matchPart +
//     parseExportparts follow the re-export chain host by host)
//   - :fullscreen / :open / :closed / :modal are implemented; view-transition
//     pseudo-elements are parsed but never match (no view-transition machinery in
//     this port), and :popover-open / :autofill / :picture-in-picture are not
//     modelled (no popover / autofill / picture-in-picture in this port)
//   - :valid / :invalid / :in-range / :out-of-range consult an injected
//     FormValidityResolver (css/validity.go): the constraint-validation state
//     lives in the html5 package, which already imports css, so the checker
//     takes a plain data snapshot instead of importing it back
//   - argument parsing for :nth-* stores An+B as integers (no full An+B syntax for
//     "even"/"odd" is exposed, but those are precomputed into (2,0) and (2,1))
//   - pseudo-element argument form (e.g. ::highlight(name)) stores the argument string
//     but does not validate against any highlight registry

package css

import (
	"strings"
)

// Match mirrors CSSSelector::Match, describing how a single simple selector matches.
type Match int

const (
	MatchUnknown Match = iota
	MatchTag
	MatchID
	MatchClass
	MatchExact        // [attr="val"]
	MatchSet          // [attr]
	MatchList         // [attr~="val"]
	MatchHyphen       // [attr|="val"]
	MatchPseudoClass
	MatchPseudoElement
	MatchContain // [attr*="val"]
	MatchBegin   // [attr^="val"]
	MatchEnd     // [attr$="val"]
)

// Relation mirrors CSSSelector::Relation, describing the combinator that precedes a
// compound selector.
type Relation int

const (
	RelationSubselector Relation = iota
	RelationDescendant       // " "
	RelationChild            // ">"
	RelationDirectAdjacent   // "+"
	RelationIndirectAdjacent // "~"
)

// AttributeMatchType mirrors CSSSelector::AttributeMatchType.
type AttributeMatchType int

const (
	AttributeMatchDefault AttributeMatchType = iota
	AttributeMatchCaseInsensitive
	AttributeMatchCaseSensitive
)

// PseudoClass enumerates the supported pseudo-classes, mirroring the entries in
// CSSSelectorPseudoClass that are commonly used in CSS. Pseudo-classes marked "unknown"
// are dropped by the parser rather than represented here.
type PseudoClass int

const (
	PseudoClassUnknown PseudoClass = iota
	PseudoClassHover
	PseudoClassFocus
	PseudoClassFocusVisible
	PseudoClassFocusWithin
	PseudoClassActive
	PseudoClassVisited
	PseudoClassLink
	PseudoClassAnyLink
	PseudoClassTarget
	PseudoClassEmpty
	PseudoClassRoot
	PseudoClassScope
	PseudoClassFirstChild
	PseudoClassLastChild
	PseudoClassOnlyChild
	PseudoClassFirstOfType
	PseudoClassLastOfType
	PseudoClassOnlyOfType
	PseudoClassChecked
	PseudoClassDisabled
	PseudoClassEnabled
	PseudoClassPlaceholderShown
	PseudoClassReadOnly
	PseudoClassReadWrite
	PseudoClassRequired
	PseudoClassOptional
	PseudoClassValid
	PseudoClassInvalid
	PseudoClassInRange
	PseudoClassOutOfRange
	PseudoClassDefault
	PseudoClassIndeterminate
	PseudoClassNthChild
	PseudoClassNthLastChild
	PseudoClassNthOfType
	PseudoClassNthLastOfType
	PseudoClassNot
	PseudoClassIs
	PseudoClassWhere
	PseudoClassHas
	PseudoClassLang
	PseudoClassDir
	PseudoClassDefined
	PseudoClassHost
	PseudoClassHostContext
	PseudoClassFullscreen
	PseudoClassOpen
	PseudoClassClosed
	PseudoClassModal
	PseudoClassUserValid
	PseudoClassUserInvalid
)

// PseudoElement enumerates the supported pseudo-elements, mirroring
// CSSSelectorPseudoElement.
type PseudoElement int

const (
	PseudoElementUnknown PseudoElement = iota
	PseudoElementBefore
	PseudoElementAfter
	PseudoElementFirstLine
	PseudoElementFirstLetter
	PseudoElementMarker
	PseudoElementPlaceholder
	PseudoElementSelection
	PseudoElementBackdrop
	PseudoElementCue
	PseudoElementSlotted
	PseudoElementPart
	PseudoElementHighlight
	PseudoElementViewTransition
	PseudoElementViewTransitionGroup
	PseudoElementViewTransitionImagePair
	PseudoElementViewTransitionOld
	PseudoElementViewTransitionNew
	PseudoElementWebkitScrollbar
	PseudoElementWebkitScrollbarThumb
	PseudoElementWebkitScrollbarTrack
)

// SimpleSelector is one component of a compound selector: a tag, an id, a class, an
// attribute selector, or a pseudo-class/element. It is the Go translation of one
// "selector" entry in WebKit's CSSSelector array.
type SimpleSelector struct {
	Match       Match
	Relation    Relation
	Value       string
	Argument    string
	Attribute   string
	AttrMatch   AttributeMatchType
	PseudoClass PseudoClass
	PseudoElem  PseudoElement
	// NthA and NthB hold the (An+B) coefficients for :nth-* pseudo-classes.
	NthA int
	NthB int
	// SelectorList holds the sub-selector list for :is(), :not(), :where(), :has().
	SelectorList *SelectorList
	// StringList holds an arbitrary list of strings for pseudo-classes that take
	// a comma-separated argument list (e.g. :lang(en, fr)).
	StringList []string
}

// CompoundSelector is a sequence of SimpleSelectors that all apply to the same
// element (no combinator between them), plus the combinator that links this compound
// to the previous compound in the complex selector.
type CompoundSelector struct {
	Selectors []SimpleSelector
	Relation  Relation // combinator linking this compound to the previous one
}

// ComplexSelector is one complex selector expressed as a slice of CompoundSelectors
// in logical (left-to-right) reading order. The first element's Relation is always
// RelationSubselector.
type ComplexSelector struct {
	Compounds []CompoundSelector
}

// SelectorList holds a list of complex selectors separated by commas, mirroring
// CSSSelectorList.
type SelectorList struct {
	Selectors []ComplexSelector
}

// String renders a SelectorList back to CSS text. This is best-effort and used only
// for diagnostics and tests.
func (l *SelectorList) String() string {
	if l == nil {
		return ""
	}
	parts := make([]string, 0, len(l.Selectors))
	for _, cs := range l.Selectors {
		parts = append(parts, cs.String())
	}
	return strings.Join(parts, ", ")
}

// String renders a ComplexSelector back to CSS text.
func (c ComplexSelector) String() string {
	var sb strings.Builder
	for i, comp := range c.Compounds {
		if i > 0 {
			sb.WriteString(relationString(comp.Relation))
		}
		for _, s := range comp.Selectors {
			sb.WriteString(s.String())
		}
	}
	return sb.String()
}

func relationString(r Relation) string {
	switch r {
	case RelationDescendant:
		return " "
	case RelationChild:
		return " > "
	case RelationDirectAdjacent:
		return " + "
	case RelationIndirectAdjacent:
		return " ~ "
	}
	return ""
}

// String renders a SimpleSelector back to CSS text.
func (s SimpleSelector) String() string {
	var sb strings.Builder
	switch s.Match {
	case MatchTag:
		sb.WriteString(s.Value)
	case MatchID:
		sb.WriteByte('#')
		sb.WriteString(s.Value)
	case MatchClass:
		sb.WriteByte('.')
		sb.WriteString(s.Value)
	case MatchSet:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteByte(']')
	case MatchExact:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchList:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("~=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchHyphen:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("|=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchBegin:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("^=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchEnd:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("$=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchContain:
		sb.WriteByte('[')
		sb.WriteString(s.Attribute)
		sb.WriteString("*=\"")
		sb.WriteString(s.Value)
		sb.WriteString("\"]")
	case MatchPseudoClass:
		sb.WriteByte(':')
		sb.WriteString(PseudoClassName(s.PseudoClass))
		if s.SelectorList != nil || s.Argument != "" || len(s.StringList) > 0 || s.PseudoClass == PseudoClassNthChild {
			sb.WriteByte('(')
			if s.SelectorList != nil {
				sb.WriteString(s.SelectorList.String())
			} else if len(s.StringList) > 0 {
				sb.WriteString(strings.Join(s.StringList, ", "))
			} else if s.PseudoClass == PseudoClassNthChild || s.PseudoClass == PseudoClassNthLastChild ||
				s.PseudoClass == PseudoClassNthOfType || s.PseudoClass == PseudoClassNthLastOfType {
				sb.WriteString(formatAnB(s.NthA, s.NthB))
			} else {
				sb.WriteString(s.Argument)
			}
			sb.WriteByte(')')
		}
	case MatchPseudoElement:
		sb.WriteString("::")
		sb.WriteString(PseudoElementName(s.PseudoElem))
	}
	return sb.String()
}

// formatAnB renders the (a, b) coefficients of an :nth-* expression back as text.
func formatAnB(a, b int) string {
	if a == 0 {
		return intToString(b)
	}
	var sb strings.Builder
	if a == 1 {
		sb.WriteByte('n')
	} else if a == -1 {
		sb.WriteString("-n")
	} else {
		sb.WriteString(intToString(a))
		sb.WriteByte('n')
	}
	if b > 0 {
		sb.WriteByte('+')
		sb.WriteString(intToString(b))
	} else if b < 0 {
		sb.WriteString(intToString(b))
	}
	return sb.String()
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var sb strings.Builder
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+(n%10)))
		n /= 10
	}
	if neg {
		sb.WriteByte('-')
	}
	for i := len(digits) - 1; i >= 0; i-- {
		sb.WriteByte(digits[i])
	}
	return sb.String()
}

// PseudoClassName returns the canonical lowercase name of a pseudo-class.
func PseudoClassName(p PseudoClass) string {
	switch p {
	case PseudoClassHover:
		return "hover"
	case PseudoClassFocus:
		return "focus"
	case PseudoClassFocusVisible:
		return "focus-visible"
	case PseudoClassFocusWithin:
		return "focus-within"
	case PseudoClassActive:
		return "active"
	case PseudoClassVisited:
		return "visited"
	case PseudoClassLink:
		return "link"
	case PseudoClassAnyLink:
		return "any-link"
	case PseudoClassTarget:
		return "target"
	case PseudoClassEmpty:
		return "empty"
	case PseudoClassRoot:
		return "root"
	case PseudoClassScope:
		return "scope"
	case PseudoClassFirstChild:
		return "first-child"
	case PseudoClassLastChild:
		return "last-child"
	case PseudoClassOnlyChild:
		return "only-child"
	case PseudoClassFirstOfType:
		return "first-of-type"
	case PseudoClassLastOfType:
		return "last-of-type"
	case PseudoClassOnlyOfType:
		return "only-of-type"
	case PseudoClassChecked:
		return "checked"
	case PseudoClassDisabled:
		return "disabled"
	case PseudoClassEnabled:
		return "enabled"
	case PseudoClassPlaceholderShown:
		return "placeholder-shown"
	case PseudoClassReadOnly:
		return "read-only"
	case PseudoClassReadWrite:
		return "read-write"
	case PseudoClassRequired:
		return "required"
	case PseudoClassOptional:
		return "optional"
	case PseudoClassValid:
		return "valid"
	case PseudoClassInvalid:
		return "invalid"
	case PseudoClassInRange:
		return "in-range"
	case PseudoClassOutOfRange:
		return "out-of-range"
	case PseudoClassDefault:
		return "default"
	case PseudoClassIndeterminate:
		return "indeterminate"
	case PseudoClassNthChild:
		return "nth-child"
	case PseudoClassNthLastChild:
		return "nth-last-child"
	case PseudoClassNthOfType:
		return "nth-of-type"
	case PseudoClassNthLastOfType:
		return "nth-last-of-type"
	case PseudoClassNot:
		return "not"
	case PseudoClassIs:
		return "is"
	case PseudoClassWhere:
		return "where"
	case PseudoClassHas:
		return "has"
	case PseudoClassLang:
		return "lang"
	case PseudoClassDir:
		return "dir"
	case PseudoClassDefined:
		return "defined"
	case PseudoClassHost:
		return "host"
	case PseudoClassHostContext:
		return "host-context"
	case PseudoClassFullscreen:
		return "fullscreen"
	case PseudoClassOpen:
		return "open"
	case PseudoClassClosed:
		return "closed"
	case PseudoClassModal:
		return "modal"
	case PseudoClassUserValid:
		return "user-valid"
	case PseudoClassUserInvalid:
		return "user-invalid"
	}
	return ""
}

// PseudoElementName returns the canonical lowercase name of a pseudo-element.
func PseudoElementName(p PseudoElement) string {
	switch p {
	case PseudoElementBefore:
		return "before"
	case PseudoElementAfter:
		return "after"
	case PseudoElementFirstLine:
		return "first-line"
	case PseudoElementFirstLetter:
		return "first-letter"
	case PseudoElementMarker:
		return "marker"
	case PseudoElementPlaceholder:
		return "placeholder"
	case PseudoElementSelection:
		return "selection"
	case PseudoElementBackdrop:
		return "backdrop"
	case PseudoElementCue:
		return "cue"
	case PseudoElementSlotted:
		return "slotted"
	case PseudoElementPart:
		return "part"
	case PseudoElementHighlight:
		return "highlight"
	case PseudoElementViewTransition:
		return "view-transition"
	case PseudoElementViewTransitionGroup:
		return "view-transition-group"
	case PseudoElementViewTransitionImagePair:
		return "view-transition-image-pair"
	case PseudoElementViewTransitionOld:
		return "view-transition-old"
	case PseudoElementViewTransitionNew:
		return "view-transition-new"
	case PseudoElementWebkitScrollbar:
		return "-webkit-scrollbar"
	case PseudoElementWebkitScrollbarThumb:
		return "-webkit-scrollbar-thumb"
	case PseudoElementWebkitScrollbarTrack:
		return "-webkit-scrollbar-track"
	}
	return ""
}

// LookupPseudoClass returns the PseudoClass for a name, or PseudoClassUnknown if the
// name is not recognized.
func LookupPseudoClass(name string) PseudoClass {
	switch strings.ToLower(name) {
	case "hover":
		return PseudoClassHover
	case "focus":
		return PseudoClassFocus
	case "focus-visible":
		return PseudoClassFocusVisible
	case "focus-within":
		return PseudoClassFocusWithin
	case "active":
		return PseudoClassActive
	case "visited":
		return PseudoClassVisited
	case "link":
		return PseudoClassLink
	case "any-link":
		return PseudoClassAnyLink
	case "target":
		return PseudoClassTarget
	case "empty":
		return PseudoClassEmpty
	case "root":
		return PseudoClassRoot
	case "scope":
		return PseudoClassScope
	case "first-child":
		return PseudoClassFirstChild
	case "last-child":
		return PseudoClassLastChild
	case "only-child":
		return PseudoClassOnlyChild
	case "first-of-type":
		return PseudoClassFirstOfType
	case "last-of-type":
		return PseudoClassLastOfType
	case "only-of-type":
		return PseudoClassOnlyOfType
	case "checked":
		return PseudoClassChecked
	case "disabled":
		return PseudoClassDisabled
	case "enabled":
		return PseudoClassEnabled
	case "placeholder-shown":
		return PseudoClassPlaceholderShown
	case "read-only":
		return PseudoClassReadOnly
	case "read-write":
		return PseudoClassReadWrite
	case "required":
		return PseudoClassRequired
	case "optional":
		return PseudoClassOptional
	case "valid":
		return PseudoClassValid
	case "invalid":
		return PseudoClassInvalid
	case "in-range":
		return PseudoClassInRange
	case "out-of-range":
		return PseudoClassOutOfRange
	case "default":
		return PseudoClassDefault
	case "indeterminate":
		return PseudoClassIndeterminate
	case "nth-child":
		return PseudoClassNthChild
	case "nth-last-child":
		return PseudoClassNthLastChild
	case "nth-of-type":
		return PseudoClassNthOfType
	case "nth-last-of-type":
		return PseudoClassNthLastOfType
	case "not":
		return PseudoClassNot
	case "is":
		return PseudoClassIs
	case "where":
		return PseudoClassWhere
	case "has":
		return PseudoClassHas
	case "lang":
		return PseudoClassLang
	case "dir":
		return PseudoClassDir
	case "defined":
		return PseudoClassDefined
	case "host":
		return PseudoClassHost
	case "host-context":
		return PseudoClassHostContext
	case "fullscreen":
		return PseudoClassFullscreen
	case "open":
		return PseudoClassOpen
	case "closed":
		return PseudoClassClosed
	case "modal":
		return PseudoClassModal
	case "user-valid":
		return PseudoClassUserValid
	case "user-invalid":
		return PseudoClassUserInvalid
	}
	return PseudoClassUnknown
}

// LookupPseudoElement returns the PseudoElement for a name, or PseudoElementUnknown
// if the name is not recognized.
func LookupPseudoElement(name string) PseudoElement {
	switch strings.ToLower(name) {
	case "before":
		return PseudoElementBefore
	case "after":
		return PseudoElementAfter
	case "first-line":
		return PseudoElementFirstLine
	case "first-letter":
		return PseudoElementFirstLetter
	case "marker":
		return PseudoElementMarker
	case "placeholder":
		return PseudoElementPlaceholder
	case "selection":
		return PseudoElementSelection
	case "backdrop":
		return PseudoElementBackdrop
	case "cue":
		return PseudoElementCue
	case "slotted":
		return PseudoElementSlotted
	case "part":
		return PseudoElementPart
	case "highlight":
		return PseudoElementHighlight
	case "view-transition":
		return PseudoElementViewTransition
	case "view-transition-group":
		return PseudoElementViewTransitionGroup
	case "view-transition-image-pair":
		return PseudoElementViewTransitionImagePair
	case "view-transition-old":
		return PseudoElementViewTransitionOld
	case "view-transition-new":
		return PseudoElementViewTransitionNew
	case "-webkit-scrollbar":
		return PseudoElementWebkitScrollbar
	case "-webkit-scrollbar-thumb":
		return PseudoElementWebkitScrollbarThumb
	case "-webkit-scrollbar-track":
		return PseudoElementWebkitScrollbarTrack
	}
	return PseudoElementUnknown
}

// ParsePseudoElementOrClass classifies a name as a pseudo-element or pseudo-class by
// the number of colons in the input. Single-colon syntax (legacy) still maps to
// pseudo-element for the four legacy pseudo-elements (before/after/first-line/
// first-letter).
func ParsePseudoElementOrClass(colons int, name string) (Match, PseudoClass, PseudoElement) {
	pe := LookupPseudoElement(name)
	if colons == 2 || (colons == 1 && pe != PseudoElementUnknown &&
		(pe == PseudoElementBefore || pe == PseudoElementAfter ||
			pe == PseudoElementFirstLine || pe == PseudoElementFirstLetter)) {
		if pe == PseudoElementUnknown {
			// Unknown pseudo-element falls back to pseudo-class with the raw name.
			pc := LookupPseudoClass(name)
			return MatchPseudoClass, pc, PseudoElementUnknown
		}
		return MatchPseudoElement, PseudoClassUnknown, pe
	}
	return MatchPseudoClass, LookupPseudoClass(name), PseudoElementUnknown
}
