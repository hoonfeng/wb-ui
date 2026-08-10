// Translation of: Source/WebCore/platform/graphics/GraphicsContext.h
//                  Source/WebCore/platform/graphics/GraphicsContext.cpp
// Completeness: 90%
// Simplifications:
//   - backed by Skia via goskia cgo bindings; real anti-aliased rasterization
//   - linear & radial gradient fills via Skia shader
//   - clip is axis-aligned rectangle OR arbitrary path via ClipPath
//   - transform supports translate, scale, rotate, skew, and full matrix via Concat
//   - text uses Skia's real font rasterizer (FontCache / ComplexTextController
//     are handled by Skia internally)

package graphics

import (
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/wtf"
)

// Color is the Go translation of WebCore::Color (the 8-bit-per-channel RGBA subset).
// It is an alias to wtf.Color, so style and rendering share the same type.
type Color = wtf.Color

// Font is the Go translation of the subset of FontCascade / FontDescription needed by
// the paint pipeline. Real WebKit carries a full FontCascade with platform font
// handles; this port keeps only the CSS-facing fields.
type Font struct {
	Family string
	Size   float64
	Weight int
	Style  string // "normal" | "italic" | "oblique"
}

// Rect is an axis-aligned rectangle in device (post-transform) coordinates. It is the
// Go counterpart of WebCore::FloatRect / IntRect as used by GraphicsContext callers.
type Rect struct {
	X, Y          float64
	Width, Height float64
}

// canvasState is the GraphicsContextState counterpart: it holds the per-save() stack
// frame of clipping, transform and drawing color state.
type canvasState struct {
	clip                   Rect
	hasClip                bool
	translateX, translateY float64
	scaleX, scaleY         float64
	fillColor              Color
	strokeColor            Color
	strokeThickness        float64
}

// Canvas is the Go translation of WebCore::GraphicsContext. It is the 2D drawing
// surface that the rendering painters draw into. Internally it wraps a Skia
// Surface and Canvas, providing real anti-aliased rasterization and text rendering.
type Canvas struct {
	width, height int
	surface       *skia.Surface
	canvas        *skia.Canvas
	fillPaint     *skia.Paint
	strokePaint   *skia.Paint
	clearPaint    *skia.Paint

	// external reports whether the surface is owned externally (e.g. a GPU
	// window surface). When true, Release does not free the surface.
	external bool

	// State tracking for HasClip/ClipRect queries.
	state  canvasState
	states []canvasState

	// gradientPaint is a temporary Paint used by gradient fills to avoid
	// overwriting the shared fillPaint's shader state in Save/Restore.
	gradientPaint *skia.Paint

	// Font cache: maps Font description to *skia.Font.
	fontCache   map[fontKey]*skia.Font
	fontCacheMu sync.Mutex

	// Pixel cache: invalidated on any draw operation, populated by Pixels()/PixelAt().
	pixelCache   []byte
	pixelCacheMu sync.Mutex
}

// fontKey is a cache key for skia.Font lookups.
type fontKey struct {
	family string
	size   float32
	weight int
	style  string
}

// NewCanvas constructs a Canvas backed by a Skia raster surface of the given
// dimensions, initialized to fully transparent black. Mirrors the GraphicsContext
// constructor that wraps a backend image buffer.
func NewCanvas(width, height int) *Canvas {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	skia.Init()
	surface, err := skia.NewRasterSurfaceN32Premul(width, height)
	if err != nil {
		// If Skia surface creation fails, there is no recovery; this is a
		// fatal initialization error.
		panic("graphics: failed to create Skia surface: " + err.Error())
	}
	return newCanvasFromSurface(surface, width, height, false)
}

// NewCanvasFromSurface wraps an existing Skia Surface (e.g. a GPU-backed
// surface from a window's framebuffer) as a graphics.Canvas, allowing the
// rendering pipeline to paint directly onto it without an intermediate
// CPU raster + blit step. The caller retains ownership of the surface
// (Release is a no-op on the returned Canvas).
func NewCanvasFromSurface(surface *skia.Surface, width, height int) *Canvas {
	return newCanvasFromSurface(surface, width, height, true)
}

// newCanvasFromSurface is the shared constructor. When external is true the
// Canvas does not own the surface and Release() will not free it.
func newCanvasFromSurface(surface *skia.Surface, width, height int, external bool) *Canvas {
	c := &Canvas{
		width:       width,
		height:      height,
		surface:     surface,
		canvas:      surface.Canvas(),
		fillPaint:   skia.NewPaint(),
		strokePaint: skia.NewPaint(),
		clearPaint:  skia.NewPaint(),
		fontCache:   make(map[fontKey]*skia.Font),
		external:    external,
	}
	c.canvas.Clear(skia.ColorTransparent)
	c.fillPaint.SetStyle(skia.PaintStyleFill)
	c.fillPaint.SetAntialias(true)
	c.strokePaint.SetStyle(skia.PaintStyleStroke)
	c.strokePaint.SetAntialias(true)
	c.clearPaint.SetStyle(skia.PaintStyleFill)
	c.clearPaint.SetBlendMode(skia.BlendModeClear)
	c.gradientPaint = skia.NewPaint()
	c.gradientPaint.SetStyle(skia.PaintStyleFill)
	c.gradientPaint.SetAntialias(true)
	c.state.scaleX = 1
	c.state.scaleY = 1
	c.state.fillColor = Color{R: 0, G: 0, B: 0, A: 0xFF}
	c.state.strokeColor = Color{R: 0, G: 0, B: 0, A: 0xFF}
	c.state.strokeThickness = 1
	return c
}

// Width / Height return the backing buffer dimensions, mirroring
// GraphicsContext::width() / height() (via the underlying ImageBuffer).
func (c *Canvas) Width() int  { return c.width }
func (c *Canvas) Height() int { return c.height }

// invalidatePixels clears the pixel cache so the next Pixels()/PixelAt() call
// re-reads from the Skia surface.
func (c *Canvas) invalidatePixels() {
	c.pixelCacheMu.Lock()
	c.pixelCache = nil
	c.pixelCacheMu.Unlock()
}

// ensurePixels reads the surface into the pixel cache if not already cached.
func (c *Canvas) ensurePixels() []byte {
	c.pixelCacheMu.Lock()
	defer c.pixelCacheMu.Unlock()
	if c.pixelCache != nil {
		return c.pixelCache
	}
	img := c.surface.Snapshot()
	if img == nil {
		return nil
	}
	defer img.Release()
	pix, err := img.ReadPixels()
	if err != nil {
		return nil
	}
	c.pixelCache = pix
	return pix
}

// Pixels returns the RGBA backing buffer, mirroring ImageBuffer::getImageData(). The
// returned slice is a snapshot of the current surface contents; callers should not
// retain it across mutations.
func (c *Canvas) Pixels() []byte {
	return c.ensurePixels()
}

// SetFillColor / FillColor mirror GraphicsContext::setFillColor() / fillColor().
func (c *Canvas) SetFillColor(col Color) { c.state.fillColor = col }
func (c *Canvas) FillColor() Color       { return c.state.fillColor }

// SetStrokeColor / StrokeColor mirror setStrokeColor() / strokeColor().
func (c *Canvas) SetStrokeColor(col Color) { c.state.strokeColor = col }
func (c *Canvas) StrokeColor() Color       { return c.state.strokeColor }

// SetStrokeThickness / StrokeThickness mirror setStrokeThickness() / strokeThickness().
func (c *Canvas) SetStrokeThickness(t float64) { c.state.strokeThickness = t }
func (c *Canvas) StrokeThickness() float64     { return c.state.strokeThickness }

// Save pushes a copy of the current graphics state onto the state stack, mirroring
// GraphicsContext::save(). The Skia canvas state is also saved so that transforms
// and clips are restored together.
func (c *Canvas) Save() {
	c.states = append(c.states, c.state)
	t0 := time.Now()
	c.canvas.Save()
	CgoTimingSave += time.Since(t0)
}

// SaveCount returns the current save-stack depth (delegates to Skia).
func (c *Canvas) SaveCount() int {
	return c.canvas.SaveCount()
}

// DeviceClipBounds returns the Skia canvas's CURRENT device-space clip
// (physical pixels, after CTM). ok=false when clip is empty. Used to verify
// that the Go-side c.state.clip matches the real Skia clip (RestoreToCount /
// ResetMatrix only manipulate the Go state in places, so they can drift).
func (c *Canvas) DeviceClipBounds() (Rect, bool) {
	l, t, r, b, ok := c.canvas.GetDeviceClipBounds()
	if !ok {
		return Rect{}, false
	}
	return Rect{X: float64(l), Y: float64(t), Width: float64(r - l), Height: float64(b - t)}, true
}

