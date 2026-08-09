// Translation of: Source/WebCore/rendering/BackgroundPainter.cpp
//                  Source/WebCore/rendering/BorderPainter.cpp
//                  Source/WebCore/rendering/OutlinePainter.cpp
//                  Source/WebCore/rendering/TextPainter.cpp
//                  Source/WebCore/rendering/TextBoxPainter.cpp
// Completeness: 85%
// Simplifications:
//   - only flat background colors AND linear gradients are painted; background-image
//     images (png/jpg/svg) and pattern fills are omitted (no Image cache / decoded
//     image backing in this port)
//   - border styles include solid, dashed, dotted, double; groove/ridge/inset/outset
//     fall back to solid
//   - outline reads outline-* from the ComputedStyle.Properties map; the dedicated
//     outline fields that WebKit keeps on RenderStyle are not modeled
//   - text is painted as a single run per InlineTextBox segment using the text
//     renderer in graphics.Canvas; no shaping / bidi / complex text
//     rasterizer in graphics.Canvas; no shaping / bidi / complex text
//   - the per-side border colors come from ComputedStyle; border widths come from the
//     resolved style lengths via lengthValue

package rendering

import (
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// asRenderBox returns the *RenderBox backing a box-bearing RenderObject. Because the Go
// port models the C++ inheritance chain with embedding (RenderBlock embeds RenderBox,
// RenderBlockFlow embeds RenderBlock, RenderView embeds RenderBlockFlow), a single type
// assertion cannot recover the box for every concrete type. This helper type-switches
// over the known box-bearing concrete types and returns a pointer to the embedded
// RenderBox. Non-box objects (RenderInline / RenderText) return nil.
func asRenderBox(o RenderObject) *RenderBox {
	switch v := o.(type) {
	case *RenderBox:
		return v
	case *RenderBlock:
		return &v.RenderBox
	case *RenderBlockFlow:
		return &v.RenderBlock.RenderBox
	case *RenderView:
		return &v.RenderBlockFlow.RenderBlock.RenderBox
	}
	return nil
}

// toGraphicsColor converts a style.Color to a graphics.Color. The two structs share the
// same 8-bit RGBA layout, so this is a field-for-field copy.
func toGraphicsColor(c style.Color) graphics.Color {
	return graphics.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// textName returns a short identity (tag.class) for a RenderText's element.
func textName(text *RenderText) string {
	if text == nil || text.Node() == nil {
		return "?"
	}
	if el, ok := text.Node().(*dom.Element); ok {
		n := el.LocalName()
		if cls := el.GetAttribute("class"); cls != "" {
			n += "." + strings.Fields(cls)[0]
		}
		return n
	}
	return text.Node().NodeName()
}

// elText returns the trimmed text content of a RenderText.
func elText(text *RenderText) string {
	if text == nil {
		return ""
	}
	t := text.Text()
	if len(t) > 30 {
		t = t[:30] + "…"
	}
	return t
}

// findTextOverflowAncestor walks up the render tree from the given RenderObject
// to find the nearest ancestor with text-overflow:ellipsis. Returns its content
// box rect, or nil if none found.
func findTextOverflowAncestor(ro RenderObject) *layout.LayoutRect {
	for p := ro.Parent(); p != nil; p = p.Parent() {
		st := p.Style()
		if st == nil {
			continue
		}
		if st.TextOverflow != style.TextOverflowEllipsis {
			continue
		}
		// Must also have overflow:hidden/auto/scroll (inheriting text-overflow
		// alone doesn't make an element the truncation container).
		if st.OverflowX != style.OverflowHidden &&
			st.OverflowX != style.OverflowAuto &&
			st.OverflowX != style.OverflowScroll {
			continue
		}
		if box := asRenderBox(p); box != nil {
			cbr := box.ContentBoxRect()
			return &cbr
		}
	}
	return nil
}

// listMarkerForRenderText walks up the render tree from a RenderText to find a
// display:list-item ancestor whose layout box carries a computed marker text.
// It returns (marker, liBox) — empty marker when the text is not inside a list
// item. The marker text is computed during layout (layout.BlockFormattingContext
// sets ElementBox.MarkerText) and mirrored to the render object's layout box.
func listMarkerForRenderText(rt *RenderText) (string, *layout.LayoutBox) {
	if rt == nil {
		return "", nil
	}
	for p := rt.Parent(); p != nil; p = p.Parent() {
		if p.Style() == nil || p.Style().Display != style.DisplayListItem {
			continue
		}
		lb := p.LayoutBox()
		if lb == nil {
			return "", nil
		}
		if lb.MarkerText != "" {
			return lb.MarkerText, lb
		}
		return "", nil
	}
	return "", nil
}

// BoxGeometry returns the border-box position and size of a render object, or
// (0,0,0,0,false) if the object is not box-bearing. Exposed for embedders/tests
// that need to inspect the laid-out geometry (e.g. for hit-testing or debugging).
func BoxGeometry(o RenderObject) (x, y, w, h float64, ok bool) {
	box := asRenderBox(o)
	if box == nil {
		return 0, 0, 0, 0, false
	}
	return box.X(), box.Y(), box.Width(), box.Height(), true
}

// toGraphicsFont builds a graphics.Font from a ComputedStyle, mirroring the FontCascade
// construction that TextPainter performs before drawing.
func toGraphicsFont(st *style.ComputedStyle) graphics.Font {
	if st == nil {
		return graphics.Font{Family: "serif", Size: 16, Weight: 400, Style: "normal"}
	}
	size := st.FontSize.Value
	if size <= 0 {
		size = 16
	}
	return graphics.Font{
		Family: st.FontFamily,
		Size:   size,
		Weight: parseFontWeight(st.FontWeight),
		Style:  st.FontStyle,
	}
}

// parseFontWeight resolves a CSS font-weight keyword / number to a numeric weight,
// mirroring the weight normalization FontCascadeDescription performs.
func parseFontWeight(w string) int {
	switch w {
	case "bold":
		return 700
	case "bolder":
		return 700
	case "lighter":
		return 300
	case "normal", "":
		return 400
	}
	if n, err := strconv.Atoi(w); err == nil {
		return n
	}
	return 400
}

// paintOpacity returns the effective opacity to apply when drawing o. When
// the paint is already inside an opacity transparency layer
// (PaintInfo.opacityLayerDepth > 0), the enclosing SaveLayer composites the
// whole subtree at that opacity — mirroring the browser's flatten-then-fade
// semantics — so painters must NOT multiply colors again (otherwise text
// drawn at 50% alpha onto a 50% background looks translucent grey instead
// of the browser's solid dark text, and a 50% border over a 50% background
// leaves a visible bright edge).
func paintOpacity(o RenderObject, info *PaintInfo) float64 {
	if info != nil && info.opacityLayerDepth > 0 {
		return 1.0
	}
	return CumulativeOpacity(o)
}

// PaintBackground paints the background color of a RenderBox, mirroring
// BackgroundPainter::paintBackground. The background fills the border-box rectangle
// (background clips to the border-box by default). Background images are not supported in
// this port. Painting is skipped when the background is fully transparent or the box is
// outside the dirty rect.
func PaintBackground(box *RenderBox, info *PaintInfo) {
	if box == nil || info == nil || info.canvas == nil {
		return
	}
	st := box.Style()
	if st == nil {
		return
	}
	// ★ 滚动容器自身背景固定于视口：paintLayerContents 对容器内容整体
	// translate(-scroll) 后，背景若用绝对坐标绘制会随内容一起滚动——
	// 背景滚出容器视口，文字继续滚动到背景区域外显示（"文字在背景外"）。
	// CSS background-attachment:scroll（默认）背景相对元素固定、不随内容
	// 滚动。这里补偿 box 自身的 scroll offset：祖先滚动（box 整体随祖先
	// 内容移动）保留，只有该 box 自身的内容滚动被抵消，背景钉回视口。
	ox, oy := 0.0, 0.0
	if info.rv != nil {
		ox, oy = info.rv.BoxScrollOffset(box)
	}
	// ★ intersects 检查用 sticky 偏移后的位置：sticky 元素绘制坐标 = 布局
	// 坐标 + 页面滚动 translate + sticky pin translate（canvas 已应用），
	// 但 dirty-rect 检查用的是未 translate 的布局坐标。pin 后元素被拉进
	// 视口，静态 rect 却在视口外 → 误 cull。这里仅修正检查矩形，绘制
	// 仍用 box.X()/box.Y()（canvas translate 已在绘制路径中）。
	checkRect := rectFromLayout(box.X()+ox+info.stickyDx, box.Y()+oy+info.stickyDy, box.Width(), box.Height())
	if !info.intersects(checkRect) {
		return
	}
	rect := rectFromLayout(box.X()+ox, box.Y()+oy, box.Width(), box.Height())
	// Paint box-shadow before the background (shadows sit behind the element).
	// Paint shadows even when the background is transparent. Inset shadows are
	// excluded here — they paint ABOVE the background (see below).
	if st.BoxShadow != "" && st.BoxShadow != "none" {
		r := lengthValue(st.BorderRadius)
		shadows := parseShadowList(st.BoxShadow)
		op := paintOpacity(box, info)
		paintBoxShadow(info.canvas, box.X()+ox, box.Y()+oy, box.Width(), box.Height(), r, shadows, op, false)
	}
	// background-image: url(...) — decode and draw with size/position.
	if url, ok := parseBackgroundURL(st.BackgroundImage); ok {
		if img := loadBackgroundImage(url, ""); img != nil && img.Loaded() {
			// Raster image: dest math uses the intrinsic size for cover/
			// contain aspect ratio.
			dx, dy, dw, dh := computeBackgroundDest(rect.X, rect.Y, rect.Width, rect.Height,
				st.BackgroundSize, st.BackgroundPosition, img.Width(), img.Height())
			paintBackgroundImageTiled(info.canvas, img, rect.X, rect.Y, rect.Width, rect.Height,
				dx, dy, dw, dh, st.BackgroundRepeat)
		} else if svg := loadBackgroundSVG(url); svg != nil {
			// Vector SVG: no intrinsic raster size — gradient-like dest
			// (auto/cover/contain fill the box; explicit sizes confine it).
			dx, dy, dw, dh := computeGradientDest(rect.X, rect.Y, rect.Width, rect.Height,
				st.BackgroundSize, st.BackgroundPosition)
			paintSVGScaled(info.canvas, svg, dx, dy, dw, dh)
		}
		return
	}

	// Paint gradient layers (background-image). Multiple comma-separated
	// layers stack with the FIRST layer on TOP (CSS background layering);
	// paint bottom-up so the first layer is drawn last. The
	// background-color (if any) paints beneath all layers.
	bgLayers := splitBackgroundLayers(st.BackgroundImage)
	if len(bgLayers) > 0 {
		r := lengthValue(st.BorderRadius)
		// Gradient destination honors background-size/position (a sub-rect
		// confined by explicit sizes, offset by position; fill box default).
		gdx, gdy, gdw, gdh := computeGradientDest(rect.X, rect.Y, rect.Width, rect.Height,
			st.BackgroundSize, st.BackgroundPosition)
		drawGradientLayer := func(i int) {
			layer := bgLayers[i]
			if lg := parseGradient(layer); lg != nil {
				if r > 0 {
					info.canvas.Save()
					info.canvas.ClipRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r)
					paintLinearGradient(info.canvas, gdx, gdy, gdw, gdh, lg)
					info.canvas.Restore()
				} else {
					paintLinearGradient(info.canvas, gdx, gdy, gdw, gdh, lg)
				}
				return
			}
			if rg := parseRadialGradient(layer); rg != nil {
				if r > 0 {
					info.canvas.Save()
					info.canvas.ClipRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r)
					paintRadialGradient(info.canvas, gdx, gdy, gdw, gdh, rg)
					info.canvas.Restore()
				} else {
					paintRadialGradient(info.canvas, gdx, gdy, gdw, gdh, rg)
				}
			}
		}
		// Color beneath the layers.
		if bgc := toGraphicsColor(st.BackgroundColor); bgc.A != 0 {
			bgc = ApplyOpacityToColor(bgc, paintOpacity(box, info))
			if r > 0 {
				info.canvas.FillRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r, bgc)
			} else {
				info.canvas.FillRect(rect.X, rect.Y, rect.Width, rect.Height, bgc)
			}
			if el, ok := box.Node().(*dom.Element); ok {
				RecordComponentPaint(el, rect.X, rect.Y, rect.Width, rect.Height, bgc, graphics.Color{}, false)
			}
		}
		// Layers bottom-up (last layer first, first layer painted last = on top).
		for i := len(bgLayers) - 1; i >= 0; i-- {
			drawGradientLayer(i)
		}
		return
	}
	bg := toGraphicsColor(st.BackgroundColor)
	// ★ 动画背景色（@keyframes background-color）：光标闪烁动画（xterm
	// block 光标 0%→50% 插值）写入 AnimatedBackgroundColor，Paint 必须
	// 消费它，否则动画改了 ComputedStyle 但画面不变（「光标不闪烁」）。
	// 未动画时 AnimatedBackgroundColor 为零值（A==0）→ 用普通背景色。
	if st.AnimatedBackgroundColor.A != 0 || st.AnimatedBackgroundColor != (style.Color{}) {
		bg = toGraphicsColor(st.AnimatedBackgroundColor)
	}
	if os.Getenv("WB_ANIM_DEBUG") != "" && (st.AnimatedBackgroundColor.A != 0 || st.AnimatedBackgroundColor != (style.Color{})) {
		log.Printf("[anim/paint] bg=(%d,%d,%d,%d) rect=(%.0f,%.0f %.0fx%.0f) cls=%q",
			bg.R, bg.G, bg.B, bg.A, rect.X, rect.Y, rect.Width, rect.Height,
			func() string { if box.Node() != nil { if el, ok := box.Node().(*dom.Element); ok { return el.ClassName() } }; return "" }())
	}
	if bg.A == 0 {
		return
	}
	// Debug: log non-trivial background paints
	if paintDebugEnabled() && (bg.R != 0 || bg.G != 0 || bg.B != 0) {
		elName := ""
		if box.Node() != nil {
			if el, ok := box.Node().(*dom.Element); ok {
				elName = el.LocalName() + "." + el.ClassName()
			}
		}
		log.Printf("[dbg/paintbg] %s rect=(%.0f,%.0f %.0fx%.0f) col=#%02x%02x%02x alpha=%d",
			elName, rect.X, rect.Y, rect.Width, rect.Height, bg.R, bg.G, bg.B, bg.A)
		// ★ Skia 真实 clip 诊断：对比 Go 侧 c.state.clip 与 Skia 侧
		//   device clip（RestoreToCount/ResetMatrix 只操作 Go state 的
		//   地方会漂移）。状态栏（y=778+）若被 Skia clip 裁掉即可见。
		if sdc, ok := info.canvas.DeviceClipBounds(); ok {
			log.Printf("[dbg/clip] %s deviceClip=(%.0f,%.0f %.0fx%.0f) goClip=(%.0f,%.0f %.0fx%.0f) has=%v",
				elName, sdc.X, sdc.Y, sdc.Width, sdc.Height,
				func() float64 { if c, h := info.canvas.ClipRect(); h { return c.X }; return -1 }(),
				func() float64 { if c, h := info.canvas.ClipRect(); h { return c.Y }; return -1 }(),
				func() float64 { if c, h := info.canvas.ClipRect(); h { return c.Width }; return -1 }(),
				func() float64 { if c, h := info.canvas.ClipRect(); h { return c.Height }; return -1 }(),
				func() bool { _, h := info.canvas.ClipRect(); return h }())
		}
	}
	bg = ApplyOpacityToColor(bg, paintOpacity(box, info))
	if bg.A == 0 {
		return
	}
	if r := lengthValue(st.BorderRadius); r > 0 {
		info.canvas.FillRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r, bg)
	} else {
		info.canvas.FillRect(rect.X, rect.Y, rect.Width, rect.Height, bg)
	}
	if el, ok := box.Node().(*dom.Element); ok {
		RecordComponentPaint(el, rect.X, rect.Y, rect.Width, rect.Height, bg, graphics.Color{}, false)
	}
	// Inset shadows paint ABOVE the background (below the border): inset 6px
	// left shadow casts onto the element's own background.
	if st.BoxShadow != "" && st.BoxShadow != "none" {
		shadows := parseShadowList(st.BoxShadow)
		op := paintOpacity(box, info)
		paintBoxShadow(info.canvas, box.X()+ox, box.Y()+oy, box.Width(), box.Height(), lengthValue(st.BorderRadius), shadows, op, true)
	}
}

