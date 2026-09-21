// Translation of: tests for Source/WebCore/platform/graphics/GraphicsContext.cpp
// Completeness: 50%
// Simplifications:
//   - tests verify pixel-level output of the Skia rasterizer; no image
//     golden comparison is performed.

package graphics

import (
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestCanvasFillRect verifies FillRect writes the supplied color to every pixel in the
// rectangle and leaves pixels outside untouched (transparent).
func TestCanvasFillRect(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	c.FillRect(2, 2, 4, 4, red)

	if got := c.PixelAt(3, 3); got != red {
		t.Fatalf("inside pixel = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(5, 5); got != red {
		t.Fatalf("corner pixel = %+v, want %+v", got, red)
	}
	// Outside the rect should remain transparent.
	if got := c.PixelAt(0, 0); got != (Color{}) {
		t.Fatalf("outside pixel = %+v, want transparent", got)
	}
	if got := c.PixelAt(7, 7); got != (Color{}) {
		t.Fatalf("outside pixel = %+v, want transparent", got)
	}
}

// TestCanvasPixelsBufferShape verifies Pixels() returns a buffer sized width*height*4.
func TestCanvasPixelsBufferShape(t *testing.T) {
	c := NewCanvas(4, 3)
	defer c.Release()
	pix := c.Pixels()
	if len(pix) != 4*3*4 {
		t.Fatalf("Pixels() len = %d, want %d", len(pix), 4*3*4)
	}
	// Fresh buffer is fully transparent.
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 0 {
			t.Fatalf("fresh buffer alpha at %d = %d, want 0", i, pix[i])
		}
	}
	// After a fill, the alpha of a covered pixel is opaque.
	c.FillRect(0, 0, 4, 3, Color{A: 0xFF})
	pix = c.Pixels() // re-read after mutation
	if pix[3] != 0xFF {
		t.Fatalf("after fill alpha = %d, want 0xFF", pix[3])
	}
}

// TestCanvasStrokeRect verifies StrokeRect draws a hollow outline: border pixels are
// colored and interior pixels are untouched.
func TestCanvasStrokeRect(t *testing.T) {
	c := NewCanvas(12, 12)
	defer c.Release()
	blue := Color{R: 0, G: 0, B: 0xFF, A: 0xFF}
	c.StrokeRect(2, 2, 8, 8, 2, blue)

	// Border pixels colored (check interior of the stroke, not AA edges).
	if got := c.PixelAt(2, 5); got != blue {
		t.Fatalf("border pixel = %+v, want %+v", got, blue)
	}
	if got := c.PixelAt(5, 2); got != blue {
		t.Fatalf("border pixel = %+v, want %+v", got, blue)
	}
	// Interior untouched.
	if got := c.PixelAt(6, 6); got != (Color{}) {
		t.Fatalf("interior pixel = %+v, want transparent", got)
	}
}

// TestCanvasClip verifies that an active clip suppresses drawing outside the clip region.
func TestCanvasClip(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	green := Color{R: 0, G: 0xFF, B: 0, A: 0xFF}
	c.Clip(Rect{X: 2, Y: 2, Width: 4, Height: 4})
	// Fill the whole canvas; only the clipped region should be painted.
	c.FillRect(0, 0, 10, 10, green)

	if got := c.PixelAt(3, 3); got != green {
		t.Fatalf("in-clip pixel = %+v, want %+v", got, green)
	}
	if got := c.PixelAt(0, 0); got != (Color{}) {
		t.Fatalf("out-of-clip pixel = %+v, want transparent", got)
	}
	if got := c.PixelAt(8, 8); got != (Color{}) {
		t.Fatalf("out-of-clip pixel = %+v, want transparent", got)
	}
}

// TestCanvasSaveRestore verifies Save/Restore round-trips clip and transform state.
func TestCanvasSaveRestore(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	white := Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	c.Save()
	c.Clip(Rect{X: 0, Y: 0, Width: 2, Height: 2})
	c.FillRect(0, 0, 10, 10, white)
	// Only top-left 2x2 painted.
	if got := c.PixelAt(0, 0); got != white {
		t.Fatalf("clipped pixel = %+v, want white", got)
	}
	if got := c.PixelAt(5, 5); got != (Color{}) {
		t.Fatalf("outside clip = %+v, want transparent", got)
	}
	c.Restore()

	// After restore the clip is gone; a fill should cover everything.
	c.FillRect(0, 0, 10, 10, white)
	if got := c.PixelAt(5, 5); got != white {
		t.Fatalf("after restore pixel = %+v, want white", got)
	}
}

// TestCanvasTranslate verifies Translate offsets subsequent draw coordinates.
func TestCanvasTranslate(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}
	c.Translate(4, 4)
	c.FillRect(0, 0, 2, 2, red)

	if got := c.PixelAt(4, 4); got != red {
		t.Fatalf("translated origin pixel = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(0, 0); got != (Color{}) {
		t.Fatalf("pre-translate origin pixel = %+v, want transparent", got)
	}
}

// TestCanvasScale verifies Scale enlarges subsequent draw output.
func TestCanvasScale(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}
	c.Scale(2, 2)
	c.FillRect(1, 1, 1, 1, red)
	// A 1x1 rect at (1,1) scaled by 2 becomes a 2x2 rect at device (2,2).
	if got := c.PixelAt(2, 2); got != red {
		t.Fatalf("scaled pixel (2,2) = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(3, 3); got != red {
		t.Fatalf("scaled pixel (3,3) = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(4, 4); got != (Color{}) {
		t.Fatalf("pixel outside scaled rect = %+v, want transparent", got)
	}
}

// TestCanvasDrawText verifies DrawText paints text with Skia's real font rasterizer.
// The test checks that the text region contains non-transparent pixels.
func TestCanvasDrawText(t *testing.T) {
	c := NewCanvas(60, 30)
	defer c.Release()
	black := Color{R: 0, G: 0, B: 0, A: 0xFF}
	font := Font{Family: "sans-serif", Size: 14, Weight: 400, Style: "normal"}
	// In Skia, y is the baseline position. Draw text with baseline at y=14.
	c.DrawText(2, 14, "Hello", font, black)

	// Check that some pixels in the text region are non-transparent.
	found := false
	for y := 2; y < 18 && !found; y++ {
		for x := 2; x < 50 && !found; x++ {
			if p := c.PixelAt(x, y); p.A > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no non-transparent pixels found in text region")
	}

	// Pixels far from the text should remain transparent.
	if got := c.PixelAt(55, 25); got != (Color{}) {
		t.Fatalf("far pixel = %+v, want transparent", got)
	}
}

// TestCanvasAlphaCompositing verifies that semi transparent fills blend with existing
// content rather than overwriting.
func TestCanvasAlphaCompositing(t *testing.T) {
	c := NewCanvas(4, 1)
	defer c.Release()
	// Fill opaque red.
	c.FillRect(0, 0, 4, 1, Color{R: 0xFF, G: 0, B: 0, A: 0xFF})
	// Overlay 50% green: result green channel should be non-zero, red reduced.
	c.FillRect(0, 0, 4, 1, Color{R: 0, G: 0xFF, B: 0, A: 0x80})
	p := c.PixelAt(1, 0) // check interior pixel to avoid AA edges
	if p.G == 0 {
		t.Fatalf("blended green = 0, want > 0")
	}
	if p.A != 0xFF {
		t.Fatalf("blended alpha = %d, want 0xFF", p.A)
	}
}

// TestCanvasClearRect verifies ClearRect erases a region back to transparent.
func TestCanvasClearRect(t *testing.T) {
	c := NewCanvas(6, 6)
	defer c.Release()
	white := Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	c.FillRect(0, 0, 6, 6, white)
	c.ClearRect(1, 1, 2, 2)

	if got := c.PixelAt(1, 1); got != (Color{}) {
		t.Fatalf("cleared pixel = %+v, want transparent", got)
	}
	if got := c.PixelAt(0, 0); got != white {
		t.Fatalf("uncleared pixel = %+v, want white", got)
	}
}

// TestCanvasFillLinearGradient verifies FillLinearGradient produces a visible
// gradient from top (startColor) to bottom (endColor). The top pixel should
// be closer to startColor and the bottom pixel closer to endColor.
func TestCanvasFillLinearGradient(t *testing.T) {
	c := NewCanvas(10, 20)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}
	blue := Color{B: 0xFF, A: 0xFF}
	c.FillLinearGradient(0, 0, 10, 20, red, blue)

	// Top pixel: should be close to red.
	top := c.PixelAt(5, 1)
	if top.R < 0x80 {
		t.Fatalf("top pixel R = %d, want >= 0x80 (close to red)", top.R)
	}
	// Bottom pixel: should be close to blue.
	bot := c.PixelAt(5, 18)
	if bot.B < 0x80 {
		t.Fatalf("bottom pixel B = %d, want >= 0x80 (close to blue)", bot.B)
	}
	// Both should be opaque.
	if top.A != 0xFF || bot.A != 0xFF {
		t.Fatalf("expected opaque: top.A=%d bot.A=%d", top.A, bot.A)
	}
}

// TestCanvasFillRadialGradient verifies FillRadialGradient produces a visible
// radial gradient from center (centerColor) outward (edgeColor).
func TestCanvasFillRadialGradient(t *testing.T) {
	c := NewCanvas(20, 20)
	defer c.Release()
	white := Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	black := Color{A: 0xFF}
	c.FillRadialGradient(10, 10, 8, white, black)

	// Center pixel should be close to white.
	center := c.PixelAt(10, 10)
	if center.R < 0x80 {
		t.Fatalf("center pixel R = %d, want >= 0x80 (close to white)", center.R)
	}
	// Edge area should be closer to black.
	edge := c.PixelAt(2, 10)
	if edge.R > 0x80 {
		t.Fatalf("edge pixel R = %d, want <= 0x80 (closer to black)", edge.R)
	}
}

// TestCanvasClipPath verifies ClipPath restricts drawing to the interior of a
// triangular path: pixels inside the triangle are painted and those outside
// are clipped.
func TestCanvasClipPath(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	green := Color{R: 0, G: 0xFF, B: 0, A: 0xFF}

	// Create a triangle path covering the top-left half of the canvas.
	path := skia.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 0)
	path.LineTo(0, 10)
	path.Close()
	defer path.Release()
	c.ClipPath(path)
	c.FillRect(0, 0, 10, 10, green)

	// Pixel inside triangle (top-left) should be painted.
	if got := c.PixelAt(2, 2); got != green {
		t.Fatalf("in-path pixel = %+v, want %+v", got, green)
	}
	// Pixel outside triangle (bottom-right) should be clipped.
	if got := c.PixelAt(8, 8); got != (Color{}) {
		t.Fatalf("out-of-path pixel = %+v, want transparent", got)
	}
}

// TestCanvasRotate verifies Rotate rotates subsequent draw output. A rect
// drawn after a 90-degree rotation should appear at a rotated position.
func TestCanvasRotate(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	// Translate to center, then rotate 90 degrees, then draw a line from
	// (0,0) to (5,0). 90 degree rotation means the horizontal line becomes
	// a vertical line pointing downward.
	c.Save()
	c.Translate(3, 3)
	c.Rotate(90)
	c.FillRect(0, 0, 5, 2, red)
	c.Restore()

	// After rotate(90), the rect at (0,0,5,2) should end up at a rotated
	// position that covers some pixel offset from the translation point.
	// At least some pixels in the rotated area should be non-transparent.
	found := false
	for y := 0; y < 10 && !found; y++ {
		for x := 0; x < 10 && !found; x++ {
			if c.PixelAt(x, y).A > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no pixels drawn after rotation")
	}
}

// TestCanvasSkew verifies Skew distorts the coordinate system such that a
// rectangle drawn at an angle appears skewed.
func TestCanvasSkew(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	// Apply a slight X skew before drawing.
	c.Save()
	c.Skew(0.3, 0)
	c.FillRect(0, 0, 5, 5, red)
	c.Restore()

	// At least some pixels should be painted in the skewed rect area.
	found := false
	for y := 0; y < 10 && !found; y++ {
		for x := 0; x < 10 && !found; x++ {
			if c.PixelAt(x, y).A > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no pixels drawn after skew")
	}
}

// TestCanvasMatrix verifies SetMatrix/GetMatrix/ResetMatrix round-trip.
func TestCanvasMatrix(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()

	// Start with identity then set a known matrix.
	c.ResetMatrix()
	id := c.GetMatrix()
	if id.ScaleX != 1 || id.ScaleY != 1 || id.TransX != 0 || id.TransY != 0 {
		t.Fatalf("identity matrix = %+v, want identity", id)
	}

	// Set a translate matrix and verify GetMatrix reflects it.
	c.SetMatrix(skia.MatrixTranslate(5, 10))
	m := c.GetMatrix()
	if m.TransX != 5 || m.TransY != 10 {
		t.Fatalf("translate matrix = %+v, want TransX=5 TransY=10", m)
	}

	// Concat another translate and verify it accumulates.
	c.Concat(skia.MatrixTranslate(3, 7))
	m2 := c.GetMatrix()
	if m2.TransX != 8 || m2.TransY != 17 {
		t.Fatalf("after concat matrix = %+v, want TransX=8 TransY=17", m2)
	}
}

// TestCanvasConcatScale verifies Concat applies a full matrix containing scale.
func TestCanvasConcatScale(t *testing.T) {
	c := NewCanvas(10, 10)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	// Scale by 2x via Concat.
	c.Concat(skia.MatrixScale(2, 2))
	c.FillRect(1, 1, 1, 1, red)

	// A 1x1 rect at (1,1) scaled by 2 becomes a 2x2 rect at device (2,2).
	if got := c.PixelAt(2, 2); got != red {
		t.Fatalf("scaled pixel (2,2) = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(3, 3); got != red {
		t.Fatalf("scaled pixel (3,3) = %+v, want %+v", got, red)
	}
	if got := c.PixelAt(5, 5); got != (Color{}) {
		t.Fatalf("pixel outside scaled rect = %+v, want transparent", got)
	}
}

// TestCanvasSkewTakesDegrees 回归 Skew 的单位：参数是【角度】（WebKit
// GraphicsContext::skew 与 CSS skew()/skewX() 一致），必须内部转成 Skia 要的
// 切线因子。曾经的实现把 45 原样传给 SkCanvas::skew，得到 tan=45 而不是
// tan(45°)=1 → 元素被斜切到画布外，画面上什么都没有。
func TestCanvasSkewTakesDegrees(t *testing.T) {
	c := NewCanvas(120, 60)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	// skewX(45deg) 且 transform-origin 在 (0,0)：x' = x + tan(45°)·y = x + y。
	// 45×45 的方块 (0,0)-(45,45) 因此变成平行四边形：上边 x∈[0,45]、
	// 下边 x∈[45,90]。
	c.Skew(45, 0)
	c.FillRect(0, 0, 45, 45, red)

	// 逆变换 (75,44) → (75-44, 44) = (31,44)，在方块内 → 必须可见。
	if got := c.PixelAt(75, 44); got != red {
		t.Errorf("(75,44) = %+v, want red —— Skew 未按角度解析（tan(45°)=1）", got)
	}
	// 逆变换 (5,40) → (-35,40)，在方块左侧之外 → 必须透明。
	if got := c.PixelAt(5, 40); got != (Color{}) {
		t.Errorf("(5,40) = %+v, want transparent —— 斜切量过大", got)
	}
	// skewDegToFactor 的边界：0 度不斜切，±90 度钳制（不产生 Inf）。
	if got := skewDegToFactor(0); got != 0 {
		t.Errorf("skewDegToFactor(0) = %v, want 0", got)
	}
	if got := skewDegToFactor(45); got < 0.99 || got > 1.01 {
		t.Errorf("skewDegToFactor(45) = %v, want ~1 (tan 45°)", got)
	}
	if v := skewDegToFactor(90); v <= 0 || v > 1e6 {
		t.Errorf("skewDegToFactor(90) = %v, want clamped positive finite", v)
	}
}

// TestCanvasPushMatrixKeepsClip 回归 PushMatrix 的核心契约：
//   ① 变换被还原（后续绘制回到原坐标系）；
//   ② 期间设置的裁剪**保留**（Skia 设置 clip 时就把它烘焙在当时的 CTM 下）。
// 渲染器用它把元素自身的 overflow 裁剪放进元素自身的变换空间，
// 从而支持旋转/缩放/斜切的非轴对齐裁剪。
func TestCanvasPushMatrixKeepsClip(t *testing.T) {
	c := NewCanvas(60, 60)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	c.Save()
	restore := c.PushMatrix()
	// 在「平移 30px」的变换空间里裁剪 (0,0)-(10,10) → 设备空间 30..40。
	c.Translate(30, 0)
	c.Clip(Rect{X: 0, Y: 0, Width: 10, Height: 10})
	restore() // ★ 矩阵还原，裁剪必须仍在 30..40

	// CTM 必须已回到原位：铺满整块画布。
	c.FillRect(0, 0, 60, 60, red)
	c.Restore()

	// 裁剪区内 → 红。
	if got := c.PixelAt(35, 5); got != red {
		t.Errorf("(35,5) = %+v, want red —— PushMatrix 后设置的裁剪丢失了", got)
	}
	// 裁剪区外 → 透明（说明裁剪确实生效且没有被放大成整块画布）。
	if got := c.PixelAt(5, 5); got != (Color{}) {
		t.Errorf("(5,5) = %+v, want transparent —— 裁剪未生效", got)
	}
	// ★ 同时验证变换确实被还原了：若 Translate 没还原，整块画布会被平移到
	// (30,0) 之后，右侧 20px 就画不到。
	if got := c.PixelAt(35, 55); got != (Color{}) {
		t.Errorf("(35,55) = %+v, want transparent —— 裁剪区超出了设定范围（被放大）", got)
	}
}

// TestCanvasPushMatrixRestoresTransform：restore() 之后 CTM 必须与调用前完全
// 一致（含 canvasState 里那份 scale/translate 镜像 —— 它喂给 Clip()/DeviceRect
// 的坐标换算，漂移会让后续裁剪算错）。
func TestCanvasPushMatrixRestoresTransform(t *testing.T) {
	c := NewCanvas(80, 20)
	defer c.Release()
	red := Color{R: 0xFF, A: 0xFF}

	c.Save()
	restore := c.PushMatrix()
	c.Translate(40, 0)
	c.Scale(2, 1)
	c.Rotate(30)
	restore()

	// CTM 已还原 → 1×1 的世界坐标矩形精确落在设备 (5,5)。
	c.FillRect(5, 5, 1, 1, red)
	c.Restore()

	if got := c.PixelAt(5, 5); got != red {
		t.Errorf("(5,5) = %+v, want red —— PushMatrix 未还原 CTM（Skia 矩阵）", got)
	}
	if got := c.PixelAt(60, 5); got != (Color{}) {
		t.Errorf("(60,5) = %+v, want transparent —— CTM 残留平移", got)
	}

	// canvasState 镜像（scale/translate）也必须回到 1/0，否则 Clip() 的世界→
	// 设备换算会带上残留缩放。
	if dr := c.DeviceRect(Rect{X: 5, Y: 5, Width: 1, Height: 1}); dr != (Rect{X: 5, Y: 5, Width: 1, Height: 1}) {
		t.Errorf("DeviceRect after PushMatrix restore = %+v, want (5,5,1,1) —— canvasState 的 scale/translate 未还原", dr)
	}
}
