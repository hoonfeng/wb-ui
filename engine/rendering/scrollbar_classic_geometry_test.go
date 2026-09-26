package rendering_test

import (
	"math"
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

// classicScrollbarHTML 复刻 gou-ide 的真实滚动条配置：html 上设
// `scrollbar-color: <thumb> transparent`（index.html 的原样写法），列表继承它。
// 该值非 auto → Chromium 忽略 ::-webkit-scrollbar，改用自绘滚动条。
const classicScrollbarHTML = `<!DOCTYPE html><html><head><style>
	html, body { margin: 0; padding: 0; }
	html { scrollbar-color: #414b64 transparent; }
	#list { width: 200px; height: 100px; overflow-y: auto; background: #ffffff; }
	#list > div { height: 40px; }
</style></head><body>
	<div id="list"><div></div><div></div><div></div><div></div><div></div></div>
</body></html>`

// TestClassicScrollbarGeometryMatchesChrome 把「scrollbar-color 自绘滚动条」的
// 几何钉在**真实有头 Edge 153** 的实测值上（2026-09-26 用 CDP 直连该实例，
// 在 9090 页面内对 .pp-list 逐像素量出，见 engine/style/scrollbar.go 顶部对照表）：
//
//	轨道宽         15px
//	箭头按钮高     15px（= 轨道宽），按钮与 thumb 之间 **gap = 3.1px**
//	thumb 宽       8.8px（= 15 - 2×3.1，左右各留 3.1px），**胶囊圆角**（半径 = 半宽）
//	箭头三角       6.4 宽 × 5.6 高，与 thumb **同色**（跟随 scrollbar-color 的 thumb 色）
//
// ⚠️ 旧实现（17px 宽 + 14px 直角 thumb）是按**无头** Chromium 的经典滚动条写的，
// 比真实浏览器明显更粗更方——这是用户反馈「滚动条样式不一样」的直接原因。
func TestClassicScrollbarGeometryMatchesChrome(t *testing.T) {
	rv, find := layoutFixture(t, classicScrollbarHTML, 400, 300)
	list := find("list")
	st := list.Style()

	// 前提：scrollbar-color 从 html 继承到了 #list（可继承属性必须下发）。
	if !style.HasCustomScrollbarColor(st) {
		t.Fatalf("scrollbar-color 未继承到 #list（got %q）", st.GetProperty("scrollbar-color"))
	}
	scrollW := style.ScrollbarWidth(st)
	if scrollW != style.ClassicScrollbarWidth {
		t.Fatalf("ScrollbarWidth = %v, want %v", scrollW, style.ClassicScrollbarWidth)
	}

	pb := list.PaddingBoxRect()
	m := rendering.VerticalScrollbarMetrics(rv, list)
	if !m.OK {
		t.Fatal("内容溢出应产生垂直滚动条")
	}
	// ★ 这一条同时证明了「走自绘样式」：箭头按钮 + thumb inset →
	// 轨道 = padding box 高 - 2×(15 + 3.1)。若误走 ::-webkit-scrollbar 分支
	// （无箭头、thumb 铺满），TrackLen 会等于 pb.Height。
	const thumbInset = 3.0 // = rendering 包内 sbThumbInset（有头 Edge 实测值）
	if want := pb.Height - 2*(scrollW+thumbInset); math.Abs(m.TrackLen-want) > 0.5 {
		t.Fatalf("TrackLen = %v, want %v（箭头按钮 15px + thumb inset 3.1px）", m.TrackLen, want)
	}

	canvas := graphics.NewCanvas(400, 300)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 400, Height: 300})

	isThumb := func(x, y int) bool {
		p := canvas.PixelAt(x, y)
		return p.R == 65 && p.G == 75 && p.B == 100
	}
	thumbSpan := func(y int) (int, int, bool) {
		lo, hi := -1, -1
		for x := int(pb.X); x < int(pb.X+pb.Width); x++ {
			if isThumb(x, y) {
				if lo < 0 {
					lo = x
				}
				hi = x
			}
		}
		return lo, hi, lo >= 0
	}

	thumbTop := int(pb.Y + scrollW + thumbInset) // 箭头按钮下方再留 thumb inset
	midY := thumbTop + int(m.ThumbLen/2)
	lo, hi, ok := thumbSpan(midY)
	if !ok {
		t.Fatalf("thumb 未绘制（y=%d）", midY)
	}
	if got, want := hi-lo+1, int(scrollW-2*thumbInset); math.Abs(float64(got-want)) > 1 {
		t.Fatalf("thumb 宽 = %d, want ≈%d（15 - 2×3.1）", got, want)
	}
	// 右侧：thumb 与滚动条右缘之间留 ≈3.1px（即用户说的「内容与滚动条之间的间距」）。
	// 纯色像素会比几何右缘短约 1px（末列落在抗锯齿层），因此容差放宽到 2..5。
	if right := int(pb.X + pb.Width); hi < right-5 || hi > right-2 {
		t.Fatalf("thumb 右缘（纯色 x=%d）与滚动条右缘 x=%d 之间的留白不是约 3.1px", hi, right)
	}
	// 胶囊圆角：thumb 首行应比中段**窄**（有头 Edge 实测 thumb 顶端数行由窄到宽；
	// 旧实现是直角，逐行等宽）。
	if lo2, hi2, ok2 := thumbSpan(thumbTop + 1); ok2 && hi2-lo2 >= hi-lo {
		t.Fatalf("thumb 顶部宽 %d 未收窄 —— 自绘滚动条 thumb 应为胶囊圆角", hi2-lo2+1)
	}

	// 上箭头三角：按钮中心行、中心列应有 thumb 色像素（箭头跟随 thumb 色）。
	btnCX := int(pb.X + pb.Width - scrollW/2)
	btnCY := int(pb.Y + scrollW/2)
	if !isThumb(btnCX, btnCY) {
		p := canvas.PixelAt(btnCX, btnCY)
		t.Fatalf("上箭头按钮中心 (%d,%d) = #%02x%02x%02x，应为 thumb 色 #414b64（箭头三角跟随 thumb 色）",
			btnCX, btnCY, p.R, p.G, p.B)
	}
}

