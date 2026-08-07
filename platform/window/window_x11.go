//go:build linux

// Package window provides an X11-based window with a Skia raster backend
// surface. It is an alternative to the GLFW backend (window_glfw.go) for
// Linux systems where GLFW is unavailable or a pure X11 backend is preferred.
//
// The window creates an X11 window, attaches a Skia CPU raster surface for
// rendering, and presents via XPutImage. This avoids any dependency on GLFW
// or OpenGL, at the cost of GPU-accelerated compositing.

package window

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/keysym.h>
#include <X11/Xatom.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/platform/graphics"
	"wb-ui/platform/ime"
)

// EventType identifies the kind of input event.
type EventType int

const (
	EventMouseButton EventType = iota
	EventChar
	EventCursorMove
	EventCursorLeave // 鼠标移出窗口（清除 hover/光标残留）
	EventKey
	EventResize
	EventScroll
	EventDrop
	EventTouch
)

// Event represents a single input event collected from the window.
type Event struct {
	Type    EventType
	X, Y    float64
	Button  int // 1=left, 2=middle, 3=right
	Action  int // 0=release, 1=press
	Key     int // X11 keysym (lower 32 bits)
	Mods    int
	Width   int // for EventResize
	Height  int
	ScrollY float64  // for EventScroll (vertical wheel offset)
	ScrollX float64  // for EventScroll (horizontal wheel offset)
	DropFiles []string // for EventDrop
	// Touch
	TouchX, TouchY float64
	TouchID        int
	TouchAction    int // 0=end, 1=begin, 2=move
}

// Window is an X11 window with a Skia raster backend surface.
type Window struct {
	display *C.Display
	xwin    C.Window
	screen  int
	gc      C.GC

	width, height        int
	fbWidth, fbHeight    int
	contentScaleX, contentScaleY float64

	canvas  *graphics.Canvas

	events   []Event
	eventsMu sync.Mutex
	closeCallback func()
	shouldClose   bool

	ime ime.Handler

	atomDeleteWindow C.Atom

	// Xdnd atoms for drag & drop
	atomXdndAware     C.Atom
	atomXdndEnter     C.Atom
	atomXdndPosition  C.Atom
	atomXdndDrop      C.Atom
	atomXdndLeave     C.Atom
	atomXdndSelection C.Atom
	atomXdndTypeList  C.Atom
	atomXdndActionCopy C.Atom
	atomXdndURIList   C.Atom

	dropCallback  func([]string)
	dropFiles     []string
	dropVersion   int
	expectingDrop bool
}

