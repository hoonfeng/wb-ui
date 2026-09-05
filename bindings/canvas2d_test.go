// bindings/canvas2d_test.go — CanvasRenderingContext2D 的绑定层测试：
// JS 侧 getContext('2d') → Go native 绘制 → 位图像素断言。
package bindings

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/rendering"
)

// mkCanvas 在 Go 侧建 canvas 元素（带固定 id）挂到文档，JS 侧经
// getElementById 取用（测试环境 document 无 appendChild 包装）。
func mkCanvas(t *testing.T, doc *dom.Document, w, h int) *dom.Element {
	t.Helper()
	el := doc.CreateElement("canvas")
	el.SetId("cv")
	el.SetAttribute("width", itoa(w))
	el.SetAttribute("height", itoa(h))
	doc.AppendChild(el)
	return el
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [12]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestCanvas2DFillRectPixel(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		if (!ctx) throw new Error("getContext('2d') returned falsy");
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(10, 10, 20, 20);
		ctx.fillStyle = 'rgb(0, 255, 0)';
		ctx.fillRect(40, 10, 20, 20);
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	r := bm.Cv.PixelAt(15, 15)
	if r.R < 200 || r.G > 60 || r.B > 60 {
		t.Fatalf("red fill pixel = %+v, want ~#ff0000", r)
	}
	g := bm.Cv.PixelAt(45, 15)
	if g.G < 200 || g.R > 60 {
		t.Fatalf("green fill pixel = %+v, want ~#00ff00", g)
	}
	out := bm.Cv.PixelAt(90, 70)
	if out.A != 0 {
		t.Fatalf("outside fill pixel = %+v, want transparent", out)
	}
}

func TestCanvas2DPathArcFill(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 100)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#0000ff';
		ctx.beginPath();
		ctx.arc(50, 50, 30, 0, Math.PI * 2);
		ctx.fill();
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	center := bm.Cv.PixelAt(50, 50)
	if center.B < 200 {
		t.Fatalf("circle center = %+v, want blue", center)
	}
	corner := bm.Cv.PixelAt(5, 5)
	if corner.A != 0 {
		t.Fatalf("corner = %+v, want transparent", corner)
	}
}

func TestCanvas2DMeasureTextNative(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	mkCanvas(t, doc, 300, 150)
	// measureText 走 __wbMeasureText（Skia 精确 advance）
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.font = '13px Consolas';
		var m = ctx.measureText('W');
		window.__mw = m.width;
		if (typeof m.width !== 'number' || !(m.width > 0)) throw new Error("measureText bad width: " + m.width);
	`)
}

func TestCanvas2DTranslateAndRotate(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 200, 100)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#00ffff';
		ctx.translate(20, 30);
		ctx.fillRect(0, 0, 30, 30);
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	p := bm.Cv.PixelAt(25, 35)
	if p.G < 200 || p.B < 200 {
		t.Fatalf("translated fill pixel = %+v, want cyan", p)
	}
	// 未平移区域应透明
	out := bm.Cv.PixelAt(5, 5)
	if out.A != 0 {
		t.Fatalf("pixel without translate = %+v, want transparent", out)
	}
}

func TestCanvas2DGlobalAlpha(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 50)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#ffffff';
		ctx.globalAlpha = 0.5;
		ctx.fillRect(0, 0, 100, 50);
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	p := bm.Cv.PixelAt(50, 25)
	if p.A > 180 || p.A < 100 {
		t.Fatalf("globalAlpha 0.5 fill alpha = %d, want ~128", p.A)
	}
}

func TestCanvas2DSaveRestoreStyle(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 200, 50)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(0, 0, 50, 50);
		ctx.save();
		ctx.fillStyle = '#00ff00';
		ctx.fillRect(50, 0, 50, 50);
		ctx.restore();
		ctx.fillRect(100, 0, 50, 50); // restore 后应回到红色
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	r1 := bm.Cv.PixelAt(25, 25)
	r2 := bm.Cv.PixelAt(75, 25)
	r3 := bm.Cv.PixelAt(125, 25)
	if r1.R < 200 || r1.G > 60 {
		t.Fatalf("first = %+v, want red", r1)
	}
	if r2.G < 200 || r2.R > 60 {
		t.Fatalf("middle = %+v, want green", r2)
	}
	if r3.R < 200 || r3.G > 60 {
		t.Fatalf("after restore = %+v, want red", r3)
	}
}

func TestCanvas2DGetImageData(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	mkCanvas(t, doc, 100, 80)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(0, 0, 20, 20);
		var d = ctx.getImageData(0, 0, 2, 2);
		window.__px0 = d.data[0];
		window.__px3 = d.data[3];
		if (d.width !== 2 || d.height !== 2) throw new Error("bad ImageData dims");
		if (!(d.data[0] > 200 && d.data[3] > 200)) throw new Error("bad ImageData px");
	`)
}

