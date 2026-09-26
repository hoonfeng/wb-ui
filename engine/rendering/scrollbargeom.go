// Translation of: Source/WebCore/rendering/RenderScrollbar.cpp (geometry part)
//
// 滚动条几何的单一事实来源：绘制（renderpipeline.go）与宿主交互
// （app/host.go 的拖动/滚轮）共用同一套公式，避免两侧参数漂移
// （此前拖动端用 padding-box 高度、绘制端用内容盒高度，且轨道长
// 差了一个 arrowGap，导致 thumb 拖动位移与内容滚动量不成比例、
// 滚不到底等异常）。
package rendering

import (
	"wb-ui/engine/style"
)

const (
	// sbThumbInset 是滚动条滑块（thumb）与轨道四周的间隙（CSS px）。
	//
	// ★ 实测（2026-09-26，真实有头 Edge 153，dpr=1.25，逐像素扫描 .pp-list）：
	// 轨道 15px，thumb **纯色**列宽 11 物理像素；轨道顶端到 thumb 顶端 18.4px。
	// 取 3.0 而非几何上的 3.1，是为了让 15-2×3 = 9 CSS px = 11.25 物理像素
	// 完整覆盖 11 列（取 3.1 时几何宽 8.8px，末列只覆盖 0.875，纯色只剩 10 列，
	// 实测就是这样少了一列——wb-ui 侧像素复验确认）。
	//
	// ⚠️ 上一版按无头 Chromium 的「经典滚动条」实现（thumb 宽 = 轨道宽-3、直角），
	// 比真实浏览器明显更宽、更方——这正是用户反馈「样式不一样」的直接原因。
	sbThumbInset = 3.0

	// sbArrowGap 是箭头按钮与 thumb 可移动轨道之间的间隙（CSS px）。
	//
	// 浏览器里箭头按钮与 thumb 之间没有额外留白，thumb 顶端距轨道顶端就是
	// 「箭头按钮高 + thumb 自身 inset」= 15 + 3.1（实测 18.4，误差 0.3px）。
	sbArrowGap = sbThumbInset
)

// sbArrowSize 返回箭头按钮的边长：平台经典滚动条的箭头按钮是**正方形**，
// 边长等于滚动条宽度（有头 Edge 实测 15px 轨道 → 15×15 按钮）。
//
// 像素核对（.pp-list，dpr=1.25）：上箭头三角中心在轨道顶端下方 8.8px，
// 与 15px 按钮的中心 7.5px 接近（差 1.3px，三角绘制本身的取整偏差）。
func sbArrowSize(scrollW float64) float64 {
	return scrollW
}

// sbThumbWidth 返回自绘滚动条（元素继承到非 auto scrollbar-color 时）的 thumb
// 宽度：轨道宽两侧各留 sbThumbInset。
//
// ★ 实测（有头 Edge 153，dpr=1.25，.pp-list）：轨道 15px、thumb 纯色列 11 物理
// 像素（≈ 9 CSS px），即 15 - 2×3。轨道过窄时退化为整宽，避免负宽度。
func sbThumbWidth(scrollW float64) float64 {
	w := scrollW - 2*sbThumbInset
	if w < 3 {
		return scrollW
	}
	return w
}

// sbThumbRadius 返回自绘滚动条 thumb 的圆角半径：实测为胶囊形，即宽度的一半
// （dpr=1.25 下 thumb 顶端 4 行由窄到宽，与半径=半宽的圆弧吻合）。
func sbThumbRadius(scrollW float64) float64 {
	return sbThumbWidth(scrollW) / 2
}

// ScrollbarMetrics 描述一条滚动条（垂直或水平）的几何。
// 绘制端据此画 thumb；宿主拖动/滚轮据此把指针位移映射为滚动偏移，
// 保证 thumb 移动 1px 对应内容滚动 (MaxScroll / travel) px，与绘制一致。
type ScrollbarMetrics struct {
	OK        bool    // 应绘制滚动条（内容溢出 + overflow 允许）
	TrackLen  float64 // thumb 可移动轨道长（track 总长减箭头与 gap）
	ThumbLen  float64 // thumb 长度（最小 18，最大 TrackLen-4，与绘制一致）
	MaxScroll float64 // sx/sy 最大值 = 内容总长 - 可视内容长
	ViewLen   float64 // 可视内容长（内容盒）
	TotalLen  float64 // 内容总长
}

