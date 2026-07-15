// Translation of: Source/WebCore/layout/formattingContexts/grid/GridFormattingContext.cpp
//                  Source/WebCore/layout/formattingContexts/grid/GridLayout.cpp
//                  Source/WebCore/layout/formattingContexts/grid/TrackSizingAlgorithm.cpp
//                  Source/WebCore/rendering/RenderGrid.cpp
// Completeness: 45%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - track sizing supports fixed px lengths, percentages (of the grid container
//     content size), the `fr` unit (fraction of the leftover free space), auto (sized
//     to the max-content of the items in the track) and minmax(a, b)
//   - grid-template-areas is parsed to fix item placement via grid-area names
//   - dense packing / sparse auto-placement is simplified to the sparse algorithm
//   - baseline alignment is approximated as start alignment
//   - subgrid, masonry, container-orientation are not supported
//   - the track sizing algorithm runs a single grow-free-space pass; the iterative
//     "increase to satisfy min-content" passes of the spec are collapsed

package layout

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/style"
)

// GridFormattingContext is the Go translation of WebCore::Layout::GridFormattingContext
// / RenderGrid. It lays out a grid container's items into rows and columns per CSS Grid
// Layout Module Level 1.
type GridFormattingContext struct{}

// Layout lays out box's grid items. The caller sets box's border-box position and
// width; Layout resolves the column and row tracks, places the items into their grid
// areas and computes the container's auto height.
func (c *GridFormattingContext) Layout(box *LayoutBox, state *LayoutState) {
	if box.Style == nil {
		box.Style = style.NewComputedStyle()
	}
	contentX := box.Rect.ContentX()
	contentY := box.Rect.ContentY()
	contentWidth := box.Rect.ContentWidth()

	// Parse tracks and the area name map.
	colTracks := parseGridTracks(box.Style.GridTemplateColumns, contentWidth)
	rowTracks := parseGridTracks(box.Style.GridTemplateRows, 0)
	areaMap, _ := parseGridAreas(box.Style.GridTemplateAreas)
	rowGap, colGap := resolveGap(box, contentWidth)

	// Collect visible in-flow items and resolve their grid spans.
	items := collectGridItems(box, areaMap)

	// Auto-place items that have no explicit column/row placement.
	autoPlaceItems(items, len(colTracks), len(rowTracks))

	// Ensure enough tracks exist for the placed items.
	maxCol, maxRow := 0, 0
	for i := range items {
		if items[i].colEnd > maxCol {
			maxCol = items[i].colEnd
		}
		if items[i].rowEnd > maxRow {
			maxRow = items[i].rowEnd
		}
	}
	for len(colTracks) < maxCol {
		colTracks = append(colTracks, gridTrack{size: "auto"})
	}
	for len(rowTracks) < maxRow {
		rowTracks = append(rowTracks, gridTrack{size: "auto"})
	}

	// Size the columns: resolve px / % / auto, then distribute fr leftover.
	colSizes := sizeTracks(colTracks, contentWidth, colGap, func(idx int) float64 {
		return maxContentWidthOfTrack(items, idx)
	})
	usedCols := sumSizes(colSizes) + colGap*float64(max(0, len(colSizes)-1))
	if usedCols > contentWidth {
		// Tracks overflow; scale down is not done (content may overflow visually).
	}

	// Lay out items along the columns first (so their width is known), then size rows.
	// Set each item's width and lay out its content; then compute row sizes from content.
	itemHeights := make([]float64, len(items))
	for i := range items {
		it := &items[i]
		margin, padding, border := computeBoxModel(it.box, contentWidth, fontSizeOf(it.box))
		it.box.Rect.Margin = margin
		it.box.Rect.Padding = padding
		it.box.Rect.Border = border
		x := contentX
		for k := 0; k < it.colStart; k++ {
			x += colSizes[k] + colGap
		}
		spanW := 0.0
		for k := it.colStart; k < it.colEnd; k++ {
			spanW += colSizes[k]
			if k > it.colStart {
				spanW += colGap
			}
		}
		it.box.Rect.X = x + margin.Left
		it.box.Rect.Width = spanW - border.Horizontal() - padding.Horizontal()
		if it.box.Rect.Width < 0 {
			it.box.Rect.Width = 0
		}
		// Lay out content against this width.
		childCtx := contextFor(it.box)
		childCtx.Layout(it.box, state)
		itemHeights[i] = it.box.Rect.Height + border.Vertical() + padding.Vertical()
	}

	// Size the rows: auto rows take the max content height of items in that row.
	rowSizes := sizeTracks(rowTracks, 0, rowGap, func(idx int) float64 {
		return maxContentHeightOfTrack(items, itemHeights, idx)
	})

	// Position items vertically.
	for i := range items {
		it := &items[i]
		y := contentY
		for k := 0; k < it.rowStart; k++ {
			y += rowSizes[k] + rowGap
		}
		it.box.Rect.Y = y + it.box.Rect.Margin.Top
	}

	// Auto height of the container: sum of row tracks + gaps.
	if heightIsAuto(box) {
		h := sumSizes(rowSizes) + rowGap*float64(max(0, len(rowSizes)-1))
		box.Rect.Height = h
	}
}

