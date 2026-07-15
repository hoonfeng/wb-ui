// Translation of: Source/WebCore/layout/formattingContexts/inline/InlineFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/inline/InlineLine.cpp
//                  Source/WebCore/layout/formattingContexts/inline/InlineLineBoxBuilder.cpp
// Completeness: 45%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - bidi is simplified to LTR only (no reordering, no unicode-bidi resolution)
//   - text breaking uses a greedy word-wrap: a "word" is a maximal run of non-space
//     characters; wrapping happens at the last fitting word boundary. CJK / break-all
//     is not implemented.
//   - line box height is computed as font-size * line-height (default 1.2); vertical
//     alignment is limited to baseline alignment of inline boxes
//   - inline-block boxes are sized by their own formatting context and contribute
//     their border-box height to the line box
//   - white-space: pre / nowrap are handled at the wrap decision level only
//   - text-align is applied to each line box (start/left default)
//   - the inline formatting context lays out the inline-level children of a single
//     block container (the anonymous wrapper produced by BuildLayoutTree)

package layout

import (
	"wb-ui/style"
)

// InlineFormattingContext is the Go translation of WebCore::Layout::InlineFormattingContext.
// It lays out the inline-level children of a block container into line boxes, wrapping
// text at the content width and computing the container's height from the line boxes.
type InlineFormattingContext struct{}

