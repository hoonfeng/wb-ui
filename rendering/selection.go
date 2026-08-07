// Translation of: Source/WebCore/editing/VisibleSelection.h
//                  Source/WebCore/editing/VisiblePosition.cpp
//                  Source/WebCore/rendering/SelectionSubtreeRoot.cpp
// Completeness: 30%
// Simplifications:
//   - only RenderText-level granularity (no caret between inline boxes)
//   - selection is a simple start/end pair of (RenderText, runeOffset)
//   - word boundary detection uses simple whitespace/punctuation splitting
//   - no shadow DOM, no editing hosts, no contenteditable
//   - selection is a simple start/end pair of (RenderText, runeOffset)
//   - word boundary detection uses simple whitespace/punctuation splitting
//   - no shadow DOM, no editing hosts, no contenteditable
//
// ⚠️ Global-state note: CurrentSelection, CaretPos and CaretVisible are
//   package-level globals, which is safe only under the single-WebView model.
//   When multi-WebView support is needed, these must be moved to RenderView
//   or WebView instance fields.
package rendering

import (
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TextPosition identifies a location within the render tree's text content,
// mirroring WebCore::VisiblePosition. It pairs a RenderText with a rune offset
// into that text. A zero-value TextPosition is "no position".
type TextPosition struct {
	RT     *RenderText
	Offset int // rune offset within RT.text
}

// IsValid reports whether this position refers to a real RenderText.
func (p TextPosition) IsValid() bool { return p.RT != nil }

// TextGranularity mirrors WebCore::TextGranularity. It controls how a selection
// is expanded to natural boundaries (character, word, line, paragraph, document).
// Click count maps to granularity: 1=character, 2=word, 3=line, 4=paragraph,
// Ctrl+A=document. When a drag follows a multi-click, the granularity is used
// to expand the dragging endpoint so that (e.g.) after a double-click, dragging
// extends the selection by whole words rather than individual characters.
type TextGranularity int

const (
	// GranularityCharacter selects individual characters (single click / drag).
	GranularityCharacter TextGranularity = iota
	// GranularityWord selects whole words (double-click + drag).
	GranularityWord
	// GranularityLine selects whole visual lines (triple-click + drag).
	GranularityLine
	// GranularityParagraph selects whole paragraphs (quadruple-click + drag).
	GranularityParagraph
	// GranularityDocument selects the entire document (Ctrl+A).
	GranularityDocument
)

// Selection represents a text selection range, mirroring WebCore::VisibleSelection.
// Start and End are the anchor points; the selection extends from the
// earlier (in tree order) to the later. Active indicates the mouse button is
// currently held down (drag-select in progress). Granularity records the
// selection granularity (character/word/line/paragraph) so that drag-extension
// can expand to the same boundary type.
type Selection struct {
	Start       TextPosition
	End         TextPosition
	Active      bool // true while dragging
	Granularity TextGranularity
}

// CurrentSelection is the global selection state. The Host event loop updates
// it from mouse events; the paint pipeline reads it to draw the highlight.
//
// ⚠️ Single-WebView global: in a multi-WebView port this would belong on
// the RenderView or WebView instance. See package doc for details.
var CurrentSelection *Selection

// CaretPos is the global caret (text cursor) position. When non-nil and
// CaretVisible is true, a blinking caret is drawn at this position. Set by
// the Host event loop; read by PaintCaret.
//
// ⚠️ Single-WebView global: see package doc.
var CaretPos *TextPosition

// CaretVisible controls caret visibility for blinking. The Host toggles this
// at ~500ms intervals, mirroring WebKit's caret blink cycle.
//
// ⚠️ Single-WebView global: see package doc.
var CaretVisible bool


// SetCaret sets the caret position. Pass nil to hide the caret.

// SetCaret sets the caret position. Pass nil to hide the caret.

// SetCaret sets the caret position. Pass nil to hide the caret.
func SetCaret(pos *TextPosition) {
	CaretPos = pos
}

// ClearSelection removes any active text selection.
func ClearSelection() {
	CurrentSelection = nil
}

// collectRenderTexts walks the render tree in pre-order and collects all
// RenderText objects with their segments. The order determines which
// TextPosition comes "first" in the document.
func collectRenderTexts(o RenderObject, list *[]*RenderText) {
	if o == nil {
		return
	}
	if rt, ok := o.(*RenderText); ok {
		if len(rt.Segments()) > 0 {
			*list = append(*list, rt)
		}
		return // RenderText is a leaf
	}
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		collectRenderTexts(c, list)
	}
}

