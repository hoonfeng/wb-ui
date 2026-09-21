// CSS transform × overflow clip 交互测试。
//
// 回归要点：transform 作用于元素的**整个盒子**（自身 + 自身裁剪 + 后代）。
// 引擎在 paintLayerContents 里先按 layout 绝对坐标设 overflow clip、之后
// 才在 visit() 里施加 canvas translate，导致 clip 停在 layout 矩形上 →
// 平移后的元素被裁掉一半（配置器弹窗只画出 227×229 的根因）。

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// paintWithLayerTree 走与真实页面一致的路径：建图层树 → Paint 遍历层树。
// ★ 必须建图层树：Paint() 只在 view.RootLayer() != nil 时走 paintLayerTree
// （layer 级 overflow 裁剪的实际路径）。不建的话 Paint 退回
// paintSubtreeByPhase，clip 在 transform 之后应用，测不出本回归。
func paintWithLayerTree(t *testing.T, rv *RenderView, canvas *graphics.Canvas, w, h int) {
	t.Helper()
	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	if rootLayer == nil {
		t.Fatal("BuildLayerTree returned nil")
	}
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})
}

// TestTransformWithOverflowClipFollowsTranslate 验证带 overflow:hidden 且
// translate 的元素，其自身的 overflow 裁剪必须一起平移。
func TestTransformWithOverflowClipFollowsTranslate(t *testing.T) {
	canvas := graphics.NewCanvas(200, 200)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 200)

	// 父：布局 (100,100) 80×80，overflow:hidden + translate(-60,-60)
	// → 视觉应出现在 (40,40)-(120,120)。
	pst := style.NewComputedStyle()
	pst.OverflowX = style.OverflowHidden
	pst.OverflowY = style.OverflowHidden
	pst.Transform = "translate(-60px, -60px)"
	parent := NewRenderBox(doc.CreateElement("div"), pst)
	parent.SetLocation(100, 100)
	parent.SetSize(80, 80)

	// 子：绝对 (110,110) 60×60 红（在父内，但越过父右下边界 70px）
	// → 视觉应覆盖 (50,50)-(110,110)，并在父视觉边界 (120,120) 处被裁。
	cst := style.NewComputedStyle()
	cst.BackgroundColor = style.Color{R: 0xFF, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), cst)
	child.SetLocation(110, 110)
	child.SetSize(60, 60)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)

	paintWithLayerTree(t, rv, canvas, 200, 200)

	red := graphics.Color{R: 0xFF, A: 0xFF}

	// ① 平移后父的左上区域：旧的（未平移）clip 是 (100,100)-(180,180)，
	//    点 (70,70) 落在此之外 → 修复前必然被误裁成透明。
	if got := canvas.PixelAt(70, 70); got != red {
		t.Errorf("(70,70) = %+v, want red —— 裁剪区没有跟随 translate 平移（被裁回 layout 矩形）", got)
	}
	// ② 平移后父的中心，落在旧 clip 内 → 修复前后都应可见（对照点）。
	if got := canvas.PixelAt(105, 105); got != red {
		t.Errorf("(105,105) = %+v, want red —— 平移后的内容本身没画出来", got)
	}
	// ③ 越过父视觉边界 (120,120) 之后必须仍被掏裁掉（overflow 语义不能丢）。
	if got := canvas.PixelAt(125, 125); got != (graphics.Color{}) {
		t.Errorf("(125,125) = %+v, want transparent —— overflow 裁剪失效", got)
	}
}

