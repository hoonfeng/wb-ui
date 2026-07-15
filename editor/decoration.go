// Translation of: CodeMirror 6 — packages/view/src/decorations.ts
//                  https://github.com/codemirror/view/blob/main/src/decorations.ts
//
// Completeness: 70%
// Differences from CM6:
//   - CM6 uses a B-tree-based DecorationSet for O(log n) lookups; this port
//     uses a flat sorted slice, which is simpler and sufficient for
//     small-to-medium documents.
//   - CM6 supports side association (startSide/endSide) with -1/0/1 values;
//     this port uses a simplified Assoc enum (Before/After/Default).
//   - The `point` flag is inferred from the decoration type (Widget/Replace
//     are point decorations; Mark/Line are range decorations).
//   - Methods use PascalCase (Go convention).
//
// Decorations are declarative overlays on the document. They describe visual
// modifications (highlighting spans, line classes, widgets, replacements)
// without modifying the document text itself. The EditorView applies them
// during rendering.

package editor

// DecorationType identifies the kind of decoration.
type DecorationType int

const (
	// DecorationMarkType is a range decoration that applies a style/class/
	// attributes to a span of text (like a <span>).
	DecorationMarkType DecorationType = iota
	// DecorationWidgetType is a point decoration that inserts a widget at
	// a single position (zero-length range).
	DecorationWidgetType
	// DecorationReplaceType is a point decoration that replaces a range
	// of text with a widget (hiding the original text).
	DecorationReplaceType
	// DecorationLineType is a line-level decoration that applies a class/
	// attributes to an entire line.
	DecorationLineType
)

// SideAssociation controls how a decoration boundary moves when text is
// inserted exactly at the boundary.
type SideAssociation int

const (
	// SideBefore means the boundary stays before inserted text.
	SideBefore SideAssociation = -1
	// SideDefault means the boundary follows the default association.
	SideDefault SideAssociation = 0
	// SideAfter means the boundary stays after inserted text.
	SideAfter SideAssociation = 1
)

// Decoration is the interface implemented by all decoration types.
// Each decoration has a range [From, To) in the document and a type.
type Decoration interface {
	// From returns the start position (rune offset).
	From() int
	// To returns the end position (rune offset).
	To() int
	// Type returns the decoration kind.
	Type() DecorationType
	// StartSide returns the association of the start boundary.
	StartSide() SideAssociation
	// EndSide returns the association of the end boundary.
	EndSide() SideAssociation
}

// DecorationMark applies a CSS class and/or attributes to a range of text.
// Used for syntax highlighting spans, search match highlights, etc.
type DecorationMark struct {
	// from is the start position (inclusive, rune offset).
	from int
	// to is the end position (exclusive, rune offset).
	to int
	// Class is the CSS class to apply to the span.
	Class string
	// Attributes is a map of HTML attributes to set on the span element.
	Attributes map[string]string
	// startSide/endSide control boundary association.
	startSide SideAssociation
	endSide   SideAssociation
}

// Mark creates a DecorationMark for the range [from, to) with the given class.
func Mark(from, to int, class string) DecorationMark {
	return DecorationMark{from: from, to: to, Class: class}
}

// MarkWithAttrs creates a DecorationMark with attributes.
func MarkWithAttrs(from, to int, class string, attrs map[string]string) DecorationMark {
	return DecorationMark{from: from, to: to, Class: class, Attributes: attrs}
}

func (d DecorationMark) From() int             { return d.from }
func (d DecorationMark) To() int               { return d.to }
func (d DecorationMark) Type() DecorationType   { return DecorationMarkType }
func (d DecorationMark) StartSide() SideAssociation { return d.startSide }
func (d DecorationMark) EndSide() SideAssociation   { return d.endSide }

// WithStartSide sets the start-side association and returns the decoration.
func (d DecorationMark) WithStartSide(s SideAssociation) DecorationMark {
	d.startSide = s
	return d
}

// WithEndSide sets the end-side association and returns the decoration.
func (d DecorationMark) WithEndSide(s SideAssociation) DecorationMark {
	d.endSide = s
	return d
}

// DecorationWidget inserts a widget at a single position (zero-length range).
// Used for interactive elements like checkboxes, fold markers, etc.
type DecorationWidget struct {
	// pos is the position where the widget is inserted (rune offset).
	pos int
	// Widget is the widget to render.
	Widget WidgetType
	// side controls which side the widget stays on when text is inserted
	// at its position.
	side SideAssociation
}

