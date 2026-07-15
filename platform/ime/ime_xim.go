//go:build linux

// Translation of: X11 XIM (X Input Method) IME handler
// Completeness: 60%
//
// This file implements the Linux XIM IME handler. XIM (X Input Method) is the
// traditional X11 protocol for CJK text input. Modern Linux desktops also
// support IBus and Fcitx via the XIM protocol bridge, so this handler works
// with most input method engines on Linux.
//
// The handler opens an XIM connection (via XOpenIM), creates an input context
// (XCreateIC) with preedit callbacks, and buffers composition events for the
// main loop to poll via PopEvents.

package ime

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xlocale.h>
#include <stdlib.h>
#include <string.h>

#define MAX_XIM_EVENTS 64
#define MAX_XIM_TEXT 256

typedef struct {
    int kind;              // 0=preedit update, 1=char commit, 2=preedit done
    char text[MAX_XIM_TEXT];
    int cursor_pos;
} XIMEvent;

typedef struct {
    XIMEvent buf[MAX_XIM_EVENTS];
    volatile int head;
    volatile int tail;
} XIMEventQueue;

static void xim_eq_push(XIMEventQueue* q, XIMEvent* e) {
    int next = (q->head + 1) % MAX_XIM_EVENTS;
    if (next == q->tail) return; // full
    q->buf[q->head] = *e;
    q->head = next;
    __sync_synchronize();
}

static int xim_eq_pop(XIMEventQueue* q, XIMEvent* e) {
    if (q->tail == q->head) return 0;
    *e = q->buf[q->tail];
    q->tail = (q->tail + 1) % MAX_XIM_EVENTS;
    __sync_synchronize();
    return 1;
}

// Per-instance data passed to XIM callbacks via the client_data pointer.
typedef struct {
    Display* display;
    Window   client_window;  // the X11 window that owns this IME
    XIM      im;
    XIC      ic;
    XIMEventQueue eq;
} XIMContext;

// XIM preedit callbacks (called from XFilterEvent / XmbLookupString)
static void xim_preedit_start(XIC ic, XPointer client_data, XPointer call_data) {
    (void)ic; (void)call_data;
    XIMContext* ctx = (XIMContext*)client_data;
    if (!ctx) return;
    XIMEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.kind = 0; // preedit update (start with empty text)
    xim_eq_push(&ctx->eq, &ev);
}

static void xim_preedit_draw(XIC ic, XPointer client_data, XIMPreeditDrawCallbackStruct* cb) {
    (void)ic;
    XIMContext* ctx = (XIMContext*)client_data;
    if (!ctx || !cb) return;
    XIMEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.kind = 0; // preedit update
    ev.cursor_pos = cb->caret;
    if (cb->text && cb->text->string.multi_byte) {
        int len = cb->text->length;
        if (len > MAX_XIM_TEXT - 1) len = MAX_XIM_TEXT - 1;
        strncpy(ev.text, cb->text->string.multi_byte, (size_t)len);
        ev.text[len] = '\0';
    }
    xim_eq_push(&ctx->eq, &ev);
}

static void xim_preedit_done(XIC ic, XPointer client_data, XPointer call_data) {
    (void)ic; (void)call_data;
    XIMContext* ctx = (XIMContext*)client_data;
    if (!ctx) return;
    XIMEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.kind = 2; // preedit done
    xim_eq_push(&ctx->eq, &ev);
}

