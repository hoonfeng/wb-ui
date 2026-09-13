// Translation of: Source/WebCore/animation/AnimationTimeline.cpp
//                  Source/WebCore/animation/KeyframeEffect.cpp
//                  Source/WebCore/animation/KeyframeModel.cpp
//                  Source/WebCore/animation/CSSAnimation.cpp
// Completeness: 45%
// Simplifications:
//   - timing-function uses simplified cubic-bezier approximation (non-analytic)
//   - no animation-composition (replace/add/accumulate)
//   - no animation-range (scroll-timeline based)
//   - no animation-play-state (always running)
//   - @keyframes are found via the global KeyframesLookup callback
package rendering

import (
	"log"
	"math"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/debugenv"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// AnimationTime is the global animation clock in seconds, set by the embedder
// (e.g. app.Host) each frame before calling Paint. It drives @keyframes-based
// animations.
var AnimationTime float64

// KeyframesLookup is a function that returns the @keyframes rule with the
// given name, or nil if not found. The embedder sets this to bridge to the
// style resolver's keyframes collection.
//
// ★ 多 WebView 安全：KeyframesLookup 是包级全局，多个 WebView（宿主多
// 挂件/多窗口）互相覆盖会让 A 页面的 @keyframes 用 B 页面的 resolver 查
// 询 → A 动画查不到关键帧直接失效（确定性污染）。因此所有查询路径优先
// 使用 rv.Resolver()（RenderTreeBuilder.Build 已自动挂接），全局变量仅
// 作为无 resolver 的旧嵌入路径兜底。
var KeyframesLookup func(name string) *css.KeyframesRule

// keyframesFor 返回某 RenderView 的 @keyframes 查询函数：首选 rv 绑定的
// resolver（每 WebView 独立，互不污染），无 resolver 时回退全局兜底。
func keyframesFor(rv *RenderView) func(name string) *css.KeyframesRule {
	if rv != nil {
		if rs := rv.Resolver(); rs != nil {
			return rs.LookupKeyframes
		}
	}
	return KeyframesLookup
}

// --- Top-level driver -------------------------------------------------------

// ApplyAnimations walks the render tree and updates each element's animated
// properties (opacity, transform translate/scale, color, background-color)
// based on the current AnimationTime and the element's animation properties.
// It also drives CSS transitions (:hover / :checked style changes).
// This must be called before Paint each frame. It returns true when a
// keyframe animation is still in its active phase or a transition is in
// flight (the host should re-layout AND re-paint this frame; when false the
// host may skip painting entirely — no animation state changed).
func ApplyAnimations(rv *RenderView) bool {
	if rv == nil {
		return false
	}
	active := false
	if kfLookup := keyframesFor(rv); kfLookup != nil {
		var walk func(o RenderObject)
		walk = func(o RenderObject) {
			if o == nil {
				return
			}
			st := o.Style()
			if st != nil && st.AnimationName != "" {
				if debugenv.Enabled("WB_ANIM_DEBUG") {
					kf := kfLookup(st.AnimationName)
					log.Printf("[anim] name=%q kf=%v", st.AnimationName, kf != nil)
				}
				if applyAnimationToStyle(st, AnimationTime, kfLookup) {
					active = true
					if debugenv.Enabled("WB_ANIM_DEBUG") {
						log.Printf("[anim] name=%q time=%.2f opacity=%.2f static=%.2f bgAnim=(%d,%d,%d,%d) bgStatic=(%d,%d,%d,%d)",
							st.AnimationName, AnimationTime, st.Opacity, st.StaticOpacity,
							st.AnimatedBackgroundColor.R, st.AnimatedBackgroundColor.G, st.AnimatedBackgroundColor.B, st.AnimatedBackgroundColor.A,
							st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
					}
				}
			}
			for c := o.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
			}
		}
		walk(RenderObject(rv))
	}
	if applyTransitions(rv, AnimationTime) {
		active = true
	}
	return active
}

// layoutAffectingAnimationProps 是影响布局的动画属性（transform 系列与
// 几何属性）——这类动画需要每帧 relayout；颜色/opacity 动画只影响绘制
// （浏览器合成器语义：颜色动画不触发布局）。
var layoutAffectingAnimationProps = map[string]bool{
	"transform": true, "translatex": true, "translatey": true,
	"scale": true, "scalex": true, "scaley": true,
	"left": true, "top": true, "right": true, "bottom": true,
	"width": true, "height": true, "margin": true, "marginleft": true,
	"margintop": true, "marginright": true, "marginbottom": true,
	"padding": true, "flex": true, "flexgrow": true, "flexshrink": true,
	"order": true, "gridcolumn": true, "gridrow": true,
}

// AnimationsAffectLayout 检查当前是否有激活动画影响布局（transform 系列
// /几何属性）。宿主用它决定动画帧是否 SetNeedsLayout——否则光标闪烁等
// 无限颜色动画每帧触发全量 relayout（复杂页面 100ms+）→ 帧率暴跌
// （「频繁无响应」）。颜色/opacity 动画只需重绘（needPaint 已覆盖）。
func AnimationsAffectLayout(rv *RenderView) bool {
	if rv == nil {
		return false
	}
	if kfLookup := keyframesFor(rv); kfLookup == nil {
		return false
	}
	affect := false
	kfLookup := keyframesFor(rv)
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if affect || o == nil {
			return
		}
		st := o.Style()
		if st != nil && st.AnimationName != "" {
			if kf := kfLookup(st.AnimationName); kf != nil {
				for _, rule := range kf.Keyframes {
					for _, d := range rule.Declarations {
						if layoutAffectingAnimationProps[strings.ToLower(d.Name)] {
							affect = true
							return
						}
					}
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
	return affect
}

// applyAnimationToStyle computes all animated properties for a single element
// and writes them directly into its ComputedStyle. It returns true while the
// animation is in its active phase (delay included): the caller uses this to
// decide whether the frame needs a re-paint. Once the animation has ended the
// final keyframe value (progress=1.0) was already applied in the last active
// frame, so returning false lets the host stop re-painting.
func applyAnimationToStyle(st *style.ComputedStyle, time float64,
	kfLookup func(name string) *css.KeyframesRule) bool {
	if kfLookup == nil {
		kfLookup = KeyframesLookup
	}
	kf := kfLookup(st.AnimationName)
	if kf == nil || len(kf.Keyframes) == 0 {
		// 无动画定义：动画驱动结束，清除驱动标志（painter 回退静态色）。
		st.AnimatedBackgroundActive = false
		st.AnimatedColorActive = false
		return false
	}
	duration := st.AnimationDuration
	if duration <= 0 {
		duration = 0.001 // prevent division by zero
	}

	// --- 1. Compute effective progress --------------------------------------

	// Apply delay.
	effectiveTime := time - st.AnimationDelay
	if effectiveTime < 0 {
		// Still in delay phase.
		if st.AnimationFillMode == "backwards" || st.AnimationFillMode == "both" {
			// Apply first keyframe.
			progress := 0.0
			applyProgressToStyle(st, kf, progress)
		}
		// Delay phase counts as active: the animation is pending and must
		// keep re-painting so its start is not missed.
		return true
	}

	// Determine iteration count and total duration.
	iterationCount := st.AnimationIterationCount
	infinite := iterationCount == 0
	var totalDuration float64
	if infinite {
		totalDuration = math.Inf(1)
	} else {
		totalDuration = duration * float64(iterationCount)
	}

	// Check if the animation has ended.
	ended := !infinite && effectiveTime > totalDuration
	if ended {
		if st.AnimationFillMode == "forwards" || st.AnimationFillMode == "both" {
			progress := 1.0
			applyProgressToStyle(st, kf, progress)
		} else {
			// 动画结束且无 forwards/both fill：动画值不再生效，清除驱动
			// 标志（painter 回退静态色，避免残留最后动画帧）。
			st.AnimatedBackgroundActive = false
			st.AnimatedColorActive = false
			// visibility 同样回退静态值（否则「淡出 + visibility:hidden」
			// 动画结束后元素永久不可见——它只是结束，不是隐藏指令）。
			if st.StaticVisibility != "" {
				st.Visibility = st.StaticVisibility
			} else {
				st.Visibility = "visible"
			}
		}
		// Ended: the final keyframe value was already rendered in the last
		// active frame (progress reached 1.0 there), so no re-paint needed.
		return false
	}

	// Compute which iteration and local progress.
	iter := int(effectiveTime / duration)
	// Clamp iteration to the last valid one (0-based) to ensure progress == 1.0
	// at the exact end of the final iteration.
	if !infinite && iter >= int(iterationCount) {
		iter = int(iterationCount) - 1
	}
	localT := effectiveTime - float64(iter)*duration
	if localT > duration {
		localT = duration
	}
	progress := localT / duration

	// Apply direction.
	direction := st.AnimationDirection
	if direction == "" {
		direction = "normal"
	}
	reverse := false
	switch direction {
	case "reverse":
		reverse = true
	case "alternate":
		reverse = (iter % 2) == 1
	case "alternate-reverse":
		reverse = (iter % 2) == 0
	}
	if reverse {
		progress = 1.0 - progress
	}

	// Apply timing-function.
	progress = applyTimingFunction(progress, st.AnimationTimingFunction)

	// Clamp.
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// --- 2. Interpolate keyframe values -------------------------------------
	applyProgressToStyle(st, kf, progress)
	// Active phase: animation state changed this frame → needs re-paint.
	return true
}

// applyProgressToStyle applies the interpolated keyframe values at the given
// progress (0.0–1.0) to the ComputedStyle.
func applyProgressToStyle(st *style.ComputedStyle, kf *css.KeyframesRule, progress float64) {
	// Opacity
	// ★ base = st.StaticOpacity：cm-blink 只有 50% 声明 opacity:0，
	// 0%/100% 未声明 → 用静态值（通常 1.0）→ 动画在 1↔0 之间闪烁
	// （此前插值对未声明端点返回 false → st.Opacity 永不变 → 光标不闪）。
	if opacity, ok := interpolateKeyframeFloat(kf, progress, "opacity", st.StaticOpacity); ok {
		st.Opacity = opacity
	}

	// Transform: translateX / translateY
	if tx, ok := interpolateTransformTranslate(kf, progress, "translateX"); ok {
		st.TranslateX = tx
	}
	if ty, ok := interpolateTransformTranslate(kf, progress, "translateY"); ok {
		st.TranslateY = ty
	}

	// Transform: scale / scaleX / scaleY
	if s, ok := interpolateTransformScale(kf, progress); ok {
		st.ScaleX = s
		st.ScaleY = s
	}
	if sx, ok := interpolateKeyframeFloat(kf, progress, "scaleX", 1.0); ok {
		st.ScaleX = sx
	}
	if sy, ok := interpolateKeyframeFloat(kf, progress, "scaleY", 1.0); ok {
		st.ScaleY = sy
	}

	// Color
	if c, ok := interpolateKeyframeColor(kf, progress, "color"); ok {
		st.AnimatedColor = style.Color{R: c.R, G: c.G, B: c.B, A: c.A}
		st.AnimatedColorActive = true
	}
	if bg, ok := interpolateKeyframeColor(kf, progress, "background-color"); ok {
		st.AnimatedBackgroundColor = style.Color{R: bg.R, G: bg.G, B: bg.B, A: bg.A}
		st.AnimatedBackgroundActive = true
	}

	// Visibility（离散可插值）。CSS-ANIM 规定：visibility 按离散属性插值，
	// **但**当区间任一端点是 visible 时，区间内插值结果就是 visible ——
	// 这正是「淡出并在结束帧 visibility:hidden」（fixture
	// animation-fill-forwards 的 @keyframes dismiss-overlay）应有的行为：
	// 元素在动画过程中始终 visible，只在到达终态时消失。
	// 此前完全不处理该属性 ⇒ 结束帧的 visibility:hidden 对引擎状态无效
	// （元素 opacity 归零但仍占位、仍参与绘制剔除与命中判定），
	// fill:forwards 的终态因此只实现了一半。
	if vis, ok := interpolateKeyframeVisibility(kf, progress, st.StaticVisibility); ok {
		st.Visibility = vis
	}
}

// --- Timing function --------------------------------------------------------

// applyTimingFunction applies a CSS timing function to a progress value [0,1].
func applyTimingFunction(t float64, tf string) float64 {
	switch tf {
	case "", "linear":
		return t
	case "ease":
		// cubic-bezier(0.25, 0.1, 0.25, 1.0)
		return cubicBezier(t, 0.25, 0.1, 0.25, 1.0)
	case "ease-in":
		// cubic-bezier(0.42, 0.0, 1.0, 1.0)
		return cubicBezier(t, 0.42, 0.0, 1.0, 1.0)
	case "ease-out":
		// cubic-bezier(0.0, 0.0, 0.58, 1.0)
		return cubicBezier(t, 0.0, 0.0, 0.58, 1.0)
	case "ease-in-out":
		// cubic-bezier(0.42, 0.0, 0.58, 1.0)
		return cubicBezier(t, 0.42, 0.0, 0.58, 1.0)
	case "step-start":
		// Jump to the end immediately: stays at 1 for all t>0.
		if t > 0 {
			return 1
		}
		return 0
	case "step-end":
		// ★ CM6 光标闪烁（step-end = steps(1)）：跳变点位于关键帧序列
		// 中点（50%）。0~50% 保持 0% 帧值（光标可见 opacity:1），
		// 50%~100% 跳变到 50% 帧值（opacity:0，隐藏）——硬切换闪烁，
		// 非线性渐变（用户「光标不闪」修复核心：此前 step-end 走线性
		// 近似且关键帧插值对未声明端点失效，opacity 从不变化）。
		if t >= 0.5 {
			return 0.5 // 采样 50% 关键帧值（隐藏相位）
		}
		return 0 // 采样 0% 关键帧值（可见相位）
	default:
		// steps(N[, start|end]) — CM6 光标闪烁用 steps(1)（等价 step-end：
		// 0~50% 保持 opacity:1，50%~100% 跳变 opacity:0，硬切换闪烁）。
		if n, ok := parseStepsCount(tf); ok {
			start := strings.Contains(tf, "start")
			if n == 1 {
				// steps(1) = step-end：跳变点在 50%（关键帧序列中点），
				// 0~50% 用 0% 帧（可见），50%~100% 用 50% 帧（隐藏）。
				if t >= 0.5 {
					return 0.5
				}
				return 0
			}
			step := 1.0 / float64(n)
			idx := int(t / step)
			if idx >= n {
				idx = n - 1
			}
			if start {
				// steps(…, start): value jumps at the beginning of each step.
				return float64(idx+1) / float64(n)
			}
			// steps(…, end) default: value jumps at the end of each step.
			return float64(idx) / float64(n)
		}
		return t
	}
}

// parseStepsCount extracts N from "steps(N)" / "steps(N, start|end)".
func parseStepsCount(tf string) (int, bool) {
	t := strings.TrimSpace(tf)
	lower := strings.ToLower(t)
	if !strings.HasPrefix(lower, "steps(") {
		return 0, false
	}
	inner := t[len("steps("):]
	if i := strings.Index(inner, ")"); i >= 0 {
		inner = inner[:i]
	}
	// Split on comma (may be "4, end" / "4,end").
	first := inner
	if i := strings.Index(inner, ","); i >= 0 {
		first = strings.TrimSpace(inner[:i])
	}
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// cubicBezier evaluates a cubic Bezier curve at parameter t using
// de Casteljau's algorithm. This is a simplified numeric approximation
// (not the analytic root-finding that WebKit uses).
func cubicBezier(t, x1, y1, x2, y2 float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	// Binary search to find the Bezier parameter u such that X(u) = t.
	// Then evaluate Y(u).
	lo, hi := 0.0, 1.0
	for i := 0; i < 20; i++ {
		mid := (lo + hi) / 2
		x := bezierComponent(mid, x1, x2)
		if x < t {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (lo + hi) / 2
	return bezierComponent(u, y1, y2)
}

// bezierComponent evaluates the X or Y component of a cubic Bezier with
// control points (0,0), (x1,y1), (x2,y2), (1,1) at parameter u.
func bezierComponent(u, c1, c2 float64) float64 {
	// B(u) = 3(1-u)²u·c1 + 3(1-u)u²·c2 + u³
	u2 := u * u
	u3 := u2 * u
	omu := 1 - u
	omu2 := omu * omu
	return 3*omu2*u*c1 + 3*omu*u2*c2 + u3
}

// --- Keyframe interpolation helpers -----------------------------------------

// keyframePointFloat holds a parsed numeric value at a keyframe offset.
type keyframePointFloat struct {
	offset float64
	value  float64
	valid  bool
}

// keyframePointColor holds a parsed color at a keyframe offset.
type keyframePointColor struct {
	offset float64
	value  graphics.Color
	valid  bool
}

// collectKeyframeFloats collects all keyframe offsets for a given property
// name from the @keyframes rule.
func collectKeyframeFloats(kf *css.KeyframesRule, propName string) []keyframePointFloat {
	seen := map[float64]bool{}
	var points []keyframePointFloat
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findFloatInDecls(rule.Declarations, propName)
			points = append(points, keyframePointFloat{offset: offset, value: val, valid: ok})
		}
	}
	sortKeyframes(points)
	return points
}

// collectKeyframeColors collects all keyframe offsets for a color property.
func collectKeyframeColors(kf *css.KeyframesRule, propName string) []keyframePointColor {
	seen := map[float64]bool{}
	var points []keyframePointColor
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findColorInDecls(rule.Declarations, propName)
			points = append(points, keyframePointColor{offset: offset, value: val, valid: ok})
		}
	}
	sortKeyframesColor(points)
	return points
}

// interpolateKeyframeFloat interpolates a numeric property at the given progress.
// base is the element's static computed value, used for undeclared keyframes
// (CSS: an undeclared keyframe keeps the static value). Pass st.StaticOpacity
// for opacity so cm-blink (only 50% declares opacity) animates 1→0→1.
func interpolateKeyframeFloat(kf *css.KeyframesRule, progress float64, propName string, base float64) (float64, bool) {
	points := collectKeyframeFloats(kf, propName)
	return interpolateFloatAt(points, progress, base)
}

// interpolateKeyframeColor interpolates a color property at the given progress.
func interpolateKeyframeColor(kf *css.KeyframesRule, progress float64, propName string) (graphics.Color, bool) {
	points := collectKeyframeColors(kf, propName)
	return interpolateColorAt(points, progress)
}

// keyframePointVisibility holds a visibility keyword at a keyframe offset.
type keyframePointVisibility struct {
	offset float64
	value  string // "visible" | "hidden" | "collapse"
	valid  bool
}

// collectKeyframeVisibility collects every keyframe offset that declares
// visibility, sorted by offset.
func collectKeyframeVisibility(kf *css.KeyframesRule) []keyframePointVisibility {
	seen := map[float64]bool{}
	var points []keyframePointVisibility
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findKeywordInDecls(rule.Declarations, "visibility")
			points = append(points, keyframePointVisibility{offset: offset, value: val, valid: ok})
		}
	}
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
	return points
}

// interpolateKeyframeVisibility evaluates the discrete `visibility` property at
// the given progress.
//
// CSS Animations interpolates visibility discretely, with one exception: if
// either endpoint of the interval is `visible`, the interpolated value is
// `visible` for the whole interval. That exception is what makes the common
// dismiss pattern (`to { opacity: 0; visibility: hidden }`) behave like a real
// fade — the element stays visible while opacity animates, and only disappears
// at the very end.
//
// base is the element's static computed value (st.StaticVisibility), used for
// keyframes that do not declare the property.
func interpolateKeyframeVisibility(kf *css.KeyframesRule, progress float64, base string) (string, bool) {
	points := collectKeyframeVisibility(kf)
	if len(points) == 0 {
		return "", false
	}
	hasValid := false
	for _, p := range points {
		if p.valid {
			hasValid = true
			break
		}
	}
	if !hasValid {
		// 属性在所有关键帧都未声明 → 不设置（与 interpolateFloatAt 同规则：
		// 不能让 base 回退把「无动画」误报为「动画值 = base」）。
		return "", false
	}
	baseVis := strings.ToLower(strings.TrimSpace(base))
	if baseVis == "" {
		baseVis = "visible"
	}
	eff := func(i int) string {
		if points[i].valid {
			return points[i].value
		}
		return baseVis
	}
	// 区间合并：任一端点 visible ⇒ visible（见函数注释）；否则离散取起点。
	merge := func(a, b string) string {
		if a == "visible" || b == "visible" {
			return "visible"
		}
		return a
	}
	if len(points) == 1 {
		p := points[0]
		if progress <= p.offset {
			return merge(baseVis, eff(0)), true
		}
		return merge(eff(0), baseVis), true
	}
	if progress <= points[0].offset {
		return eff(0), true
	}
	lastIdx := len(points) - 1
	if progress >= points[lastIdx].offset {
		return eff(lastIdx), true
	}
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			if progress == points[i].offset {
				return eff(i), true
			}
			if progress == points[i+1].offset {
				return eff(i + 1), true
			}
			return merge(eff(i), eff(i+1)), true
		}
	}
	return "", false
}