// Layout lays out box's inline-level children. box is expected to be a block container
// (or anonymous wrapper) whose children are BoxInline / BoxTextRun boxes. The caller
// sets box's border-box position and width; Layout computes the children's positions
// and box's auto height.
func (c *InlineFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	contentX := box.Rect.ContentX()
	contentY := box.Rect.ContentY()
	contentWidth := box.Rect.ContentWidth()
	fs := fontSizeOf(box)
	lineHeight := resolvedLineHeight(box, fs)
	// Vertical centering: offset the text segment within the line box so
	// the glyph (ascent + descent) is centered against the line-height.
	ascent, descent := fontAscentDescent(box)
	vCenterOffset := (lineHeight - ascent - descent) / 2
	if vCenterOffset < 0 {
		vCenterOffset = 0
	}

	// Lay out atomic inline-level boxes (inline-block and replaced elements
	// like <input>/<select>/<button>) so they have a concrete size before they
	// are measured by collectInlineItems. The parent inline formatting context
	// does not size atomic inline-level children the way a block formatting
	// context sizes block children, so we must do it here.
	for _, child := range box.Children {
		if !child.IsVisible() {
			continue
		}
		if isInlineBlock(child) || child.IsReplaced() {
			layoutInlineBlock(child, contentWidth, state)
		}
	}

	// Flatten the inline children into a sequence of layout items (text runs and
	// atomic inline-level boxes). This is a simplified version of WebKit's
	// InlineItemsBuilder that does not split on bidi boundaries.
	// Clear stale text segments from previous layout passes.
	for _, child := range box.Children {
		child.TextSegments = nil
	}
	items := collectInlineItems(box)

	// Greedy line breaking.
	cursorY := contentY
	cursorX := contentX
	lineStartIdx := 0
	lineStartX := contentX
	var lines []lineBox

	flushLine := func(endIdx int) {
		if endIdx <= lineStartIdx {
			return
		}
		// Compute the actual line height: the maximum of the font line-height
		// and the border-box height of any inline-block / replaced items on
		// this line. Inline-block boxes with an explicit height (e.g.
		// height:50px) expand the line box so the parent container grows to
		// contain them, matching browser behavior.
		actualLineHeight := lineHeight
		for j := lineStartIdx; j < endIdx; j++ {
			it := &items[j]
			if it.box != nil && (it.box.IsReplaced() || isInlineBlock(it.box)) {
				h := it.box.Rect.Height
				if h > actualLineHeight {
					actualLineHeight = h
				}
			}
		}
		// Record the resolved line box height on each item so the
		// TextSegment creation step can store LineY/LineHeight for
		// hit-testing and selection (mirrors RootInlineBox geometry).
		for j := lineStartIdx; j < endIdx; j++ {
			items[j].lineHeight = actualLineHeight
		}
		lb := lineBox{
			items:    items[lineStartIdx:endIdx],
			x:        lineStartX,
			y:        cursorY,
			width:    cursorX - lineStartX,
			height:   actualLineHeight,
			fontSize: fs,
		}
		lb.align = resolveTextAlign(box)
		lb.layoutItems(contentX, contentWidth)
		lines = append(lines, lb)
		cursorY += actualLineHeight
		cursorX = contentX
		lineStartX = contentX
		lineStartIdx = endIdx
	}

	nowrap := box.Style.WhiteSpace == style.WhiteSpaceNoWrap || box.Style.WhiteSpace == style.WhiteSpacePre

	for i := range items {
		it := &items[i]
		adv := it.advance
		if nowrap {
			// Do not wrap.
		} else if cursorX+adv > contentX+contentWidth+1e-6 && cursorX > lineStartX {
			// Wrap before this item.
			flushLine(i)
		}
		it.x = cursorX
		it.y = cursorY
		cursorX += adv
	}
	flushLine(len(items))

	// Apply text alignment to each line and position the items' boxes.
	for i := range lines {
		lb := &lines[i]
		lb.applyAlign(contentX, contentWidth)
	}

	// Write the item geometry back onto the underlying layout boxes.
	for _, it := range items {
		if it.box != nil {
			if isInlineBlock(it.box) || it.box.IsReplaced() {
				// Inline-block / replaced was laid out at (0,0); shift the whole
				// subtree to its final position on the line. Width/Height are
				// preserved from layoutInlineBlock.
				offsetSubtree(it.box, it.x-it.box.Rect.X, it.y-it.box.Rect.Y)
				continue
			}
			it.box.Rect.X = it.x
			it.box.Rect.Y = it.y
			if it.box.Type == BoxInline {
				it.box.Rect.Height = lineHeight
			}
		}
	}

	// Build TextSegments for each text run so the render tree sync step can copy
	// them onto RenderText.segments. A single text run may produce multiple
	// segments (e.g. words on different lines); they accumulate into its list.
	segByBox := map[*LayoutBox][]TextSegment{}
	textHeight := ascent + descent
	for _, it := range items {
		if it.box != nil && it.length > 0 {
			lh := it.lineHeight
			if lh <= 0 {
				lh = lineHeight
			}
			segByBox[it.box] = append(segByBox[it.box], TextSegment{
				Start:      it.start,
				Len:        it.length,
				X:          it.x,
				Y:          it.y + vCenterOffset,
				Width:      it.advance,
				Height:     textHeight,
				LineY:      it.y,
				LineHeight: lh,
			})
		}
	}
	for b, segs := range segByBox {
		b.TextSegments = segs
	}

	// Auto height: sum of line box heights.
	if heightIsAuto(box) {
		box.Rect.Height = cursorY - contentY
		if box.Rect.Height < 0 {
			box.Rect.Height = 0
		}
	}
}

// inlineItem is a single atomic piece of inline content: either a run of text or an
// inline-level box (inline-block, replaced). advance is the horizontal space the item
// occupies; box is the layout box to update (nil for text-only runs that share a box).
// start/length are the rune offset and length of this segment within the text run's
// original text, used to build TextSegments for the render tree sync.
type inlineItem struct {
	box       *LayoutBox
	text      string
	start     int // rune offset within box.Text
	length    int // rune length of this segment
	advance   float64
	x, y      float64
	lineHeight float64 // height of the line box containing this item
}

// lineBox is a single line of inline content. It groups the items that fit on one
// visual line and records their baseline/height for vertical alignment.
type lineBox struct {
	items    []inlineItem
	x, y     float64
	width    float64
	height   float64
	fontSize float64
	align    style.TextAlignType
}

// layoutItems resolves the vertical position of each item within the line box (baseline
// alignment) and the line box width.
func (lb *lineBox) layoutItems(contentX, contentWidth float64) {
	if len(lb.items) == 0 {
		return
	}
	// Default: items are left-aligned at their already-computed x.
}

