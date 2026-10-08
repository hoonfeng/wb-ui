package graphics

// 宿主图像转码兜底（AVIF 支持）的引擎侧测试。
//
// 这一层只验「兜底接线」本身：Skia 解码**失败**后是否去问宿主、宿主拒绝/未注册
// 时是否老老实实返回 nil、正常格式有没有被兜底路径影响。真实的 AVIF→PNG 转码
// 由 app 包的真 ffmpeg 用例覆盖（app/imagetranscode_test.go）；这里用假转码器，
// 保证引擎侧行为与宿主实现解耦。

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// transcoderTestPNG 造一张 8×8 的纯色 PNG（假转码器的「转码结果」）。
func transcoderTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{200, 30, 60, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG 夹具失败: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeImageFallsBackToHostTranscoder(t *testing.T) {
	pngBytes := transcoderTestPNG(t)
	SetImageTranscoder(func(data []byte) ([]byte, bool) {
		if string(data) == "fake-avif-bytes" {
			return pngBytes, true
		}
		return nil, false
	})
	t.Cleanup(func() { SetImageTranscoder(nil) })

	if !ImageTranscoderRegistered() {
		t.Fatal("注册后 ImageTranscoderRegistered 应为 true")
	}
	// ① Skia 解不了的字节 → 转码兜底后能画出。
	img := DecodeImage([]byte("fake-avif-bytes"))
	if img == nil {
		t.Fatal("注册了转码器后，Skia 解不了的字节应经转码兜底拿到图像")
	}
	if img.Width() != 8 || img.Height() != 8 {
		t.Fatalf("兜底解出 %dx%d，期望 8x8", img.Width(), img.Height())
	}
	// ② 转码器拒绝 → 必须返回 nil（不能拿半成品糊弄调用方）。
	if got := DecodeImage([]byte("not-transcodable")); got != nil {
		t.Fatal("转码器拒绝时 DecodeImage 必须返回 nil")
	}
	// ③ 常见格式仍走 Skia 原生路径（结果不受兜底影响）。
	direct := DecodeImage(pngBytes)
	if direct == nil {
		t.Fatal("PNG 必须仍能被原生解码")
	}
	if direct.Width() != 8 {
		t.Fatalf("原生解码宽度 = %d，期望 8", direct.Width())
	}
}

func TestDecodeImageWithoutHostTranscoder(t *testing.T) {
	SetImageTranscoder(nil)
	if ImageTranscoderRegistered() {
		t.Fatal("清除后 ImageTranscoderRegistered 应为 false")
	}
	if got := DecodeImage([]byte("fake-avif-bytes")); got != nil {
		t.Fatal("未注册转码器时，Skia 解不了的字节必须返回 nil（与改动前一致）")
	}
	if got := DecodeImage(nil); got != nil {
		t.Fatal("空数据必须返回 nil")
	}
}

// TestDecodeSizeUsesTranscoder 钉住「能画就能量尺寸」：布局层探测固有尺寸走的是
// 同一条 DecodeImage 路径，因此转码兜底必须同样生效（否则 AVIF 还是会在不给 CSS
// 尺寸时把盒子塌成 0）。
func TestDecodeSizeUsesTranscoder(t *testing.T) {
	pngBytes := transcoderTestPNG(t)
	SetImageTranscoder(func(data []byte) ([]byte, bool) { return pngBytes, true })
	t.Cleanup(func() { SetImageTranscoder(nil) })

	w, h, ok := DecodeSize([]byte("fake-avif-bytes"))
	if !ok || w != 8 || h != 8 {
		t.Fatalf("DecodeSize = %vx%v ok=%v，期望 8x8 ok=true（需经转码兜底）", w, h, ok)
	}
}
