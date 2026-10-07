package app

// 音频输出设备抽象（主线 A3-1 的「输出后端」）。
//
// 为什么把设备接口放在 app 而不是引擎：设备 API 是**平台**的事（Windows 是
// waveOut/WASAPI，其它平台各有各的），而引擎要跨平台。引擎侧只认识
// rendering.AudioSession（见 engine/rendering/audioframe.go）——「已经播到哪里」
// 这句话的答案由宿主给出，设备怎么把样本送到声卡是宿主的细节。
//
// 平台实现：audioout_windows.go（waveOut）。其它平台没有实现文件里的
// openAudioDevice 返回 ok=false，会话随即进入降级模式：PCM 仍然解出来、仍然交给
// tap（宿主自备输出/取证），只是没有内置输出设备。文档的支持矩阵与报告都按
// 「有内置输出 / 无内置输出（宿主自备）」两列记录。

import (
	"wb-ui/engine/rendering"
)

// audioDevice 是 PCM 输出设备（已打开状态）。
//
// 并发约定：write/playedFrames/pause/resume/flush/close 会被**推送 goroutine**
// 与**事件循环线程**同时调用（play/pause/seek 来自 JS 线程），实现必须自己加锁。
type audioDevice interface {
	// write 提交一块 PCM（s16le 交错，格式与打开时一致）。**阻塞**直到设备腾出
	// 缓冲——这正是推送循环的节流点：设备按实时速率消费，解码因此不会跑飞。
	write(data []byte) error
	// playedFrames 返回设备**已播放**的采样帧数（不含尚在缓冲里的）。这是
	// 「音频为主时钟」的位置来源。
	playedFrames() int64
	// pause 挂起输出（保留已排队的数据）；resume 继续。
	pause() error
	resume() error
	// flush 丢弃已排队的数据并把播放位置归零（seek 用）。
	flush() error
	// close 停止输出并释放设备（幂等）。
	close() error
}

// audioDeviceBuffers 是设备的缓冲深度（个数)×audioDeviceBufferMs。
//
// 4 × 100ms 是 waveOut 上的常见折中：太小会频繁 underrun（解码器一卡就爆音），
// 太大则 pause/seek 的响应变钝（flush 后仍有一段陈旧音频在设备里）。判据 A/B
// 与 TC-M-602 的定点判据都在百毫秒量级上成立，100ms 粒度足够。
const (
	audioDeviceBuffers  = 4
	audioDeviceBufferMs = 100
)

// openAudioDevice 打开本平台的默认输出设备（格式固定 s16le）。ok=false 表示
// 本平台没有实现、或本机没有任何可用输出设备——调用方进入降级模式（不是错误）。
func openAudioDevice(format rendering.AudioFormat) (audioDevice, bool) {
	if !format.Valid() {
		return nil, false
	}
	return openPlatformAudioDevice(format)
}

// AudioOutputAvailable 报告本平台、本机是否有可用的内置输出设备（支持矩阵与
// 验证报告用）。它会真的打开再关掉一次设备：这是唯一可靠的判断方式（枚举设备
// 数不为 0 不等于默认设备能被打开——被独占、格式不支持都会失败）。
func AudioOutputAvailable() bool {
	dev, ok := openAudioDevice(rendering.AudioFormat{
		SampleRate: audioPCMSampleRate,
		Channels:   audioPCMChannels,
	})
	if !ok {
		return false
	}
	_ = dev.close()
	return true
}
