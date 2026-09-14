// engine/js/bindings/canvas2d_api_test.go — canvas 2D 此前缺失/空实现的 API：
// arcTo / roundRect / isPointInPath / isPointInStroke / createPattern /
// createImageData / putImageData 的 dirty 矩形 / setLineDash（真实虚线绘制）
// / shadow*（真实阴影）/ getTransform / reset。
package bindings

import (
	"testing"

	"wb-ui/engine/js/jsc"
	"wb-ui/engine/rendering"
)

// jsString 求值一个 JS 表达式并返回字符串结果（JSON.stringify 的断言载体）。
func jsString(t *testing.T, rt *jsc.Interpreter, expr string) string {
	t.Helper()
	v, err := rt.RunJS(expr)
	if err != nil {
		t.Fatalf("eval %q failed: %v", expr, err)
	}
	return v.ToString()
}

// TestCanvas2DRoundRect 覆盖 roundRect：圆角外的角必须透明，圆角内与无圆角
// 矩形必须填满（半径 0 / 数组 / {x,y} 三种参数形式）。
func TestCanvas2DRoundRect(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.fillStyle = '#ff0000';
		ctx.beginPath();
		ctx.roundRect(0, 0, 20, 20, 8);
		ctx.fill();
		ctx.beginPath();
		ctx.roundRect(30, 0, 20, 20, 0);
		ctx.fill();
		ctx.beginPath();
		ctx.roundRect(0, 30, 20, 20, [4, 8]);
		ctx.fill();
		ctx.beginPath();
		ctx.roundRect(30, 30, 20, 20, [{ x: 6, y: 2 }]);
		ctx.fill();
	`)
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	// 圆角 8：左上角 (1,1) 在圆角外（抗锯齿残留极小）；(10,10) 内部 → 红
	if px := bm.Cv.PixelAt(1, 1); px.A > 100 {
		t.Fatalf("roundRect 圆角外像素应近透明，got %+v", px)
	}
	if px := bm.Cv.PixelAt(10, 10); px.R < 200 || px.A < 200 {
		t.Fatalf("roundRect 内部像素 = %+v，want 不透明红", px)
	}
	// 半径 0：直角，角上必须有像素
	if px := bm.Cv.PixelAt(31, 1); px.R < 200 {
		t.Fatalf("半径 0 时直角像素应有颜色，got %+v", px)
	}
	// [4, 8]：tl=4 → (30,30)（角点）在圆角外
	if px := bm.Cv.PixelAt(30, 30); px.A != 0 {
		t.Fatalf("radii=[4,8] 时左上圆角外像素应透明，got %+v", px)
	}
	if px := bm.Cv.PixelAt(40, 40); px.R < 200 {
		t.Fatalf("radii=[4,8] 内部像素应有颜色，got %+v", px)
	}
	// {x:6,y:2} 椭圆角：角点 (30,30) 在圆角外、内部 (45,40) 有颜色
	if px := bm.Cv.PixelAt(30, 30); px.A != 0 {
		t.Fatalf("椭圆角外像素应透明，got %+v", px)
	}
	if px := bm.Cv.PixelAt(45, 40); px.R < 200 {
		t.Fatalf("椭圆角矩形内部应有颜色，got %+v", px)
	}
}

// TestCanvas2DArcTo 覆盖 arcTo：几何上弧与两条切线相切（isPointInStroke 命中
// 两个切点与起点）、远离路径的点不命中，且弧中点真的被描边（像素）。
func TestCanvas2DArcTo(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.beginPath();
		ctx.moveTo(5, 5);
		ctx.arcTo(5, 35, 35, 35, 10);
		ctx.lineTo(35, 35);
		globalThis.__arc = JSON.stringify([
			ctx.isPointInStroke(5, 25),
			ctx.isPointInStroke(15, 35),
			ctx.isPointInStroke(5, 5),
			ctx.isPointInStroke(30, 10)
		]);
		ctx.strokeStyle = '#00ff00';
		ctx.lineWidth = 3;
		ctx.stroke();
	`)
	if got := jsString(t, rt, `globalThis.__arc`); got != "[true,true,true,false]" {
		t.Fatalf("arcTo 几何命中 = %v，want [true,true,true,false]", got)
	}
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	// 弧中点 ≈ (7.9, 32.1)（圆心 (15,25)、半径 10、扫过 180°→90°）
	found := false
	for x := 5; x <= 11 && !found; x++ {
		for y := 29; y <= 35; y++ {
			if px := bm.Cv.PixelAt(x, y); px.G > 150 && px.A > 150 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("arcTo 的弧中点附近没有描边像素")
	}
}