// interpolateTransformTranslate parses translateX or translateY from
// transform declarations at each keyframe.
func interpolateTransformTranslate(kf *css.KeyframesRule, progress float64, funcName string) (float64, bool) {
	seen := map[float64]bool{}
	var points []keyframePointFloat
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findTransformInDecls(rule.Declarations, funcName)
			points = append(points, keyframePointFloat{offset: offset, value: val, valid: ok})
		}
	}
	if len(points) == 0 {
		return 0, false
	}
	sortKeyframes(points)
	return interpolateFloatAt(points, progress, 0)
}

// interpolateTransformScale parses scale from transform declarations.
func interpolateTransformScale(kf *css.KeyframesRule, progress float64) (float64, bool) {
	return interpolateTransformTranslate(kf, progress, "scale")
}

// interpolateFloatAt does linear interpolation between sorted keyframe points.
// base is used for keyframes that do not declare the property (invalid
// points): CSS semantics say an undeclared keyframe keeps the element's
// static computed value. CM6's caret blink (@keyframes cm-blink only
// declares opacity at 50%; 0%/100% are empty) previously returned false
// for every progress → st.Opacity never animated → caret never blinked.
func interpolateFloatAt(points []keyframePointFloat, progress float64, base float64) (float64, bool) {
	if len(points) == 0 {
		return 0, false
	}
	// ★ 属性在所有关键帧都未声明（points 全 invalid，如 scaleX 在
	// transform:scale() 动画中）→ 返回 false（不设置），不能让 base 回退
	// 把「无动画」误报为「动画值=base」（否则 scaleX 分支覆盖 scale() 结果，
	// 且 cm-blink 的 0%/100% 未声明场景需要 base 但 50% 已声明）。
	hasValid := false
	for _, p := range points {
		if p.valid {
			hasValid = true
			break
		}
	}
	if !hasValid {
		return 0, false
	}
	// Resolve effective endpoints: invalid keyframes (property undeclared)
	// take the static value. A point is "effective" if valid, otherwise the
	// base substitutes.
	eff := func(i int) float64 {
		if points[i].valid {
			return points[i].value
		}
		return base
	}
	// ★ 补齐隐式端点：CSS 关键帧未声明某属性时取静态值（base）。
	// 真实 CM6 的 @keyframes cm-blink 只有 50% 帧（css 解析器丢弃空
	// 块 0%/100%），points 可能只有 {0.5, 0} 一项——不加端点则任何
	// progress 都返回该点值 → opacity 恒 0 → 光标不闪。这里按浏览器
	// 语义补 0%/100% 虚拟端点（值=base），让插值在 1↔0↔1 之间变化
	// （与显式空帧行为一致）。
	if len(points) == 1 {
		p := points[0]
		if p.offset <= 0 {
			return p.value, true
		}
		if p.offset >= 1 {
			return p.value, true
		}
		// 单点在中部：0% 用 base，100% 用 base。
		if progress <= p.offset {
			t := progress / p.offset
			return base + (p.value-base)*t, true
		}
		t := (progress - p.offset) / (1 - p.offset)
		return p.value + (base-p.value)*t, true
	}
	// 首点 offset>0：前面补 base 虚拟点（隐式 0% 帧）。
	if points[0].offset > 0 && progress <= points[0].offset {
		t := progress / points[0].offset
		return base + (eff(0)-base)*t, true
	}
	// 尾点 offset<1：后面补 base 虚拟点（隐式 100% 帧）。
	lastIdx := len(points) - 1
	if points[lastIdx].offset < 1 && progress >= points[lastIdx].offset {
		t := (progress - points[lastIdx].offset) / (1 - points[lastIdx].offset)
		return eff(lastIdx) + (base-eff(lastIdx))*t, true
	}
	// If progress is before the first keyframe or after the last, use nearest.
	if progress <= points[0].offset {
		return eff(0), true
	}
	last := points[len(points)-1]
	if progress >= last.offset {
		return eff(len(points) - 1), true
	}
	// Find the two surrounding keyframes.
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			t := (progress - points[i].offset) / (points[i+1].offset - points[i].offset)
			return eff(i) + (eff(i+1)-eff(i))*t, true
		}
	}
	return 0, false
}