// RestoreToCount pops the save stack down to the given depth. Used by the
// renderer to discard every ancestor clip for fixed-position layers.
func (c *Canvas) RestoreToCount(count int) {
	for len(c.states) > count {
		c.states = c.states[:len(c.states)-1]
	}
	c.canvas.RestoreToCount(count)
	c.invalidatePixels()
}

// SaveLayerWithOpacityBounds pushes an offscreen layer limited to the given
// device-space rect, composited with the given opacity on Restore. Limiting
// the layer bounds is critical on raster: SaveLayer(nil) allocates the WHOLE
// surface (1280×800) and the Restore composite costs ~0.7ms; a bounds-limited
// layer only composites the element's region (~µs).
func (c *Canvas) SaveLayerWithOpacityBounds(opacity float64, r Rect) {
	CanvasSaveLayerCount++
	c.states = append(c.states, c.state)
	paint := skia.NewPaint()
	paint.SetAntialias(true)
	alpha := uint8(opacity * 255)
	if alpha > 255 {
		alpha = 255
	}
	paint.SetColor(skia.RGBA(0xFF, 0xFF, 0xFF, alpha))
	sr := skia.RectXYWH(float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height))
	t0 := time.Now()
	c.canvas.SaveLayer(&sr, paint)
	CgoTimingSaveLayer += time.Since(t0)
}

// SaveLayerWithOpacity pushes an offscreen layer that is composited with the
// given opacity (0.0�?.0) when Restore is called. This mirrors
// GraphicsContext::beginTransparencyLayer() and is used for CSS opacity.
func (c *Canvas) SaveLayerWithOpacity(opacity float64) {
	CanvasSaveLayerCount++
	c.states = append(c.states, c.state)
	paint := skia.NewPaint()
	paint.SetAntialias(true)
	alpha := uint8(opacity * 255)
	if alpha > 255 {
		alpha = 255
	}
	paint.SetColor(skia.RGBA(0xFF, 0xFF, 0xFF, alpha))
	t0 := time.Now()
	c.canvas.SaveLayer(nil, paint)
	CgoTimingSaveLayer += time.Since(t0)
}

// CanvasSaveLayerCount counts SaveLayerWithOpacity calls (debug: WB_PAINT_STATS).
var CanvasSaveLayerCount int

// SaveLayerWithFilter pushes an offscreen layer with a Skia ImageFilter.
// The filter is applied when Restore is called. This is the primary mechanism
// for implementing CSS filters (blur, grayscale, sepia, etc.) that require
// per-pixel operations.
func (c *Canvas) SaveLayerWithFilter(imgFilter *skia.ImageFilter) {
	c.states = append(c.states, c.state)
	if imgFilter != nil {
		paint := skia.NewPaint()
		paint.SetAntialias(true)
		paint.SetImageFilter(imgFilter)
		c.canvas.SaveLayer(nil, paint)
	} else {
		c.canvas.Save()
	}
}

// SaveLayerWithBlendMode pushes an offscreen layer that is composited with the
// given blend mode when Restore is called (CSS mix-blend-mode).
func (c *Canvas) SaveLayerWithBlendMode(mode skia.BlendMode) {
	c.states = append(c.states, c.state)
	paint := skia.NewPaint()
	paint.SetAntialias(true)
	paint.SetBlendMode(mode)
	c.canvas.SaveLayer(nil, paint)
}

// Restore pops the most recently saved graphics state, mirroring GraphicsContext::
// restore(). If the stack is empty it is a no-op (WebKit asserts / logs in that case).
func (c *Canvas) Restore() {
	if len(c.states) == 0 {
		return
	}
	CanvasRestoreCount++
	c.state = c.states[len(c.states)-1]
	c.states = c.states[:len(c.states)-1]
	t0 := time.Now()
	c.canvas.Restore()
	CgoTimingRestore += time.Since(t0)
	c.invalidatePixels()
}

// CanvasRestoreCount counts Restore calls (debug: WB_PAINT_STATS prints it).
var CanvasRestoreCount int

// CgoTiming accumulates wall time inside the cgo canvas state-mutation calls
// (Save/Restore/Clip/Translate), sampled with time.Now per call. Debug-only:
// read via CanvasTimingSummary().
var (
	CgoTimingRestore time.Duration
	CgoTimingSave    time.Duration
	CgoTimingClip    time.Duration
	CgoTimingTrans   time.Duration
	CgoTimingDraw    time.Duration
	CgoTimingSaveLayer time.Duration
)

// wbDrawLogCount: WB_TEXT_DEBUG 时限制 DrawText 内容日志条数（首屏定位用）。
var wbDrawLogCount int

// CanvasTimingSummary returns the accumulated cgo timing as a string.
func CanvasTimingSummary() string {
	return fmt.Sprintf("restore=%v save=%v clip=%v trans=%v draw=%v clipRR=%d saveLayer=%d/%v",
		CgoTimingRestore.Round(time.Microsecond), CgoTimingSave.Round(time.Microsecond),
		CgoTimingClip.Round(time.Microsecond), CgoTimingTrans.Round(time.Microsecond),
		CgoTimingDraw.Round(time.Microsecond), CanvasClipRRCount, CanvasSaveLayerCount,
		CgoTimingSaveLayer.Round(time.Microsecond))
}

// Translate composes a translation into the current transform, mirroring
// GraphicsContext::translate().
func (c *Canvas) Translate(dx, dy float64) {
	c.state.translateX += dx * c.state.scaleX
	c.state.translateY += dy * c.state.scaleY
	t0 := time.Now()
	c.canvas.Translate(float32(dx), float32(dy))
	CgoTimingTrans += time.Since(t0)
	c.invalidatePixels()
}

// Scale composes a non-uniform scale into the current transform, mirroring
// GraphicsContext::scale().
func (c *Canvas) Scale(sx, sy float64) {
	c.state.scaleX *= sx
	c.state.scaleY *= sy
	c.canvas.Scale(float32(sx), float32(sy))
	c.invalidatePixels()
}

// ResetFixedTransform keeps the current scale (device content scale) but
// drops every accumulated translate, so a fixed-position layer paints
// against the viewport instead of inheriting ancestor scroll offsets.
// The browser never scrolls a fixed element with its containing block, so
// both the page-level scroll translate and per-box overflow translates must
// be discarded here (clip is handled separately by ResetClip).
func (c *Canvas) ResetFixedTransform() {
	sx, sy := c.state.scaleX, c.state.scaleY
	c.state.translateX, c.state.translateY = 0, 0
	c.canvas.ResetMatrix()
	if os.Getenv("WB_CTM_DEBUG") != "" {
		m := c.canvas.GetMatrix()
		log.Printf("[ctm] ResetFixedTransform state=(%.3f,%.3f) afterReset sx=%.3f", sx, sy, m.ScaleX)
	}
	if sx != 1 || sy != 1 {
		c.canvas.Scale(float32(sx), float32(sy))
	}
	if os.Getenv("WB_CTM_DEBUG") != "" {
		m := c.canvas.GetMatrix()
		log.Printf("[ctm] ResetFixedTransform done sx=%.3f", m.ScaleX)
	}
	c.invalidatePixels()
}

// Rotate composes a rotation (clockwise degrees) into the current transform,
// mirroring GraphicsContext::rotate(). The rotation is about the current origin.
func (c *Canvas) Rotate(degrees float64) {
	c.canvas.Rotate(float32(degrees))
	c.invalidatePixels()
}

// Skew composes a skew transform into the current matrix, mirroring the
// CSS skew() transform function. sx is the X skew angle in degrees;
// sy is the Y skew angle in degrees.
func (c *Canvas) Skew(sx, sy float64) {
	c.canvas.Skew(float32(sx), float32(sy))
	c.invalidatePixels()
}

// Concat post-multiplies the current transform by the given matrix,
// mirroring GraphicsContext::concatCTM().
func (c *Canvas) Concat(m skia.Matrix) {
	c.canvas.Concat(m)
	c.invalidatePixels()
}

// SetMatrix replaces the current transform matrix with m,
// mirroring GraphicsContext::setCTM().
func (c *Canvas) SetMatrix(m skia.Matrix) {
	c.canvas.SetMatrix(m)
	c.invalidatePixels()
}

// GetMatrix returns the current total transform matrix,
// mirroring GraphicsContext::getCTM().
func (c *Canvas) GetMatrix() skia.Matrix {
	return c.canvas.GetMatrix()
}

