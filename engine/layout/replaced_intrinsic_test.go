// 固有尺寸并集探测的单测（U3 / 缺陷 D4）：
// 绘制走 Skia（PNG/JPEG/GIF/WebP/BMP/ICO），而布局层的固有尺寸此前只走 Go
// 标准库的 image.DecodeConfig（PNG/JPEG/GIF）——WebP/BMP/ICO 因此「画得
// 出来但量不出固有尺寸」。修复后布局层先用 DecodeConfig（只读文件头），
// 失败再由 Skia 兜底探测，使两套 codec 集合取并集。
//
// 这里用**自行构造**的 BMP / ICO 字节（Go 标准库没有这两种解码器，能通过
// 就只可能是 Skia 兜底生效）；WebP 需要真实编码器，其固有尺寸由媒体验证
// 工装（cmd/psai -media 的几何 JSON）用 PIL 生成的样本覆盖。

package layout

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func putU32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

// encodeBMP 生成一张 24bit 未压缩 BMP（全黑像素）。
func encodeBMP(w, h int) []byte {
	rowSize := (w*3 + 3) &^ 3
	pixSize := rowSize * h
	buf := make([]byte, 54+pixSize)
	buf[0], buf[1] = 'B', 'M'
	putU32(buf[2:], uint32(54+pixSize)) // 文件大小
	putU32(buf[10:], 54)                // 像素数据偏移
	putU32(buf[14:], 40)                // BITMAPINFOHEADER 大小
	putU32(buf[18:], uint32(w))
	putU32(buf[22:], uint32(h))
	buf[26], buf[27] = 1, 0  // biPlanes
	buf[28], buf[29] = 24, 0 // biBitCount
	putU32(buf[34:], uint32(pixSize))
	return buf
}

// encodeICO 把一张 PNG 包成单图 ICO（现代 ICO 的标准做法：内嵌 PNG）。
func encodeICO(pngData []byte, w, h int) []byte {
	hdr := make([]byte, 6+16)
	hdr[2], hdr[3] = 1, 0 // 类型：1 = icon
	hdr[4], hdr[5] = 1, 0 // 图像数：1
	hdr[6] = byte(w)      // 宽（0 表示 256）
	hdr[7] = byte(h)
	hdr[10], hdr[11] = 1, 0  // 平面数
	hdr[12], hdr[13] = 32, 0 // 位深
	putU32(hdr[14:], uint32(len(pngData))) // 数据大小
	putU32(hdr[18:], uint32(len(hdr)))     // 数据偏移
	return append(hdr, pngData...)
}

func encodeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = 0x40
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	_ = color.RGBA{}
	return buf.Bytes()
}

func dataURIOf(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func TestRasterIntrinsicUnionCodecSets(t *testing.T) {
	const (
		wantW = 5
		wantH = 7
	)
	pngData := encodeTestPNG(t, wantW, wantH)

	cases := []struct {
		name string
		url  string
		w, h float64
	}{
		// 对照组：PNG 走 Go DecodeConfig（最轻路径）。
		{"png(go)", dataURIOf("image/png", pngData), wantW, wantH},
		// BMP / ICO：Go 标准库没有解码器 → 只有 Skia 兜底能给出尺寸。
		{"bmp(skia)", dataURIOf("image/bmp", encodeBMP(wantW, wantH)), wantW, wantH},
		{"ico(skia)", dataURIOf("image/x-icon", encodeICO(pngData, wantW, wantH)), wantW, wantH},
	}
	for _, c := range cases {
		is := lookupIntrinsic(c.url)
		if is.w != c.w || is.h != c.h {
			t.Errorf("%s: 固有尺寸 = %gx%g，want %gx%g（并集探测失败）", c.name, is.w, is.h, c.w, c.h)
		}
		if is.ratio <= 0 {
			t.Errorf("%s: 宽高比应可用，got %g", c.name, is.ratio)
		}
	}
}

func TestRasterIntrinsicRejectsGarbage(t *testing.T) {
	// 反向：垃圾数据在两条路径上都应失败，不得因兜底而「猜」出尺寸。
	is := lookupIntrinsic(dataURIOf("image/png", []byte("not an image at all")))
	if is.w != 0 || is.h != 0 {
		t.Fatalf("垃圾数据不应给出固有尺寸，got %gx%g", is.w, is.h)
	}
}
