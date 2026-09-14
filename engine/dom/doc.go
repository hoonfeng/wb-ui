// Package dom implements the DOM (Source/WebCore/dom) in Go.
//
// It mirrors WebKit's Node/Element/Document tree, Event system, Range and
// TreeWalker/NodeIterator interfaces. C++ multiple inheritance is replaced
// by Go interfaces and struct embedding.
package dom