// interpolateColorAt does linear interpolation between sorted keyframe color points.
func interpolateColorAt(points []keyframePointColor, progress float64) (graphics.Color, bool) {
	if len(points) == 0 {
		return graphics.Color{}, false
	}
	if progress <= points[0].offset {
		return points[0].value, points[0].valid
	}
	last := points[len(points)-1]
	if progress >= last.offset {
		return last.value, last.valid
	}
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			if !points[i].valid || !points[i+1].valid {
				return graphics.Color{}, false
			}
			t := (progress - points[i].offset) / (points[i+1].offset - points[i].offset)
			return lerpColor(points[i].value, points[i+1].value, t), true
		}
	}
	return graphics.Color{}, false
}

// lerpColor linearly interpolates between two colors component-wise.
func lerpColor(a, b graphics.Color, t float64) graphics.Color {
	return graphics.Color{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// --- Sorting helpers --------------------------------------------------------

func sortKeyframes(points []keyframePointFloat) {
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
}

func sortKeyframesColor(points []keyframePointColor) {
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
}

// --- Parsing helpers --------------------------------------------------------

// parseKeyframeOffset converts a keyframe selector like "0%", "50%", "from",
// "to" into a 0.0–1.0 offset. Returns -1 for unrecognized selectors.
func parseKeyframeOffset(key string) float64 {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "from":
		return 0.0
	case "to":
		return 1.0
	}
	if strings.HasSuffix(key, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(key, "%"), 64); err == nil {
			return v / 100.0
		}
	}
	return -1
}