// NewWindow creates an X11 window with the given dimensions and title.
func NewWindow(width, height int, title string) (*Window, error) {
	dpy := C.XOpenDisplay(nil)
	if dpy == nil {
		return nil, fmt.Errorf("window x11: cannot open X display (is $DISPLAY set?)")
	}

	screen := C.XDefaultScreen(dpy)
	root := C.XRootWindow(dpy, screen)

	xwin := C.XCreateSimpleWindow(dpy, root, 0, 0, C.uint(width), C.uint(height), 0, 0, 0xFFFFFF)

	titleC := C.CString(title)
	defer C.free(unsafe.Pointer(titleC))
	C.XStoreName(dpy, xwin, titleC)
	C.XSetIconName(dpy, xwin, titleC)

	C.XSelectInput(dpy, xwin,
		C.StructureNotifyMask|C.ExposureMask|C.ButtonPressMask|C.ButtonReleaseMask|
			C.PointerMotionMask|C.KeyPressMask|C.KeyReleaseMask)

	atomDelete := C.XInternAtom(dpy, C.CString("WM_DELETE_WINDOW"), C.False)
	C.XSetWMProtocols(dpy, xwin, &atomDelete, 1)

	// Initialize Xdnd atoms for drag & drop support
	atomXdndAware := C.XInternAtom(dpy, C.CString("XdndAware"), C.False)
	atomXdndEnter := C.XInternAtom(dpy, C.CString("XdndEnter"), C.False)
	atomXdndPosition := C.XInternAtom(dpy, C.CString("XdndPosition"), C.False)
	atomXdndDrop := C.XInternAtom(dpy, C.CString("XdndDrop"), C.False)
	atomXdndLeave := C.XInternAtom(dpy, C.CString("XdndLeave"), C.False)
	atomXdndSelection := C.XInternAtom(dpy, C.CString("XdndSelection"), C.False)
	atomXdndTypeList := C.XInternAtom(dpy, C.CString("XdndTypeList"), C.False)
	atomXdndActionCopy := C.XInternAtom(dpy, C.CString("XdndActionCopy"), C.False)
	atomXdndURIList := C.XInternAtom(dpy, C.CString("text/uri-list"), C.False)

	// Register as Xdnd aware (version 5)
	C.XChangeProperty(dpy, xwin, atomXdndAware, C.XA_ATOM, 32, C.PropModeReplace,
		(*C.uchar)(unsafe.Pointer(&atomXdndAware)), 1)

	gc := C.XCreateGC(dpy, xwin, 0, nil)
	C.XMapWindow(dpy, xwin)
	C.XFlush(dpy)

	gfxCanvas := graphics.NewCanvas(width, height)

	w := &Window{
		display: dpy,
		xwin:    xwin,
		screen:  screen,
		gc:      gc,
		canvas:  gfxCanvas,
		width:   width,
		height:  height,
		fbWidth: width, fbHeight: height,
		contentScaleX: 1.0, contentScaleY: 1.0,
		atomDeleteWindow:  atomDelete,
		atomXdndAware:     atomXdndAware,
		atomXdndEnter:     atomXdndEnter,
		atomXdndPosition:  atomXdndPosition,
		atomXdndDrop:      atomXdndDrop,
		atomXdndLeave:     atomXdndLeave,
		atomXdndSelection: atomXdndSelection,
		atomXdndTypeList:  atomXdndTypeList,
		atomXdndActionCopy: atomXdndActionCopy,
		atomXdndURIList:   atomXdndURIList,
		dropVersion:       5,
		ime:               ime.NewHandler(),
	}
	// Wire up the XIM IME handler. The Display pointer is passed as the
	// Init handle; the client window is set explicitly since XIM needs
	// the actual X11 Window for input context focus.
	if w.ime != nil {
		w.ime.Init(uintptr(unsafe.Pointer(dpy)))
		if setter, ok := w.ime.(interface{ SetClientWindow(uintptr) }); ok {
			setter.SetClientWindow(uintptr(xwin))
		}
	}
	w.detectDPI()
	return w, nil
}

// detectDPI sets contentScale from the X11 display's physical dimensions.
func (w *Window) detectDPI() {
	if w.display == nil {
		return
	}
	physWMM := C.XDisplayWidthMM(w.display, C.int(w.screen))
	physHMM := C.XDisplayHeightMM(w.display, C.int(w.screen))
	if physWMM <= 0 || physHMM <= 0 {
		return
	}
	pixelW := int(C.XDisplayWidth(w.display, C.int(w.screen)))
	pixelH := int(C.XDisplayHeight(w.display, C.int(w.screen)))
	if pixelW <= 0 || pixelH <= 0 {
		return
	}
	dpiX := float64(pixelW) / (float64(physWMM) / 25.4)
	dpiY := float64(pixelH) / (float64(physHMM) / 25.4)
	if dpiX > 0 {
		w.contentScaleX = dpiX / 96.0
	}
	if dpiY > 0 {
		w.contentScaleY = dpiY / 96.0
	}
	if w.contentScaleX < 1.0 {
		w.contentScaleX = 1.0
	}
	if w.contentScaleY < 1.0 {
		w.contentScaleY = 1.0
	}
}

