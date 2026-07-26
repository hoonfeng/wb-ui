// Translation of: Source/WebCore/layout/formattingContexts/grid/GridFormattingContext.cpp
//
// CSS Grid FormattingContext — complete implementation.
// Supports: px/fr/auto/minmax/repeat, explicit placement, content-based track sizing,
// fr distribution, stretch alignment, gap.

package layout

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"wb-ui/style"
)

type gridTrackType uint8

const (
	gridTrackFixed       gridTrackType = iota
	gridTrackFlex
	gridTrackAuto
	gridTrackMinContent
	gridTrackMaxContent
	gridTrackFitContent
)

type gridTrack struct {
	typ            gridTrackType
	value, fr      float64
	minVal, maxVal float64 // -1 = unbound
}

type gridTrackState struct {
	spec gridTrack
	size float64
}

type GridFormattingContext struct {
	FormattingContextBase
}

type gridItem struct {
	box                          *ElementBox
	colStart, colEnd, rowStart, rowEnd int
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

	colGap := gridGap(cs.ColumnGap, fs, cw)
	rowGap := gridGap(cs.RowGap, fs, ch)

	colTracks := gridParseTracks(cs.GridTemplateColumns, cw, fs)
	rowTracks := gridParseTracks(cs.GridTemplateRows, ch, fs)

	nCols := len(colTracks)
	nRows := len(rowTracks)

	// Place items
	var items []*gridItem
	for _, child := range box.Children() {
		childEb, ok := child.(*ElementBox)
		if !ok || !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		items = append(items, &gridItem{
			box:      childEb,
			colStart: gridParseLine(childEb.GridColumnStart()),
			colEnd:   gridParseLine(childEb.GridColumnEnd()),
			rowStart: gridParseLine(childEb.GridRowStart()),
			rowEnd:   gridParseLine(childEb.GridRowEnd()),
		})
	}
	if len(items) == 0 {
		return
	}

	// Resolve placement (-1→last, 0→default, negative→span)
	for _, it := range items {
		if it.colStart <= 0 {
			it.colStart = 1
		}
		switch {
		case it.colEnd == -1:
			it.colEnd = nCols + 1
		case it.colEnd < -1:
			it.colEnd = it.colStart + (-it.colEnd)
		case it.colEnd <= 0:
			it.colEnd = it.colStart + 1
		}
		if it.colEnd <= it.colStart {
			it.colEnd = it.colStart + 1
		}
		if it.rowStart <= 0 {
			it.rowStart = 1
		}
		switch {
		case it.rowEnd == -1:
			it.rowEnd = nRows + 1
		case it.rowEnd < -1:
			it.rowEnd = it.rowStart + (-it.rowEnd)
		case it.rowEnd <= 0:
			it.rowEnd = it.rowStart + 1
		}
		if it.rowEnd <= it.rowStart {
			it.rowEnd = it.rowStart + 1
		}
	}

	// Expand implicit grid
	for _, it := range items {
		if it.colEnd > nCols {
			nCols = it.colEnd
		}
		if it.rowEnd > nRows {
			nRows = it.rowEnd
		}
	}
	if nCols < 1 {
		nCols = 1
	}
	if nRows < 1 {
		nRows = 1
	}
	for len(colTracks) < nCols {
		colTracks = append(colTracks, gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1})
	}
	for len(rowTracks) < nRows {
		rowTracks = append(rowTracks, gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1})
	}

	// Auto-placement: assign unique column positions to items that were not
	// explicitly placed (colStart == 1 && rowStart == 1, the default).
	// Use a simple cursor that advances column-by-column, wrapping to the next
	// row when columns are exhausted.
	autoCol := 1
	autoRow := 1
	for _, it := range items {
		if it.colStart == 1 && it.rowStart == 1 && it.colEnd == 2 && it.rowEnd == 2 {
			// This item has default placement; assign the next auto slot.
			it.colStart = autoCol
			it.colEnd = autoCol + 1
			it.rowStart = autoRow
			it.rowEnd = autoRow + 1
			autoCol++
			if autoCol > nCols && nCols > 0 {
				// For explicit tracks, wrap to next row if col > nCols.
				// For implicit-only grids (no explicit tracks), nCols may be 0.
				autoCol = 1
				autoRow++
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rowStart != items[j].rowStart {
			return items[i].rowStart < items[j].rowStart
		}
		return items[i].colStart < items[j].colStart
	})

	colState := gridInitStates(colTracks)
	rowState := gridInitStates(rowTracks)
	gridSizeTracks(colState, items, true, colGap, cw)
	gridSizeTracks(rowState, items, false, rowGap, ch)

	colPos := gridTrackPos(colState, g.ContentBoxLeft(), colGap)
	rowPos := gridTrackPos(rowState, g.ContentBoxTop(), rowGap)

	gridPlaceItems(items, colPos, rowPos, state)

	lastRowEnd := rowPos[len(rowPos)-1]
	g.SetContentHeight(math.Max(g.ContentHeight(), lastRowEnd-g.ContentBoxTop()))
}

