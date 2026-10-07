// Package rendering — 音频 PCM 注入通道（主线 A3）。
//
// 为什么是这个形状（与 videoframe.go 的帧通道刻意同构，但职责不同）：
//
//	视频那边引擎要**持有像素**（painter 要在渲染线程上画出来），所以通道是
//	  「按 (url, 时刻) 取一帧字节」+ 引擎侧缓存 + 负缓存；
//	音频这边引擎**不持有样本**（没有任何绘制需求，样本的去处是声卡），引擎真正
//	  需要的只有两样：①谁来把 PCM 送出去；②「已经播到哪里了」——后者是 A3 的
//	  核心：规范要求音频为主时钟，currentTime 必须跟随输出位置，而不是自己数
//	  挂钟（挂钟与声卡时钟会漂移，1 秒的样本能差出几十毫秒，长播更明显）。
//
// 于是通道形状是「会话」而不是「取块」：宿主为一个 src 打开一次 AudioSession，
// 由宿主自己把 PCM 送去输出（Windows 是 app/audioout_windows.go 的 waveOut，
// 其余平台降级为「PCM 回调 + 宿主自备输出」），引擎只问它两件事：
// Format/TotalFrames（时长依据）与 Position（已播位置，时钟依据）。
//
// 分工与 A1/A2 一致：解码是宿主的（宿主用 ffmpeg 解 s16le），输出设备是宿主的
// （平台 API），引擎不背解码器、不碰 goskia 的跨平台构建面。
package rendering

import (
	"sync"
)

// AudioFormat 是 PCM 数据的格式。样本一律 **s16le 交错**（little-endian、
// 声道交错），这是 waveOut/WASAPI 与 ffmpeg 的 `-f s16le` 的共同最小公倍数：
// 引擎不引入重采样/位深转换（那是宿主的事），通道上只跑一种格式。
type AudioFormat struct {
	// SampleRate 是每秒采样帧数（如 48000）。
	SampleRate int
	// Channels 是声道数（1 = 单声道，2 = 立体声）。
	Channels int
}

// Valid 报告格式是否可用于输出（未打开会话 / 探测失败时为零值）。
func (f AudioFormat) Valid() bool { return f.SampleRate > 0 && f.Channels > 0 }

// BytesPerFrame 是每采样帧的字节数（s16le：每样本 2 字节）。
func (f AudioFormat) BytesPerFrame() int { return f.Channels * 2 }

// Duration 返回 frames 个采样帧的时长（秒）。
func (f AudioFormat) Duration(frames int64) float64 {
	if !f.Valid() {
		return 0
	}
	return float64(frames) / float64(f.SampleRate)
}

// AudioSession 是一次音频播放会话（宿主实现）。
//
// 生命周期：宿主在元素开始播放时创建（`Open` 语义），元素暂停/切源/卸载时由
// 绑定层调用 Pause/Seek/Close。实现约定：
//
//   - Position/Pause/Resume/Seek/Close 允许被**事件循环所在线程**调用（也就是
//     JS 线程），因此实现不能在这些方法里做长阻塞（解码与输出必须在自己的
//     goroutine 上）；
//   - Position 返回**媒体时间**（秒，绝对位置：seek 之后要包含 seek 起点），
//     它是 currentTime 的直接来源——单调不减、随输出推进；
//   - Close 之后不得再有任何交付（tap/输出），且可安全重复调用。
type AudioSession interface {
	// Format 返回会话的 PCM 格式（所有 PCM 交付都按它解释）。
	Format() AudioFormat
	// TotalFrames 返回会话的总采样帧数（0 = 未知：此时时长以元数据为准）。
	TotalFrames() int64
	// Position 返回输出侧已播放的媒体时间（秒）。这是「音频为主时钟」的依据：
	// 有真实输出设备时来自设备位置（waveOutGetPosition），降级模式下按挂钟与
	// 已交付样本数取小者模拟实时推进。
	Position() float64
	// HasOutput 报告本会话是否接上了真实输出设备。false = 降级模式（PCM 只交给
	// 宿主自备输出/取证回调），此时文档的支持矩阵标注为「无内置输出」。
	HasOutput() bool
	// Failed 报告「这个源确实没有可播的音频」（无音轨、解码器起不来、文件读
	// 不了）。引擎据此**回退挂钟时钟**并摘掉会话——不能让播放挂在一个永不推进
	// 的位置上（纯视频资源拿来开会话时最容易踩到）。注意它与 HasOutput 的区别：
	// 没有输出设备仍可能正常播放（降级模式），没有音频数据则根本无法播放。
	Failed() bool
	// Pause 暂停输出（保留已缓冲的数据，恢复后继续）。
	Pause() error
	// Resume 从暂停处恢复输出。
	Resume() error
	// Seek 把播放位置移到 seconds（秒，媒体时间）；实现须尽力清空已排队的
	// 缓冲（否则会先听到旧位置的一段音频）。
	Seek(seconds float64) error
	// Close 结束会话：停止解码与输出、释放设备。重复调用安全。
	Close() error
}