// TestAncestorOverflowClipStaysFixedUnderChildTransform：carousel 场景——
// 祖先 `overflow:hidden`（自身无 transform）下的子层带 `transform:translate`。
// 祖先的裁剪区属于祖先坐标系，子层被平移【不会】移动它。
//
// 回归：paintLayerContents 曾把「祖先裁剪 ∩ 自身裁剪」的【累计】clip 整体按
// 本层 transform 变换 → 子层的 clip 被一起平移（-100px），与祖先已在画布上
// 设置的裁剪求交后缩成空 → 整个轨道（本该显示落在视口内的 slide）画成空白。
// cssprobe 的 carousel-clip 夹具即此形态（`.viewport{overflow:hidden}` 下
// `.track{transform:translate(-300px,0)}`）。
func TestAncestorOverflowClipStaysFixedUnderChildTransform(t *testing.T) {
	canvas := graphics.NewCanvas(200, 100)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 100)

	// 祖先（视口）：layout (0,0) 100×50，overflow:hidden，无 transform。
	pst := style.NewComputedStyle()
	pst.OverflowX = style.OverflowHidden
	pst.OverflowY = style.OverflowHidden
	parent := NewRenderBox(doc.CreateElement("div"), pst)
	parent.SetLocation(0, 0)
	parent.SetSize(100, 50)

	// 轨道：layout (0,0) 300×50，translate(-100px,0)，**自身 overflow visible**
	// （只有祖先裁剪参与）→ 视觉覆盖 (-100,0)-(200,50)。
	tst := style.NewComputedStyle()
	tst.Transform = "translate(-100px, 0)"
	track := NewRenderBox(doc.CreateElement("div"), tst)
	track.SetLocation(0, 0)
	track.SetSize(300, 50)

	// 绿色 slide：layout (100,0) 100×50 → 平移后覆盖 (0,0)-(100,50)，
	// 正好铺满祖先视口。
	cst := style.NewComputedStyle()
	cst.BackgroundColor = style.Color{G: 0xFF, A: 0xFF}
	slide := NewRenderBox(doc.CreateElement("div"), cst)
	slide.SetLocation(100, 0)
	slide.SetSize(100, 50)

	rv.AddChild(parent, nil)
	parent.AddChild(track, nil)
	track.AddChild(slide, nil)

	paintWithLayerTree(t, rv, canvas, 200, 100)

	green := graphics.Color{G: 0xFF, A: 0xFF}
	// ① 视口中心（平移后 slide 的位置）必须可见——修复前此处透明。
	if got := canvas.PixelAt(50, 25); got != green {
		t.Errorf("(50,25) = %+v, want green —— 祖先 overflow 裁剪被子层 transform 带偏", got)
	}
	// ② 视口右缘内侧仍可见（修复前 clip 被平移后只剩极窄一条）。
	if got := canvas.PixelAt(90, 25); got != green {
		t.Errorf("(90,25) = %+v, want green —— 视口内右半缺失", got)
	}
	// ③ 视口之外（x=150）必须透明：祖先裁剪不能被子层平移"带走"。
	if got := canvas.PixelAt(150, 25); got != (graphics.Color{}) {
		t.Errorf("(150,25) = %+v, want transparent —— 祖先 overflow 裁剪失效", got)
	}
	// ④ 纵向越界（y=60 > 视口高 50）同样必须被祖先裁剪掉。
	if got := canvas.PixelAt(50, 60); got != (graphics.Color{}) {
		t.Errorf("(50,60) = %+v, want transparent —— 纵向越界内容漏出", got)
	}
}

// TestTransformTranslatePercentUsesBoxSize 回归 translate(%) 的参照尺寸：
// CSS 百分比以元素自身的 border box 为基准。
func TestTransformTranslatePercentUsesBoxSize(t *testing.T) {
	canvas := graphics.NewCanvas(200, 200)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 200)

	// 100×50 的块，translate(-100%,-100%) → 应移动到 (0,0)-(100,50)。
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{G: 0xFF, A: 0xFF}
	st.Transform = "translate(-100%, -100%)"
	box := NewRenderBox(doc.CreateElement("div"), st)
	box.SetLocation(100, 50)
	box.SetSize(100, 50)

	rv.AddChild(box, nil)
	paintWithLayerTree(t, rv, canvas, 200, 200)

	green := graphics.Color{G: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(10, 10); got != green {
		t.Errorf("(10,10) = %+v, want green —— translate(-100%%) 未按 100×50 解析", got)
	}
	if got := canvas.PixelAt(110, 60); got != (graphics.Color{}) {
		t.Errorf("(110,60) = %+v, want transparent —— 元素未真正移走", got)
	}
}

