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
	"strconv"
	"strings"

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
	rect := rectFromLayout(box.X(), box.Y(), box.Width(), box.Height())
	if !info.intersects(rect) {
		return
	}
	// Paint box-shadow before the background (shadows sit behind the element).
	// Paint shadows even when the background is transparent. Inset shadows are
	// excluded here — they paint ABOVE the background (see below).
	if st.BoxShadow != "" && st.BoxShadow != "none" {
		r := lengthValue(st.BorderRadius)
		shadows := parseShadowList(st.BoxShadow)
		op := CumulativeOpacity(box)
		paintBoxShadow(info.canvas, box.X(), box.Y(), box.Width(), box.Height(), r, shadows, op, false)
	}
	// background-image: url(...) — decode and draw with size/position.
	if url, ok := parseBackgroundURL(st.BackgroundImage); ok {
		img := loadBackgroundImage(url, "")
		if img != nil && img.Loaded() {
			dx, dy, dw, dh := computeBackgroundDest(rect.X, rect.Y, rect.Width, rect.Height,
				st.BackgroundSize, st.BackgroundPosition, img.Width(), img.Height())
			img.Draw(info.canvas, dx, dy, dw, dh)
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
		drawGradientLayer := func(i int) {
			layer := bgLayers[i]
			if lg := parseGradient(layer); lg != nil {
				if r > 0 {
					info.canvas.Save()
					info.canvas.ClipRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r)
					paintLinearGradient(info.canvas, rect.X, rect.Y, rect.Width, rect.Height, lg)
					info.canvas.Restore()
				} else {
					paintLinearGradient(info.canvas, rect.X, rect.Y, rect.Width, rect.Height, lg)
				}
				return
			}
			if rg := parseRadialGradient(layer); rg != nil {
				if r > 0 {
					info.canvas.Save()
					info.canvas.ClipRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r)
					paintRadialGradient(info.canvas, rect.X, rect.Y, rect.Width, rect.Height, rg)
					info.canvas.Restore()
				} else {
					paintRadialGradient(info.canvas, rect.X, rect.Y, rect.Width, rect.Height, rg)
				}
			}
		}
		// Color beneath the layers.
		if bgc := toGraphicsColor(st.BackgroundColor); bgc.A != 0 {
			bgc = ApplyOpacityToColor(bgc, CumulativeOpacity(box))
			if r > 0 {
				info.canvas.FillRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r, bgc)
			} else {
				info.canvas.FillRect(rect.X, rect.Y, rect.Width, rect.Height, bgc)
			}
		}
		// Layers bottom-up (last layer first, first layer painted last = on top).
		for i := len(bgLayers) - 1; i >= 0; i-- {
			drawGradientLayer(i)
		}
		return
	}
	bg := toGraphicsColor(st.BackgroundColor)
	if bg.A == 0 {
		return
	}
