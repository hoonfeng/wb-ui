package app

// 宿主 WebAudio 实时输出的测试：用**假设备**注入，把「float32 → s16le 转换、惰性
// 打开、无设备降级、队列溢出丢最旧、flush/Stop」变成可精确断言的事。
//
// 真设备（waveOut）的端到端取证在探针/手工验证里（有声卡才有意义）；本文件不依赖
// 本机是否有声卡 —— 这正是把设备打开点做成可注入字段的原因。

import (
	"sync"
	"testing"
	"time"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/rendering"
)

// fakeDevice 实现 audioDevice，记录写入的 PCM。
type fakeDevice struct {
	mu       sync.Mutex
	chunks   [][]byte
	frames   int64
	flushes  int
	closed   bool
	paused   bool
	block    chan struct{} // 非 nil 时 write 会阻塞，用于制造队列堆积
	writeErr error
}

func (d *fakeDevice) write(data []byte) error {
	if d.block != nil {
		<-d.block
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.writeErr != nil {
		return d.writeErr
	}
	d.chunks = append(d.chunks, append([]byte(nil), data...))
	d.frames += int64(len(data) / 4) // s16le 立体声：4 字节/采样帧
	return nil
}

func (d *fakeDevice) playedFrames() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.frames
}

func (d *fakeDevice) pause() error  { d.mu.Lock(); d.paused = true; d.mu.Unlock(); return nil }
func (d *fakeDevice) resume() error { d.mu.Lock(); d.paused = false; d.mu.Unlock(); return nil }

func (d *fakeDevice) flush() error {
	d.mu.Lock()
	d.flushes++
	d.chunks = nil
	d.mu.Unlock()
	return nil
}

func (d *fakeDevice) close() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	return nil
}

func (d *fakeDevice) bytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []byte
	for _, c := range d.chunks {
		out = append(out, c...)
	}
	return out
}

func (d *fakeDevice) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// installFakeWebAudioOutput 用假设备装配一个干净的实时输出（测试结束自动收尾）。
func installFakeWebAudioOutput(t *testing.T, dev *fakeDevice, ok bool) *WebAudioOutput {
	t.Helper()
	StopWebAudioOutput()
	o := InstallWebAudioOutput()
	o.openDevice = func(rendering.AudioFormat) (audioDevice, bool) {
		if !ok {
			return nil, false
		}
		return dev, true
	}
	t.Cleanup(StopWebAudioOutput)
	return o
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

// TC-A3-3-H1：float32 → s16le 转换与 little-endian 字节序。
func TestWebAudioOutputConvertsToS16LE(t *testing.T) {
	dev := &fakeDevice{}
	o := installFakeWebAudioOutput(t, dev, true)
	if bindings.WGAudioSink == nil {
		t.Fatal("装配后 bindings.WGAudioSink 仍为 nil")
	}
	// 一个采样帧：左 = 0（静音）、右 = 1（满刻度）→ 3 帧覆盖 0 / +1 / -1
	o.sink([]float32{0, 1, -1, 0, 0.5, -0.5}, 48000)
	waitFor(t, func() bool { return len(dev.bytes()) == 12 }, "设备收到 3 帧 PCM")

	got := dev.bytes()
	want := []byte{
		0x00, 0x00, // 0 → 0
		0xFF, 0x7F, // 32767 → 0x7FFF
		0x01, 0x80, // -32767 → 0x8001
		0x00, 0x00,
		0xFF, 0x3F, // 16383 → 0x3FFF
		0x01, 0xC0, // -16383 → 0xC001
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("字节 %d = 0x%02X（期望 0x%02X）；整串 = % X", i, got[i], want[i], got)
		}
	}
}

// TC-A3-3-H2：设备惰性打开（不发声不占设备）与帧数统计。
func TestWebAudioOutputLazyOpenAndStats(t *testing.T) {
	dev := &fakeDevice{}
	o := installFakeWebAudioOutput(t, dev, true)

	o.mu.Lock()
	opened := o.opened
	o.mu.Unlock()
	if opened {
		t.Fatal("装配即打开了设备（应为惰性：只有收到样本才打开）")
	}
	// 未发声时统计应为「无可用输出」
	if _, _, avail := WebAudioOutputStats(); avail {
		t.Fatal("未发声时 WebAudioOutputStats 报告有可用输出")
	}

	o.sink([]float32{0.25, -0.25}, 48000)
	waitFor(t, func() bool { return dev.playedFrames() == 1 }, "设备播到 1 帧")
	written, dropped, avail := WebAudioOutputStats()
	if !avail {
		t.Fatal("发声后应报告有可用输出")
	}
	if written != 1 || dropped != 0 {
		t.Fatalf("written=%d dropped=%d（期望 1/0）", written, dropped)
	}
}

