// Translation of: Source/WebCore/rendering/RenderScrollbar.cpp (geometry part)
//
// 滚动条几何的单一事实来源：绘制（renderpipeline.go）与宿主交互
// （app/host.go 的拖动/滚轮）共用同一套公式，避免两侧参数漂移
// （此前拖动端用 padding-box 高度、绘制端用内容盒高度，且轨道长
// 差了一个 arrowGap，导致 thumb 拖动位移与内容滚动量不成比例、
// 滚不到底等异常）。
package rendering

import (
	"wb-ui/style"
)

const (
	sbArrowSize = 12.0 // 箭头按钮边长
	sbArrowGap  = 5.0  // 箭头与轨道间的间隙
)

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
// painter: 12px default, 8px thin, 0 none (still scrollable).
func scrollbarWidthFor(st *style.ComputedStyle) float64 {
	if st == nil {
		return 12
	}
	switch st.GetProperty("scrollbar-width") {
	case "thin":
		return 8
	case "none":
		return 0
	}
	return 12
}

// boxViewAndContent returns the content-box viewport (padding-box minus
// padding) and the content extent (BoxContentSize) for a box.
func boxViewAndContent(rv *RenderView, box *RenderBox) (viewW, viewH, totalW, totalH float64) {
	pb := box.PaddingBoxRect()
	st := box.Style()
	padL := lengthValue(st.PaddingLeft)
	padR := lengthValue(st.PaddingRight)
	padT := lengthValue(st.PaddingTop)
	padB := lengthValue(st.PaddingBottom)
	if padL < 0 {
		padL = 0
	}
	if padR < 0 {
		padR = 0
	}
	if padT < 0 {
		padT = 0
	}
	if padB < 0 {
		padB = 0
	}
	viewW = pb.Width - padL - padR
	viewH = pb.Height - padT - padB
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
	if vh <= sbArrowSize*2+sbArrowGap*2 {
		return ScrollbarMetrics{}
	}
	trackLen := vh - sbArrowSize*2 - sbArrowGap*2
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
	if hw <= sbArrowSize*2+sbArrowGap*2 {
		return ScrollbarMetrics{}
	}
	trackLen := hw - sbArrowSize*2 - sbArrowGap*2
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