// PaintBorder paints the four border sides of a RenderBox, mirroring
// BorderPainter::paintBorder. Each side with a non-"none" style and a positive width is
// rasterized as a solid filled rectangle in the side's resolved color. Top and bottom
// spans are drawn full-width (claiming the corners); left and right spans are drawn only
// between them to avoid overwriting the corner color.
func PaintBorder(box *RenderBox, info *PaintInfo) {
	if box == nil || info == nil || info.canvas == nil {
		return
	}
	st := box.Style()
	if st == nil {
		return
	}
	// ★ 滚动容器自身边框同样固定于视口（与 PaintBackground 同理）：
	// 补偿 box 自身 scroll offset，抵消 paintLayerContents 的内容 translate。
	ox, oy := 0.0, 0.0
	if info.rv != nil {
		ox, oy = info.rv.BoxScrollOffset(box)
	}
	x, y := box.X()+ox, box.Y()+oy
	w, h := box.Width(), box.Height()
	topW := lengthValue(st.BorderTopWidth)
	rightW := lengthValue(st.BorderRightWidth)
	bottomW := lengthValue(st.BorderBottomWidth)
	leftW := lengthValue(st.BorderLeftWidth)
	if topW <= 0 && rightW <= 0 && bottomW <= 0 && leftW <= 0 {
		return
	}
	if !info.intersects(rectFromLayout(x+info.stickyDx, y+info.stickyDy, w, h)) {
		return
	}
	op := paintOpacity(box, info)
	// When border-radius is set, draw the border as a single stroked rounded
	// rectangle so the corners follow the curve. Using FillRect sides here
	// would paint sharp rectangular corners that cover the rounded background
	// produced by FillRoundRect in PaintBackground. The uniform-width,
	// uniform-color case (e.g. `border: 2px solid #e5e7eb; border-radius: 4px`)
	// is by far the most common, so it is handled directly; unequal sides
	// fall back to the per-side FillRect path below.
	btC, brC, bbC, blC := st.BorderColor("top"), st.BorderColor("right"), st.BorderColor("bottom"), st.BorderColor("left")
	// Transparent borders (e.g. `border: 1px solid transparent` on toolbars
	// and icon buttons) must not be painted — they occupy layout space but
	// stay invisible. The rounded-rect fast path must skip them too, just
	// like paintBorderSide's A==0 guard.
	if r := lengthValue(st.BorderRadius); r > 0 &&
		topW == rightW && rightW == bottomW && bottomW == leftW &&
		st.BorderTopStyle != "none" && st.BorderRightStyle != "none" &&
		st.BorderBottomStyle != "none" && st.BorderLeftStyle != "none" &&
		btC.A > 0 && brC.A > 0 && bbC.A > 0 && blC.A > 0 &&
		colorsEqual(btC, brC) &&
		colorsEqual(brC, bbC) &&
		colorsEqual(bbC, blC) {
		info.canvas.StrokeRoundRect(x, y, w, h, r, topW, ApplyOpacityToColor(toGraphicsColor(btC), op))
		return
	}
	// Top and bottom span the full width, including the corners.
	radius := lengthValue(st.BorderRadius)
	if topW > 0 && st.BorderTopStyle != "none" {
		c := ApplyOpacityToColor(toGraphicsColor(btC), op)
		if radius > 0 {
			paintRoundedBorderSide(info.canvas, "top", x, y, w, h, topW, radius, c)
		} else {
			paintBorderSide(info.canvas, x, y, w, topW, c, st.BorderTopStyle)
		}
	}
	if bottomW > 0 && st.BorderBottomStyle != "none" {
		c := ApplyOpacityToColor(toGraphicsColor(bbC), op)
		if radius > 0 {
			paintRoundedBorderSide(info.canvas, "bottom", x, y, w, h, bottomW, radius, c)
		} else {
			paintBorderSide(info.canvas, x, y+h-bottomW, w, bottomW, c, st.BorderBottomStyle)
		}
	}
	// Left and right exclude the top/bottom border regions so the corner color (top/bottom)
	// is preserved.
	midY := y + topW
	midH := h - topW - bottomW
	if midH <= 0 {
		return
	}
	// ★ 圆角 per-side 边框：border-radius>0 但 fast path 不满足（典型：仅单边
	//   有色边框，如 .conv-item.active 的 border-left: 2px solid var(--accent)）。
	//   浏览器对单边边框 + border-radius 的渲染 = 外弧(半径 r)与内弧(椭圆，
	//   沿边方向 r-width、垂直方向 r)之间的月牙环带：竖线中段为直边（贴背景
	//   左缘），两端沿圆角自然弯曲凸出弧带。由 paintRoundedBorderSide 用
	//   FillPath 按标准 CSS 几何一次填充。
	if leftW > 0 && st.BorderLeftStyle != "none" {
		c := ApplyOpacityToColor(toGraphicsColor(blC), op)
		if radius > 0 {
			paintRoundedBorderSide(info.canvas, "left", x, y, w, h, leftW, radius, c)
		} else {
			paintBorderSide(info.canvas, x, midY, leftW, midH, c, st.BorderLeftStyle)
		}
	}
	if rightW > 0 && st.BorderRightStyle != "none" {
		c := ApplyOpacityToColor(toGraphicsColor(brC), op)
		if radius > 0 {
			paintRoundedBorderSide(info.canvas, "right", x, y, w, h, rightW, radius, c)
		} else {
			paintBorderSide(info.canvas, x+w-rightW, midY, rightW, midH, c, st.BorderRightStyle)
		}
	}
	if radius <= 0 {
		// Corner bevels: when adjacent border colors differ, browsers split the
		// corner along the diagonal from the outer corner to the inner corner
		// (CSS border corner joining). The triangle on the horizontal-edge side
		// keeps the top/bottom color; the other triangle gets the left/right
		// color. Mirrors Edge pixel-for-pixel within anti-aliasing tolerance.
		paintBorderCorners(info.canvas, x, y, w, h, topW, rightW, bottomW, leftW,
			blC, brC, btC, bbC, op, st)
	}
}

