// Tests for the ::backdrop box of a modal <dialog> (HTML rendering / Fullscreen
// spec §5): showModal() must produce a viewport-sized translucent backdrop that
// paints below the dialog and above the page.

package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// dialogBackdropFixture 建一个 300x300 的页面：左上角 40x40 红块 + 一个 <dialog>
// （UA 规则把它定在视口中心）。modal 决定 dialog 是否处于模态状态。
func dialogBackdropFixture(modal bool) (*dom.Document, *dom.Element) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	body := dom.NewElement(doc, "body")
	htmlEl.AppendChild(body)

	red := dom.NewElement(doc, "div")
	red.SetAttribute("style",
		"position:absolute;left:0;top:0;width:40px;height:40px;background:#ff0000")
	body.AppendChild(red)

	dlg := dom.NewElement(doc, "dialog")
	body.AppendChild(dlg)
	dlg.SetAttribute("open", "")
	dlg.SetModalState(modal)
	return doc, dlg
}

// findRenderObject 在渲染树中查找生成给定元素的渲染对象。
func findRenderObject(root RenderObject, el *dom.Element) RenderObject {
	var found RenderObject
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if found != nil {
			return
		}
		if n, ok := o.Node().(*dom.Element); ok && n == el {
			found = o
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(root)
	return found
}

func TestDialogBackdropGeneratesRenderObject(t *testing.T) {
	// 模态：dialog 之前必须有一个无 DOM 节点的固定定位半透明对象。
	doc, dlg := dialogBackdropFixture(true)
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(300, 300)
	rv.Layout(nil)

	dlgObj := findRenderObject(rv, dlg)
	if dlgObj == nil {
		t.Fatal("渲染树中没有 <dialog> 的渲染对象")
	}
	bd := dlgObj.PreviousSibling()
	if bd == nil {
		t.Fatal("模态 dialog 之前应有 ::backdrop 渲染对象")
	}
	if bd.Node() != nil {
		t.Fatalf("::backdrop 渲染对象不应有 DOM 节点，实际 %T", bd.Node())
	}
	cs := bd.Style()
	if cs == nil {
		t.Fatal("::backdrop 没有 computed style")
	}
	if cs.Position != style.PositionFixed {
		t.Fatalf("::backdrop position = %v，want fixed（铺满视口）", cs.Position)
	}
	if cs.BackgroundColor.A == 0 || cs.BackgroundColor.A == 255 {
		t.Fatalf("::backdrop 背景应为半透明，实际 %+v", cs.BackgroundColor)
	}

	// 非模态：不应生成 ::backdrop。
	doc2, dlg2 := dialogBackdropFixture(false)
	rv2 := NewRenderTreeBuilder(resolver).Build(doc2)
	rv2.SetViewportSize(300, 300)
	rv2.Layout(nil)
	dlgObj2 := findRenderObject(rv2, dlg2)
	if dlgObj2 == nil {
		t.Fatal("渲染树中没有 <dialog> 的渲染对象（非模态）")
	}
	if prev := dlgObj2.PreviousSibling(); prev != nil && prev.Node() == nil {
		t.Fatalf("非模态 dialog 不应生成 ::backdrop（前一个兄弟是 %T）", prev)
	}
}

func TestDialogBackdropPaintsOverlay(t *testing.T) {
	paint := func(modal bool) graphics.Color {
		doc, _ := dialogBackdropFixture(modal)
		resolver := style.NewResolver()
		resolver.AddStyleSheet(html5.NewUAStyleSheet())
		rv := NewRenderTreeBuilder(resolver).Build(doc)
		rv.SetViewportSize(300, 300)
		rv.Layout(nil)
		canvas := graphics.NewCanvas(300, 300)
		defer canvas.Release()
		Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 300, Height: 300})
		return canvas.PixelAt(20, 20) // 红块内、dialog 之外
	}

	// 非模态：红块是纯红（无遮罩）。
	if got := paint(false); got.R != 0xFF || got.G != 0 || got.B != 0 {
		t.Fatalf("非模态时 (20,20) = %+v，want 纯红 #ff0000", got)
	}

	// 模态：红块被 10% 黑遮罩压暗（仍以红为主），且 dialog 之外的区域也被覆盖。
	got := paint(true)
	if got.R < 210 || got.R > 245 {
		t.Fatalf("模态时 (20,20) = %+v，want 被遮罩压暗的红（R≈229）", got)
	}
	if got.G > 12 || got.B > 12 {
		t.Fatalf("模态时 (20,20) = %+v，want 仍以红为主（G/B≈0）", got)
	}
}
