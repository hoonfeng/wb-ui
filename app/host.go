// Package app provides a high-level application host that ties together a
// platform window (GLFW + Skia GPU surface) and a webkit.WebView, running the
// render + event loop so embedders can focus on page logic instead of GL
// plumbing.
//
// This mirrors the embedding layer that real WebKit splits between
// WebView (page logic) and the platform Window/HostWindow (GL + event pump).
// Keeping the GPU/render/event loop inside the library means example
// programs and downstream embedders no longer have to recreate the
// Surface→Paint→Present→HitTest pipeline by hand.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/css"
	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/page"
	"wb-ui/platform/graphics"
	"wb-ui/platform/ime"
	"wb-ui/platform/window"
	"wb-ui/rendering"
	"wb-ui/style"
	"wb-ui/webkit"
)

// devtoolsScript 注入类似浏览器 DevTools 的调试 API（window.__devtools）：
//   - __devtools.tree(depth)  元素树（tag/class/几何/关键样式/文本），便于
//     像浏览器 Elements 面板一样审查布局（比渲染树 dump 更直观）
//   - __devtools.pick(x, y)   指定坐标处的元素链（从上到下所有命中元素）
//   - __devtools.sel(selector) 按 CSS 选择器查元素几何 + 父级链
//   - __devtools.page()       视口/滚动/文档尺寸
// 几何取自引擎真实布局（getBoundingClientRect → 渲染树几何），诊断
// 「渲染树与 DOM 几何不一致」「元素被挤压/溢出」类问题免去翻 dump 文件。
const devtoolsScript = `
window.__devtools = (function(){
  function rect(el){ try { var r = el.getBoundingClientRect(); return {x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height)}; } catch(e){ return {x:-1,y:-1,w:-1,h:-1}; } }
  function cls(el){ try { var c = el.className; return (c && c.toString) ? c.toString() : (c || ''); } catch(e){ return ''; } }
  function cs(el){ try { var s = getComputedStyle(el); return {disp:s.display, pos:s.position, bg:s.backgroundColor, vis:s.visibility, op:s.opacity, overflow:s.overflow, w:s.width, h:s.height, flex:s.flex || s.getPropertyValue('flex'), borderT:s.borderTopWidth, left:s.left, right:s.right, top:s.top, bottom:s.bottom}; } catch(e){ return {}; } }
  function txt(el){ if (el.childNodes && el.childNodes.length === 1 && el.childNodes[0].nodeType === 3) return el.childNodes[0].nodeValue.replace(/\s+/g,' ').slice(0,30); if (el.childNodes && el.childNodes.length === 0) return ''; return ''; }
  function node(el, depth){
    var n = {tag: (el.tagName||'').toLowerCase(), id: el.id||'', cls: cls(el).slice(0,60)};
    var r = rect(el), s = cs(el);
    n.x=r.x; n.y=r.y; n.w=r.w; n.h=r.h; n.disp=s.disp; n.pos=s.pos; n.bg=s.bg; n.vis=s.vis; n.op=s.op; n.overflow=s.overflow;
    var t = txt(el); if (t) n.txt = t;
    if (depth > 0 && el.children && el.children.length) {
      n.ch = [];
      for (var i=0;i<el.children.length;i++){ var c = el.children[i]; if (c && c.tagName) n.ch.push(node(c, depth-1)); }
    }
    return n;
  }
  return {
    tree: function(maxDepth){ return JSON.stringify(node(document.documentElement, (maxDepth==null)?3:maxDepth)); },
    pick: function(x, y){
      var all = [];
      function walk(el){
        if (!el || !el.tagName) return;
        var r = rect(el);
        if (x >= r.x && x <= r.x+r.w && y >= r.y && y <= r.y+r.h) {
          var s = cs(el);
          var it = {tag:(el.tagName||'').toLowerCase(), cls:cls(el).slice(0,40), x:r.x, y:r.y, w:r.w, h:r.h, disp:s.disp, pos:s.pos, bg:s.bg, vis:s.vis, overflow:s.overflow};
          var t = txt(el); if (t) it.txt = t;
          all.push(it);
        }
        if (el.children) for (var i=0;i<el.children.length;i++) walk(el.children[i]);
      }
      walk(document.documentElement);
      return JSON.stringify(all);
    },
    sel: function(sel){
      var el = document.querySelector(sel);
      if (!el) return JSON.stringify({err: 'not found: ' + sel});
      var r = rect(el), s = cs(el);
      var out = {sel: sel, tag:(el.tagName||'').toLowerCase(), cls:cls(el).slice(0,60), id:el.id||'', x:r.x, y:r.y, w:r.w, h:r.h, disp:s.disp, pos:s.pos, bg:s.bg, vis:s.vis, op:s.op, overflow:s.overflow, cw:s.w, ch:s.h, borderT:s.borderT};
      var t = txt(el); if (t) out.txt = t;
      out.parents = [];
      var p = el.parentElement;
      for (var i=0;i<6 && p;i++){
        var pr = rect(p), ps = cs(p);
        out.parents.push({tag:(p.tagName||'').toLowerCase(), cls:cls(p).slice(0,40), x:pr.x, y:pr.y, w:pr.w, h:pr.h, disp:ps.disp, pos:ps.pos, bg:ps.bg, overflow:ps.overflow, left:ps.left, right:ps.right, top:ps.top, bottom:ps.bottom});
        p = p.parentElement;
      }
      return JSON.stringify(out);
    },
    page: function(){
      return JSON.stringify({
        vw: window.innerWidth||0, vh: window.innerHeight||0,
        dW: document.documentElement ? document.documentElement.scrollWidth : 0,
        dH: document.documentElement ? document.documentElement.scrollHeight : 0,
        bodyH: document.body ? document.body.scrollHeight : 0,
        scrollY: window.scrollY || document.documentElement.scrollTop || 0
      });
    }
  };
})();
`

// encodePNG converts RGBA pixels to PNG bytes (for WB_PAINT_DUMP diagnostics).
func encodePNG(w, h int, rgba []byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, rgba)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

var DumpRTCallback func(rv *rendering.RenderView)

// debugPaintLog enables verbose paint and event diagnostics printed to stderr.
// Set to true to trace hover, click, and paint operations.
var debugPaintLog = os.Getenv("WB_HOVER_DEBUG") != ""

// ClickHandler is invoked when the user clicks an element whose onclick
// attribute does not use the "js:" prefix. el is the deepest hit-tested
// element with an onclick attribute (may be nil if nothing was hit), and
// onclick is the raw value of the element's onclick attribute (empty when
// absent). clickX / clickY are the hit point in render-tree CSS pixels
// (already scroll-adjusted), useful for positioning IME composition windows.
// Embedders typically dispatch to registered Go handlers based on onclick
// and manage IME focus from here.
type ClickHandler func(el *dom.Element, onclick string, clickX, clickY float64)

// IMEHandler is invoked each frame with any IME events that arrived for the
// currently focused editable element. The handler is only called while an
// element is focused via FocusElement.
type IMEHandler func(events []ime.Event)

// Host is the top-level application host: it owns a platform.Window (GLFW +
// Skia GPU surface) and drives a webkit.WebView render/event loop. Embedders
// create a WebView, load HTML, register Go functions, then hand everything to
// NewHost and call Run.
type Host struct {
	win *window.Window
	wv  *webkit.WebView

	// clickHandler dispatches non-js: onclick values to embedder code.
	clickHandler ClickHandler
	// imeHandler receives IME events for the focused element.
	imeHandler IMEHandler

	// IME focus state. When imeFocusedEl is non-nil, incoming IME events are
	// applied to it: composition updates append a preview, char input appends
	// confirmed text, and composition end finalizes. The embedder can read
	// FocusedElement to know which element is receiving input.
	imeFocusedEl   *dom.Element
	imeInputText   string
	imeComposing   bool
	imeComposeText string
	// downJSFocused records whether the JS mousedown handler (dispatched on
	// press) called el.focus() during its execution. Browser standard: clicking
	// a non-focusable element moves focus to <body> (blurring the focused
	// control), but if the mousedown handler re-focuses a control (xterm's
	// helper textarea on clicking the terminal), focus is preserved. Without
	// this flag, every click on the terminal (a div) triggered Unfocus →
	// cursor disappeared + input stopped ("点击终端后光标消失不可输入").
	downJSFocused bool
	// Composition insertion state: base is the element text WITHOUT the
	// in-progress composition, start is the caret offset where the composition
	// (and subsequent char input) is inserted. Without this, IME text was
	// always appended to the END of the value, ignoring the caret position —
	// clicking mid-text then typing put the new text at the tail.
	imeComposeBase  string
	imeComposeStart int

	// animStart is the wall-clock time when Run() started, used to compute
	// the animation clock (AnimationTime) each frame.
	animStart time.Time

	// firstFrame forces a render on the first loop iteration so the window
	// never stays black before any dirty rect is established.
	firstFrame bool

	// dumpPNGDone marks the single-shot WB_DUMP_PNG canvas dump (first paint
	// only). Per-frame dumps ReadPixels from the GPU surface synchronously
	// and stall the main loop (window appears frozen).
	dumpPNGDone bool

	// dbgFrame/lastPaintDbg: WB_PAINT_DEBUG 诊断帧计数与状态（needPaint 变化
	// 或每 60 帧打印一次，确认 Paint 是否执行、gpuCanvas 是否可用）。
	dbgFrame       int
	lastPaintDbg   bool

	// treeHookDoc: DOM 结构变更回调已注册的 Document（LoadHTML 重建时
	// 指针变化，重新注册）。见 ensureTreeChangeHook。
	treeHookDoc *dom.Document

	// needsResizeDump is set true on EventResize, cleared after DumpRTCallback fires
	// once on the re-laid-out tree. Prevents dumping every frame.
	needsResizeDump bool
	// lastDbgCW/CH track last logged paint canvas size (WB_RESIZE_DEBUG)
	lastDbgCW, lastDbgCH int
	// paintDumped marks the one-shot WB_PAINT_DUMP canvas pixel dump
	paintDumped bool
	// snapEnabled/snapLast drive the periodic WB_SNAP layout snapshot
	snapEnabled bool
	snapLast    time.Time
	// snapFile/snapMu guard the _layout_snap.log writer (WB_SNAP)
	snapFile *os.File
	snapMu   sync.Mutex
	// paintLogFile is the WB_PAINT_LOG=1 paint trace writer (_paint_trace.log).
	// Records dirty rect / view dirty state / canvas pixel coverage per frame
	// so "打开文件前 vs 打开文件后" paint behavior can be compared exactly.
	paintLogFile *os.File
	// lastLoggedScrollY dedupes per-frame [scroll] logs (only logs on change).
	lastLoggedScrollY int
	// lastIMEX/lastIMEY dedupe IME composition-position updates.
	lastIMEX, lastIMEY int32

	// Selection state. Text selection is tracked as CSS-pixel coordinates
	// (not RenderText pointers) so it survives render tree rebuilds. Each
	// frame, updateSelection converts these coordinates into a
	// rendering.Selection against the current render tree.
	clickCount     int                       // consecutive clicks (1-4)
	selGranularity rendering.TextGranularity // current selection granularity
	selAnchorX     float64                   // selection anchor (fixed start point)
	selAnchorY     float64
	selStartX      float64 // start position (= anchor unless shift+click)
	selStartY      float64
	selEndX        float64 // end position (drag/shift+click target)
	selEndY        float64
	selecting      bool    // mouse button held during drag
	shiftSelecting bool    // shift+click extending selection
	mouseDownX     float64 // press position for hysteresis
	mouseDownY     float64
	hysteresisMet  bool // drag threshold (3px) exceeded
	lastClickTime  time.Time
	lastClickX     float64
	lastClickY     float64

	// cursorX, cursorY track the last known cursor position (from mouse
	// move events), used for hit-testing on scroll events.
	cursorX, cursorY float64

	// hoveredEl tracks the element currently under the mouse cursor.
	// On each mouse-move, HitTest locates the deepest element and updates
	// its IsHovered state accordingly. This enables :hover pseudo-class
	// matching in the style resolver.
	hoveredEl *dom.Element
	// activeEl tracks the element being pressed (mousedown → :active).
	// Cleared on mouseup. Enables :active pseudo-class matching.
	activeEl *dom.Element

	// selectPopup is the open <select> dropdown overlay (nil when closed).
	// Clicking a <select> creates a fixed-position list layer containing its
	// <option> elements; clicking an option sets the select's value and
	// dispatches a change event (Vue v-model update), clicking anywhere else
	// closes the popup. Mirrors RenderMenuList's popup in WebKit.
	selectPopup       *dom.Element // the overlay container (div.select-popup)
	selectPopupSelect *dom.Element // the <select> this popup belongs to

	// rangeDragEl tracks an active <input type="range"> thumb drag. Pressing
	// a range snaps the value to the click position immediately (browser
	// behavior) and starts a drag; subsequent mouse moves update the value
	// along the track (input events), mouseup dispatches change and ends it.
	rangeDragEl *dom.Element
	// rangeDragRV is the RenderView owning the dragged range (iframe-aware:
	// the range may live in a child frame's render tree).
	rangeDragRV *rendering.RenderView

	// resizeDragEl tracks an active CSS resize drag on a textarea
	// (resize:vertical/both — the bottom-right corner handle). Pressing the
	// handle starts a drag; mouse moves update the element's height (written
	// back to style="height:Npx" so relayout keeps it); mouseup ends it.
	// Mirrors browser behavior: the drag is constrained by min/max-height.
	resizeDragEl      *dom.Element
	resizeDragRV      *rendering.RenderView
	resizeDragStartY  float64 // cursor Y at drag start (CSS px)
	resizeDragStartH  float64 // box height at drag start (CSS px)
	resizeDragCursor  window.CursorShape // 拖动中保持的 resize 光标（按下时按模式记录）
	// lastCursor tracks the last window cursor shape set (dedupe: only call
	// SetCursorShape when the shape actually changes).
	lastCursor window.CursorShape

	// WB_FRAMETIME=1 / WB_AUTODRAG=1：拖拽帧耗时统计与自动化拖拽（性能
	// 验证「textarea resize 高度跟手」用——headless probe 测不到 GPU
	// paint，必须在真实窗口 + vsync 下量化完整帧时间）。
	ftLog        *os.File // 帧耗时日志（_frametime.log）
	autodragOn   bool
	// WB_TERM_TEST=1：自动化验证终端（自动点「新建终端」→ 查 xterm DOM/PTY 输出）。
	termTestStep int
	termTestDone bool
	// WB_DT_DUMP=1：真实模式（无 WB_TERM_QUERY 诊断污染）下帧 300 输出
	// __devtools 布局诊断——验证真实 fit 后的行数/间隙/光标，不受
	// termTestTick 的 focus/resize 干扰。
	dtFrame   int
	dtDumped  bool
	// WB_DUMP_PNG_FRAME：Paint 帧计数（WB_DUMP_PNG 在指定帧 dump canvas
	// 真实绘制像素——终端 fit 完成后才能看到真实间隙/光标）。
	paintFrame int
	// OnFrame 每帧回调（主线程）：宿主注入，用于在主循环安全地消费
	// 外部 goroutine 投递的 JS 推送（如终端 PTY 输出）——goja 非线程
	// 安全，任何跨 goroutine 的 RunJS 都必须经此排队到主线程执行。
	OnFrame func()
	autodragEl   *dom.Element
	autodragX    float64 // 手柄视口坐标
	autodragY    float64
	autodragStep int
	autodragPanelOpen bool // 设置面板已打开（区分 -100/-200 等待期）
	autodragStart time.Time
	autodragSamples []float64

	// scrollbarDrag tracks an active scrollbar thumb drag.
	scrollbarDragging bool
	// scrollbarDragBox is the scroll container being dragged.
	scrollbarDragBox *rendering.RenderBox
	// scrollbarDragRV is the RenderView owning the dragged scroll container —
	// iframe 子文档的滚动条属于子 Frame 的 RenderView（偏移表存子 rv）。
	scrollbarDragRV *rendering.RenderView
	// scrollbarDragAxis: true = vertical, false = horizontal.
	scrollbarDragAxis bool
	// scrollbarDragStartY is the cursor Y at drag start (CSS pixels).
	scrollbarDragStart float64
	// scrollbarDragOffsetY is the scroll offset at drag start.
	scrollbarDragScroll float64

	// Smooth (wheel) scrolling state: wheel events set a TARGET offset and
	// the main loop interpolates toward it with an exponential approach,
	// mirroring browser wheel behavior. Scrollbar thumb drags stay 1:1 and
	// bypass this entirely (they write BoxScrollOffset directly).
	smoothBox    *rendering.RenderBox
	smoothRV     *rendering.RenderView // 拥有 smoothBox 偏移表的 RenderView（iframe 子文档时是子 Frame 视图）
	smoothCurX   float64
	smoothCurY   float64
	smoothTarX   float64
	smoothTarY   float64
	smoothActive bool
	smoothLast   time.Time

	// caretBlinkTime tracks the last caret visibility toggle for blinking.
	caretBlinkTime time.Time

	// frameView is the page's FrameView, cached for scroll operations in
	// processEvents (where Run's local variable is out of scope).
	frameView *page.FrameView
}

// NewHost creates a Host that drives the given WebView inside a new platform
// window of the given CSS-pixel dimensions. The window is created with DPI
// awareness so width/height are treated as logical (CSS) pixels and scaled to
// physical pixels internally. The caller should call Run to start the loop.
func NewHost(wv *webkit.WebView, width, height int, title string) (*Host, error) {
	if wv == nil {
		return nil, fmt.Errorf("app: WebView is nil")
	}
	// Initialize font manager if not already done. First try bundled resources,
	// then load from system fonts (C:\Windows\Fonts on Windows).
	if graphics.GetFontManager() == nil {
		fontDir := findFontDir()
		_ = graphics.InitFontManager(fontDir)
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	// Bridge Skia font metrics to the layout engine so inline text measurement
	// uses real glyph widths instead of fallback estimates.
	layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	wv.Resize(width, height)
	// ★ 注入 __devtools（类似浏览器 DevTools 的调试 API）：页面 JS 已执行，
	// 用真实渲染几何提供元素树/坐标拾取/选择器查询，诊断布局问题。
	if interp0 := wv.JSInterpreter(); interp0 != nil {
		if _, err := interp0.RunJS(devtoolsScript); err != nil {
			log.Printf("[devtools] inject __devtools failed: %v", err)
		}
	}
	win, err := window.NewWindow(width, height, title)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	h := &Host{win: win, wv: wv}
	// ★ JS el.focus()/el.blur() 桥接到引擎聚焦（xterm textarea.focus()、
	// Vue autofocus 等）：JS 聚焦必须设置引擎的 imeFocusedEl（否则
	// EventChar 分支 h.imeFocusedEl==nil 丢弃所有字符——「终端不可
	// 输入」根因）并派发 focus/blur DOM 事件（xterm 监听 textarea
	// focus/blur 激活输入+显示光标）。
	bindings.FocusBridge = func(el *dom.Element, focused bool) {
		if focused {
			// mousedown 派发期间 JS el.focus()（xterm 点击终端容器时
			// textarea.focus()）：标记 so Press 分支不会误 Unfocus。
			h.downJSFocused = true
			h.FocusElementByKeyboard(el, false)
		} else {
			if h.imeFocusedEl == el {
				h.Unfocus()
			} else {
				el.SetFocused(false)
			}
		}
	}
	// JS selectionStart/End/setSelectionRange ↔ 引擎光标（xterm 输入
	// 处理读 textarea selection 计算新增字符；setSelectionRange 定位
	// 点击光标）。
	bindings.GetElementComputedFont = func(el *dom.Element) (string, float64, int, string) {
		if el == nil {
			return "sans-serif", 14, 400, "normal"
		}
		fr := wv.MainFrame().Frame()
		if fr == nil || fr.Resolver() == nil {
			return "sans-serif", 14, 400, "normal"
		}
		cs := fr.Resolver().ResolveElement(el)
		if cs == nil {
			return "sans-serif", 14, 400, "normal"
		}
		size := cs.FontSize.Value
		if size <= 0 {
			size = 14
		}
		w := 400
		switch strings.ToLower(strings.TrimSpace(cs.FontWeight)) {
		case "bold", "bolder", "600", "700", "800", "900":
			w = 700
		}
		st := "normal"
		if strings.EqualFold(cs.FontStyle, "italic") || strings.EqualFold(cs.FontStyle, "oblique") {
			st = cs.FontStyle
		}
		fam := cs.FontFamily
		if fam == "" {
			fam = "sans-serif"
		}
		return fam, size, w, st
	}
	bindings.SelectionBridge = func(el *dom.Element) (int, int) {
		if el == nil || el != h.imeFocusedEl {
			return -1, -1
		}
		if sel := rendering.FocusedFormControlSel; sel != nil {
			return sel.Start, sel.End
		}
		return -1, -1
	}
	bindings.SetSelectionBridge = func(el *dom.Element, start, end int) {
		if el == nil {
			return
		}
		if el != h.imeFocusedEl {
			h.FocusElementByKeyboard(el, false)
		}
		if start < 0 {
			start = 0
		}
		if end < start {
			end = start
		}
		rendering.FocusedFormControlSel = &rendering.FormControlSelection{
			Start: start, End: end, Active: true,
		}
		h.ensureFocusedCaretVisible()
	}
	return h, nil
}

// findFontDir searches for the wb-ui bundled font resources directory.
// Tries several common locations relative to the executable and working dir.
func findFontDir() string {
	candidates := []string{
		"resources/fonts",
		filepath.Join("..", "resources", "fonts"),
		filepath.Join("F:\\syproject\\wb-ui", "resources", "fonts"),
	}
	// Try relative to the executable.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(exeDir, "resources", "fonts"),
			filepath.Join(exeDir, "..", "..", "resources", "fonts"),
			filepath.Join(exeDir, "..", "..", "..", "wb-ui", "resources", "fonts"),
		}, candidates...)
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return "resources/fonts"
}

// SetClickHandler installs the callback invoked for non-js: onclick hits.
// js:-prefixed onclick values are executed via WebView.EvalJS directly by
// the Host and are not forwarded to this handler.
func (h *Host) SetClickHandler(fn ClickHandler) { h.clickHandler = fn }

// SetIMEHandler installs the callback invoked with IME events while an
// element is focused via FocusElement.
func (h *Host) SetIMEHandler(fn IMEHandler) { h.imeHandler = fn }

// Window returns the underlying platform window. Exposed so embedders can
// query content scale, framebuffer size, or install custom GLFW callbacks
// if needed.
func (h *Host) Window() *window.Window { return h.win }

// WebView returns the WebView driven by this host.
func (h *Host) WebView() *webkit.WebView { return h.wv }

// FocusElement marks el as the current IME focus target and enables IME
// input on the platform window. Incoming IME events will be applied to el
// until Unfocus is called or another element is focused. Call this from a
// ClickHandler when the user clicks an editable element (e.g. <input>).
//
// For <input> and <textarea> elements, the text is read from / written to the
// "value" attribute (mirroring WebCore::HTMLTextFormControlElement::value());
// for other elements, text content is used (the legacy div-based editable
// element behavior).
func (h *Host) FocusElement(el *dom.Element) {
	h.FocusElementByKeyboard(el, false)
}

// scrollXFor returns the horizontal scroll offset of a scroll container.
// Form controls (input/textarea) scroll their text through the per-element
// FormControlTextScroll (set by the painter each frame), not
// BoxScrollOffset — mirror that so drag/arrow/track operations hit the
// right value, scoped to the control being dragged.
func scrollXFor(rv *rendering.RenderView, box *rendering.RenderBox) float64 {
	if el, ok := box.Node().(*dom.Element); ok {
		if el.LocalName() == "textarea" || el.LocalName() == "input" {
			return rendering.FormControlTextScroll(el)
		}
	}
	if rv == nil {
		return 0
	}
	sx, _ := rv.BoxScrollOffset(box)
	return sx
}

// setScrollXFor sets the horizontal scroll offset of a scroll container,
// routing form controls to their per-element FormControlTextScroll and
// others to BoxScrollOffset. Returns true when a change was applied.
func setScrollXFor(rv *rendering.RenderView, box *rendering.RenderBox, x float64) bool {
	if x < 0 {
		x = 0
	}
	if el, ok := box.Node().(*dom.Element); ok {
		if el.LocalName() == "textarea" || el.LocalName() == "input" {
			if rendering.FormControlTextScroll(el) == x {
				return false
			}
			rendering.SetFormControlTextScroll(el, x)
			return true
		}
	}
	if rv == nil {
		return false
	}
	sx, sy := rv.BoxScrollOffset(box)
	if sx == x {
		return false
	}
	rv.SetBoxScrollOffset(box, x, sy)
	// 水平滚动同样派发 scroll DOM 事件（前端 @scroll 懒加载依赖）。
	if el, ok := box.Node().(*dom.Element); ok {
		el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
	}
	return true
}

// markScrollDirty forces a repaint after a programmatic scroll change.
func (h *Host) markScrollDirty() {
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
}

// scrollableVertically reports whether box has a vertical scrollbar range
// (content taller than the viewport). Used by wheel chaining: a scroll
// container with no scroll range must bubble the wheel to its parent.
func scrollableVertically(rv *rendering.RenderView, box *rendering.RenderBox) bool {
	if rv == nil || box == nil {
		return false
	}
	vm := rendering.VerticalScrollbarMetrics(rv, box)
	return vm.OK && vm.MaxScroll > 0
}

// scrollableHorizontally reports whether box has a horizontal scrollbar range.
func scrollableHorizontally(rv *rendering.RenderView, box *rendering.RenderBox) bool {
	if rv == nil || box == nil {
		return false
	}
	hm := rendering.HorizontalScrollbarMetrics(rv, box)
	return hm.OK && hm.MaxScroll > 0
}

// chainScrollTargetUp walks up from box's DOM node and returns the nearest
// ancestor scroll container that actually has scroll range in some axis
// (browser wheel bubbling: a non-scrollable textarea inside a scrollable
// settings panel must let the wheel move the panel). Returns the original
// box when no scrollable ancestor exists.
func chainScrollTargetUp(rv *rendering.RenderView, box *rendering.RenderBox) *rendering.RenderBox {
	if rv == nil || box == nil {
		return box
	}
	for n := box.Node(); n != nil; n = n.ParentNode() {
		p := rv.FindScrollContainerForNode(n)
		if p == nil || p == box {
			continue
		}
		if scrollableVertically(rv, p) || scrollableHorizontally(rv, p) {
			return p
		}
	}
	return box
}

// dispatchScrollEvent 向滚动容器派发 scroll DOM 事件（不冒泡，浏览器语义），
// 让前端 @scroll 监听器（Vue onScroll → loadMoreMessages 向上翻页等）感知
// 滚动偏移变化。此前滚轮/滚动条交互只更新引擎内偏移从不派发事件——JS 的
// el.addEventListener('scroll') 永远收不到回调，历史对话打开后向上翻页永不
// 触发，只显示初始 limit=50 条原始行（tool 消息占配额，≈最后一个 run）。
func (h *Host) dispatchScrollEvent(box *rendering.RenderBox) {
	if box == nil {
		return
	}
	if n := box.Node(); n != nil {
		if el, ok := n.(*dom.Element); ok {
			el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
		}
	}
}

// FocusElementByKeyboard is FocusElement with a focus-source hint. byKeyboard
// should be true when focus moved via Tab (keyboard); false for mouse clicks.
// It drives the :focus-visible pseudo-class: the UA default outline only
// matches keyboard focus, so clicking a button/tab no longer draws the
// (browser-mismatched) focus ring.
func (h *Host) FocusElementByKeyboard(el *dom.Element, byKeyboard bool) {
	if h.imeFocusedEl != nil && h.imeFocusedEl != el {
		h.imeFocusedEl.SetFocused(false)
	}
	if el != nil {
		el.SetFocused(true)
		el.SetFocusByKeyboard(byKeyboard)
	}
	h.imeFocusedEl = el
	h.imeComposing = false
	h.imeComposeText = ""
	h.imeComposeBase = ""
	h.imeComposeStart = 0
	// Mark frame dirty so :focus style updates.
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
	}
	if el != nil {
		h.imeInputText = focusedElementValue(el)
	}
	h.win.SetIMEEnabled(true)
	// For text-type form controls (<input>/<textarea>), register the element
	// with the rendering package so paintTextInputValue draws a blinking caret.
	// These are replaced elements with no RenderText children, so the regular
	// CaretPos/PaintCaret path (which targets RenderText segments) cannot
	// locate them.
	if el != nil && isTextFormControl(el) {
		rendering.FocusedFormControl = el
	} else {
		rendering.FocusedFormControl = nil
	}
	// ★ 派发 focus DOM 事件（不冒泡，浏览器语义）：xterm.js 监听
	// textarea 的 focus 事件设置 isFocused=true（驱动光标渲染 +
	// 键盘输入激活）；CodeMirror 也用 focus/blur 事件刷新编辑器状态。
	// 此前引擎聚焦只改内部状态从不派发事件 → JS 库永远不知道焦点
	// 已切换（「点击终端无光标」根因之一）。
	if el != nil {
		el.DispatchEvent(dom.NewEvent("focus", false, false, false))
	}
}