// ResetMatrix sets the current transform to the identity matrix,
// mirroring GraphicsContext::resetTransform().
func (c *Canvas) ResetMatrix() {
	c.state.scaleX, c.state.scaleY = 1, 1
	c.state.translateX, c.state.translateY = 0, 0
	c.canvas.ResetMatrix()
	c.invalidatePixels()
}

// transform maps a world-space point to device space using the current transform.
func (c *Canvas) transform(wx, wy float64) (x, y float64) {
	return wx*c.state.scaleX + c.state.translateX,
		wy*c.state.scaleY + c.state.translateY
}

// DeviceRect maps a world-space rect to device space (current transform).
func (c *Canvas) DeviceRect(r Rect) Rect {
	x0, y0 := c.transform(r.X, r.Y)
	x1, y1 := c.transform(r.X+r.Width, r.Y+r.Height)
	return normalizeRect(Rect{X: math.Min(x0, x1), Y: math.Min(y0, y1),
		Width: math.Abs(x1 - x0), Height: math.Abs(y1 - y0)})
}

// Clip intersects the current clip with the given world-space rectangle, mirroring
// GraphicsContext::clip() for the rectangular case. The rectangle is transformed to
// device space and then intersected with any existing clip.
func (c *Canvas) Clip(r Rect) {
	x0, y0 := c.transform(r.X, r.Y)
	x1, y1 := c.transform(r.X+r.Width, r.Y+r.Height)
	screen := normalizeRect(Rect{X: math.Min(x0, x1), Y: math.Min(y0, y1),
		Width: math.Abs(x1 - x0), Height: math.Abs(y1 - y0)})
	if !c.state.hasClip {
		c.state.clip = screen
		c.state.hasClip = true
	} else {
		c.state.clip = intersectRect(c.state.clip, screen)
	}
	// Apply clip to Skia canvas in world coordinates (Skia handles the transform).
	sr := skia.RectXYWH(float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height))
	c.canvas.ClipRect(sr, skia.ClipOpIntersect, true)
	c.invalidatePixels()
}

// HasClip reports whether a clip is active.
func (c *Canvas) HasClip() bool { return c.state.hasClip }

// ResetClip replaces the current clip with the full canvas surface, discarding
// any ancestor clip. ⚠️ NOTE: this uses SkClipOp::kReplace (=5, non-standard)
// which works on raster but Skia's GPU backend turns into an EMPTY clip —
// fixed-position layers (dialogs/overlays) must NOT use this; paintLayerTree
// handles them via RestoreToCount(initialSaveCount) instead.
func (c *Canvas) ResetClip() {
	c.canvas.ClipRect(skia.RectXYWH(0, 0, float32(c.width), float32(c.height)), skia.ClipOpReplace, false)
	c.state.hasClip = false
	c.state.clip = Rect{}
	c.invalidatePixels()
}

// ClipRect returns the current device-space clip rectangle and whether one is set.
func (c *Canvas) ClipRect() (Rect, bool) { return c.state.clip, c.state.hasClip }

// ClipPath intersects the current clip with the given world-space path, mirroring
// GraphicsContext::clipPath(). The path is transformed by the current CTM and
// then intersected with any existing clip. Uses Skia's anti-aliased path clipping.
func (c *Canvas) ClipPath(path *skia.Path) {
	c.canvas.ClipPath(path, skia.ClipOpIntersect, true)
	c.state.hasClip = true
	c.invalidatePixels()
}

// ClipRoundRect intersects the current clip with a rounded rectangle
// (radius clamped to fit). Used to clip gradient fills to border-radius.
//
// ★ Standard Skia drawing: uses the native RRect clip (SkCanvas::clipRRect),
// NOT a hand-built quadratic Bézier path + ClipPath. Skia's GPU backend
// handles RRects analytically in its clip stack (no stencil buffer, no path
// tessellation), so the rounded clip is exact and reliable on both the
// raster and GPU paths — the corner geometry and anti-aliasing match Skia's
// own DrawRoundRect/FillRoundRect exactly.
func (c *Canvas) ClipRoundRect(x, y, w, h, radius float64) {
	CanvasClipRRCount++
	r := float64(math.Min(radius, math.Min(w/2, h/2)))
	if r <= 0 {
		c.Clip(Rect{X: x, Y: y, Width: w, Height: h})
		return
	}
	rr := skia.NewRRect()
	defer rr.Release()
	rr.SetRectXY(skia.RectXYWH(float32(x), float32(y), float32(w), float32(h)),
		float32(r), float32(r))
	t0 := time.Now()
	c.canvas.ClipRRect(rr, skia.ClipOpIntersect, true)
	CgoTimingClip += time.Since(t0)
	c.state.hasClip = true
	c.invalidatePixels()
}

// CanvasClipRRCount counts ClipRoundRect calls (debug: WB_PAINT_STATS).
var CanvasClipRRCount int

// FillRect fills the given world-space rectangle with the supplied color, mirroring
// GraphicsContext::fillRect(FloatRect, Color). The rectangle is drawn through the
// Skia canvas which applies the current transform, clip, and anti-aliasing.
func (c *Canvas) FillRect(x, y, w, h float64, col Color) {
	c.fillPaint.SetColor(colorToSkia(col))
	r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	c.canvas.DrawRect(r, c.fillPaint)
	c.invalidatePixels()
}

// FillRectNoAA fills a rectangle with anti-aliasing disabled. Used for
// pixel-exact bands (e.g. the rounded border taper bands): each 1×1 pixel is
// filled with an explicit alpha so the taper gradient itself is the only
// smoothing — the AA edge spread of DrawRect would otherwise bleed into
// neighbouring pixels and widen the band.
func (c *Canvas) FillRectNoAA(x, y, w, h float64, col Color) {
	c.fillPaint.SetColor(colorToSkia(col))
	c.fillPaint.SetAntialias(false)
	r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	c.canvas.DrawRect(r, c.fillPaint)
	c.fillPaint.SetAntialias(true)
	c.invalidatePixels()
}

// FillRoundRect fills a rounded rectangle with the supplied color, mirroring
// GraphicsContext::fillRoundedRect(). The corner radius is applied uniformly to
// all four corners. Used by PaintBackground when border-radius > 0.
func (c *Canvas) FillRoundRect(x, y, w, h, radius float64, col Color) {
	c.fillPaint.SetColor(colorToSkia(col))
	r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	rx := float32(radius)
	if rx < 0 {
		rx = 0
	}
	// Clamp radius to half the smaller dimension to avoid overdraw.
	half := float32(w) / 2
	if h < w {
		half = float32(h) / 2
	}
	if rx > half {
		rx = half
	}
	c.canvas.DrawRoundRect(r, rx, rx, c.fillPaint)
	c.invalidatePixels()
}

// StrokeRect strokes the outline of the given world-space rectangle with the supplied
// color and width, mirroring GraphicsContext::strokeRect(FloatRect, thickness). The
// stroke is drawn as four solid filled rectangles (one per side) inside the rect,
// matching WebKit's border painting semantics.
func (c *Canvas) StrokeRect(x, y, w, h, strokeWidth float64, col Color) {
	if strokeWidth <= 0 {
		return
	}
	half := strokeWidth / 2
	// Top and bottom spans cover the full width including the corners.
	c.FillRect(x-half, y-half, w+strokeWidth, strokeWidth, col)   // top
	c.FillRect(x-half, y+h-half, w+strokeWidth, strokeWidth, col) // bottom
	// Left and right spans exclude the corners already covered by top/bottom.
	c.FillRect(x-half, y+half, strokeWidth, h-strokeWidth, col)   // left
	c.FillRect(x+w-half, y+half, strokeWidth, h-strokeWidth, col) // right
}

// StrokeRoundRect strokes the outline of a rounded rectangle with the supplied color
// and width, mirroring GraphicsContext::strokeRoundedRect(). Unlike StrokeRect, the
// corners follow the supplied radius so CSS border-radius produces a rounded frame
// instead of sharp rectangular sides that would cover the rounded background. The
// stroke is centered on the path geometry; callers should inset by strokeWidth/2 so
// the entire border stays inside the border-box (matching CSS border painting).
func (c *Canvas) StrokeRoundRect(x, y, w, h, radius, strokeWidth float64, col Color) {
	if strokeWidth <= 0 {
		return
	}
	// Inset by half the stroke width so the stroke lies entirely within the
	// border-box rectangle, matching how CSS rasterizes borders (borders occupy
	// the space between the padding edge and the border edge).
	half := strokeWidth / 2
	rx := float32(radius)
	if rx < 0 {
		rx = 0
	}
	// Clamp radius to half the smaller inset dimension so the corner curves
	// do not overlap each other on tiny boxes.
	insetW := w - strokeWidth
	insetH := h - strokeWidth
	if insetW <= 0 || insetH <= 0 {
		return
	}
	halfMin := float32(insetW) / 2
	if insetH < insetW {
		halfMin = float32(insetH) / 2
	}
	if rx > halfMin {
		rx = halfMin
	}
	r := skia.RectXYWH(float32(x+half), float32(y+half), float32(insetW), float32(insetH))
	c.strokePaint.SetColor(colorToSkia(col))
	c.strokePaint.SetStrokeWidth(float32(strokeWidth))
	c.canvas.DrawRoundRect(r, rx, rx, c.strokePaint)
	c.invalidatePixels()
}

