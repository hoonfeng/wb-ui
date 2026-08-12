package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/style"
)

// findRenderNode 递归遍历 render tree，找到 Node() == target 的 render object。
func findRenderNode(ro RenderObject, target dom.Node) RenderObject {
	if ro == nil {
		return nil
	}
	if ro.Node() == target {
		return ro
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if found := findRenderNode(c, target); found != nil {
			return found
		}
	}
	return nil
}

// isRenderDescendant 判断 desc 是否是 anc 的 render 后代。
func isRenderDescendant(desc, anc RenderObject) bool {
	for ro := desc; ro != nil; ro = ro.Parent() {
		if ro == anc {
			return true
		}
	}
	return false
}

func TestShadowRootRendersInsteadOfLightDOM(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	_ = doc.AppendChild(html)
	body := doc.CreateElement("body")
	_ = html.AppendChild(body)
	host := doc.CreateElement("div")
	_ = body.AppendChild(host)

	// light-DOM 子节点（有 shadow root 后应被隐藏）。
	light := doc.CreateElement("span")
	light.SetTextContent("light")
	_ = host.AppendChild(light)

	// shadow tree 子节点（应替代 light 渲染）。
	sr, err := host.AttachShadow("open")
	if err != nil {
		t.Fatalf("AttachShadow: %v", err)
	}
	shadow := doc.CreateElement("div")
	shadow.SetTextContent("shadow")
	_ = sr.AppendChild(shadow)

	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("Build returned nil")
	}

	hostRO := findRenderNode(rv, host)
	if hostRO == nil {
		t.Fatal("host render object not found")
	}
	shadowRO := findRenderNode(rv, shadow)
	if shadowRO == nil {
		t.Fatal("shadow element render object not found")
	}
	if lightRO := findRenderNode(rv, light); lightRO != nil {
		t.Fatal("light-DOM element should NOT be rendered when a shadow root exists")
	}

	// shadow 元素必须是 host 的 render 后代。
	if !isRenderDescendant(shadowRO, hostRO) {
		t.Fatal("shadow element is not a render descendant of host")
	}
}

func TestNoShadowRootRendersLightDOM(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	_ = doc.AppendChild(html)
	body := doc.CreateElement("body")
	_ = html.AppendChild(body)
	host := doc.CreateElement("div")
	_ = body.AppendChild(host)

	light := doc.CreateElement("span")
	light.SetTextContent("light")
	_ = host.AppendChild(light)

	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("Build returned nil")
	}

	hostRO := findRenderNode(rv, host)
	if hostRO == nil {
		t.Fatal("host render object not found")
	}
	lightRO := findRenderNode(rv, light)
	if lightRO == nil {
		t.Fatal("light-DOM element should be rendered when no shadow root exists")
	}
	if !isRenderDescendant(lightRO, hostRO) {
		t.Fatal("light element is not a render descendant of host")
	}
}

func TestSlotProjection(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	_ = doc.AppendChild(html)
	body := doc.CreateElement("body")
	_ = html.AppendChild(body)
	host := doc.CreateElement("div")
	_ = body.AppendChild(host)

	// light-DOM 子节点（应投影到 <slot> 位置）。
	light := doc.CreateElement("span")
	light.SetTextContent("light")
	_ = host.AppendChild(light)

	// shadow tree：wrapper 内嵌 <slot>。
	sr, err := host.AttachShadow("open")
	if err != nil {
		t.Fatalf("AttachShadow: %v", err)
	}
	wrapper := doc.CreateElement("div")
	_ = sr.AppendChild(wrapper)
	slot := doc.CreateElement("slot")
	_ = wrapper.AppendChild(slot)

	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("Build returned nil")
	}

	// <slot> 自身不生成 render object。
	if slotRO := findRenderNode(rv, slot); slotRO != nil {
		t.Fatal("slot element should not generate a render object")
	}
	// light-DOM 节点被投影到 slot 位置，成为 host 的 render 后代。
	hostRO := findRenderNode(rv, host)
	if hostRO == nil {
		t.Fatal("host render object not found")
	}
	lightRO := findRenderNode(rv, light)
	if lightRO == nil {
		t.Fatal("light-DOM node should be projected into slot position")
	}
	if !isRenderDescendant(lightRO, hostRO) {
		t.Fatal("projected light node is not a render descendant of host")
	}
}
