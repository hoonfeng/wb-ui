package main

import (
	"testing"

	"image"
)

// TestOverflowEscapeViewportContainingBlock: 非定位 body + overflow:hidden +
// 高度 0（无在流内容）时，absolute 子元素（包含块=视口）必须**不被**
// body 裁剪——border 挂件模板的真实结构，此前整棵子树被 0 高裁剪裁没。
func TestOverflowEscapeViewportContainingBlock(t *testing.T) {
	c := TestCase{Name: "overflow_escape_viewport_cb", ViewportW: 400, ViewportH: 300, HTML: baseDoc(`
		<div class="frame" style="position:absolute;inset:0;background:#00ff88"></div>
	`, `<style>
		body{margin:0;background:transparent;overflow:hidden}
		div{position:static} /* 默认 */
	</style>`)}
	img, err := wbuiRenderPNG(c, c.ViewportW, c.ViewportH)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := img.RGBAAt(200, 150); got.G < 200 || got.R > 100 || got.B > 200 {
		t.Fatalf("center pixel = %+v, want green (escape body clip)", got)
	}
	// 整幅应是绿色（占满视口），抽查四角。
	for _, p := range []image.Point{{5, 5}, {394, 5}, {5, 294}, {394, 294}} {
		got := img.RGBAAt(p.X, p.Y)
		if got.G < 200 || got.R > 100 || got.B > 200 {
			t.Fatalf("corner %v = %+v, want green", p, got)
		}
	}
}

// TestOverflowClipContainedAbsolute: absolute 子元素的包含块在 overflow:hidden
// 容器**内部**（relative 祖先）时，容器裁剪必须仍然生效（回归保护：
// 逃逸规则不得把正常容器裁剪放开）。
func TestOverflowClipContainedAbsolute(t *testing.T) {
	c := TestCase{Name: "overflow_clip_contained_abs", ViewportW: 400, ViewportH: 300, HTML: baseDoc(`
		<div class="wrap">
			<div class="abs"></div>
		</div>
	`, `<style>
		body{margin:0}
		.wrap{position:relative;width:100px;height:100px;overflow:hidden;background:#cccccc}
		.abs{position:absolute;left:0;top:0;width:200px;height:200px;background:#ff0000}
	</style>`)}
	img, err := wbuiRenderPNG(c, c.ViewportW, c.ViewportH)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// 容器内 (50,50)：红色（可见）
	if got := img.RGBAAt(50, 50); got.R < 200 || got.G > 60 || got.B > 60 {
		t.Fatalf("inside pixel = %+v, want red", got)
	}
	// 容器外 (150,150)：被 wrap 的 overflow 裁剪 → 白（透明合成）
	if got := img.RGBAAt(150, 150); got.R > 230 && got.G > 230 && got.B > 230 {
		// ok white
	} else {
		t.Fatalf("outside pixel = %+v, want white (clipped)", got)
	}
}
