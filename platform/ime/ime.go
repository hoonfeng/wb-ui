// Package ime provides Input Method Editor (IME) support for the wb-ui
// window system. It is the Go translation of the WebKit platform IME
// layer, comprising:
//
//   - EditorClient (Source/WebCore/page/EditorClient.h): the abstract
//     interface through which the platform drives composition state.
//   - Editor composition state (Source/WebCore/editing/Editor.cpp):
//     SetComposition / ConfirmComposition / CancelComposition.
//   - CompositionEvent (Source/WebCore/dom/CompositionEvent.h): the DOM
//     events dispatched during composition.
//   - Platform IME handler: intercepts native IME messages and translates
//     them into Editor calls.
//
// On Windows the platform handler subclasses the GLFW window's Win32
// HWND to intercept WM_IME_COMPOSITION / WM_IME_STARTCOMPOSITION /
// WM_IME_ENDCOMPOSITION / WM_CHAR messages, because GLFW does not
// expose IME composition events directly. The subclassed window
// procedure buffers IME events; the main loop polls them via PopEvents.
//
// On Linux the handler uses XIM (X Input Method) to support CJK text input
// via XFilterEvent / XmbLookupString, with preedit callbacks for composition
// state tracking.
//
// On macOS the handler implements the NSTextInputClient protocol on the
// Cocoa NSView (GoCocoaView), intercepting setMarkedText:/insertText:/
// unmarkText calls from the NSTextInputContext and buffering composition
// events for the main loop.

package ime

// EventKind identifies the kind of IME event buffered by the platform
// handler.
type EventKind int

const (
	// EventCompositionUpdate means the composition string changed
	// (intermediate pinyin/kana text updated). The Composition and
	// CursorPos fields carry the new text and caret offset.
	EventCompositionUpdate EventKind = iota
	// EventCharInput means a character was committed (either from an IME
	// confirmation via GCS_RESULTSTR, or a plain WM_CHAR key press).
	// The Char field carries the rune.
	EventCharInput
	// EventCompositionEnd means the composition was ended (either
	// confirmed or cancelled by the IME).
	EventCompositionEnd
)

// Event represents a single buffered IME event, collected from the
// platform window procedure and consumed by the main loop.
type Event struct {
	Kind        EventKind
	Composition string // for EventCompositionUpdate / EventCompositionEnd
	CursorPos   int    // for EventCompositionUpdate
	Char        rune   // for EventCharInput
}

// Handler is the cross-platform IME interface implemented by each
// platform's ime_*.go file. The window system calls these methods to
// initialize IME, poll buffered events, set the composition window
// position, and enable/disable IME for the focused element.
type Handler interface {
	// Init attaches the IME handler to the platform window. On Windows
	// this subclasses the HWND (passed as hwnd) to intercept WM_IME_*
	// messages. On other platforms hwnd is ignored. It is idempotent.
	Init(hwnd uintptr)

	// PopEvents returns and clears the buffered IME events.
	PopEvents() []Event

	// SetCompositionPos updates the IME composition/candidate window
	// position to the given screen coordinates (in physical pixels),
	// so the candidate list appears near the text caret.
	SetCompositionPos(x, y int32)

	// SetEnabled enables or disables IME for the focused element.
	// When disabled, keystrokes bypass the IME and go directly as
	// WM_CHAR / key events.
	SetEnabled(enabled bool)

	// IsComposing reports whether an IME composition is in progress
	// (WM_IME_STARTCOMPOSITION received, no END yet).
	IsComposing() bool
}