// findFloatInDecls searches declarations for a property and returns its
// numeric value. Returns (0, false) if not found.
func findFloatInDecls(decls []css.Declaration, propName string) (float64, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, propName) {
			val := strings.TrimSpace(d.ValueString())
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// findColorInDecls searches declarations for a color property.
func findColorInDecls(decls []css.Declaration, propName string) (graphics.Color, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, propName) {
			val := strings.TrimSpace(d.ValueString())
			// ★ background-color: inherit（xterm 光标闪烁动画 50% 帧）——
			// 语义是继承父元素背景色（光标与终端背景同色 → 视觉隐藏）。
			// 引擎无法在此处查父背景，用透明色等价（深色终端背景下
			// 透明与背景同色效果一致），保证 0%→50% 帧可插值（否则
			// valid=false → 动画整体不生效 → 光标不闪烁）。
			if strings.EqualFold(val, "inherit") {
				return graphics.Color{}, true
			}
			if c, ok := parseColorSimple(val); ok {
				return c, true
			}
		}
	}
	return graphics.Color{}, false
}

// findKeywordInDecls returns the lower-cased keyword value of a declaration
// (visibility and the other keyword-valued animatable properties).
func findKeywordInDecls(decls []css.Declaration, propName string) (string, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, propName) {
			v := strings.ToLower(strings.TrimSpace(d.ValueString()))
			if v == "" {
				return "", false
			}
			return v, true
		}
	}
	return "", false
}

