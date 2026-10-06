package app

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // 抽帧结果是 PNG，测试要断言「真的能解码」
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"wb-ui/engine/js/bindings"
)

// 以下两段是 `ffmpeg -hide_banner -i <file>` 的**真实 stderr**（2026-10-06 本机
// ffmpeg 抓取，见 docs/implementation-path.md 主线 A0）：把解析逻辑钉在真实输出
// 格式上，而不是我凭空写的理想格式。
const ffmpegProbeSampleMP4 = `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'gen-video-h264.mp4':
  Metadata:
    major_brand     : isom
    minor_version   : 512
    compatible_brands: isomiso2avc1mp41
    encoder         : Lavf60.4.100
  Duration: 00:00:01.00, start: 0.000000, bitrate: 37 kb/s
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p(progressive), 120x80 [SAR 1:1 DAR 3:2], 30 kb/s, 10 fps, 10 tbr, 10240 tbn (default)
At least one output file must be specified
`

const ffmpegProbeSampleMP3 = `Input #0, mp3, from 'gen-audio-mp3.mp3':
  Metadata:
    encoder         : Lavf60.4.100
  Duration: 00:00:01.04, start: 0.025057, bitrate: 65 kb/s
  Stream #0:0: Audio: mp3, 44100 Hz, mono, fltp, 64 kb/s
At least one output file must be specified
`

// TestParseFFmpegProbe 覆盖 stderr 解析：视频（时长+尺寸）、纯音频（无尺寸）、
// 时长不可用（N/A → NaN，但不丢尺寸）、空输出（ok=false）。
func TestParseFFmpegProbe(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOK  bool
		wantDur float64 // NaN 表示期望 NaN
		wantW   int
		wantH   int
	}{
		{name: "视频 mp4：时长+尺寸", in: ffmpegProbeSampleMP4, wantOK: true, wantDur: 1.0, wantW: 120, wantH: 80},
		{name: "纯音频 mp3：只有时长", in: ffmpegProbeSampleMP3, wantOK: true, wantDur: 1.04},
		{
			name:   "长时分秒累加",
			in:     "  Duration: 01:02:03.50, start: 0.0, bitrate: 1 kb/s\n  Stream #0:0: Video: h264, yuv420p, 1920x1080\n",
			wantOK: true, wantDur: 3723.5, wantW: 1920, wantH: 1080,
		},
		{
			name:   "Duration N/A：保留尺寸、时长 NaN",
			in:     "  Duration: N/A, start: 0.000000, bitrate: N/A\n  Stream #0:0: Video: h264, yuv420p, 640x480\n",
			wantOK: true, wantDur: math.NaN(), wantW: 640, wantH: 480,
		},
		{name: "空输出", in: "", wantOK: false, wantDur: math.NaN()},
		{
			name:   "十六进制串不误判成尺寸",
			in:     "  Stream #0:0: Video: h264 (High) (avc1 / 0x31637661), yuv420p, 320x240\n",
			wantOK: true, wantDur: math.NaN(), wantW: 320, wantH: 240,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, ok := parseFFmpegProbe(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v，want %v", ok, tc.wantOK)
			}
			if math.IsNaN(tc.wantDur) {
				if !math.IsNaN(meta.Duration) {
					t.Errorf("Duration = %v，want NaN", meta.Duration)
				}
			} else if math.Abs(meta.Duration-tc.wantDur) > 1e-6 {
				t.Errorf("Duration = %v，want %v", meta.Duration, tc.wantDur)
			}
			if meta.Width != tc.wantW || meta.Height != tc.wantH {
				t.Errorf("尺寸 = %dx%d，want %dx%d", meta.Width, meta.Height, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestMediaSrcToPath 覆盖 src → 本地路径映射：file://、相对（按基准）、无基准、
// 以及三类「不是本地文件」的引用（http(s)/data/blob）与网络基准下的相对引用。
func TestMediaSrcToPath(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		base   string
		want   string
		wantOK bool
	}{
		{name: "file 绝对引用", src: "file:///F:/p/clip.mp4", want: "F:/p/clip.mp4", wantOK: true},
		{name: "相对引用 + file 基准", src: "clip.mp4", base: "file:///F:/p/index.html", want: "F:/p/clip.mp4", wantOK: true},
		{name: "相对引用 + 无基准", src: "clip.mp4", want: "clip.mp4", wantOK: true},
		{name: "http 引用", src: "https://example.com/a.mp4", wantOK: false},
		{name: "data 引用", src: "data:video/mp4;base64,AAAA", wantOK: false},
		{name: "blob 引用", src: "blob:null/1234", wantOK: false},
		{name: "相对引用 + 网络基准", src: "clip.mp4", base: "http://host/dir/index.html", wantOK: false},
		{name: "空 src", src: "  ", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mediaSrcToPath(tc.src, tc.base)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v，want %v（got %q）", ok, tc.wantOK, got)
			}
			if ok && filepath.ToSlash(got) != tc.want {
				t.Errorf("path = %q，want %q", filepath.ToSlash(got), tc.want)
			}
		})
	}
}

// TestMediaProbeEndToEnd 用真 ffmpeg 生成一个 1 秒 120x80 的测试视频再探测——
// 覆盖「调用 ffmpeg → 解析 → 缓存」的完整链路。本机没有 ffmpeg 时跳过（宿主
// 没有 ffmpeg 时 resolver 本就返回 ok=false，引擎保持「时长未知」）。
func TestMediaProbeEndToEnd(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过真实探测链路（引擎会保持 duration=NaN）")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "probe.mp4")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=120x80:rate=10",
		"-pix_fmt", "yuv420p", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败: %v\n%s", err, out)
	}

	p := NewMediaProbe(ffmpeg)
	meta, ok := p.Resolve(clip)
	if !ok {
		t.Fatal("探测 1 秒测试视频失败（ok=false）")
	}
	if math.Abs(meta.Duration-1.0) > 0.05 {
		t.Errorf("Duration = %v，want ≈1.0", meta.Duration)
	}
	if meta.Width != 120 || meta.Height != 80 {
		t.Errorf("尺寸 = %dx%d，want 120x80", meta.Width, meta.Height)
	}
	// 缓存：第二次调用不再跑 ffmpeg（把 PATH 清空也能拿到同样的结果）。
	if meta2, ok2 := p.Resolve(clip); !ok2 || meta2.Duration != meta.Duration {
		t.Errorf("缓存未命中：%+v ok=%v", meta2, ok2)
	}
	// 不存在的文件：ok=false（不是「0 秒」）。
	if _, ok := p.Resolve(filepath.Join(dir, "nope.mp4")); ok {
		t.Error("不存在的文件应返回 ok=false")
	}
}

