// Translation of: CodeMirror 6 — packages/view/src/widget.ts
//                  https://github.com/codemirror/view/blob/main/src/widget.ts
//
// Completeness: 60%
// Differences from CM6:
//   - CM6 widgets produce DOM nodes; this port produces wb-ui dom.Node
//     (which are later rendered to Skia by the rendering pipeline).
//   - The `updateDOM` method is omitted in v1 (widgets are always recreated).
//   - The `destroy` method is omitted in v1.
//   - Methods use PascalCase (Go convention).
//
// Widgets are used to embed interactive or non-text content (e.g. checkboxes,
// fold markers, line breaking indicators) at a specific position in the
// document. They are created by DecorationWidget and DecorationReplace.

package editor

import "wb-ui/dom"

// WidgetType is the interface implemented by widget decorations. Each widget
// produces a dom.Node that is inserted into the editor's content DOM.
//
// A widget must implement Eq so that the view can avoid re-rendering
// unchanged widgets during updates.
type WidgetType interface {
	// ToDOM returns the DOM node to insert at the widget's position.
	// The `view` parameter is currently an interface{} to avoid a circular
	// dependency with the EditorView type (defined in Phase 4). It will
	// be typed as *EditorView once that type exists.
	ToDOM(view interface{}) dom.Node

	// Eq reports whether this widget is equivalent to `other`. If true,
	// the view reuses the existing DOM node instead of recreating it.
	Eq(other WidgetType) bool

	// IgnoreEvent reports whether a DOM event fired inside the widget
	// should be ignored by the editor's event handlers. Returning true
	// prevents the editor from treating the event as a cursor movement
	// or selection change.
	IgnoreEvent(event interface{}) bool
}

// BaseWidget provides a default Eq implementation (identity comparison)
// and IgnoreEvent (always false). Embed this in concrete widget types
// to satisfy the WidgetType interface without implementing every method.
type BaseWidget struct{}

// Eq defaults to false (never equal), forcing the view to always recreate
// the widget's DOM node. Override in concrete types for value-based equality.
func (BaseWidget) Eq(other WidgetType) bool { return false }

// IgnoreEvent defaults to false (all events are handled by the editor).
func (BaseWidget) IgnoreEvent(event interface{}) bool { return false }