// findTransformInDecls searches declarations for a transform property and
// extracts a specific function value (e.g. translateX(10px) → 10).
func findTransformInDecls(decls []css.Declaration, funcName string) (float64, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, "transform") {
			val := d.ValueString()
			return extractTransformFunc(val, funcName)
		}
	}
	return 0, false
}

// extractTransformFunc extracts the numeric value from a CSS transform
// function like translateX(10px), scale(1.5), rotate(45deg).
func extractTransformFunc(input, funcName string) (float64, bool) {
	lower := strings.ToLower(input)
	search := strings.ToLower(funcName) + "("
	idx := strings.Index(lower, search)
	if idx < 0 {
		return 0, false
	}
	start := idx + len(funcName) + 1
	if start >= len(input) {
		return 0, false
	}
	end := strings.IndexByte(input[start:], ')')
	if end < 0 {
		return 0, false
	}
	arg := strings.TrimSpace(input[start : start+end])
	return parseCSSNumber(arg)
}

// parseCSSNumber parses a CSS numeric value like "10px", "1.5", "45deg".
// Returns the numeric value (stripping the unit).
func parseCSSNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Find the boundary between number and unit.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	dotSeen := false
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			i++
		} else if s[i] == '.' && !dotSeen {
			dotSeen = true
			i++
		} else {
			break
		}
	}
	if i == 0 {
		return 0, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	return num, true
}

