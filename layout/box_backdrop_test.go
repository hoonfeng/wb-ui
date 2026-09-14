// Tests for the ::backdrop box inside flex/grid containers: a modal <dialog>
// that is a flex item must get the same backdrop box as one in a block container
// (the render tree builder has the mirrored insertion point — both trees have to
// stay node-for-node identical so linkLayoutBoxes can pair them).

package layout

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/style"
)

// findElementBox returns the first box generated for el (nil when absent).
func findElementBox(root Box, el *dom.Element) *ElementBox {
	eb, ok := root.(*ElementBox)
	if !ok {
		return nil
	}
	if eb.element == el {
		return eb
	}
	for _, c := range eb.children {
		if found := findElementBox(c, el); found != nil {
			return found
		}
	}
	return nil
}

// backdropFixture 建一个 display:<containerDisplay> 的 body，内含一个模态
// <dialog>（open + 模态状态），返回 (html 根元素, body, dialog, resolver)。
func backdropFixture(containerDisplay string) (*dom.Element, *dom.Element, *dom.Element, *style.Resolver) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	body := dom.NewElement(doc, "body")
	htmlEl.AppendChild(body)
	body.SetAttribute("style", "display:"+containerDisplay)

	dlg := dom.NewElement(doc, "dialog")
	body.AppendChild(dlg)
	dlg.SetAttribute("open", "")
	dlg.SetModalState(true)

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	return htmlEl, body, dlg, resolver
}

// assertBackdropBeforeDialog 断言 body 的子盒里，dlgBox 的前一个兄弟是
// ::backdrop 伪元素盒（fixed 定位、无 DOM 元素）。
func assertBackdropBeforeDialog(t *testing.T, bodyBox, dlgBox *ElementBox, label string) {
	t.Helper()
	var prev *ElementBox
	for _, c := range bodyBox.children {
		eb, ok := c.(*ElementBox)
		if !ok {
			continue
		}
		if eb == dlgBox {
			break
		}
		prev = eb
	}
	if prev == nil {
		t.Fatalf("%s：dialog 之前没有兄弟盒（::backdrop 未生成）", label)
	}
	if prev.NodeType() != NodePseudoElement {
		t.Fatalf("%s：dialog 的前一个兄弟不是伪元素盒（nodeType=%v）", label, prev.NodeType())
	}
	if prev.element != nil {
		t.Fatalf("%s：::backdrop 盒不应绑定 DOM 元素，实际 <%s>", label, prev.element.LocalName())
	}
	if cs := prev.Style(); cs == nil || cs.Position != style.PositionFixed {
		t.Fatalf("%s：::backdrop 应为 position:fixed（铺满视口）", label)
	}
}

func TestBackdropBoxInFlexAndGridContainers(t *testing.T) {
	for _, display := range []string{"flex", "inline-flex", "grid", "inline-grid"} {
		htmlEl, body, dlg, resolver := backdropFixture(display)
		root := BuildLayoutTree(htmlEl, resolver)

		bodyBox := findElementBox(root, body)
		if bodyBox == nil {
			t.Fatalf("%s：布局树里没有 body 盒", display)
		}
		dlgBox := findElementBox(root, dlg)
		if dlgBox == nil {
			t.Fatalf("%s：布局树里没有 <dialog> 盒", display)
		}
		assertBackdropBeforeDialog(t, bodyBox, dlgBox, display)
	}
}

// TestBackdropBoxInBlockContainer 块级父容器是原有路径，一并锁定（防止将来
// 给 flex 分支加插入点时改坏块级分支）。
func TestBackdropBoxInBlockContainer(t *testing.T) {
	htmlEl, body, dlg, resolver := backdropFixture("block")
	root := BuildLayoutTree(htmlEl, resolver)
	bodyBox := findElementBox(root, body)
	dlgBox := findElementBox(root, dlg)
	if bodyBox == nil || dlgBox == nil {
		t.Fatal("布局树里缺少 body/dialog 盒")
	}
	assertBackdropBeforeDialog(t, bodyBox, dlgBox, "block")
}

// TestNoBackdropForNonModalDialog flex 容器里的非模态 dialog 不应生成遮罩盒。
func TestNoBackdropForNonModalDialog(t *testing.T) {
	htmlEl, body, dlg, resolver := backdropFixture("flex")
	dlg.SetModalState(false)
	root := BuildLayoutTree(htmlEl, resolver)
	bodyBox := findElementBox(root, body)
	dlgBox := findElementBox(root, dlg)
	if bodyBox == nil || dlgBox == nil {
		t.Fatal("布局树里缺少 body/dialog 盒")
	}
	for _, c := range bodyBox.children {
		if eb, ok := c.(*ElementBox); ok && eb == dlgBox {
			break
		}
		if eb, ok := c.(*ElementBox); ok && eb.NodeType() == NodePseudoElement {
			t.Fatal("非模态 dialog 不应生成 ::backdrop 盒")
		}
	}
}
