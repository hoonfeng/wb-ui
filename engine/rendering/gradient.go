package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/engine/platform/graphics"
)

type GradientDirection struct {
	Angle   float64
	IsAngle bool
}

type ColorStop struct {
	Color    graphics.Color
	Position float64
	// PositionPx >= 0 表示该色标的位置以**长度**（px）给出，例如
	// `linear-gradient(#2a2a2a 1px, transparent 1px)` 的硬边条纹网格。
	// 长度位置的语义是「沿渐变线的距离」，而线长要到绘制期才知道，
	// 因此解析期只保留原始长度、把 Position 置 -1（未定），由
	// resolveGradientStops 在 paint 时按线长换算成比例。
	// -1 = 该色标没有长度位置。
	PositionPx float64
}

type LinearGradient struct {
	Direction GradientDirection
	Stops     []ColorStop
}

// RadialGradient describes a radial-gradient background (circle shape,
// centered in the box, radius = farthest-corner — the common case).
type RadialGradient struct {
	Stops  []ColorStop
	CenterX, CenterY float64 // relative 0..1
	// Shape "circle" (1) or "ellipse" (0 = default); radius is computed at
	// paint time from the box dimensions.
	Circle bool
}

// parseRadialGradient parses "radial-gradient(...)" (subset: circle/ellipse
// shape keyword, default center and farthest-corner sizing).
func parseRadialGradient(s string) *RadialGradient {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" || !strings.HasPrefix(s, "radial-gradient(") || !strings.HasSuffix(s, ")") {
		return nil
	}
	inner := s[len("radial-gradient(") : len(s)-1]
	parts := splitGradientArgs(inner)
	if len(parts) < 2 {
		return nil
	}
	rg := &RadialGradient{CenterX: 0.5, CenterY: 0.5}
	pos := 0
	first := strings.TrimSpace(parts[0])
	if strings.HasPrefix(first, "circle") || strings.HasPrefix(first, "ellipse") ||
		strings.HasPrefix(first, "closest") || strings.HasPrefix(first, "farthest") {
		if strings.HasPrefix(first, "circle") {
			rg.Circle = true
		}
		pos = 1
	} else {
		rg.Circle = true // default shape for the common case
	}
	for i := pos; i < len(parts); i++ {
		if stop := parseColorStop(strings.TrimSpace(parts[i])); stop != nil {
			rg.Stops = append(rg.Stops, *stop)
		}
	}
	if len(rg.Stops) < 2 {
		return nil
	}
	// 含长度位置时延后到绘制期（需要渐变线长度）再补全。
	if !hasPixelStop(rg.Stops) {
		assignGradientPositions(rg.Stops)
	}
	return rg
}

func parseGradient(s string) *LinearGradient {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" || !strings.HasPrefix(s, "linear-gradient(") || !strings.HasSuffix(s, ")") {
		return nil
	}
	inner := s[len("linear-gradient(") : len(s)-1]
	lg := &LinearGradient{}
	parts := splitGradientArgs(inner)
	if len(parts) < 2 {
		return nil
	}
	first := strings.TrimSpace(parts[0])
	pos := 0
	if dir := parseGradientDirection(first); dir != nil {
		lg.Direction = *dir
		pos = 1
	} else {
		lg.Direction = GradientDirection{Angle: 180}
	}
	for i := pos; i < len(parts); i++ {
		if stop := parseColorStop(strings.TrimSpace(parts[i])); stop != nil {
			lg.Stops = append(lg.Stops, *stop)
		}
	}
	if len(lg.Stops) < 2 {
		return nil
	}
	// 含长度位置时延后到绘制期（需要渐变线长度）再补全。
	if !hasPixelStop(lg.Stops) {
		assignGradientPositions(lg.Stops)
	}
	return lg
}

