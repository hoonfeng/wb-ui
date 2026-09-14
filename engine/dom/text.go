// Translation of: Source/WebCore/dom/Text.h
//                  Source/WebCore/dom/Text.cpp
//                  Source/WebCore/dom/CharacterData.h
//                  Source/WebCore/dom/CharacterData.cpp
// Completeness: 80%
// Simplifications:
//   - CharacterData is folded into a characterDataBase struct embedded by Text/Comment
//   - data offsets/lengths count Unicode runes rather than UTF-16 code units (correct
//     for BMP, approximate for astral characters)
//   - wholeText replaces the live text node run with a plain string join

package dom

// characterDataBase holds the text payload shared by Text and Comment and implements
// the CharacterData data-manipulation API (data, length, substringData, appendData,
// insertData, deleteData, replaceData). It mirrors WebCore::CharacterData.
type characterDataBase struct {
	data string
}

// Data returns the node's data, mirroring CharacterData::data().
func (c *characterDataBase) Data() string { return c.data }

// SetData replaces the data, mirroring CharacterData::setData().
func (c *characterDataBase) SetData(s string) { c.data = s }

// Length returns the length of the data in runes, mirroring CharacterData::length().
func (c *characterDataBase) Length() int {
	return runeCount(c.data)
}

// SubstringData returns count runes starting at offset, mirroring
// CharacterData::substringData(offset, count). Out-of-range offsets are clamped.
func (c *characterDataBase) SubstringData(offset, count int) string {
	rs := []rune(c.data)
	if offset < 0 {
		offset = 0
	}
	if offset > len(rs) {
		offset = len(rs)
	}
	end := offset + count
	if count < 0 || end > len(rs) {
		end = len(rs)
	}
	return string(rs[offset:end])
}

// AppendData appends s, mirroring CharacterData::appendData(s).
func (c *characterDataBase) AppendData(s string) { c.data += s }

// InsertData inserts s at rune offset, mirroring CharacterData::insertData(offset, s).
func (c *characterDataBase) InsertData(offset int, s string) {
	rs := []rune(c.data)
	if offset < 0 {
		offset = 0
	}
	if offset > len(rs) {
		offset = len(rs)
	}
	c.data = string(rs[:offset]) + s + string(rs[offset:])
}

// DeleteData removes count runes starting at offset, mirroring
// CharacterData::deleteData(offset, count).
func (c *characterDataBase) DeleteData(offset, count int) {
	rs := []rune(c.data)
	if offset < 0 {
		offset = 0
	}
	if offset > len(rs) {
		offset = len(rs)
	}
	end := offset + count
	if count < 0 || end > len(rs) {
		end = len(rs)
	}
	c.data = string(rs[:offset]) + string(rs[end:])
}

// ReplaceData replaces count runes at offset with s, mirroring
// CharacterData::replaceData(offset, count, s).
func (c *characterDataBase) ReplaceData(offset, count int, s string) {
	rs := []rune(c.data)
	if offset < 0 {
		offset = 0
	}
	if offset > len(rs) {
		offset = len(rs)
	}
	end := offset + count
	if count < 0 || end > len(rs) {
		end = len(rs)
	}
	c.data = string(rs[:offset]) + s + string(rs[end:])
}

// runeCount returns the number of runes in s.
func runeCount(s string) int { return len([]rune(s)) }

// Text is the Go translation of WebCore::Text. It embeds nodeBase (as a leaf node) and
// characterDataBase (for the text payload). SplitText divides the node into two
// siblings; WholeText returns the concatenation of adjacent text siblings.
type Text struct {
	nodeBase
	characterDataBase
}

// NewText creates a Text node owned by doc with the given data.
func NewText(doc *Document, data string) *Text {
	t := &Text{characterDataBase: characterDataBase{data: data}}
	t.initNodeBase(t, doc, NodeText)
	return t
}

// NodeName returns "#text", mirroring Text::nodeName().
func (t *Text) NodeName() string { return "#text" }

// NodeValue returns the data, mirroring CharacterData::nodeValue().
func (t *Text) NodeValue() string { return t.data }

// SetNodeValue sets the data, mirroring CharacterData::setNodeValue().
func (t *Text) SetNodeValue(s string) error {
	t.SetData(s)
	t.notifyTreeChange()
	return nil
}

// cloneShallow returns a copy with the same data, mirroring cloneNodeInternal.
func (t *Text) cloneShallow(doc *Document) Node { return NewText(doc, t.data) }

// SplitText splits the node at rune offset and returns the new node containing the
// tail, mirroring Text::splitText(offset). The original node keeps the prefix.
func (t *Text) SplitText(offset int) (*Text, error) {
	rs := []rune(t.data)
	if offset < 0 || offset > len(rs) {
		return nil, ErrIndexSize
	}
	tail := string(rs[offset:])
	t.data = string(rs[:offset])
	if t.parentNode != nil {
		next := NewText(t.ownerDoc, tail)
		_ = t.parentNode.InsertBefore(next, t.nextSibling)
		return next, nil
	}
	return NewText(t.ownerDoc, tail), nil
}

// WholeText returns the concatenation of this node and its adjacent text-node siblings,
// mirroring Text::wholeText().
func (t *Text) WholeText() string {
	var sb []byte
	// Walk backwards to the first adjacent text node.
	first := Node(t)
	for p := t.prevSibling; p != nil && p.NodeType() == NodeText; p = nodeBaseOf(p).prevSibling {
		first = p
	}
	for n := first; n != nil && n.NodeType() == NodeText; n = nodeBaseOf(n).nextSibling {
		sb = append(sb, n.NodeValue()...)
	}
	return string(sb)
}