// paintRoundedBorderSide 绘制单边圆角边框的标准 CSS 几何（月牙形）：
// 边框区域 = 圆角矩形的外弧（半径 r）与内弧之间的环带。CSS Backgrounds
// and Borders §5.1：内弧半径 = 外弧半径 - 相邻边框宽度。单边场景（如
// .conv-item.active 的 border-left: 2px + border-radius: 6px）下，沿边
// 方向的内弧半径 = r-width，垂直方向 = r（相邻边宽 0）——内弧为椭圆。
// 用 FillPath 一次性填充该月牙，抗锯齿由底层 Skia AA 处理，边缘自然平滑；
// 与浏览器逐像素一致（dev/desktop_probe 像素对比 PASS，几何差异 0）。
// 此前用逐行渐细带 + 亚像素 alpha 渐隐反推浏览器像素（12 轮迭代的魔法
// 数字堆叠），边缘生硬、易产生锯齿/不对称；标准几何一次填充即可。
func paintRoundedBorderSide(canvas *graphics.Canvas, side string, x, y, w, h, width, r float64, col graphics.Color) {
	if width <= 0 || col.A == 0 || r <= 0 {
		return
	}
	if width >= r {
		// 内弧半径 ≤ 0：圆角不足以容纳边框，退化为直角矩形边框
		switch side {
		case "left":
			paintBorderSide(canvas, x, y, width, h, col, "solid")
		case "right":
			paintBorderSide(canvas, x+w-width, y, width, h, col, "solid")
		case "top":
			paintBorderSide(canvas, x, y, w, width, col, "solid")
		case "bottom":
			paintBorderSide(canvas, x, y+h-width, w, width, col, "solid")
		}
		return
	}
	var pts []graphics.Point
	// 采样圆弧：圆心 (cx,cy)、半径 (rx,ry)（内弧为椭圆），角度 a0→a1。
	// 屏幕坐标 y 向下、角度顺时针：右 0 / 下 π/2 / 左 π / 上 3π/2。
	arc := func(cx, cy, rx, ry, a0, a1 float64) {
		n := int(math.Abs(a1-a0) / (math.Pi / 16))
		if n < 8 {
			n = 8
		}
		for i := 0; i <= n; i++ {
			a := a0 + (a1-a0)*float64(i)/float64(n)
			pts = append(pts, graphics.Point{X: cx + rx*math.Cos(a), Y: cy + ry*math.Sin(a)})
		}
	}
	const (
		angRight = 0.0
		angDown  = math.Pi / 2
		angLeft  = math.Pi
		angUp    = 3 * math.Pi / 2
	)
	inner := r - width // 沿边方向的内弧半径（垂直方向为 r）
	switch side {
	case "left":
		// 顺时针：内缘直边底 → 内弧左下(π→π/2) → 外弧左下(π/2→π) →
		//   外缘直边 → 外弧左上(π→3π/2) → 内弧左上(3π/2→π) → 闭合
		pts = append(pts, graphics.Point{X: x + width, Y: y + h - r})
		arc(x+r, y+h-r, inner, r, angLeft, angDown)
		arc(x+r, y+h-r, r, r, angDown, angLeft)
		pts = append(pts, graphics.Point{X: x, Y: y + r})
		arc(x+r, y+r, r, r, angLeft, angUp)
		arc(x+r, y+r, inner, r, angUp, angLeft)
	case "right":
		pts = append(pts, graphics.Point{X: x + w - width, Y: y + h - r})
		arc(x+w-r, y+h-r, inner, r, angRight, angDown)
		arc(x+w-r, y+h-r, r, r, angDown, angRight)
		pts = append(pts, graphics.Point{X: x + w, Y: y + r})
		// ★ 右上角：angRight(0)↔angUp(3π/2) 数值差 270°，线性插值会扫过
		//   右→下→左→上大弧（FillPath 形状错乱、右上角凹陷）。必须把终点
		//   折算到 0 附近的 90° 区间：右→上 = 0→-π/2，上→右 = 3π/2→2π。
		arc(x+w-r, y+r, r, r, angRight, angUp-2*math.Pi)
		arc(x+w-r, y+r, inner, r, angUp, angRight+2*math.Pi)
	case "top":
		pts = append(pts, graphics.Point{X: x + w - r, Y: y + width})
		// ★ 右上角同 right 分支：90° 区间折算（上→右 = 3π/2→2π，右→上 = 0→-π/2）
		arc(x+w-r, y+r, r, inner, angUp, angRight+2*math.Pi)
		arc(x+w-r, y+r, r, r, angRight, angUp-2*math.Pi)
		pts = append(pts, graphics.Point{X: x + r, Y: y})
		arc(x+r, y+r, r, r, angUp, angLeft)
		arc(x+r, y+r, r, inner, angLeft, angUp)
	case "bottom":
		pts = append(pts, graphics.Point{X: x + w - r, Y: y + h - width})
		arc(x+w-r, y+h-r, r, inner, angDown, angRight)
		arc(x+w-r, y+h-r, r, r, angRight, angDown)
		pts = append(pts, graphics.Point{X: x + r, Y: y + h})
		arc(x+r, y+h-r, r, r, angDown, angLeft)
		arc(x+r, y+h-r, r, inner, angLeft, angDown)
	}
	if len(pts) >= 3 {
		canvas.FillPath(pts, col, false)
	}
}


