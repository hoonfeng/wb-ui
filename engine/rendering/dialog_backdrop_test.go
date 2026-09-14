// Tests for the ::backdrop box of a modal <dialog> (HTML rendering / Fullscreen
// spec §5): showModal() must produce a viewport-sized translucent backdrop that
// paints below the dialog and above the page.

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
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

// TestDialogBackdropInFlexContainer flex 容器里的模态 <dialog> 也要生成遮罩
// 对象：渲染树与布局树的插入点必须成对（此前只有块级父容器生成，两棵树在
// flex 场景下都缺 -> 遮罩不出现）。
func TestDialogBackdropInFlexContainer(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	body := dom.NewElement(doc, "body")
	htmlEl.AppendChild(body)
	body.SetAttribute("style", "display:flex")

	// absolute 让它脱离 flex 流，落在 (0,0) 40x40（与块级 fixture 同一像素断言）。
	red := dom.NewElement(doc, "div")
	red.SetAttribute("style",
		"position:absolute;left:0;top:0;width:40px;height:40px;background:#ff0000")
	body.AppendChild(red)

	dlg := dom.NewElement(doc, "dialog")
	body.AppendChild(dlg)
	dlg.SetAttribute("open", "")
	dlg.SetModalState(true)

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(300, 300)
	rv.Layout(nil)

	dlgObj := findRenderObject(rv, dlg)
	if dlgObj == nil {
		t.Fatal("渲染树中没有 flex 容器里 <dialog> 的渲染对象")
	}
	bd := dlgObj.PreviousSibling()
	if bd == nil {
		t.Fatal("flex 容器里的模态 dialog 之前应有 ::backdrop 渲染对象")
	}
	if bd.Node() != nil {
		t.Fatalf("::backdrop 渲染对象不应有 DOM 节点，实际 %T", bd.Node())
	}
	cs := bd.Style()
	if cs == nil || cs.Position != style.PositionFixed {
		t.Fatalf("::backdrop 应为 position:fixed，实际 %+v", cs)
	}
	if cs.BackgroundColor.A == 0 || cs.BackgroundColor.A == 255 {
		t.Fatalf("::backdrop 背景应为半透明，实际 %+v", cs.BackgroundColor)
	}
	// 两棵树成对：渲染对象必须链接到布局盒（linkLayoutBoxes 按匿名位置配对，
	// 结构错位时会配不上）。
	if bd.LayoutBox() == nil {
		t.Fatal("::backdrop 渲染对象没有链接到布局盒（两棵树结构错位）")
	}

	// 像素：flex 容器外/内都被 10% 黑遮罩压暗（红块 R≈229）。
	canvas := graphics.NewCanvas(300, 300)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 300, Height: 300})
	if got := canvas.PixelAt(20, 20); got.R < 210 || got.R > 245 || got.G > 12 || got.B > 12 {
		t.Fatalf("flex 场景模态时 (20,20) = %+v，want 被遮罩压暗的红（R≈229）", got)
	}

	// 同一文档里的非模态对照：flex 容器内的非模态 dialog 不生成遮罩。
	doc2 := dom.NewDocument()
	html2 := dom.NewElement(doc2, "html")
	doc2.AppendChild(html2)
	body2 := dom.NewElement(doc2, "body")
	html2.AppendChild(body2)
	body2.SetAttribute("style", "display:flex")
	dlg2 := dom.NewElement(doc2, "dialog")
	body2.AppendChild(dlg2)
	dlg2.SetAttribute("open", "")
	dlg2.SetModalState(false)
	rv2 := NewRenderTreeBuilder(resolver).Build(doc2)
	rv2.SetViewportSize(300, 300)
	rv2.Layout(nil)
	if obj2 := findRenderObject(rv2, dlg2); obj2 == nil {
		t.Fatal("渲染树中没有非模态 dialog 对象")
	} else if prev := obj2.PreviousSibling(); prev != nil && prev.Node() == nil {
		t.Fatalf("非模态 dialog 不应生成 ::backdrop（前一个兄弟是 %T）", prev)
	}
}