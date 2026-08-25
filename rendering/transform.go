// CSS transform (non-animation) painting support.
//
// Completeness: 85%
// Simplifications:
//   - animation transforms are handled by the animation apply code, not here
//   - 3D transform functions (rotateX/Y, translateZ, matrix3d) are approximated
//     or treated as no-ops; only 2D operations are rendered faithfully
//
// Parses the static transform CSS property and applies it to the canvas
// during painting. Supported functions: translate(), translateX(), translateY(),
// translateZ(), translate3d(), scale(), scaleX(), scaleY(), rotate(),
// rotateX(), rotateY(), rotateZ(), skew(), skewX(), skewY(), matrix().
// Multiple functions can be combined:
//
//	transform: translateX(10px) scale(1.5) rotate(45deg) skewX(10deg)

package rendering

import (
	"math"
	"strconv"
	"strings"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/platform/graphics"
)

// tryApplyTransform checks if el has a non-empty Transform style, parses it,
// saves the canvas, applies the transform operations, and returns a cleanup
// function that restores the canvas. Returns nil if no transform is present.
// The caller must call the returned function (if non-nil) after painting.
//
// Usage:
//
//	cleanup := tryApplyTransform(canvas, st)
//	if cleanup != nil {
//	    defer cleanup()
//	}
func tryApplyTransform(canvas *graphics.Canvas, box *RenderBox) func() {
	if canvas == nil || box == nil {
		return nil
	}
	st := box.Style()
	if st == nil {
		return nil
	}
	// Only apply non-animated transforms when the animation is not running.
	// (Animation sets AnimationName; when it's set, the animation path
	// already handles transforms via applyAnimationToStyle.)
	if st.AnimationName != "" {
		return nil
	}
	if st.Transform == "" {
		return nil
	}

	canvas.Save()
	applied := applyTransformOpsSized(canvas, st.Transform, box.Width(), box.Height())
	if applied {
		return canvas.Restore
	}
	canvas.Restore() // no ops applied, undo the save
	return nil
}

// applyTransformOps parses a CSS transform string and applies the operations
// to the canvas. Returns true if at least one operation was applied.
func applyTransformOps(canvas *graphics.Canvas, transform string) bool {
	// No element-size reference: percentages resolve to 0 (CSS 2D transform
	// percentages are relative to the box's own size; callers that have a box
	// should use applyTransformOpsSized).
	return applyTransformOpsSized(canvas, transform, 0, 0)
}

