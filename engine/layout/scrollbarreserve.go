package layout

import (
	"wb-ui/engine/style"
)

// ── 常驻滚动条的内容宽度预留（classic scrollbar gutter）────────────────────
//
// ★ 背景（2026-09-26，gou-ide「插件面板列表项分隔线盖住滚动条」根因）：
// wb-ui 的滚动条此前纯粹是绘制层的东西——布局阶段完全不知道滚动条会占位，
// 滚动容器给子元素提供的可用宽度直接取 padding box 宽（内含滚动条占的那一条）。
// 于是 width:auto（块级）/ width:100% 的子元素连同它的 border-bottom 一路铺到
// 容器最右缘，压在滚动条上、把滚动条切成若干段（逐像素实测：列表项分隔线在
// 滚动条带 x=421..431 内同样存在，且交叉处像素由轨道色 (65,75,100) 变暗为
// (54,64,90)，即内容画在滚动条之上）。
//
// 浏览器不是这样：Chrome/Windows 的 classic 滚动条**占位**——同一份 gou-ide
// 前端在 Chromium 下实测 .pp-list offsetWidth=263 / clientWidth=246、子元素
// .pp-item 宽 246（差 17px 正是滚动条），即子元素可用宽度就是 clientWidth。
//
// ★ 判定时机：只能在渲染树布局收尾做（v1 放在 BFC 内的失败教训）
// overflow:auto 是否溢出取决于「内容总高 vs 滚动视口高」，而滚动视口高在布局
// 过程中**可能尚未确定**：.pp-list{flex:1} 的高度本应由父 flex 容器分配，但
// 它的 BFC 在自己收尾时读到的 g.PaddingBoxHeight() 仍是「内容撑开」的值——
// 实测 contentH=1287 / viewportH=1295 → 恒判定为「不溢出」（need=false）。
// 同理，即便在布局内置了脏标记请求重排，第二轮也会被祖先的增量剪枝拦掉
// （祖先在自己的 MarkCleanWithGeom 里已清掉脏标记）。因此判定移到
// RenderView.Layout 的收尾 pass（rendering.syncScrollbarReserve）：那时
// flex/grid 已定高、几何已同步，用与**滚动条绘制完全相同**的口径
// （boxViewAndContent + needsScrollbars）判定，结论写入盒上的粘性标记
// sbReserved，再补跑一轮布局让预留真正生效（SetVerticalScrollbarReserved
// 内的 MarkDirty 保证补跑那轮不被剪枝）。
//
// 收敛性：预留只会让内容更窄、更高，不会在「需要/不需要」之间来回翻转，
// 因此通常一轮即收敛（收尾 pass 循环上限 3 轮兜底）。
//
// 范围：仅垂直滚动条（垂直书写模式除外）。水平滚动条占高机理相同，
// 但本次未涉及（需要时按同一套 sbReserved 模式扩展）。

// verticalScrollbarReserve 返回本轮布局中该盒应为常驻垂直滚动条预留的内容
// 宽度（= 该盒从自身内容盒中让出给滚动条的宽度）。返回 0 表示不预留。
func (b *ElementBox) verticalScrollbarReserve() float64 {
	if b == nil || b.style == nil {
		return 0
	}
	if IsVerticalWritingMode(b.style) {
		// 垂直书写模式下「垂直滚动条」是块方向的控制件，几何关系不同，
		// 不在此处处理（保持既有行为）。
		return 0
	}
	switch b.style.OverflowY {
	case style.OverflowScroll:
		// 滚动条常驻：无论内容是否溢出都占位。
		return style.ScrollbarWidth(b.style)
	case style.OverflowAuto:
		// 由渲染树收尾 pass 的实测结论决定（首轮为 false，即按不预留布局）。
		if b.sbReserved {
			return style.ScrollbarWidth(b.style)
		}
	}
	return 0
}

// SetVerticalScrollbarReserved 由渲染树的收尾 pass（rendering.syncScrollbarReserve）
// 调用，用「绘制同口径」的判定结果更新粘性标记；返回标记是否发生变化——
// 变化即表示调用方需要补跑一轮布局（预留宽度改变会重排子元素）。
//
// MarkDirty() 让补跑的那轮不被增量剪枝跳过（冒泡到祖先，保证从根链路重排）。
func (b *ElementBox) SetVerticalScrollbarReserved(v bool) bool {
	if b == nil {
		return false
	}
	if b.sbReserved == v {
		return false
	}
	b.sbReserved = v
	b.MarkDirty()
	return true
}
