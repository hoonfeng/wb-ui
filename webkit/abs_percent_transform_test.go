package webkit

import "testing"

// 最小复现：gou-ide 顶栏导航 `.tb-nav` 的定位方式
//   position:absolute; left:50%; transform:translateX(-50%)
// 实测 y = -254（= -(nav 宽 508)/2），即 translateX 的**水平**位移被串到了
// 垂直方向，导航胶囊整体跑到顶栏上方视口外。
const absPercentPage = `<!DOCTYPE html><html><head><style>
body{margin:0}
#bar{position:relative;display:flex;align-items:center;height:40px;padding:0 8px}
#nav{position:absolute;left:50%;transform:translateX(-50%);display:flex;height:24px}
#pill{display:inline-flex;height:24px;padding:0 10px}
</style></head><body><div id="bar"><div id="nav"><span id="pill">a</span></div></div></body></html>`

func TestAbsoluteLeft50PercentTranslateX(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1600, 1000)
	if err := wv.LoadHTML(absPercentPage); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	rect := dynEval(t, wv, `(function(){var r=document.getElementById("nav").getBoundingClientRect();return Math.round(r.x)+","+Math.round(r.y)+","+Math.round(r.width)+"x"+Math.round(r.height);})()`)
	t.Logf("nav getBoundingClientRect = %s （期望 y=8，x=(1600-宽)/2）", rect)
	doc, rv := wv.Document(), wv.RenderView()
	if doc == nil || rv == nil {
		t.Fatal("doc/renderView nil")
	}
	el := doc.GetElementById("nav")
	if el == nil {
		t.Fatal("#nav 不存在")
	}
	if b := rv.FindRenderBoxForNode(el); b != nil {
		t.Logf("nav layout box = x=%.1f y=%.1f w=%.1f h=%.1f", b.X(), b.Y(), b.Width(), b.Height())
	} else {
		t.Log("nav layout box = nil")
	}
	if el := doc.GetElementById("pill"); el != nil {
		if b := rv.FindRenderBoxForNode(el); b != nil {
			t.Logf("pill layout box = x=%.1f y=%.1f w=%.1f h=%.1f", b.X(), b.Y(), b.Width(), b.Height())
		}
	}
}

// CSS Transforms L1 §2：translate(tx) 单值时第二轴为 **0**（只有 scale() 是双轴同值）。
// 此前实现为 ty := tx，导致 translate(-50%) 这类「水平居中」写法把元素整体上移
// 自身宽度的一半——gou-ide 顶栏导航组（宽 508）因此被上移 254px 移出视口。
func TestTranslateSingleValueSecondAxisIsZero(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1600, 1000)
	const page = `<!DOCTYPE html><html><head><style>
body{margin:0}
#box{position:relative;width:200px;height:50px}
#in{position:absolute;left:50%;transform:translate(-50%);width:100px;height:20px}
</style></head><body><div id="box"><div id="in"></div></div></body></html>`
	if err := wv.LoadHTML(page); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got := dynEval(t, wv, `(function(){var r=document.getElementById("in").getBoundingClientRect();return Math.round(r.x)+","+Math.round(r.y);})()`)
	if got != "50,0" {
		t.Fatalf("单值 translate(-50%%)：rect(x,y)=%s（期望 50,0 —— x 居中位移 50，y 必须保持 0）", got)
	}
}