// gridItem captures the placement of a single grid item.
type gridItem struct {
	box                *LayoutBox
	colStart, colEnd   int
	rowStart, rowEnd   int
	areaName           string
}

// gridTrack describes a single grid track (column or row).
type gridTrack struct {
	size string
}

// collectGridItems returns the visible in-flow children of a grid container with their
// resolved grid placement. Items referencing a named area are resolved against areaMap.
func collectGridItems(box *LayoutBox, areaMap map[string][4]int) []gridItem {
	var out []gridItem
	for _, child := range box.Children {
		if !child.IsVisible() || !child.IsInFlow() {
			continue
		}
		if child.Style == nil {
			child.Style = style.NewComputedStyle()
		}
		gi := gridItem{box: child}
		// grid-area can be a name or <row-start> / <col-start> / <row-end> / <col-end>.
		area := child.Style.GridTemplateAreas
		if area == "" {
			area = child.Style.Properties["grid-area"]
		}
		if area != "" && areaMap != nil {
			if r, ok := areaMap[area]; ok {
				gi.rowStart, gi.colStart, gi.rowEnd, gi.colEnd = r[0], r[1], r[2], r[3]
				gi.areaName = area
				out = append(out, gi)
				continue
			}
		}
		gi.colStart, gi.colEnd = parseGridLine(child.Style.GridColumnStart, child.Style.GridColumnEnd)
		gi.rowStart, gi.rowEnd = parseGridLine(child.Style.GridRowStart, child.Style.GridRowEnd)
		out = append(out, gi)
	}
	return out
}

// parseGridLine resolves the start/end line indices for a single axis. Lines are 1-based;
// a negative index counts from the end. An empty value is treated as auto (placed later).
func parseGridLine(start, end string) (int, int) {
	s := parseLineIndex(start)
	e := parseLineIndex(end)
	if s > 0 && e == 0 {
		e = s + 1
	}
	if e > 0 && s == 0 {
		s = e - 1
	}
	if s <= 0 {
		s = 0
	}
	if e <= 0 {
		e = 0
	}
	return s - 1, e - 1
}

