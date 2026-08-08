//go:build !linux && !darwin

// Package window provides a GLFW-based window with a Skia GPU backend surface.
// It is the Go translation of the WebKit chrome/client layer that hosts a
// GraphicsContext on a platform window.
//
// The window creates an OpenGL context via GLFW, assembles a Skia GLInterface +
// DirectContext, and wraps the window's default framebuffer (FBO 0) as a Skia
// Surface. Each frame, the caller blits a CPU-rasterized Canvas (from wb-ui's
// rendering pipeline) to the GPU surface and swaps buffers.

package window

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
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
	Button  int // glfw.MouseButton*
	Action  int // glfw.Press / glfw.Release
	Char    rune   // Unicode codepoint (for EventChar)
	Key     int    // glfw.Key*
	Mods    int    // glfw.ModifierKey (for key / mouse events)
	Width   int // for EventResize
	Height  int
	ScrollY float64  // for EventScroll (vertical wheel offset)
	ScrollX float64  // for EventScroll (horizontal wheel offset, e.g. shift+scroll or trackpad)
	DropFiles []string // for EventDrop (files dropped on window)
	// For EventTouch
	TouchX, TouchY float64
	TouchID        int
	TouchAction    int // 0=end, 1=begin, 2=move
}

// Window is a GLFW window with a Skia GPU backend surface.
type Window struct {
	win       *glfw.Window
	glIface   *skia.GLInterface
	gpuCtx    *skia.DirectContext
	gpuSurface *skia.Surface

	width, height   int
	fbWidth, fbHeight int
	contentScaleX, contentScaleY float64

	events   []Event
	eventsMu sync.Mutex

	// cursors caches per-shape GLFW cursor objects (lazily created on first
	// use; see cursor.go). cursorMu guards concurrent access.
	cursors   map[CursorShape]*glfw.Cursor
	cursorMu  sync.Mutex

	closeCallback func()
	dropCallback  func([]string) // files dropped on window

	// ime is the platform IME handler, used for input method
	// (composition) support. On Windows it subclasses the HWND to
	// intercept WM_IME_* messages.
	ime ime.Handler
}

// NewWindow creates a window with the given dimensions and title.
// It initializes GLFW, creates an OpenGL context, and sets up the Skia GPU
// backend. Must be called on the main thread (call runtime.LockOSThread first).
func NewWindow(width, height int, title string) (*Window, error) {
	// Enable per-monitor DPI awareness before GLFW init so that
	// GetFramebufferSize() returns physical pixels (not logical) on HiDPI.
	enableDPIAwareness()
	if err := glfw.Init(); err != nil {
		return nil, fmt.Errorf("window: glfw init: %w", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 2)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCompatProfile)
	glfw.WindowHint(glfw.Visible, 1)

	// Because the process is per-monitor DPI-aware, glfw.CreateWindow treats
	// width/height as screen (physical) pixels. To obtain a window that is
	// width×height CSS pixels on screen, we must scale up by the monitor's
	// content scale before creating the window. Otherwise the window stays
	// at the small physical size while the rendered content is scaled by
	// ContentScale — producing a window that is too small with clipped content.
	csX, csY := float32(1), float32(1)
	if monitor := glfw.GetPrimaryMonitor(); monitor != nil {
		csX, csY = monitor.GetContentScale()
	}
	if csX <= 0 {
		csX = 1
	}
	if csY <= 0 {
		csY = 1
	}
	physW := int(float64(width) * float64(csX))
	physH := int(float64(height) * float64(csY))

	win, err := glfw.CreateWindow(physW, physH, title, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("window: create: %w", err)
	}
	win.MakeContextCurrent()
	glfw.SwapInterval(1) // vsync

	// width/height are stored as CSS (logical) pixels; fbWidth/fbHeight are
	// the physical framebuffer pixels. contentScale is the ratio between them.
	w := &Window{
		win:     win,
		width:   width,  // CSS pixels
		height:  height, // CSS pixels
		cursors: make(map[CursorShape]*glfw.Cursor),
	}

	// Get framebuffer size (physical pixels, may differ from CSS size on HiDPI).
	w.fbWidth, w.fbHeight = win.GetFramebufferSize()
	w.contentScaleX = float64(csX)
	w.contentScaleY = float64(csY)
	// ★ DPI 诊断：glfw 创建的窗口物理尺寸 vs framebuffer vs CSS。
	//   若窗口物理 ≈ CSS（1294x838）而 framebuffer=1600x1000，说明
	//   CreateWindow 的 DPI 处理异常 → canvas 内容画到过大的 framebuffer
	//   → 窗口只显示左上 80% → "内容绘制区域变小 + 右下空白"。
	if winW, winH := win.GetSize(); winW > 0 && winH > 0 {
		fmt.Printf("[window-dpi] css=%dx%d glfwSize=%dx%d fb=%dx%d scale=(%.2f,%.2f)\n",
			width, height, winW, winH, w.fbWidth, w.fbHeight, w.contentScaleX, w.contentScaleY)
	}

	// Assemble Skia GL interface from the current GL context.
	glIface, err := skia.NewGLInterface(func(name string) unsafe.Pointer {
		return unsafe.Pointer(glfw.GetProcAddress(name))
	})
	if err != nil {
		return nil, fmt.Errorf("window: skia GL interface: %w", err)
	}
	w.glIface = glIface

	gpuCtx, err := skia.NewGLContext(glIface)
	if err != nil {
		return nil, fmt.Errorf("window: skia GL context: %w", err)
	}
	w.gpuCtx = gpuCtx

	if err := w.recreateSurface(); err != nil {
		return nil, err
	}

	w.setupCallbacks()

	// Initialize the IME handler with the platform window handle.
	// On Windows this subclasses the HWND to intercept WM_IME_* messages.
	w.ime = ime.NewHandler()
	w.ime.Init(platformHWND(win))

	return w, nil
}