// TestMediaProbeWithoutFFmpeg 覆盖「本机没有 ffmpeg」：Resolve 一律 ok=false，
// 且不 panic——宿主不能因为缺依赖而崩在媒体元素上。
func TestMediaProbeWithoutFFmpeg(t *testing.T) {
	p := &MediaProbe{bin: "", cache: map[string]probeEntry{}}
	if _, ok := p.Resolve("/definitely/not/here.mp4"); ok {
		t.Error("没有 ffmpeg 时 Resolve 应返回 ok=false")
	}
	// 同一路径再问一次：仍然 false（负结果也进缓存，不会反复 stat/提示）。
	if _, ok := p.Resolve("/definitely/not/here.mp4"); ok {
		t.Error("缓存后仍应 ok=false")
	}
}

// TestMediaProbeFrameAt 用真 ffmpeg 抽帧（主线 A1）：覆盖按时刻抽帧、**越界时刻
// 收敛到最后一帧**（播放到结束时 currentTime == duration，那个精确时刻没有帧——
// 不收敛的话画面会消失）、缓存命中、以及不存在的文件。
func TestMediaProbeFrameAt(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过真实抽帧（宿主无 ffmpeg 时引擎不画帧）")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "twophase.mp4")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x3366CC:s=120x80:d=0.5:r=10",
		"-f", "lavfi", "-i", "color=c=0xCC3366:s=120x80:d=0.5:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[out]", "-map", "[out]",
		"-pix_fmt", "yuv420p", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频失败: %v\n%s", err, out)
	}
	p := NewMediaProbe(ffmpeg)
	if _, ok := p.Resolve(clip); !ok {
		t.Fatal("元数据探测失败（越界收敛依赖它拿到的时长）")
	}
	for _, at := range []float64{0, 0.25, 0.5} {
		data, ok := p.FrameAt(clip, at)
		if !ok || len(data) == 0 {
			t.Fatalf("FrameAt(%.2f) 应抽到帧，got ok=%v len=%d", at, ok, len(data))
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("FrameAt(%.2f) 返回的不是可解码位图: %v", at, err)
		}
	}
	// 时长时刻（1.0s）：没有帧，但必须回退到最后一帧——画面不该消失。
	if data, ok := p.FrameAt(clip, 1.0); !ok || len(data) == 0 {
		t.Fatalf("FrameAt(时长) 应回退到最后一帧，got ok=%v len=%d", ok, len(data))
	}
	// 缓存：把 bin 清掉（模拟 ffmpeg 不可用）后同一时刻仍能拿到结果。
	p.bin = ""
	if data, ok := p.FrameAt(clip, 1.0); !ok || len(data) == 0 {
		t.Fatalf("缓存未命中：清掉 bin 后 FrameAt(1.0) 返回 ok=%v len=%d", ok, len(data))
	}
	if _, ok := p.FrameAt(filepath.Join(dir, "nope.mp4"), 0); ok {
		t.Error("不存在的文件应 ok=false")
	}
}