// applyTransformOpsSized is applyTransformOps with the element's border-box
// width/height, used to resolve translate() percentages (CSS Transforms §2.1:
// "percentages refer to the size of the element's border box").
func applyTransformOpsSized(canvas *graphics.Canvas, transform string, refW, refH float64) bool {
	transform = strings.TrimSpace(transform)
	if transform == "" || transform == "none" {
		return false
	}

	applied := false
	// tokenize by whitespace (each token is a function call).
	tokens := tokenizeTransform(transform)
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// Parse "funcName(args)"
		paren := strings.IndexByte(tok, '(')
		if paren < 0 || !strings.HasSuffix(tok, ")") {
			continue
		}
		fn := strings.ToLower(tok[:paren])
		args := tok[paren+1 : len(tok)-1]
		switch fn {
		case "translatex":
			if v := parseTransformLen(args, refW); v != 0 {
				canvas.Translate(v, 0)
				applied = true
			}
		case "translatey":
			if v := parseTransformLen(args, refH); v != 0 {
				canvas.Translate(0, v)
				applied = true
			}
		case "translate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				tx := parseTransformLen(vals[0], refW)
				ty := tx // default: same as tx
				if len(vals) >= 2 {
					ty = parseTransformLen(vals[1], refH)
				}
				if tx != 0 || ty != 0 {
					canvas.Translate(tx, ty)
					applied = true
				}
			}
		case "scale":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseScaleValue(vals[0])
				sy := sx
				if len(vals) >= 2 {
					sy = parseScaleValue(vals[1])
				}
				if sx != 1 || sy != 1 {
					canvas.Scale(sx, sy)
					applied = true
				}
			}
		case "scalex":
			if v := parseScaleValue(args); v != 1 {
				canvas.Scale(v, 1)
				applied = true
			}
		case "scaley":
			if v := parseScaleValue(args); v != 1 {
				canvas.Scale(1, v)
				applied = true
			}
		case "rotate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				deg := parseAngle(vals[0])
				if deg != 0 {
					if len(vals) >= 3 {
						// rotate(deg cx cy): rotation about point (cx,cy),
						// i.e. translate(cx,cy) rotate(deg) translate(-cx,-cy).
						cx := parseTransformLen(vals[1], refW)
						cy := parseTransformLen(vals[2], refH)
						canvas.Translate(cx, cy)
						canvas.Rotate(deg)
						canvas.Translate(-cx, -cy)
					} else {
						canvas.Rotate(deg)
					}
					applied = true
				}
			}
		case "rotatex":
			// rotateX is a 3D transform; in our 2D canvas we
			// approximate with a scaleY (compression along Y axis).
			deg := parseAngle(args)
			if deg != 0 {
				canvas.Scale(1, math.Cos(deg*math.Pi/180.0))
				applied = true
			}
		case "rotatey":
			deg := parseAngle(args)
			if deg != 0 {
				canvas.Scale(math.Cos(deg*math.Pi/180.0), 1)
				applied = true
			}
		case "rotatez":
			deg := parseAngle(args)
			if deg != 0 {
				canvas.Rotate(deg)
				applied = true
			}
		case "skew":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseAngle(vals[0])
				sy := 0.0
				if len(vals) >= 2 {
					sy = parseAngle(vals[1])
				}
				if sx != 0 || sy != 0 {
					canvas.Skew(sx, sy)
					applied = true
				}
			}
		case "skewx":
			if v := parseAngle(args); v != 0 {
				canvas.Skew(v, 0)
				applied = true
			}
		case "skewy":
			if v := parseAngle(args); v != 0 {
				canvas.Skew(0, v)
				applied = true
			}
		case "translatez":
			// translateZ on a 2D canvas is a no-op (no perspective).
			// Parssed for compatibility.
			_ = parseLength(args)
		case "translate3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				tx := parseLength(vals[0])
				ty := parseLength(vals[1])
				if tx != 0 || ty != 0 {
					canvas.Translate(tx, ty)
					applied = true
				}
			}
		case "scalez":
			// scaleZ on a 2D canvas is a no-op.
		case "scale3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				sx := parseScaleValue(vals[0])
				sy := parseScaleValue(vals[1])
				if sx != 1 || sy != 1 {
					canvas.Scale(sx, sy)
					applied = true
				}
			}
		case "matrix":
			vals := splitSpaceComma(args)
			if len(vals) >= 6 {
				a := parseScaleValue(vals[0])
				b := parseScaleValue(vals[1])
				c := parseScaleValue(vals[2])
				d := parseScaleValue(vals[3])
				e := parseScaleValue(vals[4])
				f := parseScaleValue(vals[5])
				// Build a 3x3 matrix: [a c e; b d f; 0 0 1]
				m := skia.Matrix{
					ScaleX: float32(a), SkewX: float32(c), TransX: float32(e),
					SkewY: float32(b), ScaleY: float32(d), TransY: float32(f),
					Persp0: 0, Persp1: 0, Persp2: 1,
				}
				canvas.Concat(m)
				applied = true
			}
		}
	}
	return applied
}