// recreateSurface wraps the window's default framebuffer as a Skia surface.
// Called on initial creation and on resize.
func (w *Window) recreateSurface() error {
	if w.gpuSurface != nil {
		w.gpuSurface.Release()
		w.gpuSurface = nil
	}
	surf, err := skia.NewGPUSurfaceFromFBO(
		w.gpuCtx,
		0, // default framebuffer
		w.fbWidth, w.fbHeight,
		0, // samples
		0, // stencil bits
		skia.GLRGBA8,
		skia.ColorTypeRGBA8888,
		skia.SurfaceOriginBottomLeft, // OpenGL FBO origin is bottom-left
	)
	if err != nil {
		return fmt.Errorf("window: GPU surface: %w", err)
	}
	w.gpuSurface = surf
	return nil
}

// setupCallbacks installs GLFW callbacks for input events.
func (w *Window) setupCallbacks() {
	w.win.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		x, y := w.win.GetCursorPos()
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type:   EventMouseButton,
			X:      x,
			Y:      y,
			Button: int(button),
			Action: int(action),
			Mods:   int(mods),
		})
		w.eventsMu.Unlock()
	})
	w.win.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type: EventCursorMove,
			X:    x,
			Y:    y,
		})
		w.eventsMu.Unlock()
	})
	// ★ 鼠标移出窗口：发 EventCursorLeave，Host 清除 hover 状态与光标
	// 残留——否则 SetHovered(true) 留在 DOM 元素上，agent 输出触发渲染
	// 树重建时 resolver 读 IsHovered 把 :hover 样式错误应用（"没有操作
	// 和悬停时 agent 输出也影响渲染"）。进入窗口不发事件（下次
	// CursorMove 自然重建 hover）。
	w.win.SetCursorEnterCallback(func(_ *glfw.Window, entered bool) {
		if entered {
			return
		}
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type: EventCursorLeave,
			X:    -1e9,
			Y:    -1e9,
		})
		w.eventsMu.Unlock()
	})
	w.win.SetCharCallback(func(_ *glfw.Window, char rune) {
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type: EventChar,
			Char: char,
		})
		w.eventsMu.Unlock()
	})
	w.win.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type:   EventKey,
			Action: int(action),
			Key:    int(key),
			Mods:   int(mods),
		})
		w.eventsMu.Unlock()
	})
	w.win.SetFramebufferSizeCallback(func(_ *glfw.Window, fbW, fbH int) {
		w.eventsMu.Lock()
		w.fbWidth = fbW
		w.fbHeight = fbH
		// Convert physical framebuffer size to CSS (logical) pixels using the
		// content scale so the WebView viewport stays in CSS-pixel space.
		if w.contentScaleX > 0 {
			w.width = int(float64(fbW) / w.contentScaleX)
		}
		if w.contentScaleY > 0 {
			w.height = int(float64(fbH) / w.contentScaleY)
		}
		w.events = append(w.events, Event{
			Type:   EventResize,
			Width:  fbW,
			Height: fbH,
		})
		w.eventsMu.Unlock()
		w.recreateSurface()
	})
	w.win.SetScrollCallback(func(win *glfw.Window, xoff, yoff float64) {
		// Shift+scroll wheel → horizontal scroll (Windows standard behavior).
		if xoff == 0 && yoff != 0 {
			shift := win.GetKey(glfw.KeyLeftShift) == glfw.Press ||
				win.GetKey(glfw.KeyRightShift) == glfw.Press
			if shift {
				xoff = -yoff // negate: scroll up (yoff>0) → scroll left (xoff<0)
				yoff = 0
			}
		}
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type:    EventScroll,
			ScrollY: yoff,
			ScrollX: xoff,
		})
		w.eventsMu.Unlock()
	})
	w.win.SetCloseCallback(func(_ *glfw.Window) {
		if w.closeCallback != nil {
			w.closeCallback()
		}
	})
	// SetDropCallback receives files dragged and dropped onto the window.
	w.win.SetDropCallback(func(_ *glfw.Window, names []string) {
		w.eventsMu.Lock()
		w.events = append(w.events, Event{
			Type:      EventDrop,
			DropFiles: names,
		})
		w.eventsMu.Unlock()
		if w.dropCallback != nil {
			w.dropCallback(names)
		}
	})
}