// xim_init: initialise the XIM context for the given Display and client Window.
// Returns a XIMContext pointer, or NULL on failure. The context must be freed
// with xim_cleanup.
static XIMContext* xim_init(Display* dpy, Window win) {
    // Ensure locale is set for XIM
    setlocale(LC_ALL, "");

    XIMContext* ctx = (XIMContext*)calloc(1, sizeof(XIMContext));
    if (!ctx) return NULL;
    ctx->display = dpy;
    ctx->client_window = win;

    XIM im = XOpenIM(dpy, NULL, NULL, NULL);
    if (!im) {
        free(ctx);
        return NULL;
    }
    ctx->im = im;

    // Preedit callbacks
    XIMCallback start_cb, draw_cb, done_cb;
    start_cb.client_data = (XPointer)ctx;
    start_cb.callback = (XIMProc)xim_preedit_start;
    draw_cb.client_data = (XPointer)ctx;
    draw_cb.callback = (XIMProc)xim_preedit_draw;
    done_cb.client_data = (XPointer)ctx;
    done_cb.callback = (XIMProc)xim_preedit_done;

    XIC ic = XCreateIC(im,
        XNInputStyle,        XIMPreeditCallbacks | XIMStatusNothing,
        XNClientWindow,      win,
        XNFocusWindow,       win,
        XNPreeditStartCallback, &start_cb,
        XNPreeditDrawCallback,   &draw_cb,
        XNPreeditDoneCallback,   &done_cb,
        NULL
    );
    if (!ic) {
        XCloseIM(im);
        free(ctx);
        return NULL;
    }
    ctx->ic = ic;

    return ctx;
}

// xim_filter: pass an XEvent through the XIM filter. Returns 1 if the event
// was consumed by the IME (caller should skip normal key processing).
// Events not consumed by IME (XFilterEvent returns False) typically mean
// the key event should be treated as a plain keyboard event.
static int xim_filter(XIMContext* ctx, XEvent* ev) {
    if (!ctx || !ctx->ic) return 0;
    if (ev->type != KeyPress) return 0;
    return XFilterEvent(ev, ctx->client_window) ? 1 : 0;
}

// xim_lookup: call XmbLookupString to convert a KeyPress event to committed
// text. Returns the number of bytes written to buf (0 if no committed text),
// and sets *is_composing.
static int xim_lookup(XIMContext* ctx, XEvent* ev,
                      char* buf, int buf_len, int* is_composing) {
    if (!ctx || !ctx->ic || !buf) return 0;
    KeySym keysym;
    int status;
    int len = XmbLookupString(ctx->ic, (XKeyEvent*)ev, buf, buf_len, &keysym, &status);
    *is_composing = (status == XLookupChars || status == XLookupBoth);
    if (status == XLookupNone || status == XLookupKeySym) {
        return 0;
    }
    return len;
}

// xim_get_composing: returns 1 if a composition is in progress.
static int xim_get_composing(XIMContext* ctx) {
    if (!ctx || !ctx->ic) return 0;
    // Check if IC has pending preedit
    XIMStyles* styles = NULL;
    XGetICValues(ctx->ic, XNPreeditState, &styles, NULL);
    return styles ? 1 : 0;
}

// xim_set_focus: give IME focus (call when a text field receives focus)
static void xim_set_focus(XIMContext* ctx, int enabled) {
    if (!ctx || !ctx->ic) return;
    if (enabled) {
        XSetICFocus(ctx->ic);
    } else {
        XUnsetICFocus(ctx->ic);
    }
}

// xim_cleanup: release XIM resources and free the context.
static void xim_cleanup(XIMContext* ctx) {
    if (!ctx) return;
    if (ctx->ic) {
        XDestroyIC(ctx->ic);
        ctx->ic = NULL;
    }
    if (ctx->im) {
        XCloseIM(ctx->im);
        ctx->im = NULL;
    }
    free(ctx);
}

// xim_pop_events: drain the event queue, storing up to max events.
static int xim_pop_events(XIMContext* ctx, XIMEvent* out, int max) {
    if (!ctx || !out || max <= 0) return 0;
    int count = 0;
    XIMEvent ev;
    while (count < max && xim_eq_pop(&ctx->eq, &ev)) {
        out[count++] = ev;
    }
    return count;
}
*/
import "C"

import (
	"unsafe"
)

// ximHandler is the Linux XIM IME handler.
type ximHandler struct {
	ctx       *C.XIMContext
	composing bool

	events []Event
}

// NewHandler constructs a Handler for the current platform.
// On Linux this returns an XIM-based handler. Init must be called
// with the X11 Display* (as uintptr) before use.
func NewHandler() Handler { return &ximHandler{} }

