// Translation of: Source/WebCore/layout/formattingContexts/table/TableFormattingContext.cpp
// Simplified table layout.

package layout

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/engine/style"
)

type TableFormattingContext struct {
	FormattingContextBase
}

// collectRows flattens the table's child tree into a row list, handling
// thead/tbody/tfoot grouping elements.
func collectRows(box *ElementBox) []*ElementBox {
	var rows []*ElementBox
	for _, child := range box.Children() {
		eb, ok := child.(*ElementBox)
		if !ok || !eb.IsVisible() {
			continue
		}
		if eb.IsTableRow() {
			rows = append(rows, eb)
		} else if eb.IsTableSection() {
			for _, cc := range eb.Children() {
				if rowEb, ok := cc.(*ElementBox); ok && rowEb.IsVisible() && rowEb.IsTableRow() {
					rows = append(rows, rowEb)
				}
			}
		}
	}
	return rows
}

// collectCells returns all visible cell (table-cell) children of a row.
func collectCells(row *ElementBox) []*ElementBox {
	var cells []*ElementBox
	for _, child := range row.Children() {
		if cellEb, ok := child.(*ElementBox); ok && cellEb.IsVisible() && cellEb.IsTableCell() {
			cells = append(cells, cellEb)
		}
	}
	return cells
}