// Debug: log non-trivial background paints
	if false && (bg.R != 0 || bg.G != 0 || bg.B != 0) {
		elName := ""
		if box.Node() != nil {
			if el, ok := box.Node().(*dom.Element); ok {
				elName = el.LocalName() + "." + el.ClassName()
			}
		}
		log.Printf("[dbg/paintbg] %s rect=(%.0f,%.0f %.0fx%.0f) col=#%02x%02x%02x alpha=%d",
			elName, rect.X, rect.Y, rect.Width, rect.Height, bg.R, bg.G, bg.B, bg.A)
	}
	bg = ApplyOpacityToColor(bg, CumulativeOpacity(box))
	if bg.A == 0 {
		return
	}
	if r := lengthValue(st.BorderRadius); r > 0 {
		info.canvas.FillRoundRect(rect.X, rect.Y, rect.Width, rect.Height, r, bg)
	} else {
		info.canvas.FillRect(rect.X, rect.Y, rect.Width, rect.Height, bg)
	}
	// Inset shadows paint ABOVE the background (below the border): inset 6px
	// left shadow casts onto the element's own background.
	if st.BoxShadow != "" && st.BoxShadow != "none" {
		shadows := parseShadowList(st.BoxShadow)
		op := CumulativeOpacity(box)
		paintBoxShadow(info.canvas, box.X(), box.Y(), box.Width(), box.Height(), lengthValue(st.BorderRadius), shadows, op, true)
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
	x, y := box.X(), box.Y()
	w, h := box.Width(), box.Height()
	topW := lengthValue(st.BorderTopWidth)
	rightW := lengthValue(st.BorderRightWidth)
	bottomW := lengthValue(st.BorderBottomWidth)
	leftW := lengthValue(st.BorderLeftWidth)
	if topW <= 0 && rightW <= 0 && bottomW <= 0 && leftW <= 0 {
		return
	}
	if !info.intersects(rectFromLayout(x, y, w, h)) {
		return
	}
	op := CumulativeOpacity(box)
	// When border-radius is set, draw the border as a single stroked rounded
	// rectangle so the corners follow the curve. Using FillRect sides here
	// would paint sharp rectangular corners that cover the rounded background
	// produced by FillRoundRect in PaintBackground. The uniform-width,
	// uniform-color case (e.g. `border: 2px solid #e5e7eb; border-radius: 4px`)
	// is by far the most common, so it is handled directly; unequal sides
	// fall back to the per-side FillRect path below.
	btC, brC, bbC, blC := st.BorderColor("top"), st.BorderColor("right"), st.BorderColor("bottom"), st.BorderColor("left")
	if r := lengthValue(st.BorderRadius); r > 0 &&
		topW == rightW && rightW == bottomW && bottomW == leftW &&
		st.BorderTopStyle != "none" && st.BorderRightStyle != "none" &&
		st.BorderBottomStyle != "none" && st.BorderLeftStyle != "none" &&
		colorsEqual(btC, brC) &&
		colorsEqual(brC, bbC) &&
		colorsEqual(bbC, blC) {
		info.canvas.StrokeRoundRect(x, y, w, h, r, topW, ApplyOpacityToColor(toGraphicsColor(btC), op))
		return
	}
	// Top and bottom span the full width, including the corners.
	if topW > 0 && st.BorderTopStyle != "none" {
		paintBorderSide(info.canvas, x, y, w, topW, ApplyOpacityToColor(toGraphicsColor(btC), op), st.BorderTopStyle)
	}
	if bottomW > 0 && st.BorderBottomStyle != "none" {
		paintBorderSide(info.canvas, x, y+h-bottomW, w, bottomW, ApplyOpacityToColor(toGraphicsColor(bbC), op), st.BorderBottomStyle)
	}
	// Left and right exclude the top/bottom border regions so the corner color (top/bottom)
	// is preserved.
	midY := y + topW
	midH := h - topW - bottomW
	if midH <= 0 {
		return
	}
	if leftW > 0 && st.BorderLeftStyle != "none" {
		paintBorderSide(info.canvas, x, midY, leftW, midH, ApplyOpacityToColor(toGraphicsColor(blC), op), st.BorderLeftStyle)
	}
	if rightW > 0 && st.BorderRightStyle != "none" {
		paintBorderSide(info.canvas, x+w-rightW, midY, rightW, midH, ApplyOpacityToColor(toGraphicsColor(brC), op), st.BorderRightStyle)
	}
	// Corner bevels: when adjacent border colors differ, browsers split the
	// corner along the diagonal from the outer corner to the inner corner
	// (CSS border corner joining). The triangle on the horizontal-edge side
	// keeps the top/bottom color; the other triangle gets the left/right
	// color. Mirrors Edge pixel-for-pixel within anti-aliasing tolerance.
	paintBorderCorners(info.canvas, x, y, w, h, topW, rightW, bottomW, leftW,
		blC, brC, btC, bbC, op, st)
}

// paintBorderCorners fills the 45°-beveled corner triangles for corners where
// the adjacent vertical and horizontal border colors differ.
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
	olStyle := st.GetProperty("outline-style")
	if olStyle == "" || olStyle == "none" {
		return
	}
	width := lengthValue(parseLengthProperty(st.GetProperty("outline-width")))
	if width <= 0 {
		width = 1
	}
	color := parseColorProperty(st.GetProperty("outline-color"))
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
func PaintText(text *RenderText, info *PaintInfo) {
	if text == nil || info == nil || info.canvas == nil {
		return
	}
	st := text.Style()
	if st == nil {
		return
	}
	col := toGraphicsColor(st.Color)
	if col.A == 0 {
		return
	}
	col = ApplyOpacityToColor(col, CumulativeOpacity(text))
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
	runes := []rune(content)
	rv := info.rv
	// Browsers default selected text to white so it is legible against the
	// semi-transparent blue selection background.
	selCol := graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	// Paint text-shadow: draw the text once per shadow in the shadow color.
	textShadows := parseShadowList(st.TextShadow)
	if len(textShadows) > 0 {
		opacity := CumulativeOpacity(text)
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
			continue
		}
		baseline := seg.Y + ascent

	// ── text-overflow:ellipsis ──
		// Walk segments sequentially from the left. Track cumulative width from the
		// content box start. Once a segment's right edge exceeds the available width
		// (content-box-width minus ellipsis-width), truncate it character-by-character,
		// append "..." using the same font, and skip all remaining segments.
		if toCB != nil {
			segRelX := seg.X - toCB.X // segment X relative to content box origin
			segRight := segRelX + seg.Width
			maxTextRight := toCB.Width - textEllipsisW // max allowed from content box origin

			if segRight > maxTextRight {
				// This segment overflows into the "..." zone.
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
	if img == nil || !img.Loaded() {
		return false
	}
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
	img.Draw(info.canvas, x, y, w, h)
	return true
}
