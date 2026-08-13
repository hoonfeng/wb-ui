// Translation of: Source/WebCore/layout/formattingContexts/inline/InlineFormattingContext.cpp
package layout

import (
	"fmt"
	"math"
	"os"
	"strings"

	"wb-ui/dom"
	"wb-ui/style"
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

func (c *InlineFormattingContext) Layout(box *ElementBox, state *LayoutState) {
	defer profileLayout("ifc")()
	cs := box.Style()
	if cs == nil {
		return
	}
	g := state.GeometryForBox(box)

	contentX := g.ContentBoxLeft()
	contentY := g.ContentBoxTop()
	boxHeight := g.ContentHeight()
	contentWidth := g.ContentWidth()
	if os.Getenv("WB_LAYOUT_DEBUG") != "" && box.Element() != nil && box.Element().NodeName() == "DIV" && box.Element().GetAttribute("class") == "dialog-box" {
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
	// Use CSS line-height if explicitly set (overrides font metrics).
	cssLH := cssLineHeight(box)
	if cssLH > 0 {
		lineHeight = cssLH
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
	centeringOffset := 0.0
	isPlainInline := cs != nil && box.IsInline() && !box.IsReplaced() && cs.Display == style.DisplayInline
	if !isPlainInline && cssLH > 0 && textHeight < cssLH {
		centeringOffset = (cssLH - textHeight) / 2
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
	textAlign := style.TextAlignStart
	if cs != nil {
		textAlign = cs.TextAlign
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
	}

	// Initialize first line with float-aware available width.
	lineCx, lineCw := availableLineWidth(contentY)
	var lines []lineInfo
	var pending []pendingSeg

	currentLine := lineInfo{y: contentY, contentX: lineCx, segStart: 0, widthUsed: 0, availWidth: lineCw}

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
					newY := currentLine.y + lineHeight
					newCx, newCw := availableLineWidth(newY)
					currentLine = lineInfo{
						y:          newY,
						contentX:   newCx,
						segStart:   len(pending),
						widthUsed:  0,
						availWidth: newCw,
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
							X: currentLine.contentX + currentLine.widthUsed, Y: currentLine.y + centeringOffset,
							Width: spW, Height: textHeight,
							LineY: currentLine.y, LineHeight: lineHeight,
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
						newY := currentLine.y + lineHeight
						newCx, newCw := availableLineWidth(newY)
						currentLine = lineInfo{
							y:          newY,
							contentX:   newCx,
							segStart:   len(pending),
							widthUsed:  0,
							availWidth: newCw,
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
									newY := currentLine.y + lineHeight
									newCx, newCw := availableLineWidth(newY)
									currentLine = lineInfo{
										y:          newY,
										contentX:   newCx,
										segStart:   len(pending),
										widthUsed:  0,
										availWidth: newCw,
									}
									firstWord = true
									sepPending = false
								}
								pending = append(pending, pendingSeg{
									textBox: cld,
									seg: TextSegment{
										Start: sub.start + i, Len: 1,
										X: currentLine.contentX + currentLine.widthUsed, Y: currentLine.y + centeringOffset,
										Width: chW, Height: textHeight,
										LineY: currentLine.y, LineHeight: lineHeight,
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
							X: currentLine.contentX + nextX, Y: currentLine.y + centeringOffset,
							Width: wordWidth, Height: textHeight,
							LineY: currentLine.y, LineHeight: lineHeight,
						},
						lineIdx: len(lines), // current (in-progress) line
					})
					currentLine.widthUsed = nextX + wordWidth
					firstWord = false
				}
			}
		case *ElementBox:
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
			if cldCS := cld.Style(); cldCS != nil && cld.IsInline() && !cld.IsReplaced() && cldCS.Display == style.DisplayInline {
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
				cs := cld.Style()
				if cs != nil {
					if w, ok := definiteWidth(cs.Width, contentWidth, fs); ok && w > 0 {
						if os.Getenv("WB_LAYOUT_DEBUG") != "" && cld.Element() != nil && cld.Element().NodeName() == "INPUT" {
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
					availWidth: currentLine.availWidth,
				})
				newY := currentLine.y + lineHeight
				newCx, newCw := availableLineWidth(newY)
				currentLine = lineInfo{
					y:          newY,
					contentX:   newCx,
					segStart:   len(pending),
					widthUsed:  0,
					availWidth: newCw,
				}
				continue
			}

			// ★ inline-block shrink-to-fit（CSS 2.1 §10.3.9）：无显式宽度
			// 的 inline-block 宽度 = 内容 max-content（xterm 光标 div 1 字符
			// ≈ 8px），不撑满父行宽。childCtx.Layout（BFC）不设置容器自身
			// 的 ContentWidth（容器宽由父级决定），布局后仍为 0 → 需用内部
			// 文本测量回填（浏览器语义：光标块正好覆盖当前字符）。
			if csc := cld.Style(); csc != nil && csc.Display == style.DisplayInlineBlock {
				hasExplicitIB := csc.Width.Unit != "" && csc.Width.Unit != "auto"
				if !hasExplicitIB && cldG.ContentWidth() <= 0 {
					if txt := inlineBoxTextContent(cld); txt != "" {
						if tw := measureText(cld, txt); tw > 0 {
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
					// select ≈ 45px wide, textarea ≈ 2 columns x 2 rows.
					switch el.LocalName() {
					case "select":
						cldG.SetContentWidth(45)
						if cldG.ContentHeight() <= 0 {
							cldG.SetContentHeight(fs * 1.4)
						}
					case "textarea":
						chW := measureText(cld, "0")
						if chW <= 0 {
							chW = fs * 0.5
						}
						// ~20 cols x N rows（rows 属性，默认 2），加上 padding。
						cldG.SetContentWidth(20*chW + 4)
						cldG.SetContentHeight(textareaRows(cld)*fontLineGap(cld) + 4)
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
					lineH := lineHeight
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
			if cldBH := cldG.BorderBoxHeight(); cldBH > lineHeight {
				lineHeight = cldBH
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
				newY := currentLine.y + lineHeight
				newCx, newCw := availableLineWidth(newY)
				currentLine = lineInfo{
					y:          newY,
					contentX:   newCx,
					segStart:   len(pending),
					widthUsed:  0,
					availWidth: newCw,
				}
				// 与初始放置同一规则：纯 inline 顶贴行框顶。
				topOffset := centeringOffset
				if csc := cld.Style(); csc != nil && cld.IsInline() && !cld.IsReplaced() && csc.Display == style.DisplayInline {
					topOffset = 0
				}
				cldG.SetTopLeft(currentLine.y+topOffset, currentLine.contentX+currentLine.widthUsed)
			}
			// Apply relative offset to inline-level elements that are
			// relatively positioned (e.g. position:relative with top/left).
			if cld.IsRelativelyPositioned() && cld.IsInFlow() {
				applyRelativeOffsetForBox(cld, contentWidth, lineHeight, state)
			}
			currentLine.widthUsed += cldW
			// 有实际宽度的 inline 子元素消费了待处理的空格分隔符：
			// 后续空白节点的 advance 是新分隔符（<span>foo</span> <span>
			// bar</span> 后的 "  baz" 前导空格仍贡献一个空格）。
			if cldW > 0 {
				sepPending = false
			}
		}
	}

	// Append the final in-progress line.
	if currentLine.widthUsed > 0 {
		lines = append(lines, currentLine)
	}

	// Compute container height from line count.
	totalHeight := 0.0
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		totalHeight = (lastLine.y - contentY) + lineHeight
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
	if !isFlexItem(box) {
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

	g.SetContentHeight(math.Max(boxHeight, totalHeight))
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
// platform/graphics canvas.go runeCJK classification.
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
// from its text segments. InlineFormattingContext.Layout sets ContentHeight but
// not ContentWidth, so inline ElementBox children would get zero width causing
// subsequent text to overlap.
func computeInlineContentWidth(box *ElementBox, state *LayoutState) float64 {
	g := state.GeometryForBox(box)
	base := g.ContentBoxLeft()
	maxRight := 0.0
	var walk func(b *ElementBox)
	walk = func(b *ElementBox) {
		for _, c := range b.Children() {
			if itb, ok := c.(*InlineTextBox); ok {
				for _, seg := range itb.TextSegments {
					r := seg.X + seg.Width
					if r > maxRight {
						maxRight = r
					}
				}
			}
			if eb, ok := c.(*ElementBox); ok {
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
