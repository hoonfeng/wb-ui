// Translation of: Source/WebCore/layout/formattingContexts/inline/InlineFormattingContext.cpp
package layout

import (
	"fmt"
	"math"
	"os"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/debugenv"
	"wb-ui/engine/style"
)

// isFlexItem reports whether box is an in-flow child of a flex container
// (its main size is decided by the flex algorithm, not by its own content).
func isFlexItem(box *ElementBox) bool {
	if box == nil || box.Parent() == nil {
		return false
	}
	p := box.Parent()
	if p.Style() == nil {
		return false
	}
	disp := p.Style().Display
	if disp != style.DisplayFlex && disp != style.DisplayInlineFlex {
		return false
	}
	return box.IsInFlow() && !box.IsAbsolutelyPositioned()
}

// flexOverflowVisible reports whether both overflow axes are visible. Flex
// items with non-visible overflow have an automatic minimum size of 0
// (CSS-FLEXBOX §4.5) — the flex-resolved main size must not be inflated by
// content height.
func flexOverflowVisible(cs *style.ComputedStyle) bool {
	if cs == nil {
		return true
	}
	ox, oy := cs.OverflowX, cs.OverflowY
	return (ox == style.OverflowVisible || ox == 0) && (oy == style.OverflowVisible || oy == 0)
}

type InlineFormattingContext struct {
	FormattingContextBase
}

// pendingSeg holds a segment before its X position is finalized.
type pendingSeg struct {
	textBox *InlineTextBox
	seg     TextSegment
	lineIdx int
}

// insideFlexItem reports whether box is a flex item or a descendant of one.
// The flex algorithm decides the main size of the whole flex item subtree;
// auto-width expansion must not widen inner inline boxes (e.g. the anonymous
// wrapper holding a blockified span's text) back to their raw text extent —
// otherwise a shrunken flex item's text never line-breaks (CJK "完成摘要"
// stays one 41px line in a 27px title instead of wrapping 2+2 like Edge).
func insideFlexItem(box *ElementBox) bool {
	for b := box; b != nil; b = b.Parent() {
		if isFlexItem(b) {
			return true
		}
	}
	return false
}

// toastMeasurePrinted 限制 [toast-measure] 诊断行数（WB_TOAST_IFC=1 时生效）。
var toastMeasurePrinted int

// toastWantLineDetail 置位后，下一次 availableLineWidth 结果会随 [toast-line] 输出。
var toastWantLineDetail bool

// toastLinesPrinted 限制 [toast-lines] 输出次数。
var toastLinesPrinted int