// FillLinearGradient fills the given world-space rectangle with a linear gradient
// from startColor to endColor, running top-to-bottom (0 degrees). This mirrors
// the CSS linear-gradient(to bottom, startColor, endColor) shorthand and is the
// most common gradient used in UI backgrounds.
func (c *Canvas) FillLinearGradient(x, y, w, h float64, startColor, endColor Color) {
	if w <= 0 || h <= 0 {
		return
	}
	start := skia.Point{X: float32(x), Y: float32(y)}
	end := skia.Point{X: float32(x), Y: float32(y + h)}
	colors := []skia.Color{colorToSkia(startColor), colorToSkia(endColor)}
	shader := skia.NewLinearGradient(start, end, colors, nil, skia.TileModeClamp)
	if shader == nil {
		return
	}
	defer shader.Release()
	c.gradientPaint.SetShader(shader)
	r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	c.canvas.DrawRect(r, c.gradientPaint)
	c.gradientPaint.SetShader(nil)
	c.invalidatePixels()
}

// FillRadialGradient fills a circle centered at (cx, cy) with a radial gradient
// from centerColor at the center to edgeColor at the edge. This mirrors the CSS
// radial-gradient(circle, centerColor, edgeColor) shorthand.
func (c *Canvas) FillRadialGradient(cx, cy, radius float64, centerColor, edgeColor Color) {
	if radius <= 0 {
		return
	}
	center := skia.Point{X: float32(cx), Y: float32(cy)}
	colors := []skia.Color{colorToSkia(centerColor), colorToSkia(edgeColor)}
	shader := skia.NewRadialGradient(center, float32(radius), colors, nil, skia.TileModeClamp)
	if shader == nil {
		return
	}
	defer shader.Release()
	c.gradientPaint.SetShader(shader)
	c.canvas.DrawCircle(float32(cx), float32(cy), float32(radius), c.gradientPaint)
	c.gradientPaint.SetShader(nil)
	c.invalidatePixels()
}

// FillCircle fills a circle centered at (cx, cy) with the given radius and color,
// mirroring GraphicsContext::fillEllipse(). Used by form-control painters (radio buttons,
// slider thumbs) and any code that needs a filled disc.
func (c *Canvas) FillCircle(cx, cy, radius float64, col Color) {
	if radius <= 0 || col.A == 0 {
		return
	}
	c.fillPaint.SetColor(colorToSkia(col))
	c.canvas.DrawCircle(float32(cx), float32(cy), float32(radius), c.fillPaint)
	c.invalidatePixels()
}

// StrokeCircle strokes the outline of a circle centered at (cx, cy) with the given
// radius, stroke width and color, mirroring GraphicsContext::strokeEllipse().
func (c *Canvas) StrokeCircle(cx, cy, radius, strokeWidth float64, col Color) {
	if radius <= 0 || strokeWidth <= 0 || col.A == 0 {
		return
	}
	c.strokePaint.SetColor(colorToSkia(col))
	c.strokePaint.SetStrokeWidth(float32(strokeWidth))
	c.canvas.DrawCircle(float32(cx), float32(cy), float32(radius), c.strokePaint)
	c.invalidatePixels()
}

// StrokeLine draws a single line segment from (x0, y0) to (x1, y1) with the given
// stroke width and color, mirroring GraphicsContext::drawLine(). Used by the checkbox
// checkmark painter and any line-based decoration.
func (c *Canvas) StrokeLine(x0, y0, x1, y1, strokeWidth float64, col Color) {
	if strokeWidth <= 0 || col.A == 0 {
		return
	}
	c.strokePaint.SetColor(colorToSkia(col))
	c.strokePaint.SetStrokeWidth(float32(strokeWidth))
	c.canvas.DrawLine(float32(x0), float32(y0), float32(x1), float32(y1), c.strokePaint)
	c.invalidatePixels()
}

// FillPath fills a closed polygon/polyline point list. evenOdd selects the
// SVG fill-rule: evenodd vs nonzero (winding). Uses the native Skia path
// fill so concave and self-intersecting paths rasterize correctly (the
// triangle-fan fallback paints the wrong interior for concave shapes).
func (c *Canvas) FillPath(pts []Point, col Color, evenOdd bool) {
	if col.A == 0 || len(pts) < 3 {
		return
	}
	path := skia.NewPath()
	defer path.Release()
	path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		path.LineTo(float32(pts[i].X), float32(pts[i].Y))
	}
	path.Close()
	if evenOdd {
		path.SetFillType(skia.FillTypeEvenOdd)
	}
	c.fillPaint.SetColor(colorToSkia(col))
	c.canvas.DrawPath(path, c.fillPaint)
	c.invalidatePixels()
}

// StrokePath strokes a polyline with the given width, color and SVG
// stroke-linecap / stroke-linejoin values (butt|round|square, miter|round|
// bevel). Uses the native Skia stroke so caps and joins match the browser.
func (c *Canvas) StrokePath(pts []Point, strokeWidth float64, col Color, cap, join string) {
	if col.A == 0 || strokeWidth <= 0 || len(pts) < 2 {
		return
	}
	path := skia.NewPath()
	defer path.Release()
	path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		path.LineTo(float32(pts[i].X), float32(pts[i].Y))
	}
	c.strokePaint.SetColor(colorToSkia(col))
	c.strokePaint.SetStrokeWidth(float32(strokeWidth))
	switch cap {
	case "round":
		c.strokePaint.SetStrokeCap(skia.StrokeCapRound)
	case "square":
		c.strokePaint.SetStrokeCap(skia.StrokeCapSquare)
	default:
		c.strokePaint.SetStrokeCap(skia.StrokeCapButt)
	}
	switch join {
	case "round":
		c.strokePaint.SetStrokeJoin(skia.StrokeJoinRound)
	case "bevel":
		c.strokePaint.SetStrokeJoin(skia.StrokeJoinBevel)
	default:
		c.strokePaint.SetStrokeJoin(skia.StrokeJoinMiter)
	}
	c.canvas.DrawPath(path, c.strokePaint)
	c.invalidatePixels()
}

// FillPathGradient fills a closed polygon/polyline with a linear gradient
// running from (gx1,gy1) to (gx2,gy2) in world space (SVG linearGradient axis
// resolved against the shape's bounding box by the caller). colors/positions
// follow the Skia shader convention (positions nil = evenly spaced stops).
// Used for SVG fill="url(#gradient)" on <path> shapes, which the flat
// paintGradientOnShape rect-path cannot cover.
func (c *Canvas) FillPathGradient(pts []Point, gx1, gy1, gx2, gy2 float64, colors []Color, positions []float32, evenOdd bool) {
	if len(pts) < 3 || len(colors) < 2 {
		return
	}
	path := skia.NewPath()
	defer path.Release()
	path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		path.LineTo(float32(pts[i].X), float32(pts[i].Y))
	}
	path.Close()
	if evenOdd {
		path.SetFillType(skia.FillTypeEvenOdd)
	}
	start := skia.Point{X: float32(gx1), Y: float32(gy1)}
	end := skia.Point{X: float32(gx2), Y: float32(gy2)}
	sk := make([]skia.Color, len(colors))
	for i, col := range colors {
		sk[i] = colorToSkia(col)
	}
	shader := skia.NewLinearGradient(start, end, sk, positions, skia.TileModeClamp)
	if shader == nil {
		return
	}
	defer shader.Release()
	c.gradientPaint.SetShader(shader)
	c.canvas.DrawPath(path, c.gradientPaint)
	c.gradientPaint.SetShader(nil)
	c.invalidatePixels()
}

