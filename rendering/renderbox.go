// Translation of: Source/WebCore/rendering/RenderBox.h
//                  Source/WebCore/rendering/RenderBox.cpp
//                  Source/WebCore/rendering/RenderBoxModelObject.h
// Completeness: 55%
// Simplifications:
//   - no subpixel layout (float64 used internally, rounded on read)
//   - no pagination/fragmentation
//   - RenderBoxModelObject is folded into RenderBox; the intermediate layer that
//     holds inline-box painting state is omitted since inline boxes are represented by
//     RenderInline which does not generate a box
//   - the overflow tracking (RenderOverflow) is omitted; the visual / layout overflow
//     rects are not maintained separately
//   - the scrollable-area / scrollbar integration is omitted
//   - box geometry reuses layout.LayoutRect / layout.Edges so the render tree and the
//     layout tree share a single geometry representation

package rendering

import (
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderBox is the Go translation of WebCore::RenderBox. It extends RenderObject with the
// CSS box model: a border-box frame rectangle plus the surrounding margin and the inner
// padding/border edges. All geometry is stored on a layout.LayoutRect so the layout
// engine and the render tree share a single representation.
type RenderBox struct {
	renderObjectBase
	frame        layout.LayoutRect
	decodedImage *DecodedImage
}

// NewRenderBox constructs a RenderBox for the given DOM node with the given computed
// style. The box is not attached to a parent; use AddChild to insert it into the tree.
func NewRenderBox(node dom.Node, st *style.ComputedStyle) *RenderBox {
	rb := &RenderBox{}
	rb.initBase(rb, node, st)
	return rb
}

// Type returns ObjectBox.
func (b *RenderBox) Type() RenderObjectType { return ObjectBox }

// IsBox reports that this object generates a box, mirroring RenderObject::isBox().
func (b *RenderBox) IsBox() bool { return true }

// AsRenderBox returns the RenderBox pointer for any box-bearing render object.
// This is needed because Go type assertion cannot cast *RenderBlockFlow → *RenderBox
// (they are different concrete types despite embedding). By implementing this method
// on RenderBox, it gets promoted to all embedding types (RenderBlock, RenderBlockFlow,
// RenderView) so asRenderBox can recover the *RenderBox via interface assertion.
func (b *RenderBox) AsRenderBox() *RenderBox { return b }

// X / Y / Width / Height mirror RenderBox::x() / y() / width() / height().
func (b *RenderBox) X() float64 { return b.frame.X }
func (b *RenderBox) Y() float64 { return b.frame.Y }

// Width returns the border-box width.
func (b *RenderBox) Width() float64 { return b.frame.Width }

// Height returns the border-box height.
func (b *RenderBox) Height() float64 { return b.frame.Height }

// SetX / SetY / SetWidth / SetHeight mirror RenderBox::setX() / setY() / etc.
func (b *RenderBox) SetX(x float64)      { b.frame.X = x }
func (b *RenderBox) SetY(y float64)      { b.frame.Y = y }
func (b *RenderBox) SetWidth(w float64)  { b.frame.Width = w }
func (b *RenderBox) SetHeight(h float64) { b.frame.Height = h }

// AbsoluteX returns the absolute X position (sum of all ancestor origins).
// This is the left edge of the border box in the render tree's coordinate
// space, matching what HitTest uses for coordinate comparison.
func (b *RenderBox) AbsoluteX() float64 {
	x := b.X()
	for p := b.Parent(); p != nil; p = p.Parent() {
		if bp, ok := p.(*RenderBox); ok {
			x += bp.X()
		}
	}
	return x
}

// AbsoluteY returns the absolute Y position (sum of all ancestor origins).
func (b *RenderBox) AbsoluteY() float64 {
	y := b.Y()
	for p := b.Parent(); p != nil; p = p.Parent() {
		if bp, ok := p.(*RenderBox); ok {
			y += bp.Y()
		}
	}
	return y
}

// SetLocation sets the border-box origin, mirroring RenderBox::setLocation().
func (b *RenderBox) SetLocation(x, y float64) {
	b.frame.X = x
	b.frame.Y = y
}

// SetSize sets the border-box dimensions, mirroring RenderBox::setSize().
func (b *RenderBox) SetSize(w, h float64) {
	b.frame.Width = w
	b.frame.Height = h
}

// FrameRect returns the border-box frame rectangle, mirroring RenderBox::frameRect().
func (b *RenderBox) FrameRect() layout.LayoutRect { return b.frame }

// SetFrameRect replaces the border-box frame rectangle, mirroring
// RenderBox::setFrameRect().
func (b *RenderBox) SetFrameRect(r layout.LayoutRect) { b.frame = r }

// DecodedImage returns the decoded image attached to this box (nil if none),
// used by PaintImage to render <img> elements and CSS background images.
func (b *RenderBox) DecodedImage() *DecodedImage { return b.decodedImage }

// SetDecodedImage attaches a decoded image to this box. The caller retains
// ownership of the DecodedImage; the RenderBox does not release it.
func (b *RenderBox) SetDecodedImage(img *DecodedImage) { b.decodedImage = img }

// Margin returns the margin edges, mirroring RenderBox::marginBoxRect() sides.
func (b *RenderBox) Margin() layout.Edges { return b.frame.Margin }

// SetMargin sets the margin edges.
func (b *RenderBox) SetMargin(m layout.Edges) { b.frame.Margin = m }

// Padding returns the padding edges.
func (b *RenderBox) Padding() layout.Edges { return b.frame.Padding }

// SetPadding sets the padding edges.
func (b *RenderBox) SetPadding(p layout.Edges) { b.frame.Padding = p }

// Border returns the border edges.
func (b *RenderBox) Border() layout.Edges { return b.frame.Border }

// SetBorder sets the border edges.
func (b *RenderBox) SetBorder(br layout.Edges) { b.frame.Border = br }

// BorderBoxRect returns the border-box rectangle (origin at 0,0, size = frame size),
// mirroring RenderBox::borderBoxRect().
func (b *RenderBox) BorderBoxRect() layout.LayoutRect {
	return layout.LayoutRect{
		X:      b.frame.X,
		Y:      b.frame.Y,
		Width:  b.frame.Width,
		Height: b.frame.Height,
	}
}

// PaddingBoxRect returns the padding box rectangle (border-box inset by border),
// mirroring RenderBox::paddingBoxRect().
func (b *RenderBox) PaddingBoxRect() layout.LayoutRect {
	return layout.LayoutRect{
		X:      b.frame.X + b.frame.Border.Left,
		Y:      b.frame.Y + b.frame.Border.Top,
		Width:  b.frame.Width - b.frame.Border.Horizontal(),
		Height: b.frame.Height - b.frame.Border.Vertical(),
	}
}

// ContentBoxRect returns the content box rectangle (padding box inset by padding),
// mirroring RenderBox::contentBoxRect().
func (b *RenderBox) ContentBoxRect() layout.LayoutRect {
	return layout.LayoutRect{
		X:      b.frame.ContentX(),
		Y:      b.frame.ContentY(),
		Width:  b.frame.ContentWidth(),
		Height: b.frame.ContentHeight(),
	}
}

// ContentBoxLocation returns the content box origin, mirroring
// RenderBox::contentBoxLocation().
func (b *RenderBox) ContentBoxLocation() (x, y float64) {
	return b.frame.ContentX(), b.frame.ContentY()
}

// MarginBoxRect returns the margin box rectangle, mirroring RenderBox::marginBoxRect().
func (b *RenderBox) MarginBoxRect() layout.LayoutRect {
	return layout.LayoutRect{
		X:      b.frame.X - b.frame.Margin.Left,
		Y:      b.frame.Y - b.frame.Margin.Top,
		Width:  b.frame.Width + b.frame.Margin.Horizontal(),
		Height: b.frame.Height + b.frame.Margin.Vertical(),
	}
}

// ContainingBlock returns the containing block for this box, mirroring
// RenderBox::containingBlock(). It delegates to the base containing-block resolver.
func (b *RenderBox) ContainingBlock() RenderObject {
	return b.containingBlockBase()
}

// ParentBox returns the parent cast to *RenderBox, or nil, mirroring
// RenderBox::parentBox().
func (b *RenderBox) ParentBox() *RenderBox {
	if b.parent == nil {
		return nil
	}
	if pb, ok := b.parent.(*RenderBox); ok {
		return pb
	}
	return nil
}

// FirstChildBox returns the first child cast to *RenderBox, mirroring
// RenderBox::firstChildBox().
func (b *RenderBox) FirstChildBox() *RenderBox {
	if b.firstChild == nil {
		return nil
	}
	if cb, ok := b.firstChild.(*RenderBox); ok {
		return cb
	}
	return nil
}

// LastChildBox returns the last child cast to *RenderBox, mirroring
// RenderBox::lastChildBox().
func (b *RenderBox) LastChildBox() *RenderBox {
	if b.lastChild == nil {
		return nil
	}
	if cb, ok := b.lastChild.(*RenderBox); ok {
		return cb
	}
	return nil
}

// NextSiblingBox returns the next sibling cast to *RenderBox, mirroring
// RenderBox::nextSiblingBox().
func (b *RenderBox) NextSiblingBox() *RenderBox {
	if b.nextSibling == nil {
		return nil
	}
	if sb, ok := b.nextSibling.(*RenderBox); ok {
		return sb
	}
	return nil
}

// PreviousSiblingBox returns the previous sibling cast to *RenderBox, mirroring
// RenderBox::previousSiblingBox().
func (b *RenderBox) PreviousSiblingBox() *RenderBox {
	if b.prevSibling == nil {
		return nil
	}
	if sb, ok := b.prevSibling.(*RenderBox); ok {
		return sb
	}
	return nil
}

// IsAbsolutelyPositioned reports whether this box is out-of-flow positioned, mirroring
// RenderBox::isAbsolutelyPositioned().
func (b *RenderBox) IsAbsolutelyPositioned() bool {
	if b.style == nil {
		return false
	}
	return b.style.Position == style.PositionAbsolute || b.style.Position == style.PositionFixed
}

// IsRelativelyPositioned reports whether this box is relatively positioned, mirroring
// RenderBox::isRelativelyPositioned() (which also includes sticky).
func (b *RenderBox) IsRelativelyPositioned() bool {
	return b.style != nil && (b.style.Position == style.PositionRelative || b.style.Position == style.PositionSticky)
}

// IsStickyPositioned reports whether this box uses position: sticky.
func (b *RenderBox) IsStickyPositioned() bool {
	return b.style != nil && b.style.Position == style.PositionSticky
}

// IsFloated reports whether this box is floated, mirroring RenderBox::isFloating().
func (b *RenderBox) IsFloated() bool {
	return b.style != nil && (b.style.Float == "left" || b.style.Float == "right")
}

// IsInFlow reports whether this box participates in normal flow, mirroring
// RenderBox::isInFlow().
func (b *RenderBox) IsInFlow() bool {
	return !b.IsFloated() && !b.IsAbsolutelyPositioned()
}

// IsVisible reports whether this box should be rendered, mirroring
// RenderBox::isVisible(). display:none suppresses layout/paint entirely;
// visibility:hidden keeps the box in layout (occupies space) but skips paint.
// Both must be honoured here — the painter calls IsVisible before drawing
// background/border/text.
func (b *RenderBox) IsVisible() bool {
	return b.style == nil || (b.style.Display != style.DisplayNone && b.style.Visibility == "visible")
}

// RenderName returns a debug name for the box.
func (b *RenderBox) RenderName() string { return "RenderBox" }

// Layout lays out the box by delegating to the layout engine. If the box has an
// associated layout box, the appropriate formatting context is dispatched based on the
// box's display property; otherwise this is a no-op. This mirrors RenderBox::layout().
func (b *RenderBox) Layout(state *layout.LayoutState) {
	if b.layoutBox == nil || state == nil {
		return
	}
	ctx := contextForBox(b.layoutBox)
	ctx.Layout(b.layoutBox, state)
	b.ClearNeedsLayout()
}

// contextForBox selects the formatting context for a layout box based on its display.
// This mirrors the layout package's internal contextFor but is exposed here so the
// render tree can dispatch layout without reaching into unexported layout internals.
func contextForBox(box *layout.LayoutBox) layout.FormattingContext {
	if box.Style() == nil {
		return &layout.BlockFormattingContext{}
	}
	switch box.Style().Display {
	case style.DisplayFlex, style.DisplayInlineFlex:
		return &layout.FlexFormattingContext{}
	case style.DisplayGrid, style.DisplayInlineGrid:
		return &layout.GridFormattingContext{}
	case style.DisplayTable, style.DisplayInlineTable:
		return &layout.TableFormattingContext{}
	default:
		return &layout.BlockFormattingContext{}
	}
}