// ---------------------------------------------------------------------------
// rotate / scale / skew：裁剪区必须跟着变换成【非轴对齐】形状。
//
// 这三条测试是 TestTransformWithOverflowClipFollowsTranslate 的一般化：
// 早期实现只处理「纯平移」（把 clip 矩形挪一挪），遇到 rotate/scale/skew
// 就放弃（clip 停在 layout 矩形）。现在的做法是在元素自身变换后的空间里
// 设置裁剪（Canvas.PushMatrix + clip + 还原 CTM），所以下面的断言点全部
// 落在「视觉裁剪区内、layout 矩形外」——旧实现在这些点上必然透明。
// ---------------------------------------------------------------------------

// TestTransformWithOverflowClipFollowsRotate：rotate(90deg) 把 100×40 的
// 横条转成 40×100 的竖条，裁剪区必须一起旋转（而不是留在横条位置）。
func TestTransformWithOverflowClipFollowsRotate(t *testing.T) {
	canvas := graphics.NewCanvas(300, 300)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(300, 300)

	// 父：layout (100,100) 100×40，overflow:hidden，rotate(90deg)。
	// transform-origin 默认 50% 50% = (150,120) → 视觉是 (130,70)-(170,170)
	// 的竖条。
	pst := style.NewComputedStyle()
	pst.OverflowX = style.OverflowHidden
	pst.OverflowY = style.OverflowHidden
	pst.Transform = "rotate(90deg)"
	parent := NewRenderBox(doc.CreateElement("div"), pst)
	parent.SetLocation(100, 100)
	parent.SetSize(100, 40)

	// 子：填满父的盒子，红。
	cst := style.NewComputedStyle()
	cst.BackgroundColor = style.Color{R: 0xFF, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), cst)
	child.SetLocation(100, 100)
	child.SetSize(100, 40)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)
	paintWithLayerTree(t, rv, canvas, 300, 300)

	red := graphics.Color{R: 0xFF, A: 0xFF}
	// ① 旋转后的竖条上半部（y < 100，在 layout 矩形之外）：
	//    旧行为里 clip 仍是 layout 横条 (100,100)-(200,140) → 这一点被裁掉。
	if got := canvas.PixelAt(150, 80); got != red {
		t.Errorf("(150,80) = %+v, want red —— 裁剪区没有跟着 rotate 旋转（仍是 layout 横条）", got)
	}
	// ② 竖条下半部（y > 140），同理必须在旋转后的裁剪区内。
	if got := canvas.PixelAt(150, 160); got != red {
		t.Errorf("(150,160) = %+v, want red —— 旋转后裁剪区未覆盖下半部", got)
	}
	// ③ 竖条之外（y > 170）必须仍被裁掉：旋转不能把 overflow 语义弄丢。
	if got := canvas.PixelAt(150, 180); got != (graphics.Color{}) {
		t.Errorf("(150,180) = %+v, want transparent —— overlay 裁剪放宽过头", got)
	}
	// ④ 横条原位置（x 远大于竖条右边界 170）必须透明。
	if got := canvas.PixelAt(190, 120); got != (graphics.Color{}) {
		t.Errorf("(190,120) = %+v, want transparent —— 旧 layout 横条位置漏出内容", got)
	}
}