// blitToWindow reads canvas pixels and blits to the X11 window.
func (w *Window) blitToWindow() {
	if w.canvas == nil || w.display == nil {
		return
	}
	fbW, fbH := w.fbWidth, w.fbHeight
	if fbW <= 0 || fbH <= 0 {
		return
	}
	pixels := w.canvas.Pixels()
	if len(pixels) < fbW*fbH*4 {
		return
	}
	ximg := C.XCreateImage(w.display, C.XDefaultVisual(w.display, C.int(w.screen)),
		24, C.ZPixmap, 0, nil, C.uint(fbW), C.uint(fbH), 32, 0)
	if ximg == nil {
		return
	}
	defer C.XDestroyImage(ximg)
	data := C.malloc(C.size_t(int(ximg.bytes_per_line) * fbH))
	if data == nil {
		return
	}
	ximg.data = (*C.char)(data)
	dst := unsafe.Slice((*byte)(data), int(ximg.bytes_per_line)*fbH)
	for y := 0; y < fbH; y++ {
		srcOff := y * fbW * 4
		dstOff := y * int(ximg.bytes_per_line)
		for x := 0; x < fbW && srcOff+x*4+3 < len(pixels); x++ {
			r := pixels[srcOff+x*4+0]
			g := pixels[srcOff+x*4+1]
			b := pixels[srcOff+x*4+2]
			dst[dstOff+x*4+0] = b // X11 native ZPixmap depth 24: BGR order
			dst[dstOff+x*4+1] = g
			dst[dstOff+x*4+2] = r
			dst[dstOff+x*4+3] = 0
		}
	}
	C.XPutImage(w.display, w.xwin, w.gc, ximg, 0, 0, 0, 0, C.uint(fbW), C.uint(fbH))
}

// GPUSurface returns nil on the X11 raster backend.
func (w *Window) GPUSurface() *skia.Surface          { return nil }
func (w *Window) GPUContext() *skia.DirectContext     { return nil }

// Canvas returns the CPU raster canvas backing this software-rendered window.
func (w *Window) Canvas() *graphics.Canvas { return w.canvas }

// Present blits the raster canvas to the X11 window.
func (w *Window) Present() {
	if w.canvas == nil || w.display == nil {
		return
	}
	w.blitToWindow()
	C.XFlush(w.display)
}

// Display draws a *skia.Image onto the X11 window. Creates a temporary
// graphics.Canvas from the image, reads pixels, and blits.
func (w *Window) Display(srcImg *skia.Image) {
	if srcImg == nil || w.display == nil {
		return
	}
	imgW, imgH := srcImg.Width(), srcImg.Height()
	if imgW <= 0 || imgH <= 0 {
		return
	}
	// Create a temporary raster surface matching image dimensions,
	// draw the image onto it, then read pixels via graphics.Canvas.
	tmpSurf, err := skia.NewRasterSurfaceN32Premul(imgW, imgH)
	if err != nil {
		return
	}
	defer tmpSurf.Release()
	tmpCvs := tmpSurf.Canvas()
	srcRect := skia.RectXYWH(0, 0, float32(imgW), float32(imgH))
	paint := skia.NewPaint()
	defer paint.Release()
	tmpCvs.DrawImageRect(srcImg, srcRect, srcRect, skia.SamplingLinear, paint)
	fromSurf := graphics.NewCanvasFromSurface(tmpSurf, imgW, imgH)
	if fromSurf == nil {
		return
	}
	defer fromSurf.Release()
	pixBuf := fromSurf.Pixels()
	if len(pixBuf) < imgW*imgH*4 {
		return
	}
	ximg := C.XCreateImage(w.display, C.XDefaultVisual(w.display, C.int(w.screen)),
		24, C.ZPixmap, 0, nil, C.uint(imgW), C.uint(imgH), 32, 0)
	if ximg == nil {
		return
	}
	defer C.XDestroyImage(ximg)
	data := C.malloc(C.size_t(int(ximg.bytes_per_line) * imgH))
	if data == nil {
		return
	}
	ximg.data = (*C.char)(data)
	dst := unsafe.Slice((*byte)(data), int(ximg.bytes_per_line)*imgH)
	for y := 0; y < imgH; y++ {
		srcOff := y * imgW * 4
		dstOff := y * int(ximg.bytes_per_line)
		for x := 0; x < imgW && srcOff+x*4+3 < len(pixBuf); x++ {
			r := pixBuf[srcOff+x*4+0]
			g := pixBuf[srcOff+x*4+1]
			b := pixBuf[srcOff+x*4+2]
			dst[dstOff+x*4+0] = b
			dst[dstOff+x*4+1] = g
			dst[dstOff+x*4+2] = r
			dst[dstOff+x*4+3] = 0
		}
	}
	C.XPutImage(w.display, w.xwin, w.gc, ximg, 0, 0, 0, 0, C.uint(imgW), C.uint(imgH))
	C.XFlush(w.display)
}