// focusedElementValue returns the current text of a focused element. For
// <input> it reads the "value" attribute; for <textarea> it reads textContent;
// for other elements it reads textContent. Mirrors the value() accessor on
// HTMLTextFormControlElement.
func focusedElementValue(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if el.LocalName() == "textarea" {
		return el.TextContent()
	}
	if isTextFormControl(el) {
		return el.GetAttribute("value")
	}
	return el.TextContent()
}

// setFocusedElementValue writes the text back to a focused element. For
// <input> it sets the "value" attribute; for <textarea> it sets textContent;
// for other elements it sets textContent.
func setFocusedElementValue(el *dom.Element, text string) {
	if el == nil {
		return
	}
	// ★ contenteditable（CodeMirror 6 输入区）：绝不 SetTextContent 全文替换
	//   ——会抹掉 CM6 的结构化 DOM（.cm-line + 高亮 span），且 CM6 的 input
	//   处理发现文本未变不会重建结构，布局永久破坏。字符插入走
	//   bindings.InsertTextAtSelection（光标处插文本节点），由 CM6 的
	//   readDOMChange 同步 state 并重建正确 DOM。
	if strings.EqualFold(el.GetAttribute("contenteditable"), "true") {
		return
	}
	if el.LocalName() == "textarea" {
		el.SetTextContent(text)
		return
	}
	if isTextFormControl(el) {
		el.SetAttribute("value", text)
		return
	}
	el.SetTextContent(text)
}

// formControlEditLimits returns 0 when the focused control rejects edits
// (readonly/disabled attributes), otherwise its maxlength in runes, or -1
// when unlimited. Readonly/disabled controls still receive focus but must
// not mutate their value (browser semantics).
func formControlEditLimits(el *dom.Element) int {
	if el == nil {
		return -1
	}
	if el.GetAttribute("readonly") != "" || el.GetAttribute("disabled") != "" {
		return 0
	}
	if in, ok := html5.ToInputElement(el); ok {
		return in.MaxLength()
	}
	if ta, ok := html5.ToTextAreaElement(el); ok {
		return ta.MaxLength()
	}
	return -1
}

// truncateToMaxLen clips s to at most maxLen runes (maxLen < 0 = unlimited).
func truncateToMaxLen(s string, maxLen int) string {
	if maxLen < 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}

// isTextFormControl reports whether el is an <input> (non-checkbox/radio/
// hidden/range/color/file/submit/reset/button/image) or <textarea>, i.e. a
// form control whose text is carried by the value attribute and which
// accepts text entry via IME. Mirrors HTMLTextFormControlElement::childShouldCreateRenderer.
func isTextFormControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "textarea":
		return true
	case "input":
		t := el.GetAttribute("type")
		switch strings.ToLower(t) {
		case "checkbox", "radio", "range", "color", "file",
			"submit", "reset", "button", "image", "hidden":
			return false
		}
		return true
	default:
		return false
	}
}

// isFocusableElement 报告元素是否可聚焦（浏览器语义）。
// 点击可聚焦元素应触发 :focus 伪类（outline 指示器等），而不只是文本控件。
func isFocusableElement(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "input", "textarea", "select", "button", "summary", "label", "a", "area":
		return true
	}
	if strings.EqualFold(el.GetAttribute("contenteditable"), "true") {
		return true
	}
	if el.HasAttribute("tabindex") {
		return true
	}
	return false
}

func (h *Host) calcTextControlOffset(el *dom.Element, cssX, cssY float64) int {
	if el == nil {
		return 0
	}
	text := focusedElementValue(el)
	if len(text) == 0 {
		return 0
	}

	_, bx, by, bw, _, sy, st := h.findFormControlBox(el)
	if bx == 0 && bw == 0 {
		return 0
	}

	// Use the element's computed style font/padding so caret placement
	// matches the painted text (previously a hardcoded Consolas 14 made
	// multi-line caret land at the wrong column, and the Y axis was
	// ignored entirely so clicking line 2+ always hit line 1).
	fontSize := 14.0
	family := "Consolas"
	weight := 400
	if st != nil {
		if st.FontSize.Value > 0 && !st.FontSize.IsAuto() {
			fontSize = st.FontSize.Value
		}
		if st.FontFamily != "" {
			family = st.FontFamily
		}
		if w, err := strconv.Atoi(st.FontWeight); err == nil && w >= 600 {
			weight = 700
		} else if strings.EqualFold(st.FontWeight, "bold") {
			weight = 700
		}
	}
	font := graphics.Font{Family: family, Size: fontSize, Weight: weight}
	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = fontSize * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	// Line height: honor the CSS line-height (multiplier, px, or %) so the
	// caret line index matches the painted text. A textarea with
	// line-height:1.5 at 13px draws 19.5px rows; using font metrics alone
	// (~16px) put clicks on line 2+ at the wrong row.
	lineH := 0.0
	if st != nil {
		switch st.LineHeight.Unit {
		case "px":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value
			}
		case "%":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value / 100 * fontSize
			}
		case "":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value * fontSize
			}
		}
	}
	if lineH <= 0 {
		lineH = ascent + descent
	}
	if lineH <= 0 {
		lineH = fontSize * 1.2
	}
	padX := 4.0
	padY := 4.0
	if st != nil {
		if v := st.PaddingLeft.Value; v > 0 && !st.PaddingLeft.IsAuto() {
			padX = v
		}
		if v := st.PaddingTop.Value; v > 0 && !st.PaddingTop.IsAuto() {
			padY = v
		}
	}

	wrapMode := 0
	if st != nil && el.LocalName() == "textarea" {
		wrapMode = rendering.TextareaWrapMode(st, el)
	}
	return rendering.CalcFormControlCaretOffset(text, el.LocalName() == "textarea",
		cssX, cssY, bx, by, bw, font, padX, padY, lineH, wrapMode, sy)
}

// findFormControlBox walks the render tree to find the absolute border-box
// position/size and computed style of a form-control element.
func (h *Host) findFormControlBox(el *dom.Element) (box *rendering.RenderBox, bx, by, bw, bh, sy float64, st *style.ComputedStyle) {
	if el == nil || h.wv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	rv := h.wv.RenderView()
	if rv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	var found *rendering.RenderBox
	var walk func(rendering.RenderObject) bool
	walk = func(o rendering.RenderObject) bool {
		if o == nil {
			return false
		}
		if n := o.Node(); n != nil {
			if e, ok := n.(*dom.Element); ok && e == el {
				if b, ok := o.(*rendering.RenderBox); ok {
					found = b
					return true
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(rendering.RenderObject(rv))
	if found == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	bx = found.AbsoluteX()
	by = found.AbsoluteY()
	bw = found.Width()
	bh = found.Height()
	st = found.Style()
	_, sy = rv.BoxScrollOffset(found)
	return found, bx, by, bw, bh, sy, st
}

// ensureFocusedCaretVisible auto-scrolls the focused form control so the
// caret stays inside the visible content area after keyboard navigation,
// typing or paste (browser behavior). Horizontal auto-scroll already happens
// at paint time (computeTextScrollX keeps the caret visible in pre/nowrap
// rows); this handles the VERTICAL axis for textareas — moving the caret to
// a row above/below the viewport scrolls BoxScrollOffset.sy so the row is
// revealed, matching how clicking/dragging in a scrolled textarea works.
func (h *Host) ensureFocusedCaretVisible() {
	el := h.imeFocusedEl
	if el == nil || el.LocalName() != "textarea" {
		return
	}
	if h.wv == nil {
		return
	}
	// The input path runs RebuildRenderTree() (brand-new RenderBox with
	// zeroed geometry) immediately before this. Without an up-to-date
	// layout, findFormControlBox returns bh=0 → viewH clamps to 1 →
	// rowBottom>sy+1 computes a huge newSy and the content jumps out of
	// view on every keystroke (WB_SCROLL_DEBUG proved bh=0.0 and sy
	// oscillating 47↔65.5 per input). Layout first so geometry is valid.
	h.wv.EnsureLayout()
	rv := h.wv.RenderView()
	if rv == nil {
		return
	}
	box, _, _, _, bh, sy, st := h.findFormControlBox(el)
	if box == nil || st == nil {
		return
	}
	if bh <= 0 {
		// Geometry still invalid — never write a scroll derived from it.
		return
	}
	val := focusedElementValue(el)
	runes := []rune(val)
	pos := len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		pos = sel.Start
		if sel.End > pos {
			pos = sel.End
		}
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	if len(runes) == 0 {
		return
	}

	fontSize := 14.0
	family := "Consolas"
	weight := 400
	if st.FontSize.Value > 0 && !st.FontSize.IsAuto() {
		fontSize = st.FontSize.Value
	}
	if st.FontFamily != "" {
		family = st.FontFamily
	}
	if w, err := strconv.Atoi(st.FontWeight); err == nil && w >= 600 {
		weight = 700
	} else if strings.EqualFold(st.FontWeight, "bold") {
		weight = 700
	}
	font := graphics.Font{Family: family, Size: fontSize, Weight: weight}

	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = fontSize * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	lineH := cssControlLineHeightForHost(st, fontSize)
	if lineH <= 0 {
		lineH = ascent + descent
	}
	if lineH <= 0 {
		lineH = fontSize * 1.2
	}
	padY := 4.0
	padB := 4.0
	if st.PaddingTop.Value > 0 && !st.PaddingTop.IsAuto() {
		padY = st.PaddingTop.Value
	}
	if st.PaddingBottom.Value > 0 && !st.PaddingBottom.IsAuto() {
		padB = st.PaddingBottom.Value
	}

	// Content box height (the viewport the painter clips to).
	viewH := bh - padY - padB
	if viewH < 1 {
		viewH = 1
	}

	// Caret's visual row (soft-wrapped) and its top/bottom in content space.
	mode := rendering.TextareaWrapMode(st, el)
	contentW := st2ContentWidth(st, box) // same as painter's contentW
	row := rendering.TextareaCaretVisualRow(val, font, contentW, mode, pos)
	rowTop := padY + float64(row)*lineH
	rowBottom := rowTop + lineH

	newSy := sy
	if rowTop < sy {
		newSy = rowTop
	} else if rowBottom > sy+viewH {
		newSy = rowBottom - viewH
	}
	if newSy < 0 {
		newSy = 0
	}
	// Clamp to the same max as the painter / scrollbar drag (shared
	// geometry) so auto-scroll never overshoots the thumb's range.
	maxScroll := 0.0
	if m := rendering.VerticalScrollbarMetrics(rv, box); m.OK {
		maxScroll = m.MaxScroll
		if newSy > m.MaxScroll {
			newSy = m.MaxScroll
		}
	}
	if os.Getenv("WB_SCROLL_DEBUG") != "" {
		log.Printf("[scroll/ensure] bh=%.1f sy=%.1f viewH=%.1f lineH=%.1f contentW=%.1f mode=%v pos=%d row=%d rowTop=%.1f rowBottom=%.1f maxScroll=%.1f -> newSy=%.1f (changed=%v)",
			bh, sy, viewH, lineH, contentW, mode, pos, row, rowTop, rowBottom, maxScroll, newSy, newSy != sy)
	}
	if newSy != sy {
		rv.SetBoxScrollOffset(box, 0, newSy)
	}
}

// cssControlLineHeightForHost mirrors rendering.cssControlLineHeight for the
// host's hit-test / caret math (px, multiplier, or %).
func cssControlLineHeightForHost(st *style.ComputedStyle, fontSize float64) float64 {
	switch st.LineHeight.Unit {
	case "px":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value
		}
	case "%":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value / 100 * fontSize
		}
	case "":
		if st.LineHeight.Value > 0 {
			return st.LineHeight.Value * fontSize
		}
	}
	return 0
}

// st2ContentWidth returns the textarea's content width (padding-box minus
// horizontal padding), matching the painter's contentW.
func st2ContentWidth(st *style.ComputedStyle, box *rendering.RenderBox) float64 {
	pb := box.PaddingBoxRect()
	padL := 4.0
	padR := 4.0
	if st.PaddingLeft.Value > 0 && !st.PaddingLeft.IsAuto() {
		padL = st.PaddingLeft.Value
	}
	if st.PaddingRight.Value > 0 && !st.PaddingRight.IsAuto() {
		padR = st.PaddingRight.Value
	}
	cw := pb.Width - padL - padR
	if cw < 1 {
		cw = 1
	}
	return cw
}

// Unfocus clears the IME focus and disables text input on the platform
// window. It removes the blinking caret and clears the form control selection.
// Call this when the user clicks outside an editable element.
func (h *Host) Unfocus() {
	if h.imeFocusedEl != nil {
		blurEl := h.imeFocusedEl
		// ★ 派发 blur DOM 事件（不冒泡）：xterm 监听 textarea blur →
		// isFocused=false → 隐藏光标。对称于 FocusElementByKeyboard 的
		// focus 派发。
		blurEl.DispatchEvent(dom.NewEvent("blur", false, false, false))
		blurEl.SetFocused(false)
		h.imeFocusedEl = nil
		h.imeInputText = ""
		h.imeComposing = false
		h.imeComposeText = ""
		h.imeComposeBase = ""
		h.imeComposeStart = 0
		rendering.FocusedFormControl = nil
		rendering.FocusedFormControlSel = nil
		rendering.CaretVisible = false
		rendering.CaretVisibleControl = false
		h.win.SetIMEEnabled(false)
	}
}

// updateSelection updates the current text selection state from form-control
// selection data. It is called after mouse/touch events modify the selection.
func (h *Host) updateSelection(rv *rendering.RenderView) {
	// The form-control selection is already updated by calcTextControlOffset
	// during mouse event processing. This method exists as a hook for future
	// selection-change event dispatch.
}

// SetIMECompositionPos updates the IME composition/candidate window position
// to the given CSS-pixel coordinates (relative to the window). The Host
// converts these to physical pixels before forwarding to the platform window.
func (h *Host) SetIMECompositionPos(cssX, cssY float64) {
	h.win.SetIMECompositionPos(cssX, cssY)
}

// Run starts the render + event loop. It blocks until the window is closed.
// Each iteration: layouts the WebView, paints onto the GPU surface, presents,
// then processes input events (resize / scroll / mouse click / IME).
func (h *Host) Run() {
	// Set up the keyframes lookup bridge so the rendering package can find
	// @keyframes rules stored in the style resolver.
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			if rsv := fr.Resolver(); rsv != nil {
				rendering.KeyframesLookup = func(name string) *css.KeyframesRule {
					return rsv.LookupKeyframes(name)
				}
			}
		}
	}

	h.animStart = time.Now()
	h.firstFrame = true
	h.snapEnabled = os.Getenv("WB_SNAP") != ""
	h.snapLast = time.Now()

	// Get the FrameView for scroll management.
	frameView := h.wv.Page().MainFrame().View()
	if frameView == nil {
		return
	}

	h.frameView = frameView

	// 性能统计（WB_PERF_DEBUG=1）：统计帧总数 / 渲染帧 / 空闲帧，窗口
	// 关闭时输出，用于验证按需渲染（空闲帧跳过 Paint/Present）。
	perfDebug := os.Getenv("WB_PERF_DEBUG") != ""
	perfFrames, perfRenders := 0, 0
	var perfStart time.Time
	if perfDebug {
		perfStart = time.Now()
	}
	// WB_AUTODRAG=1：自动模拟 textarea resize 拖拽（找第一个 textarea，
	// 每秒约 60 帧逐帧 +5px，60 帧后释放），配合 [ft] 帧耗时日志量化
	// 真实窗口 + GPU paint + vsync 下的拖拽帧时间——验证「高度跟手」。
	h.autodragOn = os.Getenv("WB_AUTODRAG") != ""
	if h.autodragOn {
		h.autodragStep = -999
	}

	for !h.win.ShouldClose() {
		perfFrames++
		phaseDebug := os.Getenv("WB_PHASE_DEBUG") != ""
		var tPhaseStart time.Time
		if phaseDebug {
			tPhaseStart = time.Now()
		}
		// WB_FRAMETIME=1：逐帧测量 layout/paint/events 耗时（拖拽跟手
		// 性能验证；autodrag 激活时强制开启）。
		ftMode := h.autodragOn || os.Getenv("WB_FRAMETIME") != ""
		var ftFrameStart, ftLayoutEnd, ftPaintEnd time.Time
		if ftMode {
			ftFrameStart = time.Now()
		}
		// Fetch the GPU surface fresh each frame: resize callbacks release
		// and recreate the surface, so the cached pointer would be dangling.
		gpuSurf := h.win.GPUSurface()
		// Ensure the viewport matches the current window size. This is called
		// every frame and is a no-op (FrameView.SetSize checks for actual change)
		// but catches resize events that the FramebufferSizeCallback may have
		// missed (e.g. maximize/un-maximize on some GLFW/platform combos).
		h.wv.Resize(h.win.Width(), h.win.Height())

		// ★ DOM 结构变更回调（appendChild 等 → MarkRenderTreeDirty）——
		// 每帧幂等注册，LoadHTML 重建 Document 时自动跟随。
		h.ensureTreeChangeHook()

		// ★ 事件处理提前到布局之前（浏览器语义：输入 → JS handler →
		// dirty → 同帧布局+渲染）。此前 processEvents 在 Paint 之后，
		// mousemove 派发 → JS 改宽度 → 布局要等下一帧 → 拖拽延迟 2 帧
		// （~33ms @60fps）——「分隔栏拖拽不跟手」根因。提前后 mousemove
		// → Vue onMove 改 style → MarkRenderTreeDirty → 本帧 EnsureLayout
		// + Paint 反映新宽度（0 帧延迟）。rv 用上一帧布局后的渲染树
		// （hover/滚动条几何 1 帧滞后可接受，mousemove 派发不依赖几何）。
		h.processEvents(h.wv.RenderView())

		h.wv.EnsureLayout()
		if ftMode {
			ftLayoutEnd = time.Now()
		}
		rv := h.wv.RenderView()
		// ★ 每帧检查 ResizeObserver（布局后）：尺寸变化 → 触发回调 →
		// xterm FitAddon 按容器尺寸 resize（此前 stub 不通知 → xterm
		// 保持 80x24 超出容器 → 底部内容/光标被裁剪不可见）。每帧
		// GetElementBoxRect 对少量被观察元素（1 个）开销可忽略；重建
		// 风暴已由 rebuildCooldown 降频防护。
		if interp := h.wv.JSInterpreter(); interp != nil {
			bindings.ResizeObserverCheck(interp)
		}

		// Smooth wheel scrolling: interpolate the per-box scroll offset
		// toward the wheel-event target with an exponential approach
		// (browser-like). Scrollbar thumb drags bypass this (1:1 direct
		// writes), so the thumb never lags the cursor.
		if h.smoothActive && rv != nil && h.smoothBox != nil {
			// ★ 渲染树可能已被重建（hover 变化触发 MarkRenderTreeDirty +
			// SetNeedsLayout → 下帧 RebuildRenderTreeIfNeeded 重建整棵树，
			// 新 RenderBox 实例）。h.smoothBox 是旧树指针，直接
			// SetBoxScrollOffset(旧box) 写入的偏移在新树上读不到 →
			// 滚动条 thumb 跟着动但内容不滚。必须每帧按 DOM 节点
			// 重新解析当前树中的 box 再写入。
			// ★ iframe 子文档滚动：偏移表在子 Frame 的 RenderView 里
			// （smoothRV，滚轮命中子 Frame 滚动容器时设置）。主 rv 的
			// FindRenderBoxForNode 查不到子文档 box——必须用子 rv。
			srv := h.smoothRV
			if srv == nil {
				srv = rv
			}
			smoothBox := srv.FindRenderBoxForNode(h.smoothBox.Node())
			if smoothBox == nil {
				// 容器被移除/不可达：放弃平滑滚动。
				h.smoothActive = false
			} else {
				now := time.Now()
				dt := now.Sub(h.smoothLast).Seconds()
				h.smoothLast = now
				if dt > 0 && dt < 0.1 {
					f := 1 - math.Exp(-dt*12)
					h.smoothCurX += (h.smoothTarX - h.smoothCurX) * f
					h.smoothCurY += (h.smoothTarY - h.smoothCurY) * f
					if math.Abs(h.smoothTarX-h.smoothCurX) < 0.5 && math.Abs(h.smoothTarY-h.smoothCurY) < 0.5 {
						h.smoothCurX, h.smoothCurY = h.smoothTarX, h.smoothTarY
						h.smoothActive = false
					}
					srv.SetBoxScrollOffset(smoothBox, h.smoothCurX, h.smoothCurY)
					// 更新持有的 box 引用，避免每帧重复查找。
					h.smoothBox = smoothBox
					// ★ 派发 scroll DOM 事件，让前端 @scroll 监听器（Vue
					// 懒加载向上翻页 loadMoreMessages 等）感知滚动偏移变化。
					// 此前滚轮只更新引擎内偏移从不派发事件——JS 的
					// el.addEventListener('scroll') 永远收不到回调，历史对话
					// 向上翻页永不触发，打开会话只显示初始 limit=50 条
					// 原始行（≈最后一个 run）。
					if n := smoothBox.Node(); n != nil {
						if el, ok := n.(*dom.Element); ok {
							el.DispatchEvent(dom.NewEvent("scroll", false, false, false))
						}
					}
				}
			}
		}
		if DumpRTCallback != nil && rv != nil && h.needsResizeDump {
			DumpRTCallback(rv)
			h.needsResizeDump = false
		}
		// ★ WB_RESIZE_DEBUG=1：resize 后 dump 布局根 + 关键节点几何 + 滚动 + canvas，
		//   验证最大化/拖拽后布局是否跟随 viewport 更新（"内容绘制区域变小/编辑区偏移"）。
		if os.Getenv("WB_RESIZE_DEBUG") != "" && rv != nil && h.needsResizeDump && h.wv.MainFrame() != nil {
			if fr2 := h.wv.MainFrame().Frame(); fr2 != nil {
				if lb := rv.LayoutBox(); lb != nil {
					st := rv.LayoutState()
					g := st.GeometryForBox(lb)
					fv := fr2.View()
					log.Printf("[resize-aft] layoutRoot=(%.0f,%.0f %.0fx%.0f) viewport=%dx%d scrollY=%d contentSize=%dx%d",
						g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight(),
						fv.Width(), fv.Height(), fv.ScrollY(), fv.ContentWidth(), fv.ContentHeight())
					// 关键节点 dump（main-area / right-container / editor-area）
					var walkD func(ro rendering.RenderObject, depth int)
					walkD = func(ro rendering.RenderObject, depth int) {
						if ro == nil || depth > 14 {
							return
						}
						if n := ro.Node(); n != nil {
							if el, ok := n.(*dom.Element); ok {
								cls := el.GetAttribute("class")
								if cls == "main-area" || cls == "right-container" || cls == "editor-area" || cls == "right-panel" || cls == "sidebar" || cls == "activity-bar" {
									if rb := ro.LayoutBox(); rb != nil {
										gg := st.GeometryForBox(rb)
										log.Printf("[resize-aft] %s=(%.0f,%.0f %.0fx%.0f)", cls, gg.Left(), gg.Top(), gg.BorderBoxWidth(), gg.BorderBoxHeight())
									}
								}
							}
						}
						for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
							walkD(c, depth+1)
						}
					}
					walkD(rendering.RenderObject(rv), 0)
				}
				h.needsResizeDump = false
			}
		}
		animActive := false
		if rv != nil {
			// Drive CSS animations: update the global animation clock and
			// apply animated opacity to elements' ComputedStyle before paint.
			// CSS transitions interpolate style changes (:hover / :checked);
			// while one is in flight the frame needs a re-layout every frame
			// so interpolated left/top geometry updates (switch thumb slide).
			rendering.AnimationTime = time.Since(h.animStart).Seconds()
			animActive = rendering.ApplyAnimations(rv)
			if animActive {
				// ★ 只有影响布局的动画（transform/几何）才 SetNeedsLayout。
				// 颜色/opacity 动画（光标闪烁等无限动画）只重绘——否则每帧
				// 全量 relayout/重建（复杂页面 169ms+）→ 帧率 1.4fps
				// （「频繁无响应」主因）。重绘由 needPaint（含 animActive）
				// 覆盖。
				if rendering.AnimationsAffectLayout(rv) {
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.SetNeedsLayout(true)
						}
					}
				}
			}
		}

		// Blink the caret at ~500ms intervals, mirroring WebKit's
		// caret blink cycle. The caret is only visible when an IME
		// focus target is set or a non-selection click positioned it.
		// CaretVisibleControl follows the same cycle for form-control
		// carets (which are drawn by paintFormControlCaret, not PaintCaret).
		// A tick flags the frame that must re-render (blink state flipped).
		caretTick := false
		if rv != nil && time.Since(h.caretBlinkTime) > 500*time.Millisecond {
			rendering.CaretVisible = !rendering.CaretVisible
			rendering.CaretVisibleControl = rendering.CaretVisible
			h.caretBlinkTime = time.Now()
			caretTick = true
		}

		// ★ 按需渲染（性能核心）：仅当本帧存在任何视觉变化时才执行
		// Clear + Paint + Present；空闲帧（无 dirty、无滚动插值、无动画、
		// 无 caret 翻转、非首帧）完全跳过渲染，CPU/GPU 占用趋近于零。
		// 所有变化来源都会标记 dirty 或置位本判定：
		//   - 布局/渲染树重建（hover、DOM 变更、resize、IME 输入）→
		//     FrameView.Layout → MarkAllDirty（frameview.go）
		//   - box 滚动 / 页面滚动 → SetBoxScrollOffset / SetScrollOffset
		//     → MarkAllDirty（renderview.go / frameview.go）
		//   - 平滑滚动插值进行中 → h.smoothActive
		//   - CSS 动画/过渡活跃 → animActive（ApplyAnimations 返回值）
		//   - 光标闪烁翻转 → caretTick
		//   - 鼠标移动 → EventCursorMove 无条件 MarkAllDirty（滚动条
		//     hover 高亮依赖 cursor 位置，见 processEvents）
		needPaint := h.firstFrame || (rv != nil && (rv.IsDirty() || h.smoothActive || animActive || caretTick))
		if os.Getenv("WB_PAINT_DEBUG") != "" {
			if needPaint && (h.lastPaintDbg != needPaint || h.dbgFrame%60 == 0) {
				log.Printf("[paintdbg] frame=%d needPaint=%v firstFrame=%v dirty=%v caret=%v rv=%v",
					h.dbgFrame, needPaint, h.firstFrame, rv != nil && rv.IsDirty(), caretTick, rv != nil)
			}
			h.dbgFrame++
		}
		h.firstFrame = false
		if needPaint {
			perfRenders++
		}

		// ★ WB_SNAP=1：周期性布局快照（打开文件前后对比用）。
		//   每 600ms dump 关键布局/几何/CM6 状态到 _layout_snap.log（主循环线程内
		//   执行，避免与 RunJS 并发竞态）。用户操作 desktop（打开文件/滚动）后，
		//   对比快照时间线即可定位"布局异常"发生的时刻与变化。
		if h.snapEnabled && time.Since(h.snapLast) > 600*time.Millisecond {
			h.snapLast = time.Now()
			h.dumpLayoutSnap()
		}

		if needPaint && rv != nil {
			h.paintFrame++
			// Software-rendered backends (X11/Cocoa) expose a CPU canvas; the
			// GLFW GPU backend wraps its framebuffer surface. Unify both into
			// gpuCanvas so the paint + Present path below works on every platform.
			var gpuCanvas *graphics.Canvas
			var ownsCanvas bool
			if gpuSurf != nil && gpuSurf.IsValid() {
				gpuCanvas = graphics.NewCanvasFromSurface(gpuSurf, h.win.FramebufferWidth(), h.win.FramebufferHeight())
				ownsCanvas = true
			} else if sw := h.win.Canvas(); sw != nil {
				gpuCanvas = sw
				ownsCanvas = false
			}
			if gpuCanvas != nil {
				if os.Getenv("WB_PAINT_DEBUG") != "" {
					log.Printf("[paintdbg] gpuCanvas ok %dx%d", gpuCanvas.Width(), gpuCanvas.Height())
				}
				// Update text selection from stored coordinates against the
				// current render tree (robust to rebuilds).
				h.updateSelection(rv)

				// ★ Keep the IME composition/candidate window positioned at the
				// text caret. Previously SetIMECompositionPos was never called,
				// so Windows IME always showed its candidate list at the top-left
				// corner of the screen.
				if rendering.FocusedFormControl != nil {
					if cx, cy, ok := rendering.FormControlCaretPosition(rv); ok {
						// Caret Y is in page coordinates; account for scroll.
						cy -= float64(frameView.ScrollY())
						if cy < 0 {
							cy = 0
						}
						if h.lastIMEX != int32(cx) || h.lastIMEY != int32(cy) {
							h.lastIMEX, h.lastIMEY = int32(cx), int32(cy)
							if os.Getenv("WB_IME_DEBUG") != "" {
								log.Printf("[ime] host SetIMECompositionPos css=(%.0f,%.0f) scrollY=%d", cx, cy, frameView.ScrollY())
							}
							h.win.SetIMECompositionPos(cx, cy)
						}
					} else if os.Getenv("WB_IME_DEBUG") != "" {
						log.Printf("[ime] FormControlCaretPosition not ok (FocusedFormControl set)")
					}
				}

				bgColor := findBodyBgColor(rendering.RenderObject(rv))
				if bgColor.A == 0 {
					bgColor = graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
				}
				gpuCanvas.Clear(bgColor)

				// Clamp scroll offset to valid range after layout.
				scrollY := frameView.ScrollY()
				if scrollY < 0 {
					scrollY = 0
				}
				if maxY := frameView.MaxScrollY(); scrollY > maxY {
					if maxY > 0 {
						log.Printf("[scroll] clamp scrollY from %d to %d (maxY=%d contentH=%d viewportH=%d)\n",
							scrollY, maxY, maxY, frameView.ContentHeight(), frameView.Height())
					}
					scrollY = maxY
				}
				if scrollY != 0 && scrollY != h.lastLoggedScrollY {
					log.Printf("[scroll] scrollY=%d maxY=%d contentH=%d viewportH=%d\n",
						scrollY, frameView.MaxScrollY(), frameView.ContentHeight(), frameView.Height())
					h.lastLoggedScrollY = scrollY
				}
				frameView.SetScrollOffset(frameView.ScrollX(), scrollY)

				// ★ 每帧重置 transform：surface.Canvas() 返回持久 skia canvas
				// （surface 跨帧持有），上一帧的 Scale/Translate/scroll 会
				// 残留累积（指数放大/错位）。ResetMatrix 清为单位矩阵并
				// 同步 c.state。★ Save 必须在 Scale 之后：fixed 层
				// （paintLayerTree 的 fixed 分支）用
				// RestoreToCount(initialSaveCount) 丢弃祖先 clip，
				// initialSaveCount = Paint 入口 SaveCount()。若 Save 在
				// Scale 之前，Saved 状态是单位矩阵（scale=1.0）——
				// RestoreToCount 弹回后 scale 变 1.0，fixed 层之后的全部
				// 内容按 1:1 画到物理 canvas → 界面等比缩小 80% + 右下空白
				// （"打开文件后绘制区域变小"：编辑器引入 fixed/层路径
				// 触发该分支）。Save 在 Scale 之后 → Saved 状态含 scale
				// 1.25，RestoreToCount 恢复正确缩放。
				gpuCanvas.ResetMatrix()
				csX, csY := h.win.ContentScale()
				gpuCanvas.Scale(csX, csY)
				gpuCanvas.Save()
				gpuCanvas.Translate(0, -float64(frameView.ScrollY()))
				dirtyRect := graphics.Rect{X: 0, Y: float64(frameView.ScrollY()), Width: float64(h.win.Width()), Height: float64(h.win.Height())}
				if os.Getenv("WB_RESIZE_DEBUG") != "" && (gpuCanvas.Width() != h.lastDbgCW || gpuCanvas.Height() != h.lastDbgCH) {
					h.lastDbgCW, h.lastDbgCH = gpuCanvas.Width(), gpuCanvas.Height()
					log.Printf("[paint] canvas=%dx%d fb=%dx%d css=%dx%d dirty=(%.0f,%.0f %.0fx%.0f) scale=%.2f",
						gpuCanvas.Width(), gpuCanvas.Height(),
						h.win.FramebufferWidth(), h.win.FramebufferHeight(),
						h.win.Width(), h.win.Height(),
						dirtyRect.X, dirtyRect.Y, dirtyRect.Width, dirtyRect.Height,
						csX)
				}
				// ★ WB_PAINT_LOG=1：Paint 前记录 view 的 dirty 状态——
				//   Paint 内部若 view.IsDirty() 会用 GetDirtyRect() 覆盖
				//   paintRect（只画局部），而 Clear 是全屏 → 局部之外空白
				//   = "内容绘制区域变小"。必须在 Paint 前抓（Paint 后
				//   ClearDirty 就丢了）。
				preDirtyStr := ""
				if os.Getenv("WB_PAINT_LOG") != "" && h.paintLogFile == nil {
					f, err := os.OpenFile("_paint_trace.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
					if err == nil {
						h.paintLogFile = f
					}
				}
				csXv, _ := h.win.ContentScale()
				if h.paintLogFile != nil {
					if rv != nil {
						preDirtyStr = fmt.Sprintf("preDirty=%v", rv.IsDirty())
						if r := rv.GetDirtyRect(); r.Width > 0 || r.Height > 0 {
							preDirtyStr += fmt.Sprintf(" preDR=(%.0f,%.0f %.0fx%.0f)", r.X, r.Y, r.Width, r.Height)
						} else {
							preDirtyStr += " preDR=none"
						}
					} else {
						preDirtyStr = "rv=nil"
					}
				}
				if os.Getenv("WB_CTM_DEBUG") != "" {
					mPre := gpuCanvas.GetMatrix()
					log.Printf("[ctm] PRE-Paint scaleX=%.3f scaleY=%.3f tx=%.1f ty=%.1f saveCount=%d",
						mPre.ScaleX, mPre.ScaleY, mPre.TransX, mPre.TransY, gpuCanvas.SaveCount())
				}
				rendering.Paint(rv, gpuCanvas, dirtyRect)
				gpuCanvas.Restore()
				if os.Getenv("WB_CTM_DEBUG") != "" {
					mPost := gpuCanvas.GetMatrix()
					log.Printf("[ctm] POST-Paint scaleX=%.3f scaleY=%.3f tx=%.1f ty=%.1f saveCount=%d",
						mPost.ScaleX, mPost.ScaleY, mPost.TransX, mPost.TransY, gpuCanvas.SaveCount())
				}

				if h.paintLogFile != nil {
					// 降采样统计非背景像素覆盖率 + 包围盒 + 亮色(编辑器)包围盒（每 8px 步长）
					cov, bb, lbb := "", "", ""
					rootInfo := ""
					if rv != nil {
						if lb := rv.LayoutBox(); lb != nil {
							if st := rv.LayoutState(); st != nil {
								g := st.GeometryForBox(lb)
								rootInfo = fmt.Sprintf(" root=(%.0f,%.0f %.0fx%.0f)", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
							}
						}
						sx, sy := rv.ScrollOffset()
						rootInfo += fmt.Sprintf(" vScroll=(%.0f,%.0f) boxScroll=%v", sx, sy, rv.HasBoxScrollOffset())
					}
					if px := gpuCanvas.Pixels(); len(px) >= 4 {
						cw, ch := gpuCanvas.Width(), gpuCanvas.Height()
						if cw > 0 && ch > 0 {
							nonBg, tot := 0, 0
							minX, minY, maxX, maxY := cw, ch, -1, -1
							lminX, lminY, lmaxX, lmaxY := cw, ch, -1, -1
							lNonBg := 0
							// 背景是 Clear(bgColor) 后的统一色，取左上角像素为参考
							refR, refG, refB, refA := px[0], px[1], px[2], px[3]
							for y := 0; y < ch; y += 8 {
								for x := 0; x < cw; x += 8 {
									idx := (y*cw + x) * 4
									tot++
									if idx+3 < len(px) {
										if px[idx+3] != refA || px[idx] != refR || px[idx+1] != refG || px[idx+2] != refB {
											nonBg++
											if x < minX {
												minX = x
											}
											if x > maxX {
												maxX = x
											}
											if y < minY {
												minY = y
											}
											if y > maxY {
												maxY = y
											}
										}
										// 亮色像素（编辑器浅色背景/文字）：R+G+B 高 → 编辑器实际绘制区
										rp, gp, bp := int(px[idx]), int(px[idx+1]), int(px[idx+2])
										if rp > 180 && gp > 180 && bp > 180 {
											lNonBg++
											if x < lminX {
												lminX = x
											}
											if x > lmaxX {
												lmaxX = x
											}
											if y < lminY {
												lminY = y
											}
											if y > lmaxY {
												lmaxY = y
											}
										}
									}
								}
							}
							cov = fmt.Sprintf("cov=%d%%", 100*nonBg/tot)
							if maxX >= 0 {
								bb = fmt.Sprintf("bb=(%d,%d %dx%d)", minX, minY, maxX-minX+1, maxY-minY+1)
							} else {
								bb = "bb=none"
							}
							if lmaxX >= 0 {
								lbb = fmt.Sprintf("lightBB=(%d,%d %dx%d) lcov=%d%%", lminX, lminY, lmaxX-lminX+1, lmaxY-lminY+1, 100*lNonBg/tot)
							} else {
								lbb = "lightBB=none"
							}
						}
					}
					line := fmt.Sprintf("[%s] css=%dx%d fb=%dx%d canvas=%dx%d scale=%.2f dirty=(%.0f,%.0f %.0fx%.0f) %s %s %s %s %s%s px{side=%s rp=%s st=%s}\n",
						time.Now().Format("15:04:05.000"), h.win.Width(), h.win.Height(),
						h.win.FramebufferWidth(), h.win.FramebufferHeight(),
						gpuCanvas.Width(), gpuCanvas.Height(),
						csXv,
						dirtyRect.X, dirtyRect.Y, dirtyRect.Width, dirtyRect.Height,
						preDirtyStr, cov, bb, lbb, rootInfo, "",
						sampleCanvasPx2(gpuCanvas, 60, 100),   // sidebar (48,30) 物理
						sampleCanvasPx2(gpuCanvas, 536, 100),  // right-panel (429,30) 物理
						sampleCanvasPx2(gpuCanvas, 750, 987)) // status-bar 物理
					h.snapMu.Lock()
					_, _ = h.paintLogFile.WriteString(line)
					_ = h.paintLogFile.Sync()
					h.snapMu.Unlock()
				}
				// ★ WB_DUMP_PNG=1：Paint 后把 canvas（pixelCache 读回）
				//   内容存 PNG。独立于 WB_PAINT_LOG（paintLogFile 未开时
				//   也要能 dump）。⚠️ 每帧 dump 会从 GPU surface 同步
				//   ReadPixels（Snapshot+ReadPixels），主循环被 GPU 队列
				//   阻塞 → 窗口无响应/死锁（用户实测 WB_DUMP_PNG=1 窗口
				//   卡死）。单次 dump 可接受。
				//   ★ 时机：WB_DUMP_PNG_FRAME 指定帧（默认 0=首帧）；终端
				//   fit/渲染完成后（帧 300）dump 才能看到真实间隙/光标
				//   （首帧时终端可能未渲染）。WB_DUMP_PNG 与
				//   WB_DUMP_PNG_FRAME 任一设置都启用，dump 一次后
				//   dumpPNGDone 置位。
				if os.Getenv("WB_DUMP_PNG") != "" && !h.dumpPNGDone {
					fr := 0
					if f := os.Getenv("WB_DUMP_PNG_FRAME"); f != "" {
						fr = atoiOr(f, 0)
					}
					if h.paintFrame >= fr {
						// ★ dump 前读 viewport 区域像素：确认黑色是否在
						// 帧 300 的 canvas 上（after-fill 每帧黑色但
						// dump 无黑色——绘制后被覆盖？）
						if os.Getenv("WB_VIEWPORT_DEBUG") != "" {
							// viewport 物理区域 ~(409,784 397x189)，读底部
							// 中点（物理 500, 970）与 scrollable 底下方
							// （物理 500, 955）
							c1 := gpuCanvas.PixelAt(500, 970)
							c2 := gpuCanvas.PixelAt(500, 955)
							c3 := gpuCanvas.PixelAt(500, 945)
							log.Printf("[viewport] pre-dump px y=970:(%d,%d,%d) y=955:(%d,%d,%d) y=945:(%d,%d,%d)",
								c1.R, c1.G, c1.B, c2.R, c2.G, c2.B, c3.R, c3.G, c3.B)
						}
						h.dumpPNGDone = true
						dumpCanvasPNG(gpuCanvas, "_canvas_dump.png")
					}
				}
				if ownsCanvas {
					gpuCanvas.Release()
				}
				h.win.Present()
				if ftMode {
					ftPaintEnd = time.Now()
				}
			}
		}

		// 驱动 JS 事件循环：处理到期的 setTimeout/setInterval 宏任务、
		// Promise.then 微任务、requestAnimationFrame 动画帧回调。
		h.processEventLoop()
		// ★ 每帧回调（主线程）：宿主用它消费外部队列的 JS 推送（终端
		// PTY 输出等）——goroutine 不得直接 RunJS（goja 非线程安全）。
		if h.OnFrame != nil {
			h.OnFrame()
		}
		// WB_PHASE_DEBUG=1：每 10 帧打印各阶段耗时（定位卡顿帧）。
		if phaseDebug && perfFrames%10 == 0 {
			eventsMS := float64(time.Since(tPhaseStart).Nanoseconds()) / 1e6
			log.Printf("[phase] frame=%d total=%6.1fms", perfFrames, eventsMS)
		}
		// WB_LAYOUT_PROFILE=1：每帧输出布局耗时分布（BFC/FFC/IFC…）。
		if os.Getenv("WB_LAYOUT_PROFILE") != "" && needPaint {
			layout.DumpLayoutProfile()
		}
		if ftMode {
			now := time.Now()
			total := float64(now.Sub(ftFrameStart).Nanoseconds()) / 1e6
			layoutMS := float64(ftLayoutEnd.Sub(ftFrameStart).Nanoseconds()) / 1e6
			// paint 未执行（needPaint=false）时 ftPaintEnd 为零值——用
			// ftLayoutEnd 兜底，paint 记 0。
			paintEnd := ftPaintEnd
			if paintEnd.IsZero() {
				paintEnd = ftLayoutEnd
			}
			paintMS := float64(paintEnd.Sub(ftLayoutEnd).Nanoseconds()) / 1e6
			eventsMS := total - layoutMS - paintMS
			// 只记录有实际绘制的帧（空闲帧跳过 Paint 无意义；拖拽帧
			// needPaint=true 每帧记录，autodragSamples 汇总）。
			if needPaint {
				log.Printf("[ft] total=%7.2fms layout=%6.2f paint=%6.2f events=%6.2f needPaint=%v drag=%v",
					total, layoutMS, paintMS, eventsMS, needPaint, h.autodragOn)
				if h.autodragOn {
					h.autodragSamples = append(h.autodragSamples, total)
				}
			}
		}
	}
	if perfDebug {
		elapsed := time.Since(perfStart).Seconds()
		log.Printf("[perf] frames=%d renders=%d idle=%d (%.1f%% idle) elapsed=%.2fs avgFrame=%.2fms renderRate=%.1ffps",
			perfFrames, perfRenders, perfFrames-perfRenders,
			100*float64(perfFrames-perfRenders)/float64(max(perfFrames, 1)),
			elapsed, 1000*elapsed/float64(max(perfFrames, 1)),
			float64(perfRenders)/max(elapsed, 0.001))
	}
}

