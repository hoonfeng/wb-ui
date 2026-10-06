package app

// 异步抽帧执行器（A2）的契约测试：
//
//	每个请求恰好交付一次（队列满丢最旧时也必须交付失败——否则渲染层永远等它）
//	worker 惰性启动、空闲自退（不留进程级 goroutine）

import (
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// TestFramePumpDeliversExactlyOnce：提交远超队列容量的请求，断言**每个**请求
// 恰好交付一次。队列满时会丢最旧的请求，被丢的那个也必须交付（失败）——
// 渲染层按「已提交」去重，漏交会让那个时刻的帧永远缺席（画面停在上一帧）。
func TestFramePumpDeliversExactlyOnce(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过真实抽帧链路")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "pump.mp4")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=120x80:rate=10",
		"-pix_fmt", "yuv420p", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败: %v\n%s", err, out)
	}

	p := NewMediaProbe(ffmpeg)
	if p.pump == nil {
		t.Fatal("NewMediaProbe 应带异步抽帧执行器")
	}
	if _, ok := p.Resolve(clip); !ok {
		t.Fatal("元数据探测失败")
	}
	src := p.AsyncFrameSourceFor(nil)

	const n = framePumpQueue * 3
	var mu sync.Mutex
	delivered := map[float64]int{}
	done := make(chan float64, n)
	for i := 0; i < n; i++ {
		at := float64(i) * 0.04
		src(clip, at, func(_ []byte, _ bool) {
			mu.Lock()
			delivered[at]++
			mu.Unlock()
			done <- at
		})
	}
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Fatalf("等待交付超时：已交付 %d/%d", i, n)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		at := float64(i) * 0.04
		switch delivered[at] {
		case 1:
		case 0:
			t.Fatalf("请求 %.2fs 从未交付（渲染层会一直等它）", at)
		default:
			t.Fatalf("请求 %.2fs 交付了 %d 次（渲染层契约是恰好一次）", at, delivered[at])
		}
	}
}

// TestFramePumpWorkersExitWhenIdle：worker 惰性启动、空闲自退——没有请求时不留
// goroutine（测试与嵌入式用法都不会泄漏）。
func TestFramePumpWorkersExitWhenIdle(t *testing.T) {
	p := NewMediaProbe("") // 没有 ffmpeg：任务立刻交付失败，worker 照样跑一轮
	src := p.AsyncFrameSourceFor(nil)

	delivered := make(chan struct{}, 1)
	src("clip.mp4", 0, func(_ []byte, _ bool) { delivered <- struct{}{} })
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("没有 ffmpeg 时也应立刻交付失败（不阻塞渲染层）")
	}
	if got := pumpLive(p.pump); got == 0 {
		t.Fatal("提交请求后应有 worker 在跑")
	}
	deadline := time.Now().Add(framePumpIdle + 5*time.Second)
	for time.Now().Before(deadline) {
		if pumpLive(p.pump) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("worker 空闲 %v 后应自行退出，仍有 %d 个在跑", framePumpIdle, pumpLive(p.pump))
}

// TestAsyncFrameSourceRejectsNonLocalSrcs：非本地源（http/data/blob）直接同步
// 交付失败，不占用 worker——本引擎没有网络栈，问也问不到。
func TestAsyncFrameSourceRejectsNonLocalSrcs(t *testing.T) {
	p := NewMediaProbe("")
	src := p.AsyncFrameSourceFor(nil)
	for _, u := range []string{"https://example.com/x.mp4", "data:video/mp4;base64,AAAA", "blob:abc"} {
		got := make(chan bool, 1)
		src(u, 0, func(_ []byte, ok bool) { got <- ok })
		select {
		case ok := <-got:
			if ok {
				t.Fatalf("%q 不是本地文件，不应交付成功", u)
			}
		default:
			t.Fatalf("%q 应**同步**交付失败（不占用 worker）", u)
		}
	}
}

// TestAsyncFrameSourceForwardsLocalFrames：本地文件经 worker 抽到可解码的帧，
// 并把它交给渲染层（引擎侧再解码成位图）。
func TestAsyncFrameSourceForwardsLocalFrames(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过真实抽帧链路")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "one.mp4")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x3366CC:s=120x80:d=0.5:r=10", "-pix_fmt", "yuv420p", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败: %v\n%s", err, out)
	}
	p := NewMediaProbe(ffmpeg)
	src := p.AsyncFrameSourceFor(nil)

	type result struct {
		data []byte
		ok   bool
	}
	got := make(chan result, 1)
	src(clip, 0, func(data []byte, ok bool) { got <- result{data, ok} })
	select {
	case r := <-got:
		if !r.ok || len(r.data) == 0 {
			t.Fatalf("本地样本应抽到帧，got ok=%v len=%d", r.ok, len(r.data))
		}
		if img := rendering.NewDecodedImage(r.data); img == nil || !img.Loaded() {
			t.Fatal("交付的字节应能被引擎解码成位图")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("等待异步抽帧超时")
	}
}

// pumpLive 读当前活跃 worker 数（测试内省）。
func pumpLive(fp *framePump) int {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.live
}