// TestMediaProbeFrameAtWithoutFFmpeg：「本机没有 ffmpeg」时抽帧一律 ok=false，
// 且不 panic（宿主缺依赖不该崩在媒体元素上）。
func TestMediaProbeFrameAtWithoutFFmpeg(t *testing.T) {
	p := NewMediaProbe("")
	p.bin = ""
	if _, ok := p.FrameAt("/definitely/not/here.mp4", 0); ok {
		t.Error("没有 ffmpeg 时应 ok=false")
	}
}

// TestParseFFmpegProbeFPS 覆盖帧率解析（A2-②）：视频行的 `10 fps`、`29.97 fps`
// 要解析出来；音频行不带帧率；`Video:` 行没有帧率信息时保持 0（未知）。
func TestParseFFmpegProbeFPS(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		wantOK bool
		want   float64
	}{
		{name: "真实 mp4 输出：10 fps", in: ffmpegProbeSampleMP4, wantOK: true, want: 10},
		{name: "纯音频：无帧率", in: ffmpegProbeSampleMP3, wantOK: true, want: 0},
		{
			name:   "小数帧率 29.97",
			in:     "  Stream #0:0: Video: h264, yuv420p, 1920x1080, 29.97 fps, 29.97 tbr\n",
			wantOK: true, want: 29.97,
		},
		{
			name:   "有视频行但没有 fps 字段",
			in:     "  Stream #0:0: Video: h264, yuv420p, 640x480\n",
			wantOK: true, want: 0,
		},
		{
			name:   "帧率单独出现不构成「探测成功」",
			in:     "  Stream #0:0: Video: h264, 25 fps\n",
			wantOK: false, want: 25,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, ok := parseFFmpegProbe(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v，want %v", ok, tc.wantOK)
			}
			if math.Abs(meta.FPS-tc.want) > 1e-6 {
				t.Errorf("FPS = %v，want %v", meta.FPS, tc.want)
			}
		})
	}
}

// TestAlignFrameTime 覆盖「精确到帧」的对齐规则（A2-①）：请求时刻要被挪到**目标
// 帧区间**内（帧 k 的区间是 [k/fps, (k+1)/fps)），因为 ffmpeg 的 `-ss t` 取的是
// 「时间戳 ≥ t 的第一帧」（ceil 语义）——不对齐就会整体提前一帧。
// 期望值由「帧 k 的取样点是 (k-0.5)/fps」推导，不是照抄实现。
func TestAlignFrameTime(t *testing.T) {
	tests := []struct {
		name string
		at   float64
		fps  float64
		want float64
	}{
		{name: "帧率未知：原样返回", at: 0.55, fps: 0, want: 0.55},
		{name: "帧率未知（NaN）：原样返回", at: 0.55, fps: math.NaN(), want: 0.55},
		{name: "0：仍是首帧", at: 0, fps: 10, want: 0},
		{name: "负数：收敛到首帧", at: -0.2, fps: 10, want: -0.2},
		{name: "首帧区间内 0.09 → 帧 0", at: 0.09, fps: 10, want: 0},
		{name: "帧 5 前段 0.51 → 取样点 0.45", at: 0.51, fps: 10, want: 0.45},
		{name: "帧 5 中心 0.55 → 取样点 0.45", at: 0.55, fps: 10, want: 0.45},
		{name: "帧 5 后段 0.59 → 取样点 0.45", at: 0.59, fps: 10, want: 0.45},
		{name: "帧边界 0.6 → 帧 6 取样点 0.55", at: 0.6, fps: 10, want: 0.55},
		{name: "末帧 0.95 → 取样点 0.85", at: 0.95, fps: 10, want: 0.85},
		{name: "30fps 帧 16 中心 ≈0.55", at: 0.55, fps: 30, want: 15.5 / 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := alignFrameTime(tc.at, tc.fps)
			if math.Abs(got-tc.want) > 1e-9 {
				// NaN 参与比较时 math.Abs 也是 NaN → 单独处理
				if math.IsNaN(got) && math.IsNaN(tc.want) {
					return
				}
				t.Errorf("alignFrameTime(%v, %v) = %v，want %v", tc.at, tc.fps, got, tc.want)
			}
		})
	}
}