// parseColorSimple parses a simple CSS color string (#hex, rgb(), named).
func parseColorSimple(s string) (graphics.Color, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return graphics.Color{}, false
	}
	// #hex
	if s[0] == '#' {
		return parseHexColorSimple(s)
	}
	// rgb() / rgba()
	if strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")") {
		return parseRGBAFunc(s[5 : len(s)-1])
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		c, ok := parseRGBAFunc(s[4 : len(s)-1])
		if ok {
			c.A = 255
		}
		return c, ok
	}
	// Named colors (basic set).
	return namedColorSimple(s)
}

// parseHexColorSimple parses #rgb / #rrggbb / #rrggbbaa.
func parseHexColorSimple(s string) (graphics.Color, bool) {
	if len(s) < 2 {
		return graphics.Color{}, false
	}
	hex := s[1:]
	var r, g, b, a uint8
	a = 255
	switch len(hex) {
	case 3:
		r = hexNibble(hex[0]) * 17
		g = hexNibble(hex[1]) * 17
		b = hexNibble(hex[2]) * 17
	case 6:
		r = hexNibble(hex[0])*16 + hexNibble(hex[1])
		g = hexNibble(hex[2])*16 + hexNibble(hex[3])
		b = hexNibble(hex[4])*16 + hexNibble(hex[5])
	case 8:
		r = hexNibble(hex[0])*16 + hexNibble(hex[1])
		g = hexNibble(hex[2])*16 + hexNibble(hex[3])
		b = hexNibble(hex[4])*16 + hexNibble(hex[5])
		a = hexNibble(hex[6])*16 + hexNibble(hex[7])
	default:
		return graphics.Color{}, false
	}
	return graphics.Color{R: r, G: g, B: b, A: a}, true
}