// PollEvents processes X11 events and returns collected input events.
func (w *Window) PollEvents() []Event {
	if w.display == nil {
		return nil
	}
	for C.XPending(w.display) > 0 {
		var xev C.XEvent
		C.XNextEvent(w.display, &xev)
		switch typ := xev._type; typ {
		case C.Expose:
		case C.ConfigureNotify:
			cfg := (*C.XConfigureEvent)(unsafe.Pointer(&xev))
			newW, newH := int(cfg.width), int(cfg.height)
			if newW > 0 && newH > 0 && (newW != w.width || newH != w.height) {
				w.width, w.height = newW, newH
				w.fbWidth, w.fbHeight = newW, newH
				if w.canvas != nil {
					w.canvas.Release()
				}
				w.canvas = graphics.NewCanvas(newW, newH)
				w.eventsMu.Lock()
				w.events = append(w.events, Event{Type: EventResize, Width: newW, Height: newH})
				w.eventsMu.Unlock()
			}
		case C.ButtonPress:
			btn := (*C.XButtonEvent)(unsafe.Pointer(&xev))
			b := int(btn.button)
			if b == 4 {
				w.eventsMu.Lock()
				w.events = append(w.events, Event{Type: EventScroll, X: float64(btn.x), Y: float64(btn.y), ScrollY: 1.0})
				w.eventsMu.Unlock()
				continue
			} else if b == 5 {
				w.eventsMu.Lock()
				w.events = append(w.events, Event{Type: EventScroll, X: float64(btn.x), Y: float64(btn.y), ScrollY: -1.0})
				w.eventsMu.Unlock()
				continue
			}
			w.eventsMu.Lock()
			w.events = append(w.events, Event{Type: EventMouseButton, X: float64(btn.x), Y: float64(btn.y), Button: b, Action: 1})
			w.eventsMu.Unlock()
		case C.ButtonRelease:
			btn := (*C.XButtonEvent)(unsafe.Pointer(&xev))
			b := int(btn.button)
			if b == 4 || b == 5 {
				continue
			}
			w.eventsMu.Lock()
			w.events = append(w.events, Event{Type: EventMouseButton, X: float64(btn.x), Y: float64(btn.y), Button: b, Action: 0})
			w.eventsMu.Unlock()
		case C.MotionNotify:
			mot := (*C.XMotionEvent)(unsafe.Pointer(&xev))
			w.eventsMu.Lock()
			w.events = append(w.events, Event{Type: EventCursorMove, X: float64(mot.x), Y: float64(mot.y)})
			w.eventsMu.Unlock()
		case C.KeyPress:
			// Let the IME (XIM) filter the event first. If it consumes the
			// key (composition input), it is not delivered as a normal key.
			if w.ime != nil {
				if f, ok := w.ime.(interface{ FilterEvent(unsafe.Pointer) bool }); ok {
					if f.FilterEvent(unsafe.Pointer(&xev)) {
						continue
					}
				}
			}
			key := (*C.XKeyEvent)(unsafe.Pointer(&xev))
			ks := C.XLookupKeysym(key, 0)
			w.eventsMu.Lock()
			w.events = append(w.events, Event{Type: EventKey, Action: 1, Key: int(ks)})
			w.eventsMu.Unlock()
		case C.KeyRelease:
			key := (*C.XKeyEvent)(unsafe.Pointer(&xev))
			ks := C.XLookupKeysym(key, 0)
			w.eventsMu.Lock()
			w.events = append(w.events, Event{Type: EventKey, Action: 0, Key: int(ks)})
			w.eventsMu.Unlock()
		case C.ClientMessage:
			cm := (*C.XClientMessageEvent)(unsafe.Pointer(&xev))
			msgType := C.Atom(cm.data.l[0])
			if msgType == w.atomDeleteWindow {
				w.shouldClose = true
				if w.closeCallback != nil {
					w.closeCallback()
				}
			} else if msgType == w.atomXdndEnter {
				// XdndEnter: a drag entered the window
				// Extract version from the upper byte of data.l[1]
				w.dropVersion = int(cm.data.l[1] >> 24)
				_ = cm.data.l[2] // source window
				w.expectingDrop = true
			} else if msgType == w.atomXdndPosition {
				// XdndPosition: drag position update
				// Reply with XdndStatus to accept the drop
				srcWin := C.Window(cm.data.l[0])
				status := C.XInternAtom(w.display, C.CString("XdndStatus"), C.False)
				if status != 0 {
					var xevReply C.XEvent
					xevReply.xclient.type = C.ClientMessage
					xevReply.xclient.display = w.display
					xevReply.xclient.window = srcWin
					xevReply.xclient.message_type = status
					xevReply.xclient.format = 32
					xevReply.xclient.data.l[0] = 1 // accept the drop
					xevReply.xclient.data.l[1] = 0 // specify action rectangle
					xevReply.xclient.data.l[2] = 0
					xevReply.xclient.data.l[3] = 0
					xevReply.xclient.data.l[4] = int64(w.atomXdndActionCopy)
					C.XSendEvent(w.display, srcWin, 0, 0, &xevReply)
					C.XFlush(w.display)
				}
			} else if msgType == w.atomXdndDrop {
				// XdndDrop: content was dropped
				// Read the selection data to get the URI list
				C.XConvertSelection(w.display,
					w.atomXdndSelection,
					w.atomXdndURIList,
					w.atomXdndSelection,
					w.xwin, C.CurrentTime)
				C.XFlush(w.display)

				// After conversion, read the property
				// For simplicity, treat this as a drop notification
				// and attempt to read the XdndSelection property
				var actualType C.Atom
				var actualFormat C.int
				var nbytes C.ulong
				var bytesAfter C.ulong
				var propData *C.uchar
				C.XGetWindowProperty(w.display, w.xwin, w.atomXdndSelection,
					0, 0x7fffffff, 0, C.AnyPropertyType,
					&actualType, &actualFormat, &nbytes, &bytesAfter, &propData)
				if propData != nil {
					// The property contains a URI list (text/uri-list)
					uris := C.GoString((*C.char)(unsafe.Pointer(propData)))
					C.XFree(unsafe.Pointer(propData))
					w.eventsMu.Lock()
					if len(uris) > 0 {
						// Parse URIs, strip file:// prefix
						var files []string
						for _, uri := range parseURIList(uris) {
							files = append(files, uri)
						}
						w.events = append(w.events, Event{
							Type:      EventDrop,
							DropFiles: files,
						})
						// Notify the drop callback, mirroring glfw's behavior.
						if w.dropCallback != nil {
							cb := w.dropCallback
							cbFiles := files
							w.eventsMu.Unlock()
							cb(cbFiles)
							w.eventsMu.Lock()
						}
					}
					w.eventsMu.Unlock()
				}
				w.expectingDrop = false

				// Send XdndFinished to source
				srcWin := C.Window(cm.data.l[0])
				finished := C.XInternAtom(w.display, C.CString("XdndFinished"), C.False)
				if finished != 0 {
					var xevReply C.XEvent
					xevReply.xclient.type = C.ClientMessage
					xevReply.xclient.display = w.display
					xevReply.xclient.window = srcWin
					xevReply.xclient.message_type = finished
					xevReply.xclient.format = 32
					xevReply.xclient.data.l[0] = 1 // success
					C.XSendEvent(w.display, srcWin, 0, 0, &xevReply)
					C.XFlush(w.display)
				}
			} else if msgType == w.atomXdndLeave {
				w.expectingDrop = false
			}
		}
	}
	w.eventsMu.Lock()
	events := w.events
	w.events = nil
	w.eventsMu.Unlock()
	return events
}

