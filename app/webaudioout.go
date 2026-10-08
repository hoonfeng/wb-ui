package app

// WebAudio 实时输出（A3-3 的宿主侧）：把 AudioContext 的 destination 渲染出的
// 交错 float32 PCM 送到内置输出设备（Windows 上是 waveOut，见 audioout_windows.go）。
//
// 三条设计纪律（都是「不假装」的落地）：
//
//	① **惰性打开设备**：装配 sink 本身不碰设备，只有真的收到样本才打开。页面里存在
//	   AudioContext 但没发声时，宿主不占用声卡（探针 / CI / 无头环境也更干净）。
//	② **不阻塞渲染线程**：bindings.WGAudioSink 在事件循环线程上被调用（音频渲染发生
//	   在定时器回调里），而设备写入按设计是阻塞的（audioDevice.write 就是节流点）。
//	   因此 sink 只做 float32→s16le 转换 + 入队，写入交给专用 goroutine；队列满时丢弃
//	   **最旧**的一块 —— 宁可丢一小段已经过时的声音，也不让主循环卡顿。
//	③ **失败即静默降级**：没有可用设备 / 打开失败时，样本被丢弃并计数，不报错、不假装
//	   出声 —— 与媒体链路「无内置输出设备」的降级列同一纪律。
//
// 已知限制（如实记录，不假装支持）：多个实时 AudioContext 同时发声时，样本按到达
// 顺序**串行**写入同一设备（未做混音），因此只有「单实时上下文」这一场景在时长上
// 严格正确。引擎的离线渲染（OfflineAudioContext）不受此限。

import (
	"encoding/binary"
	"log"
	"math"
	"sync"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/rendering"
)

const (
	// webAudioQueueMs 是排队上限（毫秒）。与媒体链路的设备缓冲同量级：足够吸收一次
	// GC/重排造成的停顿，又不会让声音明显滞后（滞后会被用户听成「画面与声音不同步」）。
	webAudioQueueMs = 400
	// webAudioChannels 是实时音频图的输出声道数（内核固定 2，见 bindings 的 wgChannels）。
	webAudioChannels = 2
)

// WebAudioOutput 是实时音频图的宿主输出端。
type WebAudioOutput struct {
	mu   sync.Mutex
	cond *sync.Cond

	// openDevice 可替换（测试注入假设备；生产用 openAudioDevice）。
	openDevice func(rendering.AudioFormat) (audioDevice, bool)

	dev     audioDevice
	rate    int
	opened  bool // 是否已经尝试过打开（只试一次）
	unavail bool // 打开失败：后续样本直接丢弃
	closed  bool

	queue  [][]byte
	queued int

	written int64 // 已交给设备的采样帧数
	dropped int64 // 被丢弃的采样帧数（设备不可用或队列溢出）
}

// InstallWebAudioOutput 装配实时输出 sink（幂等，**不打开设备**）。
func InstallWebAudioOutput() *WebAudioOutput {
	webAudioOutputMu.Lock()
	defer webAudioOutputMu.Unlock()
	if webAudioOutput != nil {
		return webAudioOutput
	}
	o := &WebAudioOutput{openDevice: openAudioDevice}
	o.cond = sync.NewCond(&o.mu)
	bindings.WGAudioSink = o.sink
	bindings.WGSinkStop = o.flush
	webAudioOutput = o
	return o
}

// StopWebAudioOutput 停止实时输出并释放设备（幂等；进程退出时无需显式调用，但探针
// 与测试需要它来干净收尾）。
func StopWebAudioOutput() {
	webAudioOutputMu.Lock()
	o := webAudioOutput
	webAudioOutput = nil
	webAudioOutputMu.Unlock()
	if o != nil {
		o.Stop()
	}
}