// Widget creates a DecorationWidget at the given position.
func Widget(pos int, w WidgetType) DecorationWidget {
	return DecorationWidget{pos: pos, Widget: w}
}

func (d DecorationWidget) From() int             { return d.pos }
func (d DecorationWidget) To() int               { return d.pos }
func (d DecorationWidget) Type() DecorationType   { return DecorationWidgetType }
func (d DecorationWidget) StartSide() SideAssociation { return d.side }
func (d DecorationWidget) EndSide() SideAssociation   { return d.side }

// WithSide sets the side association and returns the decoration.
func (d DecorationWidget) WithSide(s SideAssociation) DecorationWidget {
	d.side = s
	return d
}

// DecorationReplace replaces a range of text with a widget, hiding the
// original text. Used for code folding, diff hiding, etc.
type DecorationReplace struct {
	// from is the start position (inclusive, rune offset).
	from int
	// to is the end position (exclusive, rune offset).
	to int
	// Widget is the widget to render in place of the hidden text.
	Widget WidgetType
	// InclusiveStart controls whether the start boundary is inclusive.
	InclusiveStart bool
	// InclusiveEnd controls whether the end boundary is inclusive.
	InclusiveEnd bool
}

// Replace creates a DecorationReplace for the range [from, to).
func Replace(from, to int, w WidgetType) DecorationReplace {
	return DecorationReplace{from: from, to: to, Widget: w}
}

func (d DecorationReplace) From() int             { return d.from }
func (d DecorationReplace) To() int               { return d.to }
func (d DecorationReplace) Type() DecorationType   { return DecorationReplaceType }
func (d DecorationReplace) StartSide() SideAssociation {
	if d.InclusiveStart {
		return SideAfter
	}
	return SideBefore
}
func (d DecorationReplace) EndSide() SideAssociation {
	if d.InclusiveEnd {
		return SideBefore
	}
	return SideAfter
}

// DecorationLine applies a class and/or attributes to an entire line.
// Used for line numbers, active line highlight, gutters, etc.
// The From position should be the start of the line; To is typically the
// end of the line (or the start of the next line).
type DecorationLine struct {
	// from is the start position of the line (rune offset).
	from int
	// to is the end position of the line (rune offset).
	to int
	// Class is the CSS class to apply to the line.
	Class string
	// Attributes is a map of HTML attributes to set on the line element.
	Attributes map[string]string
}

// LineDecoration creates a DecorationLine for the range [from, to).
// (Named LineDecoration rather than Line to avoid clashing with the
// Line type in text.go.)
func LineDecoration(from, to int, class string) DecorationLine {
	return DecorationLine{from: from, to: to, Class: class}
}

// LineDecorationWithAttrs creates a DecorationLine with attributes.
func LineDecorationWithAttrs(from, to int, class string, attrs map[string]string) DecorationLine {
	return DecorationLine{from: from, to: to, Class: class, Attributes: attrs}
}

func (d DecorationLine) From() int             { return d.from }
func (d DecorationLine) To() int               { return d.to }
func (d DecorationLine) Type() DecorationType   { return DecorationLineType }
func (d DecorationLine) StartSide() SideAssociation { return SideBefore }
func (d DecorationLine) EndSide() SideAssociation   { return SideAfter }

// DecorationSet is a sorted collection of decorations. Decorations are sorted
// by From position. The set is immutable; operations return new sets.
//
// In CM6, DecorationSet is a B-tree for efficient lookups. This port uses a
// flat sorted slice, which is simpler and sufficient for typical editor use.
type DecorationSet struct {
	// decos is the sorted list of decorations.
	decos []Decoration
}

// NewDecorationSet creates a DecorationSet from a slice of decorations.
// If sort is true, the decorations are sorted by From/To.
func NewDecorationSet(decos []Decoration, sort bool) DecorationSet {
	ds := DecorationSet{decos: append([]Decoration(nil), decos...)}
	if sort {
		ds.sortDecorations()
	}
	return ds
}

// sortDecorations sorts the decorations by From, then by To, then by Type
// (to ensure a stable order for decorations at the same position).
func (ds *DecorationSet) sortDecorations() {
	// Insertion sort (decoration sets are typically small).
	for i := 1; i < len(ds.decos); i++ {
		for j := i; j > 0; j-- {
			if compareDecorations(ds.decos[j-1], ds.decos[j]) > 0 {
				ds.decos[j-1], ds.decos[j] = ds.decos[j], ds.decos[j-1]
			} else {
				break
			}
		}
	}
}

