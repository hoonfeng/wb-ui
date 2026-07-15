// Translation of: Source/WebCore/dom/Comment.h
//                  Source/WebCore/dom/Comment.cpp
// Completeness: 85%
// Simplifications:
//   - Comment embeds nodeBase (leaf) + characterDataBase like Text
//   - no renderer creation path

package dom

// Comment is the Go translation of WebCore::Comment. It is a leaf character-data node
// whose data is the comment text. It embeds nodeBase and characterDataBase, inheriting
// the data-manipulation API (Data/SubstringData/InsertData/...).
type Comment struct {
	nodeBase
	characterDataBase
}

// NewComment creates a Comment node owned by doc with the given data.
func NewComment(doc *Document, data string) *Comment {
	c := &Comment{characterDataBase: characterDataBase{data: data}}
	c.initNodeBase(c, doc, NodeComment)
	return c
}

// NodeName returns "#comment", mirroring Comment::nodeName().
func (c *Comment) NodeName() string { return "#comment" }

// NodeValue returns the data, mirroring CharacterData::nodeValue().
func (c *Comment) NodeValue() string { return c.data }

// SetNodeValue sets the data, mirroring CharacterData::setNodeValue().
func (c *Comment) SetNodeValue(s string) error { c.SetData(s); return nil }

// cloneShallow returns a copy with the same data, mirroring cloneNodeInternal.
func (c *Comment) cloneShallow(doc *Document) Node { return NewComment(doc, c.data) }