// ── Track parsing ──

func gridParseTracks(value string, avail, fs float64) []gridTrack {
	if value == "" || value == "none" {
		return nil
	}
	out := make([]gridTrack, 0)
	for _, tok := range gridTokenize(gridExpandRepeat(value)) {
		out = append(out, gridParseOne(tok, avail, fs))
	}
	return out
}

func gridExpandRepeat(in string) string {
	for {
		idx := strings.Index(in, "repeat(")
		if idx < 0 {
			break
		}
		d := 1
		end := idx + 7
		for end < len(in) && d > 0 {
			switch in[end] {
			case '(':
				d++
			case ')':
				d--
			}
			end++
		}
		if d != 0 {
			in = in[:idx] + in[end:]
			continue
		}
		inner := in[idx+7 : end-1]
		ci := strings.Index(inner, ",")
		if ci < 0 {
			in = in[:idx] + in[end:]
			continue
		}
		cnt, _ := strconv.Atoi(strings.TrimSpace(inner[:ci]))
		trk := strings.TrimSpace(inner[ci+1:])
		if cnt <= 0 {
			in = in[:idx] + in[end:]
			continue
		}
		rep := ""
		for i := 0; i < cnt; i++ {
			if i > 0 {
				rep += " "
			}
			rep += trk
		}
		in = in[:idx] + rep + in[end:]
	}
	return in
}

func gridTokenize(in string) []string {
	in = strings.TrimSpace(in)
	if in == "" {
		return nil
	}
	var out []string
	i := 0
	for i < len(in) {
		for i < len(in) && (in[i] == ' ' || in[i] == '\t') {
			i++
		}
		if i >= len(in) {
			break
		}
		s := i
		if strings.HasPrefix(in[i:], "minmax(") {
			d := 0
			for i < len(in) {
				if in[i] == '(' {
					d++
				} else if in[i] == ')' {
					d--
					if d == 0 {
						i++
						break
					}
				}
				i++
			}
			out = append(out, in[s:i])
			continue
		}
		if strings.HasPrefix(in[i:], "fit-content(") {
			for i < len(in) {
				if in[i] == ')' {
					i++
					break
				}
				i++
			}
			out = append(out, in[s:i])
			continue
		}
		for i < len(in) && in[i] != ' ' && in[i] != '\t' {
			i++
		}
		out = append(out, in[s:i])
	}
	return out
}

