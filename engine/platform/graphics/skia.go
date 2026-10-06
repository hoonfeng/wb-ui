// Translation of: Source/WebCore/platform/graphics/skia/* (Skia bridge)
//                  Source/WebCore/platform/graphics/GraphicsContextSkia.cpp
// Completeness: 80%
// Simplifications:
//   - The Skia bridge is now fully wired via goskia cgo bindings.
//   - Canvas creation, drawing, text rendering, and pixel readback all go
//     through Skia's real rasterizer (see canvas.go).
//   - No GPU/compositor surface; only raster (CPU) surfaces are used.

package graphics

import "github.com/hoonfeng/goskia/skia"

// SkiaImage is an alias for the goskia Image type, exposed for use by
// the rendering package's image pipeline.
type SkiaImage = skia.Image

// TileMode aliases the goskia shader tile mode, exposed for CSS mask-image
// tiling (mask-repeat → tile mode).
type TileMode = skia.TileMode

const (
	TileModeClamp  = skia.TileModeClamp
	TileModeRepeat = skia.TileModeRepeat
	TileModeMirror = skia.TileModeMirror
	TileModeDecal  = skia.TileModeDecal
)

// DecodeImage decodes encoded image bytes (PNG, JPEG, WEBP, GIF, ...) into
// a SkiaImage using Skia's built-in decoder (which bundles libpng, libjpeg,
// libwebp, etc.). Returns nil on failure (empty data, unsupported format
// or corrupt image).
func DecodeImage(data []byte) *SkiaImage {
	if len(data) == 0 {
		return nil
	}
	img, err := skia.DecodeImage(data)
	if err != nil {
		return nil
	}
	return img
}

// DecodeSize probes the pixel dimensions of encoded image bytes with Skia's
// decoder (no bitmap is handed out to callers).
//
// 用途：**布局层**探测替换元素（`<img>`）的固有尺寸。Go 标准库的
// image.DecodeConfig 只认识 PNG/JPEG/GIF，而绘制走 Skia（PNG/JPEG/GIF/
// WebP/BMP/ICO）——WebP/BMP/ICO 因此「画得出来但量不出固有尺寸」（媒体
// 格式验证方案 §3.1 的「两套 codec 集合不一致」、缺陷 D4：`<img>` 不给
// CSS 尺寸时盒子塌成 0）。布局层先用 DecodeConfig（只读文件头、零解码
// 成本）探测，失败后再落到这里，使两套 codec 集合取并集。
//
// ★ 这是**完整解码**（Skia 没有「只读文件头」的尺寸 API），因此只应作为
// 兜底路径调用：常见格式走 DecodeConfig，代价高的解码只发生在 Go 不认识
// 而这些格式上。
func DecodeSize(data []byte) (w, h float64, ok bool) {
	img := DecodeImage(data)
	if img == nil {
		return 0, 0, false
	}
	iw, ih := img.Width(), img.Height()
	if iw <= 0 || ih <= 0 {
		return 0, 0, false
	}
	return float64(iw), float64(ih), true
}

// This file exists to document the Skia integration point and to provide
// any package-level Skia configuration helpers.
