package webkit

import (
	"fmt"
	"testing"

	"wb-ui/rendering"
)

func TestZZSelDebug(t *testing.T) {
	wv := interactTestWebView(t, `<select id="s"><option value="">自动</option><option value="HD Webcam">HD Webcam</option></select>
<script>window.__chg='';document.getElementById('s').addEventListener('change',function(){window.__chg=document.getElementById('s').value});</script>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	rv0 := wv.RenderView()
	b := rv0.FindRenderBoxForNode(sel)
	cx, cy := b.AbsoluteX()+b.Width()/2, b.AbsoluteY()+b.Height()/2
	wv.HandleMouseButton(cx, cy, 0, 0)
	rv := wv.RenderView()
	fmt.Printf("press1 done popup=%v\n", len(collectByClass(doc, "select-popup")) > 0)
	// 重新取 option + box（新树）
	opts := collectByClass(doc, "select-popup-option")
	fmt.Printf("opts=%d\n", len(opts))
	ob := rv.FindRenderBoxForNode(opts[1])
	if ob == nil { t.Fatal("opt1 box nil") }
	ox, oy := ob.AbsoluteX()+ob.Width()/2, ob.AbsoluteY()+ob.Height()/2
	fmt.Printf("opt1 box=(%.0f,%.0f)%.0fx%.0f\n", ob.AbsoluteX(), ob.AbsoluteY(), ob.Width(), ob.Height())
	hit := rendering.HitTest(rv, ox, oy, "")
	fmt.Printf("hit opt1: %v dv=%q popupattr=%q\n", hit != nil, func() string { if hit != nil { return hit.GetAttribute("data-value") }; return "" }(), func() string { if hit != nil { return hit.GetAttribute("data-select-popup") }; return "" }())
	wv.HandleMouseButton(ox, oy, 0, 0)
	it := wv.Interaction()
	fmt.Printf("popup after click=%v\n", it.selectPopup != nil)
	v, _ := wv.EvalJS(`document.getElementById('s').value`)
	fmt.Printf("value=%q chg=%q\n", v.ToString(), func() string { c, _ := wv.EvalJS(`window.__chg`); return c.ToString() }())
}