// renderChildCount counts direct render children of a render object.
func renderChildCount(ro rendering.RenderObject) int {
	n := 0
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		n++
	}
	return n
}

// dumpCanvasPNG writes the canvas contents to a PNG file (debug aid).
func dumpCanvasPNG(c *graphics.Canvas, path string) {
	px := c.Pixels()
	if len(px) < 4 {
		return
	}
	cw, ch := c.Width(), c.Height()
	if cw <= 0 || ch <= 0 {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			idx := (y*cw + x) * 4
			if idx+3 >= len(px) {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: px[idx], G: px[idx+1], B: px[idx+2], A: px[idx+3]})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

// sampleCanvasPx2 returns the hex color of the canvas pixel at physical (x,y).
// Used by the WB_PAINT_LOG trace to verify whether key regions were painted.
func sampleCanvasPx2(c *graphics.Canvas, x, y int) string {
	if c == nil {
		return "?"
	}
	px := c.Pixels()
	if len(px) < 4 {
		return "?"
	}
	cw, ch := c.Width(), c.Height()
	if cw <= 0 || ch <= 0 {
		return "?"
	}
	ix, iy := x, y
	if ix < 0 {
		ix = 0
	}
	if iy < 0 {
		iy = 0
	}
	if ix >= cw {
		ix = cw - 1
	}
	if iy >= ch {
		iy = ch - 1
	}
	idx := (iy*cw + ix) * 4
	if idx+3 >= len(px) {
		return "?"
	}
	return fmt.Sprintf("#%02x%02x%02x", px[idx], px[idx+1], px[idx+2])
}

// snapLayoutJS collects key layout geometry + CM6 state from the page (WB_SNAP).
const snapLayoutJS = `(function(){  var o = {};
  o.vw = {w: window.innerWidth, h: window.innerHeight};
  o.bodyScrollH = document.body ? document.body.scrollHeight : -1;
  o.deScrollTop = document.documentElement ? document.documentElement.scrollTop : -1;
  function rect(sel, key){
    var el = document.querySelector(sel);
    if (!el) { o[key] = sel + '=NULL'; return; }
    var r = el.getBoundingClientRect();
    var cs = getComputedStyle(el);
    o[key] = sel + '=(' + Math.round(r.left) + ',' + Math.round(r.top) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height) + ') disp=' + cs.display + ' w=' + cs.width + ' pos=' + cs.position + ' ovf=' + cs.overflow;
  }
  rect('.app-root', 'app');
  rect('.main-area', 'main');
  rect('.right-container', 'right');
  rect('.editor-area', 'ea');
  rect('.editor-body', 'eb');
  rect('.editor-wrapper', 'ew');
  rect('.code-editor-wrapper', 'cw');
  rect('.cm-editor', 'cm');
  rect('.status-bar', 'sb');
  rect('.cm-scroller', 'sc');
  rect('.cm-content', 'co');
  // ★ 聊天区滚动诊断：chat-messages 的 scrollTop/scrollHeight/clientHeight
  var cm2 = document.querySelector('.chat-messages');
  if (cm2) { o.cmScrollTop = cm2.scrollTop; o.cmScrollH = cm2.scrollHeight; o.cmClientH = cm2.clientHeight; o.cmChildCount = cm2.children.length; }
  var co = document.querySelector('.cm-content');
  if (co) { o.coChildren = co.children.length; o.coTextLen = (co.textContent || '').length; o.coScrollW = co.scrollWidth; }
  o.lineCount = document.querySelectorAll('.cm-line').length;
  o.cmExists = !!document.querySelector('.cm-editor');
  var sb = document.querySelector('.status-bar');
  if (sb) {
    o.sbText = (sb.textContent || '').replace(/\s+/g, ' ').slice(0, 60);
    o.sbChildren = sb.children.length;
    o.sbHtmlLen = (sb.innerHTML || '').length;
    var csb = getComputedStyle(sb);
    o.sbColor = csb.color;
    o.sbBg = csb.backgroundColor;
    o.sbFontSize = csb.fontSize;
    o.sbFontWeight = csb.fontWeight;
    o.sbFontFamily = csb.fontFamily;
    var sl = document.querySelector('.status-left');
    if (sl) { o.sbLeftChild = sl.children.length; o.sbLeftText = (sl.textContent || '').slice(0, 40); }
    var sr = document.querySelector('.status-right');
    if (sr) { o.sbRightChild = sr.children.length; o.sbRightText = (sr.textContent || '').slice(0, 40); }
  }
  var sc = document.querySelector('.cm-scroller');
  if (sc) { o.scScrollTop = sc.scrollTop; o.scScrollH = sc.scrollHeight; o.scClientH = sc.clientHeight; }
  o.active = (document.querySelector('.file-tree-item.active .item-name') || {}).textContent || '';
  o.ftItems = document.querySelectorAll('.file-tree-item .item-row').length;
  o.errs = (window.__errs || []).join(' ;; ');
  return JSON.stringify(o);
})()`

// dumpLayoutSnap writes a periodic layout snapshot to _layout_snap.log (WB_SNAP).
// Appends with a timestamp so the user's interactions (open file / scroll) can be
// compared along the time axis: find the moment the layout broke and what changed.
func (h *Host) dumpLayoutSnap() {
	if h.wv.JSInterpreter() == nil {
		return
	}
	geo := ""
	v, err := h.wv.JSInterpreter().RunJS(snapLayoutJS)
	if err != nil {
		geo = "[err] " + err.Error()
	} else {
		geo = v.ToString()
	}
	// Go 侧补充：布局根 + 关键渲染节点几何（与 JS getBoundingClientRect 对比）
	extra := ""
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			if rv := fr.RenderView(); rv != nil {
				if lb := rv.LayoutBox(); lb != nil {
					if st := rv.LayoutState(); st != nil {
						g := st.GeometryForBox(lb)
						extra = fmt.Sprintf(" layoutRoot=(%.0f,%.0f %.0fx%.0f)", g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight())
						if fv := fr.View(); fv != nil {
							extra += fmt.Sprintf(" viewport=%dx%d scrollY=%d", fv.Width(), fv.Height(), fv.ScrollY())
						}
					}
				}

			}
		}
	}
	line := fmt.Sprintf("[%s] %s%s\n  [paint] %s", time.Now().Format("15:04:05.000"), geo, extra,
		rendering.SnapshotComponentPaints())
	h.snapMu.Lock()
	defer h.snapMu.Unlock()
	if h.snapFile == nil {
		f, err := os.OpenFile("_layout_snap.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return
		}
		h.snapFile = f
	}
	fmt.Fprintln(h.snapFile, line)
	_ = h.snapFile.Sync()
}


// processEventLoop 驱动 JS 事件循环（对标浏览器事件循环模型）。
// 在渲染循环中每帧调用，处理到期的宏任务、微任务和动画帧回调。
func (h *Host) processEventLoop() {
	interp := h.wv.JSInterpreter()
	if interp == nil {
		return
	}
	el := interp.GetEventLoop()
	if el == nil {
		return
	}
	elapsedMs := time.Since(h.animStart).Milliseconds()
	el.ProcessTasks(elapsedMs)
}

// processEvents drains the platform event queue and dispatches each event.
// Resize updates the WebView viewport; scroll adjusts scrollY; mouse clicks
// are hit-tested against the render tree and forwarded to the click handler.
// Mouse drag/move drive text selection; Ctrl+C copies selected text.
// ─── hover style fast-path ────────────────────────────────
//
// A :hover switch normally triggers a FULL render-tree rebuild + layout +
// repaint (each O(content)). For a mouse move that only changes which element
// is hovered this is the dominant cost — with 10k+ render objects a single
// hover change took multiple seconds ("UI responds slowly as content grows").
//
// The fast path re-resolves the :hover-affected styles of ONLY the old/new
// hovered elements, swaps them onto the existing render objects, and marks
// just their regions dirty. Layout is only re-run when the computed style
// change actually affects box geometry (fonts / box model / display / flex);
// pure visual changes (color / background / shadow / opacity) repaint the
// element's rect in place.

// hoverStyleFastPath applies the :hover style change for oldEl → newEl without
// rebuilding the render tree. Falls back to a full rebuild+layout when the
// style change affects layout (geometry).
// updateCursor 根据悬停元素计算窗口光标，对齐 WebKit EventHandler::selectCursor
// + CursorWin 的语义：
//
//	1. CSS cursor 显式值优先（pointer→手型、text→IBeam、nwse-resize→↘、
//	   nesw-resize→↙、ns/ew-resize→上下/左右箭头；default/auto 走第 2 步）。
//	2. cursor:auto（默认）判定（WebKit useHandCursor 语义）：
//	   - a[href] 链接 → 手型；
//	   - textarea 右下角 resize 手柄 → nwse-resize（与 Press 命中一致）；
//	   - 文本输入控件（input 非按钮类 / textarea 本体）→ IBeam；
//	   - 其余一律默认箭头——包括 button/select/checkbox/radio/range/label
//	     等原生控件。Chromium/Edge 在 Windows 上这些控件 hover 都是箭头，
//	     只有 cursor:pointer 的元素才显示手型。之前把这些控件全部设为手型
//	     导致「鼠标指针不对」（移过按钮/滑块到处是小手）。
//
// 只在形状变化时调用 SetCursorShape（避免每帧重设系统光标）。
// h.hoveredEl 已由 Move 处理更新。
func (h *Host) updateCursor(rv *rendering.RenderView, cssX, cssY float64) {
	if h.win == nil {
		return
	}
	// 拖动中保持按下时记录的 resize 光标（鼠标移出手柄区域也不变，
	// 浏览器行为）；否则按当前 hover 元素计算。
	shape := cursorShapeForElement(rv, h.hoveredEl, cssX, cssY)
	if h.resizeDragEl != nil {
		shape = h.resizeDragCursor
	}
	if shape != h.lastCursor {
		h.lastCursor = shape
		h.win.SetCursorShape(shape)
	}
}

// cursorShapeForElement 计算某元素应显示的窗口光标形状（纯函数，便于
// 单测）。语义对齐 WebKit EventHandler::selectCursor + CursorWin：
//
//	1. CSS cursor 显式值优先（pointer→手型、text→IBeam、nwse-resize→↘、
//	   nesw-resize→↙、ns/ew-resize→上下/左右箭头；default/auto 走第 2 步）。
//	2. cursor:auto（默认）判定（WebKit useHandCursor 语义）：
//	   - a[href] 链接 → 手型；
//	   - textarea 右下角 resize 手柄 → nwse-resize（与 Press 命中一致）；
//	   - 文本输入控件（input 非按钮类 / textarea 本体）→ IBeam；
//	   - 其余一律默认箭头——包括 button/select/checkbox/radio/range/label
//	     等原生控件。Chromium/Edge 在 Windows 上这些控件 hover 都是箭头，
//	     只有 cursor:pointer 的元素才显示手型。之前把这些控件全部设为手型
//	     导致「鼠标指针不对」（移过按钮/滑块到处是小手）。
func cursorShapeForElement(rv *rendering.RenderView, el *dom.Element, cssX, cssY float64) window.CursorShape {
	shape := window.CursorArrow
	if rv == nil || el == nil {
		return shape
	}
	var st *style.ComputedStyle
	if rb := rv.FindRenderBoxForNode(el); rb != nil {
		st = rb.Style()
	}
	// ① CSS cursor 显式值（浏览器：cursor 属性决定，优先于元素类型）。
	if st != nil && st.Cursor != "" && st.Cursor != "auto" {
		switch st.Cursor {
		case "pointer":
			return window.CursorHand
		case "text", "vertical-text":
			return window.CursorIBeam
		case "nwse-resize":
			return window.CursorNWSE
		case "nesw-resize":
			return window.CursorNESW
		case "ns-resize", "row-resize", "n-resize", "s-resize":
			return window.CursorNS
		case "ew-resize", "col-resize", "e-resize", "w-resize":
			return window.CursorEW
		}
	}
	// ② cursor:auto 语义（WebKit useHandCursor：链接→手型；
	// 可编辑文本→IBeam；其余原生控件→箭头）。
	switch el.LocalName() {
	case "a":
		// 仅带 href 的链接（WebKit isOverLink；a 无 href 是箭头）。
		if el.GetAttribute("href") != "" {
			return window.CursorHand
		}
	case "input":
		switch el.GetAttribute("type") {
		case "checkbox", "radio", "submit", "reset", "button", "range", "color", "file", "hidden":
			// 原生控件：浏览器默认箭头（不是手型）。
			return window.CursorArrow
		default:
			return window.CursorIBeam
		}
	case "textarea":
		// 右下角 15px resize 手柄优先（与 Press 手柄命中区域一致，
		// 视口坐标）。光标方向按 resize 模式区分——对齐浏览器：
		// resize:vertical → 垂直双向箭头（NS）；horizontal → 水平
		// 双向箭头（EW）；both → 斜向双向箭头（NWSE）。Edge 实测：
		// 固定宽度 textarea（resize:vertical）手柄是垂直指针，不是斜的
		// （之前无条件 NWSE 导致光标与浏览器不符）。
		if st != nil && rendering.ResizeModeOf(st) != 0 {
			if rb := rv.FindRenderBoxForNode(el); rb != nil {
				bx, by, bw, bh := rendering.BoxViewportRect(rv, rb)
				if cssX > bx+bw-15 && cssY > by+bh-15 {
					switch rendering.ResizeModeOf(st) {
					case 2: // vertical
						return window.CursorNS
					case 1: // horizontal
						return window.CursorEW
					default: // both
						return window.CursorNWSE
					}
				}
			}
		}
		return window.CursorIBeam
	}
	return window.CursorArrow
}