// TestCanvas2DPointInPathStroke 覆盖 isPointInPath 的 nonzero/evenodd 与
// isPointInStroke 的线宽语义。
func TestCanvas2DPointInPathStroke(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.beginPath();
		ctx.rect(10, 10, 20, 20);
		globalThis.__pip1 = JSON.stringify([
			ctx.isPointInPath(20, 20),
			ctx.isPointInPath(5, 20),
			ctx.isPointInPath(20, 20, 'evenodd')
		]);
		// 两条同方向的同心矩形：nonzero 内部仍算内（winding=2），evenodd 算外
		ctx.beginPath();
		ctx.rect(0, 0, 40, 40);
		ctx.rect(10, 10, 20, 20);
		globalThis.__pip2 = JSON.stringify([
			ctx.isPointInPath(20, 20),
			ctx.isPointInPath(20, 20, 'evenodd'),
			ctx.isPointInPath(5, 5, 'evenodd'),
			ctx.isPointInPath(50, 50)
		]);
		// 描边：线宽 6 → 中心线 ±3
		ctx.lineWidth = 6;
		ctx.beginPath();
		ctx.moveTo(0, 60);
		ctx.lineTo(60, 60);
		globalThis.__stroke = JSON.stringify([
			ctx.isPointInStroke(30, 60),
			ctx.isPointInStroke(30, 63),
			ctx.isPointInStroke(30, 64),
			ctx.isPointInStroke(70, 60)
		]);
	`)
	if got := jsString(t, rt, `globalThis.__pip1`); got != "[true,false,true]" {
		t.Fatalf("单矩形 isPointInPath = %v，want [true,false,true]", got)
	}
	if got := jsString(t, rt, `globalThis.__pip2`); got != "[true,false,true,false]" {
		t.Fatalf("同心矩形 isPointInPath = %v，want [true,false,true,false]", got)
	}
	if got := jsString(t, rt, `globalThis.__stroke`); got != "[true,true,false,false]" {
		t.Fatalf("isPointInStroke = %v，want [true,true,false,false]", got)
	}
}

// TestCanvas2DPattern 覆盖 createPattern：源 canvas 的图案平铺填充（shader 由
// 图案注册表持有，可跨多次绘制复用）。
func TestCanvas2DPattern(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		var src = document.createElement('canvas');
		src.width = 4;
		src.height = 4;
		var sc = src.getContext('2d');
		sc.fillStyle = '#0000ff';
		sc.fillRect(0, 0, 2, 2);

		var pat = ctx.createPattern(src, 'repeat');
		globalThis.__pat = JSON.stringify([pat !== null, typeof pat.setTransform === 'function']);
		ctx.fillStyle = pat;
		ctx.fillRect(0, 0, 8, 8);

		// 不可用作图案源 → null
		globalThis.__patNull = ctx.createPattern({}, 'repeat') === null;
	`)
	if got := jsString(t, rt, `globalThis.__pat`); got != "[true,true]" {
		t.Fatalf("createPattern 返回值 = %v", got)
	}
	if got := jsString(t, rt, `globalThis.__patNull`); got != "true" {
		t.Fatalf("无效源应返回 null，got %v", got)
	}
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	if px := bm.Cv.PixelAt(1, 1); px.B < 200 || px.A < 200 {
		t.Fatalf("图案第一块像素 = %+v，want 不透明蓝", px)
	}
	if px := bm.Cv.PixelAt(3, 1); px.A != 0 {
		t.Fatalf("图案空格像素应透明（repeat 的 4x4 平铺内），got %+v", px)
	}
	if px := bm.Cv.PixelAt(4, 1); px.B < 200 {
		t.Fatalf("图案应逐 4px 平铺，(4,1) = %+v，want 蓝", px)
	}
}