// scrollbarWidthFor mirrors the CSS scrollbar-width property used by the
// painter: 12px default, 8px thin, 0 none (still scrollable). It also honors
// ::-webkit-scrollbar { width: Npx } (Blink/WebKit custom width — wins over
// the standard property, matching Chrome).
//
// ★ 2026-09-26：实现下沉到 style.ScrollbarWidth —— 布局侧的「常驻滚动条
// 预留宽度」（engine/layout/scrollbarreserve.go）必须与绘制用同一个值，
// 否则会出现「布局按 12px 预留、绘制画 8px」的错位。
func scrollbarWidthFor(st *style.ComputedStyle) float64 {
	return style.ScrollbarWidth(st)
}

// webkitCustomScrollbar reports whether the element has Blink/WebKit custom
// scrollbar styling (::-webkit-scrollbar { width/background } or
// ::-webkit-scrollbar-thumb). Custom webkit scrollbars render WITHOUT the
// classic arrow buttons and their thumb fills the full scrollbar width —
// unlike the default flat style (12px + arrow buttons).
func webkitCustomScrollbar(st *style.ComputedStyle) bool {
	if st == nil {
		return false
	}
	// ★ Chromium 语义（2026-09-26 实测）：元素继承到非 auto 的 scrollbar-color
	// 时，::-webkit-scrollbar 自定义规则被**整体忽略**，滚动条回退平台经典样式
	// （17px、带箭头、thumb 无圆角）。gou-ide 全页继承 html 的 scrollbar-color，
	// 因此这条分支把它挡回经典样式——只有显式 scrollbar-color:auto 的组件才走
	// ::-webkit-scrollbar 细滚动条。
	if style.HasCustomScrollbarColor(st) {
		return false
	}
	return st.GetProperty("-webkit-scrollbar-width") != "" ||
		st.GetProperty("-webkit-scrollbar-height") != "" ||
		st.GetProperty("-webkit-scrollbar-thumb-color") != "" ||
		st.GetProperty("-webkit-scrollbar-track-color") != "" ||
		st.GetProperty("-webkit-scrollbar-thumb-radius") != ""
}

// boxViewAndContent returns the client viewport (padding-box, per CSSOM —
// clientWidth/clientHeight include padding) and the content extent
// (BoxContentSize). Gating an overflow:auto box against the content-box
// height makes every vertically-padded container look "overflowed" (the
// content extent counts content + top padding), so ws-section /
// project-section / sidebar-content all showed spurious scrollbars even
// when content fit exactly. The painter, hit-test and drag geometry all
// share this single source.
func boxViewAndContent(rv *RenderView, box *RenderBox) (viewW, viewH, totalW, totalH float64) {
	pb := box.PaddingBoxRect()
	viewW = pb.Width
	viewH = pb.Height
	if viewW < 1 {
		viewW = 1
	}
	if viewH < 1 {
		viewH = 1
	}
	totalW, totalH = rv.BoxContentSize(box)
	return
}

// needsScrollbars mirrors the painter's needsV/needsH decision:
// overflow:scroll always, overflow:auto when content exceeds the viewport,
// never when overflow:hidden (or visible).
func needsScrollbars(st *style.ComputedStyle, totalW, totalH, viewW, viewH float64) (needV, needH bool) {
	if st == nil {
		return false, false
	}
	needV = (st.OverflowY == style.OverflowScroll ||
		(st.OverflowY == style.OverflowAuto && totalH > viewH)) &&
		st.OverflowY != style.OverflowHidden
	needH = (st.OverflowX == style.OverflowScroll ||
		(st.OverflowX == style.OverflowAuto && totalW > viewW)) &&
		st.OverflowX != style.OverflowHidden
	return
}

