//go:build darwin

// Translation of: NSTextInputClient protocol implementation for macOS IME
// Completeness: 60%
//
// This file implements the macOS IME handler via the NSTextInputClient
// protocol. The NSView subclass (GoCocoaView, defined in window_macos.go)
// implements the NSTextInputClient methods and pushes composition events
// to a C event queue. This handler reads from that queue and returns
// events via PopEvents().
//
// The handler receives the NSView pointer via Init(hwnd) and sets up the
// event queue pointer on the view's imeQueuePtr ivar.

package ime

/*
#cgo LDFLAGS: -framework Cocoa -framework CoreGraphics
#include <stdlib.h>
#include <string.h>
#include <objc/runtime.h>
#include <objc/message.h>

// IME event queue types — must match the definitions in window_macos.go
#define MAX_IME_EVENTS 64
#define MAX_IME_TEXT 512

typedef struct {
    int kind;       // 0=composition update, 1=char commit, 2=composition end
    char text[MAX_IME_TEXT];
    int cursor_pos;
} IMEEvent;

typedef struct {
    IMEEvent buf[MAX_IME_EVENTS];
    volatile int head, tail;
} IMEEventQueue;

// Create a new IMEEventQueue (heap-allocated)
static IMEEventQueue* ime_queue_create(void) {
    IMEEventQueue* q = (IMEEventQueue*)calloc(1, sizeof(IMEEventQueue));
    if (q) { q->head = 0; q->tail = 0; }
    return q;
}

// Free an IMEEventQueue
static void ime_queue_free(IMEEventQueue* q) { free(q); }

// Drain events from the queue into an array
static int ime_queue_drain(IMEEventQueue* q, IMEEvent* out, int max) {
    if (!q || !out || max <= 0) return 0;
    int count = 0;
    while (count < max && q->tail != q->head) {
        out[count] = q->buf[q->tail];
        q->tail = (q->tail + 1) % MAX_IME_EVENTS;
        __sync_synchronize();
        count++;
    }
    return count;
}

// Set the IMEEventQueue pointer on the NSView's imeQueuePtr ivar.
// The viewClass is GoCocoaView from window_macos.go; it has an
// "imeQueuePtr" ivar of type "^v".
static void ime_set_view_queue(id view, IMEEventQueue* q) {
    if (!view) return;
    Ivar ivar = class_getInstanceVariable(object_getClass(view), "imeQueuePtr");
    if (ivar) {
        object_setIvar(view, ivar, (void*)q);
    }
}
*/
import "C"

import (
	"unsafe"
)

// macosHandler is the macOS NSTextInputClient IME handler.
type macosHandler struct {
	view       unsafe.Pointer // NSView* (GoCocoaView)
	queue      *C.IMEEventQueue
	composing  bool
}

// NewHandler constructs a Handler for the current platform.
// On macOS this returns an NSTextInputClient-based handler.
// Init must be called with the NSView pointer before use.
func NewHandler() Handler { return &macosHandler{} }

// Init attaches the IME handler to the NSView. The hwnd parameter
// must be the NSView pointer (from window_macos.go's CocoaWindow.view).
func (h *macosHandler) Init(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	h.view = unsafe.Pointer(hwnd)
	// Create the event queue
	h.queue = C.ime_queue_create()
	// Set the queue pointer on the view's imeQueuePtr ivar
	if h.view != nil {
		C.ime_set_view_queue((C.id)(h.view), h.queue)
	}
}

func (h *macosHandler) PopEvents() []Event {
	if h.queue == nil {
		return nil
	}
	var buf [16]C.IMEEvent
	count := C.ime_queue_drain(h.queue, &buf[0], 16)

	events := make([]Event, 0, count)
	for i := 0; i < int(count); i++ {
		ce := buf[i]
		switch ce.kind {
		case 0: // composition update (setMarkedText:)
			h.composing = true
			text := C.GoString(&ce.text[0])
			events = append(events, Event{
				Kind:        EventCompositionUpdate,
				Composition: text,
				CursorPos:   int(ce.cursor_pos),
			})
		case 1: // char commit (insertText:)
			text := C.GoString(&ce.text[0])
			if len(text) > 0 {
				runes := []rune(text)
				for _, r := range runes {
					events = append(events, Event{
						Kind: EventCharInput,
						Char: r,
					})
				}
			}
		case 2: // composition end (unmarkText)
			h.composing = false
			events = append(events, Event{
				Kind: EventCompositionEnd,
			})
		}
	}
	return events
}

func (h *macosHandler) SetCompositionPos(x, y int32) {
	// On macOS, the candidate window position is handled by
	// firstRectForCharacterRange:actualRange: on the NSView.
	// If the window backend tracks caret position in screen
	// coordinates, the view's rect callback uses it.
	// For now, positioning updates are a no-op.
}

func (h *macosHandler) SetEnabled(enabled bool) {
	// NSTextInputContext activation/deactivation is handled
	// automatically by the NSView's respondsToFirstResponder
	// and the window's key/focus state. On macOS, explicitly
	// enabling/disabling IME is not needed: the input context
	// activates when the view becomes first responder and
	// has an active input method selected by the user.
}

func (h *macosHandler) IsComposing() bool {
	return h.composing
}
