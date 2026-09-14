// Tests for the UA default stylesheet's fullscreen rule: the fullscreen element
// is promoted to the viewport (position:fixed + inset:0 + 100% size), while
// non-fullscreen elements and the root element are unaffected.

package html5

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

func TestUAStyleSheetFullscreenPromotesElement(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)
	box := dom.NewElement(doc, "div")
	body.AppendChild(box)
	other := dom.NewElement(doc, "div")
	body.AppendChild(other)

	r := style.NewResolver()
	r.AddStyleSheet(NewUAStyleSheet())

	// 未全屏：UA 规则不命中。
	if cs := r.ResolveElement(box); cs.Position == style.PositionFixed {
		t.Fatalf("未全屏元素不应被钉成 fixed，Position=%v", cs.Position)
	}

	// 进入全屏。真实链路由宿主在状态变化时清 resolver 缓存
	// （webkit 的 OnClassChanged 桥 → Resolver.ClearCache），这里显式模拟。
	doc.SetFullscreenElement(box)
	r.ClearCache()
	cs := r.ResolveElement(box)
	if cs.Position != style.PositionFixed {
		t.Fatalf("全屏元素 Position = %v，want PositionFixed", cs.Position)
	}
	if got := cs.GetProperty("top"); got != "0px" && got != "0" {
		t.Fatalf("全屏元素 top = %q，want 0", got)
	}
	if got := cs.GetProperty("max-width"); got != "none" {
		t.Fatalf("全屏元素 max-width = %q，want none", got)
	}
	if got := cs.GetProperty("width"); got != "100%" {
		t.Fatalf("全屏元素 width = %q，want 100%%", got)
	}

	// 同级元素不受影响。
	if cs := r.ResolveElement(other); cs.Position == style.PositionFixed {
		t.Fatalf("非全屏同级元素不应被钉成 fixed，Position=%v", cs.Position)
	}

	// 根元素全屏：:not(:root) 排除 UA 规则（根元素本就铺满视口，钉成 fixed
	// 会破坏文档滚动）。
	doc.SetFullscreenElement(html)
	r.ClearCache()
	if cs := r.ResolveElement(html); cs.Position == style.PositionFixed {
		t.Fatal("根元素全屏时不应被钉成 fixed（:not(:root) 应排除 UA 规则）")
	}
}