// ScrollRange 返回元素作为滚动容器时允许的滚动范围：max* 为内容总长与可视
// 长之差（CSSOM View 的滚动上限），horizontal/vertical 表示该轴是否可滚动。
//
// ★ 与 VerticalScrollbarMetrics 的 OK 字段不是同一件事：后者回答"滚动条该
// 怎么画"，容器小到放不下箭头按钮时（vh <= 2*arrow+2*gap）它返回 OK=false
// （不绘制滚动条），但**元素依然可滚动**。浏览器里 10×10 的
// overflow:scroll 容器照样 scrollTop = 15（滚动条画不下就不画）。因此
// scrollTop/scrollLeft 赋值与 scrollTo/scrollBy 的判定必须走本函数，否则
// 小尺寸滚动容器上的程序化滚动会被静默丢弃（React 19 水合契约里的
// scroller.scrollTo(...) 正是这种容器：10×10 + 100×100 内容）。
func ScrollRange(rv *RenderView, box *RenderBox) (maxX, maxY float64, horizontal, vertical bool) {
	if rv == nil || box == nil {
		return 0, 0, false, false
	}
	viewW, viewH, totalW, totalH := boxViewAndContent(rv, box)
	needV, needH := needsScrollbars(box.Style(), totalW, totalH, viewW, viewH)
	if needV {
		maxY = totalH - viewH
		if maxY < 0 {
			maxY = 0
		}
		vertical = true
	}
	if needH {
		maxX = totalW - viewW
		if maxX < 0 {
			maxX = 0
		}
		horizontal = true
	}
	return
}

// VerticalScrollbarMetrics computes the vertical scrollbar geometry for a
// box, using exactly the same viewport/extent/arrow constants as the
// painter. Returns OK=false when no vertical scrollbar should be drawn.
func VerticalScrollbarMetrics(rv *RenderView, box *RenderBox) ScrollbarMetrics {
	if rv == nil || box == nil {
		return ScrollbarMetrics{}
	}
	viewW, viewH, totalW, totalH := boxViewAndContent(rv, box)
	needV, needH := needsScrollbars(box.Style(), totalW, totalH, viewW, viewH)
	if !needV || totalH <= viewH {
		return ScrollbarMetrics{}
	}
	pb := box.PaddingBoxRect()
	vh := pb.Height
	if needH {
		vh -= scrollbarWidthFor(box.Style())
	}
	webkit := webkitCustomScrollbar(box.Style())
	arrow := 0.0
	if !webkit {
		arrow = sbArrowSize(scrollbarWidthFor(box.Style()))
	}
	if !webkit && vh <= arrow*2+sbArrowGap*2 {
		return ScrollbarMetrics{}
	}
	trackLen := vh
	if !webkit {
		trackLen = vh - arrow*2 - sbArrowGap*2
	}
	thumbLen := trackLen * viewH / totalH
	if thumbLen < 18 {
		thumbLen = 18
	}
	if thumbLen > trackLen-4 {
		thumbLen = trackLen - 4
	}
	maxSy := totalH - viewH
	if maxSy <= 0 {
		maxSy = 1
	}
	return ScrollbarMetrics{OK: true, TrackLen: trackLen, ThumbLen: thumbLen, MaxScroll: maxSy, ViewLen: viewH, TotalLen: totalH}
}

// HorizontalScrollbarMetrics computes the horizontal scrollbar geometry.
// Returns OK=false when no horizontal scrollbar should be drawn.
func HorizontalScrollbarMetrics(rv *RenderView, box *RenderBox) ScrollbarMetrics {
	if rv == nil || box == nil {
		return ScrollbarMetrics{}
	}
	viewW, viewH, totalW, totalH := boxViewAndContent(rv, box)
	needV, needH := needsScrollbars(box.Style(), totalW, totalH, viewW, viewH)
	if !needH || totalW <= viewW {
		return ScrollbarMetrics{}
	}
	pb := box.PaddingBoxRect()
	hw := pb.Width
	if needV {
		hw -= scrollbarWidthFor(box.Style())
	}
	webkit := webkitCustomScrollbar(box.Style())
	arrow := 0.0
	if !webkit {
		arrow = sbArrowSize(scrollbarWidthFor(box.Style()))
	}
	if !webkit && hw <= arrow*2+sbArrowGap*2 {
		return ScrollbarMetrics{}
	}
	trackLen := hw
	if !webkit {
		trackLen = hw - arrow*2 - sbArrowGap*2
	}
	thumbLen := trackLen * viewW / totalW
	if thumbLen < 18 {
		thumbLen = 18
	}
	if thumbLen > trackLen-4 {
		thumbLen = trackLen - 4
	}
	maxSx := totalW - viewW
	if maxSx <= 0 {
		maxSx = 1
	}
	return ScrollbarMetrics{OK: true, TrackLen: trackLen, ThumbLen: thumbLen, MaxScroll: maxSx, ViewLen: viewW, TotalLen: totalW}
}
