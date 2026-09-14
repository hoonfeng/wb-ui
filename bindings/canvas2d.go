// bindings/canvas2d.go — <canvas> 元素的 CanvasRenderingContext2D 完整实现。
//
// 背景：此前 applyCanvas2DPatch 只为 xterm 提供 measureText 哑实现，canvas
// 无任何绘制能力。本文件实现浏览器 canvas 2D 全 API 的实用子集：
// JS 侧 getContext('2d') 返回的对象每个方法都是 Go native function，直接
// 绘制到 canvas 元素的离屏位图（rendering.CanvasBitmap → graphics.Canvas，
// 底层 Skia）；渲染管线 PaintCanvas 在页面重绘时把位图 Blit 到内容盒。
//
// 状态模型：
//   - 样式属性（fillStyle/strokeStyle/lineWidth/font/globalAlpha/...）直接
//     存在 JS 对象（goja）上；每次绘制时 Go 侧从 this 读取解析（浏览器
//     semantics：读-写都是普通属性）。
//   - save()/restore()：transform + clip 由 graphics.Canvas.Save/Restore
//     处理（skia 原生状态栈）；JS 样式属性由 Go 侧快照/回写。
//   - path 累积在 Go 侧（*skia.Path），不受 save/restore 影响（浏览器语义）。
//
// 已知限制（v1）：
//   - shadow* 属性可读写但不参与渲染（live2d/pixi 等不用 shadow）；
//   - arcTo / createPattern / isPointInPath 未实现（返回宽泛结果）；
//   - radial 渐变 r0>0 近似为 r0=0 的 Skia 径向渐变；
//   - putImageData 的 dirty 矩形参数按整幅处理。

package bindings

import (
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

// canvas 元素默认位图尺寸（浏览器 spec：300×150）。
const (
	canvasDefaultW = 300
	canvasDefaultH = 150
)

// canvasImageSourceHook 由 webkit 包在初始化时注入：把 <img> 元素映射到其
// 解码位图（渲染树 RenderBox.DecodedImage().SkiaImage()）。放在 bindings
// 包级避免 bindings→渲染树 遍历依赖（与 IFrameLookup 同模式）。
var canvasImageSourceHook func(el *dom.Element) *graphics.SkiaImage

// SetCanvasImageSourceHook 注入 <img> 元素 → 解码位图 的解析函数
//（webkit.NewWebView 初始化时调用）。
func SetCanvasImageSourceHook(fn func(el *dom.Element) *graphics.SkiaImage) {
	canvasImageSourceHook = fn
}

// canvasCtxCache 缓存每个 canvas 元素的 2D 上下文（getContext 幂等：同一
// 元素多次 getContext('2d') 返回同一对象，浏览器语义）。
var canvasCtxCache sync.Map // *dom.Element → *jsc.JSObject

// canvasSizeOf 读取 canvas width/height 属性（无则 300×150 默认）。
func canvasSizeOf(el *dom.Element) (int, int) {
	w, h := canvasDefaultW, canvasDefaultH
	if s := el.GetAttribute("width"); s != "" {
		if v, ok := parseAttrInt(s); ok {
			w = v
		}
	}
	if s := el.GetAttribute("height"); s != "" {
		if v, ok := parseAttrInt(s); ok {
			h = v
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// parseAttrInt 解析非负整数属性（canvas width/height）。
func parseAttrInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
		if v > 1<<20 {
			return 0, false
		}
	}
	return v, true
}

// ensureCanvasBitmap 返回（必要时创建）canvas 元素的离屏位图。
func ensureCanvasBitmap(el *dom.Element, w, h int) *rendering.CanvasBitmap {
	if bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap); ok && bm != nil {
		if bm.W == w && bm.H == h {
			return bm
		}
		bm.Resize(w, h)
		return bm
	}
	bm := rendering.NewCanvasBitmap(w, h)
	el.SetCanvasSurface(bm)
	return bm
}

// resizeCanvasBitmap 按新尺寸重建位图（el.width/height 属性变化时；
// 位图未创建则仅更新 attribute，getContext 时生效）。
func resizeCanvasBitmap(el *dom.Element) {
	if bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap); ok && bm != nil {
		w, h := canvasSizeOf(el)
		if bm.W != w || bm.H != h {
			bm.Resize(w, h)
		}
	}
}

// imgPixelDim 返回 <img> 元素的解码图尺寸（宽=width=true，高=height=false）。
// 经 canvasImageSourceHook（webkit 桥）按 src 解码；未解码且 hook 不可用时
// 返回 0。canvas 2D drawImage 的纹理尺寸查询（img.width/height）依赖。
func imgPixelDim(el *dom.Element, width bool) float64 {
	if canvasImageSourceHook == nil || el == nil {
		return 0
	}
	img := canvasImageSourceHook(el)
	if img == nil {
		return 0
	}
	if width {
		return float64(img.Width())
	}
	return float64(img.Height())
}

// canvas2DGetContext 构建（或取缓存）<canvas> 元素的 2D 上下文。
func canvas2DGetContext(rt *jsc.Interpreter, el *dom.Element) jsc.JSValue {
	if el == nil {
		return jsc.Null()
	}
	if cached, ok := canvasCtxCache.Load(el); ok {
		if obj, ok := cached.(*jsc.JSObject); ok && obj != nil {
			return jsc.ObjectValue(obj)
		}
	}
	w, h := canvasSizeOf(el)
	bm := ensureCanvasBitmap(el, w, h)
	ctxObj := buildCanvas2DCtx(rt, el, bm)
	canvasCtxCache.Store(el, ctxObj)
	return jsc.ObjectValue(ctxObj)
}

// canvas2DCtxState 是 ctx 的 Go 侧状态（闭包捕获，不暴露给 JS）。
type canvas2DCtxState struct {
	el   *dom.Element
	bm   *rendering.CanvasBitmap
	path *skia.Path // 当前路径（beginPath 重建）
	// dash 是 setLineDash 设置的虚线阵列（配合 lineDashOffset 参与描边绘制）。
	dash []float32
	// geom 是路径的几何副本（自维护：skia 的 Path 不可反查点集），供
	// isPointInPath / isPointInStroke 做精确几何判定。
	geom    []geomSubpath
	geomCur int // 当前子路径索引（-1 = 无）
	saved   []canvas2DSaved
}

// geomSubpath 是一条子路径的采样点（直线段的原点 + 曲线/圆弧的细分点）。
type geomSubpath struct {
	pts    []geomPoint
	closed bool
}

// canvas2DSaved 是 save() 时快照的 JS 样式属性。
type canvas2DSaved struct {
	fillStyle  jsc.JSValue
	strokeStyle jsc.JSValue
	lineWidth  float64
	miterLimit float64
	globalAlpha float64
	shadowBlur float64
	shadowOffsetX float64
	shadowOffsetY float64
	lineDashOffset float64
	lineCap   string
	lineJoin  string
	font      string
	textAlign string
	textBaseline string
	globalCompositeOperation string
	shadowColor string
	lineDash []float32
}

// ─── JS 属性读取 helpers ────────────────────────────────────────────

func c2dNum(o *jsc.JSObject, key string, def float64) float64 {
	if v, ok := o.GetByKey(key); ok && !v.IsUndefined() && !v.IsNull() {
		return v.ToNumber()
	}
	return def
}

func c2dStr(o *jsc.JSObject, key, def string) string {
	if v, ok := o.GetByKey(key); ok && !v.IsUndefined() && !v.IsNull() {
		return v.ToString()
	}
	return def
}

// ─── 颜色 / 渐变 / blend ────────────────────────────────────────────

