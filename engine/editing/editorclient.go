// Translation of: Source/WebCore/page/EditorClient.h
//                  Source/WebCore/page/EditorClient.cpp
// Completeness: 40%
// Simplifications:
//   - only the IME-relevant subset of EditorClient is translated; spell
//     checking, grammar, undo/redo, pasteboard, attachment and platform-
//     specific text-replacement methods are omitted
//   - the C++ abstract base class is modeled as a Go interface
//   - EditorClient is owned by the Page (in WebKit) / by the WebView (in this
//     port); the platform IME handler calls back into it to drive composition
//
// EditorClient is the bridge between the platform input layer (IME, keyboard)
// and WebCore's editing subsystem. The platform intercepts native IME messages
// (WM_IME_COMPOSITION on Windows, NSTextInputClient on macOS, XIM/IBus on
// Linux) and translates them into EditorClient calls:
//
//   - setInputMethodState: enable/disable IME for the focused element
//   - handleInputMethodKeydown / handleKeyboardEvent: route key events
//   - discardedComposition / canceledComposition / didUpdateComposition:
//     notify the platform of composition lifecycle changes
//
// In this port the concrete implementation lives in the webkit package
// (WebView acts as its own EditorClient) and the platform IME handler in
// engine/platform/ime drives it via the interface.

package editing

import "wb-ui/engine/dom"

// EditorClient is the Go translation of the IME-relevant subset of
// WebCore::EditorClient. It is the interface through which the Editor and
// the platform input layer communicate about composition state.
type EditorClient interface {
	// setInputMethodState enables or disables IME for the focused element,
	// mirroring EditorClient::setInputMethodState(Element*). When the focused
	// element is editable (input/textarea/contenteditable), IME is enabled;
	// otherwise it is disabled so keystrokes go directly to the page.
	SetInputMethodState(focused *dom.Element)

	// handleKeyboardEvent routes a keyboard event to the editor, mirroring
	// EditorClient::handleKeyboardEvent(KeyboardEvent&). The platform calls
	// this for non-IME key events (e.g. arrow keys, backspace, Enter).
	HandleKeyboardEvent(ev *dom.KeyboardEvent)

	// handleInputMethodKeydown routes an IME-related keydown to the editor,
	// mirroring EditorClient::handleInputMethodKeydown(KeyboardEvent&). This
	// is called before normal key dispatch so the IME can consume the event.
	HandleInputMethodKeydown(ev *dom.KeyboardEvent)

	// discardedComposition notifies the platform that WebCore discarded a
	// composition voluntarily (not per an IME request), mirroring
	// EditorClient::discardedComposition(const Document&). The platform uses
	// this to clean up its own IME state.
	DiscardedComposition(doc *dom.Document)

	// canceledComposition notifies the platform that a composition was
	// cancelled, mirroring EditorClient::canceledComposition().
	CanceledComposition()

	// didUpdateComposition notifies the platform that composition state was
	// updated, mirroring EditorClient::didUpdateComposition(). The platform
	// uses this to refresh the IME candidate window position.
	DidUpdateComposition()
}
