// Translation of: Source/WebCore/layout/formattingContexts/grid/GridFormattingContext.cpp
// CSS Grid formatting context — supports grid-template-columns/rows, explicit
// placement via grid-column/grid-row, fixed/auto/1fr track sizing.

package layout

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"wb-ui/style"
)

type GridFormattingContext struct {
	FormattingContextBase
}

type trackSpec struct {
	size  float64
	fr    float64
	auto  bool
	fixed bool
}

func (c *GridFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	cw := g.ContentWidth()
	ch := g.ContentHeight()
	if cw <= 0 {
		cw = 800
	}
	if ch <= 0 {
		ch = 600
	}
	fs := fontSizeOf(box)

	colTracks := parseGridTracks(cs.GridTemplateColumns, cw, fs)
	rowTracks := parseGridTracks(cs.GridTemplateRows, ch, fs)
	if len(colTracks) == 0 {
		colTracks = []trackSpec{{auto: true, size: cw}}
	}
	if len(rowTracks) == 0 {
		rowTracks = []trackSpec{{auto: true, size: ch}}
	}

	nCols := len(colTracks)
	nRows := len(rowTracks)

	type gridPlacement struct {
		box      *ElementBox
		colStart int
		colEnd   int
		rowStart int
		rowEnd   int
	}
	var items []*gridPlacement

	for _, child := range box.Children() {
		childEb, ok := child.(*ElementBox)
		if !ok || !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		pi := &gridPlacement{box: childEb}
		pi.colStart = parseGridLine(childEb.GridColumnStart(), 1)
		colEndStr := childEb.GridColumnEnd()
		if colEndStr == "-1" {
			pi.colEnd = -1
		} else if colEndStr == "" || colEndStr == "auto" || colEndStr == "span all" {
			pi.colEnd = pi.colStart + 1
		} else {
			pi.colEnd = parseGridLine(colEndStr, nCols+1)
		}
		pi.rowStart = parseGridLine(childEb.GridRowStart(), 1)
		rowEndStr := childEb.GridRowEnd()
		if rowEndStr == "-1" {
			pi.rowEnd = -1
		} else if rowEndStr == "" || rowEndStr == "auto" || rowEndStr == "span all" {
			pi.rowEnd = pi.rowStart + 1
		} else {
			pi.rowEnd = parseGridLine(rowEndStr, nRows+1)
		}
		items = append(items, pi)
	}

	if len(items) == 0 {
		return
	}

	maxColLine := nCols + 1
	maxRowLine := nRows + 1
	for _, it := range items {
		if it.colStart == -1 {
			it.colStart = maxColLine - 1
		}
		if it.colEnd == -1 || it.colEnd > maxColLine {
			it.colEnd = maxColLine
		}
		if it.rowStart == -1 {
			it.rowStart = maxRowLine - 1
		}
		if it.rowEnd == -1 || it.rowEnd > maxRowLine {
			it.rowEnd = maxRowLine
		}
		if it.colStart < 1 {
			it.colStart = 1
		}
		if it.colEnd < it.colStart+1 {
			it.colEnd = it.colStart + 1
		}
		if it.rowStart < 1 {
			it.rowStart = 1
		}
		if it.rowEnd < it.rowStart+1 {
			it.rowEnd = it.rowStart + 1
		}
	}

	// Track resolution: fixed → auto → fr
	colSizes := make([]float64, nCols)
	rowSizes := make([]float64, nRows)
	remainingCol := cw
	remainingRow := ch
	var frColIndices, autoColIndices []int
	var frRowIndices, autoRowIndices []int

	for i, t := range colTracks {
		if t.fixed {
			colSizes[i] = t.size
			remainingCol -= t.size
		} else if t.fr > 0 {
			frColIndices = append(frColIndices, i)
		} else if t.auto {
			autoColIndices = append(autoColIndices, i)
		}
	}
	for i, t := range rowTracks {
		if t.fixed {
			rowSizes[i] = t.size
			remainingRow -= t.size
		} else if t.fr > 0 {
			frRowIndices = append(frRowIndices, i)
		} else if t.auto {
			autoRowIndices = append(autoRowIndices, i)
		}
	}

	autoDefaultW := 200.0
	autoDefaultH := 100.0
	if len(autoColIndices) > 0 && remainingCol > 0 {
		autoSz := minFloat(remainingCol/float64(len(autoColIndices)), autoDefaultW)
		for _, idx := range autoColIndices {
			colSizes[idx] = autoSz
			remainingCol -= autoSz
		}
	}
	if len(autoRowIndices) > 0 && remainingRow > 0 {
		autoSz := minFloat(remainingRow/float64(len(autoRowIndices)), autoDefaultH)
		for _, idx := range autoRowIndices {
			rowSizes[idx] = autoSz
			remainingRow -= autoSz
		}
	}

	if len(frColIndices) > 0 && remainingCol > 0 {
		totalFr := 0.0
		for _, idx := range frColIndices {
			totalFr += colTracks[idx].fr
		}
		frSz := remainingCol / totalFr
		for _, idx := range frColIndices {
			colSizes[idx] = frSz * colTracks[idx].fr
		}
	}
	if len(frRowIndices) > 0 && remainingRow > 0 {
		totalFr := 0.0
		for _, idx := range frRowIndices {
			totalFr += rowTracks[idx].fr
		}
		frSz := remainingRow / totalFr
		for _, idx := range frRowIndices {
			rowSizes[idx] = frSz * rowTracks[idx].fr
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rowStart != items[j].rowStart {
			return items[i].rowStart < items[j].rowStart
		}
		return items[i].colStart < items[j].colStart
	})

	contentBoxLeft := g.ContentBoxLeft()
	contentBoxTop := g.ContentBoxTop()
	colPos := make([]float64, nCols+1)
	colPos[0] = contentBoxLeft
	for i := 0; i < nCols; i++ {
		colPos[i+1] = colPos[i] + colSizes[i]
	}
	rowPos := make([]float64, nRows+1)
	rowPos[0] = contentBoxTop
	for i := 0; i < nRows; i++ {
		rowPos[i+1] = rowPos[i] + rowSizes[i]
	}

	maxBottom := contentBoxTop
	for _, it := range items {
		csIdx := maxInt(0, minInt(it.colStart-1, nCols-1))
		ceIdx := maxInt(1, minInt(it.colEnd-1, nCols))
		rsIdx := maxInt(0, minInt(it.rowStart-1, nRows-1))
		reIdx := maxInt(1, minInt(it.rowEnd-1, nRows))
		if ceIdx <= csIdx {
			ceIdx = csIdx + 1
		}
		if reIdx <= rsIdx {
			reIdx = rsIdx + 1
		}

		px := colPos[csIdx]
		py := rowPos[rsIdx]
		pcellW := colPos[ceIdx] - colPos[csIdx]
		pcellH := rowPos[reIdx] - rowPos[rsIdx]

		itg := state.GeometryForBox(it.box)
		itg.SetTopLeft(py, px)

		pml := itg.MarginStart()
		pmr := itg.MarginEnd()
		pmt := itg.MarginBefore()
		pmb := itg.MarginEnd()

		paw := pcellW - pml - pmr
		if paw < 0 {
			paw = 0
		}
		pah := pcellH - pmt - pmb
		if pah < 0 {
			pah = 0
		}

		itg.SetContentWidth(paw)
		itg.SetContentHeight(pah)

		ctx := contextFor(it.box, state)
		ctx.Layout(it.box, state)

		colAllAuto := true
		for ci := csIdx; ci < ceIdx; ci++ {
			if !colTracks[ci].auto {
				colAllAuto = false
				break
			}
		}
		if !colAllAuto {
			itg.SetContentWidth(paw)
		}

		rowAllAuto := true
		for ri := rsIdx; ri < reIdx; ri++ {
			if !rowTracks[ri].auto {
				rowAllAuto = false
				break
			}
		}
		if !rowAllAuto {
			itg.SetContentHeight(pah)
		} else {
			contentH := itg.ContentHeight()
			for r := rsIdx; r < reIdx; r++ {
				if rowSizes[r] < contentH {
					rowSizes[r] = contentH
					rowPos[r+1] = rowPos[r] + rowSizes[r]
					for rr := r + 1; rr < nRows; rr++ {
						rowPos[rr+1] = rowPos[rr] + rowSizes[rr]
					}
				}
			}
		}

		pbottom := py + itg.BorderBoxHeight()
		if pbottom > maxBottom {
			maxBottom = pbottom
		}
	}
	g.SetContentHeight(math.Max(g.ContentHeight(), maxBottom-g.ContentBoxTop()))
}