func paintBorderCorners(canvas *graphics.Canvas, x, y, w, h, topW, rightW, bottomW, leftW float64,
	blC, brC, btC, bbC style.Color, op float64, st *style.ComputedStyle) {
	// fillBelow paints every pixel of the rect that lies strictly below the
	// diagonal from (0,0) to (cw,ch) with col. The diagonal maps the outer
	// border corner to the inner (padding-edge) corner.
	fillBelow := func(cx, cy, cw, ch float64, col graphics.Color) {
		if cw <= 0 || ch <= 0 || col.A == 0 {
			return
		}
		col = ApplyOpacityToColor(col, op)
		for dy := 0; dy < int(ch); dy++ {
			for dx := 0; dx < int(cw); dx++ {
				// below ⟺ dy > (dx/cw)*ch
				if float64(dy) > float64(dx)*ch/cw {
					canvas.FillRect(cx+float64(dx), cy+float64(dy), 1, 1, col)
				}
			}
		}
	}
	// fillBelowRev paints pixels below the anti-diagonal from (0,ch) to (cw,0)
	// (used when the outer corner maps to the top-right / bottom-left of the
	// corner rect, i.e. the right and left corners).
	fillBelowRev := func(cx, cy, cw, ch float64, col graphics.Color) {
		if cw <= 0 || ch <= 0 || col.A == 0 {
			return
		}
		col = ApplyOpacityToColor(col, op)
		for dy := 0; dy < int(ch); dy++ {
			for dx := 0; dx < int(cw); dx++ {
				// below ⟺ dy > ch*(1-dx/cw)
				if float64(dy) > ch*(1-float64(dx)/cw) {
					canvas.FillRect(cx+float64(dx), cy+float64(dy), 1, 1, col)
				}
			}
		}
	}
	topStyle, rightStyle := st.BorderTopStyle, st.BorderRightStyle
	bottomStyle, leftStyle := st.BorderBottomStyle, st.BorderLeftStyle
	// Top-left: diagonal outer (x,y) → inner (x+leftW, y+topW). Below = left color.
	if topW > 0 && leftW > 0 && topStyle != "none" && leftStyle != "none" && !colorsEqual(btC, blC) {
		fillBelow(x, y, leftW, topW, toGraphicsColor(blC))
	}
	// Top-right: diagonal outer (x+w,y) → inner (x+w-rightW, y+topW). Below = right color.
	if topW > 0 && rightW > 0 && topStyle != "none" && rightStyle != "none" && !colorsEqual(btC, brC) {
		fillBelowRev(x+w-rightW, y, rightW, topW, toGraphicsColor(brC))
	}
	// Bottom-left: diagonal outer (x,y+h) → inner (x+leftW, y+h-bottomW).
	// Below (toward the left edge) = left color.
	if bottomW > 0 && leftW > 0 && bottomStyle != "none" && leftStyle != "none" && !colorsEqual(bbC, blC) {
		fillBelowRev(x, y+h-bottomW, leftW, bottomW, toGraphicsColor(blC))
	}
	// Bottom-right: diagonal outer (x+w,y+h) → inner (x+w-rightW, y+h-bottomW).
	// Below (toward the right edge) = right color.
	if bottomW > 0 && rightW > 0 && bottomStyle != "none" && rightStyle != "none" && !colorsEqual(bbC, brC) {
		fillBelow(x+w-rightW, y+h-bottomW, rightW, bottomW, toGraphicsColor(brC))
	}
}