func (c *InlineFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	defer profileLayout("ifc")()
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)
	isToastBox := false // 本次调用是否为目标文本所在盒（入口诊断置位，收尾使用）

	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	boxHeight := g.ContentHeight()
	contentWidth := g.ContentWidth()
	// ★ 诊断（WB_TOAST_IFC=1）：IFC 实际接收的内容宽度与首段文本。
	// 用于定位「flex column item 的文本逐字换行（竖排）」：判定 IFC 收到的
	// 可用宽度是修正后的 250.8，还是仍是测量阶段的 0。
	if debugenv.Enabled("WB_TOAST_IFC") {
		name, class, txt := "<anon>", "", ""
		if el := box.Element(); el != nil {
			name, class = el.NodeName(), el.GetAttribute("class")
		}
		for _, ch := range box.Children() {
			if t, ok := ch.(*InlineTextBox); ok {
				txt += t.Text()
				if len(txt) > 30 {
					break
				}
			}
		}
		fmt.Printf("[toast-ifc] %s.%s cw=%.2f ch=%.2f kids=%d text=%q\n", name, class, contentWidth, boxHeight, len(box.Children()), txt)
		// ★ 字节级测量实证：单字宽 / 整段宽 / 字体族。若单字宽异常大（≈availWidth），
		// 则「每字一行」源于字体度量而非换行逻辑。
		if rr := []rune(txt); len(rr) >= 4 && string(rr[:4]) == "已选择工" && toastMeasurePrinted < 10 {
			toastMeasurePrinted++
			fmt.Printf("[toast-measure] set=%d fs=%.1f fam=%q whole=%.3f one=%q:%.3f two=%q:%.3f cw=%.2f\n",
				toastMeasurePrinted, fontSizeOf(box), fontFamilyOf(box), measureText(box, txt),
				string(rr[0]), measureText(box, string(rr[0])),
				string(rr[:2]), measureText(box, string(rr[:2])),
				contentWidth)
		}
		// 标记：下一次 availableLineWidth 求值（首行行宽）需要详报。
		toastWantLineDetail = true
		isToastBox = true
	}
	if debugenv.Enabled("WB_LAYOUT_DEBUG") && box.Element() != nil && box.Element().NodeName() == "DIV" && box.Element().GetAttribute("class") == "dialog-box" {
		fmt.Printf("[ifc] dialog-box: contentWidth=%.1f padL=%.1f parent=%v\n", contentWidth, g.PaddingLeft(), box.Parent())
	}
	// Reference width for text-align: the container's REAL content-box width,
	// captured BEFORE the auto-width expansion below. Using the widened
	// (totalW+20) width as the centering reference shifts centered text
	// right by half the expansion (span "暂无工作区" sat 10px right of its
	// flex-item box, and "创建" inside the button was off-center by the same).
	containerWidth := contentWidth
	fs := fontSizeOf(box)
	lineHeight := fontLineGap(box)
	if lineHeight <= 0 {
		lineHeight = fs * 1.2
	}
	// ★ quirks 模式的 strut 抑制（实测行为，非规范推导）。
	//   实测（本机「参照浏览器」Chrome headless，cssoracle 与 cssprobe 共用）：
	//     quirks   <form><input size=17></form> → form 高 21.2（= input 边框盒高）
	//     standards 同一结构                      → form 高 25.8（= strut descent 4.6
	//                                               + input 21.2，即 strut 参与）
	//   即 quirks 文档中，当行的内容**只有 atomic inline 子盒、没有任何文本**时，
	//   strut 的 ascent 与 descent 都不进行盒；行盒高 = 子盒的 margin box 高。
	//   cssprobe form-control-quirks 的 `.form-marker` 因此停在 y=40（= wbui 的
	//   strut 24 + form 的 1em 下边距 16）而浏览器是 37（= input 边框盒 21 + 16）。
	// ★ 该判定同时决定 strut 是否进入 maxBaseline/maxDescent（下文 strutAscent/
	//   strutDescent），故保存为变量复用（纯判定、无副作用）。
	quirksNoStrut := quirksStrutSuppressed(box)
	if quirksNoStrut {
		lineHeight = 0
	}
	// Use CSS line-height if explicitly set (overrides font metrics).
	// ★ 用 ok（而非 `> 0`）判断「显式设置」：`line-height: 0` 是有效值
	//   （零行高），而 normal/未声明用 Unit="normal" 表示 → 回退字体度量。
	cssLH, cssLHSet := cssLineHeight(box)
	if cssLHSet {
		lineHeight = cssLH
	}
	// baseLineHeight 是本 IFC 的【基准行高】（字体度量或 CSS line-height），
	// 用来初始化每一行自己的行盒高度。lineHeight 这个变量历史上被当作
	// 「跨行累积的最大值」使用（本行出现更高的 inline 子盒、或基线对齐抬高
	// 行盒时一起变大），于是后续行的推进与容器总高都跟着膨胀——form 容器因此
	// 比 Edge 高 28px（88 vs 60，底部整片死空间）。现在每行高度由 lineInfo.lineH
	// 独立记录，lineHeight 只作新行的基准初值。
	baseLineHeight := lineHeight
	// 行高诊断（WBUI_IFC_DEBUG=1）：字体度量与 CSS line-height 的最终取值，
	// 用于定位"行高与真实浏览器不一致"的夹具（font-metric-line-height）。
	if debugenv.Enabled("WBUI_IFC_DEBUG") {
		name, class := "<anon>", ""
		if el := box.Element(); el != nil {
			name, class = el.NodeName(), el.GetAttribute("class")
		}
		fmt.Fprintf(os.Stderr, "[ifc] <%s class=%q> fs=%.4f fontLineGap=%.4f cssLH=%.4f set=%v lineHeight=%.4f family=%q\n",
			name, class, fs, fontLineGap(box), cssLH, cssLHSet, lineHeight, fontFamilyOf(box))
	}
	// Text segment height should be the actual font metrics height, not CSS
	// line-height. The line-height determines line spacing and centering.
	textHeight := fontLineGap(box)
	if textHeight <= 0 {
		textHeight = fs * 1.2
	}
	// Compute the vertical centering offset: when line-height > font metrics,
	// shift text down so it appears vertically centered within the line.
	// ★ 纯 inline 子元素（display:inline 的 span 等）：父 IFC 已将其 box 顶
	// 放在 currentLine.y+centeringOffset（本文件 inline child 的 SetTopLeft），
	// 若此处再叠加 centeringOffset 会双重居中——CM6 语法高亮 token span
	// 因此比同行裸文本（括号/花括号等无样式字符）低 3px，视觉上表现为
	// 「括号与文字底部对齐」（根因）。纯 inline 文本应直接顶贴 box 内容顶
	// （与父文本共享基线），故 centeringOffset = 0。
	// ★ flex item 例外：flex 容器（align-items:center 等）的 inline 子元素是
	// block 化 item——其 box 顶由 flex 布局定位，内部文本行盒必须自己按
	// half-leading 居中（「礼物栏图标与 +N分钟 文字不垂直对齐」根因：
	// b.gd 在 inline-flex 的 span.gitem 内）。flex item 的匿名内容层
	// （父是 flex item 的 (anon) 盒，box 顶无父 IFC 行盒 offset）同样需要。
	// ★ 半行距可为负：line-height < 字体度量高时文字上移（top 溢出行盒，
	// 浏览器语义）——textHeight < cssLH 的旧限制让负半行距静默归零，
	// 文字中心比行盒中心低 (textHeight-cssLH)/2（gift 卡片实测 1.26px）。
	centeringOffset := 0.0
	parentIsFlexItem := box.Parent() != nil && isFlexItem(box.Parent())
	isPlainInline := cs != nil && box.IsInline() && !box.IsReplaced() &&
		cs.Display == style.DisplayInline && !isFlexItem(box) &&
		!(box.Element() == nil && parentIsFlexItem)
	if !isPlainInline && cssLHSet {
		centeringOffset = (cssLH - textHeight) / 2
	}
	// 本行文本段的字体 ascent：基线对齐时文本段的 Y = 行盒基线 - ascent
	// （见下面的 lineTextOffset / 表单控件基线定位）。
	textAscent, textDescent := fontAscentDescent(box)
	// ★ 字体度量**各自四舍五入到整像素**后再参与行盒计算（与 fontLineGap 同一
	//   口径）：浏览器把 ascent/descent/lineGap 分别取整后才相加，行盒基线也
	//   用取整后的 ascent —— Edge 实测 fs16 → 19/5、fs32 → 37/9，正是
	//   round(1.15625·fs) / round(0.293·fs)（h2_baseline_matrix 的 46 例逐一
	//   吻合）。用未取整值会让 strut 基线差 0.094px（h2_baseline_formula：
	//   relTop 3.906 vs Edge 4.000）。
	textAscent, textDescent = math.Round(textAscent), math.Round(textDescent)
	// halfLeading：行盒高与字体度量高之差的一半（字体在行盒内的上下留白）。
	// 行盒高取 CSS line-height（已解析为 px）或字体度量行高（line-height:normal
	// 与未声明时）。cssLineHeight 对 normal 返回 0（见其文档），故此处按
	// fontLineGap 兜底 —— 否则 line-height:normal 的元素 halfLeading 被算成
	// 「整行高的一半」，替换元素的基线对齐全错。
	// 用途：inline-block/replaced 的基线对齐（CSS 2.1 §10.8）——
	//   行盒基线（相对行盒顶）= halfLeading + ascent
	//   子盒 border-box 顶 = 行盒顶 + halfLeading + ascent - 子盒高
	halfLeading := 0.0
	{
		lineBoxH := textHeight
		if cssLHSet {
			lineBoxH = cssLH
		}
		// ★ 半行距**可为负**（CSS 2.1 §10.8.1）：line-height 小于字体度量行高
		//   时文字上下都溢出行盒。此前 `if lineBoxH > asc+desc` 把负半行距
		//   静默归零 —— `line-height:20px`（< 字体行高 23）的 strut ascent 被
		//   算成 18.5 而非 17，其中的控件/替换元素整体偏高 2px。Edge 实测
		//   （h2_baseline_matrix）：lh20px 的 input relTop=2、lh40px relTop=12
		//   （= 8.5 半行距 + 18.5 ascent − 15 控件基线），只有负半行距能复现。
		halfLeading = (lineBoxH - (textAscent + textDescent)) / 2
	}
	// strut（本 IFC 容器自身字体）在行盒里的 ascent/descent：行盒顶→基线 /
	// 基线→行盒底。CSS 2.1 §10.8：行盒高 = max(各 inline 盒 ascent) +
	// max(descent)，**strut 本身也是其中一个参与项**。
	// 此前 maxBaseline/maxDescent 只由控件/替换元素贡献、strut 完全不参与 →
	// 控件永远贴行盒顶（h2_baseline_matrix 实测 relTop 恒 0，Edge 为
	// 4/22/2/12；行盒底也少了 strut descent，ib_30x30 行盒 30 而 Edge 35）。
	strutAscent := halfLeading + textAscent
	strutDescent := halfLeading + textDescent
	// ★ quirks 下（本行只有 atomic inline 子盒、无文本）strut 不参与行盒：
	//   ascent/descent 一并归零，否则子盒会被抬到「strut 基线」上——
	//   form-control-quirks 的 .form-marker 又停回 y=41（浏览器 37）。
	if quirksNoStrut {
		strutAscent, strutDescent = 0, 0
	}

	// When contentWidth is auto (derived from intrinsic text width), widen it
	// slightly to prevent floating-point discrepancies from triggering unwanted
	// line wraps inside flex items.
	//
	// Only auto-width boxes get this widening: boxes with an explicit CSS width
	// (or width:auto) should respect their declared width so that overflow
	// clipping (overflow:hidden, text-overflow:ellipsis) works correctly.
	{
		hasExplicitWidth := cs != nil && cs.Width.Unit != "" && cs.Width.Unit != "auto"
		if !hasExplicitWidth && !insideFlexItem(box) {
			// Auto-width expansion: only expand for auto-width inline-level
			// boxes (e.g. span, inline-block). Block-level children get their
			// width from the parent BFC and must not be expanded, otherwise
			// text would not wrap (causing overflow beyond the container).
			// Flex items are excluded too: their main size is decided by the
			// flex algorithm (base ± grow/shrink, e.g. .tl-tc-param flex:1
			// gets half the header width). Widening a flex item back to its
			// raw text extent here makes a long tool-call param/summary
			// overflow its shrunk box — the "text runs past the shrunken
			// rectangle" report. The final width re-clamp below already
			// skips flex items (isFlexItem check at L~560).
			if box.IsInlineLevel() {
				allText := ""
				for _, child := range box.Children() {
					if tb, ok := child.(*InlineTextBox); ok {
						allText += tb.Text()
					}
				}
				if totalW := measureText(box, allText); totalW > 0 {
					if contentWidth < totalW+20 {
						// This box has auto-width based on text content.
						// Widen slightly to prevent float-epsilon line wraps.
						contentWidth = totalW + 20
					}
				}
			}
		}
	}

	// Determine text-align.
	// CSS 2.1 §16.2: text-align applies to block containers only. A pure
	// inline element (display:inline, non-replaced, not a blockified flex
	// item) is not a block container — its text alignment is decided by the
	// nearest block container ancestor, which shifts the whole inline run
	// (including this box's text) via the line's text-align adjustment.
	// Applying text-align inside the inline box here double-shifts the text
	// right: the anonymous inline wrapper (inheriting an inline parent's
	// text-align:center) centered its text against the forced parent-line
	// width, then the outer IFC centered the wrapper again → net ~+24px
	// right shift ("flex:1 span 内 text-align:center 文字偏右" — .xseg/.rtab
	// 选项文字不在胶囊正中).
	textAlign := style.TextAlignStart
	if cs != nil {
		isPlainInline := cs.Display == style.DisplayInline && !box.IsReplaced()
		if !isPlainInline || isFlexItem(box) {
			// legacy 值（-webkit-center 等）在行内内容上等价于对应标准值，
			// 其"块级子盒居中"的额外语义由 BFC 处理（见 InlineEquivalent）。
			textAlign = cs.TextAlign.InlineEquivalent()
		}
	}
	// (containerWidth was captured before the auto-width expansion above.)

	// Get float context for text wrapping around floats.
	fc := state.currentFloatContext()

	// availableLineWidth returns the usable width for a line at the given page Y.
	// When floats intrude at this Y, the line is narrowed accordingly.
	availableLineWidth := func(lineY float64) (lineContentX, lineWidth float64) {
		if fc != nil {
			// fcY is in FC-relative coordinates. Since lineY is absolute
			// (contentY = g.ContentBoxTop() is absolute), convert to FC-relative.
			fcY := lineY - fc.originY
			left, right := fc.contentEdgesAt(fcY)
			// Shift from FC-relative to container-relative coordinates.
			// contentEdgesAt returns edges relative to fc.originX, but the
			// container's content box starts at contentX. The offset between
			// the two must be applied before clamping to the container bounds.
			if fc.originX != contentX {
				dx := contentX - fc.originX
				left += dx
				right += dx
			}
			if left < contentX {
				left = contentX
			}
			if right > contentX+contentWidth {
				right = contentX + contentWidth
			}
			return left, right - left
		}
		return contentX, contentWidth
	}

	type lineInfo struct {
		y, contentX float64 // line Y position and content start X
		segStart    int     // index into pending (first seg on this line)
		widthUsed   float64 // actual used width (contentX .. last-right-edge)
		availWidth  float64 // available width for this line (adjusted for floats)
		// boxCount 记录本行放置过的 inline 级 ElementBox 数量。0 宽度的
		// atomic inline（img.zero 之类 width:0/height:0 的替换元素）不贡献
		// widthUsed，但浏览器里它所在的行盒依然存在并带 strut 高度
		//（CSS2.1 §10.8：行盒高度至少是 strut）——只在 widthUsed>0 时保留行
		// 会把该行丢掉，容器高度塌成 0（inline-replaced-flow 的
		// "atomic inline run retains its line-height strut"）。
		boxCount int
		// ★ 行内基线对齐（CSS 2.1 §10.8）：maxBaseline 是本行已放置的表单
		// 控件要求的最大「margin-box top → 基线」距离，baselineBoxes 记录已按
		// 该基线定位的控件——后续出现更大的 maxBaseline 时把它们的整体下移
		// （行盒顶不变，基线随最高者下移），这正是浏览器「同一行所有控件共享
		// 一条基线」的语义。此前没有这一层：每个控件各自贴行盒顶，
		// form_controls 实测 y 偏 20/20/12/5px。
		maxBaseline   float64
		baselineBoxes []*ElementBox
		// maxDescent 是本行「基线以下」的最大深度（元素底边 + margin-bottom
		// 到基线的距离）。基线对齐后元素的底边可以超出最高的盒子（form 里
		// textarea 高 38、基线在其底边，而 input 的底边比它更低），所以行盒
		// 高度必须是 maxBaseline + maxDescent，而不是「max(border-box 高)」——
		// 后者会让下一行起点偏高（form_controls 的 progress 因此差 5px）。
		maxDescent float64
		// lineH 是【本行自身】的行盒高度：从基准行高 baseLineHeight 起步，
		// 仅当【本行】出现更高的 inline 子盒、或基线对齐把本行行盒顶高时抬高。
		// 换行推进与容器总高都必须用它，不能用跨行累积的全局值。
		lineH float64
	}

	// Initialize first line with float-aware available width.
	lineCx, lineCw := availableLineWidth(contentY)
	if toastWantLineDetail {
		toastWantLineDetail = false
		fmt.Printf("[toast-line] cx=%.2f cw=%.3f fc=%v contentX=%.2f contentWidth=%.2f fs=%.1f contentY=%.1f\n",
			lineCx, lineCw, fc != nil, contentX, contentWidth, fs, contentY)
	}
	var lines []lineInfo
	var pending []pendingSeg

	currentLine := lineInfo{y: contentY, contentX: lineCx, segStart: 0, widthUsed: 0, availWidth: lineCw, lineH: baseLineHeight}

	// lineTextOffset 返回本行文本段相对行盒顶的 Y 偏移。
	// 无控件参与时就是 half-leading 居中（原行为，零变化）；本行被表单控件的
	// 基线顶高时（maxBaseline > 文本自身基线位置），文本随基线下移——
	// 浏览器语义：行盒因更高的 atomic inline 变高后，同行文本仍坐在基线上。
	lineTextOffset := func() float64 {
		off := centeringOffset
		// ★ 比较基准是 **strut 的 ascent**（行盒顶→基线）：maxBaseline 现在
		//   含 strut 项，只有当某个控件/替换元素把基线抬得比 strut 更高时，
		//   同行文本才随基线下移（CSS 2.1 §10.8：文本仍坐在新基线上）。
		//   等于 strutAscent 时不移动 → 对无控件的普通文本行零影响。
		// quirks 下 strut 不参与，基准回到「文本自身基线位置」以保持旧行为。
		base := strutAscent
		if quirksNoStrut {
			base = centeringOffset + textAscent
		}
		if d := currentLine.maxBaseline - base; d > 0 {
			off += d
		}
		return off
	}

	// Out-of-flow (absolute/fixed) children of an inline container must be
	// laid out against their containing block, not as inline content.
	// Without this, an absolute pseudo-element thumb (e.g. a switch track's
	// ::after circle) would be treated as inline text and pinned to the
	// container's content-box origin, ignoring left/top.
	var deferredAbsolutes []*ElementBox
	defer func() {
		if len(deferredAbsolutes) == 0 {
			return
		}
		root := stateRootForBox(box)
		for _, ab := range deferredAbsolutes {
			cb := containingBlockForAbsolute(ab, root)
			layoutAbsolute(ab, cb, root, state)
		}
	}()

	// sepPending（行级）：字间空格分隔符已被「显式消费」（pre 模式的空间
	// segment，或 normal 模式的节点边界空白 advance）。下一个 word 不能再
	// 叠加 spaceWidth——否则 pre 模式下 " = " 之类文本节点的空格后单词会
	// 再得一个空格宽 → 相邻 span/token 之间出现异常空隙（CM6 .cm-line 的
	// 裸文本 " = "、"; // " 全中招）；normal 模式下两个相邻的空白文本节点
	// 也会产生双空格。行级作用域：跨文本节点连续生效，换行时复位。
	sepPending := false
	for _, child := range box.Children() {
		switch cld := child.(type) {
		case *InlineTextBox:
			text := cld.Text()
			if text == "" {
				continue
			}
			// Clear segments from any previous layout pass (e.g. auto-height
			// re-layout in flex formatting context).
			cld.TextSegments = cld.TextSegments[:0]
			runes := []rune(text)
			spaceWidth := measureText(box, " ")
			if spaceWidth <= 0 {
				spaceWidth = measureText(box, " ")
			}
			cursor := 0
			firstWord := true
			// white-space 模式（pre 系列语义，浏览器标准 CSS Text 3）：
			//  - pre / pre-wrap / break-spaces：连续空格保留（不折叠），每个
			//    空格占一个 advance width（xterm 终端行必须，行内字符等宽
			//    对齐依赖空格保留）。
			//  - pre / pre-wrap / pre-line：\n 显式换行强制断行。
			//  - pre / nowrap：禁止软换行（只在 \n 处换行）。
			ws := cs.WhiteSpace
			preserveSp := preserveWhiteSpace(ws)
			preserveNL := preserveNewlines(ws)
			softWrap := allowSoftWrap(ws)
			// If this text node starts with whitespace and there's already
			// content on the current line, advance by spaceWidth. This handles
			// both pure-whitespace text nodes (" " between inline elements)
			// and leading-whitespace text nodes (" main" after </span>).
			// pre 模式不折叠：前导空格由下方保留分支逐个渲染。
			if len(runes) > 0 && isInlineWhitespace(runes[0]) && currentLine.widthUsed > 0 && !preserveSp {
				if !sepPending {
					currentLine.widthUsed += spaceWidth
					sepPending = true
				}
			}
			for cursor < len(runes) {
				// pre 系列：\n 强制换行（浏览器 white-space:pre 语义——
				// 显式换行符结束当前行，新行从 contentX 开始）。
				if preserveNL && runes[cursor] == '\n' {
					lines = append(lines, currentLine)
					newY := currentLine.y + currentLine.lineH
					newCx, newCw := availableLineWidth(newY)
					currentLine = lineInfo{
						y:          newY,
						contentX:   newCx,
						segStart:   len(pending),
						widthUsed:  0,
						availWidth: newCw, lineH: baseLineHeight,
					}
					firstWord = true
					sepPending = false
					cursor++
					continue
				}
				// pre / pre-wrap / break-spaces：空格保留为独立 segment。
				// （终端每列等宽，空格与字符同宽 7.15px；折叠会破坏列对齐。）
				if preserveSp && (runes[cursor] == ' ' || runes[cursor] == '\t') {
					spW := spaceWidth
					if runes[cursor] == '\t' {
						spW = spaceWidth * 4 // 浏览器 tab 通常推进到下一个 8 列，
						// 终端/编辑器常见 4 列；xterm 行内无 tab，此值仅兜底。
					}
					pending = append(pending, pendingSeg{
						textBox: cld,
						seg: TextSegment{
							Start: cursor, Len: 1,
							X: currentLine.contentX + currentLine.widthUsed, Y: currentLine.y + lineTextOffset(),
							Width: spW, Height: textHeight,
							LineY: currentLine.y, LineHeight: currentLine.lineH,
						},
						lineIdx: len(lines),
					})
					currentLine.widthUsed += spW
					firstWord = false
					// 空格分隔符已被显式渲染：下一个 word 不得再叠加 spaceWidth
					//（否则 pre 模式下 "= " 后单词前出现双空格空隙）。
					sepPending = true
					cursor++
					continue
				}
				// Skip leading whitespace（normal/nowrap/pre-line 折叠）。
				// pre 模式已在上方处理 \n 与空格，这里仅剩 \r/\f。
				for cursor < len(runes) && isInlineWhitespace(runes[cursor]) {
					cursor++
				}
				if cursor >= len(runes) {
					break
				}
				wordStart := cursor
				for cursor < len(runes) && !isInlineWhitespace(runes[cursor]) {
					cursor++
				}
				wordEnd := cursor

				// Split the word into breakable sub-units: in browsers every
				// CJK ideograph is a soft-wrap opportunity by default (even
				// without word-break:break-word), while a non-CJK run keeps
				// its unbreakable-word semantics. Treating a space-less CJK
				// run as one unbreakable word made e.g. "完成摘要" in a
				// cramped flex header stay on one line and overlap its
				// siblings, where the browser wraps it per character.
				subWords := splitCJKWord(runes, wordStart, wordEnd)
				for wi, sub := range subWords {
					word := sub.text
					wordWidth := measureText(box, word)

					// Compute the x where this word would be placed.
					// A space separator applies only between whitespace-
					// delimited words (wi==0); CJK sub-units split from the
					// same original word have no space between them.
					// ★ sepPending：空格已被显式消费（pre 空格 segment /
					// 节点边界空白 advance）时不再叠加——避免双空格空隙。
					nextX := currentLine.widthUsed
					if !firstWord && wi == 0 && !sepPending {
						nextX += spaceWidth
					}
					sepPending = false
					// ★ 断行比较加 0.001 epsilon：文字宽度恰好等于可用宽度
					//   （如 13px 字体下"关闭"26px 与按钮 content 26px）时，
					//   浮点微差（26.0000001 > 25.9999999）会误判折行——
					//   AboutModal 关闭按钮文字竖排（content 31.2 两行）的根因。
					//   浏览器在宽度相等时不会换行；epsilon 仅吸收亚像素误差，
					//   不影响真实换行（真实换行需求通常差 > 1px）。
					if nextX+wordWidth > currentLine.availWidth+0.001 && currentLine.widthUsed > 0 && softWrap {
						// Line wrap: record line, start new line with float-aware width.
						lines = append(lines, currentLine)
						newY := currentLine.y + currentLine.lineH
						newCx, newCw := availableLineWidth(newY)
						currentLine = lineInfo{
							y:          newY,
							contentX:   newCx,
							segStart:   len(pending),
							widthUsed:  0,
							availWidth: newCw, lineH: baseLineHeight,
						}
						firstWord = true
						sepPending = false
						nextX = 0
					}
					// A single word wider than the whole line: break it per
					// character when word-break:break-all or
					// overflow-wrap:break-word (mirrors WebCore break-word
					// handling for long URLs / CJK-free text).
					// ★ nowrap 优先：white-space:nowrap 时浏览器完全禁止软换行，
					//   word-break/overflow-wrap 均不生效（长词溢出由 overflow
					//   裁剪/省略号处理）——漏掉此检查会让 .msg-bubble 继承的
					//   word-break:break-word 把 nowrap 的 tl-tc-param 长词拆行。
					if wordWidth > currentLine.availWidth && softWrap {
						wordBreak := cs.GetProperty("word-break")
						overflowWrap := cs.GetProperty("overflow-wrap")
						if wordBreak == "break-all" || wordBreak == "break-word" || overflowWrap == "break-word" {
							for i, ch := range []rune(word) {
								chStr := string(ch)
								chW := measureText(box, chStr)
								if currentLine.widthUsed > 0 && currentLine.widthUsed+chW > currentLine.availWidth+0.001 {
									lines = append(lines, currentLine)
									newY := currentLine.y + currentLine.lineH
									newCx, newCw := availableLineWidth(newY)
									currentLine = lineInfo{
										y:          newY,
										contentX:   newCx,
										segStart:   len(pending),
										widthUsed:  0,
										availWidth: newCw, lineH: baseLineHeight,
									}
									firstWord = true
									sepPending = false
								}
								pending = append(pending, pendingSeg{
									textBox: cld,
									seg: TextSegment{
										Start: sub.start + i, Len: 1,
										X: currentLine.contentX + currentLine.widthUsed, Y: currentLine.y + lineTextOffset(),
										Width: chW, Height: textHeight,
										LineY: currentLine.y, LineHeight: currentLine.lineH,
									},
									lineIdx: len(lines),
								})
								currentLine.widthUsed += chW
								firstWord = false
							}
							continue
						}
					}
					pending = append(pending, pendingSeg{
						textBox: cld,
						seg: TextSegment{
							Start: sub.start, Len: len([]rune(word)),
							X: currentLine.contentX + nextX, Y: currentLine.y + lineTextOffset(),
							Width: wordWidth, Height: textHeight,
							LineY: currentLine.y, LineHeight: currentLine.lineH,
						},
						lineIdx: len(lines), // current (in-progress) line
					})
					currentLine.widthUsed = nextX + wordWidth
					firstWord = false
				}
			}
		case *ElementBox:
			if debugenv.Enabled("WBUI_IFC_DEBUG") {
				nm, cls, disp := "<anon>", "", "nil"
				if e := cld.Element(); e != nil {
					nm, cls = e.NodeName(), e.GetAttribute("class")
				}
				if cs := cld.Style(); cs != nil {
					disp = cs.Display.String()
				}
				fmt.Fprintf(os.Stderr, "[ifc-disp] %s class=%q display=%s inlineLevel=%v replaced=%v\n",
					nm, cls, disp, cld.IsInlineLevel(), cld.IsReplaced())
			}
			// Absolute/fixed children are out-of-flow: collect for deferred
			// layout against their containing block (handled above).
			if cld.IsAbsolutelyPositioned() {
				deferredAbsolutes = append(deferredAbsolutes, cld)
				continue
			}
			if !cld.IsInlineLevel() {
				continue
			}
			cldG := state.GeometryForBox(cld)

			// Compute margin/padding/border BEFORE the child's Layout so the
			// child's formatting context (e.g. BFC.Layout for inline-block) sees
			// the correct ContentBoxLeft/ContentBoxTop. Without this,
			// padding/border are treated as zero and text inside the child gets
			// positioned at the child's border-box top-left instead of its
			// content-box origin.
			fs := fontSizeOf(cld)
			margin, padding, border := computeBoxModelForBox(cld, contentWidth, fs)
			cldG.SetMargin(margin.Top, margin.Right, margin.Bottom, margin.Left)
			cldG.SetPadding(padding.Top, padding.Right, padding.Bottom, padding.Left)
			cldG.SetBorder(border.Top, border.Right, border.Bottom, border.Left)
			// Horizontal margins shift the child and consume line space;
			// margin-top lowers the child inside the line box.
			// ★ 纯 inline（非 replaced/inline-block）子元素：顶贴行框顶
			// （浏览器语义：inline box 顶 = 行框顶，glyph 垂直居中由绘制层
			// baseline 公式负责——PaintText 行框居中）。若此处叠加
			// centeringOffset，inline box 顶=行框顶+half-leading → 内部
			// 文字的行框 LineY 也被下移 → 行框顶≠背景盒顶（activeLine
			// 背景不居中、文字偏下的根因）。inline-block/replaced 保留
			// centeringOffset（垂直居中于行框）。
			topOffset := centeringOffset
			if cldCS := cld.Style(); cldCS != nil && ((cld.IsInline() && !cld.IsReplaced() && cldCS.Display == style.DisplayInline) || cldCS.VerticalAlign == "top") {
				topOffset = 0
			}
			cldG.SetTopLeft(currentLine.y+topOffset+margin.Top, currentLine.contentX+currentLine.widthUsed+margin.Left)

			// ★ 换行约束：无显式宽度的 inline 子元素（含 flex item 文本的
			//    匿名 inline 包装盒）必须以「父级行宽」而非自身 max-content
			//    作为换行 availWidth——否则 flex 压缩的 span 内 CJK 文本不折行
			//    （"完成摘要"在 27px 容器内保持 41px 单行溢出，Edge 中 2+2
			//    折行）。child 的真实 box 宽度在其 Layout 之后由
			//    computeInlineContentWidth 恢复为内容宽度。
			//    ★ 不能用 ContentWidth()<=0 判断：上一轮布局（如 column flex
			//    冻结项预布局 0 宽）后 computeInlineContentWidth 把匿名包装
			//    宽度恢复成内容宽（单字 12px），本轮若跳过约束，包装内 CJK
			//    文本会按 12px 换行→每字一行（.resume-text 437px 撑爆输入区、
			//    .chat-messages 被挤到 110px 的根因）。无显式宽度的 inline
			//    子一律以父行宽约束换行，布局后仍由 computeInlineContentWidth
			//    恢复真实内容宽度用于行推进。
			if !cld.IsReplaced() {
				csc := cld.Style()
				hasExplicit := csc != nil && csc.Width.Unit != "" && csc.Width.Unit != "auto"
				// ★ inline-block 例外（浏览器标准 CSS 2.1 §10.3.9）：
				// inline-block auto 宽度 = shrink-to-fit（内容宽），不是
				// 撑满父行宽。此前无显式宽度的 inline-block（xterm 光标
				// div、badge、按钮等）被 SetContentWidth(父行宽) 撑满 →
				// 终端 block 光标 172px 整行宽（应为 1 字符 ~8px，用户
				// 反馈「光标块宽度巨大」）。inline-block 内部是 BFC，换行
				// 约束不适用（内容按自身 max-content 排版）。
				isInlineBlock := csc != nil && csc.Display == style.DisplayInlineBlock
				if !hasExplicit && !isInlineBlock {
					cldG.SetContentWidth(contentWidth)
				}
			}

			// Set CSS width if definite BEFORE Layout so box-sizing:border-box
			// correctly limits the content width used by the child's Layout.
			if cldG.ContentWidth() <= 0 {
				// ★ Replaced elements (img/video/canvas) size from their
				// resource, not from the line box (CSS 2.1 §10.3.2/§10.6.2 +
				// CSS Images 3 default sizing): an auto-width inline <img>
				// takes the intrinsic width, and a ratio-only resource (SVG
				// carrying just a viewBox, i.e. most data-URI icons) fills
				// the available line width with height = width / ratio.
				// Without this a ratio-only SVG stayed 0×18 (line height)
				// and its wrapper never reserved the intrinsic box.
				if cld.IsReplaced() {
					if rw, rh, ok := replacedContentSize(cld, contentWidth, g.ContentHeight()); ok && rw > 0 {
						cldG.SetContentWidth(rw)
						if rh > 0 && cldG.ContentHeight() <= 0 {
							cldG.SetContentHeight(rh)
						}
					}
				}
			}
			if cldG.ContentWidth() <= 0 {
				cs := cld.Style()
				if cs != nil {
					if w, ok := definiteWidth(cs.Width, contentWidth, fs); ok && w > 0 {
						if debugenv.Enabled("WB_LAYOUT_DEBUG") && cld.Element() != nil && cld.Element().NodeName() == "INPUT" {
							fmt.Printf("[in] INPUT width: %% of contentWidth=%.1f → %v (container=%v class=%q)\n", contentWidth, w, box.Element(), func() string {
								if box.Element() != nil {
									return box.Element().GetAttribute("class")
								}
								return ""
							}())
						}
						if isBorderBoxForBox(cld) {
							b := cldG.BorderLeft() + cldG.BorderRight()
							p := cldG.PaddingLeft() + cldG.PaddingRight()
							cldG.SetContentWidth(w - b - p)
						} else {
							cldG.SetContentWidth(w)
						}
					}
				}
			}
			// Set CSS height if definite BEFORE Layout (inline-block/span with
			// explicit height, e.g. a switch track 34x18). Without this the
			// height collapses to the line-height, inflating the box.
			if cs := cld.Style(); cs != nil {
				if h, ok := definiteHeight(cs.Height, g.ContentHeight(), fs); ok && h > 0 {
					if isBorderBoxForBox(cld) {
						b := cldG.BorderTop() + cldG.BorderBottom()
						p := cldG.PaddingTop() + cldG.PaddingBottom()
						cldG.SetContentHeight(h - b - p)
					} else {
						cldG.SetContentHeight(h)
					}
				}
			}

			// ★ inline-block 布局前预设置 shrink-to-fit 宽度（CSS 2.1
			// §10.3.9）：childCtx.Layout 时若 content width 仍为 0（无显式
			// 宽度且上面跳过 SetContentWidth(父行宽)），内部 normal 文本会
			// 按 0 宽布局 → CJK 每字竖排（配置器「编辑」按钮 27px 宽文字
			// 竖排成多行、属性行被撑高的根因）。浏览器先按 max-content
			// 测量 inline-block 再布局内部。布局后仍有原 shrink-to-fit
			// 回填（含 max-width clamp 与重测量），此处只保证内部文本
			// 按单行 max-content 布局。
			if csc := cld.Style(); csc != nil && csc.Display == style.DisplayInlineBlock {
				hasExplicitIB := csc.Width.Unit != "" && csc.Width.Unit != "auto"
				if !hasExplicitIB && cldG.ContentWidth() <= 0 {
					if txt := inlineBoxTextContent(cld); txt != "" {
						if tw := measureText(cld, txt); tw > 0 {
							cldG.SetContentWidth(tw)
						}
					} else if iw := intrinsicContentWidth(cld, false); iw > 0 {
						// ★ 无文本的 inline-block：max-content 由子盒决定
						// （CSS 2.1 §10.3.9 shrink-to-fit 的 max-content 项）。
						// 不设置的话内部 IFC 以 0 可用宽建行：#chips 的三个
						// inline-block li 全部落在 x=0 重叠、容器宽被量成单个
						// li 的 30px（inline-block-flex-items 期望 90x20 横排）。
						// intrinsicContentWidth 返回外盒宽 → 扣自身 padding+border。
						_, pb, bd := computeBoxModel(cld, 0, fontSizeOf(cld))
						iw -= pb.Horizontal() + bd.Horizontal()
						if avail := contentWidth - margin.Horizontal() - pb.Horizontal() - bd.Horizontal(); avail > 0 && iw > avail {
							iw = avail
						}
						if iw > 0 {
							cldG.SetContentWidth(iw)
						}
					}
				}
			}

			childCtx := contextFor(cld, state)
			childCtx.Layout(cld, state)

			// ★ <br> 强制换行（浏览器语义）：br 产生一个行框（高度 =
			// lineHeight），即使行内无其他内容。此前 br 零宽不推进
			// widthUsed → 空行（.cm-line 只有 <br>）无行框 → 块高度 0
			// → CM6 行高 oracle 测到空行高度 0，输入后重校准把空行
			// gutter 元素高度设为 0px → 后续行号全部上移错乱（CM6 编辑
			// 器行号两位数/错位、滚动不绘制）。浏览器里 <div><br></div>
			// 高度 = line-height（Edge 19.59px / wb-ui 18.2px）。
			if el := cld.Element(); el != nil && el.LocalName() == "br" {
				// 提交当前行内容（br 前若有文本，先结束该行）
				if currentLine.widthUsed > 0 {
					lines = append(lines, currentLine)
				}
				// br 本身占据一个空行框（保证空行高度 = lineHeight）
				lines = append(lines, lineInfo{
					y:          currentLine.y,
					contentX:   currentLine.contentX,
					segStart:   len(pending),
					widthUsed:  0,
					availWidth: currentLine.availWidth, lineH: currentLine.lineH,
				})
				newY := currentLine.y + currentLine.lineH
				newCx, newCw := availableLineWidth(newY)
				currentLine = lineInfo{
					y:          newY,
					contentX:   newCx,
					segStart:   len(pending),
					widthUsed:  0,
					availWidth: newCw, lineH: baseLineHeight,
				}
				continue
			}

			// ★ inline-block shrink-to-fit（CSS 2.1 §10.3.9）：无显式宽度
			// 的 inline-block 宽度 = 内容 max-content（xterm 光标 div 1 字符
			// ≈ 8px），不撑满父行宽。childCtx.Layout（BFC）不设置容器自身
			// 的 ContentWidth（容器宽由父级决定），布局后仍为 0 → 需用内部
			// 文本测量回填（浏览器语义：光标块正好覆盖当前字符）。
			// ★ 无条件回填（去掉 ContentWidth()<=0 守卫）：上一轮布局可能
			// 给 inline-block 残留父行宽（配置面板「编辑」按钮 contentWidth
			// 残留 147 → 172px 大背景块），守卫会让残留不被重算。
			// ★ 受 max-width 约束（§10.3.9：shrink-to-fit ≤ max-width）：
			// 欢迎语内容预览 .txt-view max-width:110px 未截断（148px）的
			// 根因。
			if csc := cld.Style(); csc != nil && csc.Display == style.DisplayInlineBlock {
				hasExplicitIB := csc.Width.Unit != "" && csc.Width.Unit != "auto"
				// ★ 表单控件的宽度**不由内容决定**：<textarea> 的 auto 宽度取 UA
				// 固有宽度（cols × 字符宽，见 formControlContentSize），内容在控件
				// 内部滚动，不参与 shrink-to-fit。此前 textarea 的子文本节点让
				// shrink-to-fit 把内容宽写进 ContentWidth（"line1" ≈ 28px → 边框盒
				// 36px），下面 754 行的 formControlContentSize 分支因
				// `ContentWidth() <= 0` 守卫随即跳过 → 宽度只剩 36px（Edge 161px），
				// 并把同行后续的 range / progress 整体左移 125px——一致性套件
				// form_controls 的 textarea w / range x / progress x,y 四处差异
				// 是同一个根因。
				_, _, attrSized := formControlContentSize(cld)
				if !hasExplicitIB && !attrSized {
					if txt := inlineBoxTextContent(cld); txt != "" {
						if tw := measureText(cld, txt); tw > 0 {
							if mw, ok := definiteWidth(csc.MaxWidth, contentWidth, fs); ok && mw > 0 && tw > mw {
								tw = mw
							}
							cldG.SetContentWidth(tw)
						}
					}
				}
			}

			// Re-apply explicit width: the child's Layout (IFC for inline
			// content) collapses the box to content width; a definite CSS
			// width on an inline-block must win.
			if cs := cld.Style(); cs != nil {
				if w, ok := definiteWidth(cs.Width, contentWidth, fs); ok && w > 0 {
					if isBorderBoxForBox(cld) {
						b := cldG.BorderLeft() + cldG.BorderRight()
						p := cldG.PaddingLeft() + cldG.PaddingRight()
						cldG.SetContentWidth(w - b - p)
					} else {
						cldG.SetContentWidth(w)
					}
				}
			}

			// Fallback: for replaced input/button elements without explicit CSS
			// width, derive width from the HTML value attribute text. Text
			// inputs get the browser default ~20ch width (Edge reports ~177px
			// at 13.33px font) regardless of the value attribute.
			if cldG.ContentWidth() <= 0 && cld.IsReplaced() {
				if el := cld.Element(); el != nil && el.NodeName() == "INPUT" {
					typ := el.GetAttribute("type")
					val := el.GetAttribute("value")
					switch typ {
					case "text", "password", "search", "email", "url", "tel",
						"number", "date", "time", "month", "week", "datetime-local":
						// Browser default text-field width (~20ch at 13.33px;
						// Edge reports 177px border-box). The input UA styles
						// set box-sizing:border-box with 4px padding + 1px
						// borders, so the content width is 177-10=167.
						cldG.SetContentWidth(167)
					case "submit":
						val = "Submit"
					case "reset":
						val = "Reset"
					case "button":
						val = "Button"
					}
					if val != "" && cldG.ContentWidth() <= 0 {
						textW := measureText(cld, val)
						if textW > 0 {
							cldG.SetContentWidth(textW)
						}
					}
					if cldG.ContentWidth() > 0 && cldG.ContentHeight() <= 0 {
						lineH := fontLineGap(cld)
						if lineH <= 0 {
							lineH = fs * 1.2
						}
						cldG.SetContentHeight(lineH)
					}
				} else if el := cld.Element(); el != nil {
					// SELECT / TEXTAREA default sizes (browser defaults):
					// select ≈ 45px wide; input/textarea size from their
					// UA intrinsic geometry (size / cols / rows attributes).
					switch el.LocalName() {
					case "select":
						cldG.SetContentWidth(45)
						if cldG.ContentHeight() <= 0 {
							cldG.SetContentHeight(fs * 1.4)
						}
					case "textarea", "input":
						// 固有内容盒：input = size×8+9 宽 × 单行高；
						// textarea = cols×8 宽 × rows×行高（见 engine/layout/formcontrol.go）。
						if w, h, ok := formControlContentSize(cld); ok {
							if cldG.ContentWidth() <= 0 && w > 0 {
								cldG.SetContentWidth(w)
							}
							if cldG.ContentHeight() <= 0 && h > 0 {
								cldG.SetContentHeight(h)
							}
						}
					}
				}
			}
			if cldG.ContentHeight() <= 0 {
				cs := cld.Style()
				if cs != nil {
					if h, ok := definiteHeight(cs.Height, contentWidth, fs); ok && h > 0 {
						if isBorderBoxForBox(cld) {
							b := cldG.BorderTop() + cldG.BorderBottom()
							p := cldG.PaddingTop() + cldG.PaddingBottom()
							cldG.SetContentHeight(h - b - p)
						} else {
							cldG.SetContentHeight(h)
						}
					}
				}
			}
			// A definite height on an inline-block/replaced child wins over
			// the content-derived height (e.g. button height:34px). Plain
			// inline (span) heights have no layout effect per CSS.
			if cld.IsInline() && cld.Style().Display == style.DisplayInlineBlock {
				if cs := cld.Style(); cs != nil {
					if h, ok := definiteHeight(cs.Height, contentWidth, fs); ok && h > 0 {
						if isBorderBoxForBox(cld) {
							b := cldG.BorderTop() + cldG.BorderBottom()
							p := cldG.PaddingTop() + cldG.PaddingBottom()
							cldG.SetContentHeight(h - b - p)
						} else {
							cldG.SetContentHeight(h)
						}
					}
				}
			}

			// If content height is still 0 (no CSS height), use line height.
			if cldG.ContentHeight() <= 0 {
				lineH := fontLineGap(cld)
				if lineH <= 0 {
					lineH = fs * 1.2
				}
				cldG.SetContentHeight(lineH)
			}

			// ★ min-height / max-height clamp（CSS 2.1 §10.7，border-box 语义）。
			// replaced/inline-block 才有布局尺寸；min-height 是 border-box
			// 下限。浏览器中 .inst-textarea{min-height:60px} 让 rows=2 的
			// textarea 也撑到 60px——此前无 clamp：主 agent(rows=3)=59px、
			// 子 agent(rows=2)=44px，两者不一致且比浏览器矮（Edge 两者都
			// 是 60px）。textarea UA box-sizing:border-box → 内容下限 =
			// min-height - border - padding。
			if cld.IsReplaced() || (cld.IsInline() && cld.Style().Display == style.DisplayInlineBlock) {
				if cs := cld.Style(); cs != nil {
					minH, maxH, minAuto, maxAuto := resolveMinMax(cs.MinHeight, cs.MaxHeight, contentWidth, fs)
					if !minAuto || !maxAuto {
						b := cldG.BorderTop() + cldG.BorderBottom()
						p := cldG.PaddingTop() + cldG.PaddingBottom()
						ch := cldG.ContentHeight()
						if !minAuto {
							minContent := minH - b - p
							if ch < minContent {
								ch = minContent
							}
						}
						if !maxAuto {
							maxContent := maxH - b - p
							if ch > maxContent {
								ch = maxContent
							}
						}
						if ch < 0 {
							ch = 0
						}
						cldG.SetContentHeight(ch)
					}
				}
			}

			// vertical-align (CSS 2.1 §10.8): inline-block/replaced children
			// with vertical-align:middle are centered against the line box.
			// The child was initially placed at currentLine.y+centeringOffset;
			// shift it so its vertical center aligns with the line's center.
			{
				va := ""
				if cldCS := cld.Style(); cldCS != nil {
					va = cldCS.Properties["vertical-align"]
				}
				if va == "middle" {
					// The child's vertical margin participates in the line
					// box: line box height = child border-box + margins.
					childBH := cldG.BorderBoxHeight()
					childH := childBH + margin.Top + margin.Bottom
					lineH := currentLine.lineH
					if childH > lineH {
						lineH = childH
					}
					if childH > 0 && lineH > 0 {
						// Line box: from currentLine.y to currentLine.y+lineH.
						// Place child's middle at line's middle, then shift
						// by margin-top so the margin stays outside.
						lineTop := currentLine.y
						targetTop := lineTop + (lineH-childH)/2 + margin.Top
						cldG.SetTopLeft(targetTop, cldG.Left())
					}
				} else if (va == "" || va == "baseline") && cld.IsReplaced() && textAscent > 0 &&
					!isFormControlElement(cld.Element()) {
					// ★ 表单控件不走通用替换元素分支（其基线是**内部文本基线**，
					//   不是 margin box 底边）——统一交给下面的表单控件基线分支
					//   （formControlBaselineFromBorderTop）。两条分支同时作用时
					//   通用分支会把 maxBaseline 抬到 border-box 高（h2_baseline_formula
					//   的 f 用例 relTop 6 而 Edge 4）。
					// ★ vertical-align:baseline（默认）——替换元素（svg/img/…）
					// 的基线 = 其下外边距边缘（CSS 2.1 §10.8.1：替换元素无内联
					// 内容，基线取 margin box 底边），必须坐在父行盒的基线上：
					//     border-box top = 行盒顶 + halfLeading + ascent - 高 - marginBottom
					//
					// 此前只按 centeringOffset 偏移（= (line-height - 字体行高)/2），
					// 缺 ascent 项；且 line-height:normal 时 cssLH=0 → 偏移整体为 0
					// → 图标贴行盒顶。实测（1280x800，.qexec-btn 的 .qexec-caret）：
					// 引擎 svg 顶 12.0 vs 浏览器 16.0，偏差 4px（该 span 内只有 svg、
					// 无文本，行盒基线无从谈起，偏移恒为 0 暴露得最彻底）；
					// 按基线公式算 12 + (15-14.52)/2 + 11.64 - 8 = 15.88 ≈ 浏览器 16.0。
					childBH := cldG.BorderBoxHeight()
					if childBH > 0 {
						// ★ 行盒基线 = 【全体基线对齐盒】要求的最大值（CSS 2.1
						//   §10.8.1）：替换元素要求「行盒顶→基线」至少
						//   marginTop + borderBoxH（它的基线就是 margin box
						//   底边），文本只要求 halfLeading + 字体 ascent。
						//   此前固定取文本度量，比文本行高的替换元素（AboutModal
						//   的 64×64 logo）底边坐上文本基线后，顶边整体浮出行盒
						//   上方（实测 图片 y=246 vs 父容器 y=296，上移 50px →
						//   logo 与标题间出现空洞；浏览器 y=296 正好贴容器顶）。
						//   矮于文本行的图标（如 .qexec-caret 的 8px svg）required
						//   被文本基线兜住 → 定位与旧公式逐位相同，零回归。
						required := margin.Top + childBH
						textBase := halfLeading + textAscent
						if required > textBase && required > currentLine.maxBaseline {
							// 本行基线随最高的替换元素下沉；本元素登记进
							// baselineBoxes，后续出现更高者时一并下移
							// （与表单控件共用同一条行内基线机制）。
							currentLine.maxBaseline = required
						}
						baseLineTop := textBase
						if currentLine.maxBaseline > baseLineTop {
							baseLineTop = currentLine.maxBaseline
						}
						baseLine := currentLine.y + baseLineTop
						cldG.SetTopLeft(baseLine-childBH-margin.Bottom, cldG.Left())
						// 基线在 margin box 底边 → 基线以下深度即 margin.Bottom。
						currentLine.baselineBoxes = append(currentLine.baselineBoxes, cld)
						// ★ strut 的 descent 也参与：替换元素底部之上，行盒底仍要
						//   容纳 strut 基线以下的深度（Edge 实测 r_img/ib_30x30
						//   的行盒高 35 = 30 控件 + 5 strut descent）。
						if strutDescent > currentLine.maxDescent {
							currentLine.maxDescent = strutDescent
						}
						if margin.Bottom > currentLine.maxDescent {
							currentLine.maxDescent = margin.Bottom
						}
					}
				}
				// ★ inline-block 参与行盒高度（CSS 2.1 §10.8.1）：基线对齐的
				//   inline-block 其基线是**底边 margin 边**（无行内内容时），
				//   行盒至少要容纳「margin-top + border-box 高」在基线上方、
				//   「margin-bottom」在基线下方。
				//   此前只有 replaced 元素与 vertical-align:middle 参与，
				//   inline-block 完全不计入行盒：正常字体下行盒被基准行高
				//   （如 24）兜住看不出来；但 `font-size:0; line-height:0` 的
				//   容器基准行高为 0，行盒塌成 0，20px 的 inline-block 溢出
				//   行盒 —— cssprobe legacy-center 的 `#legacy` 首个行盒因此
				//   为 0（Edge 20），其后所有块整体上移。
				//   只提升行盒高、不改子盒定位（仍放在
				//   currentLine.y+centeringOffset）：仅当 inline-block 高于
				//   当前行盒时生效，因此对正常字体页面零影响。
				if va == "" || va == "baseline" {
					if cldCS := cld.Style(); cldCS != nil && cldCS.Display == style.DisplayInlineBlock {
						// ★ 空 inline-block（无行内内容）的基线是其**底边 margin 边**
						//   （CSS 2.1 §10.8.1）→ 把「marginTop + border-box 高」放在
						//   基线上方、margin-bottom 放在下方，于是
						//   行盒高 = max(strutAscent, 子盒 ascent) +
						//            max(strutDescent, 子盒 descent)。
						//   此前只把 lineH 提升到「子盒高」，漏了 strut 的 descent ——
						//   minibox 的 d（16px 容器 + 100×20 空 inline-block）行盒
						//   24 而 Edge 为 25（= 20 + strut descent 5）。
						//   有内容的 inline-block 基线在**内部**（≈ 内部 strut 基线），
						//   与「底边」不同，故此处只处理空内容情形（现有 lineH 提升
						//   分支对它们已正确）。
						// ★ 表单控件（input/button/select/textarea…）同样没有子节点
						//   （inlineIsEmpty 为真），但它们的基线是**内部文本基线**而非
						//   底边，须由下面的表单控件分支统一处理 —— 此处必须排除，
						//   否则控件被当成「空 inline-block」按底边抬到 21px
						//   （h2_baseline_formula 的 f 用例 relTop 6，Edge 4）。
						// ★ 表单控件（input/button/select/textarea…）同样没有子节点
						//   （inlineIsEmpty 为真），但它们的基线是**内部文本基线**而非
						//   底边，须由下面的表单控件分支统一处理 —— 此处必须排除，
						//   否则控件被当成「空 inline-block」按底边抬到 21px
						//   （h2_baseline_formula 的 f 用例 relTop 6，Edge 4）。
						// ★ 另一条限定「有显式高度」：无高度的空 inline-block 自身高为 0
						//   （Edge 的 ib_empty 即 ctrlH=0），其 ascent 也是 0，不该改变
						//   行盒；只有**带显式高度**的空 inline-block（基线 = 底边、
						//   ascent = 高度）才参与 max(ascent)+max(descent)。
						//   （实测若不加此条件，含文本的 ib_noheight 行盒会从 24 涨到 29。）
						chnCS := cld.Style()
						// ★ el != nil 是必须的：匿名盒（Element() == nil）没有 DOM 元素，
						//   inlineIsEmpty(nil) 会解引用空指针 panic（CJK 夹具必经此路径）。
						if el := cld.Element(); el != nil && !isFormControlElement(el) && inlineIsEmpty(el) &&
							chnCS != nil && chnCS.Height.Unit != "" && chnCS.Height.Unit != "auto" {
							asc := margin.Top + cldG.BorderBoxHeight()
							if strutAscent > asc {
								asc = strutAscent
							}
							desc := margin.Bottom
							if strutDescent > desc {
								desc = strutDescent
							}
							if asc > currentLine.maxBaseline {
								currentLine.maxBaseline = asc
							}
							if desc > currentLine.maxDescent {
								currentLine.maxDescent = desc
							}
							if h := currentLine.maxBaseline + currentLine.maxDescent; h > currentLine.lineH {
								currentLine.lineH = h
							}
						}
						if need := margin.Top + cldG.BorderBoxHeight() + margin.Bottom; need > currentLine.lineH {
							currentLine.lineH = need
						}
					}
				}
			}

			// Compute inline child's content width from text segments.
			// Without this, cldW=0 and subsequent text on same line overlaps.
			// For non-explicit-width inline children this re-computation is
			// unconditional: the wrap-constraint above may have set a
			// full-width constraint value, which must shrink back to the
			// real text extent for line advancement and hit-testing.
			hasExplicitChildWidth := false
			if csc := cld.Style(); csc != nil && csc.Width.Unit != "" && csc.Width.Unit != "auto" {
				hasExplicitChildWidth = true
			}
			if !hasExplicitChildWidth {
				if cw := computeInlineContentWidth(cld, state); cw > 0 {
					// ★ inline-block 的 shrink-to-fit 受 max-width 约束
					// （CSS 2.1 §10.3.9）：computeInlineContentWidth 按文本
					// 实宽回填（txt-view 148.3px），会覆盖 shrink 分支的
					// max-width clamp（110px）——欢迎语预览未截断的根因。
				if csc := cld.Style(); csc != nil && csc.Display == style.DisplayInlineBlock {
					if mw, ok := definiteWidth(csc.MaxWidth, contentWidth, fs); ok && mw > 0 && cw > mw {
						cw = mw
					}
				}
				cldG.SetContentWidth(cw)
				}
			} else if cldG.ContentWidth() <= 0 {
				if cw := computeInlineContentWidth(cld, state); cw > 0 {
					cldG.SetContentWidth(cw)
				}
			}
			// Expand lineHeight to match the inline child's actual height.
			// The child (e.g. anonymous wrapper around text) may use a
			// different font-size (inherited or from CSS), producing a
			// taller line than fontLineGap(box) estimates.
			// ★ 含垂直 margin：inline-block/replaced 子盒参与行盒计算的是它的
			// margin 盒（CSS 2.1 §10.8）。漏掉 margin 会让相邻行的盒子直接
			// 贴在一起——img-density-and-alt 的 .box{display:inline-block;
			// margin:6px} 三个红框因此粘连成一个连通色块（父高 124 而应为
			// 136），每个独立边框都量不出来。水平方向本已计入（见下面的
			// cldW），垂直方向是对称补齐。
			// ★ 只记到【本行】的高度。原先写全局 lineHeight（跨行累积），会让
			// 后续每一行都继承本行的最高子盒——容器高度被放大的来源之一。
			if debugenv.Enabled("WBUI_IFC_DEBUG") {
				nm := "<anon-inline>"
				if e := cld.Element(); e != nil {
					nm = e.NodeName()
				}
				fmt.Fprintf(os.Stderr, "[ifc-child] %s borderBoxH=%.3f marginV=%.3f contentH=%.3f fs=%.3f floor(before)=%.3f\n",
					nm, cldG.BorderBoxHeight(), margin.Vertical(), cldG.ContentHeight(), fontSizeOf(cld), currentLine.lineH)
			}
			if cldBH := cldG.BorderBoxHeight() + margin.Vertical(); cldBH > currentLine.lineH {
				currentLine.lineH = cldBH
			}
			cldW := cldG.BorderBoxWidth() + margin.Horizontal()
			// ★ 空 inline span（xterm DOM renderer 的空格列 span——子节点
			// 只有空白 text，渲染树构建跳过空白 → 无 RenderText）的宽度被
			// 错误算成行宽（572px）→ 后续词水平错位 572px（终端多列错位）。
			// 浏览器语义：空内联不占行宽。
			if cld.IsInline() && !cld.IsReplaced() {
				if el := cld.Element(); el != nil && inlineIsEmpty(el) &&
					cldG.BorderBoxWidth() >= currentLine.availWidth {
					cldW = 0
				}
			}
			if currentLine.widthUsed+cldW > currentLine.availWidth && currentLine.widthUsed > 0 && allowSoftWrap(cs.WhiteSpace) {
				lines = append(lines, currentLine)
				newY := currentLine.y + currentLine.lineH
				newCx, newCw := availableLineWidth(newY)
				currentLine = lineInfo{
					y:          newY,
					contentX:   newCx,
					segStart:   len(pending),
					widthUsed:  0,
					availWidth: newCw, lineH: baseLineHeight,
				}
				// 与初始放置同一规则：纯 inline 顶贴行框顶。
				topOffset := centeringOffset
				if csc := cld.Style(); csc != nil && ((cld.IsInline() && !cld.IsReplaced() && csc.Display == style.DisplayInline) || csc.VerticalAlign == "top") {
					topOffset = 0
				}
				cldG.SetTopLeft(currentLine.y+topOffset, currentLine.contentX+currentLine.widthUsed)
				// ★ 换行后 inline-block 内部子布局基于换行前的旧 x 生成
				// （其 childCtx.Layout 在行推进之前执行）：文本 seg 坐标
				// 保持旧位置 → 渲染端 syncOne 把 frame 扩展至旧 seg 右端
				// （编辑按钮渲染成 172px 大背景块），文字也画在旧位置
				// （超出右栏被裁，按钮显示为空块）。重新布局使内部 seg/
				// 几何基于新位置（幂等：布局只依赖 box 自身位置）。
				if csc := cld.Style(); csc != nil && csc.Display == style.DisplayInlineBlock {
					childCtx.Layout(cld, state)
				}
			}
			// ★ 表单控件的行内基线对齐（CSS 2.1 §10.8）：同行所有 inline 级
			// 控件共享一条基线。maxBaseline 是本行目前为止最大的「margin-box
			// top → 基线」距离，控件按
			//     border-box top = 行盒顶 + maxBaseline - 自身基线偏移
			// 定位。后续控件带来更大的 maxBaseline 时，先把本行已放置的控件
			// 整体下移 delta（行盒顶不变、基线随最高者下移），并把本行已生成
			// 的文本段一起下移，使文本仍坐在新基线上。
			//
			// 此前没有这一层：控件各自按 vertical-align:middle 的「自高中居中」
			// 摆放（只按自身高度算，不含同行更高的元素），于是 form 里每个控件
			// 都贴行盒顶——consistency 的 form_controls 实测 y 偏 20/20/12/5px，
			// 只能靠 30px 的宽松容差掩盖。
			if el := cld.Element(); el != nil {
				ba, _ := fontAscentDescent(cld)
				// ★ 控件自身的字体 ascent 同样取整（浏览器把字体度量各自 round
				//   后再用于基线定位）：Edge 的控件基线距顶恒为
				//   2(border) + 1(padding) + round(12.07) = 15，用未取整的
				//   12.07 会让 relTop 差 0.07（h2_baseline_formula 实测
				//   3.930 vs Edge 4.000）。
				ba = math.Round(ba)
				if off, ok := formControlBaselineFromBorderTop(el, cldG.BorderTop(), cldG.PaddingTop(), cldG.BorderBoxHeight(), ba); ok {
					// ★ 行盒顶→基线的距离取「strut 与控件要求」的较大者：控件
					//   基线坐在行盒基线上，而行盒基线至少由 strut 的 ascent
					//   决定（CSS 2.1 §10.8）。此前只取控件自身要求 → 控件永远
					//   贴行盒顶（h2_baseline_matrix：input/fs16 relTop 恒 0，
					//   Edge 为 4 = strutAscent 19 − 控件基线 15）。
					align := margin.Top + off
					if strutAscent > align {
						align = strutAscent
					}
					if align > currentLine.maxBaseline {
						delta := align - currentLine.maxBaseline
						currentLine.maxBaseline = align
						for _, b := range currentLine.baselineBoxes {
							bg := state.GeometryForBox(b)
							bg.SetTopLeft(bg.Top()+delta, bg.Left())
						}
						for i := currentLine.segStart; i < len(pending); i++ {
							pending[i].seg.Y += delta
						}
					}
					cldG.SetTopLeft(currentLine.y+currentLine.maxBaseline-off, cldG.Left())
					currentLine.baselineBoxes = append(currentLine.baselineBoxes, cld)
					// 行盒高度 = maxAscent(基线) + maxDescent（基线以下最深者）。
					// 基线对齐后元素底边可能超过最高的盒子，所以不能再用
					// 「max(border-box 高 + 垂直 margin)」那一套（本文件后面
					// 对非控件仍保留它作为兜底）。
					// ★ strut 的 descent 同样参与（行盒底至少到 strut 基线下方）。
					d := (cldG.BorderBoxHeight() - off) + margin.Bottom
					if strutDescent > d {
						d = strutDescent
					}
					if d > currentLine.maxDescent {
						currentLine.maxDescent = d
					}
					if h := currentLine.maxBaseline + currentLine.maxDescent; h > currentLine.lineH {
						currentLine.lineH = h
					}
				}
			}
			// Apply relative offset to inline-level elements that are
			// relatively positioned (e.g. position:relative with top/left).
			if cld.IsRelativelyPositioned() && cld.IsInFlow() {
				applyRelativeOffsetForBox(cld, contentWidth, lineHeight, state)
			}
			currentLine.widthUsed += cldW
			currentLine.boxCount++
			// 有实际宽度的 inline 子元素消费了待处理的空格分隔符：
			// 后续空白节点的 advance 是新分隔符（<span>foo</span> <span>
			// bar</span> 后的 "  baz" 前导空格仍贡献一个空格）。
			if cldW > 0 {
				sepPending = false
			}
		}
	}

	// Append the final in-progress line.
	// ★ 判定条件是「行内有内容」而非「宽度 > 0」：0 宽度的 atomic inline
	//（img.zero）也产生一个带 strut 高度的行盒，丢弃它会让容器高度塌成 0。
	if currentLine.widthUsed > 0 || currentLine.boxCount > 0 {
		lines = append(lines, currentLine)
	}

	// Compute container height from line count.
	totalHeight := 0.0
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		// ★ 用【最后一行自身】的高度，而不是跨行累积的全局 lineHeight：
		// 基线对齐会把含最高控件的那一行顶到 44px，拿它当最后一行高度会让
		// 容器底部多出 ~28px 死空间（form 88 vs Edge 60，其后的兄弟元素还会
		// 被整体下推）。
		totalHeight = (lastLine.y - contentY) + lastLine.lineH
	}
	if isToastBox {
		if toastLinesPrinted < 3 {
			toastLinesPrinted++
			fmt.Printf("[toast-lines] call=%d n=%d totalH=%.1f contentY=%.1f contentWidth=%.2f\n",
				toastLinesPrinted, len(lines), totalHeight, contentY, contentWidth)
			for i, ln := range lines {
				if i > 8 {
					break
				}
				fmt.Printf("[toast-lines]   [%d] y=%.1f cx=%.2f used=%.2f avail=%.2f h=%.2f seg=%d\n",
					i, ln.y, ln.contentX, ln.widthUsed, ln.availWidth, ln.lineH, ln.segStart)
			}
		}
	}

	// Update content width to match the actual text content width. This
	// ensures the layout box geometry reflects the real text extent so that
	// syncOne sets the correct frame width. Without this, flex items with
	// overflow:hidden would clip the text because their frame is narrower
	// than the actual text content.
	//
	// EXCEPTION: flex items — their width is decided by the flex algorithm
	// (base size ± grow/shrink) and MUST NOT be widened back to the raw text
	// extent. A long file name in .item-name (overflow:hidden + ellipsis)
	// previously got its 238px flex-shrunk width overwritten to 260px here,
	// overflowing the item-row instead of ellipsizing — the "file names only
	// show a few characters / overflow the row" report.
	var totalWidth float64
	// ★ 绝对/固定定位容器（out-of-flow）：其尺寸由 layoutAbsolute 决定
	// （box-sizing 扣减 border/padding），此处按子元素宽度回写 content 会
	// 覆盖正确的 content（如 22x22 border-box 按钮被内部文本盒宽 22 覆盖 →
	// content 8 变 22 → border-box 36x26）。浏览器中 absolute 元素尺寸
	// 独立于流内文本宽度，不应被 IFC 收尾修正。
	// ★ 表格内部盒（行/单元格/表节等）例外：它们的宽度由表格布局算法决定
	// （列轨道宽），行内文本不得回写——否则 `table-layout:fixed` 的第二行
	// 单元格含 nowrap 长文本时，列轨道宽 50 会被文本宽 277.3 覆盖
	// （fixed-table-layout 的 "later separate row cannot resize first track"，见
	// isTableInternalBox）。浏览器中单元格宽度与内容无关，内容只会溢出。
	if !isFlexItem(box) && !box.IsAbsolutelyPositioned() && !isTableInternalBox(box) {
		for _, ps := range pending {
			right := ps.seg.X + ps.seg.Width - contentX
			if right > totalWidth {
				totalWidth = right
			}
		}
		// Also include inline ElementBox children's widths (e.g. span > anonymous
		// wrapper > text). These children may have their own content width updated
		// by their IFC, but the parent box's width needs to encompass them.
		for _, child := range box.Children() {
			if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
				cg := state.GeometryForBox(eb)
				if right := cg.Left() + cg.BorderBoxWidth() - contentX; right > totalWidth {
					totalWidth = right
				}
			}
		}
		if totalWidth > 0 {
			g.SetContentWidth(totalWidth)
		}
	}

	// Apply text-align adjustment AFTER setting content width so the shift
	// is computed against the final (non-expanded) content width rather than
	// the expanded auto-width estimate. Without this, text-align:center inside
	// buttons lands the text at the wrong X because contentWidth was widened
	// by +20 during auto-width expansion but then corrected to totalWidth.
	if totalWidth > 0 {
		contentWidth = totalWidth
	}
	if textAlign != style.TextAlignLeft && len(lines) > 0 {
		for li, ln := range lines {
			var used float64
			if li < len(lines)-1 {
				for i := ln.segStart; i < lines[li+1].segStart && i < len(pending); i++ {
					s := pending[i].seg
					r := s.X + s.Width - contentX
					if r > used {
						used = r
					}
				}
			} else {
				used = ln.widthUsed
			}

			var shift float64
			switch textAlign {
			case style.TextAlignCenter:
				shift = (containerWidth - used) / 2
			case style.TextAlignRight, style.TextAlignEnd:
				shift = containerWidth - used
			}
			if shift > 0 {
				for i := ln.segStart; i < len(pending); i++ {
					if pending[i].lineIdx != li && i >= (func() int {
						if li+1 < len(lines) {
							return lines[li+1].segStart
						}
						return len(pending)
					})() {
						break
					}
					pending[i].seg.X += shift
				}
				// Also shift the inline ElementBox children on this line.
				for _, child := range box.Children() {
					if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
						ebG := state.GeometryForBox(eb)
						ebG.SetTopLeft(ebG.Top(), ebG.Left()+shift)
						// ★ 级联平移 absolute 子元素：inline-block（如
						// .ic-video .tri）在 text-align 居中前已用旧位置
						// 定位（布局时序），必须跟随父元素平移，否则三角
						// 形相对图标水平错位。
						offsetDescendants(eb, shift, 0, state)
					}
				}
			}
		}
	}

	// Vertically center single-line content when box is taller than the text.
	if len(lines) <= 1 && totalHeight > 0 && len(pending) > 0 {
		shift := (boxHeight - totalHeight) / 2
		if shift < 0 {
			shift = 0 // text taller than box; keep at top
		}
		if shift > 0 {
			for i := range pending {
				pending[i].seg.Y += shift
			}
			// Also shift inline ElementBox children on this line.
			for _, child := range box.Children() {
				if eb, ok := child.(*ElementBox); ok && eb.IsInlineLevel() {
					ebG := state.GeometryForBox(eb)
					ebG.SetTopLeft(ebG.Top()+shift, ebG.Left())
					offsetDescendants(eb, 0, shift, state)
				}
			}
		}
	}

	// Flush pending segments to their InlineTextBoxes. Must run AFTER the
	// text-align and vertical-centering adjustments above, otherwise the
	// segments copied into TextSegments would keep their pre-adjustment X/Y.
	for _, ps := range pending {
		ps.textBox.TextSegments = append(ps.textBox.TextSegments, ps.seg)
	}

	// ★ 不写回内容的两种情形（写回内容尺寸前必须先排除"高度已由 CSS 或父
	// 布局上下文确定"的盒子）：
	//
	// 1) 绝对/固定定位容器（out-of-flow）：高度由 layoutAbsolute 决定
	//    （box-sizing 扣减 border/padding），此处按文本内容高度回写会覆盖
	//    正确值（按钮 height 18 → 22，border-box 失效）。
	//
	// 2) **显式 height（definite）的盒子**：CSS 2.1 §10.6.3/§10.7 下块级盒高
	//    由 CSS 高度或内容决定，二者**互斥**——height 为 definite 时盒高就是它，
	//    内容溢出既不缩小也不撑大盒高。此前无条件 max(boxHeight, totalHeight)
	//    等于把 height 当成 min-height：`height:20px` + 16px 文本（行盒 24）
	//    被撑成 24，而 Edge 是 20（minibox 探针 a：cssHeight=20px / rect 24）。
	//    cssprobe 的 legacy-center 里 `#pure-center`、`.cell-center` 都写着
	//    `height:20px` 却量到 24，同一根因。
	//    注意：height:auto 时必须保留 max(boxHeight, …)——boxHeight 可能是父
	//    格式化上下文已分配的高度（flex cross-axis stretch / grid area），
	//    内容比它矮时不能把盒子缩回去。
	if !box.IsAbsolutelyPositioned() && heightIsAutoForBox(box) {
		g.SetContentHeight(math.Max(boxHeight, totalHeight))
	}
}

