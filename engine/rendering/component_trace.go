package rendering

// componentPaintTrace records which components were actually painted and
// where — a per-frame snapshot of real paint behavior (class, rect, bg color).
// Unlike pixel scanning this is exact: it answers "did the editor paint, and
// at what rect/color" from the painter's own state.
//
// Hot-path cost: PaintBackground/PaintText call RecordComponentPaint which only
// updates an in-memory map (no I/O). The host drains it once per snapshot
// interval (WB_SNAP) via SnapshotComponentPaints.

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

var (
	compPaintMu     sync.Mutex
	compPaintLatest = map[string]compPaintEntry{} // key = css class (or tag)
)

// ★ CursorPainted: per-paint flag — PaintBorder sets it when the caret's
// border is actually painted. renderpipeline.Paint resets it at entry and,
// if it is still false after the layer-tree walk (the caret box exists in the
// render tree but the layer walk never reaches it), draws the caret as a
// fallback so it never silently disappears (cursor invisible regression).
var CursorPainted bool

type compPaintEntry struct {
	cls  string
	x, y float64
	w, h float64
	bg   graphics.Color
	fg   graphics.Color
	text bool
	n    int // how many boxes painted this class this frame
}

// RecordComponentPaint is called from the painters (hot path, in-memory only).
func RecordComponentPaint(el *dom.Element, x, y, w, h float64, bg, fg graphics.Color, isText bool) {
	if el == nil {
		return
	}
	key := el.LocalName()
	if cls := el.GetAttribute("class"); cls != "" {
		key = componentTraceKey(cls)
	}
	compPaintMu.Lock()
	e, ok := compPaintLatest[key]
	if !ok {
		compPaintLatest[key] = compPaintEntry{cls: key, x: x, y: y, w: w, h: h, bg: bg, fg: fg, text: isText, n: 1}
	} else {
		// 合并：保留最左上的一个 + 扩展包围盒，n 累计
		if x < e.x {
			e.x = x
		}
		if y < e.y {
			e.y = y
		}
		if x+w > e.x+e.w {
			e.w = x + w - e.x
		}
		if y+h > e.y+e.h {
			e.h = y + h - e.y
		}
		e.n++
		compPaintLatest[key] = e
	}
	compPaintMu.Unlock()
}

// componentTraceKey 挑选用于合并记录的类名：优先命中「关键组件」特征类
// （光标/选中/活动行/折叠箭头等——这些是渲染诊断要单独定位的），否则
// 退回第一个类名（与元素自身样式 identity 一致）。光标（cm-cursor）是
// border 绘制（PaintBorder 调 RecordComponentPaint），背景透明——若不
// 在此优先命中，会落到第一个类名（也是 cm-cursor）——此处特判保证
// 语义清晰，且 cm-activeLine 这类「多类元素」不被 cm-line 合并吞掉。
func componentTraceKey(cls string) string {
	for _, c := range strings.Fields(cls) {
		switch c {
		case "cm-cursor", "cm-dropCursor", "cm-activeLine", "cm-selectionBackground",
			"cm-activeLineGutter", "cm-foldGutter", "cm-cursorLayer", "cm-selectionLayer",
			"cm-line", "cm-gutters", "cm-content", "cm-scroller", "cm-editor":
			return c
		}
	}
	return strings.Fields(cls)[0]
}

// ResetComponentPaints clears the accumulated per-frame entries.
func ResetComponentPaints() {
	compPaintMu.Lock()
	compPaintLatest = map[string]compPaintEntry{}
	compPaintMu.Unlock()
}

// SnapshotComponentPaints returns the accumulated paint entries as a sorted
// "cls=rect bg=#RRGGBBAA text=bool n=N" string (stable ordering for diffing).
func SnapshotComponentPaints() string {
	compPaintMu.Lock()
	defer compPaintMu.Unlock()
	keys := make([]string, 0, len(compPaintLatest))
	for k := range compPaintLatest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		e := compPaintLatest[k]
		fmt.Fprintf(&sb, "%s=(%.0f,%.0f %.0fx%.0f) bg=#%02x%02x%02x%02x fg=#%02x%02x%02x%02x text=%v n=%d | ",
			e.cls, e.x, e.y, e.w, e.h,
			e.bg.R, e.bg.G, e.bg.B, e.bg.A,
			e.fg.R, e.fg.G, e.fg.B, e.fg.A,
			e.text, e.n)
	}
	return strings.TrimSuffix(sb.String(), " | ")
}