// StrokePathGradient strokes a polyline with a linear gradient running from
// (gx1,gy1) to (gx2,gy2) in world space, honoring SVG stroke-linecap /
// stroke-linejoin. Used for SVG stroke="url(#gradient)" (e.g. the IDE logo's
// gradient-bracketed <path> strokes), which flat-color StrokePath cannot do.
func (c *Canvas) StrokePathGradient(pts []Point, strokeWidth float64, gx1, gy1, gx2, gy2 float64, colors []Color, positions []float32, cap, join string) {
	if strokeWidth <= 0 || len(colors) < 2 || len(pts) < 2 {
		return
	}
	path := skia.NewPath()
	defer path.Release()
	path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		path.LineTo(float32(pts[i].X), float32(pts[i].Y))
	}
	start := skia.Point{X: float32(gx1), Y: float32(gy1)}
	end := skia.Point{X: float32(gx2), Y: float32(gy2)}
	sk := make([]skia.Color, len(colors))
	for i, col := range colors {
		sk[i] = colorToSkia(col)
	}
	shader := skia.NewLinearGradient(start, end, sk, positions, skia.TileModeClamp)
	if shader == nil {
		return
	}
	defer shader.Release()
	c.gradientPaint.SetShader(shader)
	c.gradientPaint.SetStyle(skia.PaintStyleStroke)
	c.gradientPaint.SetStrokeWidth(float32(strokeWidth))
	switch cap {
	case "round":
		c.gradientPaint.SetStrokeCap(skia.StrokeCapRound)
	case "square":
		c.gradientPaint.SetStrokeCap(skia.StrokeCapSquare)
	default:
		c.gradientPaint.SetStrokeCap(skia.StrokeCapButt)
	}
	switch join {
	case "round":
		c.gradientPaint.SetStrokeJoin(skia.StrokeJoinRound)
	case "bevel":
		c.gradientPaint.SetStrokeJoin(skia.StrokeJoinBevel)
	default:
		c.gradientPaint.SetStrokeJoin(skia.StrokeJoinMiter)
	}
	c.canvas.DrawPath(path, c.gradientPaint)
	c.gradientPaint.SetShader(nil)
	c.gradientPaint.SetStyle(skia.PaintStyleFill)
	c.invalidatePixels()
}

// FillTriangle fills the triangle defined by three points with the supplied color.
// Used by the select dropdown arrow painter. The triangle is built as a Skia path
// (MoveTo + 2x LineTo + Close) and filled.
func (c *Canvas) FillTriangle(x0, y0, x1, y1, x2, y2 float64, col Color) {
	if col.A == 0 {
		return
	}
	path := skia.NewPath()
	path.MoveTo(float32(x0), float32(y0))
	path.LineTo(float32(x1), float32(y1))
	path.LineTo(float32(x2), float32(y2))
	path.Close()
	c.fillPaint.SetColor(colorToSkia(col))
	c.canvas.DrawPath(path, c.fillPaint)
	c.invalidatePixels()
}

// FillRoundedTriangle fills a triangle with rounded corners using QuadTo bezier
// curves at each vertex. radius controls the corner rounding amount.
func (c *Canvas) FillRoundedTriangle(x0, y0, x1, y1, x2, y2, radius float64, col Color) {
	if col.A == 0 || radius <= 0 {
		c.FillTriangle(x0, y0, x1, y1, x2, y2, col)
		return
	}
	// Clamp radius to half the shortest edge.
	type pt struct{ x, y float64 }
	v := []pt{{x0, y0}, {x1, y1}, {x2, y2}}
	minHalfEdge := radius
	for i := 0; i < 3; i++ {
		j := (i + 1) % 3
		dx := v[j].x - v[i].x
		dy := v[j].y - v[i].y
		half := float64(math.Sqrt(float64(dx*dx+dy*dy))) / 2.0
		if half < minHalfEdge {
			minHalfEdge = half
		}
	}
	r := float32(math.Min(float64(radius), float64(minHalfEdge)))
	if r <= 0 {
		c.FillTriangle(x0, y0, x1, y1, x2, y2, col)
		return
	}

	path := skia.NewPath()
	// For each vertex, find the point r distance along each incident edge.
	// Build the path: start at first edge-end of vertex 0, then for each vertex
	// do QuadTo(vertex, next-edge-start) + LineTo(next-edge-end).
	var starts, ends [3]skia.Point
	for i := 0; i < 3; i++ {
		prev := (i + 2) % 3
		next := (i + 1) % 3
		// Edge from vertex i to vertex next.
		dx1 := v[next].x - v[i].x
		dy1 := v[next].y - v[i].y
		l1 := float32(math.Sqrt(float64(dx1*dx1 + dy1*dy1)))
		// Edge from vertex prev to vertex i.
		dx2 := v[i].x - v[prev].x
		dy2 := v[i].y - v[prev].y
		l2 := float32(math.Sqrt(float64(dx2*dx2 + dy2*dy2)))
		if l1 <= 0 || l2 <= 0 {
			c.FillTriangle(x0, y0, x1, y1, x2, y2, col)
			path.Release()
			return
		}
		// Point along edge prev->i at distance r from vertex i.
		starts[i] = skia.Point{
			X: float32(v[i].x) - r*float32(dx2)/l2,
			Y: float32(v[i].y) - r*float32(dy2)/l2,
		}
		// Point along edge i->next at distance r from vertex i.
		ends[i] = skia.Point{
			X: float32(v[i].x) + r*float32(dx1)/l1,
			Y: float32(v[i].y) + r*float32(dy1)/l1,
		}
	}
	path.MoveTo(starts[0].X, starts[0].Y)
	for i := 0; i < 3; i++ {
		path.QuadTo(float32(v[i].x), float32(v[i].y), ends[i].X, ends[i].Y)
		next := (i + 1) % 3
		path.LineTo(starts[next].X, starts[next].Y)
	}
	path.Close()
	c.fillPaint.SetColor(colorToSkia(col))
	c.canvas.DrawPath(path, c.fillPaint)
	c.invalidatePixels()
	path.Release()
}

// FillPolygon fills a closed polygon defined by the given points with the supplied
// color. Used for arbitrary filled shapes (e.g. the select arrow). Points are connected
// in order and the path is closed.
func (c *Canvas) FillPolygon(pts []Point, col Color) {
	if len(pts) < 3 || col.A == 0 {
		return
	}
	skPts := make([]skia.Point, len(pts))
	for i, p := range pts {
		skPts[i] = skia.Point{X: float32(p.X), Y: float32(p.Y)}
	}
	path := skia.NewPath()
	path.AddPoly(skPts, true)
	c.fillPaint.SetColor(colorToSkia(col))
	c.canvas.DrawPath(path, c.fillPaint)
	c.invalidatePixels()
}

// Point is an (x, y) pair used by FillPolygon.
type Point struct {
	X, Y float64
}

// DrawText renders text at the given world-space baseline origin using the supplied
// font and color, mirroring GraphicsContext::drawText() (via FontCascade::drawText).
// Uses Skia's real font rasterizer for proper glyph outlines, hinting, and
// anti-aliasing. Emoji and symbol characters outside ASCII are automatically
// rendered using fallback fonts (Segoe UI Emoji / Segoe UI Symbol) when the
// primary font lacks the glyph.
func (c *Canvas) DrawText(x, y float64, text string, font Font, col Color) {
	if col.A == 0 || len(text) == 0 {
		return
	}
	if os.Getenv("WB_TEXT_DEBUG") != "" && wbDrawLogCount < 30 {
		log.Printf("[drawtext] %q at (%.0f,%.0f) family=%q size=%.1f", text, x, y, font.Family, font.Size)
		wbDrawLogCount++
	}
	skFont := c.getSkiaFont(font)
	if skFont == nil {
		return
	}
	c.fillPaint.SetColor(colorToSkia(col))

	// Always use fallback path when text contains non-ASCII characters,
	// since CJK fallback fonts often lack geometric/dingbat symbols.
	if containsNonASCII(text) {
		t0 := time.Now()
		c.drawTextWithFallback(x, y, text, font, skFont, col)
		CgoTimingDraw += time.Since(t0)
		c.invalidatePixels()
		return
	}

	t0 := time.Now()
	c.canvas.DrawText(text, float32(x), float32(y), skFont, c.fillPaint)
	CgoTimingDraw += time.Since(t0)
	c.invalidatePixels()
}