// paintBorderSide draws a single border side with the given style.
// Supports solid, dashed, dotted, double. Falls back to solid for unknown styles.
func paintBorderSide(canvas *graphics.Canvas, x, y, w, h float64, col graphics.Color, style string) {
	if canvas == nil || col.A == 0 || w <= 0 || h <= 0 {
		return
	}
	switch style {
	case "solid":
		canvas.FillRect(x, y, w, h, col)
	case "dashed":
		thick := h
		if w < h {
			thick = w
		}
		if thick <= 0 {
			thick = 1
		}
		// Edge paints 3px dashed borders as 6px dashes with 6px gaps
		// (dash = gap = 2×width), unlike CSS's unspecified defaults.
		dashLen := thick * 2
		gapLen := thick * 2
		if w >= h {
			for dx := 0.0; dx < w; dx += dashLen + gapLen {
				dw := dashLen
				if dx+dw > w {
					dw = w - dx
				}
				canvas.FillRect(x+dx, y, dw, h, col)
			}
		} else {
			for dy := 0.0; dy < h; dy += dashLen + gapLen {
				dh := dashLen
				if dy+dh > h {
					dh = h - dy
				}
				canvas.FillRect(x, y+dy, w, dh, col)
			}
		}
	case "dotted":
		thick := h
		if w < h {
			thick = w
		}
		radius := thick / 2
		spacing := thick * 2
		if radius <= 0 {
			radius = 1
		}
		if spacing <= 0 {
			spacing = 4
		}
		if w >= h {
			for dx := radius; dx < w; dx += spacing {
				canvas.FillCircle(x+dx, y+radius, radius, col)
			}
		} else {
			for dy := radius; dy < h; dy += spacing {
				canvas.FillCircle(x+radius, y+dy, radius, col)
			}
		}
	case "double":
		thick := h
		if w < h {
			thick = w
		}
		third := thick / 3
		if third < 1 {
			third = 1
		}
		if w >= h {
			canvas.FillRect(x, y, w, third, col)
			canvas.FillRect(x, y+thick-third, w, third, col)
		} else {
			canvas.FillRect(x, y, third, h, col)
			canvas.FillRect(x+thick-third, y, third, h, col)
		}
	default:
		// groove/ridge/inset/outset fall back to solid
		canvas.FillRect(x, y, w, h, col)
	}
}

// PaintOutline paints the outline of a RenderBox, mirroring OutlinePainter::paintOutline.
// The outline is drawn outside the border-box, offset by outline-offset, as a stroked
// rectangle in the outline color. Outline properties are read from the ComputedStyle
// Properties map (outline-width / outline-color / outline-style) since dedicated outline
// fields are not modeled in this port.
func PaintOutline(box *RenderBox, info *PaintInfo) {
	if box == nil || info == nil || info.canvas == nil {
		return
	}
	st := box.Style()
	if st == nil {
		return
	}
	olStyle := st.OutlineStyle
	// 优先使用 typed 字段（outline 简写经 resolver 解析后设置 OutlineSet 等）；
	// 兼容旧的 Properties map 路径（outline-style/width/color 单独属性）。
	var width float64
	var color graphics.Color
	if st.OutlineSet {
		olStyle = st.OutlineStyle
		width = lengthValue(st.OutlineWidth)
		color = toGraphicsColor(st.OutlineColor)
	} else {
		olStyle = st.GetProperty("outline-style")
		width = lengthValue(parseLengthProperty(st.GetProperty("outline-width")))
		color = parseColorProperty(st.GetProperty("outline-color"))
	}
	if olStyle == "" || olStyle == "none" {
		return
	}
	if width <= 0 {
		width = 1
	}
	if color.A == 0 {
		// Default outline color is the element's current text color.
		color = toGraphicsColor(st.Color)
	}
	offset := lengthValue(parseLengthProperty(st.GetProperty("outline-offset")))
	x := box.X() - offset - width/2
	y := box.Y() - offset - width/2
	w := box.Width() + offset*2 + width
	h := box.Height() + offset*2 + width
	if !info.intersects(rectFromLayout(x, y, w, h)) {
		return
	}
	// Outline 跟随元素的 border-radius（浏览器行为：圆角 input 的 focus
	// outline 是圆角矩形，不是直角）。必须二选一：先画直角 StrokeRect
	// 再叠圆角会在圆角弧外残留直角的角部像素（用户看到的"多余直角边框"）。
	r := lengthValue(st.BorderRadius)
	if r > 0 {
		// outline 在 border-box 外，半径相应外扩（近似：+ width/2 + offset）。
		info.canvas.StrokeRoundRect(x, y, w, h, r+width/2+offset, width, color)
		return
	}
	info.canvas.StrokeRect(x, y, w, h, width, color)
}

// PaintText paints the text content of a RenderText, mirroring TextPainter::paintText.
// Each InlineTextBox segment produced by the inline formatting context is drawn at its
// laid-out position using the segment's substring, the element color and the resolved
// font. When no segments are present the whole text is drawn at the origin as a fallback.
//
// Selected text (the portion within rendering.CurrentSelection) is drawn in an
// inverted color (white) so that it is legible against the selection highlight
// background, matching browser behavior. text-decoration (underline / line-through)
// is painted after each run.
//
// ★ WB_TEXT_DEBUG=1：每 3s 汇总一次 PaintText 调用统计（调用次数/有段数/
// 实际 DrawText 次数/被裁剪数），用于排查「DOM/布局正常但文字不显示」。
var wbTextDebugCalls, wbTextDebugWithSegs, wbTextDebugDraws, wbTextDebugSkipped int
var wbTextDebugInView, wbTextDebugOutView, wbTextDebugBadY int
var wbTextDebugLast time.Time

// wbTermLogCount: WB_TEXT_DEBUG 时限制终端区域（y>600）DrawText 日志条数。
var wbTermLogCount int

func paintTextDebugLog() {
	if os.Getenv("WB_TEXT_DEBUG") == "" {
		return
	}
	now := time.Now()
	if wbTextDebugLast.IsZero() {
		wbTextDebugLast = now
		return
	}
	if now.Sub(wbTextDebugLast) < 3*time.Second {
		return
	}
	log.Printf("[text-debug] 3s: calls=%d segs=%d draws=%d skipped=%d (跳过率 %.0f%%) 视口内y0-800=%d 视口外=%d 负y=%d",
		wbTextDebugCalls, wbTextDebugWithSegs, wbTextDebugDraws, wbTextDebugSkipped,
		100*float64(wbTextDebugSkipped)/float64(max(1, wbTextDebugCalls)),
		wbTextDebugInView, wbTextDebugOutView, wbTextDebugBadY)
	wbTextDebugCalls, wbTextDebugWithSegs, wbTextDebugDraws, wbTextDebugSkipped = 0, 0, 0, 0
	wbTextDebugInView, wbTextDebugOutView, wbTextDebugBadY = 0, 0, 0
	wbTextDebugLast = now
}