func (h *Host) hoverStyleFastPath(rv *rendering.RenderView, fr *page.Frame, oldEl, newEl *dom.Element) {
	if fr == nil || rv == nil {
		return
	}
	resolver := fr.Resolver()
	if resolver == nil {
		return
	}
	// ★ :hover 冒泡匹配：el 自身 hovered 时其全部祖先经 hasHoveredDescendant
	// 也匹配 :hover。因此 hover 切换必须重算 old/new 两元素的完整祖先链
	// （去重），否则旧祖先的 :hover 样式残留缓存（视觉上多个高亮并存）、
	// 新祖先的 :hover 也不生效。之前只重算 oldEl/newEl 两个叶子节点，
	// 悬停从一处移到另一处时旧容器高亮残留。
	seen := map[*dom.Element]bool{}
	var els []*dom.Element
	for _, el := range []*dom.Element{oldEl, newEl} {
		for e := el; e != nil; e = e.ParentElement() {
			if !seen[e] {
				seen[e] = true
				els = append(els, e)
			}
		}
	}
	layoutDirty := false
	for _, el := range els {
		resolver.Invalidate(el)
		newCS := resolver.ResolveElement(el)
		if newCS == nil {
			continue
		}
		ro := findRenderObjectForNode(rendering.RenderObject(rv), el)
		if ro == nil {
			continue
		}
		oldCS := ro.Style()
		if !layoutDirty && layoutAffectingChanged(oldCS, newCS) {
			layoutDirty = true
		}
		ro.SetStyle(newCS)
		// Mark the element's region dirty for a local repaint (pad for
		// hover shadows/borders). Scroll containers disable the dirty check
		// at paint time, so correctness is preserved there.
		if lb := ro.LayoutBox(); lb != nil && rv.LayoutState() != nil {
			g := rv.LayoutState().GeometryForBox(lb)
			r := rendering.Rect{X: g.Left() - 2, Y: g.Top() - 2, Width: g.BorderBoxWidth() + 4, Height: g.BorderBoxHeight() + 4}
			if r.Width > 0 && r.Height > 0 {
				rv.MarkDirty(r)
			}
		} else {
			rv.MarkAllDirty()
		}
	}
	if layoutDirty {
		fr.SetNeedsLayout(true)
	}
}

// layoutAffectingChanged reports whether a computed-style change from a to b
// would alter box geometry (requiring a layout pass). Pure visual properties
// (color, background, shadow, opacity, transform…) return false — they only
// need a repaint. Positioned offsets (top/left/…) are not tracked here; hover
// rules never change them in practice.
func layoutAffectingChanged(a, b *style.ComputedStyle) bool {
	if a == nil || b == nil {
		return true
	}
	// Inline formatting / font / wrapping (inherit into text layout).
	if a.FontSize != b.FontSize || a.FontFamily != b.FontFamily || a.FontWeight != b.FontWeight ||
		a.FontStyle != b.FontStyle || a.LineHeight != b.LineHeight || a.LetterSpacing != b.LetterSpacing ||
		a.WordSpacing != b.WordSpacing || a.TextIndent != b.TextIndent || a.TextAlign != b.TextAlign ||
		a.WhiteSpace != b.WhiteSpace || a.WordBreak != b.WordBreak || a.OverflowWrap != b.OverflowWrap ||
		a.WritingMode != b.WritingMode || a.Direction != b.Direction || a.Visibility != b.Visibility {
		return true
	}
	// Box model / positioning / display / flex / grid / multi-column.
	if a.Display != b.Display || a.Position != b.Position || a.Float != b.Float || a.Clear != b.Clear ||
		a.OverflowX != b.OverflowX || a.OverflowY != b.OverflowY ||
		a.Width != b.Width || a.Height != b.Height || a.MinWidth != b.MinWidth || a.MinHeight != b.MinHeight ||
		a.MaxWidth != b.MaxWidth || a.MaxHeight != b.MaxHeight ||
		a.MarginTop != b.MarginTop || a.MarginRight != b.MarginRight || a.MarginBottom != b.MarginBottom || a.MarginLeft != b.MarginLeft ||
		a.PaddingTop != b.PaddingTop || a.PaddingRight != b.PaddingRight || a.PaddingBottom != b.PaddingBottom || a.PaddingLeft != b.PaddingLeft ||
		a.BorderTopWidth != b.BorderTopWidth || a.BorderRightWidth != b.BorderRightWidth ||
		a.BorderBottomWidth != b.BorderBottomWidth || a.BorderLeftWidth != b.BorderLeftWidth ||
		a.BorderTopStyle != b.BorderTopStyle || a.BorderRightStyle != b.BorderRightStyle ||
		a.BorderBottomStyle != b.BorderBottomStyle || a.BorderLeftStyle != b.BorderLeftStyle ||
		a.BoxSizing != b.BoxSizing || a.BorderRadius != b.BorderRadius ||
		a.FlexDirection != b.FlexDirection || a.FlexWrap != b.FlexWrap ||
		a.JustifyContent != b.JustifyContent || a.AlignItems != b.AlignItems || a.AlignContent != b.AlignContent ||
		a.Gap != b.Gap || a.RowGap != b.RowGap || a.ColumnGap != b.ColumnGap ||
		a.FlexBasis != b.FlexBasis || a.FlexGrow != b.FlexGrow || a.FlexShrink != b.FlexShrink || a.Order != b.Order ||
		a.AlignSelf != b.AlignSelf || a.JustifySelf != b.JustifySelf ||
		a.VerticalAlign != b.VerticalAlign || a.ZIndex != b.ZIndex ||
		a.ColumnCount != b.ColumnCount || a.ColumnWidth != b.ColumnWidth ||
		a.GridTemplateColumns != b.GridTemplateColumns || a.GridTemplateRows != b.GridTemplateRows {
		return true
	}
	return false
}