// collectRenderTextsAll is the cross-frame variant of collectRenderTexts:
// when the traversal reaches an <iframe> element, the child Frame's text is
// collected inline at that position (browser document order — the embedded
// document's text appears where the iframe element sits in the parent tree).
// This gives a single global pre-order covering the whole frame tree, so a
// selection whose anchor is in one Frame and whose end is in another can be
// ordered and highlighted correctly.
func collectRenderTextsAll(o RenderObject, list *[]*RenderText) {
	if o == nil {
		return
	}
	if rt, ok := o.(*RenderText); ok {
		if len(rt.Segments()) > 0 {
			*list = append(*list, rt)
		}
		return // RenderText is a leaf
	}
	// iframe 元素：子 Frame 文本插在树序位置（含嵌套 iframe 的递归）。
	if el, isEl := o.Node().(*dom.Element); isEl && el.LocalName() == "iframe" {
		if sub := IFrameLookupFor(el); sub != nil && sub.RenderView() != nil {
			if sub.NeedsLayout() {
				sub.LayoutNow()
			}
			collectRenderTextsAll(RenderObject(sub.RenderView()), list)
		}
	}
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		collectRenderTextsAll(c, list)
	}
}

// globalSelTexts 是本次绘制帧的跨 Frame 文本列表（树序，主 Frame + 所有
// iframe 子 Frame）。Paint 入口由主 Frame 构建一次，子 Frame 的 Paint
// 在 PaintIFrame 中（主绘制中途）复用——保证每个 Frame 绘制时都能用
// 全局顺序判定跨 Frame 选区，同时只输出属于自己的高亮段。非绘制路径
// （测试等）为空时，选区函数回退到单 rv 收集（行为不变）。
var globalSelTexts []*RenderText

// globalTextList returns the pre-order text list covering the whole frame
// tree rooted at rv. When a cross-frame list is available (paint path) it is
// used as-is; otherwise it falls back to a single-frame collection so that
// non-paint callers (tests) keep the historical behavior.
func globalTextList(rv *RenderView) []*RenderText {
	if globalSelTexts != nil {
		return globalSelTexts
	}
	if rv == nil {
		return nil
	}
	var list []*RenderText
	collectRenderTexts(RenderObject(rv), &list)
	return list
}

// positionIndex returns the index of the RenderText in the pre-order list,
// or -1 if not found. Used to compare two TextPositions for document order.
func positionIndex(list []*RenderText, rt *RenderText) int {
	for i, r := range list {
		if r == rt {
			return i
		}
	}
	return -1
}

// normalizeSelection returns the start and end positions in document order
// (earlier first). If either position is invalid, both are zero.
func normalizeSelection(sel *Selection) (TextPosition, TextPosition) {
	if sel == nil || !sel.Start.IsValid() || !sel.End.IsValid() {
		return TextPosition{}, TextPosition{}
	}
	if sel.Start.RT == sel.End.RT {
		if sel.Start.Offset <= sel.End.Offset {
			return sel.Start, sel.End
		}
		return sel.End, sel.Start
	}
	// Different RenderTexts: compare by tree order. Use the cross-frame
	// list (paint path) so an anchor in one Frame and an end in another
	// are ordered by document position; fall back to the single-frame
	// collection for non-paint callers.
	var list []*RenderText
	rv := sel.Start.RT.View()
	if rv == nil {
		rv = sel.End.RT.View()
	}
	if rv == nil {
		return sel.Start, sel.End // can't determine order, use as-is
	}
	list = globalTextList(rv)
	si := positionIndex(list, sel.Start.RT)
	ei := positionIndex(list, sel.End.RT)
	if si < 0 || ei < 0 {
		// 跨 Frame 且无全局列表（非绘制路径）：按 View 归属粗排——
		// 不同 Frame 时无法精确比较，保持原样。
		if sel.Start.RT.View() == sel.End.RT.View() {
			return sel.Start, sel.End
		}
		return sel.Start, sel.End
	}
	if si <= ei {
		return sel.Start, sel.End
	}
	return sel.End, sel.Start
}