// WebAudioOutputStats 报告实时输出的帧数与可用性（验证报告用）。
// available=false 表示本机没有可用的内置输出设备（或尚未发声而未尝试打开）。
func WebAudioOutputStats() (written, dropped int64, available bool) {
	webAudioOutputMu.Lock()
	o := webAudioOutput
	webAudioOutputMu.Unlock()
	if o == nil {
		return 0, 0, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.written, o.dropped, o.dev != nil && !o.unavail
}

var (
	webAudioOutputMu sync.Mutex
	webAudioOutput   *WebAudioOutput
)

// sink 是 bindings.WGAudioSink 的实现：事件循环线程调用，必须尽快返回。
func (o *WebAudioOutput) sink(interleaved []float32, sampleRate int) {
	if o == nil || len(interleaved) == 0 {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	if !o.opened {
		o.openLocked(sampleRate)
	}
	if o.unavail || o.dev == nil || o.rate != sampleRate {
		// 设备不可用，或上下文采样率与设备格式不一致（**不重采样**：只按设备支持的
		// 那一档输出，其余如实丢弃并计数 —— 编造重采样会让「听到的速度」与真实不符）。
		o.dropped += int64(len(interleaved) / webAudioChannels)
		o.mu.Unlock()
		return
	}
	buf := make([]byte, len(interleaved)*2)
	for i, v := range interleaved {
		binary.LittleEndian.PutUint16(buf[i*2:], s16Sample(v))
	}
	o.queue = append(o.queue, buf)
	o.queued += len(buf)
	limit := o.rate * webAudioChannels * 2 * webAudioQueueMs / 1000
	for o.queued > limit && len(o.queue) > 1 {
		o.queued -= len(o.queue[0])
		o.dropped += int64(len(o.queue[0]) / (webAudioChannels * 2))
		o.queue = o.queue[1:]
	}
	o.cond.Signal()
	o.mu.Unlock()
}

// openLocked 惰性打开输出设备（只在首次收到样本时；只试一次）。
func (o *WebAudioOutput) openLocked(sampleRate int) {
	o.opened = true
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	open := o.openDevice
	if open == nil {
		open = openAudioDevice
	}
	dev, ok := open(rendering.AudioFormat{SampleRate: sampleRate, Channels: webAudioChannels})
	if !ok {
		o.unavail = true
		log.Printf("[webaudio] 本机没有可用的内置输出设备：AudioContext 的实时输出被丢弃" +
			"（离线渲染、解码与 <audio>/<video> 链路不受影响）")
		return
	}
	o.dev = dev
	o.rate = sampleRate
	go o.pump()
}

// pump 是唯一的设备写入者：从队列取块写入（阻塞即节流）。
func (o *WebAudioOutput) pump() {
	for {
		o.mu.Lock()
		for len(o.queue) == 0 && !o.closed {
			o.cond.Wait()
		}
		if len(o.queue) == 0 {
			// closed 且队列已空 ⇒ 收工；否则（被 Signal 唤醒但队列为空）继续等。
			done := o.closed
			o.mu.Unlock()
			if done {
				return
			}
			continue
		}
		chunk := o.queue[0]
		o.queue = o.queue[1:]
		o.queued -= len(chunk)
		o.mu.Unlock()

		if err := o.dev.write(chunk); err != nil {
			o.mu.Lock()
			o.unavail = true
			o.dropped += int64(len(chunk) / (webAudioChannels * 2))
			o.queue = nil
			o.queued = 0
			o.mu.Unlock()
			log.Printf("[webaudio] 输出设备写入失败，实时输出停止：%v", err)
			return
		}
		o.mu.Lock()
		o.written += int64(len(chunk) / (webAudioChannels * 2))
		o.mu.Unlock()
	}
}

// flush 实现 bindings.WGSinkStop：上下文关闭时丢弃已排队样本并清设备缓冲
// （否则上下文 close() 之后还会把残余的几百毫秒播完，听感上像「关不掉」）。
func (o *WebAudioOutput) flush() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.queue = nil
	o.queued = 0
	dev := o.dev
	o.mu.Unlock()
	if dev != nil {
		_ = dev.flush()
	}
}

// Stop 停止实时输出并释放设备（幂等）。
func (o *WebAudioOutput) Stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	dev := o.dev
	o.queue = nil
	o.queued = 0
	o.cond.Broadcast()
	o.mu.Unlock()

	if dev != nil {
		_ = dev.flush()
		_ = dev.close()
	}
	if bindings.WGAudioSink != nil {
		bindings.WGAudioSink = nil
	}
	if bindings.WGSinkStop != nil {
		bindings.WGSinkStop = nil
	}
}

// s16Sample 把 [-1,1] 的 float32 样本转成 16 位 PCM 样本值（小端写入由调用方用
// encoding/binary 完成，格式即 audioDevice.write 约定的 s16le）。
//
// 设备格式固定 s16le（audioDevice.write 的约定），因此这里是格式转换的唯一位置。
// NaN/±Inf 按 0 处理：钳位逻辑对 NaN 不成立（比较全为 false），会让 int32(NaN) 落到
// 实现定义值（可能是刺耳的爆音）。
func s16Sample(v float32) uint16 {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	s := int32(f * 32767)
	if s > 32767 {
		s = 32767
	} else if s < -32768 {
		s = -32768
	}
	return uint16(int16(s))
}
