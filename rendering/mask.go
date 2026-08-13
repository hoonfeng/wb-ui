// CSS mask-image application at the layer level. Mirrors the mask-image
// semantics of CSS Masking Level 1: the mask image's alpha (or luminance)
// selects which parts of the element — including its foreground text and
// descendants — survive. Unlike the old background-only mask, this runs at the
// RenderLayer level so it wraps the whole subtree.

package rendering

import (
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// hasMaskImage reports whether the style declares a usable mask-image (a
// url(...) reference that is not "none"). It is true even when the referenced
// image/mask is not yet loaded (applyMaskLayer then no-ops), so the offscreen
// layer is still established to keep the paint ordering correct.
func hasMaskImage(st *style.ComputedStyle) bool {
	if st == nil {
		return false
	}
	mv := st.GetProperty("mask-image")
	if mv == "" || strings.EqualFold(strings.TrimSpace(mv), "none") {
		return false
	}
	murl, ok := parseBackgroundURL(mv)
	if !ok {
		return false
	}
	return murl != ""
}

// maskImageFor resolves a plain image mask-image source (url(...) pointing at a
// raster image or data URI). Returns nil when no mask-image is set / it is
// "none" / it cannot be loaded. SVG <mask> references (url(file.svg#id)) are
// handled separately by applyMaskLayer because they need the target box size.
func maskImageFor(st *style.ComputedStyle) *DecodedImage {
	if st == nil {
		return nil
	}
	murl, ok := parseBackgroundURL(st.GetProperty("mask-image"))
	if !ok {
		return nil
	}
	if strings.Contains(murl, "#") {
		return nil // SVG mask reference, resolved by applyMaskLayer
	}
	img := loadBackgroundImage(murl, "")
	if img == nil || !img.Loaded() {
		return nil
	}
	return img
}

// maskLuminanceFor resolves whether the mask value is extracted as luminance,
// honoring CSS mask-mode and (for SVG <mask> sources) the mask element's
// mask-type. match-source (the default) uses alpha for images and mask-type for
// SVG masks.
func maskLuminanceFor(mode string, isSVG bool, svgMaskType string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "luminance":
		return true
	case "alpha":
		return false
	default: // match-source
		if isSVG {
			return !strings.EqualFold(svgMaskType, "alpha") // mask-type: luminance (default)
		}
		return false // image default: alpha
	}
}

// maskGeometry resolves the mask destination rect (first tile) inside rect,
// honoring mask-size (auto/cover/contain/length/%) and mask-position. Reuses
// the background-image geometry (CSS mask-size/position mirror background-*).
func maskGeometry(rect graphics.Rect, size, position string, imgW, imgH int) (dx, dy, dw, dh float64) {
	return computeBackgroundDest(rect.X, rect.Y, rect.Width, rect.Height, size, position, imgW, imgH)
}