// isInlineWhitespace reports whether r is a CSS whitespace character that
// separates words in inline layout.
func isInlineWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// preserveWhiteSpace reports whether the white-space mode preserves spaces
// (pre / pre-wrap / break-spaces): consecutive spaces are NOT collapsed and
// each space occupies its own advance width.
func preserveWhiteSpace(ws style.WhiteSpaceType) bool {
	return ws == style.WhiteSpacePre || ws == style.WhiteSpacePreWrap || ws == style.WhiteSpaceBreakSpaces
}

// preserveNewlines reports whether the white-space mode preserves explicit
// newline characters (pre / pre-wrap / pre-line): '\n' forces a line break.
func preserveNewlines(ws style.WhiteSpaceType) bool {
	return ws == style.WhiteSpacePre || ws == style.WhiteSpacePreWrap || ws == style.WhiteSpacePreLine
}

// allowSoftWrap reports whether the white-space mode permits soft wrapping at
// break opportunities. pre and nowrap forbid soft wrap (only explicit '\n'
// breaks); normal / pre-wrap / pre-line allow it.
func allowSoftWrap(ws style.WhiteSpaceType) bool {
	return ws != style.WhiteSpacePre && ws != style.WhiteSpaceNoWrap
}

// inlineWordSub is one breakable sub-unit split from a whitespace-delimited
// word: either a single CJK ideograph or a contiguous non-CJK run. start is
// the rune offset of the sub-unit within the original text runes.
type inlineWordSub struct {
	start int
	text  string
}