// drawTextWithFallback splits text into runs by character type (emoji, symbol,
// plain) and draws each run with the appropriate font. Plain ASCII uses the
// primary font. Emoji uses the emoji fallback font. Symbols (geometric shapes,
// arrows, etc.) try the primary font first using UnicharToGlyph; if it lacks
// the glyph, fall back to the symbol font (Segoe UI Symbol), then to emoji.
func (c *Canvas) drawTextWithFallback(x, y float64, text string, font Font, primarySkFont *skia.Font, col Color) {
	emojiSkFont := c.getEmojiSkiaFont(font)
	symbolSkFont := c.getSymbolSkiaFont(font)
	cjkSkFont := c.getCJKSkiaFont(font)
	runes := []rune(text)
	cx := float32(x)
	i := 0
	for i < len(runes) {
		start := i
		rtype := classifyRune(runes[i])
		for i < len(runes) && classifyRune(runes[i]) == rtype {
			i++
		}
		seg := string(runes[start:i])
		segFont := primarySkFont
		switch rtype {
		case runeEmoji:
			// Emoji: always use emoji font if available.
			if emojiSkFont != nil {
				segFont = emojiSkFont
			}
		case runeSymbol:
			// Symbols: check primary font first. If it lacks the glyph, try
			// symbol font, then emoji font as last resort.
			if primarySkFont.UnicharToGlyph(runes[start]) == 0 {
				if symbolSkFont != nil {
					segFont = symbolSkFont
				} else if emojiSkFont != nil {
					segFont = emojiSkFont
				}
			}
		case runeCJK:
			// CJK: the primary font (e.g. Consolas) usually lacks Chinese
			// glyphs — use the OS CJK font (Microsoft YaHei) which keeps
			// Skia's system fallback. This is what was missing before, so
			// Chinese rendered as tofu boxes.
			if primarySkFont.UnicharToGlyph(runes[start]) == 0 && cjkSkFont != nil {
				segFont = cjkSkFont
			}
		}
		c.canvas.DrawText(seg, cx, float32(y), segFont, c.fillPaint)
		if w, _ := segFont.MeasureText(seg, c.fillPaint); w > 0 {
			cx += w
		}
	}
}

// getCJKSkiaFont returns a *skia.Font using the OS CJK Typeface (Microsoft
// YaHei) at the same size as font, or nil if none available.
func (c *Canvas) getCJKSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil {
		return nil
	}
	return c.makeSkiaFont(mgr.CJKTypeface(), font.Size)
}

// FontAscent returns the ascent (distance from baseline up to the top of the
// font's recommended line box) of the given font description, in pixels. This
// converts a text-box top coordinate to the baseline coordinate that DrawText
// expects. If the font cannot be loaded, falls back to size * 0.8.
func (c *Canvas) FontAscent(font Font) float64 {
	skFont := c.getSkiaFont(font)
	if skFont == nil {
		size := font.Size
		if size <= 0 {
			size = 16
		}
		return size * 0.8
	}
	m, _ := skFont.Metrics()
	return float64(-m.Ascent)
}

// FontCapHeight returns the cap height (distance from baseline up to the top
// of capital letters) of the given font description, in pixels. Browsers
// position Latin text so its cap top sits near the line-box top
// (baseline = half-leading + ascent, and ascent ≈ capHeight for most
// Latin typefaces). Using capHeight for the baseline keeps our text at the
// same vertical spot as Edge/Chrome. Falls back to ascent (size*0.8) when
// the font reports no cap height (e.g. some CJK fonts).
func (c *Canvas) FontCapHeight(font Font) float64 {
	skFont := c.getSkiaFont(font)
	if skFont == nil {
		size := font.Size
		if size <= 0 {
			size = 16
		}
		return size * 0.8
	}
	m, _ := skFont.Metrics()
	if m.CapHeight > 0 {
		return float64(m.CapHeight)
	}
	return float64(-m.Ascent)
}

// FontCJKMetrics returns the ascent/descent (px) of the CJK fallback
// typeface (Microsoft YaHei) at the given font's size. CJK glyphs are
// full-em squares drawn by the CJK font regardless of the CSS family, so
// their baseline must be positioned from the CJK font's own metrics
// (browser line-box rule: baseline = lineTop + halfLeading + ascent). The
// painter uses this instead of capHeight for segments that contain CJK
// characters, because capHeight (OS/2 sCapHeight) is far smaller than the
// distance from baseline up to the top of a CJK glyph (~0.85em), which
// pushed glyph tops above the line box and clipped them.
func (c *Canvas) FontCJKMetrics(font Font) (ascent, descent float64) {
	skFont := c.getCJKSkiaFont(font)
	if skFont == nil {
		skFont = c.getSkiaFont(font)
	}
	if skFont == nil {
		size := font.Size
		if size <= 0 {
			size = 16
		}
		return size * 0.8, size * 0.2
	}
	m, _ := skFont.Metrics()
	return float64(-m.Ascent), float64(m.Descent)
}

// getSkiaFont returns a cached *skia.Font matching the given graphics.Font
// description, creating one on first use.
func (c *Canvas) getSkiaFont(font Font) *skia.Font {
	size := font.Size
	if size <= 0 {
		size = 16
	}
	key := fontKey{
		family: font.Family,
		size:   float32(size),
		weight: font.Weight,
		style:  font.Style,
	}
	c.fontCacheMu.Lock()
	defer c.fontCacheMu.Unlock()
	if f, ok := c.fontCache[key]; ok {
		return f
	}

	weight := font.Weight
	if weight == 0 {
		weight = 400
	}

	// Prefer the FontManager (Typefaces loaded from font files shipped from
	// WebKit). It resolves generic families and provides CJK coverage. Only
	// fall back to the platform name lookup when no FontManager is configured
	// or it has no matching Typeface.
	mgr := GetFontManager()
	var tf *skia.Typeface
	if mgr != nil {
		tf = mgr.LookupTypeface(font.Family, weight, font.Style)
	}
	if tf == nil {
		slant := skia.FontSlantUpright
		switch font.Style {
		case "italic":
			slant = skia.FontSlantItalic
		case "oblique":
			slant = skia.FontSlantOblique
		}
		fs := skia.FontStyle{Weight: weight, Width: 5, Slant: slant}
		tf = skia.NewTypeface(font.Family, fs)
	}
	if tf == nil {
		tf = skia.DefaultTypeface()
	}
	if tf == nil {
		return nil
	}
	f := skia.NewFont(tf, float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	// Apply synthetic bold (faux bold) only when the requested weight is
	// semibold or heavier AND the resolved Typeface is not already a real
	// bold face. When a native bold Typeface is loaded (e.g. Microsoft YaHei
	// Bold), TypefaceWeight returns >= 600 and we skip SetEmbolden to avoid
	// double-bolding. When only weight-400 faces are available, the fallback
	// TypefaceWeight returns 400 and SetEmbolden makes the text bold.
	if weight >= 600 && (mgr == nil || mgr.TypefaceWeight(tf) < 600) {
		f.SetEmbolden(true)
	}
	// Apply synthetic italic (faux italic) via SkewX when the requested style
	// is italic/oblique but the resolved Typeface is not actually slanted.
	// Many CJK fonts (Microsoft YaHei, NSimSun, etc.) lack an italic variant,
	// so the FontManager returns a regular face. In that case we shear the
	// glyphs (-0.2 radians �?11.3°) to simulate italic �?matching the behavior
	// of browsers that apply font-style: italic to non-italic fonts.
	if (font.Style == "italic" || font.Style == "oblique") && (mgr == nil || !mgr.TypefaceIsItalic(tf)) {
		f.SetSkewX(-0.2)
	}
	c.fontCache[key] = f
	return f
}

// getEmojiSkiaFont returns a *skia.Font using the emoji Typeface (Segoe UI
// Emoji / Noto Color Emoji) at the same size as font, or nil if no emoji
// font is available.
func (c *Canvas) getEmojiSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil || mgr.EmojiTypeface() == nil {
		return nil
	}
	return c.makeSkiaFont(mgr.EmojiTypeface(), font.Size)
}

// makeSkiaFont creates a *skia.Font from a Typeface and size, sharing the
// cache logic of getSkiaFont but with a given Typeface.
func (c *Canvas) makeSkiaFont(tf *skia.Typeface, size float64) *skia.Font {
	if tf == nil {
		return nil
	}
	if size <= 0 {
		size = 16
	}
	f := skia.NewFont(tf, float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	return f
}

// getSymbolSkiaFont returns a *skia.Font using the symbol Typeface (Segoe UI
// Symbol) at the same size as font, or nil if no symbol font is available.
func (c *Canvas) getSymbolSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil || mgr.SymbolTypeface() == nil {
		return nil
	}
	return c.makeSkiaFont(mgr.SymbolTypeface(), font.Size)
}

// runeClass categorizes a Unicode rune for font fallback purposes.// runeClass categorizes a Unicode rune for font fallback purposes.
type runeClass int

