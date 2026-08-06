package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/style"
)

// TestResumeBannerCJKOneLine: the resume-banner reproduction — flex row with a
// span flex:1 holding a CJK sentence. Must stay on ONE line (~16px), NOT wrap
// per character (437px balloon that shrank .chat-messages to 110px).
func TestResumeBannerCJKOneLine(t *testing.T) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, fstyle, text string) float64 {
		w := 0.0
		for _, r := range text {
			if r > 0x2E80 {
				w += size
			} else {
				w += size * 0.55
			}
		}
		return w
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, fstyle string) (float64, float64, float64) {
		return size * 0.85, size * 0.2, 0
	}
	htmlStr := `<!DOCTYPE html><html><head><style>
		html,body{margin:0;padding:0}
		.chat-input-area{display:flex;flex-direction:column;flex-shrink:0;padding:0 8px 8px 8px}
		.resume-banner{display:flex;align-items:center;gap:8px;margin:0 0 6px 0;padding:6px 10px;background:rgba(232,172,82,0.12);border:1px solid rgba(232,172,82,0.35);border-radius:6px;font-size:12px;color:#e8ac52}
		.resume-icon{flex-shrink:0;font-size:13px}
		.resume-text{flex:1;line-height:1.4}
		.resume-btn{flex-shrink:0;display:inline-flex;align-items:center;gap:4px;padding:3px 10px;background:rgba(232,172,82,0.25);font-size:11px;white-space:nowrap}
	</style></head><body>
	<div class="chat-input-area">
		<div class="resume-banner">
			<span class="resume-icon">⚠️</span>
			<span class="resume-text">上次任务未完成，本对话上下文与进度已保留，可直接继续</span>
			<button class="resume-btn">继续任务</button>
		</div>
	</div>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(1200, 400)
	rv.Layout(layout.NewLayoutState(1200, 400))

	banner := findRenderBoxByClass(rv, "resume-banner")
	if banner == nil {
		t.Fatalf("resume-banner not found")
	}
	tb := banner.BorderBoxRect()
	t.Logf("banner: (%.0f,%.0f %.0fx%.0f)", tb.X, tb.Y, tb.Width, tb.Height)
	if tb.Height > 60 {
		t.Errorf("resume-banner height %.0f px (want ~30) — CJK text ballooned to multi-line", tb.Height)
	}
}

func findRenderBoxByClass(rv *RenderView, class string) *RenderBox {
	var out *RenderBox
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if out != nil {
			return
		}
		if el, ok := o.Node().(*dom.Element); ok {
			if el.GetAttribute("class") == class {
				out = asRenderBox(o)
				return
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
	return out
}
