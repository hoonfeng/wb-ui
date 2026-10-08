// Translation of: Source/WebCore/platform/graphics/skia/* (Skia bridge)
//                  Source/WebCore/platform/graphics/GraphicsContextSkia.cpp
// Completeness: 80%
// Simplifications:
//   - The Skia bridge is now fully wired via goskia cgo bindings.
//   - Canvas creation, drawing, text rendering, and pixel readback all go
//     through Skia's real rasterizer (see canvas.go).
//   - No GPU/compositor surface; only raster (CPU) surfaces are used.

package graphics

import (
	"sync"

	"github.com/hoonfeng/goskia/skia"
)

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
//
// ★ 宿主转码兜底（SetImageTranscoder）：Skia 的二进制里**没有**编入 AVIF
// 解码（实测 goskia 的 libSkiaSharp 对 `quad.avif` 返回 unsupported），而
// 浏览器普遍支持 AVIF —— 这类「浏览器支持、Skia 不认」的格式由宿主转成
// 引擎认得的格式后再解一次。未注册转码器时行为与从前逐字节一致。
func DecodeImage(data []byte) *SkiaImage {
	if len(data) == 0 {
		return nil
	}
	if img, err := skia.DecodeImage(data); err == nil {
		return img
	}
	return decodeViaHostTranscoder(data)
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

// ─── 宿主图像转码器（“浏览器支持、Skia 未编入”的格式的兜底）────────────
//
// 为什么是注入而不是内置解码器：与本仓库一贯的分工一致（A3 音频用宿主
// ffmpeg 解 PCM、A1/A2 视频用宿主抽帧、A3-3 的 decodeAudioData 由宿主
// 解码）——引擎不背编解码器与许可，宿主提供能力。AVIF 因此与「媒体真实
// 播放」走同一条路线 (a)。
//
// 契约（三条，缺一不可）：
//  1. **只在 Skia 解码失败后调用**：成功路径零开销，常见格式不受影响；
//  2. 返回的字节必须是引擎解码器**认得的**格式（宿主负责转成 PNG）；
//  3. **由宿主决定放行哪些格式**，且只放行**浏览器支持**的那些（AVIF）。
//     浏览器同样不支持的格式（TIFF）必须继续失败——等级表把 TIFF 记为
//     L0 才是「对齐浏览器」；放行它会变成「引擎比浏览器更强」的非对齐项。
var (
	imageTranscoderMu sync.RWMutex
	imageTranscoder   func(data []byte) ([]byte, bool)
)

// SetImageTranscoder 注册（传 nil 清除）宿主的图像转码器。线程安全，可在运行中切换
// （转码结果由宿主自己缓存，引擎侧不做二次缓存）。
func SetImageTranscoder(f func(data []byte) ([]byte, bool)) {
	imageTranscoderMu.Lock()
	imageTranscoder = f
	imageTranscoderMu.Unlock()
}

// ImageTranscoderRegistered 报告是否已注册转码器（自检与诊断用）。
func ImageTranscoderRegistered() bool {
	imageTranscoderMu.RLock()
	defer imageTranscoderMu.RUnlock()
	return imageTranscoder != nil
}

// decodeViaHostTranscoder 让宿主转码后交给 Skia 再解一次（未注册/转码失败返回 nil）。
func decodeViaHostTranscoder(data []byte) *SkiaImage {
	imageTranscoderMu.RLock()
	f := imageTranscoder
	imageTranscoderMu.RUnlock()
	if f == nil {
		return nil
	}
	conv, ok := f(data)
	if !ok || len(conv) == 0 {
		return nil
	}
	img, err := skia.DecodeImage(conv)
	if err != nil {
		return nil
	}
	return img
}