// c2dColor 把 canvas 颜色字符串转为 graphics.Color（解析失败 → 黑色，
// 浏览器 canvas 对非法颜色值回退黑色）。
func c2dColor(s string) graphics.Color {
	c, ok := style.ParseColorValue(s)
	if !ok {
		return graphics.Color{R: 0, G: 0, B: 0, A: 255}
	}
	return graphics.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// c2dBlend 把 globalCompositeOperation 关键字映射到 skia BlendMode。
func c2dBlend(s string) skia.BlendMode {
	switch s {
	case "source-in":
		return skia.BlendModeSrcIn
	case "source-out":
		return skia.BlendModeSrcOut
	case "source-atop":
		return skia.BlendModeSrcATop
	case "destination-over":
		return skia.BlendModeDstOver
	case "destination-in":
		return skia.BlendModeDstIn
	case "destination-out":
		return skia.BlendModeDstOut
	case "destination-atop":
		return skia.BlendModeDstATop
	case "xor":
		return skia.BlendModeXor
	case "lighter":
		return skia.BlendModePlus
	case "copy":
		return skia.BlendModeSrc
	case "multiply":
		return skia.BlendModeMultiply
	case "screen":
		return skia.BlendModeScreen
	case "overlay":
		return skia.BlendModeOverlay
	case "darken":
		return skia.BlendModeDarken
	case "lighten":
		return skia.BlendModeLighten
	case "color-dodge":
		return skia.BlendModeColorDodge
	case "color-burn":
		return skia.BlendModeColorBurn
	case "hard-light":
		return skia.BlendModeHardLight
	case "soft-light":
		return skia.BlendModeSoftLight
	case "difference":
		return skia.BlendModeDifference
	case "exclusion":
		return skia.BlendModeExclusion
	case "hue":
		return skia.BlendModeHue
	case "saturation":
		return skia.BlendModeSaturation
	case "color":
		return skia.BlendModeColor
	case "luminosity":
		return skia.BlendModeLuminosity
	}
	return skia.BlendModeSrcOver
}

// canvasGradientStop 是渐变的一个 stop。
type canvasGradientStop struct {
	off   float64
	color graphics.Color
}

// buildGradientShader 从 JS 渐变对象（__wbGradient + __wbCoords + __wbStops）
// 重建 skia shader。调用方负责 Release。
func buildGradientShader(o *jsc.JSObject) *skia.Shader {
	kind, _ := o.GetByKey("__wbGradient")
	kindStr := ""
	if !kind.IsUndefined() && !kind.IsNull() {
		kindStr = kind.ToString()
	}
	coords, _ := o.GetByKey("__wbCoords")
	var nums []float64
	if !coords.IsUndefined() && !coords.IsNull() {
		for i := 0; ; i++ {
			v, ok := coords.AsObject().GetByKey(strconv.Itoa(i))
			if !ok {
				break
			}
			nums = append(nums, v.ToNumber())
		}
	}
	stops, _ := o.GetByKey("__wbStops")
	stopsArr := []canvasGradientStop{}
	if !stops.IsUndefined() && !stops.IsNull() {
		so := stops.AsObject()
		lv, _ := so.GetByKey("length")
		n := int(lv.ToNumber())
		for i := 0; i < n; i++ {
			v, ok := so.GetByKey(strconv.Itoa(i))
			if !ok {
				continue
			}
			item := v.AsObject()
			offV, _ := item.GetByKey("off")
			colV, _ := item.GetByKey("color")
			if colV.IsString() {
				stopsArr = append(stopsArr, canvasGradientStop{
					off:   offV.ToNumber(),
					color: c2dColor(colV.ToString()),
				})
			}
		}
	}
	// 渐变需 ≥1 stop；浏览器若 <1 stop 用 rgba(0,0,0,0)。
	if len(stopsArr) == 0 {
		stopsArr = []canvasGradientStop{{off: 0, color: graphics.Color{A: 0}}, {off: 1, color: graphics.Color{A: 0}}}
	}
	// 补全首尾 stop（浏览器语义：偏移 <0 处的最近 stop 延伸到 0）。
	if stopsArr[0].off > 0 {
		stopsArr = append([]canvasGradientStop{{off: 0, color: stopsArr[0].color}}, stopsArr...)
	}
	if stopsArr[len(stopsArr)-1].off < 1 {
		stopsArr = append(stopsArr, canvasGradientStop{off: 1, color: stopsArr[len(stopsArr)-1].color})
	}
	// 按 offset 排序，相邻同 offset 合并（保留后者）。
	sort.SliceStable(stopsArr, func(i, j int) bool { return stopsArr[i].off < stopsArr[j].off })
	merged := stopsArr[:0]
	for _, s := range stopsArr {
		if len(merged) > 0 && merged[len(merged)-1].off == s.off {
			merged[len(merged)-1] = s
		} else {
			merged = append(merged, s)
		}
	}
	stopsArr = merged

	colors := make([]skia.Color, len(stopsArr))
	positions := make([]float32, len(stopsArr))
	for i, s := range stopsArr {
		colors[i] = skia.RGBA(s.color.R, s.color.G, s.color.B, s.color.A)
		positions[i] = float32(s.off)
	}
	if kindStr == "radial" {
		if len(nums) >= 5 {
			cx, cy, r := nums[0], nums[1], nums[3]
			_ = nums[2] // r0：近似为圆心渐变（r0=0）
			if r <= 0 {
				r = nums[3]
			}
			return skia.NewRadialGradient(
				skia.Point{X: float32(cx), Y: float32(cy)}, float32(r),
				colors, positions, skia.TileModeClamp)
		}
		return skia.NewRadialGradient(skia.Point{}, 1, colors, positions, skia.TileModeClamp)
	}
	if len(nums) >= 4 {
		return skia.NewLinearGradient(
			skia.Point{X: float32(nums[0]), Y: float32(nums[1])},
			skia.Point{X: float32(nums[2]), Y: float32(nums[3])},
			colors, positions, skia.TileModeClamp)
	}
	return nil
}

// resolveFill 解析 this 的 fillStyle/strokeStyle 属性 → (颜色, shader, owns)。
// shader 非 nil 时：owns=true 表示调用方负责 Release（渐变 shader 每次新建）；
// owns=false 表示 shader 由图案注册表长期持有，**不可** Release。
func resolveStyle(o *jsc.JSObject, key string) (graphics.Color, *skia.Shader, bool) {
	v, ok := o.GetByKey(key)
	if !ok || v.IsUndefined() || v.IsNull() {
		return graphics.Color{R: 0, G: 0, B: 0, A: 255}, nil, false
	}
	if v.IsString() {
		return c2dColor(v.ToString()), nil, false
	}
	if obj := v.AsObject(); obj != nil {
		// CanvasPattern（createPattern 产物）
		if pv, ok2 := obj.GetByKey("__wbPattern"); ok2 && pv.ToBoolean() {
			if idV, ok3 := obj.GetByKey("__wbShaderId"); ok3 {
				if sh := patternShaderFor(int(idV.ToNumber())); sh != nil {
					return graphics.Color{A: 255}, sh, false
				}
			}
			return graphics.Color{A: 255}, nil, false
		}
		if gv, ok2 := obj.GetByKey("__wbGradient"); ok2 {
			if gs := gv.ToString(); gs != "" && gs != "undefined" {
				return graphics.Color{A: 255}, buildGradientShader(obj), true
			}
		}
	}
	return graphics.Color{R: 0, G: 0, B: 0, A: 255}, nil, false
}

// ─── CanvasPattern 注册表 ────────────────────────────────────────────
//
// createPattern 生成的 Skia shader 存在这里：shader 对源 image 持引用，因此
// 图案的生命周期与源位图解耦（img 的 src 变化释放解码缓存也不会让图案悬空）。
// 注册表不清理——CanvasPattern 没有显式销毁接口、goja 对象也没有 finalizer，
// 而一个页面创建的图案数量有限（通常个位数）。
var (
	patternMu      sync.Mutex
	patternNextID  int
	patternShaders = map[int]*skia.Shader{}
)

func registerPatternShader(sh *skia.Shader) int {
	patternMu.Lock()
	defer patternMu.Unlock()
	patternNextID++
	patternShaders[patternNextID] = sh
	return patternNextID
}

func patternShaderFor(id int) *skia.Shader {
	patternMu.Lock()
	defer patternMu.Unlock()
	return patternShaders[id]
}

// c2dEffect 读取 ctx 的阴影（shadowColor/shadowBlur/shadowOffsetX/Y）与虚线
// （setLineDash + lineDashOffset）状态，交给 graphics 的效果化绘制方法。
func c2dEffect(o *jsc.JSObject, dash []float32, dashPhase float64) graphics.CanvasEffect {
	eff := graphics.CanvasEffect{DashPhase: float32(dashPhase)}
	if len(dash) >= 2 {
		eff.DashIntervals = dash
	}
	col := c2dColor(c2dStr(o, "shadowColor", "rgba(0, 0, 0, 0)"))
	blur := c2dNum(o, "shadowBlur", 0)
	dx := c2dNum(o, "shadowOffsetX", 0)
	dy := c2dNum(o, "shadowOffsetY", 0)
	if col.A > 0 && (blur > 0 || dx != 0 || dy != 0) {
		eff.ShadowColor = col
		eff.ShadowAlpha = 1 // 颜色自带 alpha（c2dColor 已解析 rgba 的 a 分量）
		eff.ShadowDX, eff.ShadowDY = dx, dy
		if blur > 0 {
			eff.ShadowSigma = blur / 2 // Skia 的 sigma ≈ 模糊半径的一半
		}
	}
	return eff
}

// ctxEffect 是 c2dEffect 的常用入口（从 ctx 对象 + Go 侧状态取效果）。
func ctxEffect(o *jsc.JSObject, s *canvas2DCtxState) graphics.CanvasEffect {
	if s == nil || o == nil {
		return graphics.CanvasEffect{}
	}
	return c2dEffect(o, s.dash, c2dNum(o, "lineDashOffset", 0))
}

// ─── 路径几何副本（isPointInPath / isPointInStroke）──────────────────
//
// skia 的 Path 只进不出（无法反查点集），因此路径构建时同步维护一份采样几何：
// 直线记端点、曲线与圆弧按足够密的步长细分。判定在用户空间进行——路径与查询
// 点都在同一用户空间，无需逆变换（与 canvas 2D 规范一致）。
type geomPoint struct{ X, Y float64 }

func (s *canvas2DCtxState) geomReset() {
	s.geom = s.geom[:0]
	s.geomCur = -1
}

func (s *canvas2DCtxState) geomNewSubpath(x, y float64) {
	s.geom = append(s.geom, geomSubpath{pts: []geomPoint{{X: x, Y: y}}})
	s.geomCur = len(s.geom) - 1
}

func (s *canvas2DCtxState) geomLineTo(x, y float64) {
	if s.geomCur < 0 {
		s.geomNewSubpath(x, y)
		return
	}
	sub := &s.geom[s.geomCur]
	sub.pts = append(sub.pts, geomPoint{X: x, Y: y})
}

func (s *canvas2DCtxState) geomCurPoint() (geomPoint, bool) {
	if s.geomCur < 0 {
		return geomPoint{}, false
	}
	pts := s.geom[s.geomCur].pts
	if len(pts) == 0 {
		return geomPoint{}, false
	}
	return pts[len(pts)-1], true
}

func (s *canvas2DCtxState) geomClose() {
	if s.geomCur >= 0 {
		s.geom[s.geomCur].closed = true
	}
}

// geomSampleCubic 采样三次贝塞尔（起点取当前点）。
func (s *canvas2DCtxState) geomSampleCubic(x1, y1, x2, y2, x3, y3 float64, steps int) {
	p0, ok := s.geomCurPoint()
	if !ok {
		s.geomNewSubpath(x3, y3)
		return
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		mt := 1 - t
		x := mt*mt*mt*p0.X + 3*mt*mt*t*x1 + 3*mt*t*t*x2 + t*t*t*x3
		y := mt*mt*mt*p0.Y + 3*mt*mt*t*y1 + 3*mt*t*t*y2 + t*t*t*y3
		s.geomLineTo(x, y)
	}
}

// geomSampleQuad 采样二次贝塞尔（起点取当前点）。
func (s *canvas2DCtxState) geomSampleQuad(x1, y1, x2, y2 float64, steps int) {
	p0, ok := s.geomCurPoint()
	if !ok {
		s.geomNewSubpath(x2, y2)
		return
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		mt := 1 - t
		x := mt*mt*p0.X + 2*mt*t*x1 + t*t*x2
		y := mt*mt*p0.Y + 2*mt*t*y1 + t*t*y2
		s.geomLineTo(x, y)
	}
}

// geomArcPoints 采样椭圆弧（角度语义与 appendArc/appendEllipse 一致：ccw 决定
// 扫描方向；先从当前点直线连到弧起点）。
func (s *canvas2DCtxState) geomArcPoints(cx, cy, rx, ry, start, end, rotation float64, ccw bool) {
	delta := end - start
	if !ccw {
		if delta <= -2*math.Pi {
			delta = 2 * math.Pi
		} else if delta < 0 {
			delta += 2 * math.Pi
		} else if delta > 2*math.Pi {
			delta = 2 * math.Pi
		}
	} else {
		if delta >= 2*math.Pi {
			delta = -2 * math.Pi
		} else if delta > 0 {
			delta -= 2 * math.Pi
		} else if delta < -2*math.Pi {
			delta = -2 * math.Pi
		}
	}
	steps := int(math.Ceil(math.Abs(delta) / (math.Pi / 24)))
	if steps < 4 {
		steps = 4
	}
	cosR, sinR := math.Cos(rotation), math.Sin(rotation)
	pt := func(a float64) geomPoint {
		x := rx * math.Cos(a)
		y := ry * math.Sin(a)
		return geomPoint{X: cx + x*cosR - y*sinR, Y: cy + x*sinR + y*cosR}
	}
	p0 := pt(start)
	s.geomLineTo(p0.X, p0.Y) // 规范：arc 先连到弧起点
	for i := 1; i <= steps; i++ {
		p := pt(start + delta*float64(i)/float64(steps))
		s.geomLineTo(p.X, p.Y)
	}
}

// pointInGeom 射线法判定点是否在路径内（evenOdd=true 用奇偶规则，否则非零规则）。
func (s *canvas2DCtxState) pointInGeom(x, y float64, evenOdd bool) bool {
	winding := 0
	crossings := 0
	for _, sub := range s.geom {
		n := len(sub.pts)
		if n < 2 {
			continue
		}
		limit := n - 1
		if sub.closed {
			limit = n
		}
		for i := 0; i < limit; i++ {
			a := sub.pts[i]
			b := sub.pts[(i+1)%n]
			if a.Y == b.Y {
				continue
			}
			// 半开区间（a.Y <= y < b.Y 之类）避免顶点重复计数。
			if (a.Y <= y && b.Y > y) || (b.Y <= y && a.Y > y) {
				xin := a.X + (y-a.Y)/(b.Y-a.Y)*(b.X-a.X)
				if xin > x {
					crossings++
					if a.Y <= y {
						winding++
					} else {
						winding--
					}
				}
			}
		}
	}
	if evenOdd {
		return crossings%2 == 1
	}
	return winding != 0
}

// pointNearGeom 判定点是否落在路径描边上（到任一线段距离 ≤ halfWidth）。
func (s *canvas2DCtxState) pointNearGeom(x, y, halfWidth float64) bool {
	for _, sub := range s.geom {
		n := len(sub.pts)
		if n == 0 {
			continue
		}
		if n == 1 {
			if dist2d(x, y, sub.pts[0].X, sub.pts[0].Y) <= halfWidth {
				return true
			}
			continue
		}
		limit := n - 1
		if sub.closed {
			limit = n
		}
		for i := 0; i < limit; i++ {
			a := sub.pts[i]
			b := sub.pts[(i+1)%n]
			if pointSegmentDistance(x, y, a.X, a.Y, b.X, b.Y) <= halfWidth {
				return true
			}
		}
	}
	return false
}

func dist2d(x0, y0, x1, y1 float64) float64 {
	dx, dy := x0-x1, y0-y1
	return math.Sqrt(dx*dx + dy*dy)
}

// pointSegmentDistance 点到线段的距离。
func pointSegmentDistance(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	if dx == 0 && dy == 0 {
		return dist2d(px, py, x0, y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return dist2d(px, py, x0+t*dx, y0+t*dy)
}

// ─── path 构建 ──────────────────────────────────────────────────────

// newPathFor 返回新 path（或复用），beginPath 时旧 path 释放。
func (s *canvas2DCtxState) beginPath() {
	if s.path != nil {
		s.path.Release()
	}
	s.path = skia.NewPath()
	s.geomReset()
}

func (s *canvas2DCtxState) curPath() *skia.Path {
	if s.path == nil {
		s.path = skia.NewPath()
	}
	return s.path
}

// arcSegment 用单三次贝塞尔近似一角弧（|δ| ≤ π/2 精度优良）。
func arcSegment(p *skia.Path, cx, cy, r, a0, a1 float64) {
	if r <= 0 {
		p.LineTo(float32(cx), float32(cy))
		return
	}
	x0 := cx + r*math.Cos(a0)
	y0 := cy + r*math.Sin(a0)
	x1 := cx + r*math.Cos(a1)
	y1 := cy + r*math.Sin(a1)
	k := 4.0 / 3.0 * math.Tan((a1-a0)/4.0) * r
	dx0 := -math.Sin(a0) * k
	dy0 := math.Cos(a0) * k
	dx1 := -math.Sin(a1) * k
	dy1 := math.Cos(a1) * k
	p.CubicTo(float32(x0+dx0), float32(y0+dy0), float32(x1-dx1), float32(y1-dy1), float32(x1), float32(y1))
}

// appendArc 把 arc 追加到 path（moveTo 至弧起点后分段三次贝塞尔）。
func appendArc(p *skia.Path, cx, cy, r, start, end float64, ccw bool) {
	delta := end - start
	if !ccw {
		if delta <= -2*math.Pi {
			delta = 2 * math.Pi
		} else if delta < 0 {
			delta += 2 * math.Pi
		} else if delta > 2*math.Pi {
			delta = 2 * math.Pi
		}
	} else {
		if delta >= 2*math.Pi {
			delta = -2 * math.Pi
		} else if delta > 0 {
			delta -= 2 * math.Pi
		} else if delta < -2*math.Pi {
			delta = -2 * math.Pi
		}
	}
	segs := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	if segs < 1 {
		segs = 1
	}
	step := delta / float64(segs)
	// 弧起点（新子路径）
	sx := cx + r*math.Cos(start)
	sy := cy + r*math.Sin(start)
	p.MoveTo(float32(sx), float32(sy))
	for i := 0; i < segs; i++ {
		a0 := start + step*float64(i)
		arcSegment(p, cx, cy, r, a0, a0+step)
	}
}

// ─── 文本 ──────────────────────────────────────────────────────────

// c2dFont 从 this.font 解析 graphics.Font。
func c2dFont(o *jsc.JSObject) graphics.Font {
	fam, size, weight, stl := parseCanvasFontSpec(c2dStr(o, "font", "10px sans-serif"))
	return graphics.Font{Family: fam, Size: size, Weight: weight, Style: stl}
}

// textMetrics 返回文本宽度与字体度量（ascent/descent）。
func textMetrics(text string, font graphics.Font) (width, ascent, descent float64) {
	if layout.MeasureTextFunc != nil {
		width = layout.MeasureTextFunc(font.Family, font.Size, font.Weight, font.Style, text)
	}
	ascent = font.Size * 0.8
	descent = font.Size * 0.2
	if layout.FontMetricsFunc != nil {
		fa, fd, _ := layout.FontMetricsFunc(font.Family, font.Size, font.Weight, font.Style)
		if fa > 0 && fd > 0 {
			ascent, descent = fa, fd
		}
	}
	return width, ascent, descent
}

// textXY 按 textAlign/textBaseline 计算 baseline 绘制锚点。
func textXY(o *jsc.JSObject, text string, font graphics.Font, x, y float64) (tx, ty, tw float64) {
	w, ascent, descent := textMetrics(text, font)
	tx = x
	switch c2dStr(o, "textAlign", "start") {
	case "center":
		tx = x - w/2
	case "right", "end":
		tx = x - w
	}
	ty = y
	switch c2dStr(o, "textBaseline", "alphabetic") {
	case "top":
		ty = y + ascent
	case "middle":
		ty = y + (ascent-descent)/2
	case "bottom":
		ty = y - descent
	}
	return tx, ty, w
}

// ─── drawImage 源 ───────────────────────────────────────────────────

// sourceImageOf 解析 drawImage 的源对象（canvas 元素 / img 元素 / ImageData）。
// 返回 (位图, owned)：owned=true 表示这是新增引用（canvas 快照，调用方
// 必须 Release）；owned=false 表示借用（DecodedImage 内部共享、由缓存持有，
// 调用方不得 Release）。
func sourceImageOf(arg jsc.JSValue) (*skia.Image, bool) {
	if !arg.IsObject() {
		return nil, false
	}
	node := unwrapNode(arg)
	if node == nil {
		return nil, false
	}
	el, ok := node.(*dom.Element)
	if !ok {
		return nil, false
	}
	switch strings.ToLower(el.LocalName()) {
	case "canvas":
		if bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap); ok && bm != nil {
			return bm.SkiaImage(), true
		}
	case "img":
		if canvasImageSourceHook != nil {
			return canvasImageSourceHook(el), false
		}
	}
	return nil, false
}

// ─── ctx 对象构建（方法注册）───────────────────────────────────────

// buildCanvas2DCtx 构建 CanvasRenderingContext2D 的 JS 对象——样式属性和
// 全部 2D 方法。样式属性初始化为浏览器默认值并存于对象自身（JS 侧）；
// 方法为 Go native，闭包捕获 *canvas2DCtxState。
func buildCanvas2DCtx(rt *jsc.Interpreter, el *dom.Element, bm *rendering.CanvasBitmap) *jsc.JSObject {
	s := &canvas2DCtxState{el: el, bm: bm}
	o := jsc.NewObject(rt.ObjectPrototype())

	o.Set("fillStyle", jsc.StringValue("#000000"))
	o.Set("strokeStyle", jsc.StringValue("#000000"))
	o.Set("lineWidth", jsc.NumberValue(1))
	o.Set("lineCap", jsc.StringValue("butt"))
	o.Set("lineJoin", jsc.StringValue("miter"))
	o.Set("miterLimit", jsc.NumberValue(10))
	o.Set("font", jsc.StringValue("10px sans-serif"))
	o.Set("textAlign", jsc.StringValue("start"))
	o.Set("textBaseline", jsc.StringValue("alphabetic"))
	o.Set("globalAlpha", jsc.NumberValue(1))
	o.Set("globalCompositeOperation", jsc.StringValue("source-over"))
	o.Set("shadowBlur", jsc.NumberValue(0))
	o.Set("shadowColor", jsc.StringValue("rgba(0, 0, 0, 0)"))
	o.Set("shadowOffsetX", jsc.NumberValue(0))
	o.Set("shadowOffsetY", jsc.NumberValue(0))
	o.Set("lineDashOffset", jsc.NumberValue(0))

	// ── 状态（save/restore）──────────────────────────────────────
	o.Set("save", jsc.FunctionValue(jsc.NewNativeFunction("save",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.Save()
			}
			obj := this.AsObject()
			if obj == nil {
				return jsc.Undefined()
			}
			snap := canvas2DSaved{
				fillStyle:                   propOf(obj, "fillStyle"),
				strokeStyle:                 propOf(obj, "strokeStyle"),
				lineWidth:                   c2dNum(obj, "lineWidth", 1),
				miterLimit:                  c2dNum(obj, "miterLimit", 10),
				globalAlpha:                 c2dNum(obj, "globalAlpha", 1),
				shadowBlur:                  c2dNum(obj, "shadowBlur", 0),
				shadowOffsetX:               c2dNum(obj, "shadowOffsetX", 0),
				shadowOffsetY:               c2dNum(obj, "shadowOffsetY", 0),
				lineDashOffset:              c2dNum(obj, "lineDashOffset", 0),
				lineCap:                     c2dStr(obj, "lineCap", "butt"),
				lineJoin:                    c2dStr(obj, "lineJoin", "miter"),
				font:                        c2dStr(obj, "font", "10px sans-serif"),
				textAlign:                   c2dStr(obj, "textAlign", "start"),
				textBaseline:                c2dStr(obj, "textBaseline", "alphabetic"),
				globalCompositeOperation:    c2dStr(obj, "globalCompositeOperation", "source-over"),
				shadowColor:                 c2dStr(obj, "shadowColor", "rgba(0, 0, 0, 0)"),
				lineDash:                    append([]float32(nil), s.dash...),
			}
			s.saved = append(s.saved, snap)
			return jsc.Undefined()
		}, 0)))
	o.Set("restore", jsc.FunctionValue(jsc.NewNativeFunction("restore",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if len(s.saved) == 0 {
				return jsc.Undefined()
			}
			snap := s.saved[len(s.saved)-1]
			s.saved = s.saved[:len(s.saved)-1]
			obj := this.AsObject()
			if obj != nil {
				obj.Set("fillStyle", snap.fillStyle)
				obj.Set("strokeStyle", snap.strokeStyle)
				obj.Set("lineWidth", jsc.NumberValue(snap.lineWidth))
				obj.Set("miterLimit", jsc.NumberValue(snap.miterLimit))
				obj.Set("globalAlpha", jsc.NumberValue(snap.globalAlpha))
				obj.Set("shadowBlur", jsc.NumberValue(snap.shadowBlur))
				obj.Set("shadowOffsetX", jsc.NumberValue(snap.shadowOffsetX))
				obj.Set("shadowOffsetY", jsc.NumberValue(snap.shadowOffsetY))
				obj.Set("lineDashOffset", jsc.NumberValue(snap.lineDashOffset))
				obj.Set("lineCap", jsc.StringValue(snap.lineCap))
				obj.Set("lineJoin", jsc.StringValue(snap.lineJoin))
				obj.Set("font", jsc.StringValue(snap.font))
				obj.Set("textAlign", jsc.StringValue(snap.textAlign))
				obj.Set("textBaseline", jsc.StringValue(snap.textBaseline))
				obj.Set("globalCompositeOperation", jsc.StringValue(snap.globalCompositeOperation))
				obj.Set("shadowColor", jsc.StringValue(snap.shadowColor))
				s.dash = append([]float32(nil), snap.lineDash...)
			}
			if bm.Cv != nil {
				bm.Cv.Restore()
			}
			return jsc.Undefined()
		}, 0)))

	// ── 变换 ─────────────────────────────────────────────────────
	o.Set("translate", jsc.FunctionValue(jsc.NewNativeFunction("translate",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.Translate(argNum(a, 0, 0), argNum(a, 1, 0))
			}
			return jsc.Undefined()
		}, 2)))
	o.Set("rotate", jsc.FunctionValue(jsc.NewNativeFunction("rotate",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.Rotate(argNum(a, 0, 0))
			}
			return jsc.Undefined()
		}, 1)))
	o.Set("scale", jsc.FunctionValue(jsc.NewNativeFunction("scale",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.Scale(argNum(a, 0, 1), argNum(a, 1, 1))
			}
			return jsc.Undefined()
		}, 2)))
	o.Set("transform", jsc.FunctionValue(jsc.NewNativeFunction("transform",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.Concat(skia.Matrix{
					ScaleX: float32(argNum(a, 0, 1)), SkewY: float32(argNum(a, 1, 0)),
					SkewX: float32(argNum(a, 2, 0)), ScaleY: float32(argNum(a, 3, 1)),
					TransX: float32(argNum(a, 4, 0)), TransY: float32(argNum(a, 5, 0)),
				})
			}
			return jsc.Undefined()
		}, 6)))
	o.Set("setTransform", jsc.FunctionValue(jsc.NewNativeFunction("setTransform",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.SetMatrix(skia.Matrix{
					ScaleX: float32(argNum(a, 0, 1)), SkewY: float32(argNum(a, 1, 0)),
					SkewX: float32(argNum(a, 2, 0)), ScaleY: float32(argNum(a, 3, 1)),
					TransX: float32(argNum(a, 4, 0)), TransY: float32(argNum(a, 5, 0)),
				})
			}
			return jsc.Undefined()
		}, 6)))
	o.Set("resetTransform", jsc.FunctionValue(jsc.NewNativeFunction("resetTransform",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.ResetMatrix()
			}
			return jsc.Undefined()
		}, 0)))
	// getTransform：返回 DOMMatrix 形态对象（a..f，对应 skia 的 3x3 仿射）。
	o.Set("getTransform", jsc.FunctionValue(jsc.NewNativeFunction("getTransform",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			m := skia.Matrix{}
			if bm.Cv != nil {
				m = bm.Cv.GetMatrix()
			}
			obj := jsc.NewObject(rt.ObjectPrototype())
			obj.SetClassName("DOMMatrix")
			vals := []float64{float64(m.ScaleX), float64(m.SkewY), float64(m.SkewX), float64(m.ScaleY), float64(m.TransX), float64(m.TransY)}
			for i, key := range []string{"a", "b", "c", "d", "e", "f"} {
				obj.Set(key, jsc.NumberValue(vals[i]))
			}
			obj.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.StringValue("matrix(" +
						numStr(vals[0]) + ", " + numStr(vals[1]) + ", " + numStr(vals[2]) + ", " +
						numStr(vals[3]) + ", " + numStr(vals[4]) + ", " + numStr(vals[5]) + ")")
				}, 0)))
			return jsc.ObjectValue(obj)
		}, 0)))
	// reset()：清空路径、变换、线型与全部样式属性到初始值（canvas 2D 新 API）。
	o.Set("reset", jsc.FunctionValue(jsc.NewNativeFunction("reset",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.ResetMatrix()
				bm.Cv.ResetClip()
			}
			s.dash = nil
			s.beginPath()
			obj := this.AsObject()
			if obj != nil {
				obj.Set("fillStyle", jsc.StringValue("#000000"))
				obj.Set("strokeStyle", jsc.StringValue("#000000"))
				obj.Set("lineWidth", jsc.NumberValue(1))
				obj.Set("lineCap", jsc.StringValue("butt"))
				obj.Set("lineJoin", jsc.StringValue("miter"))
				obj.Set("miterLimit", jsc.NumberValue(10))
				obj.Set("lineDashOffset", jsc.NumberValue(0))
				obj.Set("font", jsc.StringValue("10px sans-serif"))
				obj.Set("textAlign", jsc.StringValue("start"))
				obj.Set("textBaseline", jsc.StringValue("alphabetic"))
				obj.Set("globalAlpha", jsc.NumberValue(1))
				obj.Set("globalCompositeOperation", jsc.StringValue("source-over"))
				obj.Set("shadowBlur", jsc.NumberValue(0))
				obj.Set("shadowColor", jsc.StringValue("rgba(0, 0, 0, 0)"))
				obj.Set("shadowOffsetX", jsc.NumberValue(0))
				obj.Set("shadowOffsetY", jsc.NumberValue(0))
			}
			return jsc.Undefined()
		}, 0)))

	// ── 矩形 ─────────────────────────────────────────────────────
	o.Set("fillRect", jsc.FunctionValue(jsc.NewNativeFunction("fillRect",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil {
				return jsc.Undefined()
			}
			col, sh, owns := resolveStyle(obj, "fillStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			eff := ctxEffect(obj, s)
			x, y, w, h := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0)
			if sh != nil {
				if owns {
					defer sh.Release()
				}
				bm.Cv.FillRectShaderEffect(x, y, w, h, sh, alpha, blend, eff)
			} else {
				bm.Cv.FillRectFullEffect(x, y, w, h, col, alpha, blend, eff)
			}
			return jsc.Undefined()
		}, 4)))
	o.Set("strokeRect", jsc.FunctionValue(jsc.NewNativeFunction("strokeRect",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil {
				return jsc.Undefined()
			}
			col, sh, owns := resolveStyle(obj, "strokeStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			lw := c2dNum(obj, "lineWidth", 1)
			cap := c2dStr(obj, "lineCap", "butt")
			join := c2dStr(obj, "lineJoin", "miter")
			eff := ctxEffect(obj, s)
			x, y, w, h := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0)
			rp := skia.NewPath()
			defer rp.Release()
			rp.MoveTo(float32(x), float32(y))
			rp.LineTo(float32(x+w), float32(y))
			rp.LineTo(float32(x+w), float32(y+h))
			rp.LineTo(float32(x), float32(y+h))
			rp.Close()
			if sh != nil {
				if owns {
					defer sh.Release()
				}
				bm.Cv.StrokePathShaderEffect(rp, lw, sh, alpha, cap, join, blend, eff)
			} else {
				bm.Cv.StrokePathFullEffect(rp, lw, col, alpha, cap, join, blend, eff)
			}
			return jsc.Undefined()
		}, 4)))
	o.Set("clearRect", jsc.FunctionValue(jsc.NewNativeFunction("clearRect",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv != nil {
				bm.Cv.ClearRect(argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0))
			}
			return jsc.Undefined()
		}, 4)))

	// ── 路径 ─────────────────────────────────────────────────────
	o.Set("beginPath", jsc.FunctionValue(jsc.NewNativeFunction("beginPath",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			s.beginPath()
			return jsc.Undefined()
		}, 0)))
	o.Set("closePath", jsc.FunctionValue(jsc.NewNativeFunction("closePath",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			s.curPath().Close()
			s.geomClose()
			return jsc.Undefined()
		}, 0)))
	o.Set("moveTo", jsc.FunctionValue(jsc.NewNativeFunction("moveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			x, y := argNum(a, 0, 0), argNum(a, 1, 0)
			s.curPath().MoveTo(float32(x), float32(y))
			s.geomNewSubpath(x, y)
			return jsc.Undefined()
		}, 2)))
	o.Set("lineTo", jsc.FunctionValue(jsc.NewNativeFunction("lineTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			x, y := argNum(a, 0, 0), argNum(a, 1, 0)
			s.curPath().LineTo(float32(x), float32(y))
			s.geomLineTo(x, y)
			return jsc.Undefined()
		}, 2)))
	o.Set("bezierCurveTo", jsc.FunctionValue(jsc.NewNativeFunction("bezierCurveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().CubicTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)), float32(argNum(a, 2, 0)), float32(argNum(a, 3, 0)), float32(argNum(a, 4, 0)), float32(argNum(a, 5, 0)))
			s.geomSampleCubic(argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0), argNum(a, 4, 0), argNum(a, 5, 0), 24)
			return jsc.Undefined()
		}, 6)))
	o.Set("quadraticCurveTo", jsc.FunctionValue(jsc.NewNativeFunction("quadraticCurveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().QuadTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)), float32(argNum(a, 2, 0)), float32(argNum(a, 3, 0)))
			s.geomSampleQuad(argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0), 16)
			return jsc.Undefined()
		}, 4)))
	o.Set("arc", jsc.FunctionValue(jsc.NewNativeFunction("arc",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			cx, cy, r := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0)
			start, end := argNum(a, 3, 0), argNum(a, 4, 0)
			ccw := argBool(a, 5, false)
			appendArc(s.curPath(), cx, cy, r, start, end, ccw)
			s.geomArcPoints(cx, cy, r, r, start, end, 0, ccw)
			return jsc.Undefined()
		}, 5)))
	o.Set("ellipse", jsc.FunctionValue(jsc.NewNativeFunction("ellipse",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			// v1：对椭圆做「单位圆弧 → 仿射缩放」近似
			//（rx/ry 半径 + 起始/结束角，忽略 rotation）。
			argN := func(i int, d float64) float64 { return argNum(a, i, d) }
			p := s.curPath()
			cx, cy, rx, ry := argN(0, 0), argN(1, 0), argN(2, 0), argN(3, 0)
			a0, a1 := argN(5, 0), argN(6, 2*math.Pi)
			ccw := argBool(a, 7, false)
			appendEllipse(p, cx, cy, rx, ry, a0, a1, ccw)
			s.geomArcPoints(cx, cy, rx, ry, a0, a1, 0, ccw)
			return jsc.Undefined()
		}, 7)))
	o.Set("rect", jsc.FunctionValue(jsc.NewNativeFunction("rect",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			x, y, w, h := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0)
			r := skia.RectXYWH(float32(x), float32(y), float32(w), float32(h))
			s.curPath().AddRect(r, skia.PathDirectionCW)
			s.geomNewSubpath(x, y)
			s.geomLineTo(x+w, y)
			s.geomLineTo(x+w, y+h)
			s.geomLineTo(x, y+h)
			s.geomClose()
			return jsc.Undefined()
		}, 4)))
	o.Set("arcTo", jsc.FunctionValue(jsc.NewNativeFunction("arcTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			appendArcTo(s.curPath(), s, argNum(a, 0, 0), argNum(a, 1, 0),
				argNum(a, 2, 0), argNum(a, 3, 0), argNum(a, 4, 0))
			return jsc.Undefined()
		}, 5)))
	// roundRect(x, y, w, h, radii)：radii 支持数字、[all]、[tl,br]、[tl,tr,br]、
	// [tl,tr,br,bl] 与 [{x,y},...] 形式（CSS 圆角简写语义）。
	o.Set("roundRect", jsc.FunctionValue(jsc.NewNativeFunction("roundRect",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			appendRoundRect(s.curPath(), s, argNum(a, 0, 0), argNum(a, 1, 0),
				argNum(a, 2, 0), argNum(a, 3, 0), parseRoundRectRadii(a, 4))
			return jsc.Undefined()
		}, 1)))
	o.Set("fill", jsc.FunctionValue(jsc.NewNativeFunction("fill",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || s.path == nil {
				return jsc.Undefined()
			}
			if rule := argStr(a, 0, "nonzero"); rule == "evenodd" {
				s.path.SetFillType(skia.FillTypeEvenOdd)
			} else {
				s.path.SetFillType(skia.FillTypeWinding)
			}
			col, sh, owns := resolveStyle(obj, "fillStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			eff := ctxEffect(obj, s)
			if sh != nil {
				if owns {
					defer sh.Release()
				}
				bm.Cv.FillPathShaderEffect(s.path, sh, alpha, blend, eff)
			} else {
				bm.Cv.FillPathFullEffect(s.path, col, alpha, blend, eff)
			}
			return jsc.Undefined()
		}, 1)))
	o.Set("stroke", jsc.FunctionValue(jsc.NewNativeFunction("stroke",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || s.path == nil {
				return jsc.Undefined()
			}
			col, sh, owns := resolveStyle(obj, "strokeStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			lw := c2dNum(obj, "lineWidth", 1)
			cap := c2dStr(obj, "lineCap", "butt")
			join := c2dStr(obj, "lineJoin", "miter")
			eff := ctxEffect(obj, s)
			if sh != nil {
				if owns {
					defer sh.Release()
				}
				bm.Cv.StrokePathShaderEffect(s.path, lw, sh, alpha, cap, join, blend, eff)
			} else {
				bm.Cv.StrokePathFullEffect(s.path, lw, col, alpha, cap, join, blend, eff)
			}
			return jsc.Undefined()
		}, 0)))
	// isPointInPath / isPointInStroke：基于路径的几何副本做精确判定（skia 的
	// Path 无法反查点集，几何副本在路径构建时同步维护；坐标为用户空间，与规范
	// 一致——查询点同样在用户空间，无需逆变换）。
	o.Set("isPointInPath", jsc.FunctionValue(jsc.NewNativeFunction("isPointInPath",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			evenOdd := argStr(a, 2, "nonzero") == "evenodd"
			return jsc.BooleanValue(s.pointInGeom(argNum(a, 0, 0), argNum(a, 1, 0), evenOdd))
		}, 2)))
	o.Set("isPointInStroke", jsc.FunctionValue(jsc.NewNativeFunction("isPointInStroke",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			lw := 1.0
			if obj := this.AsObject(); obj != nil {
				lw = c2dNum(obj, "lineWidth", 1)
			}
			if lw <= 0 {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(s.pointNearGeom(argNum(a, 0, 0), argNum(a, 1, 0), lw/2))
		}, 2)))
	o.Set("clip", jsc.FunctionValue(jsc.NewNativeFunction("clip",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv == nil || s.path == nil {
				return jsc.Undefined()
			}
			if rule := argStr(a, 0, "nonzero"); rule == "evenodd" {
				s.path.SetFillType(skia.FillTypeEvenOdd)
			} else {
				s.path.SetFillType(skia.FillTypeWinding)
			}
			bm.Cv.ClipPath(s.path)
			return jsc.Undefined()
		}, 1)))

	// ── 文本 ─────────────────────────────────────────────────────
	o.Set("fillText", jsc.FunctionValue(jsc.NewNativeFunction("fillText",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || len(a) < 3 {
				return jsc.Undefined()
			}
			text := a[0].ToString()
			font := c2dFont(obj)
			col, sh, owns := resolveStyle(obj, "fillStyle")
			if sh != nil {
				// 文本 v1 不支持 shader 填充，回退纯色；仅释放自建的渐变 shader
				//（图案 shader 由图案注册表持有，Release 会释放底层对象）。
				if owns {
					sh.Release()
				}
			}
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			tx, ty, _ := textXY(obj, text, font, argNum(a, 1, 0), argNum(a, 2, 0))
			bm.Cv.DrawTextAlpha(tx, ty, text, font, col, alpha, blend)
			return jsc.Undefined()
		}, 3)))
	o.Set("strokeText", jsc.FunctionValue(jsc.NewNativeFunction("strokeText",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || len(a) < 3 {
				return jsc.Undefined()
			}
			text := a[0].ToString()
			font := c2dFont(obj)
			col, sh, owns := resolveStyle(obj, "strokeStyle")
			if sh != nil {
				if owns {
					sh.Release()
				}
			}
			alpha := c2dNum(obj, "globalAlpha", 1)
			lw := c2dNum(obj, "lineWidth", 1)
			tx, ty, _ := textXY(obj, text, font, argNum(a, 1, 0), argNum(a, 2, 0))
			bm.Cv.StrokeTextAlpha(tx, ty, text, font, lw, col, alpha)
			return jsc.Undefined()
		}, 3)))
	o.Set("measureText", jsc.FunctionValue(jsc.NewNativeFunction("measureText",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			text := ""
			if len(a) > 0 {
				text = a[0].ToString()
			}
			font := c2dFont(obj)
			w, ascent, descent := textMetrics(text, font)
			h := math.Round(ascent + descent)
			fba, fbd := h*0.8, h*0.2
			m := jsc.NewObject(rt.ObjectPrototype())
			m.Set("width", jsc.NumberValue(w))
			m.Set("actualBoundingBoxAscent", jsc.NumberValue(fba))
			m.Set("actualBoundingBoxDescent", jsc.NumberValue(fbd))
			m.Set("fontBoundingBoxAscent", jsc.NumberValue(fba))
			m.Set("fontBoundingBoxDescent", jsc.NumberValue(fbd))
			m.Set("height", jsc.NumberValue(h))
			return jsc.ObjectValue(m)
		}, 1)))

	// ── 图像 ─────────────────────────────────────────────────────
	o.Set("drawImage", jsc.FunctionValue(jsc.NewNativeFunction("drawImage",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || len(a) < 3 {
				return jsc.Undefined()
			}
			img, owned := sourceImageOf(a[0])
			if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
				iw, ih := 0, 0
				if img != nil {
					iw, ih = img.Width(), img.Height()
				}
				log.Printf("[canvas2d] drawImage src=%v img=%v owned=%v iw=%d ih=%d len(a)=%d", a[0].ToString(), img != nil, owned, iw, ih, len(a))
			}
			if img == nil {
				return jsc.Undefined()
			}
			if owned {
				defer img.Release()
			}
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			iw := float64(img.Width())
			ih := float64(img.Height())
			var sx, sy, sw, sh, dx, dy, dw, dh float64
			switch len(a) {
			case 3:
				dx, dy = argNum(a, 1, 0), argNum(a, 2, 0)
				sx, sy, sw, sh = 0, 0, iw, ih
				dw, dh = iw, ih
			case 5:
				dx, dy = argNum(a, 1, 0), argNum(a, 2, 0)
				dw, dh = argNum(a, 3, 0), argNum(a, 4, 0)
				sx, sy, sw, sh = 0, 0, iw, ih
			case 9:
				sx, sy = argNum(a, 1, 0), argNum(a, 2, 0)
				sw, sh = argNum(a, 3, 0), argNum(a, 4, 0)
				dx, dy = argNum(a, 5, 0), argNum(a, 6, 0)
				dw, dh = argNum(a, 7, 0), argNum(a, 8, 0)
			default:
				return jsc.Undefined()
			}
			if sw <= 0 || sh <= 0 {
				return jsc.Undefined()
			}
			bm.Cv.DrawImageFull(img, sx, sy, sw, sh, dx, dy, dw, dh, alpha, blend)
			if os.Getenv("WB_CANVAS2D_DEBUG") != "" {
				p1 := bm.Cv.PixelAt(int(dx)+2, int(dy)+2)
				log.Printf("[canvas2d] after drawImage dst=(%.0f,%.0f %.0fx%.0f) px=%+v", dx, dy, dw, dh, p1)
			}
			return jsc.Undefined()
		}, 3)))

	// ── Live2D 高性能原语：wbDrawTriImage（非标准扩展）──────────────
	// 一次调用完成「clip 三角形 + UV→屏幕仿射 + drawImage」——Live2D
	// 渲染器逐三角形绘制时原路径每三角形 10 次 bindings 调用（save/
	// beginPath/moveTo×3/lineTo×2/closePath/clip/transform/drawImage/
	// restore ≈ 1 万次 Go↔goja 切换/帧 → 单帧 ~200ms）；原语化后每三角形
	// 1 次调用 → 单帧预计 <30ms。
	// 参数顺序：img, x0,y0, x1,y1, x2,y2, u0,v0, u1,v1, u2,v2, alpha, blend
	// （uv 为 0..1 缩放 UV；纹理像素 = u*imgW；blend 为
	// "source-over"/"lighter" 等 canvas 2D 组合模式字符串）
	o.Set("wbDrawTriImage", jsc.FunctionValue(jsc.NewNativeFunction("wbDrawTriImage",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv == nil || len(a) < 15 {
				return jsc.Undefined()
			}
			img, owned := sourceImageOf(a[0])
			if img == nil {
				return jsc.Undefined()
			}
			if owned {
				defer img.Release()
			}
			num := func(i int) float64 { return argNum(a, i, 0) }
			x0, y0 := num(1), num(2)
			x1, y1 := num(3), num(4)
			x2, y2 := num(5), num(6)
			u0, v0 := num(7), num(8)
			u1, v1 := num(9), num(10)
			u2, v2 := num(11), num(12)
			alpha := num(13)
			blend := c2dBlend(argStr(a, 14, "source-over"))
			if alpha <= 0 {
				return jsc.Undefined()
			}
			// UV→屏幕 仿射：解 2x2（两顶点差向量），x = aM*u + bM*v + tx
			ux, uy := u1-u0, v1-v0
			vx, vy := u2-u0, v2-v0
			det := ux*vy - uy*vx
			if det == 0 {
				return jsc.Undefined()
			}
			dx1, dy1 := x1-x0, y1-y0
			dx2, dy2 := x2-x0, y2-y0
			aM := (dx1*vy - dx2*uy) / det
			bM := (dx2*ux - dx1*vx) / det
			cM := (dy1*vy - dy2*uy) / det
			dM := (dy2*ux - dy1*vx) / det
			tx := x0 - (aM*u0 + bM*v0)
			ty := y0 - (cM*u0 + dM*v0)
			path := skia.NewPath()
			defer path.Release()
			path.MoveTo(float32(x0), float32(y0))
			path.LineTo(float32(x1), float32(y1))
			path.LineTo(float32(x2), float32(y2))
			path.Close()
			bm.Cv.Save()
			bm.Cv.ClipPath(path)
			iw, ih := float64(img.Width()), float64(img.Height())
			if iw > 0 && ih > 0 {
				// 纹理 UV（px/iw, py/ih）→ 屏幕：canvas transform(m11,m12,
				// m21,m22,dx,dy) 的 x'=m11*x+m21*y+dx，用与 bindings transform
				// 相同的矩阵字段映射（ScaleX=a, SkewY=b, SkewX=c, ScaleY=d）。
				bm.Cv.Concat(skia.Matrix{
					ScaleX: float32(aM / iw), SkewY: float32(cM / iw),
					SkewX: float32(bM / ih), ScaleY: float32(dM / ih),
					TransX: float32(tx), TransY: float32(ty),
				})
				bm.Cv.DrawImageFull(img, 0, 0, iw, ih, 0, 0, iw, ih, alpha, blend)
			}
			bm.Cv.Restore()
			return jsc.Undefined()
		}, 17)))

	// ── Live2D 批量原语：wbDrawVertices（Skia drawVertices 一次绘制）──────
	// 一次调用批量绘制一个 drawable 的全部三角形（替代逐三角形
	// wbDrawTriImage：绘制调用从 ~4 千次/帧降到几十次/帧，配合
	// Float32Array 零拷贝导出（goja typed array Export 共享底层数据）。
	// 参数：img, posArr(Float32Array [x0,y0,...] 屏幕坐标),
	//       uvArr(Float32Array [u0,v0,...] 0..1 归一化纹理坐标),
	//       idxArr(Uint16Array 三角形索引), alpha, blend
	o.Set("wbDrawVertices", jsc.FunctionValue(jsc.NewNativeFunction("wbDrawVertices",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv == nil || len(a) < 5 {
				return jsc.Undefined()
			}
			img, owned := sourceImageOf(a[0])
			if img == nil {
				return jsc.Undefined()
			}
			if owned {
				defer img.Release()
			}
			pos := exportFloat32Array(a[1])
			uv := exportFloat32Array(a[2])
			idx := exportUint16Array(a[3])
			if len(pos) < 6 || len(uv) < 6 || len(idx) < 3 {
				return jsc.Undefined()
			}
			alpha := argNum(a, 4, 1)
			blend := c2dBlend(argStr(a, 5, "source-over"))
			bm.Cv.DrawVerticesFull(img, pos, uv, idx, alpha, blend)
			return jsc.Undefined()
		}, 6)))

	// ── 渐变 ─────────────────────────────────────────────────────
	makeGrad := func(kind string, ncoords int) func(*jsc.Interpreter, jsc.JSValue, []jsc.JSValue) jsc.JSValue {
		return func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			g := jsc.NewObject(rt.ObjectPrototype())
			g.Set("__wbGradient", jsc.StringValue(kind))
			coords := jsc.NewObject(rt.ObjectPrototype())
			for i := 0; i < ncoords; i++ {
				coords.Set(strconv.Itoa(i), jsc.NumberValue(argNum(a, i, 0)))
			}
			g.Set("__wbCoords", jsc.ObjectValue(coords))
			stopsArr := jsc.NewObject(rt.ObjectPrototype())
			stopsArr.Set("length", jsc.NumberValue(0))
			g.Set("__wbStops", jsc.ObjectValue(stopsArr))
			g.Set("addColorStop", jsc.FunctionValue(jsc.NewNativeFunction("addColorStop",
				func(_ *jsc.Interpreter, _ jsc.JSValue, sa []jsc.JSValue) jsc.JSValue {
					if len(sa) < 2 {
						return jsc.Undefined()
					}
					lv, _ := stopsArr.GetByKey("length")
					n := int(lv.ToNumber())
					item := jsc.NewObject(rt.ObjectPrototype())
					item.Set("off", jsc.NumberValue(argNum(sa, 0, 0)))
					item.Set("color", jsc.StringValue(sa[1].ToString()))
					stopsArr.Set(strconv.Itoa(n), jsc.ObjectValue(item))
					stopsArr.Set("length", jsc.NumberValue(float64(n+1)))
					return jsc.Undefined()
				}, 2)))
			return jsc.ObjectValue(g)
		}
	}
	o.Set("createLinearGradient", jsc.FunctionValue(jsc.NewNativeFunction("createLinearGradient", makeGrad("linear", 4), 4)))
	o.Set("createRadialGradient", jsc.FunctionValue(jsc.NewNativeFunction("createRadialGradient", makeGrad("radial", 6), 6)))
	// createPattern(image, repetition)：图案源可以是 <canvas>（取当前内容快照）
	// 或 <img>；生成的 Skia shader 存进图案注册表（shader 自带源图引用，
	// 图案不随源图释放而失效）。
	o.Set("createPattern", jsc.FunctionValue(jsc.NewNativeFunction("createPattern",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) < 2 {
				return jsc.Null()
			}
			img, owned := sourceImageOf(a[0])
			if img == nil {
				return jsc.Null() // 规范：源不可用时返回 null
			}
			if owned {
				defer img.Release() // MakeShader 内部会 ref 源图
			}
			tileX, tileY := skia.TileModeRepeat, skia.TileModeRepeat
			switch argStr(a, 1, "repeat") {
			case "repeat-x":
				tileY = skia.TileModeDecal
			case "repeat-y":
				tileX = skia.TileModeDecal
			case "no-repeat":
				tileX, tileY = skia.TileModeDecal, skia.TileModeDecal
			}
			sh := img.MakeShader(tileX, tileY, &skia.SamplingLinear, nil)
			if sh == nil {
				return jsc.Null()
			}
			pat := jsc.NewObject(rt.ObjectPrototype())
			pat.SetClassName("CanvasPattern")
			pat.Set("__wbPattern", jsc.BooleanValue(true))
			pat.Set("__wbShaderId", jsc.NumberValue(float64(registerPatternShader(sh))))
			pat.Set("__wbRepetition", jsc.StringValue(argStr(a, 1, "repeat")))
			pat.Set("setTransform", jsc.FunctionValue(jsc.NewNativeFunction("setTransform",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					// 图案局部矩阵（DOMMatrix）本引擎暂不支持：接受调用，
					// 不改变平铺结果（不影响基本图案填充）。
					return jsc.Undefined()
				}, 1)))
			return jsc.ObjectValue(pat)
		}, 2)))

	// ── 像素 ─────────────────────────────────────────────────────
	// createImageData(width, height) / createImageData(imageData)：返回全透明
	// 的新 ImageData（与 getImageData 同构：data 的 length 为 4*w*h）。
	o.Set("createImageData", jsc.FunctionValue(jsc.NewNativeFunction("createImageData",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			var iw, ih int
			if v := a[0]; v.IsObject() {
				src := v.AsObject()
				wv, _ := src.GetByKey("width")
				hv, _ := src.GetByKey("height")
				iw, ih = int(wv.ToNumber()), int(hv.ToNumber())
			} else {
				iw, ih = int(argNum(a, 0, 0)), int(argNum(a, 1, 0))
			}
			if iw <= 0 || ih <= 0 {
				// 规范抛 IndexSizeError；用原生 TypeError 抛出让脚本能 catch。
				panic(rt.VM().NewTypeError("createImageData: width/height must be positive"))
			}
			ret := jsc.NewObject(rt.ObjectPrototype())
			ret.Set("__wbImageData", jsc.BooleanValue(true))
			ret.Set("width", jsc.NumberValue(float64(iw)))
			ret.Set("height", jsc.NumberValue(float64(ih)))
			arr := jsc.NewObject(rt.ObjectPrototype())
			for i := 0; i < iw*ih*4; i++ {
				arr.Set(strconv.Itoa(i), jsc.NumberValue(0))
			}
			arr.Set("length", jsc.NumberValue(float64(iw*ih*4)))
			ret.Set("data", jsc.ObjectValue(arr))
			return jsc.ObjectValue(ret)
		}, 2)))
	o.Set("getImageData", jsc.FunctionValue(jsc.NewNativeFunction("getImageData",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			ret := jsc.NewObject(rt.ObjectPrototype())
			ret.Set("__wbImageData", jsc.BooleanValue(true))
			w, h := bm.W, bm.H
			sx := int(argNum(a, 0, 0))
			sy := int(argNum(a, 1, 0))
			sw := int(argNum(a, 2, 0))
			sh := int(argNum(a, 3, 0))
			if sw < 0 || sh < 0 {
				return jsc.ObjectValue(ret)
			}
			data := make([]float64, 0, 4*sw*sh)
			for y := sy; y < sy+sh; y++ {
				for x := sx; x < sx+sw; x++ {
					if x < 0 || y < 0 || x >= w || y >= h {
						data = append(data, 0, 0, 0, 0)
						continue
					}
					c := bm.Cv.PixelAt(x, y)
					if c.A > 0 {
						// 解除预乘（Skia N32 premul → 浏览器 unpremul）
						r := int(c.R) * 255 / int(c.A)
						g := int(c.G) * 255 / int(c.A)
						b := int(c.B) * 255 / int(c.A)
						if r > 255 {
							r = 255
						}
						if g > 255 {
							g = 255
						}
						if b > 255 {
							b = 255
						}
						data = append(data, float64(r), float64(g), float64(b), float64(c.A))
					} else {
						data = append(data, 0, 0, 0, 0)
					}
				}
			}
			ret.Set("width", jsc.NumberValue(float64(sw)))
			ret.Set("height", jsc.NumberValue(float64(sh)))
			arr := jsc.NewObject(rt.ObjectPrototype())
			for i, v := range data {
				arr.Set(strconv.Itoa(i), jsc.NumberValue(v))
			}
			arr.Set("length", jsc.NumberValue(float64(len(data))))
			ret.Set("data", jsc.ObjectValue(arr))
			return jsc.ObjectValue(ret)
		}, 4)))
	o.Set("putImageData", jsc.FunctionValue(jsc.NewNativeFunction("putImageData",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if bm.Cv == nil || len(a) < 3 {
				return jsc.Undefined()
			}
			imgData := a[0].AsObject()
			if imgData == nil {
				return jsc.Undefined()
			}
			wv, _ := imgData.GetByKey("width")
			hv, _ := imgData.GetByKey("height")
			dv, _ := imgData.GetByKey("data")
			iw := int(wv.ToNumber())
			ih := int(hv.ToNumber())
			dObj := dv.AsObject()
			if dObj == nil || iw <= 0 || ih <= 0 {
				return jsc.Undefined()
			}
			dx := int(argNum(a, 1, 0))
			dy := int(argNum(a, 2, 0))
			// dirty 矩形（第 4-7 参：dirtyX/dirtyY/dirtyWidth/dirtyHeight）：
			// 只更新 ImageData 的该子区域；越界部分按规范夹取。
			sx0, sy0, sw, sh := 0, 0, iw, ih
			if len(a) >= 7 {
				sx0, sy0 = int(argNum(a, 3, 0)), int(argNum(a, 4, 0))
				sw, sh = int(argNum(a, 5, 0)), int(argNum(a, 6, 0))
				if sx0 < 0 {
					sw += sx0
					sx0 = 0
				}
				if sy0 < 0 {
					sh += sy0
					sy0 = 0
				}
				if sx0+sw > iw {
					sw = iw - sx0
				}
				if sy0+sh > ih {
					sh = ih - sy0
				}
				if sw <= 0 || sh <= 0 {
					return jsc.Undefined()
				}
			}
			// 浏览器语义：putImageData 不受 transform/clip 影响。
			bm.Cv.Save()
			bm.Cv.ResetMatrix()
			rgba := make([]byte, 0, 4*sw*sh)
			for yy := sy0; yy < sy0+sh; yy++ {
				for xx := sx0; xx < sx0+sw; xx++ {
					i := yy*iw + xx
					r, _ := dObj.GetByKey(strconv.Itoa(i*4 + 0))
					g, _ := dObj.GetByKey(strconv.Itoa(i*4 + 1))
					b, _ := dObj.GetByKey(strconv.Itoa(i*4 + 2))
					al, _ := dObj.GetByKey(strconv.Itoa(i*4 + 3))
					ar := uint8(clampByte(r.ToNumber()))
					ag := uint8(clampByte(g.ToNumber()))
					ab := uint8(clampByte(b.ToNumber()))
					aa := uint8(clampByte(al.ToNumber()))
					if aa != 0 && aa != 255 {
						// 非预乘输入 → 预乘存储
						ar = uint8(int(ar) * int(aa) / 255)
						ag = uint8(int(ag) * int(aa) / 255)
						ab = uint8(int(ab) * int(aa) / 255)
					}
					rgba = append(rgba, ar, ag, ab, aa)
				}
			}
			src, err := skia.NewImageFromPixels(
				skia.NewImageInfo(sw, sh, skia.ColorTypeRGBA8888, skia.AlphaTypePremul),
				rgba, sw*4)
			if err == nil && src != nil {
				defer src.Release()
				bm.Cv.DrawImageFull(src, 0, 0, float64(sw), float64(sh),
					float64(dx+sx0), float64(dy+sy0), float64(sw), float64(sh), 1, skia.BlendModeSrcOver)
			}
			bm.Cv.Restore()
			return jsc.Undefined()
		}, 3)))

	// ── 线型（v1 仅接收调用；虚线绘制忽略）──────────────────────
	o.Set("setLineDash", jsc.FunctionValue(jsc.NewNativeFunction("setLineDash",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.dash = nil
			if len(a) == 0 {
				return jsc.Undefined()
			}
			arr := a[0].AsObject()
			if arr == nil {
				return jsc.Undefined()
			}
			lv, ok := arr.GetByKey("length")
			if !ok {
				return jsc.Undefined()
			}
			n := int(lv.ToNumber())
			vals := make([]float32, 0, n)
			allZero := true
			for i := 0; i < n; i++ {
				v, _ := arr.GetByKey(strconv.Itoa(i))
				f := v.ToNumber()
				if math.IsNaN(f) || f < 0 {
					f = 0
				}
				if f > 0 {
					allZero = false
				}
				vals = append(vals, float32(f))
			}
			// 规范：奇数个元素复制一遍凑成偶数；全 0 视为实线（无虚线）。
			if len(vals)%2 == 1 {
				vals = append(vals, vals...)
			}
			if !allZero && len(vals) >= 2 {
				s.dash = vals
			}
			return jsc.Undefined()
		}, 1)))
	o.Set("getLineDash", jsc.FunctionValue(jsc.NewNativeFunction("getLineDash",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			arr := jsc.NewObject(rt.ObjectPrototype())
			for i, v := range s.dash {
				arr.Set(strconv.Itoa(i), jsc.NumberValue(float64(v)))
			}
			arr.Set("length", jsc.NumberValue(float64(len(s.dash))))
			return jsc.ObjectValue(arr)
		}, 0)))

	return o
}

