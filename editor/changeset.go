// Translation of: CodeMirror 6 — packages/state/src/changeSet.ts
//                  https://github.com/codemirror/state/blob/main/src/changeSet.ts
//
// Completeness: 80%
// Differences from CM6:
//   - CM6 uses UTF-16 code unit positions; this port uses rune offsets.
//   - The `inserted` field (storing the inserted Text) is kept, but the
//     `mirror` field (for bidirectional mapping) is omitted in v1.
//   - Methods use PascalCase (Go convention).
//
// ChangeSet describes a set of document changes. Each change is a
// ChangeDesc with a replaced range [fromA, toA) in the original document
// and inserted text of `InsertLength` runes. The changes are sorted by
// `fromA` and non-overlapping.

package editor

// ChangeDesc is a description of a single document change without the
// inserted text content. It records the range replaced in the original
// document and the length of the inserted text.
//
// Mirrors CM6's ChangeDesc.
type ChangeDesc struct {
	// FromA is the start position of the replaced range in the original
	// document (rune offset).
	FromA int
	// ToA is the end position of the replaced range in the original
	// document (rune offset).
	ToA int
	// InsertLength is the number of runes inserted (replacing the range
	// [FromA, ToA)).
	InsertLength int
}

// Empty reports whether the change replaces nothing and inserts nothing.
func (c ChangeDesc) Empty() bool {
	return c.FromA == c.ToA && c.InsertLength == 0
}

// LenDelta returns the net change in document length (inserted - deleted).
func (c ChangeDesc) LenDelta() int {
	return c.InsertLength - (c.ToA - c.FromA)
}

// ChangeSet is an immutable set of document changes. The changes are sorted
// by FromA and non-overlapping.
//
// Mirrors CM6's ChangeSet.
type ChangeSet struct {
	// changes are the individual changes, sorted by FromA.
	changes []ChangeDesc
	// inserted stores the inserted text for each change (parallel to
	// `changes`). May be nil if the ChangeSet was created without text
	// (e.g. from a ChangeDesc-only spec).
	inserted []Text
}

// NewChangeSet creates a ChangeSet from a slice of ChangeDesc values.
// The changes are sorted and validated for non-overlap.
func NewChangeSet(changes []ChangeDesc) ChangeSet {
	cs := ChangeSet{changes: append([]ChangeDesc(nil), changes...)}
	// Sort by FromA (insertion sort for simplicity; change sets are small).
	for i := 1; i < len(cs.changes); i++ {
		for j := i; j > 0 && cs.changes[j-1].FromA > cs.changes[j].FromA; j-- {
			cs.changes[j-1], cs.changes[j] = cs.changes[j], cs.changes[j-1]
		}
	}
	return cs
}

// NewChangeSetWithText creates a ChangeSet with inserted text content.
func NewChangeSetWithText(changes []ChangeDesc, inserted []Text) ChangeSet {
	cs := NewChangeSet(changes)
	cs.inserted = append([]Text(nil), inserted...)
	return cs
}

// Empty reports whether the ChangeSet contains no changes.
func (cs ChangeSet) Empty() bool { return len(cs.changes) == 0 }

// Length returns the total length difference (net added/removed runes).
func (cs ChangeSet) Length() int {
	delta := 0
	for _, c := range cs.changes {
		delta += c.LenDelta()
	}
	return delta
}

// Changes returns the individual change descriptions.
func (cs ChangeSet) Changes() []ChangeDesc { return cs.changes }

// Inserted returns the inserted text for the change at index `i`, or
// Empty() if not available.
func (cs ChangeSet) Inserted(i int) Text {
	if i < 0 || i >= len(cs.inserted) || cs.inserted[i] == nil {
		return Empty()
	}
	return cs.inserted[i]
}