const (
	runeASCII  runeClass = iota // ASCII printable (0x20-0x7E)
	runeCJK                     // CJK ideographs (Chinese/Japanese/Korean)
	runeEmoji                   // Emoji / SMP symbols
	runeSymbol                  // Other non-ASCII symbols (geometric shapes, arrows, etc.)
)

// classifyRune categorizes r for font fallback routing.
func classifyRune(r rune) runeClass {
	if r >= 0x20 && r <= 0x7E {
		return runeASCII
	}
	// CJK ideographs: Unified (0x4E00-0x9FFF), Ext A (0x3400-0x4DBF),
	// Compatibility (0xF900-0xFAFF), plus the CJK punctuation/radicals.
	switch {
	case r >= 0x3400 && r <= 0x4DBF, // CJK Ext A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility
		r >= 0x3000 && r <= 0x303F, // CJK Symbols and Punctuation
		r >= 0xFF00 && r <= 0xFFEF: // Fullwidth forms
		return runeCJK
	}
	if r > 0xFFFF {
		// Supplementary Multilingual Plane: emoji and SMP symbols
		return runeEmoji
	}
	// Emoji and common symbol ranges
	switch {
	case r >= 0x2190 && r <= 0x21FF: // Arrows
		return runeEmoji
	case r >= 0x2300 && r <= 0x23FF: // Miscellaneous Technical
		return runeEmoji
	case r >= 0x2400 && r <= 0x243F: // Control Pictures
		return runeEmoji
	case r >= 0x2440 && r <= 0x245F: // OCR
		return runeEmoji
	case r >= 0x2460 && r <= 0x24FF: // Enclosed Alphanumerics
		return runeEmoji
	case r >= 0x2500 && r <= 0x257F: // Box Drawing
		return runeSymbol
	case r >= 0x2580 && r <= 0x259F: // Block Elements
		return runeSymbol
	case r >= 0x25A0 && r <= 0x25FF: // Geometric Shapes (▼ U+25BC, ▶ U+25B6)
		return runeSymbol
	case r >= 0x2600 && r <= 0x27BF: // Miscellaneous Symbols, Dingbats
		return runeEmoji
	case r >= 0x2930 && r <= 0x2BFF: // Supplemental Arrows, Various Symbols
		return runeEmoji
	case r == 0x200D || r == 0xFE0F: // ZWJ, Variation Selector
		return runeEmoji
	}
	return runeSymbol
}

// containsNonASCII reports whether text contains any character outside ASCII
// printable range (0x20-0x7E).
func containsNonASCII(text string) bool {
	for _, r := range text {
		if r > 0x7E || (r < 0x20 && r != '\n' && r != '\t') {
			return true
		}
	}
	return false
}
// ImageBuffer::getImageData() for a single pixel. Out-of-range reads return transparent
// black.
func (c *Canvas) PixelAt(px, py int) Color {
	if px < 0 || py < 0 || px >= c.width || py >= c.height {
		return Color{}
	}
	pix := c.ensurePixels()
	if pix == nil {
		return Color{}
	}
	idx := (py*c.width + px) * 4
	if idx+3 >= len(pix) {
		return Color{}
	}
	return Color{R: pix[idx], G: pix[idx+1], B: pix[idx+2], A: pix[idx+3]}
}

// Clear fills the entire canvas with the supplied color, mirroring
// GraphicsContext::clearRect() over the whole surface. Embedders painting
// directly onto a GPU window surface use this to clear the surface with the
// body background color before rendering the tree (so areas outside body,
// e.g. the head margin, still show the propagated background color instead
// of transparent/black).
func (c *Canvas) Clear(col Color) {
	c.canvas.Clear(colorToSkia(col))
	c.invalidatePixels()
}

// ClearRect sets the given device-space rectangle to fully transparent, mirroring
// GraphicsContext::clearRect(). Uses Skia's BlendModeClear to erase pixels.
func (c *Canvas) ClearRect(x, y, w, h float64) {
	r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	c.canvas.DrawRect(r, c.clearPaint)
	c.invalidatePixels()
}

// --- helpers ---

// colorToSkia converts a graphics.Color to a skia.Color (0xAARRGGBB).
func colorToSkia(c Color) skia.Color {
	return skia.RGBA(c.R, c.G, c.B, c.A)
}

// normalizeRect returns r with non-negative width/height.
func normalizeRect(r Rect) Rect {
	if r.Width < 0 {
		r.X += r.Width
		r.Width = -r.Width
	}
	if r.Height < 0 {
		r.Y += r.Height
		r.Height = -r.Height
	}
	return r
}