func PaintText(text *RenderText, info *PaintInfo) {
	wbTextDebugCalls++
	paintTextDebugLog()
	if text == nil || info == nil || info.canvas == nil {
		return
	}
	st := text.Style()
	if st == nil {
		return
	}
	// ★ visibility:hidden/collapse 的文字不绘制（浏览器语义：占位但不画）。
	// xterm 的字符宽度测量 span（.xterm-char-measure-element 内容 32 个 W，
	// visibility:hidden + position:absolute）之前被 PaintText 画出 → 终端
	// 显示一长串 W。paintObjectForeground 的 RenderText 分支绕过
	// IsVisible()（RenderText 无 box），必须在此处检查。
	if st.Visibility == "hidden" || st.Visibility == "collapse" {
		return
	}
	// 诊断：32W 测量文本为何仍被绘制（若 Visibility 继承失败会走到这里）
	if os.Getenv("WB_TEXT_DEBUG") != "" && len(text.OriginalText()) >= 8 {
		if t8 := text.OriginalText()[:8]; t8 == "WWWWWWWW" {
			par := text.Parent()
			pv := "nil"
			if par != nil {
				if ps := par.Style(); ps != nil {
					pv = ps.Visibility
				}
			}
			log.Printf("[text-debug] 32W drawn! self.vis=%q parent.vis=%q", st.Visibility, pv)
		}
	}
	// 终端区域文字诊断：y>600（终端 y=624-778 区域）的 DrawText 前 20 条
	if os.Getenv("WB_TEXT_DEBUG") != "" && wbTermLogCount < 20 && len(text.Segments()) > 0 {
		seg0 := text.Segments()[0]
		if seg0.Y > 600 && seg0.Y < 800 {
			txt := text.OriginalText()
			if len(txt) > 20 {
				txt = txt[:20]
			}
			log.Printf("[text-debug] TERMREGION text=%q at (%.0f,%.0f) segs=%d vis=%q", txt, seg0.X, seg0.Y, len(text.Segments()), st.Visibility)
			wbTermLogCount++
		}
	}
	col := toGraphicsColor(st.Color)
	if col.A == 0 {
		return
	}
	col = ApplyOpacityToColor(col, paintOpacity(text, info))
	if col.A == 0 {
		return
	}
	font := toGraphicsFont(st)
	content := text.OriginalText()
	segments := text.Segments()
	ascent := info.canvas.FontAscent(font)

	// DEBUG: print segments info
	debugContent := content
	_ = debugContent

	if len(segments) == 0 {
		// Debug: log which RenderText has no segments
		_ = content
		return
	}
	wbTextDebugWithSegs++
	runes := []rune(content)
	rv := info.rv
	// Browsers default selected text to white so it is legible against the
	// semi-transparent blue selection background. A ::selection rule's
	// color overrides it.
	selCol := graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	if rv != nil && rv.Resolver() != nil {
		if _, fg, ok := rv.Resolver().SelectionColors(); ok && fg.A != 0 {
			selCol = toGraphicsColor(fg)
		}
	}

	// Paint text-shadow: draw the text once per shadow in the shadow color.
	textShadows := parseShadowList(st.TextShadow)
	if len(textShadows) > 0 {
		opacity := paintOpacity(text, info)
		for _, seg := range segments {
			end := seg.Start + seg.Len
			if seg.Start < 0 || end > len(runes) {
				continue
			}
			sub := string(runes[seg.Start:end])
			if sub == "" || sub == "\n" {
				continue
			}
			baseline := seg.Y + ascent
			paintTextShadow(info.canvas, textShadows, seg.X, baseline, sub, font, opacity)
		}
	}
	
	// text-overflow:ellipsis truncation: find ancestor with this property.
	toCB := findTextOverflowAncestor(text)
	// Compute ellipsis width: three tightly-spaced filled circles.
	textEllipsisW := float64(0)
	if toCB != nil {
		ellipsisDotR := font.Size * 0.07
		if ellipsisDotR < 0.8 {
			ellipsisDotR = 0.8
		}
		ellipsisGap := ellipsisDotR * 3.2 // ~1.2px gap between dot edges for 14px
		textEllipsisW = ellipsisGap*2 + ellipsisDotR*2
	}

	// List-item marker: draw the bullet/ordinal before the first text segment.
	// The marker text is computed during layout (ElementBox.MarkerText) and
	// exposed on the render object via ListMarkerText().
	if marker, mbox := listMarkerForRenderText(text); marker != "" && mbox != nil && len(segments) > 0 {
		first := segments[0]
		baseline := first.Y + ascent
		// The marker occupies the padding-left zone of the li (40px default);
		// draw it right-aligned within that zone, 6px before the content start.
		markerX := first.X - 6 - graphics.MeasureText(font, marker)
		info.canvas.DrawText(markerX, baseline, marker, font, col)
		_ = mbox
	}

	for _, seg := range segments {
		end := seg.Start + seg.Len
		if seg.Start < 0 || end > len(runes) {
			continue
		}
		if !info.intersects(rectFromLayout(seg.X, seg.Y, seg.Width, seg.Height)) {
			wbTextDebugSkipped++
			continue
		}
		baseline := seg.Y + ascent

	// ── text-overflow:ellipsis ──
		// Walk segments sequentially from the left. Track cumulative width from the
		// content box start. The ellipsis only kicks in when text genuinely
		// OVERFLOWS the content box (segRight > content width) — text that
		// exactly fills the box (or fits) must be drawn verbatim. Previously
		// maxTextRight = width − ellipsisWidth reserved the ellipsis even when
		// nothing overflowed, so every item whose name exactly filled its
		// container (e.g. "gou-ide" 49.4px in a 49.4px box) was truncated and
		// gained a spurious "…" — the "file names only show a few characters"
		// report.
		if toCB != nil {
			segRelX := seg.X - toCB.X // segment X relative to content box origin
			segRight := segRelX + seg.Width

			if segRight > toCB.Width {
				// Genuine overflow: now reserve room for the ellipsis.
				maxTextRight := toCB.Width - textEllipsisW
				if style.DiagEnabled("paint") {
					elName := textName(text)
					style.Diagf("paint", "ellipsis %s: segRight=%.1f toCB.w=%.1f maxTextRight=%.1f segX=%.1f text=%q",
						elName, segRight, toCB.Width, maxTextRight, segRelX, elText(text))
				}
				if segRelX >= toCB.Width {
					break
				}
				// How many characters of this segment fit within [segRelX, maxTextRight)?
				remaining := maxTextRight - segRelX
				if remaining <= 0 {
					break
				}
				subRunes := runes[seg.Start:end]
				visibleW := 0.0
				lastFit := 0
				for i, r := range subRunes {
					rw := graphics.MeasureText(font, string(r))
					if visibleW+rw > remaining {
						// Show the first character even if it slightly overflows,
						// so the text isn't completely blank.
						if i == 0 {
							visibleW += rw
							lastFit = 1
						}
						break
					}
					visibleW += rw
					lastFit = i + 1
				}
				if lastFit > 0 {
					visibleText := string(subRunes[:lastFit])
					sub := collapseWhitespace(visibleText)
					if sub != "" {
						info.canvas.DrawText(seg.X, baseline, sub, font, col)
						paintTextDecoration(info.canvas, seg.X, baseline, sub, font, st, col, ascent)
					}
				}
				// Draw "…" right after the last visible character using tightly
				// spaced filled circles, matching browser rendering where three
				// dots are approximately 1-2 px apart (no font side-bearing gaps).
				ellipsisX := seg.X + visibleW
				if lastFit == 0 {
					ellipsisX = toCB.X + toCB.Width - textEllipsisW
				}
				dotR := font.Size * 0.07
				if dotR < 0.8 {
					dotR = 0.8
				}
				dotGap := dotR * 3.2 // ~1.2px gap between dot edges for 14px font
				for i := 0; i < 3; i++ {
					info.canvas.FillCircle(ellipsisX+float64(i)*dotGap, baseline-dotR, dotR, col)
				}
				info.textOverflowEllipsisPainted = true
				break
			}
			// Segments that fully fit before the overflow point are drawn normally below.
		}

		// Determine selected range within this segment for inverted-color rendering.
		selFrom, selTo, hasSel := -1, -1, false
		if rv != nil {
			selFrom, selTo, hasSel = SelectionRangeForSegment(rv, text, seg.Start, seg.Len)
		}

		if !hasSel {
			// Entire segment unselected: draw in normal color.
			sub := collapseWhitespace(string(runes[seg.Start:end]))
			if sub != "" {
				wbTextDebugDraws++
				if seg.Y >= 0 && seg.Y < 800 {
					wbTextDebugInView++
				} else if seg.Y < 0 {
					wbTextDebugBadY++
				} else {
					wbTextDebugOutView++
				}
				info.canvas.DrawText(seg.X, baseline, sub, font, col)
				paintTextDecoration(info.canvas, seg.X, baseline, sub, font, st, col, ascent)
			}
			continue
		}

		// Partially or fully selected: split into up to 3 runs.
		// 1. Unselected prefix.
		prefixW := 0.0
		if selFrom > seg.Start {
			prefixText := collapseWhitespace(string(runes[seg.Start:selFrom]))
			if prefixText != "" {
				info.canvas.DrawText(seg.X, baseline, prefixText, font, col)
				paintTextDecoration(info.canvas, seg.X, baseline, prefixText, font, st, col, ascent)
				prefixW = graphics.MeasureText(font, prefixText)
			}
		}
		// 2. Selected portion (inverted color).
		selText := collapseWhitespace(string(runes[selFrom:selTo]))
		selW := 0.0
		if selText != "" {
			info.canvas.DrawText(seg.X+prefixW, baseline, selText, font, selCol)
			paintTextDecoration(info.canvas, seg.X+prefixW, baseline, selText, font, st, selCol, ascent)
			selW = graphics.MeasureText(font, selText)
		}
		// 3. Unselected suffix.
		if selTo < seg.Start+seg.Len {
			suffixText := collapseWhitespace(string(runes[selTo : seg.Start+seg.Len]))
			if suffixText != "" {
				info.canvas.DrawText(seg.X+prefixW+selW, baseline, suffixText, font, col)
				paintTextDecoration(info.canvas, seg.X+prefixW+selW, baseline, suffixText, font, st, col, ascent)
			}
		}
	}
}