// applyMaskLayer masks the already-painted layer content (inside a preceding
// SaveLayerForMask) using the owner's mask-image. Supports two source kinds:
//   - raster image / data URI (url(...)) honoring mask-size/repeat/position/
//     mask-mode (alpha | luminance);
//   - SVG <mask> element (url(file.svg#id)) whose region is already resolved
//     against the target box by renderSVGMask; its mask-type (luminance default
//     | alpha) selects the mask value.
func applyMaskLayer(canvas *graphics.Canvas, st *style.ComputedStyle, rect graphics.Rect, ownerEl *dom.Element) {
	if canvas == nil || st == nil {
		return
	}
	murl, ok := parseBackgroundURL(st.GetProperty("mask-image"))
	if !ok {
		return
	}

	// SVG <mask> source: url(file.svg#maskId) or url(#maskId).
	if i := strings.LastIndex(murl, "#"); i >= 0 {
		fileURL, maskID := murl[:i], murl[i+1:]
		// Same-document reference: mask-image: url(#id) points at an inline
		// <mask id="id"> element within the same document (e.g. inside an
		// inline <svg><defs>). Resolve it through the owner's document.
		if fileURL == "" && ownerEl != nil {
			if maskEl := ownerEl.OwnerDocument().GetElementById(maskID); maskEl != nil {
				if strings.EqualFold(maskEl.LocalName(), "mask") {
					if m := parseMaskElement(maskEl); m != nil {
						if img := renderSVGMask(m, rect.Width, rect.Height); img != nil {
							luminance := maskLuminanceFor(st.GetProperty("mask-mode"), true, m.maskType)
							canvas.ApplyImageMaskMode(img.SkiaImage(), rect, luminance)
							img.Release()
							return
						}
					}
				}
			}
		}
		if doc := loadBackgroundSVG(fileURL); doc != nil {
			if m := doc.masks[maskID]; m != nil {
				if img := renderSVGMask(m, rect.Width, rect.Height); img != nil {
					luminance := maskLuminanceFor(st.GetProperty("mask-mode"), true, m.maskType)
					canvas.ApplyImageMaskMode(img.SkiaImage(), rect, luminance)
					img.Release()
					return
				}
			}
		}
		// Fall through: a "#"-bearing URL that is not an SVG mask reference
		// (e.g. a fragment on a raster image) is treated as a plain image.
	}

	img := maskImageFor(st)
	if img == nil {
		return
	}
	size := st.GetProperty("mask-size")
	position := st.GetProperty("mask-position")
	repeat := st.GetProperty("mask-repeat")
	mode := st.GetProperty("mask-mode")
	luminance := maskLuminanceFor(mode, false, "")

	dx, dy, dw, dh := maskGeometry(rect, size, position, img.Width(), img.Height())
	if dw <= 0 || dh <= 0 {
		return
	}
	rep := strings.ToLower(strings.TrimSpace(repeat))
	repX := rep != "no-repeat" && rep != "repeat-y"
	repY := rep != "no-repeat" && rep != "repeat-x"

	// Effective area: repeat axes span the whole rect; no-repeat axes cover
	// only the first tile.
	effX, effY, effW, effH := rect.X, rect.Y, rect.Width, rect.Height
	if !repX {
		effX, effW = dx, dw
	}
	if !repY {
		effY, effH = dy, dh
	}
	effRect := graphics.Rect{X: effX, Y: effY, Width: effW, Height: effH}
	tileRect := graphics.Rect{X: dx, Y: dy, Width: dw, Height: dh}

	// Clip to the effective area and paint the image shader (repeat) there;
	// the no-repeat axes are confined to the single tile by the clip.
	canvas.Save()
	canvas.Clip(effRect)
	canvas.ApplyImageMaskTiled(img.SkiaImage(), effRect, tileRect, graphics.TileModeRepeat, graphics.TileModeRepeat, luminance)
	canvas.Restore()

	// Clear the region inside rect but outside the effective area (mask = 0).
	clearOutsideRect(canvas, rect, effRect)
}

// clearOutsideRect erases the four bands of outer that lie outside inner
// (top/bottom/left/right). The corner overlaps are cleared twice, which is
// harmless for a clear operation.
func clearOutsideRect(canvas *graphics.Canvas, outer, inner graphics.Rect) {
	if canvas == nil {
		return
	}
	if inner.Y > outer.Y {
		canvas.ClearRect(outer.X, outer.Y, outer.Width, inner.Y-outer.Y)
	}
	if b := inner.Y + inner.Height; b < outer.Y+outer.Height {
		canvas.ClearRect(outer.X, b, outer.Width, outer.Y+outer.Height-b)
	}
	if inner.X > outer.X {
		canvas.ClearRect(outer.X, inner.Y, inner.X-outer.X, inner.Height)
	}
	if r := inner.X + inner.Width; r < outer.X+outer.Width {
		canvas.ClearRect(r, inner.Y, outer.X+outer.Width-r, inner.Height)
	}
}