func TestCanvas2DStyleParse(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 100)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		// 非法颜色回退黑色
		ctx.fillStyle = 'not-a-color';
		ctx.fillRect(0, 0, 10, 10);
		// 渐变（线性）
		var g = ctx.createLinearGradient(0, 0, 100, 0);
		g.addColorStop(0, '#ff0000');
		g.addColorStop(1, '#0000ff');
		ctx.fillStyle = g;
		ctx.fillRect(0, 0, 100, 100);
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	left := bm.Cv.PixelAt(5, 50)
	right := bm.Cv.PixelAt(95, 50)
	if left.R < 150 || left.B > 120 {
		t.Fatalf("gradient left = %+v, want red-ish", left)
	}
	if right.B < 150 || right.R > 120 {
		t.Fatalf("gradient right = %+v, want blue-ish", right)
	}
}

// TestCanvas2DClipTransformDrawImage：clip + transform + drawImage 组合
//（live2d 三角形贴图渲染的精确路径）。cv 绿底 + 三角 clip + 20 倍缩放
// drawImage(cv2 蓝色 4x4)：三角形区域变蓝，外侧保持绿。
func TestCanvas2DClipTransformDrawImage(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	cv := mkCanvas(t, doc, 200, 200)
	_ = cv
	cv2 := doc.CreateElement("canvas")
	cv2.SetId("cv2")
	cv2.SetAttribute("width", "4")
	cv2.SetAttribute("height", "4")
	doc.AppendChild(cv2)
	// cv2 先画蓝
	mustRun(t, rt, `
		var c2 = document.getElementById('cv2');
		var ctx2 = c2.getContext('2d');
		ctx2.fillStyle = '#0000ff';
		ctx2.fillRect(0, 0, 4, 4);
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#00ff00';
		ctx.fillRect(0, 0, 200, 200);
		ctx.save();
		ctx.beginPath();
		ctx.moveTo(50, 50); ctx.lineTo(110, 50); ctx.lineTo(50, 110);
		ctx.closePath();
		ctx.clip();
		ctx.transform(20, 0, 0, 20, 0, 0); // 4x4 → 80x80
		ctx.drawImage(c2, 0, 0);
		ctx.restore();
	`)
	// getContext 创建的位图（200×200）
	bm := canvasBitmapOf(t, doc, "cv")
	if bm == nil {
		t.Fatal("no cv bitmap")
	}
	inTri := bm.Cv.PixelAt(60, 60)
	if inTri.B < 200 {
		t.Fatalf("in-triangle (60,60) = %+v, want blue (drawImage inside clip+transform)", inTri)
	}
	outTri := bm.Cv.PixelAt(30, 30)
	if outTri.G < 200 || outTri.B > 60 {
		t.Fatalf("outside triangle (30,30) = %+v, want green", outTri)
	}
	// 三角形外但被 80x80 蓝色矩形覆盖的区域（无 clip 保护时会有蓝）：
	// (90,20) 在 clip 三角形外 → 应为绿
	side := bm.Cv.PixelAt(90, 20)
	if side.G < 200 {
		t.Fatalf("clip-protected (90,20) = %+v, want green", side)
	}
}