// tokenizeTransform splits a transform string into individual function calls.
func tokenizeTransform(s string) []string {
	var tokens []string
	var cur strings.Builder
	depth := 0
	for _, c := range s {
		switch {
		case c == '(':
			depth++
			cur.WriteRune(c)
		case c == ')':
			depth--
			cur.WriteRune(c)
		case c == ' ' && depth == 0:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// splitSpaceComma splits a string by space and comma (for function arguments).
func splitSpaceComma(s string) []string {
	var parts []string
	cur := strings.Builder{}
	for _, c := range s {
		if c == ' ' || c == ',' {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// parseLength parses a CSS length like "10px", "-5px", "2em".
// Returns the numeric value (pixels; em/percentage not scaled).
func parseLength(s string) float64 {
	return parseTransformLen(s, 0)
}

// parseTransformLen parses a transform length, resolving CSS percentages
// against the element's border-box size (ref). Percentage is the standard
// behavior for translate()/translateX()/translateY() (CSS Transforms §2.1);
// all other units (px, em, rem, ...) resolve to their numeric value — the
// pre-existing behavior, accurate for px, approximate for font-relative units
// (font-size is not available in the paint phase).
func parseTransformLen(s string, ref float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Find numeric prefix.
	i := 0
	if s[i] == '+' || s[i] == '-' {
		i++
	}
	dot := false
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' && !dot) {
		if s[i] == '.' {
			dot = true
		}
		i++
	}
	if i == 0 || (i == 1 && (s[0] == '+' || s[0] == '-')) {
		return 0
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	if strings.HasSuffix(s, "%") {
		return num * ref / 100
	}
	return num
}

// parseScaleValue parses a scale value like "2", "1.5", "-1".
func parseScaleValue(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1
	}
	return v
}

// parseAngle parses a CSS angle like "45deg", "90deg", "1.57rad".
func parseAngle(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "deg") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "deg"), 64)
		if err != nil {
			return 0
		}
		return v
	}
	if strings.HasSuffix(s, "rad") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "rad"), 64)
		if err != nil {
			return 0
		}
		return v * 180.0 / math.Pi
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// ── 命中测试 transform 逆变换（hit-testing 镜像 paint 的 canvas 变换）──
// paint 对带 transform 的元素施加 canvas 正向变换（applyTransformOpsSized），
// 因此「视觉位置」≠「布局位置」（弹窗 translate(-50%,-50%) 居中：布局锚点
// 在 (left,top)，视觉在中心）。HitTest 若不逆变换，点击视觉位置会 miss
// 目标（穿透到遮罩/下层）——浏览器标准：命中测试经过 transform 逆映射。

// mat2 是 2D 仿射矩阵 [a c e; b d f; 0 0 1]：x' = a*x + c*y + e；y' = b*x + d*y + f。
type mat2 struct{ a, b, c, d, e, f float64 }

