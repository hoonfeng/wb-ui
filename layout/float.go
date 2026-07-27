// Translation of: Source/WebCore/layout/floats/PlacedFloats.h
//                  Source/WebCore/layout/floats/FloatingContext.h
//                  Source/WebCore/layout/floats/FloatingContext.cpp
//                  Source/WebCore/rendering/FloatingObjects.cpp
// Completeness: 45%
// Simplifications:
//   - no subpixel layout (integer pixels only)
//   - no pagination/fragmentation
//   - WebKit models floats with a per-BFC FloatingState of placed float geometry plus
//     a FloatingContext that answers avoider geometry queries; this port collapses the
//     two into a single floatContext that keeps an ordered slice of placed floats and
//     answers the two queries the block layout needs: "where does the next float go?"
//     and "what is the available content width at a given y?"
//   - shape-outside, clearfix and float intrusions into descendants are omitted
//   - floats do not stack into multiple "lines" with the full BFC algorithm; instead
//     the next-float positioner greedily packs floats top-to-bottom, left/right

package layout

import "math"

// placedFloat records a single floated box that has been positioned by the block
// formatting context. Coordinates are relative to the formatting context root.
type placedFloat struct {
	box    *LayoutBox
	left   bool
	x, y   float64
	w, h   float64
}

// floatContext tracks the floats placed in a block formatting context so that
// subsequent in-flow content can be shortened to avoid overlaps. It mirrors the
// combination of WebKit FloatingState + FloatingContext.
type floatContext struct {
	floats []placedFloat
	// originX / originY is the top-left of the containing-block content area in the
	// coordinate space of the formatting context root.
	originX float64
	originY float64
	// contentWidth is the width of the containing-block content area; floats must
	// remain within [originX, originX+contentWidth].
	contentWidth float64
}

// newFloatContext constructs an empty float context anchored at the given content box.
func newFloatContext(originX, originY, contentWidth float64) *floatContext {
	return &floatContext{originX: originX, originY: originY, contentWidth: contentWidth}
}

// placeFloat positions a floated box. The float is stacked below existing floats on
// its side until it fits within the remaining content width, mirroring the CSS 2.1
// left/right float placement rules (simplified: no negative margins, no clearance
// collapsing). The float own width must already be computed by the caller.
// startY is the FC-relative y at which this float should be placed (typically the
// container's contentY minus fc.originY). Using the correct startY ensures floats
// from the same container share the same y level for proper horizontal stacking.
func (fc *floatContext) placeFloat(box *LayoutBox, left bool, width, height, startY float64) (x, y float64) {
	// Start at the given FC-relative y instead of fc.originY so that floats
	// from the same container share the same y level for collision detection.
	y = startY
	for {
		// Compute the available horizontal range at y for a float on the given side.
		leftEdge, rightEdge := fc.contentEdgesAt(y)
		avail := rightEdge - leftEdge
		if width <= avail+1e-6 {
			// Fits on this line.
			if left {
				x = leftEdge
			} else {
				x = rightEdge - width
			}
			break
		}
		// Does not fit: drop to the lowest bottom among floats that overlap y.
		next := fc.lowestBottomAbove(y)
		if next <= y {
			// No further floats to clear; place at the content edge.
			if left {
				x = fc.originX
			} else {
				x = fc.originX + fc.contentWidth - width
			}
			break
		}
		y = next
	}
	fc.floats = append(fc.floats, placedFloat{box: box, left: left, x: x, y: y, w: width, h: height})
	return x, y
}

// contentEdgesAt returns the [leftX, rightX) range of the content area at the given y
// after subtracting any floats that overlap y. When no floats overlap, the result is
// the full content area [originX, originX+contentWidth].
func (fc *floatContext) contentEdgesAt(y float64) (leftX, rightX float64) {
	leftX = fc.originX
	rightX = fc.originX + fc.contentWidth
	for _, f := range fc.floats {
		if y < f.y || y >= f.y+f.h {
			continue
		}
		if f.left {
			if f.x+f.w > leftX {
				leftX = f.x + f.w
			}
		} else {
			if f.x < rightX {
				rightX = f.x
			}
		}
	}
	if leftX > rightX {
		leftX = rightX
	}
	return leftX, rightX
}

// availableWidthAt returns the content width available at y after subtracting floats.
func (fc *floatContext) availableWidthAt(y float64) float64 {
	l, r := fc.contentEdgesAt(y)
	if r < l {
		return 0
	}
	return r - l
}

// lowestBottomAbove returns the smallest float bottom that is strictly greater than y,
// used to advance the float placer when a float does not fit at the current y.
func (fc *floatContext) lowestBottomAbove(y float64) float64 {
	best := math.Inf(1)
	for _, f := range fc.floats {
		if f.y >= y && f.y < best {
			best = f.y
		}
		if f.y+f.h > y && f.y+f.h < best {
			// Allow dropping to a float bottom as well.
			b := f.y + f.h
			if b > y && b < best {
				best = b
			}
		}
	}
	return best
}

// clearedY returns the y-coordinate at which content with the given clear side may
// resume. clearSide is "left", "right", "both" or "" (no clearance).
func (fc *floatContext) clearedY(y float64, clearSide string) float64 {
	if clearSide == "" {
		return y
	}
	for _, f := range fc.floats {
		if clearSide != "both" {
			isLeft := f.left
			if (clearSide == "left") != isLeft {
				continue
			}
		}
		if b := f.y + f.h; b > y {
			y = b
		}
	}
	return y
}

// maxFloatBottom returns the lowest bottom among all placed floats, or originY when no
// floats exist. Used to size the block container height when floats are present.
func (fc *floatContext) maxFloatBottom() float64 {
	best := fc.originY
	for _, f := range fc.floats {
		if b := f.y + f.h; b > best {
			best = b
		}
	}
	return best
}

// isEmpty reports whether any floats have been placed.
func (fc *floatContext) isEmpty() bool { return len(fc.floats) == 0 }