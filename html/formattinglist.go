// Translation of: Source/WebCore/html/parser/HTMLFormattingElementList.cpp
//                  Source/WebCore/html/parser/HTMLFormattingElementList.h
// Completeness: 70%
// Simplifications:
//   - Entry stores a *dom.Element directly (no HTMLStackItem snapshot); the
//     adoption agency reads attributes live from the element
//   - Bookmark is represented as a slice index
//   - Noah's Ark clause is approximated: duplicates of the same element are
//     capped at a small count rather than the full per-element comparison

package html

import "wb-ui/dom"

// formattingList is the Go translation of WebCore::HTMLFormattingElementList: the
// "list of active formatting elements" used by the adoption agency algorithm.
type formattingList struct {
	// entries holds elements and markers. A nil entry is a marker.
	entries []*dom.Element
}

// isMarker reports whether entry i is a marker.
func (l *formattingList) isMarker(i int) bool { return l.entries[i] == nil }

// size returns the number of entries.
func (l *formattingList) size() int { return len(l.entries) }

// isEmpty reports whether the list is empty.
func (l *formattingList) isEmpty() bool { return len(l.entries) == 0 }

// appendMarker pushes a marker, mirroring appendMarker().
func (l *formattingList) appendMarker() {
	l.entries = append(l.entries, nil)
}

// append pushes an element, applying the Noah's Ark condition, mirroring
// append(HTMLStackItem&&).
func (l *formattingList) append(e *dom.Element) {
	l.applyNoahsArk(e)
	l.entries = append(l.entries, e)
}

// applyNoahsArk enforces the spec's Noah's Ark clause: before appending a
// formatting element, remove earlier entries that would make the list carry more
// than three entries of the same family (same tag, same attributes) since the
// last marker. This is a simplified approximation of the full clause.
func (l *formattingList) applyNoahsArk(candidate *dom.Element) {
	lastMarker := -1
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i] == nil {
			lastMarker = i
			break
		}
	}
	// Count matching entries since the last marker; if a fourth would be added,
	// remove the oldest match (Noah's Ark caps at three of each kind).
	count := 0
	for i := lastMarker + 1; i < len(l.entries); i++ {
		if l.sameFormattingElement(l.entries[i], candidate) {
			count++
			if count >= 3 {
				// Remove this entry; subsequent indices shift so stop scanning.
				l.removeAtIndex(i)
				return
			}
		}
	}
}

// sameFormattingElement reports whether a and b are the same kind of formatting
// element for Noah's Ark purposes (same tag name and attributes).
func (l *formattingList) sameFormattingElement(a, b *dom.Element) bool {
	if a == nil || b == nil {
		return false
	}
	if a.LocalName() != b.LocalName() {
		return false
	}
	an := a.AttributeNames()
	bn := b.AttributeNames()
	if len(an) != len(bn) {
		return false
	}
	for _, name := range an {
		if a.GetAttribute(name) != b.GetAttribute(name) {
			return false
		}
	}
	return true
}

// remove removes e from the list, mirroring remove(Element&).
func (l *formattingList) remove(e *dom.Element) {
	for i, x := range l.entries {
		if x == e {
			l.entries = append(l.entries[:i], l.entries[i+1:]...)
			return
		}
	}
}

// removeAtIndex removes the entry at index i.
func (l *formattingList) removeAtIndex(i int) {
	l.entries = append(l.entries[:i], l.entries[i+1:]...)
}

// clearToLastMarker removes all entries up to and including the last marker,
// mirroring clearToLastMarker().
func (l *formattingList) clearToLastMarker() {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i] == nil {
			l.entries = l.entries[:i]
			return
		}
	}
	// No marker: clear everything.
	l.entries = l.entries[:0]
}

// find returns the index of e, or -1.
func (l *formattingList) find(e *dom.Element) int {
	for i, x := range l.entries {
		if x == e {
			return i
		}
	}
	return -1
}

// contains reports whether e is in the list.
func (l *formattingList) contains(e *dom.Element) bool { return l.find(e) >= 0 }

// lastMarkerBefore returns the index of the nearest marker at or before i, or -1.
func (l *formattingList) lastMarkerBefore(i int) int {
	for j := i; j >= 0; j-- {
		if l.entries[j] == nil {
			return j
		}
	}
	return -1
}

// lastElement returns the last element entry, skipping trailing markers.
func (l *formattingList) lastElement() *dom.Element {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i] != nil {
			return l.entries[i]
		}
	}
	return nil
}

// bookmark marks a position in the list for the adoption agency algorithm.
type bookmark int

// bookmarkFor returns a bookmark for element e.
func (l *formattingList) bookmarkFor(e *dom.Element) bookmark {
	idx := l.find(e)
	if idx < 0 {
		idx = len(l.entries)
	}
	return bookmark(idx)
}

// insertAt inserts e at the bookmark position.
func (l *formattingList) insertAt(b bookmark, e *dom.Element) {
	idx := int(b)
	if idx > len(l.entries) {
		idx = len(l.entries)
	}
	l.entries = append(l.entries, nil)
	copy(l.entries[idx+1:], l.entries[idx:])
	l.entries[idx] = e
}

// removeUpdatingBookmark removes e and adjusts the bookmark to point at the same
// entry (or the entry that took its place), mirroring removeUpdatingBookmark.
func (l *formattingList) removeUpdatingBookmark(e *dom.Element, b *bookmark) {
	idx := l.find(e)
	if idx < 0 {
		return
	}
	l.entries = append(l.entries[:idx], l.entries[idx+1:]...)
	if int(*b) > idx {
		*b--
	}
}

// swapTo replaces oldElement with newElement at the bookmark position, mirroring
// swapTo. The new entry takes the place of the old.
func (l *formattingList) swapTo(oldElement, newElement *dom.Element) {
	idx := l.find(oldElement)
	if idx < 0 {
		return
	}
	l.entries[idx] = newElement
}

// entryAt returns the element (or nil for a marker) at index i.
func (l *formattingList) entryAt(i int) *dom.Element {
	if i < 0 || i >= len(l.entries) {
		return nil
	}
	return l.entries[i]
}