func parseGradientDirection(s string) *GradientDirection {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasSuffix(s, "deg") {
		a, err := strconv.ParseFloat(strings.TrimSuffix(s, "deg"), 64)
		if err == nil {
			return &GradientDirection{Angle: a, IsAngle: true}
		}
		return nil
	}
	s = strings.ToLower(s)
	if !strings.HasPrefix(s, "to ") {
		return nil
	}
	// Serialized CSS values may contain extra whitespace (e.g. "to  right"
	// after token re-serialization), so re-trim after removing the prefix.
	s = strings.TrimSpace(strings.TrimPrefix(s, "to "))
	switch s {
	case "top":
		return &GradientDirection{Angle: 0}
	case "right":
		return &GradientDirection{Angle: 90}
	case "bottom":
		return &GradientDirection{Angle: 180}
	case "left":
		return &GradientDirection{Angle: 270}
	case "top right", "right top":
		return &GradientDirection{Angle: 45}
	case "top left", "left top":
		return &GradientDirection{Angle: 315}
	case "bottom right", "right bottom":
		return &GradientDirection{Angle: 135}
	case "bottom left", "left bottom":
		return &GradientDirection{Angle: 225}
	}
	return &GradientDirection{Angle: 180}
}

// splitBackgroundLayers splits a background-image value into its comma-
// separated layers, tracking parenthesis depth so commas inside gradient
// function arguments are preserved.
func splitBackgroundLayers(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return nil
	}
	var layers []string
	depth := 0
	cur := strings.Builder{}
	for _, r := range s {
		switch r {
		case '(':
			depth++
			cur.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case ',':
			if depth == 0 {
				layers = append(layers, strings.TrimSpace(cur.String()))
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		layers = append(layers, strings.TrimSpace(cur.String()))
	}
	// drop empty trailing layers
	for len(layers) > 0 && layers[len(layers)-1] == "" {
		layers = layers[:len(layers)-1]
	}
	return layers
}

// paintRadialGradient fills the box with a radial gradient: each pixel's
// color is interpolated by its distance from the center over the radius
// (farthest-corner for circle, scaled axes for ellipse).
func paintRadialGradient(canvas *graphics.Canvas, x, y, w, h float64, rg *RadialGradient) {
	if rg == nil || len(rg.Stops) < 2 || w <= 0 || h <= 0 {
		return
	}
	cx := x + w*rg.CenterX
	cy := y + h*rg.CenterY
	iw, ih := int(math.Round(w)), int(math.Round(h))
	if rg.Circle {
		dx := math.Max(cx-x, x+w-cx)
		dy := math.Max(cy-y, y+h-cy)
		radius := math.Sqrt(dx*dx + dy*dy)
		if radius <= 0 {
			return
		}
		// 长度色标（`1px`）按径向渐变线（半径）换算。
		stops := resolveGradientStops(rg.Stops, radius)
		for py := 0; py < ih; py++ {
			for px := 0; px < iw; px++ {
				dxp := (x + float64(px) + 0.5) - cx
				dyp := (y + float64(py) + 0.5) - cy
				t := math.Sqrt(dxp*dxp+dyp*dyp) / radius
				if t > 1 {
					t = 1
				}
				canvas.FillRectNoAA(x+float64(px), y+float64(py), 1, 1, interpolateColor(stops, t))
			}
		}
		return
	}
	// Ellipse: normalize each axis independently (distance in ellipse space).
	radiusX := math.Max(cx-x, x+w-cx)
	radiusY := math.Max(cy-y, y+h-cy)
	if radiusX <= 0 || radiusY <= 0 {
		return
	}
	// 椭圆：长度位置按较大半径换算（<length> 位置沿渐变线量取）。
	stops := resolveGradientStops(rg.Stops, math.Max(radiusX, radiusY))
	for py := 0; py < ih; py++ {
		for px := 0; px < iw; px++ {
			dxn := ((x + float64(px) + 0.5) - cx) / radiusX
			dyn := ((y + float64(py) + 0.5) - cy) / radiusY
			t := math.Sqrt(dxn*dxn + dyn*dyn)
			if t > 1 {
				t = 1
			}
			canvas.FillRectNoAA(x+float64(px), y+float64(py), 1, 1, interpolateColor(stops, t))
		}
	}
}

// parseStopPositionToken 解析色标的位置 token（CSS 的
// `<length-percentage>`）：返回（数值, 是否百分比, 是否可用）。
// 长度目前支持 px（引擎布局的绝对单位）；em/rem 等相对长度需要字号
// 上下文，暂不支持（返回 false → 该 token 不被当作位置，与原行为一致）。
func parseStopPositionToken(tok string) (val float64, isPct bool, ok bool) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return 0, false, false
	}
	if strings.HasSuffix(tok, "%") {
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(tok, "%")), 64)
		if err != nil {
			return 0, false, false
		}
		return v, true, true
	}
	if strings.HasSuffix(tok, "px") {
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(tok, "px")), 64)
		if err != nil {
			return 0, false, false
		}
		return v, false, true
	}
	return 0, false, false
}

