// Translation of: Source/WebCore/rendering/updating/RenderTreeUpdater.cpp
//                  Source/WebCore/rendering/updating/RenderTreeUpdater.h
// Completeness: 45%
// Simplifications:
//   - no invalidation scheduling; the Update method walks all registered dirty nodes
//     immediately and applies the three change kinds (insert / remove / style-change)
//   - no :before / :after pseudo-element generation
//   - no slot assignment / shadow DOM integration
//   - the update is a full rebuild of the affected subtree rather than a surgical
//     diff; this matches the simplification used by the layout package's BuildLayoutTree

package rendering

import (
	"wb-ui/dom"
	"wb-ui/style"
)

// ChangeKind mirrors the subset of RenderTreeUpdater::Update actions this port supports.
type ChangeKind int

const (
	// ChangeNone means no change is needed.
	ChangeNone ChangeKind = iota
	// ChangeInsert means a new DOM node was inserted and needs a render object.
	ChangeInsert
	// ChangeRemove means a DOM node was removed and its render object must be detached.
	ChangeRemove
	// ChangeStyle means the element's computed style changed; the render object's style
	// is updated and the subtree may be rebuilt.
	ChangeStyle
)

// PendingChange records a queued render-tree mutation.
type PendingChange struct {
	Node      dom.Node
	Kind      ChangeKind
	OldStyle  *style.ComputedStyle
	NewStyle  *style.ComputedStyle
}

// RenderTreeUpdater is the Go translation of WebCore::RenderTreeUpdater. It applies
// incremental DOM mutations to the render tree after the initial build. Mutations are
// queued via MarkInsert / MarkRemove / MarkStyleChange and flushed by Update.
type RenderTreeUpdater struct {
	view     *RenderView
	builder  *RenderTreeBuilder
	resolver *style.Resolver
	pending  []PendingChange
}

// NewRenderTreeUpdater constructs an updater for the given render view and style
// resolver.
func NewRenderTreeUpdater(view *RenderView, resolver *style.Resolver) *RenderTreeUpdater {
	return &RenderTreeUpdater{
		view:     view,
		resolver: resolver,
		builder:  NewRenderTreeBuilder(resolver),
	}
}

// MarkInsert queues the insertion of a new DOM node, mirroring the insert path in
// RenderTreeUpdater::updateRenderTree.
func (u *RenderTreeUpdater) MarkInsert(node dom.Node) {
	u.pending = append(u.pending, PendingChange{Node: node, Kind: ChangeInsert})
}

// MarkRemove queues the removal of a DOM node, mirroring the remove path.
func (u *RenderTreeUpdater) MarkRemove(node dom.Node) {
	u.pending = append(u.pending, PendingChange{Node: node, Kind: ChangeRemove})
}

// MarkStyleChange queues a style refresh for the given element, mirroring the
// style-change path.
func (u *RenderTreeUpdater) MarkStyleChange(el *dom.Element, newStyle *style.ComputedStyle) {
	old := u.resolver.ResolveElement(el)
	u.pending = append(u.pending, PendingChange{
		Node:     el,
		Kind:     ChangeStyle,
		OldStyle: old,
		NewStyle: newStyle,
	})
}

// Update flushes all pending changes, applying them to the render tree. This mirrors
// RenderTreeUpdater::updateRenderTree() which walks the dirty nodes and dispatches.
func (u *RenderTreeUpdater) Update() {
	if len(u.pending) == 0 {
		return
	}
	for _, ch := range u.pending {
		switch ch.Kind {
		case ChangeInsert:
			u.applyInsert(ch.Node)
		case ChangeRemove:
			u.applyRemove(ch.Node)
		case ChangeStyle:
			u.applyStyleChange(ch)
		}
	}
	u.pending = u.pending[:0]
	// Rebuild the layer tree to pick up any new compositing conditions.
	if u.view != nil && u.view.compositor != nil {
		u.view.compositor.BuildLayerTree(u.view)
	}
}

// applyInsert creates a render object for the inserted node and attaches it to the
// parent's render object. Text nodes become RenderText; elements resolve their style and
// dispatch to createRenderObject.
func (u *RenderTreeUpdater) applyInsert(node dom.Node) {
	if u.view == nil {
		return
	}
	parent := node.ParentNode()
	if parent == nil {
		return
	}
	parentRO := u.findRenderObject(parent)
	if parentRO == nil {
		return
	}
	switch v := node.(type) {
	case *dom.Element:
		cs := u.builder.resolveStyle(v)
		if cs.Display == style.DisplayNone {
			return
		}
		child := u.builder.createRenderObject(v, cs)
		if child == nil {
			return
		}
		u.builder.buildChildren(child, v)
		parentRO.AddChild(child, u.findBeforeChild(node))
		child.Dirty()
	case *dom.Text:
		rt := NewRenderText(v, inheritedStyle(parentRO.Style()))
		parentRO.AddChild(rt, u.findBeforeChild(node))
		rt.Dirty()
	}
}

// findBeforeChild returns the render object corresponding to the DOM node's next sibling,
// so the new child is inserted in the correct position.
func (u *RenderTreeUpdater) findBeforeChild(node dom.Node) RenderObject {
	for sib := node.NextSibling(); sib != nil; sib = sib.NextSibling() {
		if ro := u.findRenderObject(sib); ro != nil {
			return ro
		}
	}
	return nil
}

// applyRemove detaches the render object associated with the removed node.
func (u *RenderTreeUpdater) applyRemove(node dom.Node) {
	ro := u.findRenderObject(node)
	if ro == nil {
		return
	}
	parent := ro.Parent()
	if parent != nil {
		parent.RemoveChild(ro)
	}
}

// applyStyleChange updates the render object's style and marks the subtree dirty. When
// the display type changed in a way that requires a different render object type, the
// subtree is rebuilt.
func (u *RenderTreeUpdater) applyStyleChange(ch PendingChange) {
	ro := u.findRenderObject(ch.Node)
	if ro == nil {
		// Style change on a node that has no render object (e.g. display: none was
		// cleared). Re-insert.
		u.applyInsert(ch.Node)
		return
	}
	// If the display changed to none, remove the object.
	if ch.NewStyle != nil && ch.NewStyle.Display == style.DisplayNone {
		u.applyRemove(ch.Node)
		return
	}
	// Update the style and dirty the subtree.
	ro.SetStyle(ch.NewStyle)
	u.markSubtreeDirty(ro)
}

// markSubtreeDirty flags the object and all descendants as needing layout.
func (u *RenderTreeUpdater) markSubtreeDirty(ro RenderObject) {
	ro.Dirty()
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		u.markSubtreeDirty(c)
	}
}

// findRenderObject searches the render tree for the object associated with the given DOM
// node. This mirrors the node->render-object mapping maintained by WebKit's
// NodeRenderingContext. In this simplified port the search is a linear walk from the
// view root.
func (u *RenderTreeUpdater) findRenderObject(node dom.Node) RenderObject {
	if u.view == nil || node == nil {
		return nil
	}
	for cur := RenderObject(u.view); cur != nil; cur = cur.NextInPreOrder() {
		if cur.Node() == node {
			return cur
		}
	}
	return nil
}

// HasPendingChanges reports whether the updater has queued mutations.
func (u *RenderTreeUpdater) HasPendingChanges() bool { return len(u.pending) > 0 }

// PendingCount returns the number of queued mutations.
func (u *RenderTreeUpdater) PendingCount() int { return len(u.pending) }
