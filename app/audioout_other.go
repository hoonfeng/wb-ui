//go:build !windows

package app

// 非 Windows 平台的内置输出后端（主线 A3-1）：**刻意没有实现**。
//
// 范围决策（经监督者拍板）：A3-1 只为 Windows 提供内置输出（waveOut），其余平台
// 走「PCM 回调 + 宿主自备输出」的降级路径——PCM 一样解出来（ffmpeg 在任何平台都
// 有）、一样按实时速率推进时钟，只是不替宿主打开设备：宿主可以用 ALSA/CoreAudio/
// PulseAudio 自己接，也可以只用 tap 做取证。这样既不把三套平台 API 拉进本轮，
// 也不让非 Windows 平台「完全不能播放」。
//
// 支持矩阵（报告与文档同步记录）：
//
//	平台      PCM 解码   播放时钟    内置输出        降级行为
//	Windows   ✅（ffmpeg）✅（设备位置）✅（waveOut）  —
//	其它      ✅（ffmpeg）✅（挂钟+已交付帧）无        PCM → tap（宿主自备输出）

import (
	"wb-ui/engine/rendering"
)

// openPlatformAudioDevice 在本平台没有内置输出设备：返回 ok=false，会话进入
// 降级模式（见 app/mediaaudio.go 的 newAudioSession）。
func openPlatformAudioDevice(_ rendering.AudioFormat) (audioDevice, bool) {
	return nil, false
}