// applyAlign shifts items horizontally according to the text-align value.
func (lb *lineBox) applyAlign(contentX, contentWidth float64) {
	if len(lb.items) == 0 {
		return
	}
	used := lb.width
	space := contentWidth - used
	if space <= 0 {
		return
	}
	shift := 0.0
	switch lb.align {
	case style.TextAlignCenter:
		shift = space / 2
	case style.TextAlignRight, style.TextAlignEnd:
		shift = space
	case style.TextAlignJustify:
		// Justify only the last line would not be justified; here we left-align.
		shift = 0
	default:
		shift = 0
	}
	for i := range lb.items {
		lb.items[i].x += shift
	}
}

// collectInlineItems walks box's inline-level children and produces a flat list of
// inline items in visual order (LTR). Text nodes are split into words/spaces so the
// line breaker can wrap at word boundaries.
func collectInlineItems(box *LayoutBox) []inlineItem {
	var items []inlineItem
	for _, child := range box.Children {
		switch {
		case child.IsTextRun():
			items = appendTextItems(items, child, child.Text)
		case child.IsInline() || child.IsReplaced():
			adv := inlineBoxAdvance(child)
			items = append(items, inlineItem{box: child, advance: adv})
		default:
			// Block-level boxes inside an inline context should not occur (the
			// anonymous wrapper groups them); skip defensively.
		}
	}
	return items
}

// appendTextItems splits a text run into words and whitespace items and appends them.
// Each whitespace run becomes a single item with the width of a space; each word
// becomes a single item with its measured width. start/length record the rune offset
// and length of each segment within the original text so the render tree sync can
// build InlineTextBox segments.
func appendTextItems(items []inlineItem, box *LayoutBox, text string) []inlineItem {
	fs := fontSizeOf(box)
	// Measure the space advance with the registered text measurer so that
	// word spacing matches the real font. Falls back to fs/2 when the
	// measurer is unavailable or returns zero.
	spaceAdvance := measureText(box, " ")
	if spaceAdvance <= 0 {
		spaceAdvance = fs / 2
	}
	runes := []rune(text)
	n := len(runes)
	i := 0
	for i < n {
		// Whitespace run.
		wsStart := i
		for i < n && (runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n') {
			i++
		}
		if i > wsStart {
			items = append(items, inlineItem{
				box: box, text: string(runes[wsStart:i]),
				start: wsStart, length: i - wsStart,
				advance: spaceAdvance,
			})
		}
		if i >= n {
			break
		}
		// Word run.
		wordStart := i
		for i < n && runes[i] != ' ' && runes[i] != '\t' && runes[i] != '\n' {
			i++
		}
		word := string(runes[wordStart:i])
		// Measure the real advance width with the registered text measurer
		// (Skia Font.MeasureText via layout.MeasureTextFunc). Falls back to a
		// monospace estimate only when the measurer returns zero.
		adv := measureText(box, word)
		if adv <= 0 {
			adv = float64(i-wordStart) * (fs / 2)
		}
		items = append(items, inlineItem{
			box: box, text: word,
			start: wordStart, length: i - wordStart,
			advance: adv,
		})
	}
	if len(items) == 0 && text != "" {
		// All-whitespace or non-wrappable text.
		items = append(items, inlineItem{box: box, text: text, start: 0, length: len([]rune(text)), advance: spaceAdvance})
	}
	return items
}

// inlineBoxAdvance returns the horizontal advance of an atomic inline-level box
// (inline-block / replaced). For boxes with a definite width that is used; otherwise
// the content width is approximated.
func inlineBoxAdvance(box *LayoutBox) float64 {
	if box.Rect.Width > 0 {
		return box.Rect.Width
	}
	if box.IntrinsicWidth > 0 {
		return box.IntrinsicWidth
	}
	// Approximate by content; no intrinsic sizing available.
	return fontSizeOf(box) * 2
}