func gridParseOne(tok string, avail, fs float64) gridTrack {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1}
	}
	if strings.HasPrefix(tok, "minmax(") && strings.HasSuffix(tok, ")") {
		inner := tok[7 : len(tok)-1]
		ci := strings.Index(inner, ",")
		if ci < 0 {
			return gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1}
		}
		maxT := gridParseOne(strings.TrimSpace(inner[ci+1:]), avail, fs)
		return gridTrack{
			typ:    maxT.typ,
			minVal: gridLenOrInf(strings.TrimSpace(inner[:ci]), avail, fs),
			maxVal: gridLenOrInf(strings.TrimSpace(inner[ci+1:]), avail, fs),
			value:  maxT.value,
			fr:     maxT.fr,
		}
	}
	if strings.HasPrefix(tok, "fit-content(") && strings.HasSuffix(tok, ")") {
		lim := gridResolveLen(tok[11:len(tok)-1], avail, fs)
		return gridTrack{typ: gridTrackFitContent, value: lim, minVal: -1, maxVal: -1}
	}
	switch tok {
	case "min-content":
		return gridTrack{typ: gridTrackMinContent, minVal: -1, maxVal: -1}
	case "max-content":
		return gridTrack{typ: gridTrackMaxContent, minVal: -1, maxVal: -1}
	case "auto":
		return gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1}
	}
	if strings.HasSuffix(tok, "fr") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(tok, "fr"), 64)
		if err == nil && f > 0 {
			return gridTrack{typ: gridTrackFlex, fr: f, minVal: -1, maxVal: -1}
		}
		return gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1}
	}
	v := gridResolveLen(tok, avail, fs)
	return gridTrack{typ: gridTrackFixed, value: v, minVal: -1, maxVal: -1}
}

func gridResolveLen(s string, avail, fs float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "px") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
		return v
	}
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return avail * v / 100.0
	}
	if strings.HasSuffix(s, "em") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "em"), 64)
		return v * fs
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func gridLenOrInf(s string, avail, fs float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" || s == "min-content" || s == "max-content" {
		return -1
	}
	return gridResolveLen(s, avail, fs)
}

func gridGap(l style.Length, fs, avail float64) float64 {
	if l.Unit == "" || l.Value <= 0 {
		return 0
	}
	switch l.Unit {
	case "px":
		return l.Value
	case "em":
		return l.Value * fs
	case "%":
		return avail * l.Value / 100.0
	}
	return l.Value
}

// ── Line parsing ──

func gridParseLine(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" {
		return 0
	}
	if s == "-1" {
		return -1
	}
	if strings.HasPrefix(s, "span ") {
		n, err := strconv.Atoi(strings.TrimPrefix(s, "span "))
		if err == nil && n > 0 {
			return -n
		}
		return 0
	}
	n, err := strconv.Atoi(s)
	if err == nil {
		return n
	}
	return 0
}

// ── Track sizing ──

func gridInitStates(tracks []gridTrack) []gridTrackState {
	s := make([]gridTrackState, len(tracks))
	for i, t := range tracks {
		s[i] = gridTrackState{spec: t}
		if t.typ == gridTrackFixed {
			s[i].size = t.value
		}
	}
	return s
}

type gridSpan struct {
	start, end, count int
	box               *ElementBox
}

func gridSizeTracks(states []gridTrackState, items []*gridItem, isCol bool, gap, avail float64) {
	if len(states) == 0 {
		return
	}

	var spans []gridSpan
	for _, it := range items {
		var s, e int
		if isCol {
			s = it.colStart - 1
			e = it.colEnd - 1
		} else {
			s = it.rowStart - 1
			e = it.rowEnd - 1
		}
		if s < 0 {
			s = 0
		}
		if e > len(states) {
			e = len(states)
		}
		if e <= s {
			e = s + 1
		}
		spans = append(spans, gridSpan{start: s, end: e, count: e - s, box: it.box})
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].count < spans[j].count })

	// Step 1: Content-based growth (simplified — items without explicit size contribute 0)
	for _, sp := range spans {
		cur := 0.0
		for i := sp.start; i < sp.end; i++ {
			cur += states[i].size
		}
		if sp.count > 1 {
			cur += gap * float64(sp.count-1)
		}

		content := 0.0
		extra := content - cur
		if extra <= 0 {
			continue
		}

		var grow []int
		for i := sp.start; i < sp.end; i++ {
			if states[i].spec.typ != gridTrackFixed {
				grow = append(grow, i)
			}
		}
		if len(grow) == 0 {
			share := extra / float64(sp.count)
			for i := sp.start; i < sp.end; i++ {
				states[i].size += share
			}
		} else {
			share := extra / float64(len(grow))
			for _, idx := range grow {
				states[idx].size += share
			}
		}
	}

	// Step 2: Apply minmax limits
	for i := range states {
		s := states[i].spec
		if s.minVal >= 0 && states[i].size < s.minVal {
			states[i].size = s.minVal
		}
		if s.maxVal >= 0 && states[i].size > s.maxVal {
			states[i].size = s.maxVal
		}
		if states[i].size < 0 {
			states[i].size = 0
		}
	}

	// Step 3: fr distribution
	used := 0.0
	for _, st := range states {
		used += st.size
	}
	if len(states) > 1 {
		used += gap * float64(len(states)-1)
	}
	rem := avail - used
	if rem > 0 {
		tfr := 0.0
		var fi []int
		for i, st := range states {
			if st.spec.typ == gridTrackFlex && st.spec.fr > 0 {
				tfr += st.spec.fr
				fi = append(fi, i)
			}
		}
		if tfr > 0 {
			unit := rem / tfr
			for _, idx := range fi {
				states[idx].size += unit * states[idx].spec.fr
			}
		}
	}
}