// parseColspan returns the column span for a cell from HTML attribute, default 1.
func parseColspan(cell *ElementBox) int {
	if cell.Element() == nil {
		return 1
	}
	s := cell.Element().GetAttribute("colspan")
	if s == "" {
		return 1
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return 1
}

// parseRowspan returns the row span for a cell from HTML attribute, default 1.
func parseRowspan(cell *ElementBox) int {
	if cell.Element() == nil {
		return 1
	}
	s := cell.Element().GetAttribute("rowspan")
	if s == "" {
		return 1
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return 1
}

var _ = style.DisplayTable

func (c *TableFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	defer profileLayout("table")()
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	cw := g.ContentWidth()
	if cw <= 0 {
		return
	}
	// border-collapse: collapse — cell boxes start at the table's border-box
	// edge (cell borders replace the table border), borders merge into shared
	// single lines, and explicit cell widths are content widths (border and
	// padding add to the column).
	collapse := cs.BorderCollapse == style.BorderCollapseCollapse
	// border-spacing（CSS 2.1 §17.6.1，仅 border-collapse:separate 生效）：
	// 水平间距同时出现在「表格内容边缘 ↔ 首个单元格」之间、相邻单元格之间
	// 以及「末个单元格 ↔ 内容边缘」之间；垂直方向同理。此前该属性完全没有
	// 实现——`border-spacing: 10px 0` 的表格首列起于内容盒左边缘（x=4 而非
	// 4+10=14），列宽也没扣掉 (列数+1) 个间距，第二列宽 232 而非 212
	// （fixed-table-layout 6 项失败）。
	spacingX, spacingY := 0.0, 0.0
	if !collapse {
		fs := fontSizeOf(box)
		spacingX = math.Max(0, resolveOrZero(cs.BorderSpacingH, cw, fs))
		spacingY = math.Max(0, resolveOrZero(cs.BorderSpacingV, cw, fs))
	}

	// ── Step 1: Collect all rows and cells ──
	rowBoxes := collectRows(box)
	if len(rowBoxes) == 0 {
		return
	}

	var rows []tableRow
	maxCols := 0
	for _, rb := range rowBoxes {
		rr := tableRow{box: rb, cells: collectCells(rb)}
		// 列数上界：一行内所有单元格的 colspan 之和（跨行单元格占据的列本来
		// 就属于它的起始列，不会再增加列数）。
		n := 0
		for _, cell := range rr.cells {
			n += parseColspan(cell)
		}
		if n > maxCols {
			maxCols = n
		}
		rows = append(rows, rr)
	}
	if maxCols < 1 {
		return
	}
	// 每个单元格的起始列（考虑 colspan 与上方跨行单元格的列占位）。列宽计算与
	// 单元格定位都必须以它为准：直接用 len(row.cells) 的下标会把第二行唯一的
	// 单元格算到第 1 列（table-row-geometry 的 .row-one/.row-two 期望 x=30，实测
	// x=60），也会让 colspan 之后的单元格落到不存在的列上（宽 0）。
	asn := columnAssignments(rows, maxCols)

	// ── Step 2: Compute column widths ──
	colWidths := make([]float64, maxCols)
	explicitCols := make([]bool, maxCols)

	// separate 模型下 (列数+1) 个水平 border-spacing 占掉表格宽度的一部分，
	// 列只能分配剩余空间（collapse 模型 spacingX=0，行为不变）。百分比列宽
	// 也以这个「轨道空间」为参考（table-track-geometry 的 50% 列在 400px 宽、
	// 2px 间距的表格里是 197px 而非 200px）。
	avail := cw - spacingX*float64(maxCols+1)
	if avail < 0 {
		avail = 0
	}

	// First pass: check cells in each column for explicit widths.
	for ri, row := range rows {
		for idx, cell := range row.cells {
			ci := asn[ri][idx]
			if ci < 0 || ci >= maxCols {
				continue
			}
			if explicitCols[ci] {
				continue
			}
			cs := cell.Style()
			if cs != nil && cs.Width.Unit == "px" && cs.Width.Value > 0 {
				// ★ 单元格的 width 是**内容宽**（CSS 2.1 §17.5.2.2），列宽是它的
				// border box（内容 + padding + border）——无论 border-collapse
				// 模式。此前只在 collapse 时加 padding/border，separate 表格
				// 的列比单元格窄、单元格内容/背景溢出列边界
				// （tables.html 的 "content-box fixed cell column"：td width:200 +
				// padding:4 → 列应 208，实测 200；table-track-geometry 的
				// "separate spacing reserves the outer table edges" 同理）。
				w := cs.Width.Value
				_, pad, border := computeBoxModelForBox(cell, 0, fontSizeOf(cell))
				w += pad.Horizontal() + border.Horizontal()
				colWidths[ci] = w
				explicitCols[ci] = true
			} else if cs != nil && cs.Width.Unit == "%" && cs.Width.Value > 0 && avail > 0 {
				colWidths[ci] = avail * cs.Width.Value / 100.0
				explicitCols[ci] = true
			}
			// Check HTML width attribute as fallback.
			if !explicitCols[ci] && cell.Element() != nil {
				attrW := cell.Element().GetAttribute("width")
				if attrW != "" {
					if w, err := strconv.ParseFloat(attrW, 64); err == nil && w > 0 {
						colWidths[ci] = w
						explicitCols[ci] = true
					}
				}
			}
		}
	}

	// Distribute the remaining width among non-explicit columns using the
	// table auto-layout heuristic: each column's preferred width is the
	// widest cell content (text advance + horizontal padding + border) in
	// that column; remaining space is then split in proportion to those
	// preferred widths (CSS2.1 §17.5.2.2). This matches Edge for both
	// single-char and multi-char cells.
	explicitTotal := 0.0
	autoCount := maxCols
	for ci := 0; ci < maxCols; ci++ {
		if explicitCols[ci] {
			explicitTotal += colWidths[ci]
			autoCount--
		}
	}
	remaining := avail - explicitTotal
	if remaining < 0 {
		remaining = 0
	}

	// Compute per-column preferred widths from content.
	prefCols := make([]float64, maxCols)
	for ri, row := range rows {
		for idx, cell := range row.cells {
			ci := asn[ri][idx]
			if ci < 0 || ci >= maxCols {
				continue
			}
			if explicitCols[ci] {
				continue
			}
			fs := fontSizeOf(cell)
			_, padding, border := computeBoxModelForBox(cell, remaining, fs)
			hp := padding.Left + padding.Right + border.Left + border.Right
			w := tableCellPreferredWidth(cell, fs) + hp
			if w > prefCols[ci] {
				prefCols[ci] = w
			}
		}
	}
	sumPref := 0.0
	for ci := 0; ci < maxCols; ci++ {
		if !explicitCols[ci] {
			sumPref += prefCols[ci]
		}
	}
	if autoCount > 0 && sumPref > 0 {
		for ci := 0; ci < maxCols; ci++ {
			if !explicitCols[ci] {
				colWidths[ci] = prefCols[ci] * remaining / sumPref
			}
		}
	} else if autoCount > 0 {
		autoWidth := remaining / float64(autoCount)
		for ci := 0; ci < maxCols; ci++ {
			if !explicitCols[ci] {
				colWidths[ci] = autoWidth
			}
		}
	}

	// ── Step 3: Layout rows and cells ──
	contentLeft := g.ContentBoxLeft()
	y := g.ContentBoxTop()
	if collapse {
		// Cells sit at the table border-box edge: their borders replace the
		// table's own outer border.
		contentLeft = g.Left()
		y = g.Top()
	} else {
		// separate 模型：单元格与表格内容边缘之间还有一个 border-spacing。
		contentLeft += spacingX
		y += spacingY
	}

	// ★ 表格行高需要**两遍**布局：rowspan 单元格的高度需求跨越若干行，必须
	// 先算出所有行的基础高度，才能把不足的部分分配给跨度内的行（CSS 2.1
	// §17.5.3）。第一遍收集行高与跨行需求；存在跨行需求时，用分配后的行高再
	// 跑一遍，使行 y、单元格高度与内容垂直对齐都基于最终行高。
	// 此前完全没有 rowspan 处理：跨行单元格的内容高度只抬高了它**声明所在**
	// 的那一行，第二行仍按自身内容算高（table-row-geometry 的 "rowspan
	// content sizes the complete span" 期望 30x80 @(0,180)，实测总高度不足、
	// 后续两行错位）。
	rowHeights, spans := c.layoutRowPass(rows, asn, state, colWidths, maxCols, cw,
		contentLeft, y, spacingX, spacingY, nil)
	if len(spans) > 0 {
		if adjusted := distributeRowspanHeights(rowHeights, spans, spacingY); adjusted != nil {
			rowHeights, _ = c.layoutRowPass(rows, asn, state, colWidths, maxCols, cw,
				contentLeft, y, spacingX, spacingY, adjusted)
		}
	}

	totalHeight := 0.0
	for _, h := range rowHeights {
		totalHeight += h + spacingY
	}
	if totalHeight < 0 {
		totalHeight = 0
	}
	g.SetContentHeight(math.Max(g.ContentHeight(), totalHeight))

	// ── Step 4: Size table sections (tbody/thead/tfoot). These grouping
	// elements wrap the rows and must cover their rows' extent; the flat
	// row loop above skips them, leaving their geometry at zero.
	for _, child := range box.Children() {
		eb, ok := child.(*ElementBox)
		if !ok || !eb.IsVisible() || !eb.IsTableSection() {
			continue
		}
		sg := state.GeometryForBox(eb)
		var top, bottom float64
		first := true
		for _, row := range rows {
			if row.box.Parent() != eb {
				continue
			}
			rg := state.GeometryForBox(row.box)
			if first {
				top = rg.Top()
				first = false
			}
			if rb := rg.Top() + rg.BorderBoxHeight(); rb > bottom {
				bottom = rb
			}
		}
		if first {
			continue
		}
		sg.SetTopLeft(top, contentLeft)
		sg.SetContentWidth(cw)
		sg.SetContentHeight(math.Max(1, bottom-top))
	}

	// ── Step 5: Border collapsing ──
	// border-collapse:collapse merges adjacent cell borders into a single
	// shared line. Conflicts resolve CSS-style: the cell that appears first
	// (top/left in DOM order) wins the color; the wider border wins the
	// width. The losing cell's edge is zeroed so it isn't painted twice.
	if collapse {
		applyCollapseBorders(box, rows, state)
	}
}

// tableRowSpan 记录一个跨行单元格（rowspan > 1）的高度需求。
type tableRowSpan struct {
	row    int     // 起始行在 rows 中的下标
	span   int     // 跨越的行数
	height float64 // 单元格 border-box 高度需求（已扣除跨距内部的 border-spacing）
}

// columnAssignments 计算每行各单元格的起始列索引：按 DOM 顺序为单元格分配列，跳过
// 被上方跨行单元格占位的列（CSS 2.1 §17.5.1），colspan 占据连续的若干列。返回的每行
// 与 row.cells 同序；无处可放（超出列数）时为 -1。
//
// 列宽计算与单元格定位都依赖这个映射，不能用 len(row.cells) 的下标代替：第二行唯一
// 的单元格属于第 2 列而非第 1 列（table-row-geometry 的 .row-one/.row-two），colspan
// 之后的单元格也不能落到不存在的列上。
func columnAssignments(rows []tableRow, maxCols int) [][]int {
	res := make([][]int, len(rows))
	// occupied[c] = 第 c 列还被上方跨行单元格占据的行数（含当前行）。行末统一递减，
	// 因此占位单元格在自身行写入完整的 rowspan 值。
	occupied := make([]int, maxCols)
	for ri, row := range rows {
		asn := make([]int, len(row.cells))
		for i := range asn {
			asn[i] = -1
		}
		ci := 0
		for idx, cell := range row.cells {
			for ci < maxCols && occupied[ci] > 0 {
				ci++
			}
			if ci >= maxCols {
				break
			}
			asn[idx] = ci
			colspan := parseColspan(cell)
			if colspan < 1 {
				colspan = 1
			}
			rowspan := parseRowspan(cell)
			if rowspan > len(rows)-ri {
				rowspan = len(rows) - ri
			}
			if rowspan > 1 {
				for c := ci; c < ci+colspan && c < maxCols; c++ {
					if occupied[c] < rowspan {
						occupied[c] = rowspan
					}
				}
			}
			ci += colspan
		}
		res[ri] = asn
		for c := 0; c < maxCols; c++ {
			if occupied[c] > 0 {
				occupied[c]--
			}
		}
	}
	return res
}

// layoutRowPass 执行一遍「行与单元格」布局：按行放置单元格、计算几何、汇总行
// 高、做内容的垂直对齐，并写出行盒几何。
//
// minHeights 为行高下限：nil = 第一遍（行高由内容与 tr 的 height 决定，并收集
// 跨行需求）；非 nil = 第二遍（用 distributeRowspanHeights 分配后的行高），使跨
// 行单元格的高度与内容垂直对齐都落在最终行高上。返回每行最终高度与（仅第一遍）
// 收集到的跨行需求。
//
// 列位置由 columnAssignments 预分配的 asn 给出（跨行单元格占据的列在后续行不再
// 排队，CSS 2.1 §17.5.1）——否则 table-row-geometry 的第二行单元格会落到第 1 列
// （.row-two 期望 x=30，实测 x=0）。
func (c *TableFormattingContext) layoutRowPass(rows []tableRow, asn [][]int, state *LayoutState,
	colWidths []float64, maxCols int, cw float64, contentLeft, startY float64,
	spacingX, spacingY float64, minHeights []float64) ([]float64, []tableRowSpan) {

	secondPass := minHeights != nil
	rowHeights := make([]float64, len(rows))
	for i, row := range rows {
		if secondPass && i < len(minHeights) && minHeights[i] > rowHeights[i] {
			rowHeights[i] = minHeights[i]
		}
		// 行高下限：tr 的 height 是最小行高（CSS 2.1 §17.5.3），内容更高时由内容
		// 撑高。table-row-geometry 的 "row height remains a minimum above short
		// content"：tr{height:40px} 内只有 20px 高的内容，行高与单元格高都应是 40。
		if hs := row.box.Style(); hs != nil && hs.Height.Unit == "px" && hs.Height.Value > rowHeights[i] {
			rowHeights[i] = hs.Height.Value
		}
	}

	var spans []tableRowSpan

	y := startY
	for ri, row := range rows {
		rg := state.GeometryForBox(row.box)
		rowH := rowHeights[ri]

		for cellIdx, cell := range row.cells {
			ci := asn[ri][cellIdx]
			if ci < 0 || ci >= maxCols {
				continue
			}

			colspan := parseColspan(cell)
			if colspan < 1 {
				colspan = 1
			}
			rowspan := parseRowspan(cell)
			if rowspan < 1 {
				rowspan = 1
			}
			if rowspan > len(rows)-ri {
				rowspan = len(rows) - ri
			}

			// 单元格左边缘由列索引推导（跨行占位会跳过若干列）。
			cellX := contentLeft
			for c := 0; c < ci; c++ {
				cellX += colWidths[c] + spacingX
			}

			// Compute total width for this cell (sum of column widths it spans).
			cellTotalWidth := 0.0
			for s := 0; s < colspan && ci+s < maxCols; s++ {
				cellTotalWidth += colWidths[ci+s]
			}
			// 跨列单元格还要占满它跨越的列之间的水平间距（separate 模型）。
			if colspan > 1 {
				cellTotalWidth += spacingX * float64(colspan-1)
			}

			// Compute box model for the cell.
			fs := fontSizeOf(cell)
			cg := state.GeometryForBox(cell)
			margin, padding, border := computeBoxModelForBox(cell, cellTotalWidth, fs)
			cg.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
			cg.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
			cg.SetBorder(border.Top, border.Right, border.Bottom, border.Left)

			// Set cell's border-box position.
			cg.SetTopLeft(y+margin.Top, cellX+margin.Left)

			// Compute and set content width.
			borderBoxW := cellTotalWidth - margin.Horizontal()
			if borderBoxW < 0 {
				borderBoxW = 0
			}
			contentW := borderBoxW - border.Horizontal() - padding.Horizontal()
			if contentW < 0 {
				contentW = 0
			}
			cg.SetContentWidth(contentW)

			// Layout cell content.
			cellCtx := contextFor(cell, state)
			if cellCtx != nil {
				cellCtx.Layout(cell, state)
			}

			// ★ 单元格的 height 是**最小高度**（CSS 2.1 §17.5.1：单元格高度 =
			// max(指定 height, 内容高度)），内容更高时由内容撑高。布局容器对带显式
			// height 的块不会按内容增长（单元格的 contentHeight 停在 style height），
			// 因此这里按子盒的实际延伸补足：#text-over-height 的
			// td{height:10px; line-height:15px} 里 15px 的行盒应把单元格撑到 15px
			// （此前靠渲染层扩展 frame 掩盖，布局几何本身是错的）。
			if cs := cell.Style(); cs != nil && cs.Height.Unit == "px" && cs.Height.Value > 0 {
				if ext := cellContentExtent(cell, state); ext > cg.ContentHeight() {
					cg.SetContentHeight(ext)
				}
				if floor := cs.Height.Value; cg.ContentHeight() < floor {
					cg.SetContentHeight(floor)
				}
			}

			cellH := cg.BorderBoxHeight()
			if rowspan > 1 {
				// 跨行单元格的高度需求不属于某一行：交给 distributeRowspanHeights
				// 在跨距内分配。否则它的内容高度只抬高起始行，第二行仍按自身内容
				// 算高（table-row-geometry 的 "rowspan content sizes the complete
				// span" 期望 30x80 @(0,180)）。
				if !secondPass {
					need := cellH - spacingY*float64(rowspan-1)
					if need < 0 {
						need = 0
					}
					spans = append(spans, tableRowSpan{row: ri, span: rowspan, height: need})
				}
			} else if cellH > rowH {
				rowH = cellH
			}
		}

		// Equalize all cells in this row to the computed row height.
		for _, cell := range row.cells {
			cg2 := state.GeometryForBox(cell)
			// 跨行单元格的高度是它跨越的所有行之高加上内部间距，不是单行行高。
			rowspan := parseRowspan(cell)
			if rowspan < 1 {
				rowspan = 1
			}
			if rowspan > len(rows)-ri {
				rowspan = len(rows) - ri
			}
			extent := rowH
			for k := 1; k < rowspan; k++ {
				extent += spacingY + rowHeights[ri+k]
			}
			cellContentH := extent - cg2.VerticalBorderAndPadding()
			if cellContentH < 1 {
				cellContentH = 1
			}
			// ★ 单元格内容的垂直对齐（CSS 2.1 §17.5.1）：vertical-align 决定内容
			// 在行高内的位置——middle 居中、bottom 贴底、top/baseline 贴顶。此前
			// 完全没有实现，所有单元格内容一律贴顶；而 UA 样式表让
			// thead/tbody/tfoot 为 middle、td/th 继承之，因此「无显式声明的单元格
			// 内容居中」是浏览器默认行为（table-track-geometry 的 "row-group
			// default vertically centers cell content" 期望 60px 行内的 20px 方块
			// 位于 y=260，实测贴顶 240）。
			vaRaw := "baseline"
			if cs := cell.Style(); cs != nil && cs.VerticalAlign != "" {
				vaRaw = cs.VerticalAlign
			}
			if va := strings.ToLower(strings.TrimSpace(vaRaw)); va == "middle" || va == "bottom" {
				if contentH := cellContentExtent(cell, state); contentH > 0 && contentH < cellContentH {
					delta := (cellContentH - contentH) / 2
					if va == "bottom" {
						delta = cellContentH - contentH
					}
					if delta > 0 {
						shiftCellContent(cell, delta, state)
					}
				}
			}
			cg2.SetContentHeight(cellContentH)
		}

		// Set row geometry.
		rg.SetTopLeft(y, contentLeft)
		rg.SetContentWidth(cw)
		if rowH < 1 {
			rowH = 1
		}
		rg.SetContentHeight(rowH)
		rowHeights[ri] = rowH

		y += rowH + spacingY
	}

	return rowHeights, spans
}

// distributeRowspanHeights 把跨行单元格的高度需求分配到它跨越的各行上，返回调整
// 后的行高；需求已被满足时返回 nil（调用方跳过第二遍布局）。
//
// CSS 2.1 §17.5.3 只要求「跨行单元格所占高度之和不小于它的高度需求」，没有规定
// 分配方式；Blink/WebKit 把不足的部分在跨距内的各行之间**均分**。
// table-row-geometry 的 rowspan 夹具即此语义：.spanning 内 80px 的内容，两行基础
// 行高各 20px（各自非跨行内容高），差额 40px 平分后两行各 40px——.row-one 期望
// y=190（40px 行内居中）、.row-two 期望 y=230（第二行 40px 行内居中）。
func distributeRowspanHeights(rowHeights []float64, spans []tableRowSpan, spacingY float64) []float64 {
	adjusted := make([]float64, len(rowHeights))
	copy(adjusted, rowHeights)
	changed := false
	// 多个跨行单元格可能共享行，一次分配改变行高后又会改变其它 span 的缺口，因此
	// 迭代到不再有缺口为止（上界 = 跨度数 + 1）。
	for iter := 0; iter <= len(spans); iter++ {
		progress := false
		for _, sp := range spans {
			rows := sp.span
			if sp.row+rows > len(adjusted) {
				rows = len(adjusted) - sp.row
			}
			if rows <= 0 {
				continue
			}
			have := 0.0
			for k := 0; k < rows; k++ {
				if k > 0 {
					have += spacingY
				}
				have += adjusted[sp.row+k]
			}
			deficit := sp.height - have
			if deficit <= 0 {
				continue
			}
			each := deficit / float64(rows)
			for k := 0; k < rows; k++ {
				adjusted[sp.row+k] += each
			}
			progress = true
			changed = true
		}
		if !progress {
			break
		}
	}
	if !changed {
		return nil
	}
	return adjusted
}

// cellContentExtent 返回单元格内容在垂直方向的占用高度：内容区顶部到最靠
// 下的子盒底边。无可见子盒时返回 0（此时不需要垂直对齐）。
func cellContentExtent(cell *ElementBox, state *LayoutState) float64 {
	cg := state.GeometryForBox(cell)
	top := cg.ContentBoxTop()
	bottom := top
	for _, ch := range cell.Children() {
		cb, ok := ch.(*ElementBox)
		if !ok || !cb.IsVisible() {
			continue
		}
		g := state.GeometryForBox(cb)
		if b := g.Top() + g.BorderBoxHeight(); b > bottom {
			bottom = b
		}
	}
	return bottom - top
}

// shiftCellContent 把单元格的**内容**（子盒与匿名文本）整体下移 dy，
// 单元格自身几何不变——用于 vertical-align:middle/bottom。
func shiftCellContent(cell *ElementBox, dy float64, state *LayoutState) {
	for _, ch := range cell.Children() {
		switch c := ch.(type) {
		case *ElementBox:
			shiftBoxAndDescendants(c, dy, 0, state)
		case *InlineTextBox:
			for i := range c.TextSegments {
				c.TextSegments[i].Y += dy
			}
		}
	}
}

// tableRow groups a row box with its collected cells.
type tableRow struct {
	box   *ElementBox
	cells []*ElementBox
}

// borderWidthOf returns the resolved pixel width of a border side.
func borderWidthOf(cs *style.ComputedStyle, side string) float64 {
	if cs == nil {
		return 0
	}
	var l style.Length
	switch side {
	case "top":
		l = cs.BorderTopWidth
	case "right":
		l = cs.BorderRightWidth
	case "bottom":
		l = cs.BorderBottomWidth
	default:
		l = cs.BorderLeftWidth
	}
	return resolveLength(l, 0, 0).Value
}

// setBorderWidth overwrites a border side width on the cell's computed style.
// The painter reads border widths from the style, so this directly controls
// what gets painted (geometry is already laid out).
func setBorderWidth(box *ElementBox, side string, w float64) {
	cs := box.Style()
	if cs == nil {
		return
	}
	l := style.Length{Value: w, Unit: "px"}
	switch side {
	case "top":
		cs.BorderTopWidth = l
	case "right":
		cs.BorderRightWidth = l
	case "bottom":
		cs.BorderBottomWidth = l
	default:
		cs.BorderLeftWidth = l
	}
}

// applyCollapseBorders resolves border conflicts for a border-collapse:collapse
// table (CSS 2.1 §17.6.2.1, simplified to the dominant cases):
//   - a cell's border always beats the table's own outer border (same position;
//     the cell is painted later and covers it)
//   - adjacent cells share the line between them: the first cell (top/left in
//     DOM order) provides the color, the wider border provides the width
//   - the losing edge is zeroed so it paints once
func applyCollapseBorders(table *ElementBox, rows []tableRow, state *LayoutState) {
	ts := table.Style()
	if ts == nil {
		return
	}
	tableW := map[string]float64{
		"top":    borderWidthOf(ts, "top"),
		"right":  borderWidthOf(ts, "right"),
		"bottom": borderWidthOf(ts, "bottom"),
		"left":   borderWidthOf(ts, "left"),
	}
	maxWidth := func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	}
	for ri, row := range rows {
		lastRow := ri == len(rows)-1
		for ci, cell := range row.cells {
			cs := cell.Style()
			if cs == nil {
				continue
			}
			// Top edge: first row conflicts with the table's top border
			// (cell wins → keep width max); inner rows lose to the cell
			// above (which paints the shared line).
			if ri == 0 {
				setBorderWidth(cell, "top", maxWidth(borderWidthOf(cs, "top"), tableW["top"]))
			} else {
				setBorderWidth(cell, "top", 0)
			}
			// Left edge: first column conflicts with the table's left border
			// (cell wins); inner columns lose to the left neighbor.
			if ci == 0 {
				setBorderWidth(cell, "left", maxWidth(borderWidthOf(cs, "left"), tableW["left"]))
			} else {
				setBorderWidth(cell, "left", 0)
			}
			// Right edge: this cell paints the shared line with the right
			// neighbor — width = max of both, color = ours (we come first).
			if ci < len(row.cells)-1 {
				next := row.cells[ci+1]
				ns := next.Style()
				w := maxWidth(borderWidthOf(cs, "right"), borderWidthOf(ns, "left"))
				setBorderWidth(cell, "right", w)
			} else {
				setBorderWidth(cell, "right", maxWidth(borderWidthOf(cs, "right"), tableW["right"]))
			}
			// Bottom edge: this cell paints the shared line with the row
			// below — width = max, color = ours. Last row conflicts with the
			// table's bottom border (cell wins).
			if !lastRow {
				var below *style.ComputedStyle
				if ri+1 < len(rows) && ci < len(rows[ri+1].cells) {
					below = rows[ri+1].cells[ci].Style()
				}
				w := borderWidthOf(cs, "bottom")
				if below != nil {
					w = maxWidth(w, borderWidthOf(below, "top"))
				}
				setBorderWidth(cell, "bottom", w)
			} else {
				setBorderWidth(cell, "bottom", maxWidth(borderWidthOf(cs, "bottom"), tableW["bottom"]))
			}
		}
	}
}

// tablePreferredWidth returns the max-content width of a table: the sum of
// each column's widest cell (text advance + padding + border). Used for
// shrink-to-fit auto width (CSS 2.1 §17.5.2.1).
func tablePreferredWidth(table *ElementBox) float64 {
	rowBoxes := collectRows(table)
	rows := make([]tableRow, 0, len(rowBoxes))
	maxCols := 0
	for _, rb := range rowBoxes {
		cells := collectCells(rb)
		n := 0
		for _, cell := range cells {
			n += parseColspan(cell)
		}
		if n > maxCols {
			maxCols = n
		}
		rows = append(rows, tableRow{box: rb, cells: cells})
	}
	var colPrefs []float64
	// 列归属同样要用 columnAssignments：第二行唯一的单元格属于第 2 列，按
	// row.cells 下标算会把它并入第 1 列（table-row-geometry 的 #rowspan 表格
	// 偏好宽 120 而非 90，列宽 40/80 而非 30/60）。
	var asn [][]int
	if maxCols > 0 {
		asn = columnAssignments(rows, maxCols)
	}
	for ri, row := range rows {
		for idx, cell := range row.cells {
			ci := -1
			if asn != nil {
				ci = asn[ri][idx]
			}
			if ci < 0 {
				continue
			}
			fs := fontSizeOf(cell)
			_, padding, border := computeBoxModelForBox(cell, 0, fs)
			w := tableCellPreferredWidth(cell, fs) + padding.Horizontal() + border.Horizontal()
			// Explicit cell width wins over content-derived width.
			if cs := cell.Style(); cs != nil && cs.Width.Unit == "px" && cs.Width.Value > 0 {
				w = cs.Width.Value + padding.Horizontal() + border.Horizontal()
			}
			for len(colPrefs) <= ci {
				colPrefs = append(colPrefs, 0)
			}
			if w > colPrefs[ci] {
				colPrefs[ci] = w
			}
		}
	}
	total := 0.0
	for _, w := range colPrefs {
		total += w
	}
	// separate 模型：表格宽度还要容纳 (列数+1) 个水平间距。
	if cs := table.Style(); cs != nil && cs.BorderCollapse == style.BorderCollapseSeparate {
		fs := fontSizeOf(table)
		sp := math.Max(0, resolveOrZero(cs.BorderSpacingH, 0, fs))
		total += sp * float64(len(colPrefs)+1)
	}
	return total
}

// explicitBorderBoxWidth 返回元素由 CSS `width` 指定的 border box 宽（含
// padding/border/margin）；没有显式 px 宽度时返回 0（调用方退回内容推导）。
func explicitBorderBoxWidth(box *ElementBox) float64 {
	cs := box.Style()
	if cs == nil || cs.Width.Unit != "px" || cs.Width.Value <= 0 {
		return 0
	}
	w := cs.Width.Value
	if !isBorderBoxForBox(box) {
		_, pad, border := computeBoxModelForBox(box, 0, fontSizeOf(box))
		w += pad.Horizontal() + border.Horizontal()
	}
	margin, _, _ := computeBoxModelForBox(box, 0, fontSizeOf(box))
	return w + margin.Horizontal()
}

// tableCellPreferredWidth returns the preferred (max-content) width of a
// table cell: the advance width of its text content, or the widest child
// element for non-text content.
func tableCellPreferredWidth(cell *ElementBox, fs float64) float64 {
	total := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				total += measureText(b, itb.Text())
			} else if eb, ok := c.(*ElementBox); ok {
				// Inline-level children (span/em) contribute their width;
				// block children reset the total (each starts a new line).
				// ★ 子元素的显式 width 优先于内容推导：`<td><div style="width:10px">`
				// 的 preferred width 是 10px（+ 其 padding/border/margin），
				// 而不是「无文本 → fs*0.5」的兜底值——否则空 div 单元格的
				// 自动列宽只有 6.5px，内容溢出单元格
				// （table-track-geometry 的 "UA cell padding is one pixel on every
				// edge"：期望 td 12x12，实测 8x12）。
				childW := explicitBorderBoxWidth(eb)
				if childW <= 0 {
					childW = tableCellPreferredWidth(eb, fontSizeOf(eb))
				}
				if eb.IsInlineLevel() {
					total += childW
				} else if childW > total {
					total = childW
				}
			}
		}
	}
	walk(cell)
	if total <= 0 {
		total = fs * 0.5
	}
	return total
}

// ── helper methods for ElementBox ──

func (b *ElementBox) IsTableRow() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableRow
}

func (b *ElementBox) IsTableSection() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableRowGroup ||
		b.style.Display == style.DisplayTableHeaderGroup ||
		b.style.Display == style.DisplayTableFooterGroup
}

func (b *ElementBox) IsTableCell() bool {
	if b.style == nil {
		return false
	}
	return b.style.Display == style.DisplayTableCell
}