// TestFrameTimeQuantizer 覆盖「同一帧的不同时刻折叠成一个键」（A2-①）：帧率已知
// 时，同一帧区间内的任意时刻都量化到同一个值（渲染层据此复用缓存条目、预取与
// 绘制互相命中）；帧率未知或 src 不是本地文件时原样返回（不折叠，不猜）。
func TestFrameTimeQuantizer(t *testing.T) {
	p := NewMediaProbe("")
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	abs, err := filepath.Abs(clip)
	if err != nil {
		t.Fatalf("绝对化失败: %v", err)
	}
	// 直接灌探测缓存（不跑 ffmpeg）：只验证折叠规则与元数据的耦合。
	p.mu.Lock()
	p.cache[abs] = probeEntry{meta: bindings.MediaMetadata{Duration: 1, FPS: 10}, ok: true}
	p.mu.Unlock()

	q := p.FrameTimeQuantizer(nil)
	a, b, c := q(clip, 0.51), q(clip, 0.55), q(clip, 0.59)
	if a != b || b != c {
		t.Errorf("同一帧的三个时刻应折叠成一个值：0.51→%v 0.55→%v 0.59→%v", a, b, c)
	}
	if a != 0.45 {
		t.Errorf("第 5 帧的取样点 = %v，want 0.45", a)
	}
	if next := q(clip, 0.6); next != 0.55 {
		t.Errorf("跨帧边界要换键：0.6 → %v，want 0.55", next)
	}
	if first := q(clip, 0.05); first != 0 {
		t.Errorf("首帧区间应收敛到 0：0.05 → %v", first)
	}
	// 帧率未知：不折叠。
	p.mu.Lock()
	delete(p.cache, abs)
	p.mu.Unlock()
	if got := q(clip, 0.51); got != 0.51 {
		t.Errorf("帧率未知时应原样返回，got %v", got)
	}
	// 非本地文件：原样返回（宿主根本不抽这种帧）。
	if got := q("https://example.com/a.mp4", 0.51); got != 0.51 {
		t.Errorf("网络源应原样返回，got %v", got)
	}
}

// pngPixels 把 PNG 字节解码成 RGBA 序列（逐像素比较用）。
func pngPixels(t *testing.T, data []byte) []byte {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码 PNG 失败: %v", err)
	}
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8), byte(a>>8))
		}
	}
	return out
}

// TestMediaProbeFrameAtExactFrame 端到端验证「精确到帧的 seek」（A2-①）：请求某一
// 帧区间内的任意时刻，抽到的都必须是**那一帧**。参照来自 `-vf select=eq(n,k)`（按
// 帧号的独立路径），而不是再用 `-ss` 抽一遍——否则就是拿同一个语义自我印证，测不出
// 提前一帧的偏差。样本用长 GOP（每 100 帧一个关键帧），关键帧定位不可能准，精确性
// 只能来自对齐规则。
func TestMediaProbeFrameAtExactFrame(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过精确抽帧链路")
	}
	dir := t.TempDir()
	colors := []string{"0xFF0000", "0x00FF00", "0x0000FF", "0xFFFF00", "0x00FFFF",
		"0xFF00FF", "0xFFFFFF", "0x000000", "0x808080", "0xFF8000"}
	for i, c := range colors {
		one := filepath.Join(dir, fmt.Sprintf("f%02d.png", i))
		gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=64x48", c),
			"-frames:v", "1", one)
		if out, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("生成第 %d 帧失败: %v\n%s", i, err, out)
		}
	}
	clip := filepath.Join(dir, "ten.mp4")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-framerate", "10", "-i", filepath.Join(dir, "f%02d.png"),
		"-c:v", "libx264", "-g", "100", "-keyint_min", "100", "-sc_threshold", "0",
		"-pix_fmt", "yuv420p", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("编码测试视频失败: %v\n%s", err, out)
	}
	refs := make([][]byte, len(colors))
	for k := range colors {
		one := filepath.Join(dir, fmt.Sprintf("ref%02d.png", k))
		gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
			"-i", clip, "-vf", fmt.Sprintf("select=eq(n\\,%d)", k),
			"-frames:v", "1", one)
		if out, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("抽参照帧 %d 失败: %v\n%s", k, err, out)
		}
		data, err := os.ReadFile(one)
		if err != nil {
			t.Fatalf("读参照帧 %d 失败: %v", k, err)
		}
		refs[k] = pngPixels(t, data)
	}

	p := NewMediaProbe(ffmpeg)
	if _, ok := p.Resolve(clip); !ok {
		t.Fatal("元数据探测失败（帧率是精确抽帧的前提）")
	}
	if fps, ok := p.FPS(clip); !ok || math.Abs(fps-10) > 1e-6 {
		t.Fatalf("帧率 = %v ok=%v，want 10", fps, ok)
	}
	for k := range colors {
		for _, frac := range []float64{0.05, 0.5, 0.95} {
			at := (float64(k) + frac) / 10
			data, ok := p.FrameAt(clip, at)
			if !ok || len(data) == 0 {
				t.Fatalf("FrameAt(%.3f) 应抽到帧", at)
			}
			if got := pngPixels(t, data); !bytes.Equal(got, refs[k]) {
				t.Errorf("FrameAt(%.3f) 不是第 %d 帧（该时刻所属帧）", at, k)
			}
		}
	}
}