// compareDecorations returns -1/0/1 based on position ordering.
func compareDecorations(a, b Decoration) int {
	if a.From() != b.From() {
		if a.From() < b.From() {
			return -1
		}
		return 1
	}
	if a.To() != b.To() {
		if a.To() < b.To() {
			return -1
		}
		return 1
	}
	return int(a.Type()) - int(b.Type())
}

// Empty reports whether the set contains no decorations.
func (ds DecorationSet) Empty() bool { return len(ds.decos) == 0 }

// Size returns the number of decorations in the set.
func (ds DecorationSet) Size() int { return len(ds.decos) }

// Iter returns an iterator over all decorations in the set, in sorted order.
func (ds DecorationSet) Iter() []Decoration { return ds.decos }

// FindRange returns decorations that overlap the range [from, to).
// A decoration overlaps if its [From, To) intersects [from, to).
func (ds DecorationSet) FindRange(from, to int) []Decoration {
	var result []Decoration
	for _, d := range ds.decos {
		if d.To() <= from {
			continue // decoration is entirely before the range
		}
		if d.From() >= to {
			break // decoration is entirely after the range (sorted)
		}
		result = append(result, d)
	}
	return result
}

// FindAt returns decorations that cover position `pos`.
func (ds DecorationSet) FindAt(pos int) []Decoration {
	var result []Decoration
	for _, d := range ds.decos {
		if d.From() <= pos && pos < d.To() {
			result = append(result, d)
		} else if d.From() > pos {
			break // sorted, no more matches
		}
	}
	return result
}

// Add returns a new DecorationSet with the given decorations added.
func (ds DecorationSet) Add(decos []Decoration) DecorationSet {
	combined := append([]Decoration{}, ds.decos...)
	combined = append(combined, decos...)
	return NewDecorationSet(combined, true)
}

// Map maps all decoration ranges through the given ChangeSet, returning a new
// DecorationSet. Decorations whose ranges are completely deleted are removed.
// This is the key mechanism for keeping decorations in sync with edits.
func (ds DecorationSet) Map(changes ChangeSet) DecorationSet {
	if changes.Empty() {
		return ds
	}
	var result []Decoration
	for _, d := range ds.decos {
		mappedFrom := mapPosSide(changes, d.From(), d.StartSide())
		mappedTo := mapPosSide(changes, d.To(), d.EndSide())

		// If the range collapsed (from >= to after mapping), skip it
		// unless it's a point decoration (Widget), which survives at
		// the mapped position.
		if mappedFrom >= mappedTo {
			if d.Type() == DecorationWidgetType {
				// Point decorations survive as zero-length at the
				// mapped position.
				result = append(result, remapWidget(d, mappedFrom))
			}
			// Range decorations (Mark/Replace/Line) are dropped if
			// their range was deleted.
			continue
		}
		result = append(result, remapDecoration(d, mappedFrom, mappedTo))
	}
	return DecorationSet{decos: result}
}

// mapPosSide maps a position through a ChangeSet with a side association.
// SideBefore (-1): the position stays before inserted text.
// SideAfter (+1): the position stays after inserted text.
// SideDefault (0): same as SideAfter for mapping purposes.
func mapPosSide(cs ChangeSet, pos int, side SideAssociation) int {
	// CM6's mapPos uses assoc: -1 for before, 1 for after.
	// Our ChangeSet.MapPos uses assoc: <0 for before, >=0 for after.
	assoc := int(side)
	return cs.MapPos(pos, assoc)
}

// remapDecoration returns a copy of the decoration with updated positions.
func remapDecoration(d Decoration, from, to int) Decoration {
	switch v := d.(type) {
	case DecorationMark:
		v.from = from
		v.to = to
		return v
	case DecorationReplace:
		v.from = from
		v.to = to
		return v
	case DecorationLine:
		v.from = from
		v.to = to
		return v
	case DecorationWidget:
		v.pos = from
		return v
	}
	return d
}

// remapWidget returns a copy of a widget decoration with the new position.
func remapWidget(d Decoration, pos int) Decoration {
	if w, ok := d.(DecorationWidget); ok {
		w.pos = pos
		return w
	}
	return d
}

// Filter returns a new DecorationSet containing only decorations for which
// the predicate returns true.
func (ds DecorationSet) Filter(pred func(Decoration) bool) DecorationSet {
	var result []Decoration
	for _, d := range ds.decos {
		if pred(d) {
			result = append(result, d)
		}
	}
	return DecorationSet{decos: result}
}
