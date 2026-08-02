//go:build windows

// Translation of: Source/WebKit/WebKitLegacy/win/WebView.cpp  (IME message handling)
//                  Source/WebKit/UIProcess/win/WebView.cpp    (IME composition)
//                  GWui/app/ime_windows.go                     (Win32 subclassing reference)
// Completeness: 70%
//
// This file implements the Windows platform IME handler. Because GLFW does
// not expose WM_IME_COMPOSITION / WM_IME_STARTCOMPOSITION / WM_IME_ENDCOMPOSITION
// messages, the handler subclasses the GLFW window's Win32 HWND via
// SetWindowLongW(GWL_WNDPROC) to intercept IME messages directly.
//
// The subclassed window procedure:
//   - clears ISC_SHOWUICOMPOSITIONWINDOW on WM_IME_SETCONTEXT so the IME
//     does not draw its own composition window (we draw composition text
//     ourselves via the rendering pipeline);
//   - sets the composition/candidate window position on
//     WM_IME_STARTCOMPOSITION so the candidate list appears near the caret;
//   - reads GCS_COMPSTR (composition string) and GCS_RESULTSTR (committed
//     text) from ImmGetCompositionStringW on WM_IME_COMPOSITION;
//   - buffers the events for the main loop to poll via PopEvents.
//
// The events are buffered because the Win32 window procedure runs on the
// same thread as the GLFW main loop but outside the Go scheduler's control;
// buffering decouples the Win32 callback from the Go-side event dispatch.

package ime