// TestCanvas2DPatternNoRepeat 覆盖 no-repeat 的平铺语义：图案只在原点铺一次
// （Skia 的 Decal 平铺），填充区域的其余部分为空。
func TestCanvas2DPatternNoRepeat(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		var src = document.createElement('canvas');
		src.width = 4;
		src.height = 4;
		var sc = src.getContext('2d');
		sc.fillStyle = '#0000ff';
		sc.fillRect(0, 0, 4, 4);
		ctx.fillStyle = ctx.createPattern(src, 'no-repeat');
		ctx.fillRect(0, 0, 20, 20);
	`)
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	if px := bm.Cv.PixelAt(1, 1); px.B < 200 || px.A < 200 {
		t.Fatalf("no-repeat 原点图案像素 = %+v，want 不透明蓝", px)
	}
	if px := bm.Cv.PixelAt(5, 5); px.A != 0 {
		t.Fatalf("no-repeat 时 (5,5) 应为空，got %+v", px)
	}
}

// TestCanvas2DCreateImageDataAndDirtyPut 覆盖 createImageData 与 putImageData
// 的 dirty 矩形（第 4-7 参）：只更新指定子区域，区域外保持原内容。
func TestCanvas2DCreateImageDataAndDirtyPut(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		var img = ctx.createImageData(4, 4);
		globalThis.__img = JSON.stringify([
			img.width, img.height, img.data.length,
			typeof img.data[0] === 'number'
		]);
		// 像素 0 = 红
		img.data[0] = 255; img.data[3] = 255;
		ctx.putImageData(img, 0, 0);

		// 像素 1 = 绿；dirty 只写 (1,0)
		var img2 = ctx.createImageData(4, 4);
		img2.data[5] = 255; img2.data[7] = 255;
		ctx.putImageData(img2, 0, 0, 1, 0, 1, 1);
	`)
	if got := jsString(t, rt, `globalThis.__img`); got != "[4,4,64,true]" {
		t.Fatalf("createImageData = %v，want [4,4,64,true]", got)
	}
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	if px := bm.Cv.PixelAt(0, 0); px.R < 200 || px.A < 200 {
		t.Fatalf("(0,0) 应为红色，got %+v", px)
	}
	if px := bm.Cv.PixelAt(1, 0); px.G < 200 || px.A < 200 {
		t.Fatalf("dirty 区域内的 (1,0) 应为绿色，got %+v", px)
	}
	if px := bm.Cv.PixelAt(2, 0); px.A != 0 {
		t.Fatalf("dirty 区域外（2,0）不应被写入，got %+v", px)
	}
}

