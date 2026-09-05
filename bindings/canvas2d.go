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
	// lineDash 由 JS 属性 lineDashOffset 之外独立阵列维护（v1 不参与绘制）。
	saved []canvas2DSaved
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

// resolveFill 解析 this 的 fillStyle/strokeStyle 属性 → (颜色, shader)。
// shader 非 nil 时调用方负责 Release。
func resolveStyle(o *jsc.JSObject, key string) (graphics.Color, *skia.Shader) {
	v, ok := o.GetByKey(key)
	if !ok || v.IsUndefined() || v.IsNull() {
		return graphics.Color{R: 0, G: 0, B: 0, A: 255}, nil
	}
	if v.IsString() {
		return c2dColor(v.ToString()), nil
	}
	if obj := v.AsObject(); obj != nil {
		if gv, ok2 := obj.GetByKey("__wbGradient"); ok2 {
			if gs := gv.ToString(); gs != "" && gs != "undefined" {
				return graphics.Color{A: 255}, buildGradientShader(obj)
			}
		}
	}
	return graphics.Color{R: 0, G: 0, B: 0, A: 255}, nil
}

// ─── path 构建 ──────────────────────────────────────────────────────

// newPathFor 返回新 path（或复用），beginPath 时旧 path 释放。
func (s *canvas2DCtxState) beginPath() {
	if s.path != nil {
		s.path.Release()
	}
	s.path = skia.NewPath()
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

	// ── 矩形 ─────────────────────────────────────────────────────
	o.Set("fillRect", jsc.FunctionValue(jsc.NewNativeFunction("fillRect",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil {
				return jsc.Undefined()
			}
			col, sh := resolveStyle(obj, "fillStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			x, y, w, h := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0)
			if sh != nil {
				defer sh.Release()
				bm.Cv.FillRectShader(x, y, w, h, sh, alpha, blend)
			} else {
				bm.Cv.FillRectFull(x, y, w, h, col, alpha, blend)
			}
			return jsc.Undefined()
		}, 4)))
	o.Set("strokeRect", jsc.FunctionValue(jsc.NewNativeFunction("strokeRect",
		func(_ *jsc.Interpreter, this jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil {
				return jsc.Undefined()
			}
			col, sh := resolveStyle(obj, "strokeStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			lw := c2dNum(obj, "lineWidth", 1)
			cap := c2dStr(obj, "lineCap", "butt")
			join := c2dStr(obj, "lineJoin", "miter")
			x, y, w, h := argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0), argNum(a, 3, 0)
			rp := skia.NewPath()
			defer rp.Release()
			rp.MoveTo(float32(x), float32(y))
			rp.LineTo(float32(x+w), float32(y))
			rp.LineTo(float32(x+w), float32(y+h))
			rp.LineTo(float32(x), float32(y+h))
			rp.Close()
			if sh != nil {
				defer sh.Release()
				bm.Cv.StrokePathShader(rp, lw, sh, alpha, cap, join, blend)
			} else {
				bm.Cv.StrokePathFull(rp, lw, col, alpha, cap, join, blend)
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
			return jsc.Undefined()
		}, 0)))
	o.Set("moveTo", jsc.FunctionValue(jsc.NewNativeFunction("moveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().MoveTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)))
			return jsc.Undefined()
		}, 2)))
	o.Set("lineTo", jsc.FunctionValue(jsc.NewNativeFunction("lineTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().LineTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)))
			return jsc.Undefined()
		}, 2)))
	o.Set("bezierCurveTo", jsc.FunctionValue(jsc.NewNativeFunction("bezierCurveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().CubicTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)), float32(argNum(a, 2, 0)), float32(argNum(a, 3, 0)), float32(argNum(a, 4, 0)), float32(argNum(a, 5, 0)))
			return jsc.Undefined()
		}, 6)))
	o.Set("quadraticCurveTo", jsc.FunctionValue(jsc.NewNativeFunction("quadraticCurveTo",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			s.curPath().QuadTo(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)), float32(argNum(a, 2, 0)), float32(argNum(a, 3, 0)))
			return jsc.Undefined()
		}, 4)))
	o.Set("arc", jsc.FunctionValue(jsc.NewNativeFunction("arc",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			appendArc(s.curPath(), argNum(a, 0, 0), argNum(a, 1, 0), argNum(a, 2, 0),
				argNum(a, 3, 0), argNum(a, 4, 0), argBool(a, 5, false))
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
			return jsc.Undefined()
		}, 7)))
	o.Set("rect", jsc.FunctionValue(jsc.NewNativeFunction("rect",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			r := skia.RectXYWH(float32(argNum(a, 0, 0)), float32(argNum(a, 1, 0)), float32(argNum(a, 2, 0)), float32(argNum(a, 3, 0)))
			s.curPath().AddRect(r, skia.PathDirectionCW)
			return jsc.Undefined()
		}, 4)))
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
			col, sh := resolveStyle(obj, "fillStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			if sh != nil {
				defer sh.Release()
				bm.Cv.FillPathShader(s.path, sh, alpha, blend)
			} else {
				bm.Cv.FillPathFull(s.path, col, alpha, blend)
			}
			return jsc.Undefined()
		}, 1)))
	o.Set("stroke", jsc.FunctionValue(jsc.NewNativeFunction("stroke",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			obj := this.AsObject()
			if obj == nil || bm.Cv == nil || s.path == nil {
				return jsc.Undefined()
			}
			col, sh := resolveStyle(obj, "strokeStyle")
			alpha := c2dNum(obj, "globalAlpha", 1)
			blend := c2dBlend(c2dStr(obj, "globalCompositeOperation", "source-over"))
			lw := c2dNum(obj, "lineWidth", 1)
			cap := c2dStr(obj, "lineCap", "butt")
			join := c2dStr(obj, "lineJoin", "miter")
			if sh != nil {
				defer sh.Release()
				bm.Cv.StrokePathShader(s.path, lw, sh, alpha, cap, join, blend)
			} else {
				bm.Cv.StrokePathFull(s.path, lw, col, alpha, cap, join, blend)
			}
			return jsc.Undefined()
		}, 0)))
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
			col, sh := resolveStyle(obj, "fillStyle")
			if sh != nil {
				sh.Release() // 文本 v1 不支持 shader 填充，回退纯色
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
			col, sh := resolveStyle(obj, "strokeStyle")
			if sh != nil {
				sh.Release()
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

	// ── 像素 ─────────────────────────────────────────────────────
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
			// 浏览器语义：putImageData 不受 transform/clip 影响。
			bm.Cv.Save()
			bm.Cv.ResetMatrix()
			rgba := make([]byte, 0, 4*iw*ih)
			for i := 0; i < iw*ih; i++ {
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
			src, err := skia.NewImageFromPixels(
				skia.NewImageInfo(iw, ih, skia.ColorTypeRGBA8888, skia.AlphaTypePremul),
				rgba, iw*4)
			if err == nil && src != nil {
				defer src.Release()
				bm.Cv.DrawImageFull(src, 0, 0, float64(iw), float64(ih), float64(dx), float64(dy), float64(iw), float64(ih), 1, skia.BlendModeSrcOver)
			}
			bm.Cv.Restore()
			return jsc.Undefined()
		}, 3)))

	// ── 线型（v1 仅接收调用；虚线绘制忽略）──────────────────────
	o.Set("setLineDash", jsc.FunctionValue(jsc.NewNativeFunction("setLineDash",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	o.Set("getLineDash", jsc.FunctionValue(jsc.NewNativeFunction("getLineDash",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			arr := jsc.NewObject(rt.ObjectPrototype())
			arr.Set("length", jsc.NumberValue(0))
			return jsc.ObjectValue(arr)
		}, 0)))

	return o
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