// TestWebkitScrollbarSurvivesWhenColorAuto：#list 显式 `scrollbar-color:auto`
// 时（gou-ide 的 StatsRail/AutopilotPanel 就是这种），::-webkit-scrollbar 的
// 自定义宽度才生效 —— 滚动条变细、无箭头（TrackLen = pb.Height）。
func TestWebkitScrollbarSurvivesWhenColorAuto(t *testing.T) {
	const page = `<!DOCTYPE html><html><head><style>
		html, body { margin: 0; padding: 0; }
		html { scrollbar-color: #414b64 transparent; }
		#list { width: 200px; height: 100px; overflow-y: auto; background: #ffffff;
		        scrollbar-color: auto; }
		#list::-webkit-scrollbar { width: 9px; }
		#list > div { height: 40px; }
	</style></head><body>
		<div id="list"><div></div><div></div><div></div><div></div><div></div></div>
	</body></html>`
	rv, find := layoutFixture(t, page, 400, 300)
	list := find("list")
	st := list.Style()
	if style.HasCustomScrollbarColor(st) {
		t.Fatalf("#list 显式 scrollbar-color:auto 不应被判定为自定义配色")
	}
	if got := style.ScrollbarWidth(st); got != 9 {
		t.Fatalf("ScrollbarWidth = %v, want 9（scrollbar-color:auto 放行 ::-webkit-scrollbar{width:9px}）", got)
	}
	pb := list.PaddingBoxRect()
	if m := rendering.VerticalScrollbarMetrics(rv, list); !m.OK {
		t.Fatal("应产生滚动条")
	} else if math.Abs(m.TrackLen-pb.Height) > 0.5 {
		t.Fatalf("TrackLen = %v, want %v（webkit 自定义滚动条无箭头按钮）", m.TrackLen, pb.Height)
	}
}
