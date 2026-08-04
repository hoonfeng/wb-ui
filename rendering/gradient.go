package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/platform/graphics"
)

type GradientDirection struct {
	Angle   float64
	IsAngle bool
}

type ColorStop struct {
	Color    graphics.Color
	Position float64
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
	assignGradientPositions(rg.Stops)
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
	assignGradientPositions(lg.Stops)
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
		for py := 0; py < ih; py++ {
			for px := 0; px < iw; px++ {
				dxp := (x + float64(px) + 0.5) - cx
				dyp := (y + float64(py) + 0.5) - cy
				t := math.Sqrt(dxp*dxp+dyp*dyp) / radius
				if t > 1 {
					t = 1
				}
				canvas.FillRectNoAA(x+float64(px), y+float64(py), 1, 1, interpolateColor(rg.Stops, t))
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
	for py := 0; py < ih; py++ {
		for px := 0; px < iw; px++ {
			dxn := ((x + float64(px) + 0.5) - cx) / radiusX
			dyn := ((y + float64(py) + 0.5) - cy) / radiusY
			t := math.Sqrt(dxn*dxn + dyn*dyn)
			if t > 1 {
				t = 1
			}
			canvas.FillRectNoAA(x+float64(px), y+float64(py), 1, 1, interpolateColor(rg.Stops, t))
		}
	}
}

func parseColorStop(s string) *ColorStop {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	pctPos := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '%' {
			// Walk back over the position's numeric part only (digits and
			// decimal point). Must NOT consume the separating space, or
			// "#ff0000 0%" would fail (the space would be skipped and j
			// would land inside the color).
			j := i - 1
			for j >= 0 && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j--
			}
			if j >= 0 && s[j] == ' ' {
				pctPos = j
				break
			}
		}
	}
	var colorStr string
	var pct float64
	hasPos := pctPos >= 0
	if hasPos {
		colorStr = strings.TrimSpace(s[:pctPos])
		pctStr := strings.TrimSpace(s[pctPos+1:])
		pctStr = strings.TrimSuffix(pctStr, "%")
		v, err := strconv.ParseFloat(pctStr, 64)
		if err == nil {
			pct = v / 100.0
		} else {
			hasPos = false
		}
	} else {
		colorStr = s
	}
	c, ok := parseColorSimple(colorStr)
	if !ok {
		return nil
	}
	cs := &ColorStop{Color: c}
	if hasPos {
		cs.Position = pct
	} else {
		cs.Position = -1
	}
	return cs
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
	iw, ih := int(w), int(h)
	if math.Abs(gx) < 0.001 {
		for py := 0; py < ih; py++ {
			pos := y + float64(py) + 0.5
			t := ((pos-cy)*gy - minP) / gradLen
			if t > 1 {
				t = 1
			}
			if t < 0 {
				t = 0
			}
			canvas.FillRectNoAA(x, y+float64(py), w, 1, interpolateColor(lg.Stops, t))
		}
		return
	}
	if math.Abs(gy) < 0.001 {
		for px := 0; px < iw; px++ {
			pos := x + float64(px) + 0.5
			t := ((pos-cx)*gx - minP) / gradLen
			if t > 1 {
				t = 1
			}
			if t < 0 {
				t = 0
			}
			canvas.FillRectNoAA(x+float64(px), y, 1, h, interpolateColor(lg.Stops, t))
		}
		return
	}
	sc := lg.Stops[0].Color
	ec := lg.Stops[len(lg.Stops)-1].Color
	for py := 0; py < ih; py++ {
		t := float64(py) / float64(ih)
		canvas.FillRectNoAA(x, y+float64(py), w, 1, lerpColor(sc, ec, t))
	}
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