// termTestTick 自动化验证终端（WB_TERM_TEST=1 时每帧调用）：
// ① 第 30 帧点击「新建终端」按钮（.term-create-btn）触发 newTerminal
// ② 第 150 帧查询 xterm DOM（.xterm 是否渲染）与 PTY 输出（term 内容）
// ③ 输出结果后标记完成
func (h *Host) termTestTick() {
	h.termTestStep++
	if os.Getenv("WB_FRAME_DEBUG") != "" {
		if h.termTestStep%20 == 0 {
			log.Printf("[fpsdbg] step=%d now=%s", h.termTestStep, time.Now().Format("15:04:05.000"))
		}
	}
	interp := h.wv.JSInterpreter()
	if interp == nil {
		return
	}
	// WB_TERM_QUERY=1 纯查询模式：不点击、不注入测试数据，只查询终端
	// DOM 状态（xterm rows 渲染 + helper textarea computed style），用于
	// 复现用户真实场景（仅 PTY 真实输出，无任何测试注入）。
	queryMode := os.Getenv("WB_TERM_QUERY") != "" && os.Getenv("WB_TERM_TEST") == ""
	if queryMode && h.termTestStep != 200 && h.termTestStep != 260 && h.termTestStep != 320 && h.termTestStep != 400 && h.termTestStep != 460 && h.termTestStep != 500 {
		return
	}
	switch h.termTestStep {
	case 30:
		if os.Getenv("WB_CLICK_TEST") != "" {
			// ★ 真实点击路径：查按钮中心 → handleClick（HitTest + DOM
			// click 派发），与用户鼠标点击完全一致。dispatchEvent 是
			// 直接触发监听器、绕过 HitTest/handleClick，可能掩盖
			// 「真实点击不生效」问题——用户反馈手动点「新建终端」后
			// 终端空白（只显示输入字符 w），怀疑 handleClick 链路。
			log.Printf("[termtest] 真实 handleClick 点击新建终端")
			v, _ := interp.RunJS(`(function(){
				var btn = document.querySelector('.term-create-btn');
				if (!btn) return '';
				var r = btn.getBoundingClientRect();
				return JSON.stringify({x: r.left + r.width/2, y: r.top + r.height/2});
			})()`)
			var pos struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}
			if err := json.Unmarshal([]byte(v.ToString()), &pos); err == nil && pos.X > 0 {
				csX, csY := h.win.ContentScale()
				if csX <= 0 {
					csX = 1
				}
				if csY <= 0 {
					csY = 1
				}
				rv := h.wv.RenderView()
				if rv != nil {
					log.Printf("[termtest] handleClick at css=(%.0f,%.0f)", pos.X, pos.Y)
					h.handleClick(rv, window.Event{
						Type:   window.EventMouseButton,
						X:      pos.X * csX,
						Y:      pos.Y * csY,
						Button: int(glfw.MouseButton1),
						Action: int(glfw.Release),
					})
				}
			} else {
				log.Printf("[termtest] 按钮 rect 查询失败: %q", v.ToString())
			}
		} else {
			log.Printf("[termtest] 点击新建终端按钮")
			_, _ = interp.RunJS(`(function(){
				var btn = document.querySelector('.term-create-btn');
				if (!btn) { window.__termDiag = 'NO_CREATE_BTN'; return; }
				var ev = new Event('click', {bubbles:true});
				btn.dispatchEvent(ev);
			})()`)
		}
	case 180:
		// ① 测异步调度：xterm 6 的 write 处理用 setTimeout/microtask 调度，
		//   渲染依赖 rAF——若某环不驱动则 DOM 渲染器永不渲染。
		//   ★ 注意：setTimeout/rAF 回调在下一帧 processEventLoop 才执行，
		//   因此结果在 case 200 读取。
		_, _ = interp.RunJS(`(function(){
			window.__st = 0; setTimeout(function(){ window.__st = 1; }, 0);
			window.__pm = 0; Promise.resolve().then(function(){ window.__pm = 1; });
			window.__raf = 0;
			try { requestAnimationFrame(function(){ window.__raf = 1; }); } catch(e) { window.__raf = 'ERR:' + e.message; }
		})()`)
		// ② 手动向终端实例推一段测试文本，区分「Go→JS 推送链路」与
		//   「xterm DOM 渲染器」哪个环节未工作。
		_, _ = interp.RunJS(`(function(){
			var ks = Object.keys(window.__desktopTerminals || {});
			if (ks.length > 0) {
				var ws = window.__desktopTerminals[ks[0]];
				if (ws && typeof ws.onmessage === 'function') {
					ws.onmessage({data: 'TEST-PUSH-123'});
				}
			}
		})()`)
	case 200:
		v, _ := interp.RunJS(`(function(){
			var out = {};
			out.createBtn = !!document.querySelector('.term-create-btn');
			out.xtermCount = document.querySelectorAll('.xterm').length;
			out.rows = document.querySelectorAll('.xterm .xterm-rows > div').length;
			var lines = [];
			var divs = document.querySelectorAll('.xterm .xterm-rows > div');
			for (var i=0;i<Math.min(divs.length, 6);i++){
				lines.push(i + ':[' + (divs[i].textContent || '').slice(0, 80) + ']');
			}
			out.lines = lines.join(' | ');
			out.recv = window.__termRecv || 0;
			out.recvLast = window.__termRecvLast || '';
			// ★ helper textarea 样式诊断：xterm.css 要求 absolute+opacity:0+
			// left:-9999em 脱离文档流；若引擎未应用则 textarea 可见占位
			try {
				var ta = document.querySelector('.xterm-helper-textarea');
				if (ta) {
					var cs = getComputedStyle(ta);
					out.ta = {pos: cs.position, op: cs.opacity, left: cs.left, top: cs.top, w: cs.width, h: cs.height, z: cs.zIndex, disp: cs.display};
					out.taValue = (ta.value || '').slice(0, 40);
					out.taPlaceholder = ta.placeholder || '';
					var r = ta.getBoundingClientRect();
					out.taRect = {x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height)};
				} else { out.ta = 'NO_TEXTAREA'; }
			} catch(e) { out.taErr = String(e.message || e); }
			// xterm 容器几何（终端区域位置）
			try {
				var xt = document.querySelector('.xterm');
				if (xt) {
					var xr = xt.getBoundingClientRect();
					out.xtermRect = {x: Math.round(xr.left), y: Math.round(xr.top), w: Math.round(xr.width), h: Math.round(xr.height)};
					var csx = getComputedStyle(xt);
					out.xtermOverflow = csx.overflow;
				} else { out.xterm = 'NO_XTERM'; }
			} catch(e) { out.xtermErr = String(e.message || e); }
			out.st = window.__st; out.pm = window.__pm; out.raf = window.__raf;
			// xterm 区域含 W 的元素（用户反馈终端显示一长串 W——怀疑 xterm
			// 字符宽度测量 span「WWWW...」被引擎渲染；浏览器中它应
			// visibility:hidden 不显示）
			try {
				var ws = [];
				var xels = document.querySelectorAll('.xterm *');
				for (var i = 0; i < xels.length; i++) {
					var t = (xels[i].textContent || '');
					var tr = t.trim();
					if (tr.indexOf('W') >= 0 && tr.length > 5) {
						var r = xels[i].getBoundingClientRect();
						var csx = getComputedStyle(xels[i]);
						ws.push({tag: xels[i].tagName, cls: (xels[i].className || '').toString().slice(0, 30), txt: t.slice(0, 50), x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height), disp: csx.display, vis: csx.visibility, pos: csx.position, op: csx.opacity});
					}
				}
				out.termW = ws;
				// rows 第一个字符 span 的 computed style（渲染树构建是否跳过）
				try {
					var r0 = document.querySelector('.xterm-rows > div > span, .xterm-rows div span');
					if (r0) {
						var rcs = getComputedStyle(r0);
						out.rowSpan = {txt: r0.textContent.slice(0, 10), disp: rcs.display, vis: rcs.visibility, pos: rcs.position, fontSz: rcs.fontSize, lineH: rcs.lineHeight, h: rcs.height};
						var pr = r0.parentNode;
						var pcs = pr ? getComputedStyle(pr) : null;
						out.rowDiv = pcs ? {disp: pcs.display, vis: pcs.visibility, pos: pcs.position, fontSz: pcs.fontSize, lineH: pcs.lineHeight, h: pcs.height, styleH: pr.style ? pr.style.height : '', styleLH: pr.style ? pr.style.lineHeight : ''} : 'no-parent';
					} else { out.rowSpan = 'NO_SPAN'; }
				} catch(e) { out.rowSpanErr = String(e.message || e); }
				// xterm 测量元素 offsetHeight/offsetWidth（cellHeight/cellWidth
				// 计算来源——若返回 NaN/0 则 xterm 算出 NaN 行高）
				try {
					var ms = document.querySelector('.xterm-char-measure-element');
					if (ms) {
						out.measure = {offsetH: ms.offsetHeight, offsetW: ms.offsetWidth, rectH: ms.getBoundingClientRect().height, rectW: ms.getBoundingClientRect().width};
					}
				} catch(e) { out.measureErr = String(e.message || e); }
				// canvas 2D measureText（xterm 的 cellWidth/cellHeight 计算来源）
				try {
					var cv = document.createElement('canvas');
					var cx = cv.getContext('2d');
					if (cx && cx.measureText) {
						cx.font = '13px monospace';
						var mt = cx.measureText('W');
						out.cvMeasure = {
							width: mt.width, actualBB: mt.actualBoundingBoxAscent, actualBBD: mt.actualBoundingBoxDescent,
							fontBBAscent: mt.fontBoundingBoxAscent, fontBBD: mt.fontBoundingBoxDescent,
							hasAscent: typeof mt.actualBoundingBoxAscent
						};
					} else { out.cvMeasure = 'NO_CTX'; }
				} catch(e) { out.cvMeasureErr = String(e.message || e); }
			} catch(e) { out.termWErr = String(e.message || e); }
			// 文件浏览器诊断：toolbar 标题 + 工作区列表几何/文字（用户反馈
			// 「文件浏览器那几个文字的那一栏显示异常」，查布局与颜色）
			try {
				var fe = document.querySelector('.file-explorer');
				if (fe) {
					var fr = fe.getBoundingClientRect();
					out.fe = {x: Math.round(fr.left), y: Math.round(fr.top), w: Math.round(fr.width), h: Math.round(fr.height)};
					var items = [];
					var wss = document.querySelectorAll('.ws-item');
					for (var i = 0; i < wss.length; i++) {
						var wr = wss[i].getBoundingClientRect();
						var nm = wss[i].querySelector('.ws-name');
						items.push({x: Math.round(wr.left), y: Math.round(wr.top), w: Math.round(wr.width), h: Math.round(wr.height), txt: nm ? nm.textContent.slice(0, 20) : ''});
					}
					out.feWs = items;
					var tt = document.querySelector('.tb-title');
					if (tt) {
						var tr = tt.getBoundingClientRect();
						var tcs = getComputedStyle(tt);
						out.feTitle = {txt: tt.textContent, x: Math.round(tr.left), y: Math.round(tr.top), w: Math.round(tr.width), h: Math.round(tr.height), color: tcs.color, bg: tcs.backgroundColor, disp: tcs.display};
					}
				} else { out.fe = 'NO_FILE_EXPLORER'; }
			} catch(e) { out.feErr = String(e.message || e); }
			// xterm 内部 buffer 状态（区分「数据没进 buffer」vs「渲染没执行」）
			try {
				var lt = window.__lastTerm;
				if (lt && lt.buffer && lt.buffer.active) {
					out.bufLines = lt.buffer.active.length;
					var l0 = lt.buffer.active.getLine(0);
					out.line0 = l0 ? l0.translateToString().slice(0, 40) : 'null';
					out.cursorY = lt.buffer.active.cursorY;
					out.cursorHidden = lt._coreService ? lt._coreService.isCursorHidden : 'no-core';
				} else { out.bufLines = 'no-buffer'; }
			} catch(e) { out.bufErr = String(e.message || e); }
			// ★ 光标元素诊断（用户反馈「光标不闪烁」）：.xterm-cursor 的
			// 存在性/类名/animation 属性/背景色——闪烁依赖 xterm-cursor-blink
			// 类（isFocused）+ CSS animation 驱动。
			try {
				var cur = document.querySelector('.xterm .xterm-cursor');
				if (cur) {
					var cr = cur.getBoundingClientRect();
					var ccs = getComputedStyle(cur);
					out.cursor = {
						cls: (cur.className || '').toString(),
						x: Math.round(cr.left), y: Math.round(cr.top), w: Math.round(cr.width), h: Math.round(cr.height),
						disp: ccs.display, vis: ccs.visibility, anim: ccs.animationName, bg: ccs.backgroundColor,
						hasBlink: (cur.className || '').toString().indexOf('xterm-cursor-blink') >= 0,
						// ★ 光标宽度诊断：内联 style.width vs computed width——
						// 若 style.width=13px 但 rect 172px → 引擎布局没应用
						// 内联宽度（inline-block 宽度计算 bug）。
						styleW: cur.style ? cur.style.width : 'no-style',
						styleH: cur.style ? cur.style.height : 'no-style',
						computedW: ccs.width, computedH: ccs.height,
						txt: (cur.textContent || '').slice(0, 20)
					};
				} else { out.cursor = 'NO_CURSOR_EL'; }
				// xterm 容器 focus 类（isFocused 的 DOM 反映）
				var xt2 = document.querySelector('.xterm');
				if (xt2) { out.xtermFocusCls = (xt2.className || '').toString().slice(0, 60); }
			} catch(e) { out.cursorErr = String(e.message || e); }
			out.errs = (window.__errs || []).slice(-5);
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] RESULT %s", v.ToString())
		// ★ 稳定后再次 dump canvas（重置一次性标志 → 下帧 Paint 重新 dump）
		h.dumpPNGDone = false
		// ★ 测试 DOM 变更 → 渲染树重建链路：appendChild 一个固定节点，
		//   下帧 dump 看它是否进渲染树（若不在则重建未触发）。
		_, _ = interp.RunJS(`(function(){
			var t = document.getElementById('__tree_test');
			if (!t) {
				t = document.createElement('div');
				t.id = '__tree_test';
				t.style.cssText = 'position:fixed;left:5px;top:500px;width:10px;height:10px;background:red';
				document.body.appendChild(t);
			}
		})()`)
		// ★ PTY 输出后重新 dump 渲染树（DumpRTCallback 下帧触发，desktop_diag.log
		//   覆盖）——此时 xterm rows 已更新，看 rows 的 RenderText 是否在树里。
		h.needsResizeDump = true
		// ③ 键盘输入方向验证：向终端发送 `dir` 命令，PTY 执行后输出回显。
		//    （WB_TERM_QUERY 卡死排查：dir 输出渲染死循环时注释此段）
		if os.Getenv("WB_TERM_DIR") != "" {
			_, _ = interp.RunJS(`(function(){
				var ks = Object.keys(window.__desktopTerminals || {});
				if (ks.length > 0) {
					window.__desktopTerminals[ks[0]].send('dir\r');
				}
			})()`)
		}
	case 260:
		// ★ 先聚焦 textarea（JS focus() → FocusBridge → 引擎聚焦 + focus
		// 事件 → xterm isFocused=true → 光标渲染）——用户「点击终端」的
		// 等价操作。聚焦后查询光标元素状态（是否存在/闪烁类/动画）。
		_, _ = interp.RunJS(`(function(){
			var ta = document.querySelector('.xterm-helper-textarea');
			if (ta) ta.focus();
			// ★ 诊断：手动 fit 看 rows 是否变化（xterm 24 行超出容器）
			if (window.__lastTerm && window.__lastTerm._core) {
				try {
					var dims = window.__lastTerm._core._renderService.dimensions;
					window.__fitDiag = {
						cellW: dims && dims.css ? dims.css.cell.width : 'no-dims',
						cellH: dims && dims.css ? dims.css.cell.height : 'no-dims',
						rowsBefore: window.__lastTerm.rows
					};
					// 手动 fit
					var t = window.__lastTerm;
					if (t._fitAddon) {
						t._fitAddon.fit();
						window.__fitDiag.rowsAfter = t.rows;
						window.__fitDiag.fitResult = 'called';
					} else {
						window.__fitDiag.fitResult = 'no-fitaddon';
						// 直接 resize 验证容器行数计算（与 FitAddon 同公式：
						// innerHeight = parentH - xterm padding - border）
						var pe2 = t.element.parentElement;
						var ph = parseInt(getComputedStyle(pe2).height) || 0;
						var xs = getComputedStyle(t.element);
						var padT = parseInt(xs.getPropertyValue('padding-top')) || 0;
						var padB = parseInt(xs.getPropertyValue('padding-bottom')) || 0;
						var bdT = parseInt(xs.getPropertyValue('border-top-width')) || 0;
						var bdB = parseInt(xs.getPropertyValue('border-bottom-width')) || 0;
						var innerH = ph - padT - padB - bdT - bdB;
						var rows = Math.floor(innerH / dims.css.cell.height);
						window.__fitDiag.xtermPad = padT + '/' + padB;
						window.__fitDiag.innerH = innerH;
						window.__fitDiag.parentH = ph;
						window.__fitDiag.parentRectH = pe2.getBoundingClientRect().height;
						t.resize(dims.css.cell.width ? Math.floor(pe2.getBoundingClientRect().width / dims.css.cell.width) : 80, Math.max(1, rows));
						window.__fitDiag.rowsAfter = t.rows;
						window.__fitDiag.calcRows = rows;
					}
				} catch(e) { window.__fitDiag = 'err:' + e.message; }
			}
		})()`)
		v2, _ := interp.RunJS(`(function(){
			var lines = [];
			var divs = document.querySelectorAll('.xterm .xterm-rows > div');
			for (var i=0;i<Math.min(divs.length, 8);i++){
				lines.push(i + ':[' + (divs[i].textContent || '').slice(0, 100) + ']');
			}
			var cur = document.querySelector('.xterm .xterm-cursor');
			var out = {lines: lines.join(' | '), errs: (window.__errs || []).slice(-5)};
			if (cur) {
				var cr = cur.getBoundingClientRect();
				var ccs = getComputedStyle(cur);
				out.cursor = {
					cls: (cur.className || '').toString(),
					x: Math.round(cr.left), y: Math.round(cr.top), w: Math.round(cr.width), h: Math.round(cr.height),
					anim: ccs.animationName, anim2: ccs.getPropertyValue('animation-name'), animFull: ccs.getPropertyValue('animation'),
					bg: ccs.backgroundColor, bg2: ccs.getPropertyValue('background-color'),
					hasBlink: (cur.className || '').toString().indexOf('xterm-cursor-blink') >= 0,
					styleW: cur.style ? cur.style.width : 'no-style',
					computedW: ccs.width, txt: (cur.textContent || '').slice(0, 20),
					pos: ccs.position, disp: ccs.display, left: ccs.left, top: ccs.top,
					parentCls: cur.parentElement ? (cur.parentElement.className || '').toString().slice(0, 40) : '',
					parentRect: cur.parentElement && cur.parentElement.getBoundingClientRect ? (function(){ var r = cur.parentElement.getBoundingClientRect(); return {x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height)}; })() : null
				};
			} else { out.cursor = 'NO_CURSOR_EL'; }
			var xt = document.querySelector('.xterm');
			if (xt) out.xtermCls = (xt.className || '').toString();
			var ta2 = document.querySelector('.xterm-helper-textarea');
			if (ta2) {
				var tcs = getComputedStyle(ta2);
				out.taFocusStyle = {vis: tcs.visibility, op: tcs.opacity};
			}
			out.fitDiag = window.__fitDiag;
			// ★ 诊断 fit 容器高度（getComputedStyle(parentElement).height）
			if (window.__lastTerm && window.__lastTerm.element) {
				try {
					var pe = window.__lastTerm.element.parentElement;
					if (pe) {
						var pcs = getComputedStyle(pe);
						var sel = window.__lastTerm.element;
						var secs = getComputedStyle(sel);
						out.fitParent = {
							tag: pe.tagName, cls: (pe.className||'').toString().slice(0,40),
							h: pcs.height, w: pcs.width, rectH: (pe.getBoundingClientRect?pe.getBoundingClientRect().height:0)
						};
						out.fitSelf = {h: secs.height, rectH: sel.getBoundingClientRect?sel.getBoundingClientRect().height:0};
					}
				} catch(e) { out.fitParent = 'err:' + e.message; }
			}
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] DIR-RESULT %s", v2.ToString())
		// ★ 不置 termTestDone：继续 case 320（用户用真实键盘 Enter 后查询
		// buffer 行数变化，验证「回车执行」链路 keydown→xterm→PTY）。
		h.needsResizeDump = true
	case 320:
		// ★ 中文输入验证：用 xterm.send()（真实输入路径——用户打字/粘贴
		// 最终都走 send → triggerDataEvent → PTY）。中文以 UTF-8 字节进
		// PTY → cmd（chcp 65001）解码回显 → 验证「中文输入+中文回显」
		// 链路（GBK 时代会乱码）。\r 触发 cmd 执行「中文测试」→ 报错
		// 「不是内部或外部命令」回显中文。
		_, _ = interp.RunJS(`(function(){
			var ks = Object.keys(window.__desktopTerminals || {});
			if (ks.length === 0) { window.__cnInj = 'NO_TERMS'; return; }
			try {
				window.__desktopTerminals[ks[0]].send('中文测试\r');
				window.__cnInj = 'sent via xterm.send';
			} catch(e) {
				window.__cnInj = 'err:' + (e.message || e);
			}
		})()`)
		v5, _ := interp.RunJS(`(function(){
			var out = {inj: window.__cnInj};
			var lt = window.__lastTerm;
			if (lt && lt.buffer && lt.buffer.active) {
				out.bufLines = lt.buffer.active.length;
				out.cursorY = lt.buffer.active.cursorY;
				var l = lt.buffer.active.getLine(lt.buffer.active.length - 1);
				out.lastLine = l ? l.translateToString(true).slice(0, 60) : '';
			}
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] CN-INJECT %s", v5.ToString())
	case 400:
		// ★ JS 派发 keydown Enter 到 textarea（绕过引擎 EventKey，直接走
		// DOM 事件系统）：若 xterm 处理（写 \r → PTY 执行）→ 说明引擎
		// EventKey 派发的事件对象有属性问题；若也不处理 → xterm listener
		// 未绑定 textarea 或 PTY 链路断。
		_, _ = interp.RunJS(`(function(){
			var ta = document.querySelector('.xterm-helper-textarea');
			if (!ta) { window.__jsEnter = 'NO_TEXTAREA'; return; }
			var ev = new KeyboardEvent('keydown', {
				key: 'Enter', code: 'Enter', keyCode: 13, which: 13,
				bubbles: true, cancelable: true
			});
			var prevented = !ta.dispatchEvent(ev);
			window.__jsEnter = 'prevented=' + prevented + ' canceled=' + ev.defaultPrevented;
		})()`)
		v4, _ := interp.RunJS(`(function(){
			var out = {jsEnter: window.__jsEnter, taVal: '', evtSys: ''};
			var ta = document.querySelector('.xterm-helper-textarea');
			if (ta) out.taVal = (ta.value || '').slice(0, 40);
			// ★ 引擎事件系统自检：临时注册 keydown listener + dispatch，
			// 验证 addEventListener('keydown') 在引擎里是否工作。
			try {
				var hits = 0;
				var tmp = function(ev){ hits++; window.__tmpKey = ev.key + '/' + ev.keyCode; };
				ta.addEventListener('keydown', tmp);
				ta.dispatchEvent(new KeyboardEvent('keydown', {key:'X', keyCode:88, bubbles:true, cancelable:true}));
				out.evtSys = 'hits=' + hits + ' last=' + (window.__tmpKey || 'none') + ' sameEl=' + (document.querySelector('.xterm-helper-textarea') === ta);
				ta.removeEventListener('keydown', tmp);
			} catch(e) { out.evtSys = 'err:' + (e.message || e); }
			try {
				var lt = window.__lastTerm;
				if (lt && lt.buffer && lt.buffer.active) {
					out.bufLines = lt.buffer.active.length;
					out.cursorY = lt.buffer.active.cursorY;
				}
			} catch(e) { out.err = String(e.message || e); }
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] JS-ENTER %s", v4.ToString())
	case 460:
		// ★ 回车执行验证：case 260 聚焦 + fit 后，外部脚本（Win32
		// WM_KEYDOWN Enter）已发送回车（约 200 帧窗口）。查询 buffer
		// 行数与最新行——PTY 执行空命令应输出新 prompt（行数 +1 或
		// 出现新行）。
		v3, _ := interp.RunJS(`(function(){
			var out = {lines: [], total: -1};
			try {
				var lt = window.__lastTerm;
				if (lt && lt.buffer && lt.buffer.active) {
					out.total = lt.buffer.active.length;
					for (var i = Math.max(0, lt.buffer.active.length - 4); i < lt.buffer.active.length; i++) {
						var l = lt.buffer.active.getLine(i);
						out.lines.push(i + ':[' + (l ? l.translateToString().slice(0, 80) : 'null') + ']');
					}
					out.cursorY = lt.buffer.active.cursorY;
				} else { out.err = 'no-buffer'; }
			} catch(e) { out.err = String(e.message || e); }
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] ENTER-RESULT %s", v3.ToString())
		// 不置 termTestDone：继续 case 500（__devtools 布局诊断）。
	case 500:
		// ★ __devtools 布局诊断：查 main-area / right-container /
		// bottom-panel / 终端容器 的几何与父级链——定位「终端矩形背景」
		// （main-area 被右侧面板挤压 / 终端容器尺寸异常）。
		v7, _ := interp.RunJS(`(function(){
			var out = {};
			if (!window.__devtools) return JSON.stringify({err: 'no __devtools'});
			var sels = ['.app-root', '.main-area', '.right-container', '.bottom-panel', '.panel-content', '.terminal-panel', '.term-content', '.term-tabs', '.term-tab', '.term-xterm-wrap', '.xterm', '.xterm-scrollable-element', '.xterm-viewport', '.xterm-screen', '.xterm-rows', '.status-bar'];
			for (var i=0;i<sels.length;i++){
				try { out[sels[i]] = JSON.parse(window.__devtools.sel(sels[i])); } catch(e){ out[sels[i]] = 'err:' + (e.message||e); }
			}
			out.page = JSON.parse(window.__devtools.page());
			return JSON.stringify(out);
		})()`)
		log.Printf("[termtest] DEVTOOLS %s", v7.ToString())
		h.termTestDone = true
	}
}

// dumpDevtoolsDiag 真实模式下输出 __devtools 布局诊断（WB_DT_DUMP=1 帧 300
// 调用）：查终端/状态栏几何 + 光标位置 + xterm 行数，验证真实 fit 结果。
func (h *Host) dumpDevtoolsDiag() {
	interp := h.wv.JSInterpreter()
	if interp == nil {
		log.Printf("[dtdump] no interpreter")
		return
	}
	v, _ := interp.RunJS(`(function(){
		var out = {};
		if (!window.__devtools) return JSON.stringify({err: 'no __devtools'});
		var sels = ['.xterm', '.xterm-viewport', '.xterm-scrollable-element', '.xterm-screen', '.xterm-rows', '.status-bar', '.terminal-panel'];
		for (var i=0;i<sels.length;i++){
			try { var o = JSON.parse(window.__devtools.sel(sels[i])); out[sels[i]] = {x:o.x, y:o.y, w:o.w, h:o.h, bg:o.bg, vis:o.vis, disp:o.disp, pos:o.pos}; } catch(e){ out[sels[i]] = 'err:' + (e.message||e); }
		}
		// xterm 行数 + 光标
		var rows = document.querySelectorAll('.xterm-rows > div').length;
		out.xtermRows = rows;
		// ★ xterm 内部元素 computed style：渲染树缺失 viewport/scrollable
		// （painter 未绘制黑色背景）——查 display/visibility/position 是否
		// 导致渲染树构建跳过。
		['.xterm', '.xterm-viewport', '.xterm-scrollable-element', '.xterm-screen', '.xterm-rows', '.xterm-helper-textarea'].forEach(function(sel){
			var e = document.querySelector(sel);
			if (!e) { out[sel + '-cs'] = 'MISSING'; return; }
			var s = getComputedStyle(e);
			out[sel + '-cs'] = {disp: s.display, vis: s.visibility, pos: s.position, w: s.width, h: s.height, op: s.opacity};
		});
		var cur = document.querySelector('.xterm-cursor');
		if (cur) { var cr = cur.getBoundingClientRect(); out.cursor = {x: Math.round(cr.left), y: Math.round(cr.top), w: Math.round(cr.width), h: Math.round(cr.height), cls: (cur.className||'').toString()}; } else { out.cursor = 'NO_CURSOR'; }
		var t = window.__lastTerm;
		if (t) { out.termRows = t.rows; out.termCols = t.cols; }
		return JSON.stringify(out);
	})()`)
	log.Printf("[dtdump] %s", v.ToString())
	// ★ 触发渲染树 re-dump（DumpRTCallback → desktop_diag_*.log）：
	// 验证 viewport 是否在渲染树（painter 遍历依赖渲染树结构）。
	h.needsResizeDump = true
}

// ensureTreeChangeHook 每帧幂等注册 DOM 结构变更回调：DOM 变更
// （appendChild/removeChild/setTextContent 等）→ MarkRenderTreeDirty →
// 下帧重建渲染树。否则动态 DOM 更新（xterm 每字符 appendChild span 到
// rows）不触发重建，内容永远不显示。LoadHTML 重建 Document 时指针变化，
// 自动重新注册。
func (h *Host) ensureTreeChangeHook() {
	if h.wv == nil || h.wv.MainFrame() == nil {
		return
	}
	doc := h.wv.MainFrame().Document()
	if doc == nil || doc == h.treeHookDoc {
		return
	}
	h.treeHookDoc = doc
	if os.Getenv("WB_TERM_DEBUG") != "" {
		log.Printf("[treehook] registered doc=%p", doc)
	}
	doc.SetTreeChangeCallback(func() {
		if os.Getenv("WB_TERM_DEBUG") != "" {
			log.Printf("[treehook] tree change!")
		}
		if mf := h.wv.MainFrame(); mf != nil {
			if fr := mf.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
			}
		}
	})
}

// autodragTick 自动模拟 textarea resize 拖拽（WB_AUTODRAG=1 时每帧调用）：// 自动打开设置面板「指令」tab → 找第一个 textarea 的右下角手柄 → 模拟
// 按下 → 逐帧 +5px 拖 60 帧 → 释放。帧耗时由 Run 循环的 [ft] 日志记录
// （autodragSamples 汇总）。目的：在真实窗口（GPU paint + vsync）下量化
// 拖拽帧时间，验证「高度跟手」。
// autodragStep 状态机（负值=等待帧计数，0+ = 拖拽进度）：
//   -1        首次：点击活动栏底部按钮打开设置面板 → 设 -30
//   -30..-1   等待面板渲染（逐帧递增）
//   0         点击「指令」tab → 设 -60
//   -60..-1   等待 tab 渲染（逐帧递增）
//   0+        找 textarea；找到则开始拖拽（每帧 +5px，60 帧后释放）
func (h *Host) autodragTick() {
	if h.autodragEl == nil {
		interp := h.wv.JSInterpreter()
		// 首次（step=-999 哨兵，与等待期 -100..-1 不冲突）：打开设置面板
		if h.autodragStep == -999 {
			h.autodragStep = -100
			log.Printf("[autodrag] 打开设置面板")
			if interp != nil {
				_, _ = interp.RunJS(`(function(){
					var btn = document.querySelector('.activity-bottom button');
					if (btn) { var ev = new Event('click', {bubbles:true}); btn.dispatchEvent(ev); }
				})()`)
			}
			return
		}
		// 等待期（-100..-1 或 -200..-1 递增）：到 0 时按阶段注入下一步
		if h.autodragStep < 0 {
			h.autodragStep++
			if h.autodragStep == 0 && !h.autodragPanelOpen {
				// 面板等待期结束：切到指令 tab，进入 -200 第二阶段
				h.autodragPanelOpen = true
				h.autodragStep = -200
				log.Printf("[autodrag] 切到指令 tab")
				if interp != nil {
					_, _ = interp.RunJS(`(function(){
						var btns = document.querySelectorAll('.settings-tabs button');
						for (var i=0;i<btns.length;i++){
							if (btns[i].textContent.indexOf('指令') >= 0) {
								var ev = new Event('click', {bubbles:true}); btns[i].dispatchEvent(ev); break;
							}
						}
					})()`)
				}
			}
			return
		}
		// 找第一个 textarea
		rv := h.wv.RenderView()
		if rv == nil {
			return
		}
		var el *dom.Element
		var fallback *dom.Element
		var walk func(ro rendering.RenderObject)
		walk = func(ro rendering.RenderObject) {
			if ro == nil {
				return
			}
			if n := ro.Node(); n != nil {
				if e, ok := n.(*dom.Element); ok && e.LocalName() == "textarea" {
					if strings.Contains(e.GetAttribute("class"), "inst-textarea") {
						if el == nil {
							el = e
						}
					} else if fallback == nil {
						fallback = e
					}
				}
			}
			for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
			}
		}
		walk(rendering.RenderObject(rv))
		if el == nil {
			el = fallback
		}
		if el == nil {
			return // 面板未就绪，继续等
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			return
		}
		bx, by, bw, bh := rendering.BoxViewportRect(rv, box)
		h.autodragX = bx + bw - 5
		h.autodragY = by + bh - 5
		h.MockTextareaResizePress(el, rv, h.autodragY)
		h.autodragEl = el
		h.autodragStart = time.Now()
		log.Printf("[autodrag] textarea (%.0f,%.0f %.0fx%.0f) handle=(%.0f,%.0f) start",
			bx, by, bw, bh, h.autodragX, h.autodragY)
		return
	}
	h.autodragStep++
	// 双向拖拽：0..29 向下 +5px/帧；30..59 向上 -5px/帧（回到起点附近）。
	// 每帧采样 textarea boxH，验证「向下跟手、向上缩跟手」。
	dy := 5.0 * float64(h.autodragStep+1)
	if h.autodragStep >= 30 {
		dy = 5.0 * float64(60-h.autodragStep)
	}
	h.MockEventCursorMove(h.wv, h.autodragX, h.autodragY+dy)
	if h.autodragStep == 0 || h.autodragStep == 30 || (h.autodragStep%10 == 0) {
		if rv2 := h.wv.RenderView(); rv2 != nil {
			if bx := rv2.FindRenderBoxForNode(h.autodragEl); bx != nil {
				log.Printf("[drag] step=%d dy=%+.0f boxH=%.1f style=%q", h.autodragStep, dy, bx.Height(), h.autodragEl.GetAttribute("style"))
			}
		}
	}
	if h.autodragStep >= 60 {
		h.MockTextareaResizeRelease()
		el := h.autodragEl
		h.autodragEl = nil
		n := len(h.autodragSamples)
		if n > 0 {
			sum, maxV := 0.0, 0.0
			for _, s := range h.autodragSamples {
				sum += s
				if s > maxV {
					maxV = s
				}
			}
			log.Printf("[autodrag] DONE steps=%d avgFrame=%.2fms maxFrame=%.2fms finalH=%s",
				n, sum/float64(n), maxV, el.GetAttribute("style"))
		}
		h.autodragOn = false
	}
}

// handleCharInput 处理单个字符输入（window.EventChar 分支核心逻辑）。
// 抽成独立方法：真实键盘（processEvents）与测试（MockKeyChar）共享同一
// 实现，避免「probe 验证通过、真实使用失败」的路径分叉。
func (h *Host) handleCharInput(ev window.Event) {
	if h.imeFocusedEl == nil {
		return
	}
	char := string(ev.Char)
	if isTextFormControl(h.imeFocusedEl) {
		// Get current value and insert character at cursor position,
		// replacing any active selection (like a browser).
		val := focusedElementValue(h.imeFocusedEl)
		runes := []rune(val)
		sel := rendering.FocusedFormControlSel
		// Caret default = END of text when no click positioned it
		// (browsers focus with the caret at the end; inserting at 0
		// put every char at the HEAD). A click-positioned caret is
		// honored regardless of Active — Release sets Active=false
		// when the drag ends, but the caret must stay where the
		// click placed it.
		start, end := len(runes), len(runes)
		if sel != nil {
			start, end = sel.Start, sel.End
		}
		if start > end {
			start, end = end, start
		}
		if runes == nil {
			runes = []rune{}
		}
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = 0
		}
		if start > len(runes) {
			start = len(runes)
		}
		if end > len(runes) {
			end = len(runes)
		}
		newRunes := make([]rune, 0, len(runes)+1)
		newRunes = append(newRunes, runes[:start]...)
		newRunes = append(newRunes, []rune(char)...)
		newRunes = append(newRunes, runes[end:]...)
		newVal := string(newRunes)
		setFocusedElementValue(h.imeFocusedEl, newVal)
		if os.Getenv("WB_IME_DEBUG") != "" {
			log.Printf("[ime] evchar char=%q start=%d end=%d → %q", char, start, end, newVal)
		}
		// Cursor lands right after the inserted character.
		newPos := start + len([]rune(char))
		if sel == nil {
			rendering.FocusedFormControlSel = &rendering.FormControlSelection{
				Start: newPos, End: newPos, Active: true,
			}
		} else {
			sel.Start = newPos
			sel.End = newPos
		}
		if mf := h.wv.MainFrame(); mf != nil {
			if fr := mf.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
			}
		}
		h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
		h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
	} else if strings.EqualFold(h.imeFocusedEl.GetAttribute("contenteditable"), "true") {
		// contenteditable（CodeMirror 6 输入区）：光标处插入文本节点，
		// 派发 input → CM6 的 DOMObserver readDOMChange 同步 state。
		// 不能用 value/textContent 全文替换（会抹掉结构化 DOM）。
		ok := bindings.InsertTextAtSelection(char)
		if mf := h.wv.MainFrame(); mf != nil {
			if fr := mf.Frame(); fr != nil {
				fr.MarkRenderTreeDirty()
			}
		}
		if !ok {
			// 无有效 DOM Selection：仅派发 input，让 CM6 的 input
			// handler 有机会走 state 更新路径（回退语义）。
		}
		h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
	}
}

func (h *Host) processEvents(rv *rendering.RenderView) {
	// WB_AUTODRAG=1：自动拖拽验证；WB_TERM_TEST=1：终端自动化验证。
	if h.autodragOn {
		h.autodragTick()
	}
	if (os.Getenv("WB_TERM_TEST") != "" || os.Getenv("WB_TERM_QUERY") != "") && !h.termTestDone {
		h.termTestTick()
	}
	// WB_DT_DUMP=1：真实模式 DEVTOOLS dump（帧 300 一次性）。
	if os.Getenv("WB_DT_DUMP") != "" && !h.dtDumped {
		h.dtFrame++
		if h.dtFrame == 300 {
			h.dtDumped = true
			h.dumpDevtoolsDiag()
		}
	}
	// Consume buffered IME events (composition updates, committed chars,
	// composition end) and apply them to the focused form control. The
	// Win32 IME handler buffers these in its subclassed WndProc; without
	// this per-frame poll, composition text / confirmed characters never
	// reach the element value ("can't type anything").
	if imeEvs := h.win.PollIMEEvents(); len(imeEvs) > 0 {
		h.applyIMEEvents(imeEvs)
	}
	for _, ev := range h.win.PollEvents() {
		switch ev.Type {
		case window.EventResize:
			h.wv.Resize(h.win.Width(), h.win.Height())
			h.needsResizeDump = true
			log.Printf("[resize-ev] fb=%dx%d css=%dx%d scale=(%.2f,%.2f) viewport=%dx%d",
				h.win.FramebufferWidth(), h.win.FramebufferHeight(),
				h.win.Width(), h.win.Height(),
				func() float64 { a, _ := h.win.ContentScale(); return a }(), 0.0,
				func() int {
					if fv := h.wv.Page().MainFrame().View(); fv != nil {
						return fv.Width()
					}
					return -1
				}(), 0)
			// Also dump on the first frame after resize: compare scroll and content sizes
			if rv != nil && rv.LayoutState() != nil {
				lb := rv.LayoutBox()
				if lb != nil {
					log.Printf("[resize-diag] viewport=%dx%d layoutRoot=(%.0f,%.0f %.0fx%.0f)",
						h.win.Width(), h.win.Height(),
						rv.LayoutState().GeometryForBox(lb).ContentBoxLeft(),
						rv.LayoutState().GeometryForBox(lb).ContentBoxTop(),
						rv.LayoutState().GeometryForBox(lb).ContentWidth(),
						rv.LayoutState().GeometryForBox(lb).ContentHeight())
				}
			}
		case window.EventScroll:
			if rv == nil {
				break
			}
			// GLFW: ScrollY > 0 when scrolling up (away from user), ScrollX > 0 when scrolling right.
			// Browser: scroll up → see content above → scrollY decreases.
			//          scroll right → see content to the right → scrollX increases.
			// Try per-box scroll first: hit-test under cursor for overflow:scroll/auto.
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := h.cursorX / csX
			cssY := h.cursorY/csY + float64(h.wv.Page().MainFrame().View().ScrollY())
			// ★ ScrollTargetAt 解析滚动容器 + 其所属 RenderView：iframe 内
			// 滚动时 box 属于子 Frame，偏移表在子 RenderView（偏移读写与
			// scrollbar metrics 必须用子 rv——主 rv 查不到子 box 的偏移，
			// 写入也不生效，因为子文档绘制读自己的偏移表）。
			tgt := rv.ScrollTargetAt(cssX, cssY)
			// ★ 浏览器标准：wheel 事件先派发到 DOM（冒泡），JS 监听器
			// （如 xterm 6 的 ScrollableElement——终端的滚动完全由 JS 侧
			// Widget 处理，引擎的滚动容器模型不适用）可通过
			// preventDefault/stopPropagation 消费；只有未被消费时才由引擎
			// 自身的滚动容器逻辑处理。此前引擎直接滚动容器、从不派发
			// DOM wheel 事件 → xterm 收不到 wheel → 终端无法滚动。
			if tgt.Box != nil {
				if el, ok := tgt.Box.Node().(*dom.Element); ok {
					// GLFW ScrollY>0 = 向上滚 → 浏览器 deltaY 为负（内容向
					// 下移）；每格约 100px（Chrome 鼠标滚轮默认量级）。
					deltaY := -ev.ScrollY * 100
					deltaXv := -ev.ScrollX * 100
					we := dom.NewWheelEventFromInit("wheel", dom.WheelEventInit{
						MouseEventInit: dom.MouseEventInit{EventInit: dom.EventInit{Bubbles: true, Cancelable: true}},
						DeltaX:         deltaXv,
						DeltaY:         deltaY,
						DeltaMode:      dom.DOMDeltaPixel,
					})
					consumed := !el.DispatchEvent(we)
					if os.Getenv("WB_SCROLL_DEBUG") != "" {
						log.Printf("[scroll] dom wheel dispatched to %s.%s deltaY=%.0f consumed=%v defaultPrevented=%v",
							el.LocalName(), el.GetAttribute("class"), deltaY, consumed, we.DefaultPrevented())
					}
					if consumed {
						if os.Getenv("WB_SCROLL_DEBUG") != "" {
							log.Printf("[scroll] wheel consumed by JS (xterm), skip engine scroll")
						}
						break
					}
				}
			}
			if tgt.Box != nil {
				scrollBox := tgt.Box
				srv := tgt.RV
				// ★ 链式滚动（浏览器语义）：滚轮命中的滚动容器在该方向
				// 不可滚（无滚动范围，如内容未溢出的 textarea）时，冒泡到
				// 父级最近的滚动容器（如外层 settings-body）继续滚动——
				// 否则「在编辑框区域滚轮窗口不动」（textarea 吃掉事件后
				// 自身无滚动，外层也收不到）。
				if !scrollableVertically(srv, scrollBox) && !scrollableHorizontally(srv, scrollBox) {
					if chained := chainScrollTargetUp(srv, scrollBox); chained != nil && chained != scrollBox {
						if os.Getenv("WB_SCROLL_DEBUG") != "" {
							bn := "?"
							if n := scrollBox.Node(); n != nil {
								if el, ok := n.(*dom.Element); ok {
									bn = el.LocalName() + "." + el.GetAttribute("class")
								}
							}
							log.Printf("[scroll] %s not scrollable → chain to parent scroll container", bn)
						}
						scrollBox = chained
					}
				}
				if os.Getenv("WB_SCROLL_DEBUG") != "" {
					boxName := "?"
					if n := scrollBox.Node(); n != nil {
						if el, ok := n.(*dom.Element); ok {
							boxName = el.LocalName() + "." + el.GetAttribute("class")
						}
					}
					log.Printf("[scroll] hit-box=%s at (%.0f,%.0f)\n", boxName, cssX, cssY)
				}
				log.Printf("[scroll] per-box hit at (%.0f,%.0f) scrollY=%d\n",
					cssX, cssY, h.wv.Page().MainFrame().View().ScrollY())
				if os.Getenv("WB_SCROLL_DEBUG") != "" && h.wv.JSInterpreter() != nil {
					if v, err := h.wv.JSInterpreter().RunJS(`(function(){
						try {
							var msgs = document.querySelectorAll('.msg-item, .message, .chat-msg, .msg-row, .tl-row');
							var convs = document.querySelectorAll('.conv-item');
							return 'domMsgs=' + msgs.length + ' domConvs=' + convs.length;
						} catch(e){ return 'err:'+e.message; }
					})()`); err == nil {
						fmt.Fprintf(os.Stderr, "[scroll] %s\n", v.ToString())
					}
				}
				sx, sy := srv.BoxScrollOffset(scrollBox)
				deltaX := int(ev.ScrollX * 40)  // positive = right → sx increases
				deltaY := -int(ev.ScrollY * 40) // positive = up → sy decreases
				newSx := int(sx) + deltaX
				newSy := int(sy) + deltaY
				// Clamp to valid range.
				if newSy < 0 {
					newSy = 0
				}
				if newSx < 0 {
					newSx = 0
				}
				// Clamp to the shared scrollbar geometry (content-box
				// viewport, not the padding box) so wheel scrolling reaches
				// the same max as the thumb — the painter's max is
				// totalH - viewH with viewH = padding-box minus padding.
				if vm := rendering.VerticalScrollbarMetrics(srv, scrollBox); vm.OK {
					maxY := int(vm.MaxScroll)
					if newSy > maxY {
						newSy = maxY
					}
				} else if newSy > 0 {
					newSy = 0
				}
				if hm := rendering.HorizontalScrollbarMetrics(srv, scrollBox); hm.OK {
					maxX := int(hm.MaxScroll)
					if newSx > maxX {
						newSx = maxX
					}
				} else if newSx > 0 {
					newSx = 0
				}
				// Wheel scroll is SMOOTHED (browser-like): record the target
				// and let the main loop interpolate toward it every frame.
				// A new wheel event while animating simply re-targets from
				// the current interpolated position.
				if !h.smoothActive || h.smoothBox != scrollBox {
					h.smoothBox = scrollBox
					h.smoothRV = srv
					h.smoothCurX, h.smoothCurY = sx, sy
				}
				h.smoothTarX = float64(newSx)
				h.smoothTarY = float64(newSy)
				h.smoothActive = true
				h.smoothLast = time.Now()
			} else {
				log.Printf("[scroll] FrameView.ScrollBy(dx=%d, dy=%d) scrollY=%d maxY=%d contentH=%d viewportH=%d\n",
					-int(ev.ScrollX*40), -int(ev.ScrollY*40),
					h.frameView.ScrollY(), h.frameView.MaxScrollY(),
					h.frameView.ContentHeight(), h.frameView.Height())
				h.frameView.ScrollBy(-int(ev.ScrollX*40), -int(ev.ScrollY*40))
			}

		case window.EventCursorLeave:
			// ★ 鼠标移出窗口：清除 hover 状态与光标残留。
			// 之前 SetHovered(true) 留在最后一个 hover 的 DOM 元素上，
			// 且从不清除——agent 输出/任何渲染树重建时 resolver 读
			// el.IsHovered() 把 :hover 样式错误应用（"没有操作和悬停
			// 时 agent 输出也影响渲染"：无交互画面却出现 hover 高亮/
			// 按钮变色）。离开窗口即视为不再悬停任何元素。
			if h.hoveredEl != nil {
				h.hoveredEl.SetHovered(false)
				h.hoveredEl = nil
			}
			// 光标移出窗口：恢复默认箭头。
			if h.lastCursor != window.CursorArrow {
				h.lastCursor = window.CursorArrow
				if h.win != nil {
					h.win.SetCursorShape(window.CursorArrow)
				}
			}
			// 光标移到视口外：滚动条 hover 高亮（isHover 用 cursor
			// 位置判定）与子 Frame 光标一并清除。
			if rv != nil {
				rendering.SetCursorPosRecursive(rv, -1e9, -1e9)
				rv.MarkAllDirty()
			}
			// 拖拽选区若在鼠标移出窗口后仍进行（按住拖出窗口），
			// 保留 selEnd 不再更新（浏览器行为：拖出窗口后选区停在
			// 最后位置，直到鼠标回来）。
			h.cursorX, h.cursorY = ev.X, ev.Y

		case window.EventCursorMove:
			h.cursorX, h.cursorY = ev.X, ev.Y
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := ev.X / csX
			cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY()) // page coords for scrollbar hover

			// Update RenderView cursor for scrollbar hover highlight.
			// ★ 递归传播到 iframe 子 Frame：光标落在 iframe 内容框内时
			// 换算成子文档坐标设置子 rv 的光标（子文档滚动条 hover 高亮
			// 读子 rv 的 cursor；之前只设主 rv，子 Frame 滚动条永不高亮）。
			if rv != nil {
				rendering.SetCursorPosRecursive(rv, cssX, cssY)
				// ★ 窗口光标（CSS cursor 语义）：悬停 textarea 右下角手柄 →
				// nwse-resize；input/textarea 内容 → 文本光标；按钮/链接 →
				// 手型；默认箭头。此前引擎从不设置光标（用户反馈「鼠标
				// 光标都不会变化」）。
				h.updateCursor(rv, cssX, cssY)
				// ★ 鼠标移动必须标记重绘：滚动条 thumb 的 hover 高亮由
				// cursor 位置决定（renderpipeline.go isHover 判定），而 hover
				// 元素可能未变（移入/移出滚动条轨道不改 DOM hover）。按需
				// 渲染下若不标记 dirty，鼠标在滚动条上滑动时画面不更新。
				// 开销：鼠标移动期间全量重绘（与按需渲染前的行为一致），
				// 鼠标静止时零重绘。
				rv.MarkAllDirty()
			}

			// ── Range thumb drag ──
			// While a range drag is active (mouse held after pressing the
			// slider), map the cursor x onto the track every move and
			// dispatch input. A slider drag starts immediately (no 3px
			// hysteresis — pressing the track jumps the thumb to the cursor,
			// then it follows 1:1, mirroring the browser).
			if h.rangeDragEl != nil {
				// ★ 必须用当前帧最新 rv：拖动中渲染树会因 Vue input
				// 每帧重建（新 RenderView 实例），press 时缓存的
				// rangeDragRV 是旧实例——其 box 几何（AbsoluteX/Width）
				// 与新渲染树的鼠标坐标错配 → value 乱跳、thumb 抖动
				// （「拖拽不跟手」真凶）。按 DOM 节点重新解析。
				drv := h.resolveDragRV(h.rangeDragEl, h.rangeDragRV)
				h.rangeDragRV = drv
				if h.setRangeValueFromX(h.rangeDragEl, drv, cssX) {
					h.rangeDragEl.DispatchEvent(dom.NewEvent("input", true, false, false))
					// ★ 同 resize：value 变更不改变渲染树结构，跳过全量
					// rebuild 增量更新该元素样式（thumb 位置由 paint 从
					// value 重算，无需重建树）。
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							if !fr.RebuildStyleForElement(h.rangeDragEl) {
								fr.MarkRenderTreeDirty()
							}
						}
					}
				}
			}

			// ── Textarea CSS resize drag ──
			// While a resize drag is active (pressed the bottom-right
			// handle), track the cursor Y: new height = start height + dy,
			// clamped by min-height/max-height (browser resize semantics).
			// The result is written back to style="height:Npx" so the next
			// relayout keeps it (the rows intrinsic height would otherwise
			// reset it every layout).
			if h.resizeDragEl != nil {
				// ★ 同 range 拖拽：渲染树重建后旧 resizeDragRV 过期，
				// 必须按 DOM 节点解析当前实例（box 几何错配会让
				// 高度计算跳变）。
				drv := h.resolveDragRV(h.resizeDragEl, h.resizeDragRV)
				h.resizeDragRV = drv
				rb := drv.FindRenderBoxForNode(h.resizeDragEl)
				if rb == nil {
					h.resizeDragEl = nil
				} else {
					newH := h.resizeDragStartH + (cssY - h.resizeDragStartY)
					// ★ 与浏览器一致：style 写入 raw 高度，min/max-height 由
					// CSS 在布局层 clamp（浏览器拖动同样写入 raw 值，
					// getComputedStyle 才显示 clamp 后的高度）。
					// 仅保留 10px 下限防止拖没（浏览器无此下限，但 textarea
					// 拖到 0 无实际意义；min-height 未设置时兜底）。
					if newH < 10 {
						newH = 10
					}
					h.resizeDragEl.SetAttribute("style", fmt.Sprintf("height:%.0fpx", newH))
					// ★ 跳过全量 RebuildRenderTree：style height 变更只影响
					// 几何，增量重算该元素 ComputedStyle 并同步到渲染树 +
					// 布局树即可（全树重建在复杂页面 30ms+，是「拖拽不跟手」
					// 的主因）。结构属性（display/position/float）变化时
					// RebuildStyleForElement 返回 false，自动回退全量重建。
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							if !fr.RebuildStyleForElement(h.resizeDragEl) {
								fr.MarkRenderTreeDirty()
							}
						}
					}
				}
			}

			// ── Drag-to-select inside a focused text form control ──
			// While the mouse button is held (selecting) and the drag
			// threshold is met, extend the selection End to the cursor.
			if h.selecting && !h.scrollbarDragging && h.imeFocusedEl != nil &&
				isTextFormControl(h.imeFocusedEl) && rendering.FocusedFormControlSel != nil {
				if !h.hysteresisMet {
					dx := cssX - h.mouseDownX
					dy := cssY - h.mouseDownY
					if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
						// Not yet dragging.
					} else {
						h.hysteresisMet = true
					}
				}
				if h.hysteresisMet {
					offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
					rendering.FocusedFormControlSel.End = offset
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
			}

			// ── Drag-to-select plain text (non form control) ──
			// 鼠标按住（selecting）且拖过阈值时，实时扩展选区 End 到光标
			// 位置（Active=true → 高亮跟随拖动）。anchor 在 Press 时由
			// HitTestText 定位；跨 Frame：HitTestText 对 iframe 下钻，选区
			// 终点可落在子文档文本上（SelectionRects 按全局树序输出两
			// Frame 各自的高亮段）。
			if h.selecting && !h.scrollbarDragging && rv != nil &&
				h.imeFocusedEl == nil {
				if !h.hysteresisMet {
					dx := cssX - h.mouseDownX
					dy := cssY - h.mouseDownY
					if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
						// Not yet dragging.
					} else {
						h.hysteresisMet = true
					}
				}
				if h.hysteresisMet {
					h.selEndX = cssX
					h.selEndY = cssY
					h.handleDragSelection(rv)
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
			}

			// ── Hover tracking (normal cursor move, outside scrollbar drag) ──
			if rv != nil && !h.scrollbarDragging {
				newEl := rendering.HitTest(rv, cssX, cssY, "")
				elName := "<nil>"
				if newEl != nil {
					elName = newEl.LocalName()
					if cn := newEl.ClassName(); cn != "" {
						elName += "." + cn
					}
				}
				if debugPaintLog {
					if newEl != h.hoveredEl {
						oldName := "<nil>"
						if h.hoveredEl != nil {
							oldName = h.hoveredEl.LocalName()
						}
						log.Printf("[dbg/hover] css=(%.0f,%.0f) %s → %s", cssX, cssY, oldName, elName)
					}
				}
				if newEl != h.hoveredEl {
					oldHover := h.hoveredEl
					if oldHover != nil {
						oldHover.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					// ★ hover 快速路径：只重算新旧 hover 元素的样式，不重建
					// 整个渲染树（内容多时全树 rebuild+layout 需数秒）。
					// 仅当样式变化影响几何时才回退全树布局。
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							h.hoverStyleFastPath(rv, fr, oldHover, newEl)
						}
					}
				}
				// ★ 派发 JS mousemove（浏览器标准）：JS 层拖拽
				// （document.addEventListener('mousemove')，如侧栏分隔条）
				// 依赖它。此前 EventCursorMove 只做引擎内部 hover/滚动条
				// 处理，从不派发 mousemove DOM 事件 → JS 拖拽失效。
				// 坐标用视口 CSS 坐标（clientX/clientY 语义，与 mousedown/
				// mouseup 同基准）。newEl 为当前指针下元素，Bubbles 冒泡
				// 到 document。
				if newEl != nil {
					newEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseMove, dom.MouseEventInit{
						EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
						ClientX:   ev.X / csX,
						ClientY:   ev.Y / csY,
						Button:    dom.MouseButtonNone,
						Buttons:   0,
						Detail:    0,
					}))
				}
			}

			// Handle scrollbar thumb drag.
			if h.scrollbarDragging && rv != nil && h.scrollbarDragBox != nil {
				// ★ 拖拽的滚动容器可能属于 iframe 子 Frame——偏移表在子
				// RenderView，全部读写用 dragRV（按下时记录）。
				drv := h.scrollbarDragRV
				if drv == nil {
					drv = rv // 兼容旧路径：未记录 RV 时回退主视图
				}
				// ★ 拖动中渲染树可能已重建（Vue DOM 变更 → 新 RenderView
				// 实例）：主文档 box 重新解析到当前实例，避免用过期的
				// 旧实例几何（滚动容器尺寸变化导致 thumb 比例错乱）。
				if n := h.scrollbarDragBox.Node(); n != nil {
					if el, ok := n.(*dom.Element); ok {
						if cur := h.resolveDragRV(el, drv); cur != nil {
							drv = cur
						}
					}
				}
				h.scrollbarDragRV = drv
				csX, csY := h.win.ContentScale()
				if csX <= 0 {
					csX = 1
				}
				if csY <= 0 {
					csY = 1
				}
				cssX := ev.X / csX
				cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY()) // match EventMouseButton coordinate space
				if h.scrollbarDragAxis {
					// Vertical drag: cursor delta → scroll offset delta,
					// using the same geometry as the painter (shared
					// ScrollbarMetrics) so the thumb tracks the cursor
					// 1:1 and the content follows the thumb.
					dy := cssY - h.scrollbarDragStart
					m := rendering.VerticalScrollbarMetrics(drv, h.scrollbarDragBox)
					if m.OK {
						travel := m.TrackLen - m.ThumbLen
						if travel < 1 {
							travel = 1
						}
						newSy := h.scrollbarDragScroll + dy*(m.MaxScroll/travel)
						if newSy < 0 {
							newSy = 0
						}
						if newSy > m.MaxScroll {
							newSy = m.MaxScroll
						}
						// Preserve the horizontal offset (a vertical drag
						// must never reset a box's sx).
						sx, _ := drv.BoxScrollOffset(h.scrollbarDragBox)
						drv.SetBoxScrollOffset(h.scrollbarDragBox, sx, newSy)
						h.dispatchScrollEvent(h.scrollbarDragBox)
					}
				} else {
					// Horizontal drag: cursor delta → scroll offset delta,
					// using the shared ScrollbarMetrics (same geometry as
					// the painter) so the thumb tracks the cursor 1:1.
					dx := cssX - h.scrollbarDragStart
					m := rendering.HorizontalScrollbarMetrics(drv, h.scrollbarDragBox)
					if m.OK {
						travel := m.TrackLen - m.ThumbLen
						if travel < 1 {
							travel = 1
						}
						newSx := h.scrollbarDragScroll + dx*(m.MaxScroll/travel)
						if newSx < 0 {
							newSx = 0
						}
						if newSx > m.MaxScroll {
							newSx = m.MaxScroll
						}
						if setScrollXFor(drv, h.scrollbarDragBox, newSx) {
							h.markScrollDirty()
						}
					}
				}
				// ── Hover tracking ──
				newEl := rendering.HitTest(rv, cssX, cssY, "")
				if newEl != h.hoveredEl {
					oldHover := h.hoveredEl
					if oldHover != nil {
						oldHover.SetHovered(false)
					}
					if newEl != nil {
						newEl.SetHovered(true)
					}
					h.hoveredEl = newEl
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							h.hoverStyleFastPath(rv, fr, oldHover, newEl)
						}
					}
				}
			}
		case window.EventMouseButton:
			if os.Getenv("WB_EVT_DEBUG") != "" {
				log.Printf("[evt] MouseButton action=%d at=(%.0f,%.0f) cur=(%.0f,%.0f) win=%dx%d",
					ev.Action, ev.X, ev.Y, h.cursorX, h.cursorY, h.win.Width(), h.win.Height())
			}
			csX, csY := h.win.ContentScale()
			if csX <= 0 {
				csX = 1
			}
			if csY <= 0 {
				csY = 1
			}
			cssX := ev.X / csX
			cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY())

			if ev.Action == int(glfw.Press) {
				// ★ 按钮过滤（浏览器标准）：仅左键触发点击交互（active 状态、
				// select 弹窗、checkbox/radio/range、选区、resize 拖拽）。
				// 右键按下在浏览器中只预备 contextmenu，不触发任何点击行为；
				// Release 时由 handleContextMenu 派发 contextmenu 事件。
				if ev.Button != int(glfw.MouseButton1) {
					break // 非左键按下：跳过全部点击交互
				}
				if debugPaintLog {
					log.Printf("[dbg/click] press at css=(%.0f,%.0f)", cssX, cssY)
				}
				// ── Active state ──
				if rv != nil {
					activeEl := rendering.HitTest(rv, cssX, cssY, "")
					if activeEl != nil {
						if debugPaintLog {
							log.Printf("[dbg/click] hit=%s class=%q type=%q", activeEl.LocalName(), activeEl.ClassName(), activeEl.GetAttribute("type"))
						}
						activeEl.SetActive(true)
						h.activeEl = activeEl

						// ★ 派发 JS mousedown（浏览器标准）：Vue 的 @mousedown
						// 与 document.addEventListener('mousedown') 依赖它。
						// 此前引擎只在内部处理 active/select/checkbox/range/
						// 滚动条/textarea-resize/选区，从不派发 mousedown DOM
						// 事件 → 侧栏分隔条、面板分隔条、输入框拖拽等基于
						// mousedown+mousemove 的 JS 拖拽全部失效。
						// 坐标用视口 CSS 坐标（clientX/clientY 语义，与
						// mousemove/mouseup 同基准，位移差值正确）。
						h.downJSFocused = false
						activeEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseDown, dom.MouseEventInit{
							EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
							ClientX:   ev.X / csX,
							ClientY:   ev.Y / csY,
							Button:    dom.MouseButtonLeft,
							Buttons:   1,
							Detail:    1,
						}))

						// ── <select> dropdown: open/close popup ──
						// WebKit RenderMenuList opens a native popup on click.
						// We emulate it with a fixed-position overlay layer
						// listing the <option> children. Clicking an option
						// sets the value + dispatches change (Vue v-model);
						// clicking outside closes.
						if h.selectPopup != nil {
							// popup 打开中：命中 popup 内 option → 选择并关闭；
							// 否则点击 popup 外 → 关闭。
							if h.popupContains(activeEl) {
								h.selectPopupOptionClicked(activeEl)
								h.closeSelectPopup()
							} else {
								h.closeSelectPopup()
							}
						}
						sel := activeEl
						if sel.LocalName() != "select" {
							// 允许点击 select 的箭头/内部子元素时也打开（子元素少见）
							for p := sel.ParentElement(); p != nil; p = p.ParentElement() {
								if p.LocalName() == "select" {
									sel = p
									break
								}
							}
						}
						if sel.LocalName() == "select" && h.selectPopup == nil {
							h.handleSelectClick(sel, rv, cssX, cssY)
						}

						// ── Toggle checkbox / radio on click ──
						if activeEl.LocalName() == "input" {
							inputType := activeEl.GetAttribute("type")
							if inputType == "checkbox" || inputType == "radio" {
								if in, ok := html5.ToInputElement(activeEl); ok {
									if inputType == "checkbox" {
										in.SetChecked(!in.Checked())
									} else if inputType == "radio" {
										// Uncheck all radio buttons with same name
										name := activeEl.GetAttribute("name")
										if name != "" && h.wv.MainFrame() != nil {
											doc := h.wv.MainFrame().Document()
											if doc != nil {
												allInputs := doc.GetElementsByTagName("input")
												for _, r := range allInputs {
													if r.GetAttribute("type") == "radio" && r.GetAttribute("name") == name {
														if r2, ok2 := html5.ToInputElement(r); ok2 {
															r2.SetChecked(false)
														}
													}
												}
											}
										}
										in.SetChecked(true)
									}
									// ★ 派发 change 事件：Vue 的 v-model（checkbox/radio）
									// 监听 change 更新组件状态。此前只 toggle 了
									// DOM checked 属性而不派发事件，导致「组件不可
									// 操作」——视觉上勾选但 Vue 状态未同步。
									activeEl.DispatchEvent(dom.NewEvent("change", true, false, false))
									if debugPaintLog {
										log.Printf("[dbg/click] toggled %s checked=%v", inputType, in.Checked())
									}
									h.wv.RebuildRenderTree()
								}
							} else if inputType == "range" {
								// ── Range slider ──
								// 浏览器行为：点击 track 立即把 value 吸附到
								// 点击位置（step 取整）并派发 input，同时进入
								// 拖动跟踪（thumb 吸到手指下，随鼠标移动继续
								// 更新）；mouseup 派发 change。
								if in, ok := html5.ToInputElement(activeEl); ok && !in.Disabled() {
									if h.setRangeValueFromX(activeEl, rv, cssX) {
										activeEl.DispatchEvent(dom.NewEvent("input", true, false, false))
										h.wv.RebuildRenderTree()
									}
									h.rangeDragEl = activeEl
									h.rangeDragRV = rv
									if debugPaintLog {
										log.Printf("[dbg/click] range press x=%.0f value=%s", cssX, in.Value())
									}
								}
							}
						}

						if mf := h.wv.MainFrame(); mf != nil {
							if fr := mf.Frame(); fr != nil {
								fr.MarkRenderTreeDirty()
							}
						}
					}
				}
				// Check for scrollbar interaction.
				scrollHit := rendering.HitTestScrollbar(rv, cssX, cssY)
				if scrollHit != nil && !scrollHit.IsCorner {
					box := scrollHit.Box
					// ★ 滚动条所属的 RenderView：iframe 子文档滚动条属于
					// 子 Frame 的 rv（偏移表/几何在子 rv），全部读写用 srv。
					srv := scrollHit.RV
					if srv == nil {
						srv = rv // 兼容：未记录 RV 时回退主视图
					}
							// ── Thumb drag ──
							if scrollHit.IsVThumb {
								h.smoothActive = false // thumb drag is 1:1, not smoothed
								h.scrollbarDragging = true
								h.scrollbarDragBox = box
								h.scrollbarDragRV = srv
								h.scrollbarDragAxis = true // vertical
								_, sy := srv.BoxScrollOffset(box)
								h.scrollbarDragStart = cssY
								h.scrollbarDragScroll = sy
								break
							}
							if scrollHit.IsHThumb {
								h.smoothActive = false // thumb drag is 1:1, not smoothed
								h.scrollbarDragging = true
						h.scrollbarDragBox = box
						h.scrollbarDragRV = srv
						h.scrollbarDragAxis = false // horizontal
						h.scrollbarDragStart = cssX
						h.scrollbarDragScroll = scrollXFor(srv, box)
						break
					}
					// ── Arrow buttons → line scroll (clamped to the same
					// range as the thumb, shared geometry) ──
					const lineStep = 16.0
					if scrollHit.IsVUpArrow {
						sx, sy := srv.BoxScrollOffset(box)
						sy -= lineStep
						if sy < 0 {
							sy = 0
						}
						if m := rendering.VerticalScrollbarMetrics(srv, box); m.OK && sy > m.MaxScroll {
							sy = m.MaxScroll
						}
						srv.SetBoxScrollOffset(box, sx, sy)
						h.dispatchScrollEvent(box)
						h.markScrollDirty()
						break
					}
					if scrollHit.IsVDownArrow {
						sx, sy := srv.BoxScrollOffset(box)
						sy += lineStep
						if m := rendering.VerticalScrollbarMetrics(srv, box); m.OK {
							if sy < 0 {
								sy = 0
							}
							if sy > m.MaxScroll {
								sy = m.MaxScroll
							}
						}
						srv.SetBoxScrollOffset(box, sx, sy)
						h.dispatchScrollEvent(box)
						h.markScrollDirty()
						break
					}
					if scrollHit.IsHLeftArrow {
						if setScrollXFor(srv, box, scrollXFor(srv, box)-lineStep) {
							h.markScrollDirty()
						}
						break
					}
					if scrollHit.IsHRightArrow {
						if setScrollXFor(srv, box, scrollXFor(srv, box)+lineStep) {
							h.markScrollDirty()
						}
						break
					}
					// ── Track click (non-thumb) → page scroll ──
					if scrollHit.IsVTrack {
						sx, sy := srv.BoxScrollOffset(box)
						pb := box.PaddingBoxRect()
						// Determine click position relative to the thumb
						// center using the SAME geometry as the painter.
						m := rendering.VerticalScrollbarMetrics(srv, box)
						if m.OK {
							syRatio := sy / m.MaxScroll
							if syRatio < 0 {
								syRatio = 0
							}
							if syRatio > 1 {
								syRatio = 1
							}
							thumbTrackSpace := m.TrackLen - m.ThumbLen
							thumbCenterY := pb.Y + 12.0 + 5.0 + syRatio*thumbTrackSpace + m.ThumbLen/2
							pageH := m.ViewLen // one page = the visible content height
							if cssY < thumbCenterY {
								sy -= pageH
							} else {
								sy += pageH
							}
							if sy < 0 {
								sy = 0
							}
							if sy > m.MaxScroll {
								sy = m.MaxScroll
							}
							srv.SetBoxScrollOffset(box, sx, sy)
							h.dispatchScrollEvent(box)
						}
						break
					}
					if scrollHit.IsHTrack {
						pb := box.PaddingBoxRect()
						// Same geometry as the painter (shared metrics).
						m := rendering.HorizontalScrollbarMetrics(srv, box)
						if m.OK {
							hSx := scrollXFor(srv, box)
							sxRatio := hSx / m.MaxScroll
							if sxRatio < 0 {
								sxRatio = 0
							}
							if sxRatio > 1 {
								sxRatio = 1
							}
							thumbTrackSpace := m.TrackLen - m.ThumbLen
							thumbCenterX := pb.X + 12.0 + 5.0 + sxRatio*thumbTrackSpace + m.ThumbLen/2
							pageW := m.ViewLen // one page = visible content width
							newSx := hSx - pageW
							if cssX >= thumbCenterX {
								newSx = hSx + pageW
							}
							if newSx < 0 {
								newSx = 0
							}
							if newSx > m.MaxScroll {
								newSx = m.MaxScroll
							}
							if setScrollXFor(srv, box, newSx) {
								h.markScrollDirty()
							}
						}
						break
					}
				}

				now := time.Now()
				dx := ev.X - h.lastClickX
				dy := ev.Y - h.lastClickY
				isConsecutive := now.Sub(h.lastClickTime) < 500*time.Millisecond &&
					dx > -5 && dx < 5 && dy > -5 && dy < 5

				if isConsecutive {
					h.clickCount++
					if h.clickCount > 4 {
						h.clickCount = 4
					}
				} else {
					h.clickCount = 1
				}

				// Map click count to granularity: 1=char, 2=word, 3=line, 4=paragraph
				var gran rendering.TextGranularity
				switch h.clickCount {
				case 2:
					gran = rendering.GranularityWord
				case 3:
					gran = rendering.GranularityLine
				case 4:
					gran = rendering.GranularityParagraph
				default:
					gran = rendering.GranularityCharacter
				}
				h.selGranularity = gran

				// Shift+Click extends selection from the anchor.
				if (ev.Mods&int(glfw.ModShift)) != 0 && h.clickCount > 0 &&
					(h.selAnchorX != 0 || h.selAnchorY != 0) {
					h.selEndX = cssX
					h.selEndY = cssY
					h.shiftSelecting = true
					h.selecting = false
				} else {
					// Reset caret blink so the caret is immediately visible on focus.
					h.caretBlinkTime = time.Now()
					rendering.CaretVisible = true
					rendering.CaretVisibleControl = true

					// HitTest the click position to find the element under cursor.
					// If it's a form control (input/textarea/select), set focus.
					resizeHandle := false
					if rv != nil {
						hitEl := rendering.HitTest(rv, cssX, cssY, "")
						if debugPaintLog {
							log.Printf("[dbg/click] Press at css=(%.0f,%.0f) imeFocusedEl=%v hitEl=%v localName=%q type=%q",
								cssX, cssY, h.imeFocusedEl != nil, hitEl != nil,
								func() string { if hitEl != nil { return hitEl.LocalName() }; return "" }(),
								func() string { if hitEl != nil { return hitEl.GetAttribute("type") }; return "" }())
						}
						// ── textarea CSS resize handle ──
						// Pressing the bottom-right corner (resize:vertical/
						// both) starts a resize drag instead of focusing /
						// entering the textarea (browser behavior). The drag
						// updates the element's height on mouse move; the
						// press is consumed so no caret placement happens.
						if hitEl != nil && hitEl.LocalName() == "textarea" && h.resizeDragEl == nil {
							if rb := rv.FindRenderBoxForNode(hitEl); rb != nil {
								if st := rb.Style(); st != nil && rendering.ResizeModeOf(st) != 0 {
									// ★ 视口坐标比较：rb.X/Y 是 layout 坐标（不含
									// 祖先滚动容器的滚动偏移），cssX/cssY 是视口
									// 坐标。settings-body 滚动后 layout 与视口
									// 偏差可达数百 px → 手柄命中失败 → 「按下不能
									// 拖动」。BoxViewportRect 减祖先滚动偏移换算。
									bx, by, bw, bh := rendering.BoxViewportRect(rv, rb)
									const rHandle = 15.0
									if cssX > bx+bw-rHandle && cssY > by+bh-rHandle {
										h.resizeDragEl = hitEl
										h.resizeDragRV = rv
										h.resizeDragStartY = cssY
										h.resizeDragStartH = rb.Height()
										// 记录拖动中保持的光标（按 resize 模式：
										// vertical→NS、horizontal→EW、both→NWSE）
										switch rendering.ResizeModeOf(st) {
										case 2:
											h.resizeDragCursor = window.CursorNS
										case 1:
											h.resizeDragCursor = window.CursorEW
										default:
											h.resizeDragCursor = window.CursorNWSE
										}
										resizeHandle = true
									}
								}
							}
						}
						if !resizeHandle && hitEl != nil && isFocusableElement(hitEl) {
							if hitEl != h.imeFocusedEl {
								h.FocusElement(hitEl)
							}
						} else if !resizeHandle && h.imeFocusedEl != nil && !h.downJSFocused {
							// ★ 点击不可聚焦元素时失焦（浏览器：焦点移到
							// body）。但若 mousedown 派发期间 JS 已重新聚焦
							// （xterm 点击终端容器 → textarea.focus() →
							// FocusBridge 置 downJSFocused），则保持新焦点
							// ——否则「点击终端后光标消失+不可输入」。
							h.Unfocus()
						}
					}

					// If the click is on a text form control, calculate the
					// character offset and set the form-control selection.
					if !resizeHandle && h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						offset := h.calcTextControlOffset(h.imeFocusedEl, cssX, cssY)
						// Start a mouse-drag selection from this press: the
						// anchor stays at the press point; subsequent mouse
						// moves extend the End. Previously selecting was never
						// set true, so drag-to-select did nothing.
						h.selecting = true
						h.shiftSelecting = false
						h.mouseDownX = cssX
						h.mouseDownY = cssY
						h.hysteresisMet = false
						if (ev.Mods&int(glfw.ModShift)) != 0 && rendering.FocusedFormControlSel != nil {
							// Shift+Click extends form-control selection.
							h.selecting = false
							rendering.FocusedFormControlSel.End = offset
							rendering.FocusedFormControlSel.Active = true
						} else {
							rendering.FocusedFormControlSel = &rendering.FormControlSelection{
								Start:  offset,
								End:    offset,
								Active: true,
							}
							if os.Getenv("WB_IME_DEBUG") != "" {
								log.Printf("[ime] click-pos css=(%.0f,%.0f) box=(%.0f,%.0f) offset=%d sel={%d,%d}",
									cssX, cssY, h.mouseDownX, h.mouseDownY, offset, offset, offset)
							}
						}
					} else if h.imeFocusedEl != nil {
						// Click outside a text control clears the form-control selection.
						h.selecting = false
						rendering.FocusedFormControlSel = nil
					} else {
						// ★ 普通文本（非 form control）拖拽选区接线。
						// 此前 handleDragSelection 从未被调用（无调用点、
						// selAnchor 无赋值）——普通文本按下 selecting 恒为
						// false，拖选毫无高亮。现在 Press 定位 anchor：
						// 后续 mousemove 更新 selEnd 实时扩展（跨 Frame 由
						// HitTestText 下钻支持），Release 保留最终选区。
						h.selecting = false
						if rv != nil {
							tp := rendering.HitTestText(rv, cssX, cssY)
							if tp.IsValid() {
								if !h.shiftSelecting {
									h.selAnchorX = cssX
									h.selAnchorY = cssY
									h.selEndX = cssX
									h.selEndY = cssY
									h.mouseDownX = cssX
									h.mouseDownY = cssY
									h.hysteresisMet = false
									h.selecting = true
								}
								h.handleDragSelection(rv)
								if mf := h.wv.MainFrame(); mf != nil {
									if fr := mf.Frame(); fr != nil {
										fr.MarkRenderTreeDirty()
									}
								}
							} else {
								// 点击空白：清除遗留选区。
								h.shiftSelecting = false
								rendering.ClearSelection()
							}
						}
					}
				}
			} else if ev.Action == int(glfw.Release) {
				// ── 右键释放：浏览器在 mouseup 之后触发 contextmenu
				//    （cancelable，前端 @contextmenu.prevent 打开右键菜单）。
				//    右键不参与 click/active/选区/拖拽清理。──
				if ev.Button != int(glfw.MouseButton1) {
					if rv != nil {
						h.handleContextMenu(rv, ev)
					}
					break
				}
				// ── Clear active state ──
				if h.activeEl != nil {
					h.activeEl.SetActive(false)
					h.activeEl = nil
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
				// ── Dispatch DOM click on release ──
				// Vue binds @click via addEventListener('click'); the click
				// must be dispatched here so those handlers actually run.
				if rv != nil {
					h.handleClick(rv, ev)
				}
				// ★ 派发 JS mouseup（浏览器标准）：拖拽收尾
				// （document.addEventListener('mouseup')）依赖它。此前
				// Release 只派发 click，JS 拖拽（mousedown 已派发）在
				// mouseup 时永远收不了尾（监听不解除 → 拖拽状态悬挂）。
				// 坐标用视口 CSS 坐标，与 mousedown/mousemove 同基准。
				if rv != nil {
					if upEl := rendering.HitTest(rv, ev.X/csX, ev.Y/csY, ""); upEl != nil {
						upEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseUp, dom.MouseEventInit{
							EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
							ClientX:   ev.X / csX,
							ClientY:   ev.Y / csY,
							Button:    dom.MouseButtonLeft,
							Buttons:   0,
							Detail:    1,
						}))
					}
				}
				// End scrollbar drag if active.
				if h.scrollbarDragging {
					h.scrollbarDragging = false
					h.scrollbarDragBox = nil
					h.scrollbarDragRV = nil
				}
				// End range thumb drag: dispatch change (the final value,
				// after the drag), then clear the drag state.
				if h.rangeDragEl != nil {
					h.rangeDragEl.DispatchEvent(dom.NewEvent("change", true, false, false))
					h.rangeDragEl = nil
					h.rangeDragRV = nil
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
				// End textarea CSS resize drag.
				if h.resizeDragEl != nil {
					h.resizeDragEl = nil
					h.resizeDragRV = nil
					h.resizeDragCursor = window.CursorArrow
				}
				if h.selecting {
					csX, csY := h.win.ContentScale()
					if csX <= 0 {
						csX = 1
					}
					if csY <= 0 {
						csY = 1
					}
					cssX := ev.X / csX
					cssY := ev.Y/csY + float64(h.wv.Page().MainFrame().View().ScrollY())

					// Hysteresis: only start dragging after moving > 3px from
					// mouseDown. A plain click (no movement) ends the drag
					// selection immediately — it becomes a caret placement.
					if !h.hysteresisMet {
						dx := cssX - h.mouseDownX
						dy := cssY - h.mouseDownY
						if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
							h.selecting = false
							h.hysteresisMet = false
							if rendering.FocusedFormControlSel != nil {
								rendering.FocusedFormControlSel.Active = false
							}
							if rv != nil && h.imeFocusedEl == nil {
								// 普通文本点击（未拖动）：清空选区并显示
								// caret（若命中可编辑文本）。
								h.handleDragSelection(rv)
							}
							continue // not yet dragging; treat as a click
						}
						h.hysteresisMet = true
					}
					// ★ Do NOT recompute End here from the release point:
					// the selection range is finalized by the last
					// mousemove while dragging. Recomputing with the release
					// coordinates makes the selection jump when the mouse is
					// released outside the control or after a fast drag
					// (browsers keep the last drag position on mouseup).
					// End the drag selection.
					h.selecting = false
					h.hysteresisMet = false
					if rendering.FocusedFormControlSel != nil {
						rendering.FocusedFormControlSel.Active = false
					}
					if rv != nil && h.imeFocusedEl == nil {
						// 普通文本拖拽结束：用最后一次 mousemove 的 End
						// 重算选区（Active=false 停止跟随），跨 Frame 的
						// 终点保持子文档位置（HitTestText 下钻）。
						h.handleDragSelection(rv)
					}
					if mf := h.wv.MainFrame(); mf != nil {
						if fr := mf.Frame(); fr != nil {
							fr.MarkRenderTreeDirty()
						}
					}
				}
			}
		case window.EventChar:
			h.handleCharInput(ev)

		case window.EventKey:
			// ★ 浏览器标准事件顺序：先派发 keydown 到焦点元素，JS 决定
			// 是否 preventDefault。preventDefault 后引擎不做默认行为
			// （编辑/滚动）。xterm.js 的 textarea 聚焦时用 keydown 处理
			// 快捷键/组合键（Ctrl+C 发送 \x03、箭头发送 \x1b[A、Enter 发
			// \r）并 preventDefault；普通字符键不 preventDefault → 引擎
			// EventChar 照常插入 value + 派发 input。此前引擎从不派发
			// keydown → xterm 的快捷键全部失效（「终端不可输入」根因之
			// 一：组合键没有入口）。
			if h.imeFocusedEl != nil && (ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat)) {
				keyEv := dom.NewKeyboardEventFromInit(dom.EventKeyDown, dom.KeyboardEventInit{
					EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
					Key:       domKeyName(ev),
					Code:      domKeyCode(ev),
					CtrlKey:   (ev.Mods & int(glfw.ModControl)) != 0,
					ShiftKey:  (ev.Mods & int(glfw.ModShift)) != 0,
					AltKey:    (ev.Mods & int(glfw.ModAlt)) != 0,
					MetaKey:   (ev.Mods & int(glfw.ModSuper)) != 0,
					Repeat:    ev.Action == int(glfw.Repeat),
					// ★ DOM keyCode（浏览器标准 Windows VK 码）：xterm 等
					// 旧式库用 e.keyCode 判断（Enter=13）——此前传 GLFW
					// 键码（257）导致 Enter 分支永不匹配（回车不能执行）。
					KeyCode: domKeyCodeValue(ev),
				})
				if os.Getenv("WB_EVT_DEBUG") != "" {
					log.Printf("[evt/key] key=%d name=%q code=%q action=%d focused=%v", ev.Key, domKeyName(ev), domKeyCode(ev), ev.Action, h.imeFocusedEl != nil)
				}
				prevented := !h.imeFocusedEl.DispatchEvent(keyEv)
				if os.Getenv("WB_EVT_DEBUG") != "" {
					log.Printf("[evt/key] dispatched keydown key=%q → prevented=%v (xterm handler)", domKeyName(ev), prevented)
				}
				if prevented {
					break // JS preventDefault：引擎不做默认编辑/滚动
				}
			}
			// ★ Text editing keys (backspace/delete/arrows/home/end) take
			// priority over scrolling when a form control is focused. Without
			// this, Backspace/Delete did nothing and arrows scrolled the page.
			edited := false
			if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) && !h.imeComposing {
				if ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat) {
					switch ev.Key {
					case int(glfw.KeyBackspace):
						h.deleteFocusedChar(false)
						edited = true
					case int(glfw.KeyDelete):
						h.deleteFocusedChar(true)
						edited = true
					case int(glfw.KeyLeft):
						h.moveFocusedCaret(-1)
						edited = true
					case int(glfw.KeyRight):
						h.moveFocusedCaret(1)
						edited = true
					case int(glfw.KeyHome):
						h.setFocusedCaret(0)
						edited = true
					case int(glfw.KeyEnd):
						h.setFocusedCaret(-1) // clamps to end
						edited = true
					case int(glfw.KeyEnter), int(glfw.KeyKPEnter):
						// Enter inserts a newline only in multi-line
						// controls (textarea); single-line inputs submit
						// (not implemented here, so the key is a no-op).
						if h.imeFocusedEl.LocalName() == "textarea" {
							h.pasteIntoFocused("\n")
							edited = true
						}
					}
				}
			}
			if edited {
				break
			}
			// Keyboard scrolling for PageUp/PageDown/Arrow keys.
			if ev.Action == int(glfw.Press) || ev.Action == int(glfw.Repeat) {
				if rv != nil {
					csX, csY := h.win.ContentScale()
					if csX <= 0 {
						csX = 1
					}
					if csY <= 0 {
						csY = 1
					}
					cssX := h.cursorX / csX
					cssY := h.cursorY / csY
					// ★ 键盘滚动同样路由到子 Frame：iframe 内滚动容器属于
					// 子 RenderView，偏移读写用子 rv（同滚轮路径）。
					tgt := rv.ScrollTargetAt(cssX, cssY)
					if tgt.Box != nil {
						scrollBox := tgt.Box
						srv := tgt.RV
						sx, sy := srv.BoxScrollOffset(scrollBox)
						pb := scrollBox.PaddingBoxRect()
						pageH := pb.Height
						delta := 0.0
						switch ev.Key {
						case int(glfw.KeyPageUp):
							delta = -pageH * 0.8
						case int(glfw.KeyPageDown):
							delta = pageH * 0.8
						case int(glfw.KeyUp):
							delta = -60.0
						case int(glfw.KeyDown):
							delta = 60.0
						case int(glfw.KeyLeft):
							delta = -40.0
						case int(glfw.KeyRight):
							delta = 40.0
						}
						if delta != 0 {
							if ev.Key == int(glfw.KeyLeft) || ev.Key == int(glfw.KeyRight) {
								newSx := sx + delta
								cw, _ := srv.BoxContentSize(scrollBox)
								if newSx < 0 {
									newSx = 0
								}
								if maxSx := cw - pb.Width; newSx > maxSx {
									newSx = maxSx
								}
								srv.SetBoxScrollOffset(scrollBox, newSx, sy)
								h.dispatchScrollEvent(scrollBox)
							} else {
								newSy := sy + delta
								_, ch := srv.BoxContentSize(scrollBox)
								if newSy < 0 {
									newSy = 0
								}
								if maxSy := ch - pb.Height; newSy > maxSy {
									newSy = maxSy
								}
								srv.SetBoxScrollOffset(scrollBox, sx, newSy)
								h.dispatchScrollEvent(scrollBox)
							}
							break
						}
					} else {
						// FrameView-level keyboard scroll.
						delta := 0
						vh := h.frameView.Height()
						switch ev.Key {
						case int(glfw.KeyPageUp):
							delta = -int(float64(vh) * 0.8)
						case int(glfw.KeyPageDown):
							delta = int(float64(vh) * 0.8)
						case int(glfw.KeyUp):
							delta = -60
						case int(glfw.KeyDown):
							delta = 60
						}
						if delta != 0 {
							h.frameView.ScrollBy(0, delta)
						}
					}
				}
			}
			if ev.Action == int(glfw.Press) && (ev.Mods&int(glfw.ModControl)) != 0 {
				switch ev.Key {
				case int(glfw.KeyA):
					// Ctrl+A: select all text in the focused form control.
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						val := focusedElementValue(h.imeFocusedEl)
						runes := []rune(val)
						rendering.FocusedFormControlSel = &rendering.FormControlSelection{
							Start: 0,
							End:   len(runes),
						}
						h.wv.RebuildRenderTree()
					}
				case int(glfw.KeyV):
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						clipText := h.win.GetClipboardString()
						if clipText != "" {
							h.pasteIntoFocused(clipText)
						}
					}
				case int(glfw.KeyX):
					if h.imeFocusedEl != nil && isTextFormControl(h.imeFocusedEl) {
						sel := rendering.FocusedFormControlSel
						if sel != nil && sel.Start != sel.End {
							start, end := sel.Start, sel.End
							if start > end {
								start, end = end, start
							}
							val := focusedElementValue(h.imeFocusedEl)
							runes := []rune(val)
							if start >= 0 && end <= len(runes) {
								cutText := string(runes[start:end])
								h.win.SetClipboardString(cutText)
								newVal := string(runes[:start]) + string(runes[end:])
								setFocusedElementValue(h.imeFocusedEl, newVal)
								rendering.FocusedFormControlSel = &rendering.FormControlSelection{
									Start: start, End: start,
								}
								h.imeInputText = newVal
								h.wv.RebuildRenderTree()
							}
						}
					}
				}
			}
		}
	}
}

// domKeyName maps a GLFW key code to the DOM KeyboardEvent.key value
// (browser semantics). Letters reflect the Shift state (a/A); other keys
// map to their standard DOM names ("Enter", "ArrowLeft", "F1", ...).
func domKeyName(ev window.Event) string {
	shift := (ev.Mods & int(glfw.ModShift)) != 0
	switch glfw.Key(ev.Key) {
	case glfw.KeyEnter, glfw.KeyKPEnter:
		return "Enter"
	case glfw.KeyBackspace:
		return "Backspace"
	case glfw.KeyTab:
		return "Tab"
	case glfw.KeyEscape:
		return "Escape"
	case glfw.KeyDelete:
		return "Delete"
	case glfw.KeyInsert:
		return "Insert"
	case glfw.KeyLeft:
		return "ArrowLeft"
	case glfw.KeyRight:
		return "ArrowRight"
	case glfw.KeyUp:
		return "ArrowUp"
	case glfw.KeyDown:
		return "ArrowDown"
	case glfw.KeyHome:
		return "Home"
	case glfw.KeyEnd:
		return "End"
	case glfw.KeyPageUp:
		return "PageUp"
	case glfw.KeyPageDown:
		return "PageDown"
	case glfw.KeyCapsLock:
		return "CapsLock"
	case glfw.KeyLeftShift, glfw.KeyRightShift:
		return "Shift"
	case glfw.KeyLeftControl, glfw.KeyRightControl:
		return "Control"
	case glfw.KeyLeftAlt, glfw.KeyRightAlt:
		return "Alt"
	case glfw.KeyLeftSuper, glfw.KeyRightSuper:
		return "Meta"
	case glfw.KeySpace:
		return " "
	}
	// Letters (ASCII 65..90) and digits (48..57): GLFW uses ASCII codes.
	if ev.Key >= 'A' && ev.Key <= 'Z' {
		if shift {
			return string(rune(ev.Key))
		}
		return string(rune(ev.Key) + 32) // lowercase
	}
	if ev.Key >= '0' && ev.Key <= '9' {
		return string(rune(ev.Key))
	}
	// Punctuation: GLFW ASCII key codes match the character for these.
	switch glfw.Key(ev.Key) {
	case glfw.KeyComma:
		return ","
	case glfw.KeyPeriod:
		return "."
	case glfw.KeySlash:
		return "/"
	case glfw.KeySemicolon:
		return ";"
	case glfw.KeyApostrophe:
		return "'"
	case glfw.KeyLeftBracket:
		return "["
	case glfw.KeyRightBracket:
		return "]"
	case glfw.KeyBackslash:
		return "\\"
	case glfw.KeyGraveAccent:
		return "`"
	case glfw.KeyMinus:
		return "-"
	case glfw.KeyEqual:
		return "="
	}
	// Function keys F1..F25.
	if ev.Key >= int(glfw.KeyF1) && ev.Key <= int(glfw.KeyF25) {
		return fmt.Sprintf("F%d", ev.Key-int(glfw.KeyF1)+1)
	}
	return "Unidentified"
}

// domKeyCodeValue maps a GLFW key code to the DOM legacy keyCode integer
// (Windows virtual-key codes, what xterm/legacy libs read via e.keyCode).
// Browser standard: Enter=13, Backspace=8, arrows=37-40, letters/digits =
// ASCII. Previously the engine passed the raw GLFW scancode (KeyEnter=257),
// so xterm's `e.keyCode === 13` Enter branch never matched → keydown was
// dispatched but not preventDefault-ed → "回车不能触发执行".
func domKeyCodeValue(ev window.Event) int {
	switch glfw.Key(ev.Key) {
	case glfw.KeyEnter, glfw.KeyKPEnter:
		return 13
	case glfw.KeyBackspace:
		return 8
	case glfw.KeyTab:
		return 9
	case glfw.KeyEscape:
		return 27
	case glfw.KeyDelete:
		return 46
	case glfw.KeyInsert:
		return 45
	case glfw.KeyLeft:
		return 37
	case glfw.KeyUp:
		return 38
	case glfw.KeyRight:
		return 39
	case glfw.KeyDown:
		return 40
	case glfw.KeyHome:
		return 36
	case glfw.KeyEnd:
		return 35
	case glfw.KeyPageUp:
		return 33
	case glfw.KeyPageDown:
		return 34
	case glfw.KeySpace:
		return 32
	}
	// Letters/digits/punctuation: GLFW ASCII codes match DOM keyCode for
	// these (both use ASCII / Windows VK convention).
	if ev.Key >= 32 && ev.Key <= 126 {
		return ev.Key
	}
	return 0
}

// domKeyCode maps a GLFW key code to the DOM KeyboardEvent.code value
// (physical key identifier, "KeyA"/"Digit1"/"ArrowLeft"/...).
func domKeyCode(ev window.Event) string {
	switch glfw.Key(ev.Key) {
	case glfw.KeyEnter, glfw.KeyKPEnter:
		return "Enter"
	case glfw.KeyBackspace:
		return "Backspace"
	case glfw.KeyTab:
		return "Tab"
	case glfw.KeyEscape:
		return "Escape"
	case glfw.KeyDelete:
		return "Delete"
	case glfw.KeyInsert:
		return "Insert"
	case glfw.KeyLeft:
		return "ArrowLeft"
	case glfw.KeyRight:
		return "ArrowRight"
	case glfw.KeyUp:
		return "ArrowUp"
	case glfw.KeyDown:
		return "ArrowDown"
	case glfw.KeyHome:
		return "Home"
	case glfw.KeyEnd:
		return "End"
	case glfw.KeyPageUp:
		return "PageUp"
	case glfw.KeyPageDown:
		return "PageDown"
	case glfw.KeyCapsLock:
		return "CapsLock"
	case glfw.KeySpace:
		return "Space"
	case glfw.KeyLeftShift:
		return "ShiftLeft"
	case glfw.KeyRightShift:
		return "ShiftRight"
	case glfw.KeyLeftControl:
		return "ControlLeft"
	case glfw.KeyRightControl:
		return "ControlRight"
	case glfw.KeyLeftAlt:
		return "AltLeft"
	case glfw.KeyRightAlt:
		return "AltRight"
	case glfw.KeyLeftSuper:
		return "MetaLeft"
	case glfw.KeyRightSuper:
		return "MetaRight"
	}
	if ev.Key >= 'A' && ev.Key <= 'Z' {
		return "Key" + string(rune(ev.Key))
	}
	if ev.Key >= '0' && ev.Key <= '9' {
		return "Digit" + string(rune(ev.Key))
	}
	switch glfw.Key(ev.Key) {
	case glfw.KeyComma:
		return "Comma"
	case glfw.KeyPeriod:
		return "Period"
	case glfw.KeySlash:
		return "Slash"
	case glfw.KeySemicolon:
		return "Semicolon"
	case glfw.KeyApostrophe:
		return "Quote"
	case glfw.KeyLeftBracket:
		return "BracketLeft"
	case glfw.KeyRightBracket:
		return "BracketRight"
	case glfw.KeyBackslash:
		return "Backslash"
	case glfw.KeyGraveAccent:
		return "Backquote"
	case glfw.KeyMinus:
		return "Minus"
	case glfw.KeyEqual:
		return "Equal"
	}
	if ev.Key >= int(glfw.KeyF1) && ev.Key <= int(glfw.KeyF25) {
		return fmt.Sprintf("F%d", ev.Key-int(glfw.KeyF1)+1)
	}
	return "Unidentified"
}

// handleSelection processes text selection based on granularity and drag state.
func (h *Host) handleSelection(rv *rendering.RenderView, pos rendering.TextPosition) {
	rendering.SetCaret(nil)
}

// handleDragSelection handles text selection after a drag or shift+click.
// handleDragSelection handles text selection after a drag or shift+click.
func (h *Host) handleDragSelection(rv *rendering.RenderView) {
	start := rendering.HitTestText(rv, h.selAnchorX, h.selAnchorY)
	end := rendering.HitTestText(rv, h.selEndX, h.selEndY)

	if !start.IsValid() && !end.IsValid() {
		// Click outside text: clear selection and caret.
		rendering.ClearSelection()
		rendering.SetCaret(nil)
		return
	}
	if !start.IsValid() {
		start = end
	}
	if !end.IsValid() {
		end = start
	}

	// Apply granularity expansion for drag with word/line/paragraph.
	if h.selGranularity > rendering.GranularityCharacter && h.selecting {
		forward := h.selEndY > h.selAnchorY ||
			(h.selEndY == h.selAnchorY && h.selEndX >= h.selAnchorX)
		end = rendering.ExpandPosition(end, h.selGranularity, forward)
		start = rendering.ExpandPosition(start, h.selGranularity, !forward)
	}

	// When start == end (click without drag), show caret instead of empty selection.
	if !h.selecting && !h.shiftSelecting &&
		start.RT == end.RT && start.Offset == end.Offset {
		rendering.ClearSelection()
		if h.isRenderTextEditable(start.RT) {
			pos := start
			rendering.SetCaret(&pos)
		} else {
			rendering.SetCaret(nil)
		}
		return
	}

	rendering.CurrentSelection = &rendering.Selection{
		Start:       start,
		End:         end,
		Active:      h.selecting,
		Granularity: h.selGranularity,
	}
	if h.selecting || (start.RT != end.RT) || (start.Offset != end.Offset) {
		rendering.SetCaret(nil)
	}
}

// isRenderTextEditable reports whether rt belongs to the IME-focused (editable)
// element. Used to decide whether to show a caret when clicking on text.
func (h *Host) isRenderTextEditable(rt *rendering.RenderText) bool {
	if h.imeFocusedEl == nil || rt == nil {
		return false
	}
	node := rt.Node()
	if node == nil {
		return false
	}
	parent := node.ParentNode()
	if parent == nil {
		return false
	}
	el, ok := parent.(*dom.Element)
	return ok && el == h.imeFocusedEl
}

// updateCaret positions the caret for the IME-focused element (e.g. an input
// box), placing it at the end of the element's text content. Called when no
// mouse-driven selection is active.
func (h *Host) updateCaret(rv *rendering.RenderView) {
	if rv == nil || h.imeFocusedEl == nil {
		rendering.SetCaret(nil)
		return
	}
	var found *rendering.RenderText
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if rt, ok := o.(*rendering.RenderText); ok {
			if node := rt.Node(); node != nil {
				if parent := node.ParentNode(); parent != nil {
					if el, ok := parent.(*dom.Element); ok && el == h.imeFocusedEl {
						found = rt
						return
					}
				}
			}
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	if found != nil {
		pos := rendering.TextPosition{RT: found, Offset: found.Length()}
		rendering.SetCaret(&pos)
	}
}

// handleContextMenu 在右键释放时把 contextmenu 事件派发到命中元素
// （浏览器行为：mouseup 后触发 contextmenu，可冒泡、可取消）。
// 前端 Vue 的 @contextmenu.prevent 监听器调用 preventDefault 并打开
// 右键菜单；派发后必须跑微任务队列 + 重建渲染树让菜单 visible 生效。
func (h *Host) handleContextMenu(rv *rendering.RenderView, ev window.Event) {
	csX, csY := 1.0, 1.0
	if h.win != nil {
		csX, csY = h.win.ContentScale()
		if csX <= 0 {
			csX = 1
		}
		if csY <= 0 {
			csY = 1
		}
	}
	cssX := ev.X / csX
	cssY := ev.Y / csY
	if fv := h.wv.Page().MainFrame().View(); fv != nil {
		cssY += float64(fv.ScrollY())
	}

	deepest := rendering.HitTest(rv, cssX, cssY, "")
	if deepest == nil {
		return
	}
	if os.Getenv("WB_EVT_DEBUG") != "" {
		log.Printf("[ctx] hit=%s.%s at (%.0f,%.0f)", deepest.LocalName(), deepest.GetAttribute("class"), cssX, cssY)
	}
	me := dom.NewMouseEventFromInit(dom.EventContextMenu, dom.MouseEventInit{
		EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
		ClientX:   cssX,
		ClientY:   cssY,
		Button:    dom.MouseButtonRight,
		Detail:    1,
	})
	deepest.DispatchEvent(me)
	// Vue 菜单打开（visible=true）是响应式更新：跑微任务 + 事件循环 + 重建。
	h.processEventLoop()
	if h.wv.JSInterpreter() != nil {
		interp := h.wv.JSInterpreter()
		deadline := time.Now().Add(180 * time.Millisecond)
		for {
			interp.RunJobs()
			h.processEventLoop()
			el := interp.GetEventLoop()
			if el == nil || el.PendingTasks() == 0 {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	h.wv.RebuildRenderTree()
}

// handleClick converts the physical-pixel click coordinates to CSS pixels
// (the render tree's coordinate space), hit-tests the render tree, and
// dispatches a DOM click event (so JS addEventListener('click') listeners,
// e.g. Vue @click, run) plus the onclick value: "js:" prefix → EvalJS,
// otherwise → click handler.
func (h *Host) handleClick(rv *rendering.RenderView, ev window.Event) {
	if os.Getenv("WB_EVT_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[click] handleClick entered rv=%v at=(%.0f,%.0f) action=%d\n",
			rv != nil, ev.X, ev.Y, ev.Action)
	}
	if rv == nil {
		return
	}
	csX, csY := h.win.ContentScale()
	if csX <= 0 {
		csX = 1
	}
	if csY <= 0 {
		csY = 1
	}
	clickCSSX := ev.X / csX
	clickCSSY := ev.Y / csY
	clickY := clickCSSY + float64(h.wv.Page().MainFrame().View().ScrollY())

	el := rendering.HitTest(rv, clickCSSX, clickY, "onclick")
	if os.Getenv("WB_EVT_DEBUG") != "" {
		en := "<nil>"
		if el != nil {
			en = el.LocalName() + "." + el.GetAttribute("class")
		}
		fmt.Fprintf(os.Stderr, "[click] hit-onclick=%s at (%.0f,%.0f)\n", en, clickCSSX, clickY)
	}
	if el == nil {
		deepest := rendering.HitTest(rv, clickCSSX, clickY, "")
		prevFocus := rendering.FocusedFormControl
		// Dispatch a bubbling DOM click so JS listeners (Vue @click,
		// addEventListener) fire — previously they never ran, so every
		// button/icon/switch click did nothing.
		if deepest != nil {
			// ★ click 必须携带 clientX/clientY（浏览器标准：click 坐标 = 触发
			// 它的 mousedown/mouseup 坐标）。此前用 NewMouseEvent（clientX/Y=0）
			// 派发 → CM6 等用 event.clientY 计算点击行的库全部错位（行号差
			// 一行/恒 0）。
			deepest.DispatchEvent(dom.NewMouseEventFromInit(dom.EventClick, dom.MouseEventInit{
				EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
				ClientX:   clickCSSX,
				ClientY:   clickCSSY,
				Button:    dom.MouseButtonLeft,
				Buttons:   1,
				Detail:    1,
			}))
		}
		if h.clickHandler != nil {
			h.clickHandler(deepest, "", clickCSSX, clickCSSY)
		}
		if deepest != nil {
			handleFormSubmitClick(deepest)
			handleLabelToggle(deepest)
			if deepest.LocalName() == "a" {
				h.handleAnchorClick(deepest)
			}
		}
		// ★ Always rebuild: JS listeners may have mutated the DOM (tabs,
		// list selection, dialog visibility). Without this the tree is
		// stale until some other action (e.g. focusing an input) rebuilds.
		// ★ Flush the JS microtask queue FIRST — Vue's reactive updates
		// (state.activeActivity etc.) are scheduled as Promise.then
		// microtasks, so without this the DOM still shows the OLD state
		// and every click looks like it "does nothing".
		h.processEventLoop()
		// ★ Vue 的 scheduler（nextTick/component update）用 Promise 微任务
		// （goja 队列），与 processEventLoop 驱动的 queueMicrotask（jsc 队列）
		// 是两套：dispatch 时 jsListener 里已 RunJobs 一次，但 Vue 的
		// flushJobs 可能在 await 恢复链更后面，这里再补一次确保触发。
		if h.wv.JSInterpreter() != nil {
			interp := h.wv.JSInterpreter()
			// ★ 智能等待（替代原固定 8×15ms=120ms sleep）：
			//   事件派发后 Vue 的 async 链（@click → await apiPost →
			//   await loadXXX …）由 RunJobs（goja microtask）+ ProcessTasks
			//   （宏任务/rAF）交替驱动。普通点击无异步链，PendingTasks
			//   立即排空 → 提前退出（省 120ms 固定延迟，点击响应更快）；
			//   切换工作区等长链持续驱动直到排空或超时（上限保护）。
			//   Vue 的 nextTick 是 Promise.microtask，RunJobs 一次会清空
			//   队列；仅当链中调度了宏任务（setTimeout/rAF）才需要
			//   ProcessTasks 补一轮，因此无需人为固定 sleep。
			el := interp.GetEventLoop()
			deadline := time.Now().Add(180 * time.Millisecond)
			for {
				interp.RunJobs()
				h.processEventLoop()
				if el == nil || el.PendingTasks() == 0 {
					break
				}
				if time.Now().After(deadline) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		h.wv.RebuildRenderTree()
		if os.Getenv("WB_EVT_DEBUG") != "" && h.wv.JSInterpreter() != nil {
			if v, err := h.wv.JSInterpreter().RunJS(`(function(){
				try {
					var out = [];
					var items = document.querySelectorAll('.conv-item');
					var titles = [];
					for (var i=0;i<items.length && i<20;i++) {
						var t = items[i].textContent.replace(/\s+/g,' ').slice(0,20);
						var a = items[i].className.indexOf('active')>=0 ? 'A' : ' ';
						titles.push(i + a + ':' + t);
					}
					out.push('items=' + items.length + ' [' + titles.join(' | ') + ']');
					// 找当前 active 的 title
					var actTitle = '';
					for (var i=0;i<items.length;i++) if (items[i].className.indexOf('active')>=0) { actTitle = items[i].textContent.slice(0,25); break; }
					out.push('activeTitle=' + actTitle);
					return out.join(' | ');
				} catch(e){ return 'err:'+e.message; }
			})()`); err == nil {
				fmt.Fprintf(os.Stderr, "[click] post-dispatch %s\n", v.ToString())
			}
		}
		if rendering.FocusedFormControl != prevFocus {
			h.processEventLoop()
			h.wv.RebuildRenderTree()
		}
		return
	}
	onclickVal := el.GetAttribute("onclick")
	if onclickVal == "" {
		el.DispatchEvent(dom.NewMouseEventFromInit(dom.EventClick, dom.MouseEventInit{
			EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:   clickCSSX,
			ClientY:   clickCSSY,
			Button:    dom.MouseButtonLeft,
			Buttons:   1,
			Detail:    1,
		}))
		if h.clickHandler != nil {
			h.clickHandler(el, "", clickCSSX, clickCSSY)
		}
		handleFormSubmitClick(el)
		handleLabelToggle(el)
		if el.LocalName() == "a" {
			h.handleAnchorClick(el)
		}
		h.processEventLoop()
		// ★ Vue 的 scheduler（nextTick/组件更新）用 Promise 微任务
		// （goja 队列），与 processEventLoop 驱动的 queueMicrotask（jsc 队列）
		// 是两套：dispatch 后必须补 goja RunJobs，否则 Vue 的
		// flushJobs 不跑，DOM 永远停留在旧状态（点击"看起来没反应"）。
		if h.wv.JSInterpreter() != nil {
			interp := h.wv.JSInterpreter()
			interp.RunJobs()
			// 智能等待：点击 handler 里的 async 链（@click → await fetch →
			// Vue 更新）立即推进到排空，避免依赖下一帧渲染循环才推进。
			if el0 := interp.GetEventLoop(); el0 != nil {
				deadline := time.Now().Add(120 * time.Millisecond)
				for {
					interp.RunJobs()
					h.processEventLoop()
					if el0.PendingTasks() == 0 || time.Now().After(deadline) {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		}
		h.wv.RebuildRenderTree()
		if os.Getenv("WB_EVT_DEBUG") != "" && h.wv.JSInterpreter() != nil {
			if v, err := h.wv.JSInterpreter().RunJS(`(function(){
				try {
					var out = [];
					// 1. Vue devtools hook
					var vhook = window.__VUE__;
					out.push('VUE=' + typeof vhook);
					// 2. app 实例
					var appEl = document.querySelector('#app');
					var app = appEl && appEl.__vue_app__;
					out.push('app=' + (app ? 'yes' : 'no'));
					// 3. 遍历组件树找 RightPanel 实例读 setupState.state.currentConvId
					if (app) {
						var found = [];
						var walk = function(inst, depth) {
							if (!inst || depth > 6) return;
							if (inst.setupState && inst.setupState.state && inst.setupState.state.currentConvId !== undefined) {
								found.push('cur=' + inst.setupState.state.currentConvId + ' msgs=' + (inst.setupState.state.messages||[]).length);
							}
							if (inst.setupState && Object.keys(inst.setupState).length) {
								var ks = Object.keys(inst.setupState);
								if (ks.indexOf('switchConv') >= 0) {
									found.push('hasSwitchConv cur=' + (inst.setupState.state ? inst.setupState.state.currentConvId : '?'));
								}
							}
							if (inst.subTree && inst.subTree.component) walk(inst.subTree.component, depth+1);
							if (inst.subTree && inst.subTree.children) {
								for (var i=0;i<inst.subTree.children.length;i++) {
									if (inst.subTree.children[i] && inst.subTree.children[i].component) walk(inst.subTree.children[i].component, depth+1);
								}
							}
						};
						walk(app._instance, 0);
						out.push('FOUND=' + (found.length ? found.join(';') : 'none'));
					}
					// 4. DOM active 索引
					var items = document.querySelectorAll('.conv-item');
					var active = [];
					for (var i=0;i<items.length;i++) if (items[i].className.indexOf('active')>=0) active.push(i);
					out.push('items=' + items.length + ' active=' + JSON.stringify(active));
					return out.join(' | ');
				} catch(e){ return 'err:'+e.message; }
			})()`); err == nil {
				fmt.Fprintf(os.Stderr, "[click] post-dispatch %s\n", v.ToString())
			}
		}
		return
	}
	if strings.HasPrefix(onclickVal, "js:") {
		_, _ = h.wv.EvalJS(onclickVal[3:])
		h.processEventLoop()
		h.wv.RebuildRenderTree()
		return
	}
	el.DispatchEvent(dom.NewMouseEventFromInit(dom.EventClick, dom.MouseEventInit{
		EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
		ClientX:   clickCSSX,
		ClientY:   clickCSSY,
		Button:    dom.MouseButtonLeft,
		Buttons:   1,
		Detail:    1,
	}))
	if h.clickHandler != nil {
		h.clickHandler(el, onclickVal, clickCSSX, clickCSSY)
	}
	h.processEventLoop()
	h.wv.RebuildRenderTree()
}
// handleSelectClick opens the <select> dropdown popup for sel: a
// fixed-position overlay listing the select's <option> elements, positioned
// just below the select box. Mirrors WebKit RenderMenuList.showPopup.
// setRangeValueFromX computes a <input type="range"> value from a horizontal
// CSS-viewport x coordinate, snapped to the input's step, and writes it back
// to the DOM (SetValue). Mirrors the browser: clicking the track or dragging
// the thumb maps the cursor position onto [min,max] in steps. The thumb
// geometry is the full input box (paintRangeSlider centers the 8px track and
// draws the thumb across the full width), so the clickable range spans the
// whole control box.
func (h *Host) setRangeValueFromX(el *dom.Element, rv *rendering.RenderView, cssX float64) (changed bool) {
	if el == nil || rv == nil {
		return false
	}
	in, ok := html5.ToInputElement(el)
	if !ok || in.Disabled() {
		return false
	}
	minV := parseHostFloat(in.Min(), 0)
	maxV := parseHostFloat(in.Max(), 100)
	stepV := parseHostFloat(in.Step(), 1)
	if stepV <= 0 {
		stepV = 1
	}
	if maxV <= minV {
		maxV = minV + 1
	}
	box := rv.FindRenderBoxForNode(el)
	if box == nil {
		return false
	}
	bw := box.Width()
	if bw <= 0 {
		return false
	}
	// AbsoluteX is page coordinates; the event x is viewport coordinates —
	// subtract the box's scroll offset to align (same as handleSelectClick).
	left := box.AbsoluteX()
	if _, so := rv.BoxScrollOffset(box); so > 0 {
		left -= so
	}
	frac := (cssX - left) / bw
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	val := minV + frac*(maxV-minV)
	val = minV + math.Round((val-minV)/stepV)*stepV
	if val < minV {
		val = minV
	} else if val > maxV {
		val = maxV
	}
	// 按 step 属性的小数位格式化（step="0.1" → 1 位小数），避免浮点
	// 误差产生长尾小数：val = min + Round(frac*range/step)*step 中
	// 乘法是 float64（如 6*0.1 = 0.6000000000000001），%g 取最短往返
	// 表示会把误差原样输出，设置面板温度值出现 "0.6000000000000001"。
	s := strconv.FormatFloat(val, 'f', decimalsForStep(in.Step()), 64)
	if in.Value() == s {
		return false
	}
	in.SetValue(s)
	return true
}

// resolveDragRV 返回拖拽元素当前所属的 RenderView 实例。
// ★ 拖动中渲染树会因 Vue input 每帧重建（新 RenderView 实例），press
// 时缓存的 RV 是旧实例——其 box 几何（AbsoluteX/Width）与当前鼠标坐标
// 错配 → range value 乱跳、thumb 抖动（「拖拽不跟手」真凶）。按 DOM
// 节点重新解析：先主文档，再 iframe 子文档；都找不到回退旧值（元素
// 可能已从树中移除）。
func (h *Host) resolveDragRV(el *dom.Element, old *rendering.RenderView) *rendering.RenderView {
	if el == nil {
		return old
	}
	if rv := h.wv.RenderView(); rv != nil && rv.FindRenderBoxForNode(el) != nil {
		return rv
	}
	var found *rendering.RenderView
	page.ForEachIFrame(func(_ *dom.Element, f *page.Frame) {
		if found != nil {
			return
		}
		if frv := f.RenderView(); frv != nil && frv.FindRenderBoxForNode(el) != nil {
			found = frv
		}
	})
	if found != nil {
		return found
	}
	return old
}

// decimalsForStep returns the number of decimal places implied by a
// <input> step attribute string ("0.1" → 1, "1" → 0, "0.05" → 2).
func decimalsForStep(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s) - i - 1
	}
	return 0
}

// parseHostFloat parses a float string with a default on error/empty.
func parseHostFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// atoiOr parses an int string with a default on error/empty.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func (h *Host) handleSelectClick(sel *dom.Element, rv *rendering.RenderView, cssX, cssY float64) {
	selEl, ok := html5.ToSelectElement(sel)
	if !ok {
		return
	}
	if selEl.Disabled() {
		return
	}
	// 定位 select 的屏幕位置（相对于当前 RenderView 的滚动偏移）。
	var sx, sy float64
	var boxW float64 = 180
	var boxH float64 = 0
	if box := rv.FindRenderBoxForNode(sel); box != nil {
		sx, sy = box.AbsoluteX(), box.AbsoluteY()
		boxH = box.Height()
		if bw := box.Width(); bw > 0 {
			boxW = bw
		}
		// popup 层是 fixed 定位（相对视口），select 的 absolute 坐标
		// 已含滚动偏移，减去当前滚动量得到视口坐标。
		if _, sy0 := rv.BoxScrollOffset(box); sy0 > 0 {
			sy -= sy0
		}
	}
	// 创建浮层容器（fixed 定位，覆盖在 select 下方）。
	doc := h.wv.MainFrame().Document()
	if doc == nil {
		return
	}
	overlay := doc.CreateElement("div")
	overlay.SetAttribute("class", "select-popup")
	// 高度：option 行数 * 行高（用实际 option 数量；超 8 项滚动）。
	opts := selEl.Options()
	rowH := 24
	n := len(opts)
	if n > 8 {
		n = 8
	}
	popH := n*rowH + 4
	// 浮层从 select 底部下方展开（浏览器标准），不覆盖 select 本身。
	popTop := sy + boxH
	viewH := h.wv.Height()
	// 底部空间不足时向上展开（浮层底部对齐视口底部）。
	if popTop+float64(popH) > float64(viewH)-8 {
		popTop = sy - float64(popH)
		if popTop < 0 {
			popTop = 0
		}
	}
	overlay.SetAttribute("style", fmt.Sprintf("position:fixed;left:%.0fpx;top:%.0fpx;width:%.0fpx;height:%dpx;background:var(--bg-secondary);border:1px solid var(--border-color);border-radius:4px;box-shadow:0 4px 12px rgba(0,0,0,0.35);z-index:9999;overflow-y:auto;", sx, popTop, boxW, popH))
	// 添加每个 option。
	current := selEl.Value()
	// ★ option 全部用 class 驱动样式（布局/hover/选中/禁用态都放样式表，
	// 见 index.html 的 .select-popup-option 规则族）——内联 style 优先级
	// 高于外部规则，会阻挡 :hover 伪类的背景切换。与浏览器一致：popup
	// option 的样式由 CSS 规则管理。
	for _, opt := range opts {
		optEl := doc.CreateElement("div")
		optEl.SetAttribute("class", "select-popup-option")
		optVal := opt.GetAttribute("value")
		if optVal == "" {
			optVal = opt.TextContent()
		}
		optEl.SetAttribute("data-value", optVal)
		optEl.SetAttribute("data-select-popup", "1")
		if opt.HasAttribute("disabled") {
			optEl.SetAttribute("class", "select-popup-option select-popup-option-disabled")
		} else if optVal == current {
			optEl.SetAttribute("class", "select-popup-option select-popup-option-selected")
		}
		txt := doc.CreateTextNode(opt.TextContent())
		_ = optEl.AppendChild(txt)
		_ = overlay.AppendChild(optEl)
	}
	// 挂到 body 末尾（fixed 定位层）。
	body := doc.Body()
	if body == nil {
		return
	}
	_ = body.AppendChild(overlay)
	h.selectPopup = overlay
	h.selectPopupSelect = sel
	h.wv.RebuildRenderTree()
	h.wv.EnsureLayout()
}

// closeSelectPopup removes the open dropdown overlay, if any.
func (h *Host) closeSelectPopup() {
	if h.selectPopup == nil {
		return
	}
	body := h.wv.MainFrame().Document().Body()
	if body != nil {
		_ = body.RemoveChild(h.selectPopup)
	}
	h.selectPopup = nil
	h.selectPopupSelect = nil
	h.wv.RebuildRenderTree()
}

// popupContains reports whether el is inside the currently open popup layer.
func (h *Host) popupContains(el *dom.Element) bool {
	if el == nil || h.selectPopup == nil {
		return false
	}
	for p := el; p != nil; p = p.ParentElement() {
		if p == h.selectPopup {
			return true
		}
	}
	return false
}

// selectPopupOptionClicked applies the clicked <option>'s value to the owning
// <select> and dispatches a change event so Vue v-model updates the model.
func (h *Host) selectPopupOptionClicked(el *dom.Element) {
	if el == nil || h.selectPopupSelect == nil {
		return
	}
	if el.LocalName() == "div" && el.GetAttribute("data-value") != "" {
		// disabled option 不选择。
		cls := el.GetAttribute("class")
		if strings.Contains(cls, "disabled") {
			return
		}
		sel := h.selectPopupSelect
		if selEl, ok := html5.ToSelectElement(sel); ok {
			selEl.SetValue(el.GetAttribute("data-value"))
		}
		// 派发 change（冒泡）→ Vue v-model 更新。
		sel.DispatchEvent(dom.NewEvent("change", true, false, false))
		if debugPaintLog {
			log.Printf("[dbg/click] select set value=%q", el.GetAttribute("data-value"))
		}
	}
}

func handleFormSubmitClick(el *dom.Element) {
	if el == nil {
		return	}
	switch el.LocalName() {
	case "input":
		in, ok := html5.ToInputElement(el)
		if !ok {
			return
		}
		if in.Type() != html5.InputSubmit && in.Type() != html5.InputImage {
			return
		}
		form := in.Form()
		if form == nil {
			return
		}
		f, ok := html5.ToFormElement(form)
		if ok {
			f.RequestSubmit(el)
		}
	case "button":
		btn, ok := html5.ToButtonElement(el)
		if !ok {
			return
		}
		// Default button type is "submit".
		if btn.Type() != html5.ButtonSubmit {
			return
		}
		form := btn.Form()
		if form == nil {
			return
		}
		f, ok := html5.ToFormElement(form)
		if ok {
			f.RequestSubmit(el)
		}
	}
}

// handleLabelToggle 实现 <label> 的点击转发：点击 label 或其任意后代，
// 切换内部包裹的 checkbox/radio 的选中状态（浏览器 label 语义）。
// 开关（switch）的 track span 点击因此能 toggle 内嵌 checkbox，
// 且 `input:checked + .track::after` 滑块随之移动。
func handleLabelToggle(el *dom.Element) {
	if el == nil {
		return
	}
	// 向上找 label 祖先。
	lab := el
	for lab != nil && lab.LocalName() != "label" {
		lab = lab.ParentElement()
	}
	if lab == nil {
		return
	}
	// 找 label 内第一个 checkbox/radio。
	for c := lab.FirstChild(); c != nil; c = c.NextSibling() {
		e, ok := c.(*dom.Element)
		if !ok || e.LocalName() != "input" {
			continue
		}
		typ := e.GetAttribute("type")
		if typ != "checkbox" && typ != "radio" {
			continue
		}
		in, ok := html5.ToInputElement(e)
		if !ok {
			continue
		}
		if typ == "checkbox" {
			in.SetChecked(!in.Checked())
		} else {
			in.SetChecked(true)
		}
		return
	}
}

// handleAnchorClick performs navigation when an <a> element is clicked.
// For external URLs (http/https) it attempts to open the system browser;
// for local file paths it reads and loads the referenced file as new HTML.
func (h *Host) handleAnchorClick(el *dom.Element) {
	a, ok := html5.ToAnchorElement(el)
	if !ok {
		return
	}
	href := a.Href()
	if href == "" || href == "#" {
		return
	}

	// For external links, log (opening the system browser is platform-specific
	// and left to the embedder; here we just log).
	if a.IsExternalLink() {
		fmt.Printf("app: external link: %s (open in system browser)\n", href)
		return
	}

	// For local links, try to load the referenced file as new HTML.
	if h.wv != nil {
		frame := h.wv.MainFrame()
		if frame != nil {
			// Resolve the path: strip file:// prefix if present.
			filePath := href
			if strings.HasPrefix(filePath, "file://") {
				filePath = strings.TrimPrefix(filePath, "file://")
			}
			// Try to read and load the file as HTML.
			data, err := os.ReadFile(filePath)
			if err == nil {
				frame.LoadHTML(string(data))
			} else {
				fmt.Printf("app: cannot load link %q: %v\n", href, err)
			}
		}
	}
}

// applyIMEEvents updates the focused element's text from IME
// applyIMEEvents updates the focused element's text from IME
// composition / character input events and dispatches the appropriate DOM
// events (input, change, compositionstart/update/end) so that JavaScript
// event listeners and the wb-ui form submission pipeline are notified.
// After modifying the element's value, the render tree is rebuilt so the
// next paint frame reflects the updated content.
// next paint frame reflects the updated content.

// FocusedElement returns the currently IME-focused element, or nil if none.
func (h *Host) FocusedElement() *dom.Element {
	return h.imeFocusedEl
}

// pasteIntoFocused inserts text into the currently focused form control,
// After updating the value, it dispatches input and change DOM events and
// rebuilds the render tree so the next frame reflects the update.
func (h *Host) pasteIntoFocused(text string) {
	if h.imeFocusedEl == nil {
		return
	}
	if formControlEditLimits(h.imeFocusedEl) == 0 {
		return // readonly/disabled: no paste
	}

	sel := rendering.FocusedFormControlSel
	val := focusedElementValue(h.imeFocusedEl)
	runes := []rune(val)

	var newVal string
	var newOffset int
	textRunes := []rune(text)

	if sel != nil && sel.Start != sel.End {
		// Replace selection with pasted text.
		start, end := sel.Start, sel.End
		if start > end {
			start, end = end, start
		}
		newVal = string(runes[:start]) + text + string(runes[end:])
		newOffset = start + len(textRunes)
	} else if sel != nil {
		// Insert at cursor position.
		pos := sel.Start
		if pos > len(runes) {
			pos = len(runes)
		}
		newVal = string(runes[:pos]) + text + string(runes[pos:])
		newOffset = pos + len(textRunes)
	} else {
		// Append to end.
		newVal = val + text
		newOffset = len([]rune(newVal))
	}

	if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
		newVal = truncateToMaxLen(newVal, maxLen)
		if newOffset > len([]rune(newVal)) {
			newOffset = len([]rune(newVal))
		}
	}

	setFocusedElementValue(h.imeFocusedEl, newVal)
	h.imeInputText = newVal
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{
		Start: newOffset,
		End:   newOffset,
	}

	// Dispatch input event (inputType="insertFromPaste").
	inputEvent := dom.NewInputEvent("insertFromPaste", text, false)
	h.imeFocusedEl.DispatchEvent(inputEvent)

	// Dispatch change event (bubbles, not cancelable).
	h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))

	h.wv.RebuildRenderTree()
	h.ensureFocusedCaretVisible()
}

// deleteFocusedChar deletes one character (or the active selection) in the
// focused form control. forward=true deletes after the caret (Delete key),
// forward=false deletes before it (Backspace), matching browser behavior.
func (h *Host) deleteFocusedChar(forward bool) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	val := focusedElementValue(el)
	runes := []rune(val)
	start, end := len(runes), len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		start, end = sel.Start, sel.End
		if start > end {
			start, end = end, start
		}
	}
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start == end {
		if forward {
			if end >= len(runes) {
				return // nothing after the caret
			}
			end++
		} else {
			if start <= 0 {
				return // nothing before the caret
			}
			start--
		}
	}
	newVal := string(runes[:start]) + string(runes[end:])
	setFocusedElementValue(el, newVal)
	h.imeInputText = newVal
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: start, End: start}

	inputType := "deleteContentForward"
	if !forward {
		inputType = "deleteContentBackward"
	}
	el.DispatchEvent(dom.NewInputEvent(inputType, "", false))
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	h.wv.RebuildRenderTree()
}

// moveFocusedCaret moves the caret of the focused form control by delta
// runes (negative = left). Home/End are handled via moveFocusedCaretTo.
func (h *Host) moveFocusedCaret(delta int) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	runes := []rune(focusedElementValue(el))
	pos := len(runes)
	if sel := rendering.FocusedFormControlSel; sel != nil {
		pos = sel.Start
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	h.setFocusedCaret(pos)
}

// moveFocusedCaretTo sets the caret of the focused form control to an
// absolute rune index.
func (h *Host) setFocusedCaret(pos int) {
	el := h.imeFocusedEl
	if el == nil {
		return
	}
	runes := []rune(focusedElementValue(el))
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: pos, End: pos}
	if mf := h.wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
	h.ensureFocusedCaretVisible()
}

// applyIMEEvents updates the focused element's text from IME
// composition/handwriting events, dispatching DOM input/composition/change
// events and rebuilding the render tree as needed. Text is inserted at the
// caret (FocusedFormControlSel.Start), replacing any selection — not appended
// to the end of the value (browser behavior).
func (h *Host) applyIMEEvents(events []ime.Event) {
	needsRebuild := false
	for _, ev := range events {
		switch ev.Kind {
		case ime.EventCompositionUpdate:
			if !h.imeComposing {
				// Composition starts: snapshot the base text (everything
				// EXCEPT the in-progress composition) and the insertion point.
				if formControlEditLimits(h.imeFocusedEl) == 0 {
					continue // readonly/disabled: never start a composition
				}
				h.imeComposing = true
				h.imeComposeBase = focusedElementValue(h.imeFocusedEl)
				// Default caret = END of text when no click positioned it
				// (browsers put the caret at the end on focus; inserting at
				// 0 put new text at the HEAD).
				baseRunes := []rune(h.imeComposeBase)
				start := len(baseRunes)
				if sel := rendering.FocusedFormControlSel; sel != nil {
					start = sel.Start
					if start < 0 {
						start = 0
					}
				}
				if start > len(baseRunes) {
					start = len(baseRunes)
				}
				h.imeComposeStart = start
			}
			h.imeComposeText = ev.Composition
			if h.imeFocusedEl != nil {
				runes := []rune(h.imeComposeBase)
				pos := h.imeComposeStart
				if pos > len(runes) {
					pos = len(runes)
				}
				newText := string(runes[:pos]) + h.imeComposeText + string(runes[pos:])
				setFocusedElementValue(h.imeFocusedEl, newText)
				needsRebuild = true

				// Dispatch compositionupdate event
				h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionupdate", ev.Composition))

				// Dispatch input event with insertCompositionText
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertCompositionText", ev.Composition, true))
			}

		case ime.EventCharInput:
			wasComposing := h.imeComposing
			char := string(ev.Char)
			// Normalize Enter's CR (if a platform delivers it as a char)
			// to a newline in multi-line controls; single-line inputs drop it.
			if ev.Char == '\r' {
				if h.imeFocusedEl != nil && h.imeFocusedEl.LocalName() == "textarea" {
					char = "\n"
				} else {
					break
				}
			}
			var newText string
			if wasComposing {
				// Composition confirmed: replace the composition preview with
				// the committed char, keeping the base text around it.
				h.imeComposing = false
				h.imeComposeText = ""
				runes := []rune(h.imeComposeBase)
				pos := h.imeComposeStart
				if pos > len(runes) {
					pos = len(runes)
				}
				newText = string(runes[:pos]) + char + string(runes[pos:])
				if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
					newText = truncateToMaxLen(newText, maxLen)
				}
				h.imeComposeBase = ""
				if os.Getenv("WB_IME_DEBUG") != "" {
					log.Printf("[ime] compose-commit char=%q pos=%d base=%q → %q", char, pos, string(runes), newText)
				}
			} else {
				// Plain character input: insert at the caret, replacing any
				// selection (browser behavior). Without a click-positioned
				// caret the default is the END of the text (browsers focus
				// with the caret at the end) — inserting at 0 put every
				// character at the HEAD of the value.
				if formControlEditLimits(h.imeFocusedEl) == 0 {
					break // readonly/disabled: ignore the keystroke
				}
				val := focusedElementValue(h.imeFocusedEl)
				runes := []rune(val)
				start, end := len(runes), len(runes)
				if sel := rendering.FocusedFormControlSel; sel != nil {
					start, end = sel.Start, sel.End
					if start > end {
						start, end = end, start
					}
					if start < 0 {
						start = 0
					}
					if end > len(runes) {
						end = len(runes)
					}
				}
				newText = string(runes[:start]) + char + string(runes[end:])
				if maxLen := formControlEditLimits(h.imeFocusedEl); maxLen > 0 {
					newText = truncateToMaxLen(newText, maxLen)
				}
				// Move caret after the inserted char (clamped by maxlength).
				caretPos := start + 1
				if n := len([]rune(newText)); caretPos > n {
					caretPos = n
				}
				rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: caretPos, End: caretPos}
				if os.Getenv("WB_IME_DEBUG") != "" {
					selInfo := "nil"
					if s := rendering.FocusedFormControlSel; s != nil {
						selInfo = fmt.Sprintf("Start=%d End=%d", s.Start, s.End)
					}
					log.Printf("[ime] char char=%q start=%d end=%d sel=%s → %q", char, start, end, selInfo, newText)
				}
			}
			h.imeInputText = newText
			if h.imeFocusedEl != nil {
				if os.Getenv("WB_SCROLL_DEBUG") != "" {
					log.Printf("[scroll/input] IME char=%q value → %q len=%d sel=%s", char, newText, len([]rune(newText)),
						fmt.Sprintf("Start=%d End=%d", rendering.FocusedFormControlSel.Start, rendering.FocusedFormControlSel.End))
				}
				if strings.EqualFold(h.imeFocusedEl.GetAttribute("contenteditable"), "true") {
					// ★ contenteditable（CodeMirror 6 输入区）：光标处插入单个字符，
					//   不全文替换（全文替换会抹掉 CM6 的 .cm-line/高亮 span 结构，
					//   且 CM6 发现文本未变不重建 → 布局永久破坏）。插入后派发
					//   insertText → CM6 readDOMChange 同步 state 并重建结构。
					if !bindings.InsertTextAtSelection(char) {
						break // 无有效 selection：跳过 DOM 修改（保住现有结构）
					}
					h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
				} else {
					setFocusedElementValue(h.imeFocusedEl, newText)
					needsRebuild = true
					h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertText", char, false))
				}

				if wasComposing {
					// End composition
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", newText))
				}

				// Dispatch change event (bubbles, not cancelable)
				h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
			}

		case ime.EventCompositionEnd:
			wasComposing := h.imeComposing
			h.imeComposing = false
			h.imeComposeText = ""
			finalText := h.imeInputText
			if h.imeFocusedEl != nil {
				setFocusedElementValue(h.imeFocusedEl, finalText)
				needsRebuild = true

				if wasComposing {
					// Dispatch compositionend event
					h.imeFocusedEl.DispatchEvent(dom.NewCompositionEvent("compositionend", finalText))
				}

				// Dispatch input event with insertFromComposition
				h.imeFocusedEl.DispatchEvent(dom.NewInputEvent("insertFromComposition", finalText, false))

				// Dispatch change event
				h.imeFocusedEl.DispatchEvent(dom.NewEvent("change", true, false, false))
			}
		}
	}
	if needsRebuild {
		h.wv.RebuildRenderTree()
		h.ensureFocusedCaretVisible()
	}

}