// numStr 把浮点数格式化为 JS 风格的最短表示（getTransform().toString 用）。
func numStr(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// appendArcTo 实现 canvas 2D 的 arcTo(x1,y1,x2,y2,r)：从当前点到「P0→P1、
// P1→P2 两条切线的切点之间」画一段圆弧（skia 与几何副本同步）。
//
// 规范要点：当前点不存在时等价于 moveTo(x1,y1)；半径 0 或三点共线退化为
// lineTo(x1,y1)；半径大于可用切线长度时按规范放大（保证弧与两边相切）。
func appendArcTo(p *skia.Path, s *canvas2DCtxState, x1, y1, x2, y2, r float64) {
	p0, ok := s.geomCurPoint()
	if !ok {
		p.MoveTo(float32(x1), float32(y1))
		s.geomNewSubpath(x1, y1)
		return
	}
	if math.IsNaN(r) || r < 0 {
		r = 0
	}
	v1x, v1y := p0.X-x1, p0.Y-y1
	v2x, v2y := x2-x1, y2-y1
	l1, l2 := math.Hypot(v1x, v1y), math.Hypot(v2x, v2y)
	degenerate := func() {
		p.LineTo(float32(x1), float32(y1))
		s.geomLineTo(x1, y1)
	}
	if l1 == 0 || l2 == 0 || r == 0 {
		degenerate()
		return
	}
	cosT := (v1x*v2x + v1y*v2y) / (l1 * l2)
	if cosT > 1 {
		cosT = 1
	} else if cosT < -1 {
		cosT = -1
	}
	theta := math.Acos(cosT)
	if theta <= 1e-9 || math.IsNaN(theta) {
		degenerate()
		return
	}
	// 切点到 P1 的距离（半径过大时按规范夹取到可用切线长度）。
	tanLen := r / math.Tan(theta/2)
	if limit := math.Min(l1, l2); tanLen > limit {
		tanLen = limit
		r = tanLen * math.Tan(theta/2)
	}
	u1x, u1y := v1x/l1, v1y/l1
	u2x, u2y := v2x/l2, v2y/l2
	t1x, t1y := x1+u1x*tanLen, y1+u1y*tanLen
	t2x, t2y := x1+u2x*tanLen, y1+u2y*tanLen
	// 圆心在角平分线上。
	bx, by := u1x+u2x, u1y+u2y
	bl := math.Hypot(bx, by)
	if bl == 0 {
		degenerate()
		return
	}
	dist := r / math.Sin(theta/2)
	cx, cy := x1+bx/bl*dist, y1+by/bl*dist
	p.LineTo(float32(t1x), float32(t1y))
	s.geomLineTo(t1x, t1y)
	start := math.Atan2(t1y-cy, t1x-cx)
	end := math.Atan2(t2y-cy, t2x-cx)
	// 走短弧（两切点之间的弧 < π）：负向增量用 ccw=true。
	delta := end - start
	for delta > math.Pi {
		delta -= 2 * math.Pi
	}
	for delta <= -math.Pi {
		delta += 2 * math.Pi
	}
	ccw := delta < 0
	appendArc(p, cx, cy, r, start, end, ccw)
	s.geomArcPoints(cx, cy, r, r, start, end, 0, ccw)
}

// resolveRadius 解析单个圆角半径值（数字或 {x, y}）。
func resolveRadius(v jsc.JSValue) geomPoint {
	if o := v.AsObject(); o != nil {
		xv, _ := o.GetByKey("x")
		yv, _ := o.GetByKey("y")
		return geomPoint{X: xv.ToNumber(), Y: yv.ToNumber()}
	}
	f := v.ToNumber()
	if math.IsNaN(f) || f < 0 {
		f = 0
	}
	return geomPoint{X: f, Y: f}
}

// parseRoundRectRadii 解析 roundRect 的 radii 参数（CSS 圆角简写顺序：
// 单值 / [tl,br] / [tl,tr,br] / [tl,tr,br,bl]，元素可为数字或 {x,y}）。
func parseRoundRectRadii(a []jsc.JSValue, idx int) [4]geomPoint {
	var out [4]geomPoint
	if len(a) <= idx {
		return out
	}
	v := a[idx]
	obj := v.AsObject()
	if obj == nil {
		r := resolveRadius(v)
		for i := 0; i < 4; i++ {
			out[i] = r
		}
		return out
	}
	lv, ok := obj.GetByKey("length")
	if !ok {
		// 单个 {x, y}
		r := resolveRadius(v)
		for i := 0; i < 4; i++ {
			out[i] = r
		}
		return out
	}
	n := int(lv.ToNumber())
	var vals [4]geomPoint
	got := 0
	for i := 0; i < n && i < 4; i++ {
		item, _ := obj.GetByKey(strconv.Itoa(i))
		vals[i] = resolveRadius(item)
		got++
	}
	if got == 0 {
		return out
	}
	switch got {
	case 1:
		for i := 0; i < 4; i++ {
			out[i] = vals[0]
		}
	case 2:
		out[0], out[1], out[2], out[3] = vals[0], vals[0], vals[1], vals[1]
	case 3:
		out[0], out[1], out[2], out[3] = vals[0], vals[1], vals[2], vals[1]
	default:
		out = vals
	}
	return out
}

// appendRoundRectCorner 用一段三次贝塞尔近似椭圆角弧（≤90°），同步几何副本。
func appendRoundRectCorner(p *skia.Path, s *canvas2DCtxState, cx, cy, rx, ry, a0, a1 float64) {
	if rx <= 0 || ry <= 0 {
		p.LineTo(float32(cx), float32(cy))
		s.geomLineTo(cx, cy)
		return
	}
	x0 := cx + rx*math.Cos(a0)
	y0 := cy + ry*math.Sin(a0)
	x1 := cx + rx*math.Cos(a1)
	y1 := cy + ry*math.Sin(a1)
	k := 4.0 / 3.0 * math.Tan((a1-a0)/4.0)
	c1x, c1y := x0-k*rx*math.Sin(a0), y0+k*ry*math.Cos(a0)
	c2x, c2y := x1+k*rx*math.Sin(a1), y1-k*ry*math.Cos(a1)
	p.CubicTo(float32(c1x), float32(c1y), float32(c2x), float32(c2y), float32(x1), float32(y1))
	s.geomSampleCubic(c1x, c1y, c2x, c2y, x1, y1, 8)
}

// appendRoundRect 构造圆角矩形路径（radii 顺序 tl/tr/br/bl；负值归零、
// 超过半宽半高按规范夹取）。
func appendRoundRect(p *skia.Path, s *canvas2DCtxState, x, y, w, h float64, radii [4]geomPoint) {
	if w < 0 {
		x += w
		w = -w
	}
	if h < 0 {
		y += h
		h = -h
	}
	clampR := func(r geomPoint) geomPoint {
		out := r
		if math.IsNaN(out.X) || out.X < 0 {
			out.X = 0
		}
		if math.IsNaN(out.Y) || out.Y < 0 {
			out.Y = 0
		}
		if out.X > w/2 {
			out.X = w / 2
		}
		if out.Y > h/2 {
			out.Y = h / 2
		}
		return out
	}
	tl, tr, br, bl := clampR(radii[0]), clampR(radii[1]), clampR(radii[2]), clampR(radii[3])
	const halfPi = math.Pi / 2
	p.MoveTo(float32(x+tl.X), float32(y))
	s.geomNewSubpath(x+tl.X, y)
	p.LineTo(float32(x+w-tr.X), float32(y))
	s.geomLineTo(x+w-tr.X, y)
	appendRoundRectCorner(p, s, x+w-tr.X, y+tr.Y, tr.X, tr.Y, -halfPi, 0)
	p.LineTo(float32(x+w), float32(y+h-br.Y))
	s.geomLineTo(x+w, y+h-br.Y)
	appendRoundRectCorner(p, s, x+w-br.X, y+h-br.Y, br.X, br.Y, 0, halfPi)
	p.LineTo(float32(x+bl.X), float32(y+h))
	s.geomLineTo(x+bl.X, y+h)
	appendRoundRectCorner(p, s, x+bl.X, y+h-bl.Y, bl.X, bl.Y, halfPi, math.Pi)
	p.LineTo(float32(x), float32(y+tl.Y))
	s.geomLineTo(x, y+tl.Y)
	appendRoundRectCorner(p, s, x+tl.X, y+tl.Y, tl.X, tl.Y, math.Pi, 3*halfPi)
	p.Close()
	s.geomClose()
}

// appendEllipse 追加椭圆弧（rx/ry 半径，a0..a1 角，忽略 rotation——v1）。
func appendEllipse(p *skia.Path, cx, cy, rx, ry, a0, a1 float64, ccw bool) {
	if rx == 0 || ry == 0 {
		p.LineTo(float32(cx), float32(cy))
		return
	}
	// 用单位圆弧采样 + 缩放近似：先在单位圆上画弧，再仿射到椭圆。
	// （直接分段三次贝塞尔：k = 4/3 * tan(δ/4) * r）
	delta := a1 - a0
	if !ccw {
		if delta <= -2*math.Pi {
			delta = 2 * math.Pi
		} else if delta < 0 {
			delta += 2 * math.Pi
		} else if delta > 2*math.Pi {
			delta = 2 * math.Pi
		}
	} else {
		if delta >= 2*math.Pi {
			delta = -2 * math.Pi
		} else if delta > 0 {
			delta -= 2 * math.Pi
		} else if delta < -2*math.Pi {
			delta = -2 * math.Pi
		}
	}
	segs := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	if segs < 1 {
		segs = 1
	}
	step := delta / float64(segs)
	sx := cx + rx*math.Cos(a0)
	sy := cy + ry*math.Sin(a0)
	p.MoveTo(float32(sx), float32(sy))
	// 椭圆弧的切线控制点：单位圆控制点缩放 rx/ry 即可
	for i := 0; i < segs; i++ {
		b0 := a0 + step*float64(i)
		b1 := b0 + step
		x0 := rx * math.Cos(b0)
		y0 := ry * math.Sin(b0)
		x1 := rx * math.Cos(b1)
		y1 := ry * math.Sin(b1)
		k := 4.0 / 3.0 * math.Tan(step/4.0)
		dx0 := -rx * math.Sin(b0) * k
		dy0 := ry * math.Cos(b0) * k
		dx1 := -rx * math.Sin(b1) * k
		dy1 := ry * math.Cos(b1) * k
		p.CubicTo(float32(cx+x0+dx0), float32(cy+y0+dy0), float32(cx+x1-dx1), float32(cy+y1-dy1), float32(cx+x1), float32(cy+y1))
	}
}

// ─── 参数 helpers ──────────────────────────────────────────────────

// exportFloat32Array 从 Float32Array JS 值导出为 []float32（goja typed
// array Export 共享底层数据、零拷贝）；非 Float32Array 返回 nil。
func exportFloat32Array(v jsc.JSValue) []float32 {
	if !v.IsObject() {
		return nil
	}
	if f, ok := v.Export().([]float32); ok {
		return f
	}
	return nil
}

// exportUint16Array 从 Uint16Array JS 值导出为 []uint16（零拷贝）。
func exportUint16Array(v jsc.JSValue) []uint16 {
	if !v.IsObject() {
		return nil
	}
	if u, ok := v.Export().([]uint16); ok {
		return u
	}
	return nil
}

func argNum(a []jsc.JSValue, i int, def float64) float64 {
	if i < len(a) && !a[i].IsUndefined() && !a[i].IsNull() {
		return a[i].ToNumber()
	}
	return def
}

func argStr(a []jsc.JSValue, i int, def string) string {
	if i < len(a) && !a[i].IsUndefined() && !a[i].IsNull() {
		return a[i].ToString()
	}
	return def
}

func argBool(a []jsc.JSValue, i int, def bool) bool {
	if i < len(a) && !a[i].IsUndefined() && !a[i].IsNull() {
		return a[i].ToBoolean()
	}
	return def
}

func propOf(o *jsc.JSObject, key string) jsc.JSValue {
	if v, ok := o.GetByKey(key); ok {
		return v
	}
	return jsc.Undefined()
}

func clampByte(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}