// GPUSurface returns the Skia GPU surface backing this window. Callers can
// wrap it with graphics.NewCanvasFromSurface to paint directly onto the
// GPU surface (avoiding the CPU raster + blit path). The caller must not
// Release the returned surface; it is owned by the Window.
func (w *Window) GPUSurface() *skia.Surface {
	return w.gpuSurface
}

// Canvas returns the CPU raster canvas for software-rendered backends
// (X11/Cocoa). The GLFW GPU backend returns nil — callers should use
// GPUSurface instead.
func (w *Window) Canvas() *graphics.Canvas {
	return nil
}

// GPUContext returns the Skia DirectContext for the GPU surface. Callers
// need it to call FlushAndSubmit after painting directly on the GPU surface.
func (w *Window) GPUContext() *skia.DirectContext {
	return w.gpuCtx
}

// Present flushes pending GPU commands and swaps the window's back buffer.
// Call this after painting directly on the GPU surface obtained from
// GPUSurface(). Mirrors the flush + SwapBuffers sequence that Display
// used to perform internally.
//
// ★ CRITICAL: FlushAndSubmit must be SYNCHRONOUS (sync=true). With async
// (false), SwapBuffers swaps immediately while the tail of the draw queue
// (status bar, bottom of sidebar/main) is still in flight — the front
// buffer gets a partially-completed back buffer and the queued tail draws
// (status bar background/text) never appear. Symptom: "状态栏没有内容
// 显示" + sidebar/main bottom content missing while everything above
// renders fine (queue head completed before the swap).
func (w *Window) Present() {
	if w.gpuCtx != nil {
		w.gpuCtx.FlushAndSubmit(true)
	}
	w.win.SwapBuffers()
}

// Display blits the given source image (CPU-rasterized) to the GPU surface
// and swaps buffers. This is the legacy CPU→GPU blit path; prefer painting
// directly on the GPU surface via GPUSurface() + graphics.NewCanvasFromSurface.
func (w *Window) Display(srcImg *skia.Image) {
	if w.gpuSurface == nil || srcImg == nil {
		return
	}
	canvas := w.gpuSurface.Canvas()
	canvas.Clear(skia.ColorTransparent)
	srcW := srcImg.Width()
	srcH := srcImg.Height()
	srcRect := skia.RectXYWH(0, 0, float32(srcW), float32(srcH))
	dstRect := skia.RectXYWH(0, 0, float32(w.fbWidth), float32(w.fbHeight))
	canvas.DrawImageRect(srcImg, srcRect, dstRect, skia.SamplingLinear, nil)
	w.gpuCtx.FlushAndSubmit(false)
	w.win.SwapBuffers()
}

// PollEvents processes GLFW events and returns collected input events.
// The returned slice is consumed; the internal queue is cleared.
func (w *Window) PollEvents() []Event {
	glfw.PollEvents()
	w.eventsMu.Lock()
	events := w.events
	w.events = nil
	w.eventsMu.Unlock()
	return events
}

