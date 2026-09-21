package graphics

import "testing"

// PixelAt/Pixels 的像素语义契约：Skia 后端 surface 是 N32Premul，读出来的是
// **预乘**字节（R/G/B 已经乘过 A），而不是浏览器 getImageData 那样的非预乘值。
// 需要浏览器语义的调用方必须自己反预乘——engine/js/bindings/canvas2d.go 的
// getImageData 正是这么做的（"解除预乘 Skia N32 premul → 浏览器 unpremul"）。
//
// 这条契约此前既没写进注释也没测试保护，踩过一次坑：dev/suites/consistency
// 的白底合成按非预乘处理 PixelAt 的返回值、又乘了一次 A，于是所有抗锯齿边缘
// 被压暗成 a²·src+(1-a)·255（TestPxTransformCompose: got=#bf7777 vs exp=#ff7777）。
func TestPixelAtReturnsPremultiplied(t *testing.T) {
	// 50% alpha 红画在透明底上：预乘 => (128,0,0,128)；非预乘会是 (255,0,0,128)。
	c := NewCanvas(20, 20)
	defer c.Release()
	c.FillRect(2, 2, 10, 10, Color{R: 255, G: 0, B: 0, A: 128})
	if p := c.PixelAt(5, 5); !near8(p.R, 128) || p.G > 1 || p.B > 1 || !near8(p.A, 128) {
		t.Errorf("50%% alpha red on transparent = {R:%d G:%d B:%d A:%d}, want {128 0 0 128} (premultiplied)",
			p.R, p.G, p.B, p.A)
	}

	// 25% alpha：预乘 => (64,0,0,64)；非预乘会是 (255,0,0,64)。
	c2 := NewCanvas(20, 20)
	defer c2.Release()
	c2.FillRect(2, 2, 10, 10, Color{R: 255, G: 0, B: 0, A: 64})
	if q := c2.PixelAt(5, 5); !near8(q.R, 64) || q.G > 1 || !near8(q.A, 64) {
		t.Errorf("25%% alpha red = {R:%d G:%d B:%d A:%d}, want {64 0 0 64} (premultiplied)",
			q.R, q.G, q.B, q.A)
	}

	// 合成到不透明底上：读出的是普通不透明色（A=255 时预乘与非预乘等价），
	// 这也是"把预乘值当非预乘再合成"会出错的反例——正确值只有一个。
	c3 := NewCanvas(20, 20)
	defer c3.Release()
	c3.Clear(Color{R: 255, G: 255, B: 255, A: 255})
	c3.FillRect(2, 2, 10, 10, Color{R: 255, G: 0, B: 0, A: 128})
	if r := c3.PixelAt(5, 5); r.A != 255 || r.R < 252 || r.G < 124 || r.G > 132 || r.B < 124 || r.B > 132 {
		t.Errorf("50%% alpha red over opaque white = {R:%d G:%d B:%d A:%d}, want ~{255 127 127 255}",
			r.R, r.G, r.B, r.A)
	}
}

// near8 reports whether v is within 2 of want (Skia's 8-bit premultiply rounds).
func near8(v uint8, want int) bool {
	d := int(v) - want
	return d >= -2 && d <= 2
}
