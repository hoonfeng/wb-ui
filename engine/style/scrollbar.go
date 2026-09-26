package style

// 滚动条宽度常量 —— Chromium/Edge 真实渲染实测值（CSS px）。
//
// ★ 实测依据（2026-09-26 修正版）：以**真实有头 Microsoft Edge 153**（Windows 11，
// devicePixelRatio = 1.25）为基准，用 CDP 直连该实例，在 9090 页面内用
// offsetWidth - clientWidth 读出滚动条的**占位宽**，再用逐像素扫描核对绘制几何：
//
//	容器配置                                                        | 占位宽
//	---------------------------------------------------------------|-------
//	继承 html{scrollbar-color: <thumb> transparent}（页面绝大多数容器）  | 15px
//	显式 scrollbar-color:auto（走 index.html 的 ::-webkit-scrollbar）    |  9px
//	scrollbar-width: thin                                              | 10px
//	scrollbar-width: none                                              |  0px
//
// gou-ide 的 .pp-list 实测 offsetWidth=263 / clientWidth=248 → 差 15px，与首行一致。
//
// ⚠️ 教训（上一版错的根因）：这里的 17px / 11px 是**无头 Chromium**
// （Playwright headless）量出来的。无头内核不走 Chromium 的 scrollbar-color
// 自绘路径，回退到「经典 17px + 大箭头按钮」，与有头浏览器的真实渲染不一致，
// 导致按它实现的 wb-ui 滚动条比浏览器明显更粗、滑块更宽且方形。
// ★ 凡涉及平台控件几何，一律以**有头浏览器实测**为准。
const (
	// ClassicScrollbarWidth 是元素继承到非 auto 的 scrollbar-color 时，
	// Chromium 自绘滚动条的宽度（CSS px，与 devicePixelRatio 无关）。
	ClassicScrollbarWidth = 15.0
	// ThinScrollbarWidth 是 scrollbar-width:thin 的宽度（CSS px）。
	ThinScrollbarWidth = 10.0
)

// ScrollbarWidth 返回该元素滚动条的宽度（CSS px）。
//
// ★ 单一事实来源（2026-09-26）：布局（为常驻滚动条预留内容宽度）与绘制
// （渲染轨道/滑块几何）必须使用同一个宽度值——否则会出现「布局预留了 15px、
// 绘制却画 12px」的错位，表现为内容与滚动条之间的缝隙或重叠。
// engine/rendering/scrollbargeom.go 的 scrollbarWidthFor 与
// engine/layout/scrollbarreserve.go 的预留逻辑都委托到这里。
//
// 规则镜像 Chromium（Windows，有头实测）：
//   - 默认 15px（继承到 scrollbar-color 时的自绘滚动条）；
//   - scrollbar-width: thin   → 10px；
//   - scrollbar-width: none   → 0（仍可滚动，只是不占位、不绘制）；
//   - ::-webkit-scrollbar { width: Npx } 覆盖标准属性（Chrome 中
//     Blink/WebKit 自定义滚动条声明优先于 scrollbar-width）——但**只在元素没有
//     继承到非 auto 的 scrollbar-color 时才生效**（见 HasCustomScrollbarColor）。
func ScrollbarWidth(cs *ComputedStyle) float64 {
	if cs == nil {
		return ClassicScrollbarWidth
	}
	switch cs.GetProperty("scrollbar-width") {
	case "thin":
		return ThinScrollbarWidth
	case "none":
		return 0
	}
	if !HasCustomScrollbarColor(cs) {
		if wv := cs.GetProperty("-webkit-scrollbar-width"); wv != "" {
			if l, ok := parseLength(wv); ok && l.Value > 0 && (l.Unit == "px" || l.Unit == "") {
				return l.Value
			}
		}
	}
	return ClassicScrollbarWidth
}

// HasCustomScrollbarColor 报告元素是否带非 auto 的 scrollbar-color。
//
// ★ Chromium 语义（实测）：只要元素继承到**非 auto** 的 `scrollbar-color`，
// Chromium 就**忽略**该元素上的 ::-webkit-scrollbar 自定义规则，改用其
// scrollbar-color 自绘滚动条：轨道 15px、上下各一个 15px 箭头按钮、滑块宽约
// 8.8px 且为**胶囊圆角**（左右各留 ~3.1px）。
//
// gou-ide 的 index.html 给 html 设了
// `scrollbar-color: var(--scrollbar-thumb, #6e7681) transparent`，而该属性
// **可继承**，因此全页绝大多数滚动容器都走这条路径；只有显式写
// `scrollbar-color: auto` 的组件（StatsRail / AutopilotPanel）才走
// ::-webkit-scrollbar{width:9px/4px} 的细滚动条。
func HasCustomScrollbarColor(cs *ComputedStyle) bool {
	if cs == nil {
		return false
	}
	sc := cs.GetProperty("scrollbar-color")
	return sc != "" && sc != "auto"
}