// HitTestText finds the text position at the given CSS-pixel coordinates.
// Returns an invalid TextPosition if no text is hit.
func HitTestText(rv *RenderView, x, y float64) TextPosition {
	if rv == nil {
		return TextPosition{}
	}
	// ★ iframe 下钻：点落在 iframe 内容框内时，HitTest 会递归进子 Frame
	// 并记录 lastDive（子坐标 + 子视图）。子文档的 RenderText 不在主 rv
	// 的文本列表里——必须用子坐标递归子文档的 HitTestText（浏览器
	// 文本选择跨 iframe 的语义：选择锚/终点落在子文档文本上）。
	el := HitTest(rv, x, y, "")
	if el != nil {
		if od := el.OwnerDocument(); od != nil && od != rv.Document() {
			if lastDive.ok && lastDive.sub != nil && lastDive.sub.Document() == od &&
				lastDive.sub.RenderView() != nil {
				sub := lastDive.sub
				lastDive.ok = false // 一次性消费
				if sub.NeedsLayout() {
					sub.LayoutNow()
				}
				return HitTestText(sub.RenderView(), lastDive.x, lastDive.y)
			}
		}
	}
	var best TextPosition
	var bestDist float64 = -1
	var list []*RenderText
	collectRenderTexts(RenderObject(rv), &list)
	for _, rt := range list {
		pos, dist := hitTestRenderText(rt, x, y)
		if pos.IsValid() && (bestDist < 0 || dist < bestDist) {
			best = pos
			bestDist = dist
		}
	}
	return best
}

// lineBoxBounds returns the (top, height) of the line box containing seg.
// Falls back to the text-box geometry for segments that don't carry line
// box information (e.g. produced by older layout code).
func lineBoxBounds(seg InlineTextBox) (top, height float64) {
	if seg.LineHeight > 0 {
		return seg.LineY, seg.LineHeight
	}
	return seg.Y, seg.Height
}

// hitTestRenderText checks if (x, y) falls within any segment of rt.
// Returns the TextPosition and the distance to the segment center (for
// nearest-match when no segment contains the point exactly).
//
// A single line may contain multiple segments (one per word / whitespace
// run produced by the inline formatting). We must check ALL segments on
// the same line, not just the first one whose right edge x exceeds —
// otherwise clicking on a later word (e.g. CJK text after "Go ") would
// incorrectly return the end of the earlier segment.
func hitTestRenderText(rt *RenderText, x, y float64) (TextPosition, float64) {
	segments := rt.Segments()
	if len(segments) == 0 {
		return TextPosition{}, -1
	}
	st := rt.Style()
	font := toGraphicsFont(st)
	content := rt.OriginalText()
	runes := []rune(content)

	// Collect all segments whose line box contains y. Using the line box
	// (RootInlineBox) bounds — not the text-box bounds — means clicks in
	// the inter-line whitespace register, matching browser behavior.
	var lineSegs []InlineTextBox
	for _, seg := range segments {
		lineTop, lineH := lineBoxBounds(seg)
		if y >= lineTop && y <= lineTop+lineH {
			lineSegs = append(lineSegs, seg)
		}
	}

	if len(lineSegs) > 0 {
		for i, seg := range lineSegs {
			if x < seg.X {
				// x is before this segment. If there's a previous segment on
				// this line, x falls in the gap — return its end (closer).
				if i > 0 {
					prev := lineSegs[i-1]
					return TextPosition{RT: rt, Offset: prev.Start + prev.Len}, 0
				}
				// x is before the first segment on this line.
				return TextPosition{RT: rt, Offset: seg.Start}, 0
			}
			if x <= seg.X+seg.Width {
				// x is within this segment: find the closest character offset
				// using per-rune MeasureText.
				accWidth := 0.0
				for j := 0; j < seg.Len; j++ {
					runeIdx := seg.Start + j
					if runeIdx >= len(runes) {
						break
					}
					ch := string(runes[runeIdx])
					runeW := graphics.MeasureText(font, ch)
					if x < seg.X+accWidth+runeW/2 {
						return TextPosition{RT: rt, Offset: runeIdx}, 0
					}
					accWidth += runeW
				}
				// x is past the last character of this segment.
				return TextPosition{RT: rt, Offset: seg.Start + seg.Len}, 0
			}
			// x is past this segment; continue to the next segment on the line.
		}
		// x is past all segments on this line; return end of the last segment.
		last := lineSegs[len(lineSegs)-1]
		return TextPosition{RT: rt, Offset: last.Start + last.Len}, 0
	}

	// Fallback: y is outside all line boxes (e.g. above the first line or
	// below the last). Find the nearest line and return its start or end,
	// mirroring how browsers place the caret when clicking outside the text.
	var nearestIdx = -1
	var nearestDist float64 = -1
	for i, seg := range segments {
		lineTop, lineH := lineBoxBounds(seg)
		lineCenter := lineTop + lineH/2
		d := y - lineCenter
		if d < 0 {
			d = -d
		}
		if nearestIdx < 0 || d < nearestDist {
			nearestIdx = i
			nearestDist = d
		}
	}
	if nearestIdx >= 0 {
		seg := segments[nearestIdx]
		lineTop, _ := lineBoxBounds(seg)
		if y < lineTop {
			return TextPosition{RT: rt, Offset: seg.Start}, nearestDist
		}
		return TextPosition{RT: rt, Offset: seg.Start + seg.Len}, nearestDist
	}

	// No segment hit on this RenderText; return invalid
	return TextPosition{}, -1
}