func parseLineIndex(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// autoPlaceItems assigns auto-placed items to the next available grid cell (sparse
// algorithm). Items with a definite column but auto row go into the next free row of
// that column; items with auto column and auto row flow in row order.
func autoPlaceItems(items []gridItem, nCols, nRows int) {
	occupied := map[[2]int]bool{}
	claim := func(r, c, spanR, spanC int) bool {
		for rr := r; rr < r+spanR; rr++ {
			for cc := c; cc < c+spanC; cc++ {
				if occupied[[2]int{rr, cc}] {
					return false
				}
			}
		}
		for rr := r; rr < r+spanR; rr++ {
			for cc := c; cc < c+spanC; cc++ {
				occupied[[2]int{rr, cc}] = true
			}
		}
		return true
	}
	for i := range items {
		it := &items[i]
		spanR := max(1, it.rowEnd-it.rowStart)
		spanC := max(1, it.colEnd-it.colStart)
		if it.colStart < 0 || it.colEnd <= 0 {
			it.colStart, it.colEnd = -1, 0
		}
		if it.rowStart < 0 || it.rowEnd <= 0 {
			it.rowStart, it.rowEnd = -1, 0
		}
		// Resolve auto column.
		if it.colStart < 0 {
			r := max(0, it.rowStart)
			for {
				placed := false
				for c := 0; c+spanC <= max(nCols, 1); c++ {
					if claim(max(r, 0), c, spanR, spanC) {
						it.colStart, it.colEnd = c, c+spanC
						if it.rowStart < 0 {
							it.rowStart, it.rowEnd = r, r+spanR
						}
						placed = true
						break
					}
				}
				if placed {
					break
				}
				r++
			}
			continue
		}
		// Resolve auto row.
		if it.rowStart < 0 {
			for r := 0; ; r++ {
				if claim(r, it.colStart, spanR, spanC) {
					it.rowStart, it.rowEnd = r, r+spanR
					break
				}
			}
		} else {
			claim(it.rowStart, it.colStart, spanR, spanC)
		}
	}
	// Normalize negative indices (count-from-end not supported; default to 0).
	for i := range items {
		if items[i].colStart < 0 {
			items[i].colStart = 0
		}
		if items[i].colEnd <= items[i].colStart {
			items[i].colEnd = items[i].colStart + 1
		}
		if items[i].rowStart < 0 {
			items[i].rowStart = 0
		}
		if items[i].rowEnd <= items[i].rowStart {
			items[i].rowEnd = items[i].rowStart + 1
		}
	}
}

// parseGridTracks parses a grid-template-columns / grid-rows value into tracks.
// Supports px, %, fr, auto and minmax(a, b).
func parseGridTracks(spec string, reference float64) []gridTrack {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "none" {
		return nil
	}
	parts := splitGridSpec(spec)
	var tracks []gridTrack
	for _, p := range parts {
		tracks = append(tracks, gridTrack{size: p})
	}
	return tracks
}

// splitGridSpec splits a grid-template value into track tokens, respecting minmax().
func splitGridSpec(spec string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range spec {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ' ':
			if depth == 0 {
				if i > start {
					out = append(out, strings.TrimSpace(spec[start:i]))
				}
				start = i + 1
			}
		}
	}
	if start < len(spec) {
		out = append(out, strings.TrimSpace(spec[start:]))
	}
	return out
}