// AudioSessionSource 由宿主注入：为 src 打开一次播放会话（从 startSeconds 开始
// 播放），ok=false 表示这个源没有可播放的音频（纯视频/探测失败/没有 ffmpeg）
// ——引擎随即退回自己的挂钟时钟（不是「不能播放」，而是「没有音频时钟可用」）。
//
// src 是元素的**原始**属性值（与 MediaMetadataResolver / VideoFrameSource 收到
// 的是同一个字符串）；解析成文件路径、解出采样格式都是宿主的事。
//
// ★ 本函数在 JS 线程（事件循环）上被调用，实现必须尽快返回：解码进程的启动、
// 输出设备的打开都应在会话内部的 goroutine 上完成（见 app/mediaaudio.go）。
type AudioSessionSource func(src string, startSeconds float64) (AudioSession, bool)

// AudioChannelStats 是音频通道的计数（诊断与自检用）。
type AudioChannelStats struct {
	// Sessions 是成功打开的会话数。
	Sessions int64
	// Failed 是宿主拒绝/打开失败的次数（纯视频、无 ffmpeg、文件坏）。
	Failed int64
}

var audioChannel = struct {
	mu    sync.Mutex
	src   AudioSessionSource
	stats AudioChannelStats
}{}

// SetAudioSessionSource 注册（传 nil 清除）宿主的音频会话源。注册时丢弃计数：
// 计数描述的是「当前这套音频源」的行为（与 SetVideoFrameSource 同）。
func SetAudioSessionSource(src AudioSessionSource) {
	audioChannel.mu.Lock()
	audioChannel.src = src
	audioChannel.stats = AudioChannelStats{}
	audioChannel.mu.Unlock()
}

// AudioSessionSourceRegistered 报告是否已注册音频会话源（绑定层用它决定要不要
// 尝试打开音频会话；未注册时一切照旧走挂钟时钟，引擎独立使用不受影响）。
func AudioSessionSourceRegistered() bool {
	audioChannel.mu.Lock()
	defer audioChannel.mu.Unlock()
	return audioChannel.src != nil
}

// OpenAudioSession 请宿主为 src 打开播放会话（未注册音频源时返回 ok=false）。
// 绑定层在 play() 时调用；打开失败不改变元素的可播放性，只是时钟退回挂钟。
func OpenAudioSession(src string, startSeconds float64) (AudioSession, bool) {
	audioChannel.mu.Lock()
	src0 := audioChannel.src
	audioChannel.mu.Unlock()
	if src0 == nil {
		return nil, false
	}
	sess, ok := src0(src, startSeconds)
	audioChannel.mu.Lock()
	if ok && sess != nil {
		audioChannel.stats.Sessions++
	} else {
		audioChannel.stats.Failed++
	}
	audioChannel.mu.Unlock()
	if !ok || sess == nil {
		return nil, false
	}
	return sess, true
}

// AudioChannelStatsSnapshot 返回音频通道计数（只读，自检用）。
func AudioChannelStatsSnapshot() AudioChannelStats {
	audioChannel.mu.Lock()
	defer audioChannel.mu.Unlock()
	return audioChannel.stats
}
