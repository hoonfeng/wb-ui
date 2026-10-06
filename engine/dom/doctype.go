// Translation of: Source/WebCore/dom/DocumentType.h
//                  Source/WebCore/dom/DocumentType.cpp
// Completeness: 80%
// Simplifications:
//   - DocumentType embeds nodeBase (leaf); name/publicID/systemID are plain strings
//   - the entities/notations NamedNodeMaps are not modelled (the tokenizer never
//     produces them; HTML documents would carry two empty maps)
//   - remove() lives in the binding layer (DocumentType::remove idiom)

package dom

// DocumentType is the Go translation of WebCore::DocumentType: the leaf node that
// represents a document's <!DOCTYPE> declaration. It embeds nodeBase and — like
// Comment — can never have children (canHaveChildren() rejects NodeDocumentType).
//
// ★ 第 17 次监督轮新增。此前 engine/html 的 Initial insertion mode 把 DOCTYPE 伪装
// 成 Comment 节点（`<!--DOCTYPE html-->`）挂到 document 下，后果：
//   - document.firstChild 返回的是注释，浏览器里应是 DocumentType 节点；
//   - DocumentType 这个节点类型完全不存在（document.doctype 无从提供）。
//
// 浏览器语义（DOM §4.9）：DOCTYPE 是**独立的节点类型**（nodeType=10），nodeName 与
// name 都是 doctype 名字（HTML 下为 "html"），publicId/systemId 来自 tokenizer
// 解析出的两个标识符（HTML5 doctype 两者皆空）。
type DocumentType struct {
	nodeBase

	// name 是 doctype 名字（HTML 文档恒为 "html"）；publicID/systemID 是声明里的
	// 公开/系统标识符（HTML5 doctype 下都是空串）。
	name     string
	publicID string
	systemID string
}

// NewDocumentType creates a DocumentType node owned by doc. name is stored verbatim
// (the tokenizer passes the doctype name as written in the source).
func NewDocumentType(doc *Document, name, publicID, systemID string) *DocumentType {
	t := &DocumentType{name: name, publicID: publicID, systemID: systemID}
	t.initNodeBase(t, doc, NodeDocumentType)
	return t
}

// Name returns the doctype name, mirroring DocumentType::name().
func (t *DocumentType) Name() string { return t.name }

// PublicID returns the public identifier, mirroring DocumentType::publicId().
func (t *DocumentType) PublicID() string { return t.publicID }

// SystemID returns the system identifier, mirroring DocumentType::systemId().
func (t *DocumentType) SystemID() string { return t.systemID }

// NodeName for a DocumentType is its name — DOM §4.9.1: "The nodeName attribute of a
// DocumentType node is its name", with no case folding (unlike Element, which
// upper-cases for an HTML document).
func (t *DocumentType) NodeName() string { return t.name }

// NodeValue for a DocumentType is the empty string; the binding layer maps it to
// null, matching DOM §4.9.1 ("DocumentType.nodeValue is always null").
func (t *DocumentType) NodeValue() string { return "" }

// SetNodeValue has no effect on a DocumentType, mirroring Node::setNodeValue().
func (t *DocumentType) SetNodeValue(string) error { return nil }

// TextContent for a DocumentType is the empty string (null per the spec): a doctype
// is a leaf, so there is no subtree to concatenate.
func (t *DocumentType) TextContent() string { return "" }

// SetTextContent has no effect on a DocumentType — it has no children and no data,
// so there is nothing to replace.
func (t *DocumentType) SetTextContent(string) error { return nil }

// cloneShallow returns a copy carrying the same name and identifiers, mirroring
// cloneNodeInternal for DocumentType.
func (t *DocumentType) cloneShallow(doc *Document) Node {
	return NewDocumentType(doc, t.name, t.publicID, t.systemID)
}