// paintTextDecoration draws underline and/or line-through decorations for a
// text run, mirroring InlineTextBox::paintDecoration(). The decoration
// positions follow CSS conventions: underline sits just below the baseline,
// line-through crosses the midline of the x-height.
func paintTextDecoration(canvas *graphics.Canvas, x, baseline float64, text string, font graphics.Font, st *style.ComputedStyle, col graphics.Color, ascent float64) {
	if st == nil || text == "" {
		return
	}
	dec := strings.ToLower(st.TextDecoration)
	if dec == "" || dec == "none" {
		return
	}
	w := graphics.MeasureText(font, text)
	if w <= 0 {
		return
	}
	if strings.Contains(dec, "underline") {
		// Position the underline just below the descent line.
		y := baseline + 1
		canvas.FillRect(x, y, w, 1, col)
	}
	if strings.Contains(dec, "line-through") {
		// Line-through at approximately the midline (half the ascent above baseline).
		y := baseline - ascent*0.4
		canvas.FillRect(x, y, w, 1, col)
	}
}

// PaintCaret draws the text caret (insertion point) at CaretPos, mirroring
// CaretBase::paintCaret(). The caret is a 1px-wide vertical bar that spans
// the text ascent + descent. It blinks on/off at ~500ms intervals (controlled
// by CaretVisible).
func PaintCaret(rv *RenderView, info *PaintInfo) {
	if rv == nil || info == nil || info.canvas == nil {
		return
	}
	if CaretPos == nil || !CaretPos.IsValid() || !CaretVisible {
		return
	}
	rt := CaretPos.RT
	st := rt.Style()
	if st == nil {
		return
	}
	font := toGraphicsFont(st)
	ascent := info.canvas.FontAscent(font)
	descent := graphics.GlobalFontDescent(font)

	// Find the segment containing the caret offset to determine X position.
	segs := rt.Segments()
	var caretX, caretY float64
	found := false
	for _, seg := range segs {
		if CaretPos.Offset >= seg.Start && CaretPos.Offset <= seg.Start+seg.Len {
			runes := []rune(rt.OriginalText())
			prefix := ""
			if CaretPos.Offset > seg.Start && CaretPos.Offset <= len(runes) {
				prefix = collapseWhitespace(string(runes[seg.Start:CaretPos.Offset]))
			}
			caretX = seg.X + graphics.MeasureText(font, prefix)
			caretY = seg.Y
			found = true
			break
		}
	}
	if !found {
		return
	}
	caretCol := toGraphicsColor(st.Color)
	if caretCol.A == 0 {
		caretCol = graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
	}
	h := ascent + descent
	if h < 2 {
		h = 2
	}
	info.canvas.FillRect(caretX, caretY, 1, h, caretCol)
}

// PaintSelection draws the selection highlight rectangles for the current
// text selection. It is called between the background and foreground phases so
// that text is painted on top of the highlight. Mirrors
// RenderView::paintSelection () / FrameSelection::paint().
func PaintSelection(rv *RenderView, info *PaintInfo) {
	if rv == nil || info == nil || info.canvas == nil {
		return
	}
	rects := SelectionRects(rv)
	if len(rects) == 0 {
		return
	}
	selColor := graphics.Color{R: 0x33, G: 0x99, B: 0xFF, A: 0x66}
	// ::selection background-color (if the stylesheets declare one) overrides
	// the default highlight.
	if res := rv.Resolver(); res != nil {
		if bg, _, ok := res.SelectionColors(); ok {
			selColor = toGraphicsColor(bg)
			// A fully-opaque ::selection background is typical; keep opacity
			// as authored (the default is translucent).
			if selColor.A == 0 {
				selColor.A = 0x66
			}
		}
	}
	for _, r := range rects {
		if !info.intersects(r) {
			continue
		}
		info.canvas.FillRect(r.X, r.Y, r.Width, r.Height, selColor)
	}
}

// collapseWhitespace replaces any run of CSS whitespace (space, tab, newline,
// form feed) with a single ASCII space, matching the "white-space: normal"
// collapsing step. A leading/trailing whitespace-only segment collapses to a
// single space so that word spacing stays consistent with the layout engine's
// single-space advance.
func collapseWhitespace(s string) string {
	if !strings.ContainsAny(s, " \t\n\r\f") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inWS := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f':
			if !inWS {
				b.WriteByte(' ')
				inWS = true
			}
		default:
			inWS = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseLengthProperty parses a CSS length string like "2px" into a style.Length. It is a
// minimal parser used for outline-* properties that live in the Properties map as raw
// strings. Known keywords ("auto", "", "none") yield a zero length.
func parseLengthProperty(s string) style.Length {
	if s == "" || s == "auto" || s == "none" {
		return style.Length{}
	}
	// Split leading number from trailing unit.
	var num []byte
	var unit []byte
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		num = append(num, s[i])
		i++
	}
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		num = append(num, s[i])
		i++
	}
	unit = append(unit, []byte(s[i:])...)
	v, err := strconv.ParseFloat(string(num), 64)
	if err != nil {
		return style.Length{}
	}
	return style.Length{Value: v, Unit: string(unit)}
}

// parseColorProperty parses a CSS color string (#rgb / #rrggbb / #rrggbbaa) into a
// graphics.Color. It supports only the hex forms that the resolver emits for outline
// colors read from the Properties map. Unknown values yield transparent.
func parseColorProperty(s string) graphics.Color {
	if len(s) == 0 || s[0] != '#' {
		return graphics.Color{}
	}
	hex := s[1:]
	switch len(hex) {
	case 3:
		return graphics.Color{
			R: hexDouble(hex[0]),
			G: hexDouble(hex[1]),
			B: hexDouble(hex[2]),
			A: 0xFF,
		}
	case 6:
		return graphics.Color{
			R: hexVal(hex[0:2]),
			G: hexVal(hex[2:4]),
			B: hexVal(hex[4:6]),
			A: 0xFF,
		}
	case 8:
		return graphics.Color{
			R: hexVal(hex[0:2]),
			G: hexVal(hex[2:4]),
			B: hexVal(hex[4:6]),
			A: hexVal(hex[6:8]),
		}
	}
	return graphics.Color{}
}