// splitCJKWord splits runes[ws:we] (a whitespace-delimited word, no spaces
// inside) into breakable sub-units: every CJK ideograph becomes its own
// sub-unit (browsers allow line breaks between CJK characters by default),
// while contiguous non-CJK characters stay together as one sub-unit
// (unbreakable-word semantics preserved for Latin runs).
func splitCJKWord(runes []rune, ws, we int) []inlineWordSub {
	var out []inlineWordSub
	var cur []rune
	curStart := -1
	flush := func() {
		if len(cur) > 0 {
			out = append(out, inlineWordSub{start: curStart, text: string(cur)})
			cur = nil
			curStart = -1
		}
	}
	for i := ws; i < we; i++ {
		r := runes[i]
		if isCJKChar(r) {
			flush()
			out = append(out, inlineWordSub{start: i, text: string(r)})
		} else {
			if curStart < 0 {
				curStart = i
			}
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// isCJKChar reports whether r is a CJK ideograph / fullwidth form that
// browsers treat as a default soft-wrap opportunity. Ranges mirror
// engine/platform/graphics canvas.go runeCJK classification.
func isCJKChar(r rune) bool {
	switch {
	case r >= 0x3400 && r <= 0x4DBF, // CJK Ext A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility
		r >= 0x3000 && r <= 0x303F, // CJK Symbols and Punctuation
		r >= 0xFF00 && r <= 0xFFEF: // Fullwidth forms
		return true
	}
	return false
}

// computeInlineContentWidth computes the inline content width of an ElementBox
// from its inline-level content. InlineFormattingContext.Layout sets
// ContentHeight but not ContentWidth, so inline ElementBox children would get
// zero width causing subsequent text to overlap.
//
// ★ 2026-09-26：替换元素（svg/img/canvas/...）**不产生 TextSegment**，其占位
// 宽度只能从自身的布局几何取得。此前只统计文本段，含 svg 的 inline 盒会被
// 量窄：顶栏「快速执行」按钮（display:inline-flex，内容 = bolt svg + 文本 +
// caret svg）的内容宽被算成 59（= 文本 44 + ？），而内部 flex 布局按自然宽
// 73 摆放子项 → 末尾 caret svg 越过按钮右边界 14px（= svg 8 + gap 4 +
// margin 2，实测引擎 caret 右缘 1583 vs 按钮右边界 1578）。帮助菜单
// `.menu-btn` 的 chevron 同源（引擎按钮宽 40 vs 浏览器 55，图标越界 7px）——
// 该缺陷此前被误判为「flex 固有宽度问题」，修复打在了 intrinsicContentWidth
// （flexformattingcontext.go）上，而真正生效的宽度回填走的是本函数。
//
// walk 递归进非替换子盒以收集其内部文本段与替换元素：替换元素无子盒
// （box.go 的 buildChildren 对替换元素直接 return），故其右缘直接取几何。
func computeInlineContentWidth(box *ElementBox, state *LayoutState) float64 {
	g := state.GeometryForBox(box)
	base := g.ContentBoxLeft()
	maxRight := 0.0
	bump := func(r float64) {
		if r > maxRight {
			maxRight = r
		}
	}
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				for _, seg := range itb.TextSegments {
					bump(seg.X + seg.Width)
				}
			}
			if eb, ok := c.(*ElementBox); ok {
				if eb.IsReplaced() {
					rg := state.GeometryForBox(eb)
					bump(rg.Left() + rg.BorderBoxWidth())
				}
				walk(eb)
			}
		}
	}
	walk(box)
	if maxRight <= base {
		return 0
	}
	return maxRight - base
}