// SelectWord selects the word at the given position (for double-click).
func SelectWord(pos TextPosition) {
	if !pos.IsValid() {
		return
	}
	rt := pos.RT
	text := []rune(rt.OriginalText())
	if len(text) == 0 {
		return
	}
	idx := pos.Offset
	if idx < 0 {
		idx = 0
	}
	if idx > len(text) {
		idx = len(text)
	}
	// Scan backward to find word start
	start := idx
	for start > 0 && !isWordBoundary(text[start-1]) {
		start--
	}
	// Scan forward to find word end
	end := idx
	for end < len(text) && !isWordBoundary(text[end]) {
		end++
	}
	CurrentSelection = &Selection{
		Start:       TextPosition{RT: rt, Offset: start},
		End:         TextPosition{RT: rt, Offset: end},
		Active:      false,
		Granularity: GranularityWord,
	}
}

// isWordBoundary returns true for characters that separate words.
func isWordBoundary(r rune) bool {
	return strings.ContainsRune(" \t\n\r\f.,;:!?()[]{}\"'`", r)
}

// SelectionRects returns the rectangles that should be painted for the
// current selection highlight. Each rectangle corresponds to a portion of
// a text segment that is within the selection range.
func SelectionRects(rv *RenderView) []graphics.Rect {
	if CurrentSelection == nil || rv == nil {
		return nil
	}
	start, end := normalizeSelection(CurrentSelection)
	if !start.IsValid() || !end.IsValid() {
		return nil
	}
	if start.RT == end.RT {
		// 选区在别的 Frame 时本 rv 无高亮（该 Frame 绘制时自己输出）。
		if start.RT.View() == rv {
			return rectsForRenderText(start.RT, start.Offset, end.Offset)
		}
		return nil
	}
	// Selection spans multiple RenderTexts: collect all in tree order —
	// the cross-frame list covers iframe sub-documents, so a selection
	// whose anchor/end sits in another Frame is ordered correctly. Only
	// RenderTexts owned by this rv are emitted: sub-document coordinates
	// belong to the sub-frame's own canvas (PaintIFrame translates).
	list := globalTextList(rv)
	si := positionIndex(list, start.RT)
	ei := positionIndex(list, end.RT)
	if si < 0 || ei < 0 {
		return nil
	}
	var rects []graphics.Rect
	for i := si; i <= ei; i++ {
		rt := list[i]
		if rt.View() != rv {
			continue // 属于其他 Frame，由该 Frame 绘制时输出
		}
		textLen := rt.Length()
		if i == si {
			rects = append(rects, rectsForRenderText(rt, start.Offset, textLen)...)
		} else if i == ei {
			rects = append(rects, rectsForRenderText(rt, 0, end.Offset)...)
		} else {
			rects = append(rects, rectsForRenderText(rt, 0, textLen)...)
		}
	}
	return rects
}