// mul 按 Skia Concat 语义右乘：M = M·N（与 paint 的 canvas 操作序列一致）。
func (m mat2) mul(n mat2) mat2 {
	return mat2{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// apply 变换点。
func (m mat2) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// inv 求逆矩阵（行列式为 0 时返回 false——退化不参与命中）。
func (m mat2) inv() (mat2, bool) {
	det := m.a*m.d - m.b*m.c
	if det == 0 {
		return m, false
	}
	id := 1.0 / det
	return mat2{
		a: m.d * id,
		b: -m.b * id,
		c: -m.c * id,
		d: m.a * id,
		e: (m.c*m.f - m.d*m.e) * id,
		f: (m.b*m.e - m.a*m.f) * id,
	}, true
}

// hitInverseTransform 把点击点从父坐标系逆变换到元素本地坐标系。
// 返回值：变换后的坐标、是否发生了非平凡变换（无 transform 时原样返回）。
func hitInverseTransform(transform string, refW, refH, x, y float64) (float64, float64, bool) {
	transform = strings.TrimSpace(transform)
	if transform == "" || transform == "none" {
		return x, y, false
	}
	m := mat2{a: 1, d: 1}
	applied := false
	identity := m
	tokens := tokenizeTransform(transform)
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		paren := strings.IndexByte(tok, '(')
		if paren < 0 || !strings.HasSuffix(tok, ")") {
			continue
		}
		fn := strings.ToLower(tok[:paren])
		args := tok[paren+1 : len(tok)-1]
		var n mat2
		skip := false
		switch fn {
		case "translatex":
			if v := parseTransformLen(args, refW); v != 0 {
				n = mat2{a: 1, d: 1, e: v}
			} else {
				skip = true
			}
		case "translatey":
			if v := parseTransformLen(args, refH); v != 0 {
				n = mat2{a: 1, d: 1, f: v}
			} else {
				skip = true
			}
		case "translate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				tx := parseTransformLen(vals[0], refW)
				ty := tx
				if len(vals) >= 2 {
					ty = parseTransformLen(vals[1], refH)
				}
				if tx != 0 || ty != 0 {
					n = mat2{a: 1, d: 1, e: tx, f: ty}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "scale":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseScaleValue(vals[0])
				sy := sx
				if len(vals) >= 2 {
					sy = parseScaleValue(vals[1])
				}
				if sx != 1 || sy != 1 {
					n = mat2{a: sx, d: sy}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "scalex":
			if v := parseScaleValue(args); v != 1 {
				n = mat2{a: v, d: 1}
			} else {
				skip = true
			}
		case "scaley":
			if v := parseScaleValue(args); v != 1 {
				n = mat2{a: 1, d: v}
			} else {
				skip = true
			}
		case "rotate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				deg := parseAngle(vals[0])
				if deg != 0 {
					if len(vals) >= 3 {
						cx := parseTransformLen(vals[1], refW)
						cy := parseTransformLen(vals[2], refH)
						n = mat2{a: 1, d: 1, e: cx, f: cy}.
							mul(mat2{a: math.Cos(deg * math.Pi / 180), b: math.Sin(deg * math.Pi / 180),
								c: -math.Sin(deg * math.Pi / 180), d: math.Cos(deg * math.Pi / 180)}).
							mul(mat2{a: 1, d: 1, e: -cx, f: -cy})
					} else {
						r := deg * math.Pi / 180
						n = mat2{a: math.Cos(r), b: math.Sin(r), c: -math.Sin(r), d: math.Cos(r)}
					}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "rotatex":
			deg := parseAngle(args)
			if deg != 0 {
				n = mat2{a: 1, d: math.Cos(deg * math.Pi / 180)}
			} else {
				skip = true
			}
		case "rotatey":
			deg := parseAngle(args)
			if deg != 0 {
				n = mat2{a: math.Cos(deg * math.Pi / 180), d: 1}
			} else {
				skip = true
			}
		case "rotatez":
			deg := parseAngle(args)
			if deg != 0 {
				r := deg * math.Pi / 180
				n = mat2{a: math.Cos(r), b: math.Sin(r), c: -math.Sin(r), d: math.Cos(r)}
			} else {
				skip = true
			}
		case "skew":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseAngle(vals[0])
				sy := 0.0
				if len(vals) >= 2 {
					sy = parseAngle(vals[1])
				}
				if sx != 0 || sy != 0 {
					n = mat2{a: 1, d: 1, b: math.Tan(sy * math.Pi / 180), c: math.Tan(sx * math.Pi / 180)}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "skewx":
			if v := parseAngle(args); v != 0 {
				n = mat2{a: 1, d: 1, c: math.Tan(v * math.Pi / 180)}
			} else {
				skip = true
			}
		case "skewy":
			if v := parseAngle(args); v != 0 {
				n = mat2{a: 1, d: 1, b: math.Tan(v * math.Pi / 180)}
			} else {
				skip = true
			}
		case "translatez":
			skip = true
		case "translate3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				tx := parseLength(vals[0])
				ty := parseLength(vals[1])
				if tx != 0 || ty != 0 {
					n = mat2{a: 1, d: 1, e: tx, f: ty}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "scalez":
			skip = true
		case "scale3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				sx := parseScaleValue(vals[0])
				sy := parseScaleValue(vals[1])
				if sx != 1 || sy != 1 {
					n = mat2{a: sx, d: sy}
				} else {
					skip = true
				}
			} else {
				skip = true
			}
		case "matrix":
			vals := splitSpaceComma(args)
			if len(vals) >= 6 {
				a := parseScaleValue(vals[0])
				b := parseScaleValue(vals[1])
				c := parseScaleValue(vals[2])
				d := parseScaleValue(vals[3])
				e := parseScaleValue(vals[4])
				f := parseScaleValue(vals[5])
				n = mat2{a: a, b: b, c: c, d: d, e: e, f: f}
				_ = a
			} else {
				skip = true
			}
		default:
			skip = true
		}
		if skip {
			continue
		}
		m = m.mul(n)
		applied = true
	}
	if !applied || m == identity {
		return x, y, false
	}
	if inv, ok := m.inv(); ok {
		nx, ny := inv.apply(x, y)
		return nx, ny, true
	}
	return x, y, false
}