// hexDouble expands a single hex digit to a byte (e.g. 'f' -> 0xFF).
func hexDouble(c byte) uint8 {
	return hexVal(string([]byte{c, c}))
}

// hexVal parses a two-digit hex string into a byte.
func hexVal(s string) uint8 {
	var v uint8
	for i := 0; i < len(s) && i < 2; i++ {
		c := s[i]
		var d uint8
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		}
		v = v<<4 | d
	}
	return v
}

// colorsEqual reports whether two style.Color values are identical. Used by
// PaintBorder to detect the uniform-color case where a single stroked rounded
// rectangle can replace four per-side fills.
func colorsEqual(a, b style.Color) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B && a.A == b.A
}

// PaintIFrame paints the child Frame's document (if any) into an <iframe>
// element's content box, mirroring RenderIFrame::paint in WebKit. The child
// frame's RenderView is laid out and painted with a translate to the content
// box origin and a clip to its size, so the embedded document clips and
// (when it has its own scroll offsets) scrolls independently of the parent.
// Returns true when a child document was painted, false otherwise (the caller
// then falls through to the default box painting).
func PaintIFrame(box *RenderBox, info *PaintInfo) bool {
	if box == nil || info == nil || info.canvas == nil {
		return false
	}
	if !box.IsVisible() {
		return false
	}
	el, ok := box.Node().(*dom.Element)
	if !ok {
		return false
	}
	sub := IFrameLookupFor(el)
	if sub == nil || sub.RenderView() == nil {
		return false
	}
	st := box.Style()
	if st == nil {
		return false
	}
	// Content box origin/size (border-box minus padding), mirroring the
	// coordinate math in PaintImage.
	pL := lengthValue(st.PaddingLeft)
	pT := lengthValue(st.PaddingTop)
	pR := lengthValue(st.PaddingRight)
	pB := lengthValue(st.PaddingBottom)
	x := box.X() + pL
	y := box.Y() + pT
	w := box.Width() - pL - pR
	h := box.Height() - pT - pB
	if w <= 0 || h <= 0 {
		return false
	}
	// Child document layout: the parent's layout pass does not lay out
	// child frames (they are independent FrameViews); ensure the child
	// viewport matches the content box and is laid out before painting.
	sub.LayoutNow()
	canvas := info.canvas
	canvas.Save()
	canvas.Translate(x, y)
	canvas.Clip(graphics.Rect{X: 0, Y: 0, Width: w, Height: h})
	Paint(sub.RenderView(), canvas, Rect{X: 0, Y: 0, Width: w, Height: h})
	canvas.Restore()
	return true
}

// PaintImage paints a decoded image for a replaced-element RenderBox (e.g.
// <img>). The image is drawn at the element's content-box position and size,
// scaled to fill the box. If the box has no decoded image data attached (e.g.
// the image has not loaded yet), nothing is painted. Returns true if an image
// was painted, false otherwise.
func PaintImage(box *RenderBox, info *PaintInfo) bool {
	if box == nil || info == nil || info.canvas == nil {
		return false
	}
	if !box.IsVisible() {
		return false
	}
	img := box.DecodedImage()
	st := box.Style()
	if st == nil {
		return false
	}
	// Draw at the content-box position (border-box + padding offset).
	pL := lengthValue(st.PaddingLeft)
	pT := lengthValue(st.PaddingTop)
	pR := lengthValue(st.PaddingRight)
	pB := lengthValue(st.PaddingBottom)
	x := box.X() + pL
	y := box.Y() + pT
	w := box.Width() - pL - pR
	h := box.Height() - pT - pB
	if w <= 0 || h <= 0 {
		return false
	}
	// Lazy-load from the src attribute: raster images decode synchronously
	// (data/file) or asynchronously (http, via the shared background-image
	// cache); SVG sources parse and paint as vectors.
	var src string
	if el, ok := box.Node().(*dom.Element); ok {
		src = el.GetAttribute("src")
	}
	if (img == nil || !img.Loaded()) && src != "" {
		img = loadBackgroundImage(src, "")
		if img != nil && img.Loaded() {
			box.SetDecodedImage(img)
		} else if sd := loadBackgroundSVG(src); sd != nil {
			paintSVGScaled(info.canvas, sd, x, y, w, h)
			return true
		}
	}
	if img == nil || !img.Loaded() {
		// Fallback: draw the alt text (if any) like a broken-image state.
		if el, ok := box.Node().(*dom.Element); ok {
			if alt := el.GetAttribute("alt"); alt != "" {
				font := toGraphicsFont(st)
				altCol := toGraphicsColor(st.Color)
				if altCol.A == 0 {
					altCol = graphics.Color{R: 96, G: 96, B: 96, A: 255}
				}
				ascent := info.canvas.FontAscent(font)
				info.canvas.DrawText(x, y+ascent, alt, font, altCol)
			}
		}
		return false
	}
	// object-fit (default fill) controls how the image scales into the
	// content box, mirroring CSS Images §3. object-position (default 50%
	// 50%) controls alignment within the box.
	iw, ih := float64(img.Width()), float64(img.Height())
	fit := strings.ToLower(st.GetProperty("object-fit"))
	opx, opy := parseObjectPosition(st.GetProperty("object-position"))
	switch fit {
	case "cover":
		if iw > 0 && ih > 0 {
			ratio := w / iw
			if h/ih > ratio {
				ratio = h / ih
			}
			dw, dh := iw*ratio, ih*ratio
			img.Draw(info.canvas, x+(w-dw)*opx, y+(h-dh)*opy, dw, dh)
		} else {
			img.Draw(info.canvas, x, y, w, h)
		}
	case "contain":
		if iw > 0 && ih > 0 {
			ratio := w / iw
			if h/ih < ratio {
				ratio = h / ih
			}
			dw, dh := iw*ratio, ih*ratio
			img.Draw(info.canvas, x+(w-dw)*opx, y+(h-dh)*opy, dw, dh)
		} else {
			img.Draw(info.canvas, x, y, w, h)
		}
	case "none":
		if iw > 0 && ih > 0 {
			img.Draw(info.canvas, x+(w-iw)*opx, y+(h-ih)*opy, iw, ih)
		} else {
			img.Draw(info.canvas, x, y, w, h)
		}
	default: // fill (stretch to the content box)
		img.Draw(info.canvas, x, y, w, h)
	}
	return true
}

// parseObjectPosition parses an object-position value ("left top",
// "30% 60%", "10px 20px", …). Missing parts default to 50%. Returns the
// horizontal and vertical ratios (0..1) usable as (space * ratio) offsets.
func parseObjectPosition(s string) (float64, float64) {
	ox, oy := 0.5, 0.5
	parts := strings.Fields(strings.ToLower(s))
	if len(parts) >= 1 {
		ox = parsePosPart(parts[0])
	}
	if len(parts) >= 2 {
		oy = parsePosPart(parts[1])
	} else if len(parts) == 1 {
		// One value: vertical = horizontal.
		oy = ox
	}
	return ox, oy
}

// parsePosPart parses a single object-position component: a percentage
// ("30%" → 0.3) or a keyword (left/top=0, center=0.5, right/bottom=1).
func parsePosPart(s string) float64 {
	if strings.HasSuffix(s, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64); err == nil {
			return v / 100
		}
		return 0.5
	}
	switch s {
	case "left", "top":
		return 0
	case "right", "bottom":
		return 1
	case "center":
		return 0.5
	}
	return 0.5
}