// TC-A3-3-H3：没有可用设备时静默降级（丢弃并计数，不报错、不 panic）。
func TestWebAudioOutputUnavailableDegrades(t *testing.T) {
	dev := &fakeDevice{}
	o := installFakeWebAudioOutput(t, dev, false)
	o.sink([]float32{1, 1, 1, 1}, 48000) // 2 帧
	written, dropped, avail := WebAudioOutputStats()
	if avail {
		t.Fatal("无设备时不应报告可用输出")
	}
	if written != 0 || dropped != 2 {
		t.Fatalf("written=%d dropped=%d（期望 0/2：如实丢弃并计数）", written, dropped)
	}
	// 再次调用不得 panic / 死锁
	o.sink([]float32{1, 1}, 48000)
	if _, dropped, _ := WebAudioOutputStats(); dropped != 3 {
		t.Fatalf("累计丢弃 = %d（期望 3）", dropped)
	}
}

// TC-A3-3-H4：设备被卡住（write 阻塞）时，队列溢出丢**最旧**的块，且 sink 不阻塞。
func TestWebAudioOutputQueueOverflowDropsOldest(t *testing.T) {
	dev := &fakeDevice{block: make(chan struct{})}
	o := installFakeWebAudioOutput(t, dev, true)

	// 每块 128 帧 × 2 声道 = 1024 字节；上限 400ms@48k = 153600 字节 ≈ 150 块。
	block := make([]float32, 128*2)
	for i := range block {
		block[i] = 0.5
	}
	for i := 0; i < 300; i++ {
		o.sink(block, 48000)
	}
	o.mu.Lock()
	queued, dropped := o.queued, o.dropped
	o.mu.Unlock()
	if dropped == 0 {
		t.Fatalf("300 块（≈800ms）全塞进 400ms 队列却未丢帧：queued=%d", queued)
	}
	if queued > 48000*4*webAudioQueueMs/1000 {
		t.Fatalf("排队字节 %d 超过上限", queued)
	}
	// sink 必须是非阻塞的：上面的循环能在毫秒级跑完（write 一直阻塞着）
	close(dev.block)
	waitFor(t, func() bool { return dev.playedFrames() > 0 }, "放行后设备开始消费")
}

// TC-A3-3-H5：flush 清空排队与设备缓冲；Stop 关闭设备并解绑注入。
func TestWebAudioOutputFlushAndStop(t *testing.T) {
	dev := &fakeDevice{block: make(chan struct{})}
	o := installFakeWebAudioOutput(t, dev, true)
	o.sink([]float32{0.1, 0.1, 0.1, 0.1}, 48000)

	o.flush()
	o.mu.Lock()
	queued := o.queued
	o.mu.Unlock()
	if queued != 0 {
		t.Fatalf("flush 后仍有 %d 字节排队", queued)
	}
	if dev.flushes == 0 {
		t.Fatal("flush 未调用设备 flush")
	}

	close(dev.block)
	o.Stop()
	if !dev.isClosed() {
		t.Fatal("Stop 未关闭设备")
	}
	if bindings.WGAudioSink != nil || bindings.WGSinkStop != nil {
		t.Fatal("Stop 后应解绑 WGAudioSink/WGSinkStop 注入")
	}
	o.Stop() // 幂等
}

// TC-A3-3-H6：上下文采样率与设备格式不一致时不重采样（如实丢弃并计数）。
func TestWebAudioOutputSampleRateMismatchDrops(t *testing.T) {
	dev := &fakeDevice{}
	o := installFakeWebAudioOutput(t, dev, true)
	o.sink([]float32{0.5, 0.5}, 48000) // 打开设备（48k）
	waitFor(t, func() bool { return dev.playedFrames() == 1 }, "48k 帧写入")
	o.sink([]float32{0.5, 0.5, 0.5, 0.5}, 44100) // 另一档采样率
	_, dropped, _ := WebAudioOutputStats()
	if dropped != 2 {
		t.Fatalf("采样率不一致时应丢弃 2 帧，实际 %d", dropped)
	}
}