// rectsForRenderText returns selection rectangles for the given range within
// a single RenderText. Each segment that overlaps the range contributes a
// rectangle covering the selected portion. The rectangle height covers the
// full line box (RootInlineBox), matching how browsers paint the selection
// highlight — not just the text glyph height.
func rectsForRenderText(rt *RenderText, fromOffset, toOffset int) []graphics.Rect {
	if rt == nil || fromOffset >= toOffset {
		return nil
	}
	st := rt.Style()
	font := toGraphicsFont(st)
	content := rt.OriginalText()
	runes := []rune(content)
	var rects []graphics.Rect
	for _, seg := range rt.Segments() {
		segEnd := seg.Start + seg.Len
		// No overlap with selection range
		if segEnd <= fromOffset || seg.Start >= toOffset {
			continue
		}
		// Clamp selection to this segment
		selStart := fromOffset
		if selStart < seg.Start {
			selStart = seg.Start
		}
		selEnd := toOffset
		if selEnd > segEnd {
			selEnd = segEnd
		}
		// Measure the width before the selection start (within this segment)
		prefixRunes := runes[seg.Start:selStart]
		prefixW := 0.0
		if len(prefixRunes) > 0 {
			prefixW = graphics.MeasureText(font, string(prefixRunes))
		}
		// Measure the width of the selected text (within this segment)
		selRunes := runes[selStart:selEnd]
		selW := 0.0
		if len(selRunes) > 0 {
			selW = graphics.MeasureText(font, string(selRunes))
		}
		// Use the line box height for the selection rectangle so the
		// highlight covers the full line, like browsers do.
		lineTop, lineH := lineBoxBounds(seg)
		rects = append(rects, graphics.Rect{
			X:      seg.X + prefixW,
			Y:      lineTop,
			Width:  selW,
			Height: lineH,
		})
	}
	return rects
}