// TestTransformWithOverflowClipFollowsScale：scale(2) 绕中心把 60×60 放大成
// 120×120，裁剪区必须一起放大。
func TestTransformWithOverflowClipFollowsScale(t *testing.T) {
	canvas := graphics.NewCanvas(300, 300)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(300, 300)

	// 父：layout (100,100) 60×60，overflow:hidden，scale(2)。
	// origin = (130,130) → 视觉 (70,70)-(190,190)。
	pst := style.NewComputedStyle()
	pst.OverflowX = style.OverflowHidden
	pst.OverflowY = style.OverflowHidden
	pst.Transform = "scale(2)"
	parent := NewRenderBox(doc.CreateElement("div"), pst)
	parent.SetLocation(100, 100)
	parent.SetSize(60, 60)

	cst := style.NewComputedStyle()
	cst.BackgroundColor = style.Color{G: 0xFF, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), cst)
	child.SetLocation(100, 100)
	child.SetSize(60, 60)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)
	paintWithLayerTree(t, rv, canvas, 300, 300)

	green := graphics.Color{G: 0xFF, A: 0xFF}
	// ① 放大后溢出 layout 矩形的左上角（x,y < 100）→ 旧行为被裁掉。
	if got := canvas.PixelAt(80, 80); got != green {
		t.Errorf("(80,80) = %+v, want green —— 裁剪区没有跟着 scale 放大", got)
	}
	// ② 右下角同理（> 160）。
	if got := canvas.PixelAt(180, 180); got != green {
		t.Errorf("(180,180) = %+v, want green —— 放大后的裁剪区未覆盖右下", got)
	}
	// ③ 视觉边界 (70,70)-(190,190) 之外必须透明。
	if got := canvas.PixelAt(60, 130); got != (graphics.Color{}) {
		t.Errorf("(60,130) = %+v, want transparent —— scale 后的裁剪放宽过头", got)
	}
}

// TestTransformWithOverflowClipFollowsSkew：skewX(45deg) 把矩形切成平行四边形，
// 裁剪区必须是同一个平行四边形（这是最不可能靠「轴对齐矩形」蒙对的情形）。
func TestTransformWithOverflowClipFollowsSkew(t *testing.T) {
	canvas := graphics.NewCanvas(320, 260)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(320, 260)

	// 父：layout (100,100) 100×40，overflow:hidden，skewX(45deg)。
	// origin = (150,120)，tan45 = 1 → x' = x + (y-120)：
	//   上边 y=100：x 从 80 到 180
	//   下边 y=140：x 从 120 到 220
	pst := style.NewComputedStyle()
	pst.OverflowX = style.OverflowHidden
	pst.OverflowY = style.OverflowHidden
	pst.Transform = "skewX(45deg)"
	parent := NewRenderBox(doc.CreateElement("div"), pst)
	parent.SetLocation(100, 100)
	parent.SetSize(100, 40)

	cst := style.NewComputedStyle()
	cst.BackgroundColor = style.Color{B: 0xFF, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), cst)
	child.SetLocation(100, 100)
	child.SetSize(100, 40)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)
	paintWithLayerTree(t, rv, canvas, 320, 260)

	blue := graphics.Color{B: 0xFF, A: 0xFF}
	// ① 斜切后左上角伸出 layout 矩形（x=95 < 100，y=105 → 左边界 85）
	//    → 旧轴对齐裁剪会把这块切掉。
	if got := canvas.PixelAt(95, 105); got != blue {
		t.Errorf("(95,105) = %+v, want blue —— 裁剪区没有跟着 skewX 斜切", got)
	}
	// ② 斜切后右下角伸出（y=135 处右边界 215）→ x=205 在内部。
	if got := canvas.PixelAt(205, 135); got != blue {
		t.Errorf("(205,135) = %+v, want blue —— 斜切后的裁剪区未覆盖右下角", got)
	}
	// ③ 平行四边形之外必须仍被裁掉（y=105 处右边界 185 → 200 在外部）。
	if got := canvas.PixelAt(200, 105); got != (graphics.Color{}) {
		t.Errorf("(200,105) = %+v, want transparent —— 斜切裁剪放宽过头", got)
	}
	// ④ 上边界之外（y=90 < 100）必须透明，无论斜切把 x 推到哪。
	if got := canvas.PixelAt(110, 90); got != (graphics.Color{}) {
		t.Errorf("(110,90) = %+v, want transparent —— 斜切后纵向越界内容漏出", got)
	}
}