// parseGridAreas parses grid-template-areas into a name -> [rowStart, colStart, rowEnd, colEnd]
// map (0-based, half-open intervals). The areas string uses rows separated by newlines
// or quoted strings; columns within a row are space-separated names ("." = empty).
func parseGridAreas(spec string) (map[string][4]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "none" {
		return nil, nil
	}
	// Split into rows by quotes / newlines.
	var rows []string
	cur := strings.Builder{}
	inQuote := false
	for _, r := range spec {
		if r == '"' {
			if inQuote {
				rows = append(rows, cur.String())
				cur.Reset()
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			cur.WriteRune(r)
		} else if r == '\n' {
			rows = append(rows, cur.String())
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		rows = append(rows, cur.String())
	}
	areaMap := map[string][4]int{}
	for r, row := range rows {
		names := strings.Fields(row)
		for c, name := range names {
			if name == "." {
				continue
			}
			ex, ok := areaMap[name]
			if !ok {
				areaMap[name] = [4]int{r, c, r + 1, c + 1}
				continue
			}
			if r < ex[0] {
				ex[0] = r
			}
			if r+1 > ex[2] {
				ex[2] = r + 1
			}
			if c < ex[1] {
				ex[1] = c
			}
			if c+1 > ex[3] {
				ex[3] = c + 1
			}
			areaMap[name] = ex
		}
	}
	return areaMap, nil
}

// sizeTracks resolves each track to a pixel size. px / % tracks use their declared
// size; auto tracks use the max-content size reported by maxContent; fr tracks share
// the leftover free space after the other tracks are sized.
func sizeTracks(tracks []gridTrack, reference, gap float64, maxContent func(idx int) float64) []float64 {
	sizes := make([]float64, len(tracks))
	totalFr := 0.0
	used := 0.0
	for i, t := range tracks {
		s, fr, ok := resolveTrackSize(t.size, reference, maxContent(i))
		if ok {
			sizes[i] = s
			used += s
		} else {
			totalFr += fr
		}
	}
	// Account for gaps between tracks.
	used += gap * float64(max(0, len(tracks)-1))
	free := reference - used
	if free < 0 {
		free = 0
	}
	if totalFr > 0 {
		for i, t := range tracks {
			_, fr, ok := resolveTrackSize(t.size, reference, maxContent(i))
			if !ok && fr > 0 {
				sizes[i] = free * fr / totalFr
			}
		}
	}
	return sizes
}

// resolveTrackSize returns (usedSize, frWeight, isDefinite). When isDefinite is false
// the track is an fr track and frWeight holds its weight.
func resolveTrackSize(spec string, reference, maxContent float64) (float64, float64, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "auto" {
		return maxContent, 0, true
	}
	if strings.HasPrefix(spec, "minmax(") && strings.HasSuffix(spec, ")") {
		inner := spec[len("minmax(") : len(spec)-1]
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) == 2 {
			// Use the maximum of min and (the smaller of max / max-content) per spec
			// simplification: just use the max track as the base.
			_, _, _ = parts, inner, spec
			min := parseLen(strings.TrimSpace(parts[0]), reference)
			max := parseLen(strings.TrimSpace(parts[1]), reference)
			if max > 0 {
				return math.Max(min, math.Min(max, maxContent)), 0, true
			}
			return math.Max(min, maxContent), 0, true
		}
	}
	if strings.HasSuffix(spec, "fr") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(spec, "fr"), 64)
		if err == nil {
			return 0, n, false
		}
	}
	return parseLen(spec, reference), 0, true
}

// parseLen parses a px / % length into a pixel value.
func parseLen(spec string, reference float64) float64 {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0
	}
	if strings.HasSuffix(spec, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(spec, "%"), 64)
		if err == nil {
			return n * reference / 100
		}
	}
	if strings.HasSuffix(spec, "px") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(spec, "px"), 64)
		if err == nil {
			return n
		}
	}
	n, err := strconv.ParseFloat(spec, 64)
	if err == nil {
		return n
	}
	return 0
}

// resolveGap returns the row / column gap resolved to pixels against the container
// content width.
func resolveGap(box *LayoutBox, reference float64) (rowGap, colGap float64) {
	if box.Style == nil {
		return
	}
	if box.Style.RowGap.Value > 0 || box.Style.RowGap.Unit != "" {
		rowGap = resolveOrZero(box.Style.RowGap, reference, fontSizeOf(box))
	}
	if box.Style.ColumnGap.Value > 0 || box.Style.ColumnGap.Unit != "" {
		colGap = resolveOrZero(box.Style.ColumnGap, reference, fontSizeOf(box))
	}
	if rowGap == 0 && box.Style.Gap.Value > 0 {
		rowGap = resolveOrZero(box.Style.Gap, reference, fontSizeOf(box))
	}
	if colGap == 0 && box.Style.Gap.Value > 0 {
		colGap = resolveOrZero(box.Style.Gap, reference, fontSizeOf(box))
	}
	return
}

func sumSizes(s []float64) float64 {
	s2 := 0.0
	for _, v := range s {
		s2 += v
	}
	return s2
}

func maxContentWidthOfTrack(items []gridItem, idx int) float64 {
	best := 0.0
	for _, it := range items {
		if it.colStart <= idx && it.colEnd > idx {
			if it.box.Rect.Width > best {
				best = it.box.Rect.Width
			}
		}
	}
	return best
}

func maxContentHeightOfTrack(items []gridItem, heights []float64, idx int) float64 {
	best := 0.0
	for i := range items {
		if items[i].rowStart <= idx && items[i].rowEnd > idx {
			if heights[i] > best {
				best = heights[i]
			}
		}
	}
	return best
}
