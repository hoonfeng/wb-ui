package app

// 动图解码（实现路径主线 A4）：宿主用 goskia 的 SkCodec 解出多帧，注入渲染层的
// 动图通道；引擎按帧时长选帧（engine/rendering/imageanimation.go）。
//
// 为什么解码在宿主：与 A1 的视频帧同一条理由——渲染层不绑定具体解码库，宿主可以换
// 实现（SkCodec / 平台解码器）。goskia 的 codec 绑定是 2026-10-07 为这项能力加的
// （`skia.NewCodec` / `DecodeFrames`，见 goskia 的 skia/codec.go）。

import (
	"github.com/hoonfeng/goskia/skia"

	"wb-ui/engine/rendering"
)

// InstallAnimatedImageSource 把 SkCodec 多帧解码接成渲染层的动图源，返回注销函数。
//
// 判定为动图的条件是**帧数 > 1**：
//   - Skia 对不提供帧计数的容器（静态 PNG/JPEG）返回 FrameCount() == 0 → 交给单帧路径；
//   - 单帧 GIF/WebP 也走单帧路径（解多帧只多花时间，画面一样）。
//
// 帧位图由 `DecodedImage` 直接持有（NewDecodedImageFromSkia），不经过 PNG 中转。
func InstallAnimatedImageSource() func() {
	rendering.SetAnimatedImageSource(func(url string, data []byte) ([]rendering.AnimatedFrame, int, bool) {
		codec, err := skia.NewCodec(data)
		if err != nil {
			return nil, 0, false // 不是 Skia 能解的图像：让单帧路径去试
		}
		defer codec.Release()
		if codec.FrameCount() <= 1 {
			return nil, 0, false
		}
		frames, err := codec.DecodeFrames()
		if err != nil || len(frames) <= 1 {
			return nil, 0, false
		}
		loops := codec.RepetitionCount()
		out := make([]rendering.AnimatedFrame, 0, len(frames))
		for _, f := range frames {
			img := rendering.NewDecodedImageFromSkia(f.Image)
			if img == nil {
				return nil, 0, false
			}
			out = append(out, rendering.AnimatedFrame{Image: img, DurationMS: f.DurationMS})
		}
		return out, loops, true
	})
	return func() { rendering.SetAnimatedImageSource(nil) }
}

// PinAnimatedImagePhase 把动图取帧相位钉在 phaseMS 毫秒（取证用：让截图可复现），
// 见 rendering.SetAnimatedImagePhase 的推导。默认不调用 = 动画按真实时间推进。
func PinAnimatedImagePhase(phaseMS int) { rendering.SetAnimatedImagePhase(phaseMS) }

// UnpinAnimatedImagePhase 解除动图取帧相位锁定，恢复按真实经过时间选帧。
func UnpinAnimatedImagePhase() { rendering.ClearAnimatedImagePhase() }