func (w *Window) ShouldClose() bool                     { return w.shouldClose }
func (w *Window) Width() int                            { return w.width }
func (w *Window) Height() int                           { return w.height }
func (w *Window) FramebufferWidth() int                 { return w.fbWidth }
func (w *Window) FramebufferHeight() int                { return w.fbHeight }
func (w *Window) ContentScale() (float64, float64)      { return w.contentScaleX, w.contentScaleY }
func (w *Window) SetCloseCallback(fn func())            { w.closeCallback = fn }
func (w *Window) SetDropCallback(fn func([]string))     { w.dropCallback = fn }

// PostEvent appends an event to the internal queue, mirroring the GLFW
// backend's PostEvent. Used by tests and synthetic event injection.
func (w *Window) PostEvent(ev Event) {
	w.eventsMu.Lock()
	w.events = append(w.events, ev)
	w.eventsMu.Unlock()
}

// Focus raises and gives keyboard focus to the window.
func (w *Window) Focus() {
	if w.display == nil {
		return
	}
	C.XRaiseWindow(w.display, w.xwin)
	C.XSetInputFocus(w.display, w.xwin, C.RevertToParent, C.CurrentTime)
	C.XFlush(w.display)
}

func (w *Window) IME() ime.Handler                      { return w.ime }
func (w *Window) PollIMEEvents() []ime.Event {
	if w.ime == nil {
		return nil
	}
	return w.ime.PopEvents()
}
func (w *Window) SetIMECompositionPos(cssX, cssY float64) {
	if w.ime == nil {
		return
	}
	scaleX, scaleY := 1.0, 1.0
	if w.width > 0 {
		scaleX = float64(w.fbWidth) / float64(w.width)
	}
	if w.height > 0 {
		scaleY = float64(w.fbHeight) / float64(w.height)
	}
	w.ime.SetCompositionPos(int32(cssX*scaleX), int32(cssY*scaleY))
}
func (w *Window) SetIMEEnabled(enabled bool) {
	if w.ime == nil {
		return
	}
	w.ime.SetEnabled(enabled)
}
func (w *Window) SetClipboardString(s string)           {}
func (w *Window) GetClipboardString() string            { return "" }

// Close destroys the window and releases resources.
func (w *Window) Close() {
	if w.canvas != nil {
		w.canvas.Release()
		w.canvas = nil
	}
	if w.gc != nil {
		C.XFreeGC(w.display, w.gc)
		w.gc = nil
	}
	if w.xwin != nil {
		C.XDestroyWindow(w.display, w.xwin)
		w.xwin = nil
	}
	if w.display != nil {
		C.XCloseDisplay(w.display)
		w.display = nil
	}
}

// parseURIList parses a text/uri-list (RFC 2483) into file paths.
// Each line is a URI; file:// URIs are decoded to local paths.
func parseURIList(data string) []string {
	var files []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Decode file:// URI to path
		if strings.HasPrefix(line, "file://") {
			path := line[7:]
			// URL-decode percent-encoded characters
			path = strings.ReplaceAll(path, "%20", " ")
			path = strings.ReplaceAll(path, "%23", "#")
			path = strings.ReplaceAll(path, "%25", "%")
			// Handle Windows paths: file:///C:/... → C:/...
			if len(path) > 2 && path[0] == '/' && path[2] == ':' {
				path = path[1:]
			}
			files = append(files, path)
		} else {
			files = append(files, line)
		}
	}
	return files
}