// TestCanvas2DLineDash 覆盖 setLineDash/getLineDash 与真实虚线描边。
func TestCanvas2DLineDash(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.setLineDash([4, 4]);
		globalThis.__dash = JSON.stringify([ctx.getLineDash().length, ctx.getLineDash()[0], ctx.getLineDash()[1]]);
		ctx.setLineDash([3]);
		globalThis.__odd = JSON.stringify([ctx.getLineDash().length, ctx.getLineDash()[1]]);
		ctx.setLineDash([4, 4]);
		ctx.strokeStyle = '#ffffff';
		ctx.lineWidth = 2;
		ctx.beginPath();
		ctx.moveTo(0, 10);
		ctx.lineTo(40, 10);
		ctx.stroke();
		// 全 0 视为实线
		ctx.setLineDash([0, 0]);
		globalThis.__zero = ctx.getLineDash().length;
	`)
	if got := jsString(t, rt, `globalThis.__dash`); got != "[2,4,4]" {
		t.Fatalf("getLineDash = %v，want [2,4,4]", got)
	}
	if got := jsString(t, rt, `globalThis.__odd`); got != "[2,3]" {
		t.Fatalf("奇数阵列应复制成偶数 = %v，want [2,3]", got)
	}
	if got := jsString(t, rt, `globalThis.__zero`); got != "0" {
		t.Fatalf("全 0 虚线应视为实线（空阵列），got %v", got)
	}
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	if px := bm.Cv.PixelAt(2, 10); px.A < 200 {
		t.Fatalf("虚线 on 段 (2,10) 应有像素，got %+v", px)
	}
	if px := bm.Cv.PixelAt(6, 10); px.A != 0 {
		t.Fatalf("虚线 gap 段 (6,10) 应透明，got %+v", px)
	}
	if px := bm.Cv.PixelAt(10, 10); px.A < 200 {
		t.Fatalf("虚线第二个 on 段 (10,10) 应有像素，got %+v", px)
	}
}

// TestCanvas2DShadow 覆盖 shadow*（此前属性可读写但不参与绘制）。
func TestCanvas2DShadow(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.shadowColor = 'rgba(0, 0, 0, 1)';
		ctx.shadowBlur = 0;
		ctx.shadowOffsetX = 10;
		ctx.shadowOffsetY = 10;
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(0, 0, 10, 10);
	`)
	bm, _ := el.CanvasSurface().(*rendering.CanvasBitmap)
	if bm == nil {
		t.Fatal("canvas has no backing bitmap")
	}
	if px := bm.Cv.PixelAt(5, 5); px.R < 200 {
		t.Fatalf("形状本体应保持红色，got %+v", px)
	}
	shadow := bm.Cv.PixelAt(15, 15)
	if shadow.A < 200 || shadow.R > 80 || shadow.G > 80 || shadow.B > 80 {
		t.Fatalf("偏移处应有黑色阴影，got %+v", shadow)
	}
	if px := bm.Cv.PixelAt(30, 30); px.A != 0 {
		t.Fatalf("阴影之外应透明，got %+v", px)
	}
}

// TestCanvas2DTransformAndReset 覆盖 getTransform 与 reset()。
func TestCanvas2DTransformAndReset(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	mkCanvas(t, doc, 100, 80)

	mustRun(t, rt, `
		var c = document.getElementById('cv');
		var ctx = c.getContext('2d');
		ctx.scale(2, 3);
		ctx.translate(5, 7);
		var m = ctx.getTransform();
		globalThis.__m = JSON.stringify([m.a, m.b, m.c, m.d, m.e, m.f]);

		ctx.fillStyle = '#123456';
		ctx.lineWidth = 7;
		ctx.setLineDash([2, 2]);
		ctx.shadowBlur = 5;
		ctx.reset();
		var m2 = ctx.getTransform();
		globalThis.__after = JSON.stringify([
			m2.a, m2.d, m2.e, m2.f,
			ctx.fillStyle, ctx.lineWidth, ctx.getLineDash().length, ctx.shadowBlur
		]);
	`)
	if got := jsString(t, rt, `globalThis.__m`); got != "[2,0,0,3,10,21]" {
		t.Fatalf("getTransform = %v，want [2,0,0,3,10,21]", got)
	}
	if got := jsString(t, rt, `globalThis.__after`); got != `[1,1,0,0,"#000000",1,0,0]` {
		t.Fatalf("reset() 之后 = %v，want [1,1,0,0,\"#000000\",1,0,0]", got)
	}
}