func hexNibble(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// parseRGBAFunc parses the inner arguments of rgb()/rgba().
func parseRGBAFunc(args string) (graphics.Color, bool) {
	parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) < 3 {
		return graphics.Color{}, false
	}
	r := parseCSSByte(parts[0])
	g := parseCSSByte(parts[1])
	b := parseCSSByte(parts[2])
	a := uint8(255)
	if len(parts) >= 4 {
		a = parseCSSAlpha(parts[3])
	}
	return graphics.Color{R: r, G: g, B: b, A: a}, true
}

func parseCSSByte(s string) uint8 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

func parseCSSAlpha(s string) uint8 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 255
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return uint8(v * 255)
}

// namedColorSimple returns a basic named CSS color.
func namedColorSimple(name string) (graphics.Color, bool) {
	switch strings.ToLower(name) {
	case "black":
		return graphics.Color{R: 0, G: 0, B: 0, A: 255}, true
	case "white":
		return graphics.Color{R: 255, G: 255, B: 255, A: 255}, true
	case "red":
		return graphics.Color{R: 255, G: 0, B: 0, A: 255}, true
	case "green", "lime":
		return graphics.Color{R: 0, G: 255, B: 0, A: 255}, true
	case "blue":
		return graphics.Color{R: 0, G: 0, B: 255, A: 255}, true
	case "yellow":
		return graphics.Color{R: 255, G: 255, B: 0, A: 255}, true
	case "transparent":
		return graphics.Color{R: 0, G: 0, B: 0, A: 0}, true
	}
	return graphics.Color{}, false
}

// --- Existing helpers (preserved) -------------------------------------------

// CumulativeOpacity returns the effective opacity for a render object by
// multiplying its own opacity with all ancestors' opacities.
func CumulativeOpacity(o RenderObject) float64 {
	if o == nil {
		return 1.0
	}
	op := 1.0
	for cur := o; cur != nil; cur = cur.Parent() {
		st := cur.Style()
		if st == nil {
			continue
		}
		op *= st.Opacity
	}
	if op < 0 {
		op = 0
	}
	if op > 1 {
		op = 1
	}
	return op
}

// ApplyOpacityToColor multiplies the alpha channel of a graphics color by the
// given opacity factor.
func ApplyOpacityToColor(c graphics.Color, opacity float64) graphics.Color {
	if opacity >= 1.0 {
		return c
	}
	if opacity < 0 {
		opacity = 0
	}
	alpha := float64(c.A) * opacity
	if alpha > 255 {
		alpha = 255
	}
	return graphics.Color{R: c.R, G: c.G, B: c.B, A: uint8(alpha)}
}