// quirksStrutSuppressed reports whether the line-box strut must be dropped for
// box. Only quirks-mode documents do this, and only when the box's inline
// content is exclusively atomic (replaced / inline-block) boxes with no text —
// exactly the `<form><input size=17></form>` shape.
//
// 实测（Chrome headless，本机参考浏览器）：
//
//	quirks    <form><input size=17></form> → form 21.2（= input 边框盒高）
//	standards 同一结构                      → form 25.8（input 21.2 + strut descent 4.6）
//
// 即 quirks 下 strut 的 ascent/descent 都不进行盒，行盒高 = 子盒 margin box 高。
// 该分支只在「全部子盒都是 atomic 且无可见文本」时成立：任何文本节点或行内非
// 替换盒都会让判定失败，从而保持原有的 strut 语义（正常字体页面零影响）。
func quirksStrutSuppressed(box *ElementBox) bool {
	if box == nil {
		return false
	}
	// 文档 quirks 判定：box 可能是匿名盒（Element() == nil），沿父链找元素。
	quirks := false
	for b := box; b != nil; b = b.Parent() {
		el := b.Element()
		if el == nil {
			continue
		}
		if doc := el.OwnerDocument(); doc != nil {
			quirks = doc.Quirks()
		}
		break
	}
	if !quirks {
		return false
	}
	children := box.Children()
	if len(children) == 0 {
		return false
	}
	for _, c := range children {
		switch t := c.(type) {
		case *InlineTextBox:
			for _, r := range t.Text() {
				switch r {
				case ' ', '\t', '\n', '\r', '\f', '\v':
				default:
					return false
				}
			}
		case *ElementBox:
			cs := t.Style()
			if cs == nil {
				return false
			}
			if !t.IsReplaced() && cs.Display != style.DisplayInlineBlock {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// inlineIsEmpty reports whether an inline element has no visible content
// (no children, or only whitespace-only text children). Used to avoid the
// xterm whitespace-column span occupying a whole line width.
func inlineIsEmpty(el *dom.Element) bool {
	if el.FirstChild() == nil {
		return true
	}
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*dom.Text); ok {
			if strings.TrimSpace(t.Data()) != "" {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

var _ = style.DisplayInline
