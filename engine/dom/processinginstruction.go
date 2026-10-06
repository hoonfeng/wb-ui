// Translation of: Source/WebCore/dom/ProcessingInstruction.h
//                  Source/WebCore/dom/ProcessingInstruction.cpp
//                  Source/WebCore/dom/CDATASection.h
//                  Source/WebCore/dom/CDATASection.cpp
// Completeness: 75%
// Simplifications:
//   - ProcessingInstruction embeds nodeBase (leaf) + characterDataBase, like Comment;
//     target is a plain string and the `sheet` link of xml-stylesheet PIs is omitted
//   - CDATASection is a pure CharacterData leaf; the HTML parser never produces one
//     (HTML has no CDATA sections — the tokenizer only synthesises them inside
//     foreign content), so it is reachable only through the DOM API on a
//     non-HTML document
//   - EntityReference/notation children of a PI are omitted

package dom

import "strings"

// ErrInvalidCharacter mirrors WebCore's InvalidCharacterError, raised by the DOM
// factories that validate their input (createProcessingInstruction, createElement with
// an invalid name, ...).
var ErrInvalidCharacter = errInvalidCharacter{}

type errInvalidCharacter struct{}

func (errInvalidCharacter) Error() string { return "dom: InvalidCharacterError" }

// isValidProcessingInstructionTarget reports whether target is a valid processing
// instruction target per DOM §4.9.6: a valid XML Name that is not an ASCII
// case-insensitive match for "xml" (the reserved target).
func isValidProcessingInstructionTarget(target string) bool {
	if target == "" || strings.EqualFold(target, "xml") {
		return false
	}
	for i := 0; i < len(target); i++ {
		c := target[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= 0x80:
			// 非 ASCII 字节：XML Name 允许 Unicode 字母，交给上层按 UTF-8 处理。
		case i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}

// ProcessingInstruction is the Go translation of WebCore::ProcessingInstruction: the
// leaf node type (nodeType=7) holding a "<?target data?>" instruction.
//
// ★ 第 18 次监督轮新增：document.createProcessingInstruction() 此前完全缺失
// （ProcessingInstruction 节点类型不存在）。它由 target（节点名）与 data（节点值）
// 两部分组成，两者都是 CharacterData 语义的数据。
type ProcessingInstruction struct {
	nodeBase
	characterDataBase

	// target 是 PI 的目标名（"<?xml-stylesheet ...?>" 里的 "xml-stylesheet"），
	// 也是它的 nodeName；按 DOM §4.9.1 它是只读的。
	target string
}

// NewProcessingInstruction creates a ProcessingInstruction node owned by doc.
func NewProcessingInstruction(doc *Document, target, data string) *ProcessingInstruction {
	p := &ProcessingInstruction{target: target, characterDataBase: characterDataBase{data: data}}
	p.initNodeBase(p, doc, NodeProcessingInstruction)
	return p
}

// Target returns the PI target, mirroring ProcessingInstruction::target().
func (p *ProcessingInstruction) Target() string { return p.target }

// NodeName for a ProcessingInstruction is its target.
func (p *ProcessingInstruction) NodeName() string { return p.target }

// NodeValue for a ProcessingInstruction is its data.
func (p *ProcessingInstruction) NodeValue() string { return p.data }

// SetNodeValue sets the data, mirroring CharacterData::setNodeValue().
func (p *ProcessingInstruction) SetNodeValue(s string) error {
	p.SetData(s)
	return nil
}

// cloneShallow returns a copy with the same target and data.
func (p *ProcessingInstruction) cloneShallow(doc *Document) Node {
	return NewProcessingInstruction(doc, p.target, p.data)
}

// CDATASection is the Go translation of WebCore::CDATASection: a CharacterData leaf
// (nodeType=4) whose node name is "#cdata-section".
type CDATASection struct {
	nodeBase
	characterDataBase
}

// NewCDATASection creates a CDATASection node owned by doc with the given data.
func NewCDATASection(doc *Document, data string) *CDATASection {
	c := &CDATASection{characterDataBase: characterDataBase{data: data}}
	c.initNodeBase(c, doc, NodeCDATASection)
	return c
}

// NodeName returns "#cdata-section", mirroring CDATASection::nodeName().
func (c *CDATASection) NodeName() string { return "#cdata-section" }

// NodeValue returns the data, mirroring CharacterData::nodeValue().
func (c *CDATASection) NodeValue() string { return c.data }

// SetNodeValue sets the data, mirroring CharacterData::setNodeValue().
func (c *CDATASection) SetNodeValue(s string) error {
	c.SetData(s)
	return nil
}

// cloneShallow returns a copy with the same data.
func (c *CDATASection) cloneShallow(doc *Document) Node { return NewCDATASection(doc, c.data) }