// findBodyBgColor walks the render tree to find the body element's background
// color. In a browser the body background propagates to the root canvas,
// filling the viewport (including the head margin area). Returns transparent
// when not found so the caller can fall back to white.
func findBodyBgColor(o rendering.RenderObject) graphics.Color {
	if o == nil {
		return graphics.Color{}
	}
	// Use the DOM Body() method for direct access to the body element,
	// then find its corresponding RenderObject via render-tree walk.
	if rv := o.View(); rv != nil {
		if doc := rv.Document(); doc != nil {
			if body := doc.Body(); body != nil {
				if bodyRO := findRenderObjectForNode(rendering.RenderObject(rv), body); bodyRO != nil {
					if st := bodyRO.Style(); st != nil {
						if st.BackgroundColor.A > 0 {
							return graphics.Color{R: st.BackgroundColor.R, G: st.BackgroundColor.G, B: st.BackgroundColor.B, A: st.BackgroundColor.A}
						}
						// Body bg is transparent — search children recursively
						// (NOT findBodyBgColor which re-enters doc.Body() path).
						if col := firstNonTransBg(bodyRO); col.A > 0 {
							return col
						}
					}
				}
			}
		}
	}
	// Fallback 1: walk from o's own subtree.
	if col := firstNonTransBg(o); col.A > 0 {
		return col
	}
	// Fallback 2: if o is a RenderView, walk from its first child (the <html> root).
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		if col := firstNonTransBg(c); col.A > 0 {
			return col
		}
	}
	return graphics.Color{}
}

// firstNonTransBg walks a render subtree and returns the first non-transparent
// background color found. Used when body bg is transparent to find the effective
// viewport background from body's children (e.g. #app div).
func firstNonTransBg(o rendering.RenderObject) graphics.Color {
	if o == nil {
		return graphics.Color{}
	}
	// Check the element itself first.
	if st := o.Style(); st != nil && st.BackgroundColor.A > 0 {
		return graphics.Color{R: st.BackgroundColor.R, G: st.BackgroundColor.G, B: st.BackgroundColor.B, A: st.BackgroundColor.A}
	}
	// Recurse into children.
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		if col := firstNonTransBg(c); col.A > 0 {
			return col
		}
	}
	return graphics.Color{}
}

// findRenderObjectForNode searches the render tree for the RenderObject
// that corresponds to the given DOM node.
func findRenderObjectForNode(ro rendering.RenderObject, target dom.Node) rendering.RenderObject {
	if ro == nil {
		return nil
	}
	if ro.Node() == target {
		return ro
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if found := findRenderObjectForNode(c, target); found != nil {
			return found
		}
	}
	return nil
}

// owner carries the given CSS class (used by the [skia] dialog probe).