// Init attaches the XIM handler to the X11 display. The hwnd parameter
// must be the X11 Display pointer (as uintptr), obtained from the X11
// window backend's C.Display field.
func (h *ximHandler) Init(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	dpy := (*C.Display)(unsafe.Pointer(hwnd))

	// We need the client window. For the ximHandler, we obtain it by
	// creating a temporary window just for IME setup, OR the caller
	// must set it separately. In practice, window_x11.go calls Init
	// and the window is already created. We'll extract the default
	// root window as a fallback and let the caller pass the window
	// via the hwnd upper bits, or use a separate method.

	// For now, use the default root window. In production, the host
	// application should call SetClientWindow after Init to set the
	// actual X11 Window ID.
	root := C.XDefaultRootWindow(dpy)
	h.ctx = C.xim_init(dpy, root)
}

// SetClientWindow sets the X11 client window for this IME context.
// This must be called after Init with the actual window handle from
// window_x11.go. If not called, the root window is used as fallback.
func (h *ximHandler) SetClientWindow(win uintptr) {
	if h.ctx == nil || win == 0 {
		return
	}
	h.ctx.client_window = C.Window(win)
	if h.ctx.ic != nil {
		C.XSetICFocus(h.ctx.ic)
	}
}

// FilterEvent passes an X11 event through the XIM filter. Returns true
// if the event was consumed by the IME (caller should skip normal key
// processing). Call this from the X11 window backend's event loop when
// processing KeyPress events.
//
// This is an extension to the standard Handler interface, needed because
// XIM requires active involvement in the event loop (XFilterEvent).
func (h *ximHandler) FilterEvent(ev unsafe.Pointer) bool {
	if h.ctx == nil {
		return false
	}
	// The caller passes a pointer to the C.XEvent struct
	xev := (*C.XEvent)(ev)
	return C.xim_filter(h.ctx, xev) != 0
}

// LookupString converts a KeyPress event to committed text via
// XmbLookupString. Returns the committed string and whether a
// composition is in progress. Call this when FilterEvent returns
// false (IME did not consume the event) OR when FilterEvent returns
// true and you need to check if there's committed text.
func (h *ximHandler) LookupString(ev unsafe.Pointer) (string, bool) {
	if h.ctx == nil {
		return "", false
	}
	xev := (*C.XEvent)(ev)
	var buf [256]C.char
	isComp := C.int(0)
	n := C.xim_lookup(h.ctx, xev, &buf[0], C.int(len(buf)), &isComp)
	if n > 0 {
		return C.GoString(&buf[0]), isComp != 0
	}
	return "", isComp != 0
}

func (h *ximHandler) PopEvents() []Event {
	if h.ctx == nil {
		return nil
	}
	var buf [16]C.XIMEvent
	count := C.xim_pop_events(h.ctx, &buf[0], 16)

	events := make([]Event, 0, count)
	for i := 0; i < int(count); i++ {
		ce := buf[i]
		switch ce.kind {
		case 0: // preedit update
			h.composing = true
			text := C.GoString(&ce.text[0])
			events = append(events, Event{
				Kind:        EventCompositionUpdate,
				Composition: text,
				CursorPos:   int(ce.cursor_pos),
			})
		case 1: // char commit
			text := C.GoString(&ce.text[0])
			if len(text) > 0 {
				// Take the first rune of committed text
				runes := []rune(text)
				events = append(events, Event{
					Kind: EventCharInput,
					Char: runes[0],
				})
			}
		case 2: // preedit done
			h.composing = false
			events = append(events, Event{
				Kind: EventCompositionEnd,
			})
		}
	}

	// Also drain any old events from our own queue if needed
	if len(h.events) > 0 {
		events = append(events, h.events...)
		h.events = nil
	}

	return events
}

func (h *ximHandler) SetCompositionPos(x, y int32) {
	// XIM preedit callbacks handle candidate window positioning
	// via the XIM server. For IBus/Fcitx, the position is handled
	// by the XIM protocol itself; explicit COMPOSITION_POSITION
	// requests are not needed.
}

func (h *ximHandler) SetEnabled(enabled bool) {
	if h.ctx == nil {
		return
	}
	if enabled {
		C.xim_set_focus(h.ctx, 1)
	} else {
		C.xim_set_focus(h.ctx, 0)
	}
}

func (h *ximHandler) IsComposing() bool {
	return h.composing
}
