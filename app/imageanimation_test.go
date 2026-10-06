package app

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// TestAnimatedImageSourceDecodesGIFAndAdvances：宿主源（goskia SkCodec）+ 渲染层动图
// 通道的**集成**验证（不起窗口、不进页面）——自检判据 A4-1 失败时靠它把问题收敛到
// 「宿主源没生效」还是「页面绘制路径没走这个入口」。
func TestAnimatedImageSourceDecodesGIFAndAdvances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anim.gif")
	pal := color.Palette{
		color.RGBA{R: 204, G: 51, B: 51, A: 255},
		color.RGBA{R: 51, G: 204, B: 51, A: 255},
	}
	var g gif.GIF
	for i := 0; i < 2; i++ {
		f := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				f.SetColorIndex(x, y, uint8(i))
			}
		}
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 10) // 1/100 秒 → 100ms
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &g); err != nil {
		t.Fatalf("编码 GIF 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 GIF 失败: %v", err)
	}

	off := InstallAnimatedImageSource()
	defer off()
	rendering.ResetAnimatedImages()
	defer rendering.ResetAnimatedImages()

	url := "file://" + filepath.ToSlash(path)
	first := rendering.LoadImageSync(url)
	if first == nil {
		t.Fatal("动图首帧应可加载（解码失败 → 画面会空白而不是静止）")
	}
	if !rendering.HasAnimatedImages() {
		t.Fatal("宿主源未识别为动图（HasAnimatedImages = false）：goskia 的 SkCodec 没生效？")
	}
	time.Sleep(150 * time.Millisecond) // 跨过一帧（100ms）
	second := rendering.LoadImageSync(url)
	if second == nil {
		t.Fatal("第二帧应可加载")
	}
	if second == first {
		t.Error("150ms 后应换帧（拿到同一个 DecodedImage 指针 = 引擎没按帧时长选帧）")
	}
}
