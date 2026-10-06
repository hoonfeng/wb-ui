// Translation of: Source/WebCore/dom/Attr.h
//                  Source/WebCore/dom/Attr.cpp
// Completeness: 70%
// Simplifications:
//   - Attr embeds nodeBase (nodeType 2) but is NOT part of the child node list:
//     Element.attributes exposes it, ChildNodes() never does (DOM §4.9: an attribute
//     is an "attribute" node, not a child of its element)
//   - while attached, the Attr is a **live reflector** of the element's attribute
//     storage: Value() reads el.attrs[name] and SetValue() writes it through
//     el.SetAttribute, so the element and the Attr can never disagree. Only a detached
//     Attr keeps a private value copy (that is what createAttribute returns)
//   - the attribute's namespace bookkeeping lives in Element (see nsAttrs in
//     element.go): Attr only mirrors it for namespaceURI/prefix/localName reporting
//   - EntityReference/notation hooks, Attr-level mutation records and the XML
//     "attribute value is a node tree" model are omitted (values are plain strings)

package dom

// Attr is the Go translation of WebCore::Attr: the node type representing an
// attribute of an Element (nodeType=2).
//
// ★ 第 18 次监督轮新增：此前绑定层只注册了一个空的 Attr 构造器（`new Attr()` 可用），
// 但没有真实的 Attr 节点对象——document.createAttribute() 无从实现，
// `element.getAttributeNode()` 也没有可返回的类型。这里补上建模；命名空间信息
// （namespace/prefix/localName）在 setAttributeNS 路径上随属性一起记录。
type Attr struct {
	nodeBase

	// namespace 是属性的命名空间 URI（普通属性为空串，对应 IDL 的 null）；
	// prefix 是限定名的前缀（"xlink"），localName 是去掉前缀的本地名（"href"）；
	// name 是限定名本身（HTML 文档里已小写化）。
	namespace string
	prefix    string
	localName string
	name      string

	// value 只在**游离** Attr（ownerElement == nil）上有效；挂载后一律以
	// ownerElement 的属性表为准（见 Value/SetValue）。
	value string

	ownerElement *Element
}

// NewAttr creates a detached Attr node owned by doc with the given (qualified) name.
// It mirrors Document::createAttribute(name).
func NewAttr(doc *Document, name, value string) *Attr {
	lowered := lowerAttrName(name)
	prefix, local := splitQualifiedAttrName(lowered)
	a := &Attr{name: lowered, prefix: prefix, localName: local, value: value}
	a.initNodeBase(a, doc, NodeAttribute)
	return a
}

// NewAttrNS creates a detached Attr node with an explicit namespace, mirroring the
// "create an attribute" step of Document::setAttributeNS / createAttributeNS.
func NewAttrNS(doc *Document, namespace, qualifiedName, value string) *Attr {
	a := NewAttr(doc, qualifiedName, value)
	a.namespace = namespace
	return a
}

// splitQualifiedAttrName splits a qualified attribute name into (prefix, localName).
// With no colon the whole name is the local name and the prefix is empty.
func splitQualifiedAttrName(qualified string) (prefix, local string) {
	if i := indexByte(qualified, ':'); i >= 0 {
		return qualified[:i], qualified[i+1:]
	}
	return "", qualified
}

// indexByte is a tiny local wrapper (avoids pulling strings into this file's imports
// for one call site; strings.IndexByte 语义一致).
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// Name returns the attribute's qualified name, mirroring Attr::name().
func (a *Attr) Name() string { return a.name }

// NodeName for an Attr is its qualified name (DOM §4.9.1).
func (a *Attr) NodeName() string { return a.name }

// NamespaceURI returns the namespace URI, mirroring Attr::namespaceURI().
func (a *Attr) NamespaceURI() string { return a.namespace }

// Prefix returns the namespace prefix, mirroring Attr::prefix().
func (a *Attr) Prefix() string { return a.prefix }

// LocalName returns the local name, mirroring Attr::localName().
func (a *Attr) LocalName() string { return a.localName }

// OwnerElement returns the element this attribute belongs to, mirroring
// Attr::ownerElement(). It is nil for a detached attribute.
func (a *Attr) OwnerElement() *Element { return a.ownerElement }

// Value returns the attribute value, mirroring Attr::value(). While the attribute is
// attached the value is read back from the owning element (single source of truth).
func (a *Attr) Value() string {
	if a.ownerElement != nil {
		return a.ownerElement.GetAttribute(a.name)
	}
	return a.value
}

// NodeValue for an Attr is its value (DOM §4.9.1).
func (a *Attr) NodeValue() string { return a.Value() }

// TextContent for an Attr is its value (DOM §4.9.1).
func (a *Attr) TextContent() string { return a.Value() }

// SetValue sets the attribute value, mirroring Attr::setValue(). On an attached
// attribute the write goes through the owning element so that styles, selectors and
// serialisation all observe it (and the value-change hooks fire）。
func (a *Attr) SetValue(v string) {
	a.value = v
	if a.ownerElement != nil {
		a.ownerElement.SetAttribute(a.name, v)
	}
}

// SetNodeValue mirrors Node::setNodeValue() for an Attr (it is its value).
func (a *Attr) SetNodeValue(s string) error { a.SetValue(s); return nil }

// SetTextContent mirrors Node::setTextContent() for an Attr (it is its value).
func (a *Attr) SetTextContent(s string) error { a.SetValue(s); return nil }

// IsEqualNode reports structural equality for attributes: same qualified name, same
// namespace, same value (mirroring Node::isEqualNode for Attr nodes).
func (a *Attr) IsEqualNode(other Node) bool {
	o, ok := other.(*Attr)
	if !ok || o == nil {
		return false
	}
	return a.name == o.name && a.namespace == o.namespace && a.Value() == o.Value()
}

// attachTo binds the attribute to el (used by the binding layer when an Attr created
// by createAttribute is handed to setAttributeNode-like flows). The value currently
// held by the Attr is written into the element.
func (a *Attr) attachTo(el *Element) {
	a.ownerElement = el
	if el != nil {
		el.SetAttributeNS(a.namespace, a.name, a.Value())
	}
}

// detach unties the attribute from its element, keeping the last value locally.
func (a *Attr) detach() {
	if a.ownerElement != nil {
		a.value = a.ownerElement.GetAttribute(a.name)
		a.ownerElement = nil
	}
}

// cloneShallow returns a detached copy carrying the same name/value/namespace,
// mirroring cloneNodeInternal for Attr.
func (a *Attr) cloneShallow(doc *Document) Node {
	c := NewAttrNS(doc, a.namespace, a.name, a.Value())
	return c
}
