// Package editor implements a CodeMirror 6-style code editor component for
// wb-ui. It provides an immutable document model (Text), selections,
// transactions, and an extension system, layered on top of the WebKit
// editing subsystem (Phase 0) for undo/redo.
//
// The architecture closely follows CodeMirror 6's state package
// (https://codemirror.net/docs/ref/#state), translated to Go:
//
//   - Text / TextLeaf / TextNode — immutable rope-like document model
//   - EditorSelection / Range — multi-cursor selection model
//   - ChangeSet / ChangeDesc — document change description
//   - Transaction — atomic state update
//   - EditorState — immutable state (doc + selection + fields)
//   - Extension / Facet / StateField — extension system
package editor