// SelectedText returns the text content of the current selection, suitable
// for copying to the clipboard. Returns empty string if no selection.
func SelectedText(rv *RenderView) string {
	if CurrentSelection == nil || rv == nil {
		return ""
	}
	start, end := normalizeSelection(CurrentSelection)
	if !start.IsValid() || !end.IsValid() {
		return ""
	}
	if start.RT == end.RT {
		text := []rune(start.RT.OriginalText())
		if start.Offset >= end.Offset || start.Offset >= len(text) {
			return ""
		}
		to := end.Offset
		if to > len(text) {
			to = len(text)
		}
		return string(text[start.Offset:to])
	}
	// Multi-RenderText selection
	var list []*RenderText
	collectRenderTexts(RenderObject(rv), &list)
	si := positionIndex(list, start.RT)
	ei := positionIndex(list, end.RT)
	if si < 0 || ei < 0 {
		return ""
	}
	var b strings.Builder
	for i := si; i <= ei; i++ {
		rt := list[i]
		text := []rune(rt.OriginalText())
		if i == si {
			if start.Offset < len(text) {
				b.WriteString(string(text[start.Offset:]))
			}
		} else if i == ei {
			to := end.Offset
			if to > len(text) {
				to = len(text)
			}
			b.WriteString(string(text[:to]))
		} else {
			b.WriteString(string(text))
		}
		if i < ei {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// HasSelection reports whether there is a non-empty text selection.
func HasSelection(rv *RenderView) bool {
	if CurrentSelection == nil {
		return false
	}
	start, end := normalizeSelection(CurrentSelection)
	if !start.IsValid() || !end.IsValid() {
		return false
	}
	if start.RT == end.RT {
		return start.Offset < end.Offset
	}
	return true
}

// SelectLine selects the entire visual line containing pos, mirroring WebKit's
// triple-click line selection. A visual line is identified by the LineY/LineHeight
// of the segment containing pos; all segments (across all RenderTexts) whose
// line box overlaps that Y range are selected.
func SelectLine(rv *RenderView, pos TextPosition) {
	if !pos.IsValid() || rv == nil {
		return
	}
	// Find the segment at pos to determine the line box Y range.
	segIdx := -1
	for i, s := range pos.RT.Segments() {
		if pos.Offset >= s.Start && pos.Offset <= s.Start+s.Len {
			segIdx = i
			break
		}
	}
	if segIdx < 0 {
		return
	}
	seg := pos.RT.Segments()[segIdx]
	lineTop, lineH := lineBoxBounds(seg)
	lineBottom := lineTop + lineH
	// Collect all RenderTexts and find segments overlapping this line.
	var list []*RenderText
	collectRenderTexts(RenderObject(rv), &list)
	var first, last TextPosition
	found := false
	for _, rt := range list {
		for _, s := range rt.Segments() {
			sTop, sH := lineBoxBounds(s)
			sBottom := sTop + sH
			if sTop < lineBottom && sBottom > lineTop {
				if !found {
					first = TextPosition{RT: rt, Offset: s.Start}
					found = true
				}
				last = TextPosition{RT: rt, Offset: s.Start + s.Len}
			}
		}
	}
	if found {
		CurrentSelection = &Selection{
			Start:       first,
			End:         last,
			Granularity: GranularityLine,
		}
	}
}

// findContainingBlockFlow walks up from o to find the nearest RenderBlockFlow
// ancestor (the paragraph container). Returns the RenderView as fallback.
func findContainingBlockFlow(o RenderObject) RenderObject {
	cur := o
	for cur != nil {
		if _, ok := cur.(*RenderBlockFlow); ok {
			return cur
		}
		cur = cur.Parent()
	}
	return o
}

// SelectParagraph selects the entire paragraph containing pos. A paragraph is
// approximated as the nearest RenderBlockFlow ancestor of pos.RT; all
// RenderTexts under that block are selected. Mirrors WebKit's quadruple-click.
func SelectParagraph(rv *RenderView, pos TextPosition) {
	if !pos.IsValid() || rv == nil {
		return
	}
	block := findContainingBlockFlow(pos.RT)
	var list []*RenderText
	collectRenderTexts(block, &list)
	if len(list) == 0 {
		// Fallback: select all text in the document.
		collectRenderTexts(RenderObject(rv), &list)
	}
	if len(list) == 0 {
		return
	}
	first := list[0]
	last := list[len(list)-1]
	CurrentSelection = &Selection{
		Start:       TextPosition{RT: first, Offset: 0},
		End:         TextPosition{RT: last, Offset: last.Length()},
		Granularity: GranularityParagraph,
	}
}

// SelectDocument selects all text in the document, mirroring Ctrl+A.
func SelectDocument(rv *RenderView) {
	if rv == nil {
		return
	}
	var list []*RenderText
	collectRenderTexts(RenderObject(rv), &list)
	if len(list) == 0 {
		return
	}
	first := list[0]
	last := list[len(list)-1]
	CurrentSelection = &Selection{
		Start:       TextPosition{RT: first, Offset: 0},
		End:         TextPosition{RT: last, Offset: last.Length()},
		Granularity: GranularityDocument,
	}
}

// ExpandPosition expands a position to the given granularity boundary in the
// specified direction, mirroring WebKit's VisiblePosition expansion. For
// GranularityWord the position moves to the next/previous word boundary; for
// GranularityLine it moves to the line start/end; for GranularityParagraph
// it moves to the paragraph start/end. Returns the expanded position.
func ExpandPosition(pos TextPosition, granularity TextGranularity, forward bool) TextPosition {
	if !pos.IsValid() {
		return pos
	}
	switch granularity {
	case GranularityWord:
		return expandToWordBoundary(pos, forward)
	case GranularityLine:
		return expandToLineBoundary(pos, forward)
	case GranularityParagraph:
		return expandToParagraphBoundary(pos, forward)
	}
	return pos
}

// expandToWordBoundary moves pos to the next/previous word boundary.
func expandToWordBoundary(pos TextPosition, forward bool) TextPosition {
	rt := pos.RT
	text := []rune(rt.OriginalText())
	idx := pos.Offset
	if forward {
		for idx < len(text) && isWordBoundary(text[idx]) {
			idx++
		}
		for idx < len(text) && !isWordBoundary(text[idx]) {
			idx++
		}
	} else {
		for idx > 0 && isWordBoundary(text[idx-1]) {
			idx--
		}
		for idx > 0 && !isWordBoundary(text[idx-1]) {
			idx--
		}
	}
	return TextPosition{RT: rt, Offset: idx}
}

// expandToLineBoundary moves pos to the start/end of the visual line within
// the same RenderText.
func expandToLineBoundary(pos TextPosition, forward bool) TextPosition {
	rt := pos.RT
	segs := rt.Segments()
	if len(segs) == 0 {
		return pos
	}
	segIdx := -1
	for i, s := range segs {
		if pos.Offset >= s.Start && pos.Offset <= s.Start+s.Len {
			segIdx = i
			break
		}
	}
	if segIdx < 0 {
		return pos
	}
	seg := segs[segIdx]
	lineTop, lineH := lineBoxBounds(seg)
	lineBottom := lineTop + lineH
	if forward {
		last := seg
		for i := segIdx + 1; i < len(segs); i++ {
			sTop, sH := lineBoxBounds(segs[i])
			if sTop < lineBottom && sTop+sH > lineTop {
				last = segs[i]
			} else {
				break
			}
		}
		return TextPosition{RT: rt, Offset: last.Start + last.Len}
	}
	first := seg
	for i := segIdx - 1; i >= 0; i-- {
		sTop, sH := lineBoxBounds(segs[i])
		if sTop < lineBottom && sTop+sH > lineTop {
			first = segs[i]
		} else {
			break
		}
	}
	return TextPosition{RT: rt, Offset: first.Start}
}

// expandToParagraphBoundary moves pos to the paragraph start/end.
// Simplified: paragraph = entire RenderText content.
func expandToParagraphBoundary(pos TextPosition, forward bool) TextPosition {
	rt := pos.RT
	if forward {
		return TextPosition{RT: rt, Offset: rt.Length()}
	}
	return TextPosition{RT: rt, Offset: 0}
}

// SetSelectionWithGranularity sets the current selection with explicit
// start/end positions and granularity. Used by drag-extension to apply
// granularity expansion to the dragging endpoint.
func SetSelectionWithGranularity(start, end TextPosition, granularity TextGranularity, active bool) {
	CurrentSelection = &Selection{
		Start:       start,
		End:         end,
		Active:      active,
		Granularity: granularity,
	}
}

// IsOffsetInSelection reports whether the given (RenderText, rune offset) is
// within the current selection. Used by PaintText to determine which portions
// of a text segment should be drawn in the inverted (highlight) color.
func IsOffsetInSelection(rv *RenderView, rt *RenderText, offset int) bool {
	if CurrentSelection == nil || rv == nil {
		return false
	}
	start, end := normalizeSelection(CurrentSelection)
	if !start.IsValid() || !end.IsValid() {
		return false
	}
	if start.RT == end.RT {
		return rt == start.RT && offset >= start.Offset && offset < end.Offset
	}
	// 跨 Frame 选区用全局树序（Paint 路径）判定 rt 是否落在
	// [start, end] 之间；回退单 rv 收集（非绘制路径）。
	list := globalTextList(rv)
	si := positionIndex(list, start.RT)
	ei := positionIndex(list, end.RT)
	ri := positionIndex(list, rt)
	if si < 0 || ei < 0 || ri < 0 {
		return false
	}
	if ri < si || ri > ei {
		return false
	}
	if ri == si && offset < start.Offset {
		return false
	}
	if ri == ei && offset >= end.Offset {
		return false
	}
	return true
}

// SelectionRangeForSegment returns the (fromOffset, toOffset) range within
// the segment [segStart, segStart+segLen] that falls inside the current
// selection. Returns (0, 0, false) when the segment is not selected at all.
// Used by PaintText to split a segment into unselected/selected parts for
// inverted-color rendering.
func SelectionRangeForSegment(rv *RenderView, rt *RenderText, segStart, segLen int) (from, to int, selected bool) {
	if CurrentSelection == nil || rv == nil || rt == nil || segLen <= 0 {
		return 0, 0, false
	}
	segEnd := segStart + segLen
	start, end := normalizeSelection(CurrentSelection)
	if !start.IsValid() || !end.IsValid() {
		return 0, 0, false
	}
	if start.RT == end.RT {
		if rt != start.RT {
			return 0, 0, false
		}
		from = start.Offset
		if from < segStart {
			from = segStart
		}
		to = end.Offset
		if to > segEnd {
			to = segEnd
		}
		return from, to, from < to
	}
	list := globalTextList(rv)
	si := positionIndex(list, start.RT)
	ei := positionIndex(list, end.RT)
	ri := positionIndex(list, rt)
	if si < 0 || ei < 0 || ri < 0 {
		return 0, 0, false
	}
	// Entire segment is selected.
	if ri > si && ri < ei {
		return segStart, segEnd, true
	}
	// At selection start: partial from start.Offset.
	if ri == si {
		from = start.Offset
		if from < segStart {
			from = segStart
		}
		to := segEnd
		if ri == ei && end.Offset < to {
			to = end.Offset
		}
		return from, to, from < to
	}
	// At selection end: partial up to end.Offset.
	if ri == ei {
		to := end.Offset
		if to > segEnd {
			to = segEnd
		}
		return segStart, to, segStart < to
	}
	return 0, 0, false
}