// intersectRect returns the intersection of a and b, or a zero rect if they are
// disjoint.
func intersectRect(a, b Rect) Rect {
	x0 := math.Max(a.X, b.X)
	y0 := math.Max(a.Y, b.Y)
	x1 := math.Min(a.X+a.Width, b.X+b.Width)
	y1 := math.Min(a.Y+a.Height, b.Y+b.Height)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

// Snapshot returns a *skia.Image capturing the current surface contents. The
// returned image is a snapshot (immutable); subsequent draws to the canvas do not
// affect it. The caller must Release the image when done. Exposed so embedders
// can blit the canvas to a GPU-backed window surface.
func (c *Canvas) Snapshot() *skia.Image {
	return c.surface.Snapshot()
}

// Release frees the Skia resources backing this canvas. After Release the canvas
// must not be used. If the canvas wraps an externally-owned surface (e.g. a GPU
// window surface), the surface itself is not freed.
// DrawImage draws an skia.Image at the given position and size.
// Mirrors GraphicsContext::drawImage(). The image is scaled to fill the
// destination rect (w, h) preserving no aspect ratio.
func (c *Canvas) DrawImage(img *skia.Image, x, y, w, h float64) {
	if img == nil || c.canvas == nil {
		return
	}
	src := skia.RectXYWH(0, 0, float32(img.Width()), float32(img.Height()))
	dst := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
	paint := skia.NewPaint()
	paint.SetStyle(skia.PaintStyleFill)
	paint.SetAntialias(true)
	defer paint.Release()
	c.canvas.DrawImageRect(img, src, dst, skia.SamplingLinear, paint)
}

// Release frees Skia resources held by this Canvas.
func (c *Canvas) Release() {
	if c.fillPaint != nil {
		c.fillPaint.Release()
		c.fillPaint = nil
	}
	if c.strokePaint != nil {
		c.strokePaint.Release()
		c.strokePaint = nil
	}
	if c.clearPaint != nil {
		c.clearPaint.Release()
		c.clearPaint = nil
	}
	if c.gradientPaint != nil {
		c.gradientPaint.Release()
		c.gradientPaint = nil
	}
	if c.surface != nil && !c.external {
		c.surface.Release()
	}
	c.surface = nil
}

// --- Global text measurement (used by the layout package via hooks) ---

// globalFontCache caches *skia.Font by font description for the package-level
// measurement helpers, mirroring Canvas.fontCache but usable without a Canvas.
var (
	globalFontCache          = make(map[fontKey]*skia.Font)
	globalFontCacheMu        sync.Mutex
	globalMeasurePaintCached *skia.Paint
	globalMeasureOnce        sync.Once
)

// globalWidthCache caches MeasureText results by (font, text). Layout calls
// MeasureTextFunc for every inline text segment every layout pass; chat /
// IDE UIs are full of repeated strings (buttons, labels, identical message
// fragments), so caching the Skia advance-width avoids re-running the
// rasterizer's text measurement for the same string thousands of times per
// frame. The cache is bounded: past the cap it is rebuilt from scratch
// (cheaper than unbounded memory growth; a rebuild costs one layout pass of
// misses, which is still far cheaper than measuring every frame).
type widthCacheKey struct {
	font fontKey
	text string
}

var (
	globalWidthCache     = make(map[widthCacheKey]float64)
	globalWidthCacheMu   sync.Mutex
	globalWidthCacheMax  = 8192
	globalWidthCacheHits, globalWidthCacheMisses int64
)

// globalMeasurePaintInstance lazily creates a shared Paint for text measurement.
func globalMeasurePaintInstance() *skia.Paint {
	globalMeasureOnce.Do(func() {
		globalMeasurePaintCached = skia.NewPaint()
		globalMeasurePaintCached.SetStyle(skia.PaintStyleFill)
		globalMeasurePaintCached.SetAntialias(true)
	})
	return globalMeasurePaintCached
}

// globalSkiaFont returns a cached *skia.Font for the given font description,
// mirroring Canvas.getSkiaFont but without requiring a Canvas instance.
func globalSkiaFont(font Font) *skia.Font {
	size := font.Size
	if size <= 0 {
		size = 16
	}
	key := fontKey{
		family: font.Family,
		size:   float32(size),
		weight: font.Weight,
		style:  font.Style,
	}
	globalFontCacheMu.Lock()
	defer globalFontCacheMu.Unlock()
	if f, ok := globalFontCache[key]; ok {
		return f
	}
	weight := font.Weight
	if weight == 0 {
		weight = 400
	}
	mgr := GetFontManager()
	var tf *skia.Typeface
	if mgr != nil {
		tf = mgr.LookupTypeface(font.Family, weight, font.Style)
	}
	if tf == nil {
		slant := skia.FontSlantUpright
		switch font.Style {
		case "italic":
			slant = skia.FontSlantItalic
		case "oblique":
			slant = skia.FontSlantOblique
		}
		fs := skia.FontStyle{Weight: weight, Width: 5, Slant: slant}
		tf = skia.NewTypeface(font.Family, fs)
	}
	if tf == nil {
		tf = skia.DefaultTypeface()
	}
	if tf == nil {
		return nil
	}
	f := skia.NewFont(tf, float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	// Apply synthetic bold only when no real bold Typeface was resolved.
	if weight >= 600 && (mgr == nil || mgr.TypefaceWeight(tf) < 600) {
		f.SetEmbolden(true)
	}
	// Apply synthetic italic when requested but Typeface isn't slanted.
	if (font.Style == "italic" || font.Style == "oblique") && (mgr == nil || !mgr.TypefaceIsItalic(tf)) {
		f.SetSkewX(-0.2)
	}
	globalFontCache[key] = f
	return f
}

// MeasureText returns the advance width of text rendered with the given font,
// using Skia's real font rasterizer. Exposed for the layout package to measure
// text width before painting (via layout.MeasureTextFunc). Returns 0 when the
// font cannot be loaded. Non-ASCII characters (emoji, symbols, geometric
// shapes) are measured using the emoji fallback font when the primary font
// lacks the glyph. Results are memoized in globalWidthCache (bounded) so
// repeated strings across layout passes cost one map lookup instead of a
// Skia rasterizer measurement.
func MeasureText(font Font, text string) float64 {
	if text == "" {
		return 0
	}
	sz := font.Size
	if sz <= 0 {
		sz = 16
	}
	key := widthCacheKey{font: fontKey{family: font.Family, size: float32(sz), weight: font.Weight, style: font.Style}, text: text}
	globalWidthCacheMu.Lock()
	if w, ok := globalWidthCache[key]; ok {
		globalWidthCacheHits++
		globalWidthCacheMu.Unlock()
		return w
	}
	globalWidthCacheMisses++
	globalWidthCacheMu.Unlock()

	w := measureTextUncached(font, text)

	globalWidthCacheMu.Lock()
	if len(globalWidthCache) >= globalWidthCacheMax {
		// Rebuild from scratch at the cap: bounded memory, and a one-pass
		// rebuild (misses) is far cheaper than measuring every segment every
		// frame. Rebuilding on overflow amortizes well for long-running
		// sessions with unbounded user text.
		globalWidthCache = make(map[widthCacheKey]float64, 512)
		globalWidthCacheHits, globalWidthCacheMisses = 0, 0
	}
	globalWidthCache[key] = w
	globalWidthCacheMu.Unlock()
	return w
}

func measureTextUncached(font Font, text string) float64 {
	skFont := globalSkiaFont(font)
	if skFont == nil {
		return 0
	}
	if !containsNonASCII(text) {
		w, _ := skFont.MeasureText(text, globalMeasurePaintInstance())
		return float64(w)
	}
	// Non-ASCII present: measure segment by segment with the correct font.
	emojiSkFont := globalEmojiSkiaFont(font)
	symbolSkFont := globalSymbolSkiaFont(font)
	cjkSkFont := globalCJKSkiaFont(font)
	total := float64(0)
	runes := []rune(text)
	i := 0
	paint := globalMeasurePaintInstance()
	for i < len(runes) {
		start := i
		rtype := classifyRune(runes[i])
		for i < len(runes) && classifyRune(runes[i]) == rtype {
			i++
		}
		seg := string(runes[start:i])
		f := skFont
		switch rtype {
		case runeEmoji:
			if emojiSkFont != nil {
				f = emojiSkFont
			}
		case runeSymbol:
			// If primary font lacks the glyph, use symbol font.
			if skFont.UnicharToGlyph(runes[start]) == 0 {
				if symbolSkFont != nil {
					f = symbolSkFont
				} else if emojiSkFont != nil {
					f = emojiSkFont
				}
			}
		case runeCJK:
			// If primary font (e.g. Consolas) lacks the CJK glyph, measure
			// with the OS CJK font (Microsoft YaHei) so layout matches the
			// painted glyph width.
			if skFont.UnicharToGlyph(runes[start]) == 0 && cjkSkFont != nil {
				f = cjkSkFont
			}
		}
		if w, _ := f.MeasureText(seg, paint); w > 0 {
			total += float64(w)
		}
	}
	return total
}

// globalEmojiSkiaFont returns a cached *skia.Font using the emoji Typeface at
// the given font size, or nil if no emoji font is loaded.
func globalEmojiSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil || mgr.EmojiTypeface() == nil {
		return nil
	}
	size := font.Size
	if size <= 0 {
		size = 16
	}
	key := fontKey{family: "_emoji", size: float32(size), weight: 400}
	globalFontCacheMu.Lock()
	defer globalFontCacheMu.Unlock()
	if f, ok := globalFontCache[key]; ok {
		return f
	}
	f := skia.NewFont(mgr.EmojiTypeface(), float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	globalFontCache[key] = f
	return f
}

// globalSymbolSkiaFont returns a cached *skia.Font using the symbol Typeface
// (Segoe UI Symbol) at the given font size, or nil if not available.
func globalSymbolSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil || mgr.SymbolTypeface() == nil {
		return nil
	}
	size := font.Size
	if size <= 0 {
		size = 16
	}
	key := fontKey{family: "_symbol", size: float32(size), weight: 400}
	globalFontCacheMu.Lock()
	defer globalFontCacheMu.Unlock()
	if f, ok := globalFontCache[key]; ok {
		return f
	}
	f := skia.NewFont(mgr.SymbolTypeface(), float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	globalFontCache[key] = f
	return f
}

// globalCJKSkiaFont returns a cached *skia.Font using the OS CJK Typeface
// (Microsoft YaHei) at the given font size, or nil if not available.
func globalCJKSkiaFont(font Font) *skia.Font {
	mgr := GetFontManager()
	if mgr == nil || mgr.CJKTypeface() == nil {
		return nil
	}
	size := font.Size
	if size <= 0 {
		size = 16
	}
	key := fontKey{family: "_cjk", size: float32(size), weight: 400}
	globalFontCacheMu.Lock()
	defer globalFontCacheMu.Unlock()
	if f, ok := globalFontCache[key]; ok {
		return f
	}
	f := skia.NewFont(mgr.CJKTypeface(), float32(size))
	if f == nil {
		return nil
	}
	f.SetEdging(skia.FontEdgingAntialias)
	f.SetSubpixel(true)
	globalFontCache[key] = f
	return f
}

// GlobalFontAscent returns the ascent (positive distance from baseline to the
// font's recommended top) for the given font. Mirrors Canvas.FontAscent but
// usable without a Canvas instance. Falls back to size * 0.8 on failure.
func GlobalFontAscent(font Font) float64 {
	skFont := globalSkiaFont(font)
	if skFont == nil {
		size := font.Size
		if size <= 0 {
			size = 16
		}
		return size * 0.8
	}
	m, _ := skFont.Metrics()
	return float64(-m.Ascent)
}

// GlobalFontDescent returns the descent (positive distance from baseline to the
// font's recommended bottom) for the given font. Returns 0 on failure.
func GlobalFontDescent(font Font) float64 {
	skFont := globalSkiaFont(font)
	if skFont == nil {
		return 0
	}
	m, _ := skFont.Metrics()
	return float64(m.Descent)
}

// GlobalFontLineGap returns the line gap (leading) for the given font,
// mirroring Skia's FontMetrics.Leading. The inline formatting context uses
// this to compute the "normal" line-height as ascent + descent + lineGap,
// matching how browsers compute the default line box height from the font's
// hhea/sTypo lineGap value. Returns 0 on failure.
func GlobalFontLineGap(font Font) float64 {
	skFont := globalSkiaFont(font)
	if skFont == nil {
		return 0
	}
	m, _ := skFont.Metrics()
	return float64(m.Leading)
}