// parseColorStop 解析单个色标 `<color> [<length-percentage>]`。
//
// ★ 位置写在颜色**之后**，且颜色本身可能含空格（`rgb(42, 42, 42)`）→
// 只在**括号深度 0**的位置上取最后一个空格切分，避免切进函数参数内部。
//
// ★ 长度位置（`#2a2a2a 1px`）此前只认 `%`：整串 "#2a2a2a 1px" 被当作
// 颜色交给 parseColorSimple → 解析失败 → 色标被丢；两个色标都带长度位置时
// 整条渐变因 stops<2 被丢弃（实测 `linear-gradient(#2a2a2a 1px,transparent
// 1px)` 网格背景在真实页面里整片消失，而 Edge 正常）。
func parseColorStop(s string) *ColorStop {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	depth, lastSpace := 0, -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ':
			if depth == 0 {
				lastSpace = i
			}
		}
	}
	colorStr := s
	cs := &ColorStop{Position: -1, PositionPx: -1}
	if lastSpace > 0 {
		if val, isPct, ok := parseStopPositionToken(s[lastSpace+1:]); ok {
			colorStr = strings.TrimSpace(s[:lastSpace])
			if isPct {
				cs.Position = val / 100.0
			} else {
				cs.PositionPx = val
			}
		}
	}
	c, ok := parseColorSimple(colorStr)
	if !ok {
		return nil
	}
	cs.Color = c
	return cs
}

// hasPixelStop 报告色标集合里是否存在以长度（px）给出的位置。长度位置
// 必须等绘制期知道渐变线长度后再换算，故解析期若有它则**跳过**
// assignGradientPositions —— 否则长度色标会被当成「未指定位置」而被填入
// 插值位置，换算时再覆盖就成了错误的锚点。
func hasPixelStop(stops []ColorStop) bool {
	for _, s := range stops {
		if s.PositionPx >= 0 {
			return true
		}
	}
	return false
}

// resolveGradientStops 在已知渐变线长度后，把长度位置换算成比例（clamp 到
// [0,1]）并补齐未指定位置的色标。无长度色标时返回与入参逐字段相同的副本
// （Position 已由解析期的 assignGradientPositions 填好，不会重算）——保证
// 既有渐变路径逐像素不变。
func resolveGradientStops(stops []ColorStop, length float64) []ColorStop {
	out := make([]ColorStop, len(stops))
	copy(out, stops)
	needFill := false
	for i := range out {
		if out[i].PositionPx >= 0 {
			p := 0.0
			if length > 0 {
				p = out[i].PositionPx / length
			}
			if p < 0 {
				p = 0
			} else if p > 1 {
				p = 1
			}
			out[i].Position = p
			out[i].PositionPx = -1
		}
		if out[i].Position < 0 {
			needFill = true
		}
	}
	if needFill {
		assignGradientPositions(out)
	}
	return out
}