import (
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// ============================================================================
// Win32 DLL handles and procedure pointers
// ============================================================================

var (
	imm32  = syscall.NewLazyDLL("imm32.dll")
	user32 = syscall.NewLazyDLL("user32.dll")

	procImmGetContext            = imm32.NewProc("ImmGetContext")
	procImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	procImmSetCompositionWindow  = imm32.NewProc("ImmSetCompositionWindow")
	procImmSetCandidateWindow    = imm32.NewProc("ImmSetCandidateWindow")
	procImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
	procImmAssociateContext      = imm32.NewProc("ImmAssociateContext")

	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW    = user32.NewProc("CallWindowProcW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procSendMessageW       = user32.NewProc("SendMessageW")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procClientToScreen     = user32.NewProc("ClientToScreen")
)

// ============================================================================
// Win32 message and flag constants
// ============================================================================

const (
	wmIMEStartComposition = 0x010D
	wmIMEEndComposition   = 0x010E
	wmIMEComposition      = 0x010F
	wmIMESetContext       = 0x0281
	wmIMENotify           = 0x0282
	wmIMERequest          = 0x0288
	wmChar                = 0x0102

	// WM_IME_REQUEST (0x0288) wParam request types.
	imrCompositionWindow   = 0x0001
	imrCandidateWindow     = 0x0002
	imrQueryCharPosition   = 0x0006

	gcsCompStr   = 0x0008
	gcsResultStr = 0x0800
	gcsCursorPos = 0x0080

	iscShowUICompositionWindow uint32 = 0x80000000
	iscShowUICandidateWindow   uint32 = 0x00000001

	cfsPoint         = 0x0002
	cfsCandidatePos  = 0x0004

	gwlWndProc = ^uintptr(3) // -4 as uintptr
)

// ============================================================================
// Win32 structures for IME positioning
// ============================================================================

type compositionForm struct {
	Style                          uint32
	X, Y                           int32
	RectLeft, RectTop              int32
	RectRight, RectBottom          int32
}

type candidateForm struct {
	Index                          uint32
	Style                          uint32
	X, Y                           int32
	RectLeft, RectTop              int32
	RectRight, RectBottom          int32
}

// winRect mirrors the Win32 RECT structure (four LONGs).
type winRect struct {
	Left, Top, Right, Bottom int32
}

// candidateWindowHeight estimates the height of the IME candidate window
// (one row of candidates + padding). TSF Microsoft Pinyin treats the point
// returned by IMR_QUERYCHARPOSITION as the candidate list's BOTTOM edge
// and expands the list upward, so the reported caret Y must be pushed down
// by this height for the list to render below the caret.
const candidateWindowHeight = 36

// imeCharPosition mirrors the Win32 IMECHARPOSITION structure, used by
// WM_IME_REQUEST / IMR_QUERYCHARPOSITION to ask the application for the
// on-screen position of a character in the composition string.
type imeCharPosition struct {
	Size     uint32 // dwSize — size of the structure
	CharPos  uint32 // dwCharPos — character index in the string
	PtX, PtY int32  // pt — SCREEN coordinates of the character (top-left)
	Hwnd     uintptr
}

// ============================================================================
// WindowsHandler implements the Handler interface on Windows.
// ============================================================================

// WindowsHandler is the Windows IME handler. It subclasses the GLFW window's
// HWND to intercept IME messages and buffers events for the main loop.
type WindowsHandler struct {
	mu sync.Mutex

	// hwnd is the subclassed window handle.
	hwnd uintptr
	// origWndProc is the original window procedure, saved before
	// subclassing so messages not handled by the IME proc are forwarded.
	origWndProc uintptr
	// subclassCallback is the Go callback registered as the new WndProc.
	subclassCallback uintptr

	// events buffers IME events from the subclassed window procedure.
	events []Event

	// composing tracks whether a composition is in progress
	// (WM_IME_STARTCOMPOSITION received, no END yet).
	composing bool

	// pendingChars holds the characters whose WM_CHAR duplicates may follow
	// an IME confirmation (GCS_RESULTSTR). WM_CHAR is dropped only when it
	// MATCHES the pending character — a stale count would swallow the next
	// real keystroke (typically Space) when the IME delivers fewer WM_CHARs
	// than expected (TSF often consumes them itself).
	pendingChars []rune

	// cached composition position (physical pixels) for WM_IME_STARTCOMPOSITION.
	compX, compY int32
	// compPosSet tracks whether compX/compY have been set.
	compPosSet bool

	// himc is the IME context handle saved during Init, used to re-associate
	// the context after SetEnabled(false) disassociates it. Without this,
	// the first Unfocus→Focus cycle permanently loses the IME context.
	himc uintptr
}

// NewHandler constructs a Handler appropriate for the current platform.
// On Windows this returns a *WindowsHandler; on other platforms a no-op stub.
func NewHandler() Handler {
	return &WindowsHandler{}
}

// Init subclasses the given HWND to intercept IME messages. It is idempotent.
func (h *WindowsHandler) Init(hwnd uintptr) {
	if hwnd == 0 || h.hwnd != 0 {
		return
	}
	h.hwnd = hwnd

	h.subclassCallback = syscall.NewCallback(h.imeWndProc)
	ret, _, _ := procSetWindowLongPtrW.Call(hwnd, uintptr(gwlWndProc), h.subclassCallback)
	h.origWndProc = ret
	if os.Getenv("WB_IME_DEBUG") != "" {
		log.Printf("[ime] Init hwnd=%#x SetWindowLongPtrW ret=%#x", hwnd, ret)
	}

	// Re-associate the IME context to force a WM_IME_SETCONTEXT so our
	// subclassed proc can clear ISC_SHOWUICOMPOSITIONWINDOW. The initial
	// WM_IME_SETCONTEXT (during window creation) was handled by GLFW before
	// we subclassed, so the composition window flag was not cleared.
	oldHimc, _, _ := procImmAssociateContext.Call(hwnd, 0)
	if oldHimc != 0 {
		h.himc = oldHimc
		procImmAssociateContext.Call(hwnd, oldHimc)
	}
}

// PopEvents returns and clears the buffered IME events.
func (h *WindowsHandler) PopEvents() []Event {
	h.mu.Lock()
	events := h.events
	h.events = nil
	h.mu.Unlock()
	return events
}

// SetCompositionPos updates the cached IME composition position (physical
// pixels). The position is used when WM_IME_STARTCOMPOSITION fires to place
// the composition and candidate windows near the text caret. While a
// composition is in progress the candidate window is also refreshed right
// away, so it follows the caret as it moves / the candidate list updates
// (previously the position was only applied once at composition start and
// the candidate list stayed at the old spot).
func (h *WindowsHandler) SetCompositionPos(x, y int32) {
	h.mu.Lock()
	h.compX = x
	h.compY = y
	h.compPosSet = true
	composing := h.composing
	hwnd := h.hwnd
	h.mu.Unlock()
	if composing && hwnd != 0 {
		h.setCompositionPos(hwnd, x, y)
	}
}

// SetEnabled enables or disables IME for the focused element.
func (h *WindowsHandler) SetEnabled(enabled bool) {
	if h.hwnd == 0 {
		return
	}
	if enabled {
		// Re-associate the saved IME context. After SetEnabled(false)
		// disassociated it, ImmAssociateContext(hwnd, 0) would return 0
		// (no context), so we must use the saved handle from Init.
		if h.himc != 0 {
			procImmAssociateContext.Call(h.hwnd, h.himc)
		}
		procSendMessageW.Call(h.hwnd, wmIMESetContext, 1, uintptr(iscShowUICandidateWindow))
	} else {
		procImmAssociateContext.Call(h.hwnd, 0)
	}
}

// IsComposing reports whether an IME composition is in progress.
func (h *WindowsHandler) IsComposing() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.composing
}

// pushEvent appends an IME event to the buffer (thread-safe, called from
// the subclassed window procedure).
func (h *WindowsHandler) pushEvent(ev Event) {
	h.mu.Lock()
	h.events = append(h.events, ev)
	h.mu.Unlock()
}

// ============================================================================
// Subclassed window procedure
// ============================================================================

// imeWndProc is the subclassed window procedure that intercepts IME messages.
// It is called by Win32 on the same thread that runs the GLFW event loop.
func (h *WindowsHandler) imeWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if os.Getenv("WB_IME_DEBUG") != "" {
		switch msg {
		case wmIMESetContext, wmIMEStartComposition, wmIMEComposition, wmIMEEndComposition, wmIMENotify, wmChar:
			log.Printf("[ime] wndproc msg=0x%x wParam=%#x lParam=%#x", msg, wParam, lParam)
		}
	}
	switch msg {
	case wmIMESetContext:
		// Clear ISC_SHOWUICOMPOSITIONWINDOW to suppress the IME's own
		// composition window (we draw composition text ourselves). Keep
		// ISC_SHOWUICANDIDATEWINDOW so the candidate list still shows.
		lParam &^= uintptr(iscShowUICompositionWindow)
		procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return 0

	case wmIMEStartComposition:
		h.mu.Lock()
		h.composing = true
		x, y := h.compX, h.compY
		h.mu.Unlock()
		h.setCompositionPos(hwnd, x, y)
		// Explicitly notify IME not to show the composition window.
		procDefWindowProcW.Call(hwnd, uintptr(wmIMESetContext), 1, uintptr(iscShowUICandidateWindow))
		return 0

	case wmIMEComposition:
		h.handleIMEComposition(hwnd, lParam)
		// Refresh the candidate window position as the composition text
		// changes (candidate list cycling), keeping it at the caret.
		if h.composing {
			h.mu.Lock()
			x, y := h.compX, h.compY
			h.mu.Unlock()
			h.setCompositionPos(hwnd, x, y)
		}
		// Re-hide the composition window after each composition update,
		// since some IMEs re-show it when the text changes.
		procDefWindowProcW.Call(hwnd, uintptr(wmIMESetContext), 1, uintptr(iscShowUICandidateWindow))
		return 0

	case wmIMEEndComposition:
		h.mu.Lock()
		h.composing = false
		h.pendingChars = nil
		h.mu.Unlock()
		h.pushEvent(Event{Kind: EventCompositionEnd})
		return 0

	case wmChar:
		ch := rune(wParam)
		// Drop only the WM_CHAR that duplicates a GCS_RESULTSTR-confirmed
		// character (character-matched, not counted). If the expected
		// duplicate never arrives (TSF consumes it), the pending slot stays
		// for one message and any OTHER real keystroke (e.g. Space) clears
		// it and is processed normally — a stale counter used to swallow
		// the next input entirely ("space does nothing").
		h.mu.Lock()
		if len(h.pendingChars) > 0 {
			if h.pendingChars[0] == ch {
				h.pendingChars = h.pendingChars[1:]
				h.mu.Unlock()
				return 0
			}
			h.pendingChars = nil
		}
		h.mu.Unlock()
		if ch >= 32 && ch != 127 {
			h.pushEvent(Event{Kind: EventCharInput, Char: ch})
		}
		return 0

	case wmIMENotify:
		return 0

	case wmIMERequest:
		// The IME asks the application for the composition/candidate window
		// position (WebKit's WebView.cpp onIMERequest handles
		// IMR_COMPOSITIONWINDOW / IMR_CANDIDATEWINDOW the same way). TSF
		// compatible IMEs (Microsoft Pinyin) query via this message, so the
		// candidate list follows the caret even when ImmSetCandidateWindow
		// alone is ignored. lParam points to the structure to fill; the
		// return value is the structure size in bytes.
		h.mu.Lock()
		x, y := h.compX, h.compY
		h.mu.Unlock()
		if os.Getenv("WB_IME_DEBUG") != "" {
			log.Printf("[ime] wmIMERequest wParam=%#x lParam=%#x pos=(%d,%d)", wParam, lParam, x, y)
		}
		switch wParam {
	case imrQueryCharPosition:
		// TSF-compatible IMEs (Microsoft Pinyin) ask for the character's
		// SCREEN position via IMR_QUERYCHARPOSITION; the candidate list
		// is positioned from this. This is the primary mechanism that
		// makes the candidate window follow the caret — ImmSetCandidateWindow
		// alone is ignored by TSF IMEs.
		if lParam != 0 {
			cp := (*imeCharPosition)(unsafe.Pointer(lParam))
			var pt struct{ X, Y int32 }
			procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			cp.PtX = pt.X + x
			// The reported point is the anchor for the candidate window's
			// BOTTOM edge (TSF Microsoft Pinyin expands the list upward from
			// it). To place the list BELOW the caret we must push Y down by
			// the candidate window's own height — the caret-bottom anchor
			// alone left the list flush with (covering) the caret line.
			cp.PtY = pt.Y + y + candidateWindowHeight
			cp.Hwnd = hwnd
			if os.Getenv("WB_IME_DEBUG") != "" {
				log.Printf("[ime] IMR_QUERYCHARPOSITION charPos=%d screen=(%d,%d) hwnd=%#x", cp.CharPos, cp.PtX, cp.PtY, hwnd)
			}
			return uintptr(unsafe.Sizeof(imeCharPosition{}))
		}
		return 0
		case imrCompositionWindow:
			if lParam != 0 {
				f := (*compositionForm)(unsafe.Pointer(lParam))
				f.Style = cfsPoint
				f.X, f.Y = x, y
				return uintptr(unsafe.Sizeof(compositionForm{}))
			}
		case imrCandidateWindow:
			if lParam != 0 {
				f := (*candidateForm)(unsafe.Pointer(lParam))
				f.Index = 0
				f.Style = cfsCandidatePos
				f.X, f.Y = x, y + candidateWindowHeight
				return uintptr(unsafe.Sizeof(candidateForm{}))
			}
		}
		return 0
	}

	return h.callOldWndProc(hwnd, msg, wParam, lParam)
}