// resolvedLineHeight returns the used line-height in pixels for box. A unit-less
// line-height multiplies the font size; a length line-height is resolved directly.
// When line-height is "normal" (auto), the font's intrinsic line spacing
// (ascent + descent + lineGap) is used, matching how browsers compute the
// default line box height from the font's hhea/sTypo metrics.
func resolvedLineHeight(box *LayoutBox, fs float64) float64 {
	normalLH := func() float64 {
		a, d, lg := fontMetricsTriple(box)
		h := a + d + lg
		if h <= 0 {
			return fs * 1.2
		}
		return h
	}
	if box.Style == nil {
		return normalLH()
	}
	lh := box.Style.LineHeight
	if lh.IsAuto() {
		return normalLH()
	}
	if lh.Unit == "" && lh.Value > 0 {
		// Unit-less number multiplies font size.
		return fs * lh.Value
	}
	r := resolveLength(lh, 0, fs)
	if r.Definite && r.Value > 0 {
		return r.Value
	}
	return normalLH()
}

// resolveTextAlign returns the effective text-align for box, mapping start to left
// (LTR only simplification).
func resolveTextAlign(box *LayoutBox) style.TextAlignType {
	if box.Style == nil {
		return style.TextAlignLeft
	}
	switch box.Style.TextAlign {
	case style.TextAlignStart, style.TextAlignLeft:
		return style.TextAlignLeft
	case style.TextAlignEnd, style.TextAlignRight:
		return style.TextAlignRight
	default:
		return box.Style.TextAlign
	}
}

// isInlineBlock reports whether box is an inline-block (display: inline-block).
// The layout box type is BoxInline (set by newBoxForElement) but the display
// property distinguishes it from a plain inline box.
func isInlineBlock(box *LayoutBox) bool {
	return box.Style != nil && box.Style.Display == style.DisplayInlineBlock
}

// layoutInlineBlock sizes and lays out an inline-block box. The box model
// (margin/padding/border) and width are computed here because the parent
// inline formatting context does not size atomic inline-level children the way
// a block formatting context sizes block children. After this returns the box
// has a concrete width and height and its descendants are laid out relative to
// (0,0); the caller positions it on the line via offsetSubtree.
func layoutInlineBlock(box *LayoutBox, availableWidth float64, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	fs := fontSizeOf(box)
	margin, padding, border := computeBoxModel(box, availableWidth, fs)
	box.Rect.Margin = margin
	box.Rect.Padding = padding
	box.Rect.Border = border

	// Width: definite -> use it; auto -> shrink-to-fit (measure then clamp).
	widthAuto := false
	if w, ok := definiteWidth(box.Style.Width, availableWidth, fs); ok {
		var borderBox float64
		if isBorderBox(box) {
			borderBox = w
		} else {
			borderBox = w + border.Horizontal() + padding.Horizontal()
		}
		minW, maxW, minAuto, maxAuto := resolveMinMax(box.Style.MinWidth, box.Style.MaxWidth, availableWidth, fs)
		box.Rect.Width = clampSize(borderBox, minW, maxW, minAuto, maxAuto)
	} else {
		// Provisional width for measurement: lay out at the available width,
		// then shrink to the rightmost content edge.
		box.Rect.Width = availableWidth
		widthAuto = true
	}

	box.Rect.X = 0
	box.Rect.Y = 0

	// Lay out descendants via the block formatting context (inline-block
	// establishes a BFC). This computes children positions and the box's
	// auto/definite height.
	ctx := contextFor(box)
	ctx.Layout(box, state)

	// Shrink-to-fit: clamp the border-box width to the max-content width
	// discovered among descendants (text segments and inline-level child boxes
	// summed per line). Measuring by line width rather than absolute right
	// edge means text-align: center / right does not inflate the result.
	// Block-level children are excluded because their Width is the fill width,
	// not their content's intrinsic width.
	if widthAuto {
		contentW := maxContentWidth(box)
		if contentW < 0 {
			contentW = 0
		}
		box.Rect.Width = contentW + padding.Horizontal() + border.Horizontal()
	}
}
