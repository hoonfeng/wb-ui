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
	spec       gridTrack
	size       float64
	maxContent float64 // intrinsic max-content contribution (column tracks)
}

type GridFormattingContext struct {
	FormattingContextBase
}

type gridItem struct {
	box                          *ElementBox
	colStart, colEnd, rowStart, rowEnd int
}

func (c *GridFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	defer profileLayout("grid")()
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
	// auto-fit collapses the tracks that no item occupies, so the repetition
	// count of `repeat(auto-fit, …)` needs the number of in-flow children.
	inFlowChildren := gridInFlowChildCount(box)

	colTracks := gridParseTracks(cs.GridTemplateColumns, cw, fs, colGap, inFlowChildren)
	rowTracks := gridParseTracks(cs.GridTemplateRows, ch, fs, rowGap, inFlowChildren)

	nCols := len(colTracks)
	nRows := len(rowTracks)

	// Resolve grid-template-areas (named template): parse the area matrix and
	// expand the implicit grid so every named cell fits.
	var areas [][]string
	if cs.GridTemplateAreas != "" {
		areas = parseGridTemplateAreas(cs.GridTemplateAreas)
		if len(areas) > 0 {
			aRows, aCols := len(areas), 0
			for _, row := range areas {
				if len(row) > aCols {
					aCols = len(row)
				}
			}
			if aCols > nCols {
				nCols = aCols
			}
			if aRows > nRows {
				nRows = aRows
			}
			for len(colTracks) < nCols {
				colTracks = append(colTracks, gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1})
			}
			for len(rowTracks) < nRows {
				rowTracks = append(rowTracks, gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1})
			}
		}
	}

	// Place items
	var items []*gridItem
	for _, child := range box.Children() {
		childEb, ok := child.(*ElementBox)
		if !ok || !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		// ★ Anonymous boxes that only wrap inter-element whitespace are not
		// grid items (CSS Grid §6.1: an anonymous item must have
		// non-whitespace content). Treating one as an item consumed the first
		// cell and pushed every real item one column right, wrapping the last
		// one to the next row (replaced-block-wrapper: #frame landed in
		// column 2 and #media wrapped to row 2, so neither panel matched).
		if anonymousWhitespaceBox(childEb) {
			continue
		}
		it := &gridItem{
			box:      childEb,
			colStart: gridParseLine(childEb.GridColumnStart()),
			colEnd:   gridParseLine(childEb.GridColumnEnd()),
			rowStart: gridParseLine(childEb.GridRowStart()),
			rowEnd:   gridParseLine(childEb.GridRowEnd()),
		}
		// Named grid-area placement: grid-area: <name> → the cell span of the
		// matching template area.
		if name := childEb.GridArea(); name != "" && name != "auto" && len(areas) > 0 {
			if cs2, rs2, ce2, re2 := gridAreaRect(areas, name); cs2 >= 0 {
				it.colStart = cs2 + 1
				it.rowStart = rs2 + 1
				it.colEnd = ce2 + 1
				it.rowEnd = re2 + 1
			}
		}
		items = append(items, it)
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

	// Expand implicit grid. colEnd/rowEnd are line numbers: an item spanning
	// tracks i..j has end line j+1, so the implicit column/row count is
	// end-1. An end line of nTracks+1 (the far edge of the explicit grid)
	// must NOT create an extra implicit track.
	for _, it := range items {
		if it.colEnd-1 > nCols {
			nCols = it.colEnd - 1
		}
		if it.rowEnd-1 > nRows {
			nRows = it.rowEnd - 1
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

	// Auto-placement: assign unique grid positions to items that were not
	// explicitly placed (colStart == 1 && rowStart == 1 && colEnd == 2 &&
	// rowEnd == 2, the default). Scan row-major for the first free cell so
	// auto items never overlap explicitly placed or spanning items (matches
	// Edge's sparse auto-flow: ge4 in the probe falls to col1,row2 instead of
	// colliding with the col1-2,row1 span).
	occupied := make(map[int]map[int]bool)
	mark := func(r, cc int) {
		if occupied[r] == nil {
			occupied[r] = map[int]bool{}
		}
		occupied[r][cc] = true
	}
	for _, it := range items {
		if isAutoItem(it) {
			continue // auto item, placed below
		}
		for r := it.rowStart; r < it.rowEnd; r++ {
			for cc := it.colStart; cc < it.colEnd; cc++ {
				mark(r, cc)
			}
		}
	}
	for _, it := range items {
		if isAutoItem(it) {
			placed := false
			for r := 1; r <= nRows && !placed; r++ {
				for cc := 1; cc <= nCols && !placed; cc++ {
					if !occupied[r][cc] {
						it.colStart = cc
						it.colEnd = cc + 1
						it.rowStart = r
						it.rowEnd = r + 1
						mark(r, cc)
						placed = true
					}
				}
			}
			if !placed {
				// Grid is full: append an implicit row.
				nRows++
				rowTracks = append(rowTracks, gridTrack{typ: gridTrackAuto, minVal: -1, maxVal: -1})
				it.colStart = 1
				it.colEnd = 2
				it.rowStart = nRows
				it.rowEnd = nRows + 1
				mark(nRows, 1)
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
	// ★ content-distribution（CSS Box Alignment §5）：轨道组在容器内容盒内按
	// justify-content/align-content 对齐，space-* 关键字把剩余空间分到轨道之间。
	// 此前轨道恒从内容盒起点排起，`place-content: center end` 的容器里轨道组
	// 全贴起点（item-self-alignment 的 #grid-content 两个轨道项）。
	colPos = gridDistributeTracks(colPos, colState, g.ContentBoxLeft(), cw, colGap, gridKeyword(cs.JustifyContent, "start"))
	rowPos = gridDistributeTracks(rowPos, rowState, g.ContentBoxTop(), ch, rowGap, gridKeyword(cs.AlignContent, "start"))
	// ★ RTL：列线从内容盒的 inline-start 侧起编号（CSS Grid §7.1），
	// direction:rtl 下 inline-start 是右边 —— 整条列带贴右，剩余空间留在
	// 左侧。此前列位置恒从 ContentBoxLeft 向右累加，RTL 网格的 item 全部
	// 偏左（direction-rtl 的 .case.grid 期望首列 track 在 x=850，实测 x=0）。
	if isRTL(cs) {
		colPos = gridMirrorLines(colPos, g.ContentBoxLeft()+cw)
	}

	gridPlaceItems(items, colPos, rowPos, colState, rowState, colGap, rowGap,
		gridKeyword(cs.AlignItems, "stretch"), gridKeyword(cs.JustifyItems, "stretch"), state)

	// Lay out absolutely/fixed-positioned children against this grid
	// container as their containing block (CSS-GRID-1 §9.2). Previously they
	// were silently dropped — .toast-container (position:fixed; top:40px;
	// right:16px) inside app-root stayed at 0x0 instead of the viewport
	// corner. position:fixed resolves against the viewport (root).
	root := stateRootForBox(box)
	for _, child := range box.Children() {
		if childEb, ok := child.(*ElementBox); ok && childEb.IsAbsolutelyPositioned() {
			cb := containingBlockForAbsolute(childEb, root)
			layoutAbsolute(childEb, cb, root, state)
		}
	}

	lastRowEnd := rowPos[len(rowPos)-1]
	contentH := math.Max(0, lastRowEnd-g.ContentBoxTop())
	// The container's height:auto resolves to its content extent. Do NOT
	// max() against the pre-layout ContentHeight: block ancestors pre-size
	// grid boxes to the viewport height before their FC runs, so max() would
	// leave the box stuck at viewport height. Only an explicit CSS height
	// (which ancestors set before this FC) should be preserved.
	if !heightIsAutoForBox(box) {
		g.SetContentHeight(math.Max(g.ContentHeight(), contentH))
	} else {
		g.SetContentHeight(contentH)
	}
}



// ── Track parsing ──

func gridParseTracks(value string, avail, fs, gap float64, itemCount int) []gridTrack {
	if value == "" || value == "none" {
		return nil
	}
	out := make([]gridTrack, 0)
	for _, tok := range gridTokenize(gridExpandRepeat(value, avail, gap, itemCount)) {
		out = append(out, gridParseOne(tok, avail, fs))
	}
	return out
}

// gridExpandRepeat rewrites every repeat() function into an explicit track list.
// `auto-fill` / `auto-fit` are resolved against the container's available size:
// the count is the largest number of tracks whose definite minimum size (plus
// the gap) fits, and auto-fit additionally drops the tracks no item will occupy.
// Previously the repetition count was parsed with strconv.Atoi, which fails on
// `auto-fill`/`auto-fit` and made the whole repeat() be dropped: the container
// got no explicit columns, so every item collapsed to width 0 (fixture
// render-repros/grid-auto-repeat.html).
func gridExpandRepeat(in string, avail, gap float64, itemCount int) string {
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
		countToken := strings.TrimSpace(inner[:ci])
		trk := strings.TrimSpace(inner[ci+1:])
		cnt := 0
		switch {
		case strings.EqualFold(countToken, "auto-fill"):
			cnt = gridAutoRepeatCount(trk, avail, gap, false, itemCount)
		case strings.EqualFold(countToken, "auto-fit"):
			cnt = gridAutoRepeatCount(trk, avail, gap, true, itemCount)
		default:
			cnt, _ = strconv.Atoi(countToken)
		}
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

// gridInFlowChildCount counts the children that take part in grid placement
// (in-flow and visible). auto-fit uses it to collapse the tracks left empty.
func gridInFlowChildCount(box *ElementBox) int {
	n := 0
	for _, child := range box.Children() {
		if child.IsInFlow() && child.IsVisible() {
			n++
		}
	}
	return n
}

// gridAutoRepeatCount resolves the repetition count of
// `repeat(auto-fill|auto-fit, <track>)`: the largest number of tracks of the
// track's definite minimum size (plus the gap) that fits in avail. auto-fit
// additionally collapses the tracks no item occupies — otherwise an empty track
// would keep its minimum size and squeeze the occupied ones.
func gridAutoRepeatCount(track string, avail, gap float64, autoFit bool, itemCount int) int {
	step := gridTrackDefiniteMin(track) + gap
	if step <= 0 || avail <= 0 {
		return 1
	}
	n := int(math.Floor((avail + gap) / step))
	if n < 1 {
		n = 1
	}
	if autoFit && itemCount > 0 && itemCount < n {
		n = itemCount
	}
	return n
}

// gridTrackDefiniteMin returns the definite (px) minimum size of a track
// definition, or 0 when the definition has none — `fr`, `auto` and percentages
// are not definite for this purpose.
func gridTrackDefiniteMin(track string) float64 {
	s := strings.TrimSpace(track)
	if strings.HasPrefix(s, "minmax(") {
		inner := strings.TrimSuffix(s[len("minmax("):], ")")
		if i := strings.Index(inner, ","); i >= 0 {
			s = strings.TrimSpace(inner[:i])
		}
	}
	if strings.HasSuffix(strings.ToLower(s), "px") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s[:len(s)-2]), 64); err == nil {
			return v
		}
	}
	return 0
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
	if l.Unit == "" || l.Unit == "normal" || l.Unit == "auto" {
		return 0
	}
	// 复用 resolveLength 的完整单位/上下文支持：rem 取根字号、vw/vh/vmin/vmax
	// 取视口、% 按容器对应尺寸、calc() 带真实 context 求值。此前这里只认
	// px/em/%，其余单位落到 default 分支返回**字面量**——`gap: 4rem` 变成 4px、
	// `gap: calc(1rem + 2vw)` 因为 Unit=="calc" 且 Value==0 直接返回 0，
	// 于是 `gap: 4rem calc(1rem + 2vw)` 的行/列间距双双失效
	// （contextual-grid-gap：期望行间距 40px、列间距 calc(10px + 18px)=28px，
	// 实测 4px 与 0）。
	r := resolveLength(l, avail, fs)
	if !r.Definite {
		return 0
	}
	// gap 不接受负值（CSS Box Alignment §8.1 视同 0）。
	return math.Max(0, r.Value)
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

	// Step 1: Content-based growth. For auto/min-content/max-content tracks,
	// the track grows to fit the content of the items that span it. For row
	// tracks (isCol=false) the content contribution is the item's intrinsic
	// height; for column tracks it is the intrinsic width.
	for _, sp := range spans {
		cur := 0.0
		for i := sp.start; i < sp.end; i++ {
			cur += states[i].size
		}
		if sp.count > 1 {
			cur += gap * float64(sp.count-1)
		}

		// Content contribution: for fixed-size items the declared size is
		// used; otherwise measure intrinsic content (text line height for
		// rows, measured text width for columns).
		content := 0.0
		if cb := sp.box; cb != nil {
			cfs := fontSizeOf(cb)
			_, p, b := computeBoxModel(cb, avail, cfs)
			hp := p.Left + p.Right + b.Left + b.Right
			csb := cb.Style()
			if csb != nil {
				var declared float64
				if isCol {
					if w, ok := definiteWidth(csb.Width, avail, cfs); ok && w > 0 {
						declared = w
					}
				} else {
					if h, ok := definiteHeight(csb.Height, 100, cfs); ok && h > 0 {
						declared = h
					}
				}
				if declared > 0 {
					if isBorderBox(cb) {
						declared = math.Max(0, declared-hp) // content-box
					}
					content = declared
				} else {
					// Intrinsic: text content height (row) or width (col).
					if isCol {
						// Auto tracks use MIN-content as their base (CSS Grid
						// §12.4: fr maximizes first, auto grows to max-content
						// only with remaining space). Using max-content here
						// let the auto right-panel consume 855px and crushed
						// the 1fr main column to 98px.
						content = gridIntrinsicMinTextWidth(cb, cfs)
						// Remember the max-content width so Step 4 can grow
						// this track toward it without overshooting.
						mc := gridIntrinsicTextWidth(cb, cfs) + hp
						if mc > 0 {
							for i := sp.start; i < sp.end; i++ {
								if mc/float64(sp.count) > states[i].maxContent {
									states[i].maxContent = mc / float64(sp.count)
								}
							}
						}
					} else {
						// Intrinsic content height: measure the item's real
						// content height (fixed-height children, padding,
						// wrapped text) rather than only the font line gap.
						// A grid row holding a card with a 26px icon used to
						// get a ~line-gap row, so tall cards overlapped.
						content = intrinsicContentHeight(cb)
						if content <= 0 {
							lh := fontLineGap(cb)
							if lh <= 0 {
								lh = cfs * 1.2
							}
							content = lh
						}
					}
				}
			}
		}
		extra := content - cur
		if extra <= 0 {
			continue
		}

		var grow []int
		for i := sp.start; i < sp.end; i++ {
			// fr tracks have a base size of 0 (CSS Grid §12.4): they are
			// sized purely by flex distribution in step 3, never by content.
			// Feeding content width into an fr track made the grid overflow
			// its container (e.g. a 1fr main column stuck at its text width
			// while the auto right panel already consumed the space).
			if states[i].spec.typ != gridTrackFixed && states[i].spec.typ != gridTrackFlex {
				grow = append(grow, i)
			}
		}
		if len(grow) == 0 {
			// Only auto tracks absorb intrinsic content growth; fr tracks
			// remain at their flex base (0) even when no other track can
			// grow, otherwise a 1fr column balloons to its text width and
			// overflows the grid.
			share := extra / float64(sp.count)
			for i := sp.start; i < sp.end; i++ {
				if states[i].spec.typ != gridTrackFlex {
					states[i].size += share
				}
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

	// Step 4: grow auto/min/max COLUMN tracks toward max-content with any
	// space that remains after fr distribution (CSS Grid §12.4). Without this
	// the auto right-panel would be stuck at its min-content (a few words)
	// while the 1fr main column absorbed all remaining width. Row tracks are
	// excluded: their height is content/stretch driven and sharing leftover
	// space equally would inflate every implicit row. Growth is capped at the
	// recorded max-content so empty auto tracks stay 0.
	if rem > 0 && isCol {
		var grow []int
		for i := range states {
			if states[i].spec.typ != gridTrackFixed && states[i].spec.typ != gridTrackFlex {
				grow = append(grow, i)
			}
		}
		if len(grow) > 0 {
			// Distribute greedily: grow each track up to its max-content,
			// then hand the leftover to the next track.
			leftover := rem
			for len(grow) > 0 && leftover > 0 {
				share := leftover / float64(len(grow))
				next := grow[:0]
				for _, idx := range grow {
					cap := states[idx].maxContent - states[idx].size
					if cap <= 0 {
						continue
					}
					add := share
					if add > cap {
						add = cap
					}
					states[idx].size += add
					leftover -= add
					if states[idx].size < states[idx].maxContent {
						next = append(next, idx)
					}
				}
				grow = next
			}
		}
	}
}

func gridIntrinsicTextWidth(box *ElementBox, fs float64) float64 {
	total := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				total += measureText(box, itb.Text())
			} else if eb, ok := c.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)
	return total
}

// gridIntrinsicMinTextWidth returns the min-content contribution: the widest
// single word / unbreakable token. CSS Grid sizes auto tracks from min-content
// first, then grows them toward max-content with leftover space. Summing all
// text (max-content) as the base made an auto grid column balloon to its full
// unwrapped width and crush sibling fr columns.
func gridIntrinsicMinTextWidth(box *ElementBox, fs float64) float64 {
	maxW := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				for _, w := range strings.Fields(itb.Text()) {
					if mw := measureText(box, w); mw > maxW {
						maxW = mw
					}
				}
			} else if eb, ok := c.(*ElementBox); ok {
				walk(eb)
			}
		}
	}
	walk(box)
	return maxW
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

// gridMirrorLines 把按 LTR（列线 1 在最左）累加出的列线位置镜像到 RTL：
// 列线 1 落在 inline-start（右侧），其余列线依次向左。列线编号语义不变
// （索引仍是逻辑顺序），只有物理 x 变成递减——因此调用方取单元左边界时
// 要用 min(p[cs], p[ce]) 而不是 p[cs]。right 为内容盒的右边界。
func gridMirrorLines(p []float64, right float64) []float64 {
	out := make([]float64, len(p))
	if len(p) == 0 {
		return out
	}
	base := p[0]
	for i, v := range p {
		out[i] = right - (v - base)
	}
	return out
}

// gridKeyword resolves an alignment keyword, treating the initial values
// (empty, `auto`, `normal`) as "not specified" so the fallback wins: an item's
// align-self:auto falls back to the container's align-items, and the
// container's initial value to stretch (CSS Box Alignment §4–6).
func gridKeyword(value, fallback string) string {
	switch v := strings.ToLower(strings.TrimSpace(value)); v {
	case "", "auto", "normal":
		return fallback
	default:
		return v
	}
}

// gridDistributeTracks applies align-content/justify-content to a track line
// list: the free space inside the container is used to shift the whole track
// group and/or to spread the tracks apart (CSS Box Alignment §5).
func gridDistributeTracks(pos []float64, states []gridTrackState, origin, containerSize, gap float64, keyword string) []float64 {
	n := len(states)
	if len(pos) < 2 || n == 0 || containerSize <= 0 {
		return pos
	}
	used := 0.0
	for i := range states {
		used += states[i].size
	}
	free := containerSize - used
	if n > 1 {
		free -= gap * float64(n-1)
	}
	if free <= 0 {
		return pos
	}
	offset, extra := 0.0, 0.0
	switch keyword {
	case "center":
		offset = free / 2
	case "end", "flex-end", "right", "bottom":
		offset = free
	case "space-between":
		if n > 1 {
			extra = free / float64(n-1)
		}
	case "space-around":
		extra = free / float64(n)
		offset = extra / 2
	case "space-evenly":
		extra = free / float64(n+1)
		offset = extra
	default:
		// start / stretch / normal / auto: tracks begin at the content-box
		// origin, which is what gridTrackPos already produced.
		return pos
	}
	out := make([]float64, len(pos))
	out[0] = origin + offset
	for i := 0; i < n; i++ {
		out[i+1] = out[i] + states[i].size
		if i < n-1 {
			out[i+1] += gap + extra
		}
	}
	return out
}

func gridPlaceItems(items []*gridItem, colPos, rowPos []float64, colState, rowState []gridTrackState, colGap, rowGap float64, alignItems, justifyItems string, state *LayoutState) {
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
		rt := rowPos[rs]
		// RTL 网格的列线是镜像的（x 递减），单元左边界是两条列线中较小的
		// 那条；LTR 下 colPos[cs] < colPos[ce]，min 结果不变。
		if clEnd := colPos[ce]; clEnd < cl {
			cl = clEnd
		}
		// Cell size: sum of spanned track sizes + internal gaps only. Using
		// colPos[ce]-colPos[cs] would include the trailing gap after every
		// non-last track, over-sizing the cell by one gap.
		cw := 0.0
		for i := cs; i < ce; i++ {
			cw += colState[i].size
		}
		if ce-cs > 1 {
			cw += colGap * float64(ce-cs-1)
		}
		ch := 0.0
		for i := rs; i < re; i++ {
			ch += rowState[i].size
		}
		if re-rs > 1 {
			ch += rowGap * float64(re-rs-1)
		}

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

		// ★ The available width/height is the interior of the item's margin
		// box, so padding and border come off in *both* box-sizing modes: a
		// stretched item's border box fills the cell either way, hence a
		// content-box item with 20px padding lays out at cell−40, not at the
		// full cell width (replaced-grid-order: the padded media wrapper grew
		// to 340px inside a 300px column and its 100%-wide image overflowed).
		hp := padding.Left + padding.Right + border.Left + border.Right
		vp := padding.Top + padding.Bottom + border.Top + border.Bottom
		// ★ Self-alignment (CSS Box Alignment §6): a definite width/height
		// keeps its own size and is then positioned by justify-self/align-self;
		// otherwise the item stretches across the cell. The item's own
		// align-self/justify-self (when not auto) override the container's
		// align-items/justify-items, whose initial value means stretch.
		itemCS := it.box.Style()
		fsItem := fontSizeOf(it.box)
		align, justify := alignItems, justifyItems
		if itemCS != nil {
			align = gridKeyword(itemCS.AlignSelf, align)
			justify = gridKeyword(itemCS.JustifySelf, justify)
		}
		contentW := math.Max(0, aw-hp)
		if itemCS != nil {
			if w, ok := definiteWidth(itemCS.Width, aw, fsItem); ok && w > 0 {
				if isBorderBox(it.box) {
					contentW = math.Max(0, w-hp)
				} else {
					contentW = w
				}
			}
		}
		contentH := math.Max(0, ah-vp)
		if itemCS != nil {
			if h, ok := definiteHeight(itemCS.Height, ah, fsItem); ok && h > 0 {
				if isBorderBox(it.box) {
					contentH = math.Max(0, h-vp)
				} else {
					contentH = h
				}
			}
		}
		ig.SetContentWidth(contentW)
		ig.SetContentHeight(contentH)
		// ★ 记录 grid 分配的固定高度（行高/stretch），供 item 自身 auto 高度计算
		// 区分「父分配高度」与「自身 auto 高度残留」（复用 geometry 时）。
		it.box.SetParentSetHeight(contentH)

		// Position BEFORE layout, so child layout sees correct absolute coordinates.
		// The border box sits inside the cell according to the alignment
		// keywords; start (and stretch) keep the cell's leading edge.
		bbw, bbh := contentW+hp, contentH+vp
		x := cl + ml
		switch justify {
		case "center":
			x = cl + ml + math.Max(0, (aw-bbw)/2)
		case "end", "flex-end", "right":
			x = cl + ml + math.Max(0, aw-bbw)
		}
		y := rt + mt
		switch align {
		case "center":
			y = rt + mt + math.Max(0, (ah-bbh)/2)
		case "end", "flex-end", "bottom":
			y = rt + mt + math.Max(0, ah-bbh)
		}
		ig.SetTopLeft(y, x)

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

// anonymousWhitespaceBox reports whether box is an anonymous box (one that has
// no DOM element of its own) whose only content is whitespace. CSS Grid §6.1:
// whitespace-only text does not generate an anonymous grid item, so such a box
// must be skipped during placement.
//
// The "actually has content" test matters: synthetic trees (unit tests,
// detached fragments) also build boxes with no DOM element, and those are real
// items — an *empty* anonymous box is therefore not discarded here.
func anonymousWhitespaceBox(box *ElementBox) bool {
	if box.Element() != nil {
		return false
	}
	sawText := false
	var walk func(b *ElementBox) bool
	walk = func(b *ElementBox) bool {
		for _, c := range b.Children() {
			switch v := c.(type) {
			case *InlineTextBox:
				if strings.TrimSpace(v.Text()) != "" {
					return false
				}
				sawText = true
			case *ElementBox:
				if v.Element() != nil || !walk(v) {
					return false
				}
			}
		}
		return true
	}
	return walk(box) && sawText
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

// isAutoItem reports whether the item has no explicit/named placement.
func isAutoItem(it *gridItem) bool {
	return it.box.GridArea() == "" &&
		it.colStart == 1 && it.rowStart == 1 && it.colEnd == 2 && it.rowEnd == 2
}

// parseGridTemplateAreas parses the grid-template-areas value into a matrix
// of area names. Values look like: "title title" "actbar sidebar" — each
// quoted string is one row; dots (.) mean empty cells.
func parseGridTemplateAreas(s string) [][]string {
	var rows [][]string
	// Split by quoted strings first (each "..." is a row).
	strs := extractQuotedStrings(s)
	if len(strs) == 0 {
		// Fallback: whitespace rows (unquoted form).
		strs = strings.Fields(s)
		if len(strs) == 0 {
			return nil
		}
		rows = append(rows, strs)
		return rows
	}
	for _, row := range strs {
		fields := strings.Fields(row)
		if len(fields) == 0 {
			continue
		}
		rows = append(rows, fields)
	}
	return rows
}

// extractQuotedStrings pulls every "..." (or '...') chunk out of a CSS value.
func extractQuotedStrings(s string) []string {
	var out []string
	for len(s) > 0 {
		q := strings.IndexAny(s, "\"'")
		if q < 0 {
			break
		}
		quote := s[q]
		rest := s[q+1:]
		end := strings.IndexByte(rest, quote)
		if end < 0 {
			break
		}
		out = append(out, rest[:end])
		s = rest[end+1:]
	}
	return out
}

// gridAreaRect finds the rectangular span of the named template area.
// Returns (colStart, rowStart, colEnd, rowEnd) as 0-based cell indices
// (end exclusive), or colStart=-1 when the name is not present.
func gridAreaRect(areas [][]string, name string) (int, int, int, int) {
	minR, minC := -1, -1
	maxR, maxC := -1, -1
	for r, row := range areas {
		for c, cell := range row {
			if cell == name {
				if minR < 0 || r < minR {
					minR = r
				}
				if minC < 0 || c < minC {
					minC = c
				}
				if r > maxR {
					maxR = r
				}
				if c > maxC {
					maxC = c
				}
			}
		}
	}
	if minR < 0 {
		return -1, -1, -1, -1
	}
	return minC, minR, maxC + 1, maxR + 1
}

var _ = style.DisplayGrid
