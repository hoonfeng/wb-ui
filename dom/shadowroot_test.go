package dom

import "testing"

func TestAttachShadow(t *testing.T) {
	doc := NewDocument()
	host := doc.CreateElement("div")

	sr, err := host.AttachShadow("open")
	if err != nil {
		t.Fatalf("AttachShadow: %v", err)
	}
	if sr == nil {
		t.Fatal("AttachShadow returned nil")
	}
	if host.ShadowRoot() != sr {
		t.Fatal("ShadowRoot() != attached shadow root")
	}
	if sr.Host() != host {
		t.Fatal("shadow root host != element")
	}
	if sr.Mode() != "open" {
		t.Fatalf("mode = %q, want open", sr.Mode())
	}
	if sr.NodeName() != "#shadow-root" {
		t.Fatalf("nodeName = %q, want #shadow-root", sr.NodeName())
	}
	if sr.NodeType() != NodeDocumentFragment {
		t.Fatalf("nodeType = %d, want %d (DocumentFragment)", sr.NodeType(), NodeDocumentFragment)
	}
	// ShadowRoot 不挂到 host 的 child 链上，parentNode 应为 nil。
	if sr.ParentNode() != nil {
		t.Fatalf("shadow root parentNode = %v, want nil", sr.ParentNode())
	}
}

func TestAttachShadowClosed(t *testing.T) {
	doc := NewDocument()
	host := doc.CreateElement("div")

	if _, err := host.AttachShadow("closed"); err != nil {
		t.Fatalf("AttachShadow closed: %v", err)
	}
	// closed shadow root 不可从 ShadowRoot() 访问。
	if host.ShadowRoot() != nil {
		t.Fatal("closed shadow root should be inaccessible via ShadowRoot()")
	}
}

func TestAttachShadowDuplicate(t *testing.T) {
	doc := NewDocument()
	host := doc.CreateElement("div")

	if _, err := host.AttachShadow("open"); err != nil {
		t.Fatalf("first AttachShadow: %v", err)
	}
	if _, err := host.AttachShadow("open"); err == nil {
		t.Fatal("second AttachShadow should fail (ErrNotSupported)")
	}
}

func TestFirstComposedChild(t *testing.T) {
	doc := NewDocument()
	host := doc.CreateElement("div")
	light := doc.CreateElement("span")
	if err := host.AppendChild(light); err != nil {
		t.Fatalf("append light child: %v", err)
	}

	// 无 shadow root：返回 light-DOM 的第一个子节点。
	if FirstComposedChild(host) != light {
		t.Fatal("FirstComposedChild without shadow root != light child")
	}

	// 有 shadow root：返回 shadow root 的第一个子节点（light 被隐藏）。
	sr, _ := host.AttachShadow("open")
	shadowEl := doc.CreateElement("b")
	if err := sr.AppendChild(shadowEl); err != nil {
		t.Fatalf("append shadow child: %v", err)
	}
	if FirstComposedChild(host) != shadowEl {
		t.Fatal("FirstComposedChild with shadow root != shadow child")
	}
}

func TestShadowRootChildChain(t *testing.T) {
	doc := NewDocument()
	host := doc.CreateElement("div")
	sr, _ := host.AttachShadow("open")

	first := doc.CreateElement("i")
	second := doc.CreateElement("u")
	_ = sr.AppendChild(first)
	_ = sr.AppendChild(second)

	// shadow root 的子节点链完整可遍历。
	c := FirstComposedChild(host)
	if c != first {
		t.Fatalf("first composed child = %v, want %v", c, first)
	}
	if c.NextSibling() != second {
		t.Fatalf("second composed child = %v, want %v", c.NextSibling(), second)
	}
}