func assignGradientPositions(stops []ColorStop) {
	type ip struct {
		idx int
		pos float64
	}
	var expl []ip
	for i, s := range stops {
		if s.Position >= 0 {
			expl = append(expl, ip{i, s.Position})
		}
	}
	if len(expl) == 0 {
		for i := range stops {
			if len(stops) > 1 {
				stops[i].Position = float64(i) / float64(len(stops)-1)
			} else {
				stops[i].Position = 0
			}
		}
		return
	}
	for i := expl[0].idx - 1; i >= 0; i-- {
		if expl[0].idx > 0 {
			stops[i].Position = expl[0].pos - float64(expl[0].idx-i)/float64(expl[0].idx)*expl[0].pos
			if stops[i].Position < 0 {
				stops[i].Position = 0
			}
		}
	}
	for e := 0; e < len(expl)-1; e++ {
		si, ei := expl[e].idx, expl[e+1].idx
		sp, ep := expl[e].pos, expl[e+1].pos
		gap := ei - si
		for i := si + 1; i < ei; i++ {
			t := float64(i-si) / float64(gap)
			stops[i].Position = sp + (ep-sp)*t
		}
	}
	li := expl[len(expl)-1].idx
	lp := expl[len(expl)-1].pos
	for i := li + 1; i < len(stops); i++ {
		rem := len(stops) - 1 - li
		if rem > 0 {
			stops[i].Position = lp + float64(i-li)/float64(rem)*(1.0-lp)
			if stops[i].Position > 1 {
				stops[i].Position = 1
			}
		}
	}
}

func splitGradientArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func paintLinearGradient(canvas *graphics.Canvas, x, y, w, h float64, lg *LinearGradient) {
	if lg == nil || len(lg.Stops) < 2 || w <= 0 || h <= 0 {
		return
	}
	rad := lg.Direction.Angle * math.Pi / 180.0
	gx := math.Sin(rad)
	gy := -math.Cos(rad)
	cx := x + w/2
	cy := y + h/2
	corners := [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}}
	minP, maxP := 0.0, 0.0
	for i, c := range corners {
		proj := (c[0]-cx)*gx + (c[1]-cy)*gy
		if i == 0 || proj < minP {
			minP = proj
		}
		if i == 0 || proj > maxP {
			maxP = proj
		}
	}
	gradLen := maxP - minP
	if gradLen <= 0 {
		return
	}
	// ★ 一次性交给 Skia 渐变 shader（此前是逐像素 FillRectNoAA(1x1)：
	// 对角线渐变每个像素一次 Skia 调用，真实页面上是每帧数百毫秒的
	// 头号热点）。两者数值等价：Skia 的渐变参数
	//   t' = ((p-start)·(end-start)) / |end-start|²
	// 取 start = c + d·minP、end = c + d·maxP（d 为单位方向向量），则
	//   end-start = d·gradLen，|end-start|² = gradLen²，t' = ((p-c)·d - minP)/gradLen
	// 正是逐像素版的 t（且投影越界时两边都 clamp 到 [0,1]，Skia 侧由
	// TileModeClamp 保证）。多色标按 CSS 语义整段插值——旧版本对角分支
	// 曾用「首尾色 + 纵向近似」把 135deg 七色渐变画成纯色，这条路径不再
	// 存在（方向正确性由 shader 的轴端点天然保证）。
	// ★ 长度位置色标（`#2a2a2a 1px`）在此换算成比例：gradLen 即渐变线长度
	// （css 的 <length> 位置沿渐变线量取）。
	stops := resolveGradientStops(lg.Stops, gradLen)
	colors := make([]graphics.Color, len(stops))
	positions := make([]float32, len(stops))
	for i, s := range stops {
		colors[i] = s.Color
		positions[i] = float32(s.Position)
		// Skia 要求色标位置非递减；CSS 里手写越界/逆序的 stop 在此钳平，
		// 避免 shader 行为未定义。
		if i > 0 && positions[i] < positions[i-1] {
			positions[i] = positions[i-1]
		}
	}
	canvas.FillRectLinearGradientStops(x, y, w, h,
		cx+gx*minP, cy+gy*minP, cx+gx*maxP, cy+gy*maxP, colors, positions)
}

func interpolateColor(stops []ColorStop, t float64) graphics.Color {
	if len(stops) == 0 {
		return graphics.Color{}
	}
	if t <= stops[0].Position {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if t >= last.Position {
		return last.Color
	}
	for i := 0; i < len(stops)-1; i++ {
		if t >= stops[i].Position && t <= stops[i+1].Position {
			span := stops[i+1].Position - stops[i].Position
			if span <= 0 {
				return stops[i].Color
			}
			lt := (t - stops[i].Position) / span
			return lerpColor(stops[i].Color, stops[i+1].Color, lt)
		}
	}
	return stops[len(stops)-1].Color
}