func canvasBitmapOf(t *testing.T, doc *dom.Document, id string) *rendering.CanvasBitmap {
	t.Helper()
	var walk func(n dom.Node) *rendering.CanvasBitmap
	walk = func(n dom.Node) *rendering.CanvasBitmap {
		if el, ok := n.(*dom.Element); ok && el.GetAttribute("id") == id {
			if bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap); ok {
				return bm
			}
			return nil
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if b := walk(c); b != nil {
				return b
			}
		}
		return nil
	}
	return walk(doc)
}

// TestCanvas2DTransformTranslateOnly：transform 带平移（缩放+位移组合）。
func TestCanvas2DTransformTranslateOnly(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	mkCanvas(t, doc, 300, 200)
	c2 := doc.CreateElement("canvas")
	c2.SetId("c2")
	c2.SetAttribute("width", "64")
	c2.SetAttribute("height", "64")
	doc.AppendChild(c2)
	mustRun(t, rt, `
		var c2 = document.getElementById('c2');
		var cx2 = c2.getContext('2d');
		cx2.fillStyle = '#ff0000';
		cx2.fillRect(0, 0, 64, 64);
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#00ff00';
		ctx.fillRect(0, 0, 300, 200);
		// transform 带平移：缩放 0.25 + 平移(200, 50) → 64px 图 → 16px 屏幕区
		ctx.save();
		ctx.transform(0.25, 0, 0, 0.25, 200, 50);
		ctx.drawImage(c2, 0, 0);
		ctx.restore();
	`)
	bm := canvasBitmapOf(t, doc, "cv")
	if bm == nil {
		t.Fatal("no cv bitmap")
	}
	in := bm.Cv.PixelAt(210, 60) // 屏幕 (200,50)-(216,66) 内
	if in.R < 200 {
		t.Fatalf("transform+translate (210,60) = %+v, want red", in)
	}
	out := bm.Cv.PixelAt(280, 100)
	if out.G < 200 || out.R > 60 {
		t.Fatalf("outside (280,100) = %+v, want green", out)
	}
}

func TestCanvas2DClipAndTransform(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 200, 200)
	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		// A. clip 三角形（0,0)(60,0)(0,60) + 满幅 fillRect：仅三角形内写入
		ctx.save();
		ctx.beginPath();
		ctx.moveTo(0, 0); ctx.lineTo(60, 0); ctx.lineTo(0, 60);
		ctx.closePath();
		ctx.clip();
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(0, 0, 200, 200);
		ctx.restore();
		// B. translate(100, 100) 后 fillRect(0,0,30,30)
		ctx.save();
		ctx.translate(100, 100);
		ctx.fillStyle = '#00ff00';
		ctx.fillRect(0, 0, 30, 30);
		ctx.restore();
		// C. transform 缩放（2x）+ 原点的 fillRect(0,0,10,10) → 屏幕 (0,0)-(20,20)
		ctx.save();
		ctx.transform(2, 0, 0, 2, 0, 0);
		ctx.fillStyle = '#0000ff';
		ctx.fillRect(0, 0, 10, 10);
		ctx.restore();
	`)
	bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap)
	if !ok || bm == nil {
		t.Fatal("no bitmap")
	}
	// A: 三角形内 (2,40) 应有红色；三角外 (80,80) 应透明
	inTri := bm.Cv.PixelAt(2, 40)
	if inTri.R < 200 || inTri.G > 60 {
		t.Fatalf("clip inner = %+v, want red", inTri)
	}
	outTri := bm.Cv.PixelAt(80, 80)
	if outTri.A != 0 {
		t.Fatalf("clip outer = %+v, want transparent", outTri)
	}
	// B: translate 后 (110,110) 有绿色
	tr := bm.Cv.PixelAt(110, 110)
	if tr.G < 200 || tr.R > 60 {
		t.Fatalf("translate = %+v, want green", tr)
	}
	// C: transform 2x 后 (15,15) 有蓝色、(5,5) 红色被覆盖为蓝？
	tf := bm.Cv.PixelAt(15, 15)
	if tf.B < 200 {
		t.Fatalf("transform = %+v, want blue", tf)
	}
}