func (h *WindowsHandler) callOldWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if h.origWndProc == 0 {
		return 0
	}
	ret, _, _ := procCallWindowProcW.Call(h.origWndProc, hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// ============================================================================
// IME composition string reading
// ============================================================================

// handleIMEComposition processes WM_IME_COMPOSITION, reading the
// composition string (GCS_COMPSTR) and/or result string (GCS_RESULTSTR)
// from the IME context.
func (h *WindowsHandler) handleIMEComposition(hwnd uintptr, lParam uintptr) {
	if lParam&uintptr(gcsResultStr) != 0 {
		h.pushEvent(Event{Kind: EventCompositionEnd})
		result := h.getCompositionString(hwnd, gcsResultStr)
		if result != "" {
			for _, ch := range result {
				h.pushEvent(Event{Kind: EventCharInput, Char: ch})
			}
			// Track the exact confirmed characters whose duplicate WM_CHAR
			// messages may follow (see wmChar). Matched against the actual
			// WM_CHAR so a missing duplicate never swallows the next real
			// keystroke.
			h.mu.Lock()
			h.pendingChars = append(h.pendingChars, []rune(result)...)
			h.mu.Unlock()
		}
	}

	if lParam&uintptr(gcsCompStr) != 0 {
		comp := h.getCompositionString(hwnd, gcsCompStr)
		cursorPos := h.getCursorPos(hwnd)
		h.pushEvent(Event{Kind: EventCompositionUpdate, Composition: comp, CursorPos: cursorPos})
	}
}

// getCompositionString reads a composition string of the given format
// (GCS_COMPSTR or GCS_RESULTSTR) from the IME context.
func (h *WindowsHandler) getCompositionString(hwnd uintptr, format uint32) string {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return ""
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	size, _, _ := procImmGetCompositionStringW.Call(himc, uintptr(format), 0, 0)
	if size <= 0 {
		return ""
	}

	buf := make([]uint16, size/2)
	procImmGetCompositionStringW.Call(himc, uintptr(format), uintptr(unsafe.Pointer(&buf[0])), size)
	return syscall.UTF16ToString(buf)
}

// getCursorPos reads the composition cursor position (GCS_CURSORPOS).
func (h *WindowsHandler) getCursorPos(hwnd uintptr) int {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return 0
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	var pos int32
	size, _, _ := procImmGetCompositionStringW.Call(himc, gcsCursorPos, uintptr(unsafe.Pointer(&pos)), 4)
	if size > 0 {
		return int(pos)
	}
	return 0
}

// ============================================================================
// IME window position setting
// ============================================================================

// setCompositionPos sets the IME composition and candidate window position
// to the given coordinates. The incoming x/y are relative to the window's
// client area (page caret position); ImmSetCompositionWindow /
// ImmSetCandidateWindow require SCREEN coordinates, so ClientToScreen
// converts them first. Without this the candidate window showed at the
// screen top-left corner (offset missing = client-origin treated as
// screen-origin).
func (h *WindowsHandler) setCompositionPos(hwnd uintptr, x, y int32) {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		if os.Getenv("WB_IME_DEBUG") != "" {
			log.Printf("[ime] setCompositionPos: ImmGetContext failed hwnd=%#x", hwnd)
		}
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	// The incoming x/y are CLIENT-AREA coordinates (physical pixels relative
	// to the window's client origin), matching the caret position computed by
	// FormControlCaretPosition and scaled by Window.SetIMECompositionPos.
	// ImmSetCompositionWindow / ImmSetCandidateWindow expect coordinates
	// relative to the window client area — NOT screen coordinates. Converting
	// with ClientToScreen pushed the candidate list down-right by the window's
	// client-area offset on screen ("candidate drifts right/down of caret").
	// WebKit (WebView.cpp IME handling) passes the caret rect as-is.
	if os.Getenv("WB_IME_DEBUG") != "" {
		// For screen-coordinate verification: window rect (screen) + client
		// origin => caret's actual screen position, to compare against the
		// candidate window's observed position on screen.
		var wr winRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
		log.Printf("[ime] setCompositionPos client=(%d,%d) winRect=(%d,%d,%d,%d) caretScreen≈(%d,%d)",
			x, y, wr.Left, wr.Top, wr.Right, wr.Bottom,
			wr.Left+int32(x), wr.Top+int32(y))
	}

	cf := compositionForm{
		Style: cfsPoint,
		X:     x,
		Y:     y,
	}
	r1, _, _ := procImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&cf)))

	// CFS_CANDIDATEPOS (not CFS_POINT) for the candidate window: CFS_POINT
	// ties the candidate list to the composition window position, and
	// Microsoft Pinyin renders its (tall) composition box above the caret,
	// pushing the candidate list down-right of the caret. CFS_CANDIDATEPOS
	// anchors the candidate list directly at the caret (Chromium's approach),
	// so it hugs the caret instead of drifting right/down.
	cand := candidateForm{
		Index: 0,
		Style: cfsCandidatePos,
		X:     x,
		Y:     y,
	}
	r2, _, _ := procImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&cand)))
	if os.Getenv("WB_IME_DEBUG") != "" {
		log.Printf("[ime] setCompositionPos client=(%d,%d) hwnd=%#x ImmSetComposition=%d ImmSetCandidate=%d",
			x, y, hwnd, r1, r2)
	}
}
