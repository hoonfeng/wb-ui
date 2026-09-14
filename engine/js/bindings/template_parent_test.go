package bindings

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// 逐步对比 JS 路径与 Go 路径的 parentNode 状态。
func TestTemplateInnerHTMLParentLinks(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	tpl := doc.CreateElement("template")
	doc.AppendChild(tpl)

	// JS 设置 innerHTML
	if _, err := rt.RunJS(`document.querySelector('template').innerHTML = '<svg><circle/><path/></svg>';`); err != nil {
		t.Fatalf("set innerHTML: %v", err)
	}
	svg := tpl.FirstChild()
	if svg == nil {
		t.Fatal("template has no children after JS innerHTML set")
	}
	p := svg.ParentNode()
	t.Logf("JS path: svg=%s parentNode=%v template==parent:%v", svg.NodeName(), pname(p), p == tpl)
	if p != tpl {
		t.Fatalf("JS innerHTML: svg.parentNode != template (got %v)", pname(p))
	}
	circle := svg.FirstChild()
	if circle == nil {
		t.Fatal("svg has no children")
	}
	t.Logf("JS path: circle parent=%v", pname(circle.ParentNode()))
	if circle.ParentNode() != svg {
		t.Fatalf("JS innerHTML: circle.parentNode != svg (got %v)", pname(circle.ParentNode()))
	}

	// Go 直接设置
	tpl2 := doc.CreateElement("template")
	doc.AppendChild(tpl2)
	_ = tpl2.SetInnerHTML("<svg><circle/></svg>")
	svg2 := tpl2.FirstChild()
	if svg2 == nil || svg2.ParentNode() != tpl2 {
		t.Fatalf("Go SetInnerHTML: parent link broken (svg2=%v)", svg2)
	}
	t.Logf("Go path OK")
	_ = jsc.Undefined
}

func pname(n dom.Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.NodeName()
}