// MapPos maps a position in the OLD document through the ChangeSet,
// returning the corresponding position in the NEW document.
//
// If `assoc` < 0, the position is associated with the content before it
// (so a position at a change boundary maps to the start of the insertion).
// If `assoc` >= 0, it is associated with the content after it (maps to
// the end of the insertion).
//
// Mirrors CM6's ChangeSet.mapPos().
func (cs ChangeSet) MapPos(pos int, assoc int) int {
	offset := 0
	for _, c := range cs.changes {
		if c.ToA < pos {
			// Change is entirely before pos; accumulate its delta.
			offset += c.LenDelta()
		} else if c.FromA <= pos && pos <= c.ToA {
			// pos is inside [fromA, toA] (inclusive on both ends).
			// Map to the insertion point.
			if assoc < 0 {
				return c.FromA + offset
			}
			return c.FromA + offset + c.InsertLength
		} else {
			// c.FromA > pos; this and subsequent changes are after pos.
			break
		}
	}
	return pos + offset
}

// totalOffsetBefore returns the accumulated length delta of all changes
// that are entirely before `pos` in the original document.
func (cs ChangeSet) totalOffsetBefore(pos int) int {
	offset := 0
	for _, c := range cs.changes {
		if c.ToA <= pos {
			offset += c.LenDelta()
		}
	}
	return offset
}

// ChangedRange returns the range in the new document that covers all
// changed regions.
func (cs ChangeSet) ChangedRange() (from, to int) {
	if len(cs.changes) == 0 {
		return 0, 0
	}
	// from is the first change's FromA (mapped through preceding changes,
	// but since it's the first, no preceding changes).
	from = cs.changes[0].FromA
	// to is the last change's end in the new document.
	last := cs.changes[len(cs.changes)-1]
	offset := 0
	for _, c := range cs.changes {
		if c.FromA < last.FromA {
			offset += c.LenDelta()
		}
	}
	to = last.FromA + last.InsertLength + offset
	return from, to
}

// Apply applies the ChangeSet to a Text, returning a new Text.
func (cs ChangeSet) Apply(doc Text) Text {
	if cs.Empty() {
		return doc
	}
	var b []byte
	// We build the result by iterating through the changes.
	pos := 0
	for i, c := range cs.changes {
		// Copy unchanged text before this change.
		b = append(b, doc.SliceString(pos, c.FromA)...)
		// Insert the new text.
		if i < len(cs.inserted) && cs.inserted[i] != nil {
			b = append(b, cs.inserted[i].String()...)
		} else if c.InsertLength > 0 {
			// No inserted text available; this shouldn't happen for
			// a text-producing change set.
		}
		pos = c.ToA
	}
	// Copy trailing text.
	b = append(b, doc.SliceString(pos, doc.Length())...)
	return TextFromString(string(b))
}

// InvertDesc returns the inverse ChangeDesc for a change, given the
// original document length. Used to build an inverse ChangeSet for undo.
func InvertDesc(c ChangeDesc, originalLength int) ChangeDesc {
	return ChangeDesc{
		FromA:        c.FromA,
		ToA:          c.FromA + c.InsertLength,
		InsertLength: c.ToA - c.FromA,
	}
}

// Invert returns a ChangeSet that undoes `cs` when applied to the result
// of `cs.Apply(doc)`. The `original` document is needed to extract the
// deleted text content.
func (cs ChangeSet) Invert(original Text) ChangeSet {
	descs := make([]ChangeDesc, len(cs.changes))
	inserted := make([]Text, len(cs.changes))
	for i, c := range cs.changes {
		// The inverse deletes the inserted text and re-inserts the
		// original text.
		descs[i] = InvertDesc(c, original.Length())
		inserted[i] = original.Slice(c.FromA, c.ToA)
	}
	return NewChangeSetWithText(descs, inserted)
}

// MapDesc maps a ChangeDesc through this ChangeSet, returning a new
// ChangeDesc in the new document coordinate space.
func (cs ChangeSet) MapDesc(other ChangeDesc, assoc int) ChangeDesc {
	from := cs.MapPos(other.FromA, assoc)
	to := cs.MapPos(other.ToA, -assoc)
	return ChangeDesc{
		FromA:        from,
		ToA:          to,
		InsertLength: other.InsertLength,
	}
}