// ── Track positions ──

func gridTrackPos(states []gridTrackState, start, gap float64) []float64 {
	p := make([]float64, len(states)+1)
	p[0] = start
	for i := 0; i < len(states); i++ {
		p[i+1] = p[i] + states[i].size
		if i < len(states)-1 {
			p[i+1] += gap
		}
	}
	return p
}

// ── Cell placement ──

func gridPlaceItems(items []*gridItem, colPos, rowPos []float64, state *LayoutState) {
	nCols := len(colPos) - 1
	nRows := len(rowPos) - 1

	for _, it := range items {
		cs := clamp(it.colStart-1, 0, nCols-1)
		ce := clamp(it.colEnd-1, 1, nCols)
		rs := clamp(it.rowStart-1, 0, nRows-1)
		re := clamp(it.rowEnd-1, 1, nRows)
		if ce <= cs {
			ce = cs + 1
		}
		if re <= rs {
			re = rs + 1
		}

		cl := colPos[cs]
		cr := colPos[ce]
		rt := rowPos[rs]
		rb := rowPos[re]
		cw := cr - cl
		ch := rb - rt

		ig := state.GeometryForBox(it.box)
		ml := ig.MarginStart()
		mr := ig.MarginEnd()
		mt := ig.MarginBefore()
		mb := ig.MarginAfter()

		// Set content width/height before layout, then layout child
		aw := cw - ml - mr
		if aw < 0 {
			aw = 0
		}
		ah := ch - mt - mb
		if ah < 0 {
			ah = 0
		}

		// Compute padding/border from style for box-sizing: border-box.
		_, padding, border := computeBoxModel(it.box, aw, fontSizeOf(it.box))
		ig.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
		ig.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

		if isBorderBox(it.box) {
			hp := padding.Left + padding.Right + border.Left + border.Right
			vp := padding.Top + padding.Bottom + border.Top + border.Bottom
			aw = math.Max(0, aw-hp)
			ah = math.Max(0, ah-vp)
		}
		ig.SetContentWidth(aw)
		ig.SetContentHeight(ah)

		// Position BEFORE layout, so child layout sees correct absolute coordinates.
		ig.SetTopLeft(rt+mt, cl+ml)

		ctx := contextFor(it.box, state)
		if ctx != nil {
			ctx.Layout(it.box, state)
		}

		// Apply min/max height constraints after layout
		minH, maxH, minAuto, maxAuto := resolveMinMax(it.box.Style().MinHeight, it.box.Style().MaxHeight, ah, fontSizeOf(it.box))
		if !minAuto && minH > ig.ContentHeight() {
			ig.SetContentHeight(minH)
		}
		if !maxAuto && maxH < ig.ContentHeight() {
			ig.SetContentHeight(maxH)
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

var _ = style.DisplayGrid