func parseGridTracks(value string, available, fontSize float64) []trackSpec {
	if value == "" || value == "none" {
		return nil
	}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil
	}
	tracks := make([]trackSpec, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var t trackSpec
		if p == "auto" {
			t.auto = true
			t.size = 100
		} else if strings.HasSuffix(p, "fr") {
			frStr := strings.TrimSuffix(p, "fr")
			if f, err := strconv.ParseFloat(frStr, 64); err == nil && f > 0 {
				t.fr = f
				t.size = 100
			} else {
				t.auto = true
				t.size = 100
			}
		} else if strings.HasSuffix(p, "px") {
			vStr := strings.TrimSuffix(p, "px")
			if v, err := strconv.ParseFloat(vStr, 64); err == nil {
				t.fixed = true
				t.size = v
			} else {
				t.auto = true
				t.size = 100
			}
		} else {
			if v, err := strconv.ParseFloat(p, 64); err == nil {
				t.fixed = true
				t.size = v
			} else {
				t.auto = true
				t.size = 100
			}
		}
		tracks = append(tracks, t)
	}
	return tracks
}

func parseGridLine(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" {
		return defaultVal
	}
	if strings.HasPrefix(s, "span ") {
		spanStr := strings.TrimPrefix(s, "span ")
		if span, err := strconv.Atoi(spanStr); err == nil && span > 0 {
			return span
		}
		return defaultVal
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return defaultVal
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

var _ = style.DisplayGrid
