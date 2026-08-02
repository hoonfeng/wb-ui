// CSS transition support: when an element's computed style changes between
// rebuilds (e.g. :hover / :checked pseudo-classes), the changed properties
// animate from the previously-rendered value to the new value over the
// transition duration, mirroring WebKit's CSSAnimation / CSSTransition.
//
// Architecture note: wb-ui rebuilds the whole render tree on style changes,
// so RenderObjects (and their ComputedStyles) are discarded each time. The
// transition state therefore lives in a per-DOM-element registry (elements
// survive rebuilds). Each frame we compare the element's current computed
// value against the registry:
//
//   - equal & animating   → keep interpolating (from → to by progress)
//   - equal & settled     → idle (registry == target)
//   - different           → start a new transition from the last rendered
//     value to the new target
//
// Interpolated values are written back into the element's ComputedStyle
// (BackgroundColor / Properties["left"] / Properties["top"] / Opacity /
// transform), and the host marks the frame needing layout so the geometry
// (e.g. the switch thumb's left) updates each frame during the transition.

package rendering

import (
	"fmt"
	"strconv"
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

type transitionKind int

const (
	transColor transitionKind = iota
	transLength
	transOpacity
)

// transitionAnim records one in-flight property transition for an element.
type transitionAnim struct {
	prop     string
	kind     transitionKind
	fromC    graphics.Color
	toC      graphics.Color
	fromN    float64
	toN      float64
	start    float64
	duration float64
	// cssProp is the ComputedStyle field/property to write back.
	cssProp string
	// stylePtr identifies the ComputedStyle instance this anim was created
	// against. Rebuilds produce a new ComputedStyle pointer; when it changes
	// the target must be re-read from the fresh (un-interpolated) style.
	stylePtr *style.ComputedStyle
}

// transitionRegistry tracks the last rendered value per (element, property).
// Keys are DOM element pointers, which survive render-tree rebuilds.
// Pseudo-element boxes (no DOM node) share the owner element but are kept in
// a separate namespace so the thumb's background/left never collides with the
// host track's background transition.
type transitionKey struct {
	el      *dom.Element
	pseudo  bool // true = a ::before/::after box of el
}

var transitionRegistry = map[transitionKey]map[string]*transitionAnim{}

// applyTransitions walks the render tree and drives CSS transitions for every
// element with a non-zero transition-duration. time is the global animation
// clock in seconds (AnimationTime). It returns true if any transition is
// still in flight (the host should re-layout this frame).
func applyTransitions(rv *RenderView, time float64) bool {
	if rv == nil {
		return false
	}
	anyActive := false
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if o == nil {
			return
		}
		if st := o.Style(); st != nil && st.TransitionDuration > 0 {
			// 宿主元素：普通 box 用自身 Node()；伪元素/匿名 box（Node()==nil）
			// 向上找最近的 DOM 宿主（如 .track 的 ::after 滑块）。
			if el, isPseudo := transitionOwnerElement(o); el != nil {
				if applyElementTransitions(el, isPseudo, st, time) {
					anyActive = true
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
	return anyActive
}

// transitionOwnerElement returns the DOM element driving transitions for o:
// the element itself, or (for pseudo-element / anonymous boxes with no Node)
// the nearest ancestor box that has a DOM element. isPseudo=true for the
// pseudo-element/anonymous boxes (kept in a separate registry namespace).
func transitionOwnerElement(o RenderObject) (*dom.Element, bool) {
	if n := o.Node(); n != nil {
		if e, ok := n.(*dom.Element); ok {
			return e, false
		}
	}
	// Walk up the render-tree ancestors.
	for c := o.Parent(); c != nil; c = c.Parent() {
		if n := c.Node(); n != nil {
			if e, ok := n.(*dom.Element); ok {
				return e, true
			}
		}
	}
	return nil, false
}

// applyElementTransitions drives transitions for one element. Returns true if
// a transition is in flight (needs re-layout).
func applyElementTransitions(el *dom.Element, isPseudo bool, st *style.ComputedStyle, time float64) bool {
	dur := st.TransitionDuration
	if dur <= 0 {
		return false
	}
	key := transitionKey{el: el, pseudo: isPseudo}
	reg := transitionRegistry[key]
	if reg == nil {
		reg = map[string]*transitionAnim{}
		transitionRegistry[key] = reg
	}
	inFlight := false
	for _, p := range transitionProps {
		anim := reg[p]
		// stylePtr 标识 style 是否被 rebuild 重建（rebuild → 新对象）。
		// 重建时 st 是未插值的原始 CSS 值 → 重新检测目标；
		// 未重建时 st 可能已被插值覆盖 → 目标用 anim 快照（anim.to）。
		if anim == nil || anim.stylePtr != st {
			target := readTransitionValue(st, p)
			if !target.ok {
				continue
			}
			if anim == nil {
				// First sighting: record the target as the settled value.
				reg[p] = settledAnim(p, target, time, st)
			} else if !sameTransitionValue(anim, target, p) {
				// Style was rebuilt with a different value: start/restart a
				// transition from the last rendered value to the new target.
				cur := interpolate(anim, time)
				reg[p] = newTransitionAnim(p, anim, target, time, dur, st)
				writeTransitionValue(st, p, cur.color, cur.num)
				inFlight = true
			} else {
				// Rebuilt but the target is unchanged: stay settled.
				reg[p] = settledAnim(p, target, time, st)
			}
			continue
		}
		// Style object unchanged since the last frame: advance the in-flight
		// interpolation (target = anim.to, never re-read from the possibly
		// interpolated st).
		if anim.settled() {
			continue
		}
		prog := (time - anim.start) / anim.duration
		if prog >= 1 {
			writeTransitionValue(st, p, anim.toC, anim.toN)
			reg[p] = settledAnim(p, transitionValue{ok: true, color: anim.toC, num: anim.toN}, time, st)
		} else {
			cur := interpolate(anim, time)
			writeTransitionValue(st, p, cur.color, cur.num)
			inFlight = true
		}
	}
	return inFlight
}

// transitionProps enumerates the interpolatable properties we support.
var transitionProps = []string{
	"background-color",
	"left",
	"top",
	"opacity",
}

type transitionValue struct {
	ok   bool
	color graphics.Color
	num  float64
}

// readTransitionValue extracts the current value of prop from a ComputedStyle.
func readTransitionValue(st *style.ComputedStyle, prop string) transitionValue {
	switch prop {
	case "background-color":
		return transitionValue{ok: true, color: toGraphicsColor(st.BackgroundColor)}
	case "left", "top":
		if s := st.Properties[prop]; s != "" {
			if v, ok := parsePxValue(s); ok {
				return transitionValue{ok: true, num: v}
			}
		}
	case "opacity":
		return transitionValue{ok: true, num: st.Opacity}
	}
	return transitionValue{ok: false}
}

// writeTransitionValue writes an interpolated value back into the ComputedStyle.
func writeTransitionValue(st *style.ComputedStyle, prop string, c graphics.Color, n float64) {
	switch prop {
	case "background-color":
		st.BackgroundColor = toStyleColor(c)
	case "left", "top":
		st.Properties[prop] = fmt.Sprintf("%.3fpx", n)
	case "opacity":
		st.Opacity = n
	}
}

func settledAnim(prop string, v transitionValue, time float64, st *style.ComputedStyle) *transitionAnim {
	a := &transitionAnim{prop: prop, cssProp: prop, stylePtr: st}
	a.toC, a.toN = v.color, v.num
	a.fromC, a.fromN = v.color, v.num
	a.start = time - 1 // settle immediately
	a.duration = 0
	return a
}

func newTransitionAnim(prop string, prev *transitionAnim, target transitionValue, time, dur float64, st *style.ComputedStyle) *transitionAnim {
	a := &transitionAnim{prop: prop, cssProp: prop, start: time, duration: dur, stylePtr: st}
	// From the previously rendered value (the interpolated value if the old
	// transition was mid-flight, otherwise the settled target).
	if prev != nil {
		cur := interpolate(prev, time)
		a.fromC, a.fromN = cur.color, cur.num
	}
	a.toC, a.toN = target.color, target.num
	return a
}

// settled reports whether the animation has finished (duration 0 or past end).
func (a *transitionAnim) settled() bool { return a.duration <= 0 }

// interpolate computes the value at time (clamped to [0,1] progress, ease-out
// timing function).
func interpolate(a *transitionAnim, time float64) transitionValue {
	if a.duration <= 0 {
		return transitionValue{ok: true, color: a.toC, num: a.toN}
	}
	prog := (time - a.start) / a.duration
	if prog < 0 {
		prog = 0
	}
	if prog > 1 {
		prog = 1
	}
	// ease-out cubic (CSS default timing function).
	e := 1 - (1-prog)*(1-prog)*(1-prog)
	return transitionValue{
		ok:    true,
		color: lerpColor(a.fromC, a.toC, e),
		num:   a.fromN + (a.toN-a.fromN)*e,
	}
}

func sameTransitionValue(a *transitionAnim, v transitionValue, prop string) bool {
	switch prop {
	case "background-color":
		return a.toC == v.color
	case "left", "top", "opacity":
		return a.toN == v.num
	}
	return true
}

func toStyleColor(c graphics.Color) style.Color {
	return style.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// parsePxValue parses "18px" → 18.
func parsePxValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSuffix(s, "PX")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