// PostEvent appends a synthetic event to the window's event queue so it will
// be picked up by the next PollEvents call. Useful for automated testing.
func (w *Window) PostEvent(ev Event) {
	w.eventsMu.Lock()
	w.events = append(w.events, ev)
	w.eventsMu.Unlock()
}

// ShouldClose reports whether the window has been asked to close.
func (w *Window) ShouldClose() bool { return w.win.ShouldClose() }

// Focus brings the window to the foreground and gives it input focus.
// Call this after creating the window to ensure it appears on top.
func (w *Window) Focus() { w.win.Focus(); w.win.Show() }

// Width / Height return the window size in logical (CSS) pixels.
func (w *Window) Width() int  { return w.width }
func (w *Window) Height() int { return w.height }

// FramebufferWidth / FramebufferHeight return the physical pixel dimensions.
func (w *Window) FramebufferWidth() int  { return w.fbWidth }
func (w *Window) FramebufferHeight() int { return w.fbHeight }

// ContentScale returns the monitor content scale (DPI ratio). On a 1.25x DPI
// display this returns (1.25, 1.25). Callers use this to scale the canvas so
// CSS pixels map to the correct number of physical pixels.
func (w *Window) ContentScale() (float64, float64) {
	return w.contentScaleX, w.contentScaleY
}

// SetCloseCallback sets a callback invoked when the window is asked to close.
func (w *Window) SetCloseCallback(fn func()) {
	w.closeCallback = fn
}

// SetDropCallback sets a callback invoked when files are dropped on the window.
// The callback receives the list of dropped file paths.
func (w *Window) SetDropCallback(fn func([]string)) {
	w.dropCallback = fn
}

// Close destroys the window and releases resources.
func (w *Window) Close() {
	if w.gpuSurface != nil {
		w.gpuSurface.Release()
		w.gpuSurface = nil
	}
	if w.gpuCtx != nil {
		w.gpuCtx.Release()
		w.gpuCtx = nil
	}
	if w.glIface != nil {
		w.glIface.Release()
		w.glIface = nil
	}
	if w.win != nil {
		w.win.Destroy()
		w.win = nil
	}
}

// init locks the OS thread for the main goroutine (GLFW requirement).
func init() {
	runtime.LockOSThread()
}

// IME returns the platform IME handler, or nil if not initialized.
// Callers use it to poll IME events, set composition position, and
// enable/disable IME for the focused element.
func (w *Window) IME() ime.Handler { return w.ime }

// PollIMEEvents returns and clears buffered IME events (composition
// updates, character inputs, composition end). The main loop should
// call this each frame after PollEvents to process IME input.
func (w *Window) PollIMEEvents() []ime.Event {
	if w.ime == nil {
		return nil
	}
	return w.ime.PopEvents()
}

// SetIMECompositionPos updates the IME composition/candidate window
// position to the given logical (CSS) pixel coordinates. The position
// is converted to physical pixels using the window's content scale.
// This should be called whenever the text caret moves.
func (w *Window) SetIMECompositionPos(cssX, cssY float64) {
	if w.ime == nil {
		return
	}
	// Convert CSS pixels to physical pixels via the framebuffer/size ratio.
	scaleX := 1.0
	scaleY := 1.0
	if w.width > 0 {
		scaleX = float64(w.fbWidth) / float64(w.width)
	}
	if w.height > 0 {
		scaleY = float64(w.fbHeight) / float64(w.height)
	}
	w.ime.SetCompositionPos(int32(cssX*scaleX), int32(cssY*scaleY))
}

// SetIMEEnabled enables or disables IME for the focused element.
// Should be called when focus changes to/from an editable element.
func (w *Window) SetIMEEnabled(enabled bool) {
	if w.ime == nil {
		return
	}
	w.ime.SetEnabled(enabled)
}

// SetClipboardString sets the system clipboard text, mirroring
// GLFWSetClipboardString / WebKit's clipboard write.
func (w *Window) SetClipboardString(s string) {
	w.win.SetClipboardString(s)
}

// GetClipboardString returns the system clipboard text, mirroring
// GLFWGetClipboardString.
func (w *Window) GetClipboardString() string {
	return w.win.GetClipboardString()
}
