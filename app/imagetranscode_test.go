package app

// 宿主图像转码器（AVIF 支持）的测试。
//
// 分两半，缺一不可：
//
//	① **白名单与格式判定**——按内容判定 AVIF（不看扩展名，与引擎「按内容解码」
//	   的口径一致），且**只**放行浏览器支持的格式（TIFF 之类必须继续拒绝，否则
//	   就不是「对齐浏览器」而是「引擎比浏览器更强」）；
//	② **真 ffmpeg 转码链路**——AVIF 字节 → PNG 字节，且输出能被 Skia 解码、
//	   尺寸与源一致（只看「函数返回 true」是不够的，转出来的东西必须真能画）。
//
// 夹具由测试自己造（Go 画 PNG → 本机 ffmpeg 编 AVIF），不依赖 dev/media/samples
// （样本不入库，见 media-format-verification-plan.md 决策 5）；本机没有 ffmpeg
// 或 ffmpeg 编不出 AVIF 时 skip（与媒体链路「没有外部解码器就不假装通过」一致）。

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoonfeng/goskia/skia"

	"wb-ui/engine/platform/graphics"
)

// ftypBox 造一个 ISO-BMFF 的 `ftyp` 盒（AVIF/HEIF 的容器头）。
func ftypBox(major string, compat ...string) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 0, 0}) // 盒长占位
	b.WriteString("ftyp")
	b.WriteString(major)
	b.Write([]byte{0, 0, 0, 0}) // minor_version
	for _, c := range compat {
		b.WriteString(c)
	}
	raw := b.Bytes()
	binary.BigEndian.PutUint32(raw[0:4], uint32(len(raw)))
	return raw
}

// synthPNG 画一张 64×48 的四色块 PNG（转码夹具的源图）。
const (
	fixtureW, fixtureH = 64, 48
)

func synthPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, fixtureW, fixtureH))
	for y := 0; y < fixtureH; y++ {
		for x := 0; x < fixtureW; x++ {
			var c color.RGBA
			switch {
			case x < fixtureW/2 && y < fixtureH/2:
				c = color.RGBA{255, 0, 0, 255}
			case x >= fixtureW/2 && y < fixtureH/2:
				c = color.RGBA{0, 255, 0, 255}
			case x < fixtureW/2:
				c = color.RGBA{0, 0, 255, 255}
			default:
				c = color.RGBA{255, 255, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG 夹具失败: %v", err)
	}
	return buf.Bytes()
}

// synthAVIF 用本机 ffmpeg 把合成 PNG 编成真 AVIF（libaom-av1 + avif muxer）。
func synthAVIF(t *testing.T) []byte {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机无 ffmpeg：跳过图像转码用例（与媒体链路同一处理）")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	dst := filepath.Join(dir, "out.avif")
	if err := os.WriteFile(src, synthPNG(t), 0o600); err != nil {
		t.Fatalf("写 PNG 夹具失败: %v", err)
	}
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-i", src,
		"-frames:v", "1", "-c:v", "libaom-av1", "-still-picture", "1", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("本机 ffmpeg 编不出 AVIF（%v / %s）：跳过", err, strings.TrimSpace(string(out)))
	}
	b, err := os.ReadFile(dst)
	if err != nil || len(b) == 0 {
		t.Skipf("ffmpeg 未产出 AVIF 文件: %v", err)
	}
	return b
}

// TestIsAVIFImage 钉住「按内容判定、不看扩展名」的口径。
func TestIsAVIFImage(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"major_brand=avif", ftypBox("avif"), true},
		{"major_brand=avis（序列 AVIF）", ftypBox("avis"), true},
		{"compatible_brands 含 avif", ftypBox("mif1", "miaf", "avif"), true},
		{"major_brand=mif1 无 avif（HEIF，不是 AVIF）", ftypBox("mif1", "heic"), false},
		{"PNG", synthPNG(t), false},
		{"空数据", nil, false},
		{"短数据", []byte("ftyp"), false},
		{"非 ftyp 容器", []byte("RIFF....WEBPVP8 "), false},
	}
	for _, c := range cases {
		if got := isAVIFImage(c.data); got != c.want {
			t.Errorf("%s: isAVIFImage = %v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestImageTranscoderWhitelist 钉住放行白名单：只放行浏览器支持的 AVIF。
//
// ★ 这是「对齐浏览器」的核心判据：ffmpeg 什么都能解，若把它的能力全放出去，
// 等级表会出现引擎 L3 / 浏览器 L0 的非对齐项（TIFF 即典型）。
func TestImageTranscoderWhitelist(t *testing.T) {
	tr := NewImageTranscoder("")
	pngBytes := synthPNG(t)

	if _, ok := tr.Transcode(pngBytes); ok {
		t.Fatal("PNG 不该走转码路径（它不是 AVIF，且 Skia 本就认识）")
	}
	if _, ok := tr.Transcode([]byte("II*\x00tiff-ish")); ok {
		t.Fatal("TIFF 必须保持不支持（浏览器同样不支持，放行即失去对齐）")
	}
	st := tr.Stats()
	if st.Rejected != 2 {
		t.Fatalf("Rejected = %d，期望 2（两次都在白名单外被拒）", st.Rejected)
	}
	if st.Transcodes != 0 {
		t.Fatalf("Transcodes = %d，期望 0（被拒的输入不该起 ffmpeg）", st.Transcodes)
	}
	// 未注册到引擎时，引擎侧的能力查询应为 false。
	graphics.SetImageTranscoder(nil)
	if graphics.ImageTranscoderRegistered() {
		t.Fatal("清除后 ImageTranscoderRegistered 应为 false")
	}
}

// TestImageTranscoderAVIFToPNG 走真 ffmpeg：AVIF → PNG，且输出能被 Skia 解码。
func TestImageTranscoderAVIFToPNG(t *testing.T) {
	avif := synthAVIF(t)
	if !isAVIFImage(avif) {
		t.Fatal("ffmpeg 产出的 AVIF 未被 isAVIFImage 识别（判定口径与真实容器不符）")
	}
	tr := NewImageTranscoder("")
	pngBytes, ok := tr.Transcode(avif)
	if !ok {
		t.Fatal("真 ffmpeg 转码失败（本机 ffmpeg 能解 AVIF，见文件头实测）")
	}
	if len(pngBytes) == 0 {
		t.Fatal("转码输出为空")
	}
	img, err := skia.DecodeImage(pngBytes)
	if err != nil || img == nil {
		t.Fatalf("转码输出不是引擎能解码的图像: %v", err)
	}
	if img.Width() != fixtureW || img.Height() != fixtureH {
		t.Fatalf("转码尺寸 %dx%d，期望 %dx%d", img.Width(), img.Height(), fixtureW, fixtureH)
	}
	if st := tr.Stats(); st.Transcodes != 1 || st.Failures != 0 {
		t.Fatalf("统计异常: Transcodes=%d Failures=%d（期望 1 / 0）", st.Transcodes, st.Failures)
	}
}

// TestImageTranscoderCachesResult 钉住「同一份内容只起一次 ffmpeg」——绘制路径
// 会反复询问，没缓存就会每次绘制都起一个进程。
func TestImageTranscoderCachesResult(t *testing.T) {
	avif := synthAVIF(t)
	tr := NewImageTranscoder("")
	first, ok := tr.Transcode(avif)
	if !ok {
		t.Fatal("首次转码失败")
	}
	second, ok := tr.Transcode(avif)
	if !ok {
		t.Fatal("第二次转码失败（应命中缓存）")
	}
	if !bytes.Equal(first, second) {
		t.Fatal("两次转码结果不一致（缓存返回的不是同一份字节）")
	}
	if st := tr.Stats(); st.Transcodes != 1 || st.Hits != 1 {
		t.Fatalf("统计异常: Transcodes=%d Hits=%d（期望 1 / 1）", st.Transcodes, st.Hits)
	}
}

// TestImageTranscoderWithoutFFmpeg 钉住「没有 ffmpeg 就如实失败」：不假装成功，
// 且失败会被负缓存（同一份坏输入不反复起进程）。
func TestImageTranscoderWithoutFFmpeg(t *testing.T) {
	avif := synthAVIF(t)
	tr := NewImageTranscoder(filepath.Join(t.TempDir(), "no-such-ffmpeg.exe"))
	if tr.Available() {
		t.Fatal("指向不存在的 ffmpeg 时 Available 应为 false")
	}
	if _, ok := tr.Transcode(avif); ok {
		t.Fatal("没有 ffmpeg 时必须如实失败")
	}
	if _, ok := tr.Transcode(avif); ok {
		t.Fatal("第二次仍必须失败")
	}
	st := tr.Stats()
	if st.Failures != 1 {
		t.Fatalf("Failures = %d，期望 1（第二次命中负缓存，不再计失败）", st.Failures)
	}
}
