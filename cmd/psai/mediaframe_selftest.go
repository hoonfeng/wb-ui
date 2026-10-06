package main

// 视频出画面的无头自检（实现路径主线 A1：「宿主注入帧流」路线）。
//
// 判据（对齐 docs/implementation-path.md §2 与 docs/media-format-verification-plan.md
// 的 TC-M-502）：
//
//	A1-1 无 poster 的 <video> 画出宿主注入的**帧**（截图中心像素 = 参照帧像素）
//	A1-2 带 poster 的 <video> 未播放时画 poster（HTML show poster flag，§4.8.8）
//	A1-3 seek 到非 0 后 poster 让位给真实帧
//
// 期望像素**不手写**：由独立的 ffmpeg 抽帧（绕开被测代码，但同一条解码链）取
// 中心像素作为参照——避免「猜一个颜色再让被测实现去凑」。取样点另做
// elementFromPoint 遮挡校验：读到的像素必须确实来自我们插入的元素。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// 视频自检的布局常量（fixed 定位 → 与页面滚动无关；取样点 = 内容盒中心）。
const (
	mvW         = 120
	mvH         = 80
	mvLeft      = 8
	mvFrameTop  = 8
	mvPosterTop = 140
)

// mvColorTolerance 是像素比对的容差：ffmpeg 抽帧（PNG/sRGB）→ Skia 绘制 →
// 预乘 RGBA 读回，整条链上允许 ±1~2 的舍入；视频本身还经过 yuv420p 转换
// （参照值同样来自该转换，所以容差只需覆盖渲染链）。
const mvColorTolerance = 14

// mediaFrameSelfCheck 跑主线 A1 的三条判据。本机没有 ffmpeg 时打印跳过。
func mediaFrameSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：视频出画面（主线 A1 · 宿主注入帧流）===")
	clip, err := ensureColorClip()
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-1/2/3：跳过——%v\n", err)
		return
	}
	want0, err := referenceFrameRGB(clip, 0)
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-1/2/3：跳过——参照帧抽取失败: %v\n", err)
		return
	}
	poster, err := solidPNGDataURI(color.NRGBA{R: 255, A: 255}, 16, 16)
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-1/2/3：跳过——生成 poster 图失败: %v\n", err)
		return
	}

	cx := mvLeft + mvW/2
	frameCY := mvFrameTop + mvH/2
	posterCY := mvPosterTop + mvH/2

	script := fmt.Sprintf(`(function(){
		window.__mv = {};
		var mk = function(id, top, posterURI){
			var v = document.createElement('video');
			v.id = id;
			v.src = %s;
			v.setAttribute('style', 'position:fixed;left:%dpx;top:' + top + 'px;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
			if (posterURI) { v.setAttribute('poster', posterURI); }
			document.body.appendChild(v);
			window.__mv[id] = v;
			return v;
		};
		mk('mv-frame', %d, null);
		mk('mv-poster', %d, %s);
		var hit = function(x, y){ var e = document.elementFromPoint(x, y); return e ? (e.id || e.tagName) : 'null'; };
		window.__mv.hitFrame = hit(%d, %d);
		window.__mv.hitPoster = hit(%d, %d);
	})()`,
		jsString(fileURLOf(clip)), mvLeft, mvW, mvH,
		mvFrameTop, mvPosterTop, jsString(poster),
		cx, frameCY, cx, posterCY)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A1-1/2/3：插入 <video> 失败: %v\n", err)
		return
	}
	// ★ 自检不得留痕：这两个元素是 fixed + 最高 z-index（取样点必须稳定），会挡住
	// 后续自检的点击目标（CDP 判据 5 点的就是页面左上角）——测完立即移除。
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			['mv-frame','mv-poster'].forEach(function(id){
				var e = document.getElementById(id);
				if (e && e.parentNode) { e.parentNode.removeChild(e); }
			});
			window.__mv = null;
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()
	ic.settle(wv, 10)

	// 遮挡校验：取样点必须命中插入的元素，否则下面读到的像素属于别的元素。
	// elementFromPoint 缺失时（本引擎应已实现）只提示、不判失败。
	probe(wv, "window.__mv.hitFrame")
	probe(wv, "window.__mv.hitPoster")

	pixels, err := wv.Render()
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-1/2：渲染失败: %v\n", err)
		return
	}
	w := wv.Width()

	// ── 判据 A1-1：无 poster → 画出注入帧 ──
	gotFrame, alpha := mvPixelRGBA(pixels, w, cx, frameCY)
	if alpha < 250 {
		fmt.Printf("  ✗ 判据 A1-1：取样点 alpha=%d（<video> 内容盒没被画满）\n", alpha)
	} else if mvColorNear(gotFrame, want0, mvColorTolerance) {
		fmt.Printf("  ✓ 判据 A1-1：无 poster 的 <video> 画出注入帧（像素 %v ≈ 参照 %v）\n", gotFrame, want0)
	} else {
		fmt.Printf("  ✗ 判据 A1-1：像素 %v ≠ 参照帧 %v（宿主未注帧或 painter 未画）\n", gotFrame, want0)
	}

	// ── 判据 A1-2：有 poster 且未播放 → 画 poster ──
	gotPoster, _ := mvPixelRGBA(pixels, w, cx, posterCY)
	redFrame := [3]uint8{255, 0, 0}
	if mvColorNear(gotPoster, redFrame, mvColorTolerance) {
		fmt.Printf("  ✓ 判据 A1-2：带 poster 的 <video> 显示 poster（像素 %v ≈ 红）\n", gotPoster)
	} else if mvColorNear(gotPoster, want0, mvColorTolerance) {
		fmt.Printf("  ✗ 判据 A1-2：显示的是视频帧 %v —— show poster flag 未生效（帧越过了 poster）\n", gotPoster)
	} else {
		fmt.Printf("  ✗ 判据 A1-2：像素 %v 既非 poster 红也非视频帧 %v\n", gotPoster, want0)
	}

	// ── 判据 A1-3：seek 到非 0 → poster 让位给帧 ──
	if _, err := wv.EvalJS(`(function(){ window.__mv["mv-poster"].currentTime = 0.5; })()`); err != nil {
		fmt.Printf("  ✗ 判据 A1-3：设置 currentTime 失败: %v\n", err)
		return
	}
	ic.settle(wv, 8)
	want05, err := referenceFrameRGB(clip, 0.5)
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-3：跳过——0.5s 参照帧抽取失败: %v\n", err)
		return
	}
	pixels2, err := wv.Render()
	if err != nil {
		fmt.Printf("  ✗ 判据 A1-3：渲染失败: %v\n", err)
		return
	}
	gotSeek, _ := mvPixelRGBA(pixels2, w, cx, posterCY)
	if mvColorNear(gotSeek, want05, mvColorTolerance) {
		fmt.Printf("  ✓ 判据 A1-3：seek 到 0.5s 后显示帧（像素 %v ≈ 参照 %v），poster 让位\n", gotSeek, want05)
	} else {
		fmt.Printf("  ✗ 判据 A1-3：像素 %v ≠ 0.5s 参照帧 %v（poster 未让位或帧未更新）\n", gotSeek, want05)
	}
}

// ensureColorClip 生成/复用纯色视频（120x80、1 秒、10fps）：纯色让「画出的像素
// 是不是视频帧」可以逐通道判定，比 testsrc 的花屏稳定得多。产物在 _temp 下，
// 与其它证据同规矩（不入库）。
func ensureColorClip() (string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("本机没有 ffmpeg：无法抽取视频帧")
	}
	dir := filepath.Join("_temp", "mediaverify")
	out := filepath.Join(dir, "solid-120x80-1s.mp4")
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// 0x3366CC：(51,102,204)——三个通道都远离 0/255，量化误差不会把它推到判据的
	// 边界上。
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x3366CC:s=120x80:d=1:r=10",
		"-pix_fmt", "yuv420p", abs)
	if outp, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("生成纯色样本失败: %v（%s）", err, string(outp))
	}
	return abs, nil
}

// referenceFrameRGB 用独立的 ffmpeg 调用抽 at 秒处的帧（绕开 MediaProbe），
// 解码 PNG 后取中心像素——作为「引擎画出来的像素应该等于什么」的参照。
func referenceFrameRGB(clip string, at float64) ([3]uint8, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return [3]uint8{}, fmt.Errorf("本机没有 ffmpeg")
	}
	args := []string{"-hide_banner", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at, 'f', 3, 64))
	}
	args = append(args, "-i", clip, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	out, err := exec.Command(ffmpeg, args...).Output()
	if err != nil {
		return [3]uint8{}, fmt.Errorf("ffmpeg 抽帧失败: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		return [3]uint8{}, fmt.Errorf("解码参照帧失败: %v", err)
	}
	b := img.Bounds()
	c := color.NRGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.NRGBA)
	return [3]uint8{c.R, c.G, c.B}, nil
}

// mvTailEpsilon 是「结束时刻参照」的回退量（秒）：与 app/mediaprobe.go 的
// mediaFrameFallbackStep 同值——宿主把 `currentTime == duration` 的请求收敛到
// `duration - 0.1`，自检参照取同一位置的帧。宿主不用 `-sseof`（本机实测该选项在
// 这些样本上抽不到帧，见 app/mediaprobe.go 注释），所以这里也用显式 `-ss`。
const mvTailEpsilon = 0.1

// solidPNGDataURI 生成纯色 PNG 的 data: URI（poster 用它：data: 引用不受资源
// 策略限制，避免自检依赖 Toolkit 模式的放行开关）。
func solidPNGDataURI(c color.NRGBA, w, h int) (string, error) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// mvPixelRGBA 读渲染缓冲（预乘 RGBA、行优先）中 (x,y) 的像素。
func mvPixelRGBA(pixels []byte, w, x, y int) ([3]uint8, uint8) {
	i := (y*w + x) * 4
	if x < 0 || y < 0 || i+3 >= len(pixels) {
		return [3]uint8{}, 0
	}
	return [3]uint8{pixels[i], pixels[i+1], pixels[i+2]}, pixels[i+3]
}

// mvColorNear 判定两个不透明像素是否在容差内相等（逐通道）。
func mvColorNear(got, want [3]uint8, tol int) bool {
	for i := 0; i < 3; i++ {
		d := int(got[i]) - int(want[i])
		if d < 0 {
			d = -d
		}
		if d > tol {
			return false
		}
	}
	return true
}

// ─── 主线 A2 起步：播放时画面随时间推进（L4 的起点）────────────────────────

// mediaPlaybackSelfCheck 覆盖 A2 的第一条：**播放时画面真的会随时间变化**。
//
// 判据：
//
//	A2-1 currentTime 被播放时钟推进，且该时刻的画面 = 独立抽帧的参照帧
//	A2-2 播放到末尾后画面与**播放前的首帧**不同（帧真的推进了，不是永远停在第一帧）
//	A2-3 事件序列含 play → playing → timeupdate → ended（TC-M-503 的时钟部分）
//	A2-4 播放全程渲染线程**没有同步**向宿主要过帧（帧全靠预取/异步交付，
//	     engine/rendering 的 VideoFrameStats.SyncRequests 为 0），且预取确实在跑
//
// 样本用「前 0.5s 蓝、后 0.5s 洋红」的两段式视频：单色样本无法区分「换帧了」与
// 「停在同一帧」。参照值同样由独立 ffmpeg 抽帧给出，时刻取自引擎回读的
// currentTime——因此判据不依赖 tick 落在哪个瞬间。
func mediaPlaybackSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：播放时画面推进（主线 A2 起步）===")
	clip, err := ensureTwoPhaseClip()
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-1/2/3：跳过——%v\n", err)
		return
	}
	cx := mvLeft + mvW/2
	cy := mvFrameTop + mvH/2
	script := fmt.Sprintf(`(function(){
		window.__mplay = { events: [] };
		var v = document.createElement('video');
		v.id = 'mv-play';
		v.src = %s;
		v.setAttribute('style', 'position:fixed;left:%dpx;top:%dpx;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
		['play','playing','timeupdate','ended'].forEach(function(t){
			v.addEventListener(t, function(){ window.__mplay.events.push(t); });
		});
		document.body.appendChild(v);
		window.__mplay.v = v;
	})()`, jsString(fileURLOf(clip)), mvLeft, mvFrameTop, mvW, mvH)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A2-1/2/3：插入并播放 <video> 失败: %v\n", err)
		return
	}
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			var e = document.getElementById('mv-play');
			if (e && e.parentNode) { e.parentNode.removeChild(e); }
			window.__mplay = null;
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()

	// 起点参照：**播放前**的首帧（show poster flag 置位但元素没有 poster → 画的
	// 就是当前帧，见 rendering.videoShowsPoster）。A2-2 用它作对照。
	ic.settle(wv, 8)
	want0, err := referenceFrameRGB(clip, 0)
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-2：起点参照帧抽取失败: %v\n", err)
		return
	}
	pixels0, err := wv.Render()
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-2：渲染失败: %v\n", err)
		return
	}
	got0, _ := mvPixelRGBA(pixels0, wv.Width(), cx, cy)
	if !mvColorNear(got0, want0, mvColorTolerance) {
		fmt.Printf("  ✗ 判据 A2-2：播放前画面 %v ≠ 首帧参照 %v（起点不成立，后续比较无意义）\n", got0, want0)
		return
	}
	fmt.Printf("  · 起点画面 %v = 首帧参照（前段色）\n", got0)

	// 判据 A2-4 的统计基线：只看**播放期间**的取帧方式（上面加载首帧走的是同步
	// 路径，属静止态，不计入）。
	rendering.ResetVideoFrameStats()
	if _, err := wv.EvalJS(`window.__mplay.v.play();`); err != nil {
		fmt.Printf("  ✗ 判据 A2-1/2/3：play() 调用失败: %v\n", err)
		return
	}

	// 推进真实时间：媒体时钟（250ms tick）挂在事件循环的 wall-clock 上。
	advance := func(ms int) {
		deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
		for time.Now().Before(deadline) {
			ic.settle(wv, 1)
			time.Sleep(8 * time.Millisecond)
		}
	}

	advance(420)
	t1, got1, matched1 := mvSampleStable(wv, ic, clip, cx, cy)
	want1, _ := mvReferenceRGB(clip, t1)
	if t1 <= 0 {
		fmt.Printf("  ✗ 判据 A2-1：播放后 currentTime 未推进（仍为 %v）\n", t1)
	} else if matched1 {
		fmt.Printf("  ✓ 判据 A2-1：currentTime=%.2fs 时画面 = 该时刻参照帧 %v\n", t1, want1)
	} else {
		fmt.Printf("  ✗ 判据 A2-1：currentTime=%.2fs 时像素 %v ≠ 参照 %v（帧没跟上时钟）\n", t1, got1, want1)
	}

	// 播放到末尾（时长 1s）：画面应变成后段颜色，且与开头不同。
	advance(1100)
	t2, ok2 := mvEvalNumber(wv, `window.__mplay.v.currentTime`)
	ended, _ := mvEvalString(wv, `String(window.__mplay.v.ended)`)
	events, _ := mvEvalString(wv, `window.__mplay.events.join(",")`)
	if !ok2 {
		fmt.Printf("  ✗ 判据 A2-2：读取 currentTime 失败\n")
		return
	}
	// 判据 A2-4 的统计在此刻取（播放推进刚结束、还没有为采样多画几帧）：
	// 「播放全程有没有在渲染线程上等解码」看的就是它。
	stats := rendering.VideoFrameStatsSnapshot()
	dur, _ := mvEvalNumber(wv, `window.__mplay.v.duration`)
	// 结束时刻（currentTime == duration）在时间轴上没有帧：规范要求保留**最后一帧**
	// ——宿主把该请求收敛到 `duration - mvTailEpsilon`（app/mediaprobe.go 的
	// clampFrameTime）。参照独立取同一位置的帧：命令单独写、不复用宿主收敛逻辑。
	tail := t2
	if dur > 0 && t2 >= dur-0.001 {
		tail = dur - mvTailEpsilon
		if tail < 0 {
			tail = 0
		}
	}
	want2, rerr := mvReferenceRGB(clip, tail)
	got2 := got0
	if rerr == nil {
		got2, _ = mvSampleUntil(wv, ic, cx, cy, want2, mvSampleWindow)
	}
	switch {
	case t2 <= t1:
		fmt.Printf("  ✗ 判据 A2-2：currentTime 停在 %.2fs（未继续推进，起点 %.2fs）\n", t2, t1)
	case rerr != nil:
		fmt.Printf("  ✗ 判据 A2-2：参照帧抽取失败: %v\n", rerr)
	case got2 == got0:
		fmt.Printf("  ✗ 判据 A2-2：结束后画面仍 = 播放前首帧 %v（帧未推进）\n", got2)
	case mvColorNear(got2, want2, mvColorTolerance):
		fmt.Printf("  ✓ 判据 A2-2：结束后（currentTime=%.2fs/时长 %.2fs）画面 %v = 末尾参照 %v，与播放前首帧 %v 不同\n",
			t2, dur, got2, want2, got0)
	default:
		fmt.Printf("  ✗ 判据 A2-2：结束时像素 %v ≠ 参照 %v\n", got2, want2)
	}

	if t2 > t1 && ended == "true" && strings.Contains(events, "play") && strings.Contains(events, "playing") &&
		strings.Contains(events, "timeupdate") && strings.Contains(events, "ended") {
		fmt.Printf("  ✓ 判据 A2-3：事件序列 %s（play→playing→timeupdate→ended）\n", events)
	} else {
		fmt.Printf("  ✗ 判据 A2-3：事件序列 %q（ended=%s，t1=%.2f t2=%.2f）\n", events, ended, t1, t2)
	}

	// ── 判据 A2-4：播放全程渲染线程没有同步等过解码 ──
	// 每换一帧就在渲染线程上跑一次 ffmpeg 是「能用但卡」的实现；A2 的目标是把解码
	// 搬离渲染线程——帧由绑定层预取（下一个时钟步长的时刻）+ 未到时异步交付，
	// painter 只读缓存。SyncRequests 正是「渲染线程等解码」的次数。
	if stats.SyncRequests == 0 && stats.PrefetchRequests > 0 {
		fmt.Printf("  ✓ 判据 A2-4：播放全程同步抽帧 0 次（预取 %d 次、异步交付 %d 帧、回退上一帧 %d 次）\n",
			stats.PrefetchRequests, stats.AsyncFrames, stats.StaleFrames)
	} else {
		fmt.Printf("  ✗ 判据 A2-4：同步抽帧 %d 次（应为 0）、预取 %d 次（应 > 0）——帧还在渲染线程上等解码\n",
			stats.SyncRequests, stats.PrefetchRequests)
	}
}

// ensureTwoPhaseClip 生成「前 0.5s 蓝、后 0.5s 洋红」的两段式视频：帧推进类判据
// 需要帧间可区分的样本（单色样本下「换帧」与「停在同帧」不可分辨）。
func ensureTwoPhaseClip() (string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("本机没有 ffmpeg：无法抽取视频帧")
	}
	dir := filepath.Join("_temp", "mediaverify")
	out := filepath.Join(dir, "twophase-120x80-1s.mp4")
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x3366CC:s=120x80:d=0.5:r=10",
		"-f", "lavfi", "-i", "color=c=0xCC3366:s=120x80:d=0.5:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[out]", "-map", "[out]",
		"-pix_fmt", "yuv420p", abs)
	if outp, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("生成两段式样本失败: %v（%s）", err, string(outp))
	}
	return abs, nil
}

// mvEvalString 执行 JS 并返回字符串结果。
func mvEvalString(wv *webkit.WebView, script string) (string, bool) {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "", false
	}
	return v.ToString(), true
}

// mvEvalNumber 执行 JS 并把结果解析为 float（失败时 ok=false）。
func mvEvalNumber(wv *webkit.WebView, script string) (float64, bool) {
	s, ok := mvEvalString(wv, script)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// mvSampleWindow 是「等画面跟上时钟」的收敛窗口（A2 的异步语义）：帧由宿主
// worker 交付，比时钟晚几十毫秒；窗口内反复重绘，直到像素匹配期望值。
const mvSampleWindow = 500 * time.Millisecond

// mvRefCache 缓存「样本 + 量化时刻 → 参照帧中心像素」：收敛窗口里会反复比对，
// 每次都跑一遍 ffmpeg 太慢。量化到 50ms 对判据无影响（样本的分段边界远粗于
// 50ms，抽帧精度也到不了毫秒）。
var mvRefCache = map[string][3]uint8{}

// mvReferenceRGB 是带缓存的参照帧取色（绕开被测代码，见 referenceFrameRGB）。
func mvReferenceRGB(clip string, at float64) ([3]uint8, error) {
	if at < 0 {
		at = 0
	}
	key := clip + "@" + strconv.FormatInt(int64(at*20+0.5), 10)
	if c, ok := mvRefCache[key]; ok {
		return c, nil
	}
	c, err := referenceFrameRGB(clip, at)
	if err != nil {
		return c, err
	}
	mvRefCache[key] = c
	return c, nil
}

// mvSampleStable 采样「画面已跟上 currentTime」的证据（判据 A2-1）：每轮重绘并
// 重读 currentTime，直到取样点像素 = 该时刻的参照帧、或窗口耗尽。返回最后一次
// 的 (currentTime, 像素, 是否匹配)——「是否匹配」才是判据，像素用于失败诊断。
func mvSampleStable(wv *webkit.WebView, ic *Interceptor, clip string, cx, cy int) (float64, [3]uint8, bool) {
	deadline := time.Now().Add(mvSampleWindow)
	var t float64
	var got [3]uint8
	for {
		ic.settle(wv, 1)
		if v, ok := mvEvalNumber(wv, `window.__mplay.v.currentTime`); ok {
			t = v
		}
		if pixels, err := wv.Render(); err == nil {
			got, _ = mvPixelRGBA(pixels, wv.Width(), cx, cy)
			if want, rerr := mvReferenceRGB(clip, t); rerr == nil && mvColorNear(got, want, mvColorTolerance) {
				return t, got, true
			}
		}
		if time.Now().After(deadline) {
			return t, got, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// mvSampleUntil 反复重绘直到取样点像素匹配期望色（或窗口耗尽），返回最后的像素
// 与是否匹配（判据 A2-2 的末尾帧用它）。
func mvSampleUntil(wv *webkit.WebView, ic *Interceptor, cx, cy int, want [3]uint8, window time.Duration) ([3]uint8, bool) {
	deadline := time.Now().Add(window)
	var got [3]uint8
	for {
		ic.settle(wv, 1)
		if pixels, err := wv.Render(); err == nil {
			got, _ = mvPixelRGBA(pixels, wv.Width(), cx, cy)
			if mvColorNear(got, want, mvColorTolerance) {
				return got, true
			}
		}
		if time.Now().After(deadline) {
			return got, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ─── A2-① 精确到帧的 seek + TC-M-507 连续帧采样 ─────────────

// mvGridColors 是「每帧一色」样本的颜色表（10 帧 @ mvGridFPS）。
// 精度类判据问的是「画面到底是哪一帧」，所以颜色就是帧号：彼此远离、逐通道比对不
// 会混淆即可（单色样本只能答「变了/没变」）。
var mvGridColors = [][3]uint8{
	{204, 51, 51},   // 帧 0 红
	{51, 204, 51},   // 帧 1 绿
	{51, 51, 204},   // 帧 2 蓝
	{204, 204, 51},  // 帧 3 黄
	{51, 204, 204},  // 帧 4 青
	{204, 51, 204},  // 帧 5 品红
	{230, 230, 230}, // 帧 6 近白
	{25, 25, 25},    // 帧 7 近黑
	{128, 128, 128}, // 帧 8 灰
	{230, 128, 25},  // 帧 9 橙
}

// mvGridFPS 是「每帧一色」样本的帧率（10 帧 = 1 秒）。
const mvGridFPS = 10

// ensureFrameGridClip 生成「每帧一色」的 10 帧样本，并用**长 GOP** 编码（每 100 帧
// 一个关键帧）：关键帧定位在这种样本上不可能凑巧对上，帧精度只能来自对齐规则——
// 正是要测的东西。
func ensureFrameGridClip() (string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("本机没有 ffmpeg：无法抽取视频帧")
	}
	dir := filepath.Join("_temp", "mediaverify", "grid")
	abs, err := filepath.Abs(filepath.Join("_temp", "mediaverify", "grid-10frames.mp4"))
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for i, c := range mvGridColors {
		one := filepath.Join(dir, fmt.Sprintf("f%02d.png", i))
		cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=0x%02X%02X%02X:s=%dx%d", c[0], c[1], c[2], mvW, mvH),
			"-frames:v", "1", one)
		if outp, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("生成第 %d 帧失败: %v（%s）", i, err, string(outp))
		}
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-framerate", strconv.Itoa(mvGridFPS), "-i", filepath.Join(dir, "f%02d.png"),
		"-c:v", "libx264", "-g", "100", "-keyint_min", "100", "-sc_threshold", "0",
		"-pix_fmt", "yuv420p", abs)
	if outp, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("编码每帧一色样本失败: %v（%s）", err, string(outp))
	}
	return abs, nil
}

// referenceFrameRGBAtFrame 按**帧号**抽参照帧（`-vf select=eq(n,k)`）。它与宿主
// 抽帧用的 `-ss` 是两条独立路径——因此能测出「-ss 的 ceil 语义让画面整体提前一帧」
// 这类偏差；用同一个 `-ss` 去抽参照只会自我印证（本机实测：请求 0.05s 得到第 1 帧、
// 请求 0.45s 得到第 5 帧）。
func referenceFrameRGBAtFrame(clip string, frame int) ([3]uint8, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return [3]uint8{}, fmt.Errorf("本机没有 ffmpeg")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-i", clip,
		"-vf", fmt.Sprintf("select=eq(n\\,%d)", frame), "-frames:v", "1",
		"-f", "image2pipe", "-vcodec", "png", "-"}
	out, err := exec.Command(ffmpeg, args...).Output()
	if err != nil {
		return [3]uint8{}, fmt.Errorf("ffmpeg 按帧号抽帧失败: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		return [3]uint8{}, fmt.Errorf("解码参照帧失败: %v", err)
	}
	b := img.Bounds()
	c := color.NRGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.NRGBA)
	return [3]uint8{c.R, c.G, c.B}, nil
}

// mvRefFrameCache 缓存「样本 + 帧号 → 中心像素」（判据里同一帧会被反复比对）。
var mvRefFrameCache = map[string][3]uint8{}

// mvRefFrameRGB 是带缓存的按帧号取参照色。
func mvRefFrameRGB(clip string, frame int) ([3]uint8, error) {
	key := clip + "#" + strconv.Itoa(frame)
	if c, ok := mvRefFrameCache[key]; ok {
		return c, nil
	}
	c, err := referenceFrameRGBAtFrame(clip, frame)
	if err != nil {
		return c, err
	}
	mvRefFrameCache[key] = c
	return c, nil
}

// mvNearestFrame 报告当前取样点像素最接近哪一帧（只在失败时用于诊断：判据说
// 「画面不是第 k 帧」时，还要说清它**是**第几帧，否则看不出是提前还是滞后）。
func mvNearestFrame(wv *webkit.WebView, clip string, cx, cy int) int {
	pixels, err := wv.Render()
	if err != nil {
		return -1
	}
	got, _ := mvPixelRGBA(pixels, wv.Width(), cx, cy)
	best, bestDist := -1, 1<<30
	for k := range mvGridColors {
		want, err := mvRefFrameRGB(clip, k)
		if err != nil {
			continue
		}
		d := 0
		for i := 0; i < 3; i++ {
			dd := int(got[i]) - int(want[i])
			if dd < 0 {
				dd = -dd
			}
			d += dd
		}
		if d < bestDist {
			best, bestDist = k, d
		}
	}
	return best
}

// mediaExactSeekSelfCheck 是判据 A2-5：**精确到帧的 seek**（主线 A2-①）。
//
// 对每一帧 k，seek 到该帧区间的**后段** (k+0.95)/fps 与**前段** (k+0.05)/fps，画面
// 都必须等于第 k 帧（参照值来自按帧号的独立抽取，见 referenceFrameRGBAtFrame）。
// 它能直接抓住「帧对齐规则缺失」：ffmpeg 的 `-ss` 取「时间戳 ≥ t 的第一帧」（ceil
// 语义），后段时刻会整帧落到下一帧上，画面就不再是第 k 帧。
func mediaExactSeekSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：精确到帧的 seek（主线 A2-①）===")
	clip, err := ensureFrameGridClip()
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-5：跳过——%v\n", err)
		return
	}
	cx := mvLeft + mvW/2
	cy := mvFrameTop + mvH/2
	script := fmt.Sprintf(`(function(){
		window.__mgrid = {};
		var v = document.createElement('video');
		v.id = 'mv-grid';
		v.src = %s;
		v.muted = true;
		v.setAttribute('style', 'position:fixed;left:%dpx;top:%dpx;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
		document.body.appendChild(v);
		window.__mgrid.v = v;
	})()`, jsString(fileURLOf(clip)), mvLeft, mvFrameTop, mvW, mvH)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A2-5：插入 <video> 失败: %v\n", err)
		return
	}
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			var e = document.getElementById('mv-grid');
			if (e && e.parentNode) { e.parentNode.removeChild(e); }
			window.__mgrid = null;
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()
	ic.settle(wv, 8) // 等元数据与首帧

	checked, fail := 0, 0
	for k := range mvGridColors {
		want, err := mvRefFrameRGB(clip, k)
		if err != nil {
			fmt.Printf("  ✗ 判据 A2-5：第 %d 帧参照抽取失败: %v\n", k, err)
			return
		}
		for _, frac := range []float64{0.05, 0.95} {
			at := (float64(k) + frac) / mvGridFPS
			if _, err := wv.EvalJS(fmt.Sprintf(`window.__mgrid.v.currentTime = %.4f;`, at)); err != nil {
				fmt.Printf("  ✗ 判据 A2-5：设置 currentTime=%.4f 失败: %v\n", at, err)
				return
			}
			got, ok := mvSampleUntil(wv, ic, cx, cy, want, mvSampleWindow)
			checked++
			if !ok {
				fail++
				fmt.Printf("  ✗ 判据 A2-5：seek 到 %.3fs（第 %d 帧区间）画面 %v ≠ 第 %d 帧 %v（实际最像第 %d 帧）\n",
					at, k, got, k, want, mvNearestFrame(wv, clip, cx, cy))
			}
		}
	}
	if fail == 0 {
		fmt.Printf("  ✓ 判据 A2-5：%d 个采样点（每帧取区间前段/后段）画面都 = 该时刻所属帧\n", checked)
	}
}

// mediaContinuousFrameSelfCheck 是判据 A2-6：**播放中连续多帧都落在该落的帧上**
// （媒体验证设计稿的 TC-M-507「连续帧差异」）。比「两帧不同」强的地方：每一帧都要
// 等于它自己的参照色，而不只是「画面变了」。
//
// 用 0.25 倍速播放：每帧（100ms 媒体时间）≈400ms 真实时间，采样窗口（250ms）内
// 时钟不会跨帧——否则「等画面追上时钟」会把时钟等过界，判据自己制造抖动。
func mediaContinuousFrameSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：连续帧采样（TC-M-507）===")
	clip, err := ensureFrameGridClip()
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-6：跳过——%v\n", err)
		return
	}
	cx := mvLeft + mvW/2
	cy := mvFrameTop + mvH/2
	script := fmt.Sprintf(`(function(){
		window.__mseq = {};
		var v = document.createElement('video');
		v.id = 'mv-seq';
		v.src = %s;
		v.muted = true;
		v.setAttribute('style', 'position:fixed;left:%dpx;top:%dpx;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
		document.body.appendChild(v);
		window.__mseq.v = v;
	})()`, jsString(fileURLOf(clip)), mvLeft, mvFrameTop, mvW, mvH)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A2-6：插入 <video> 失败: %v\n", err)
		return
	}
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			var e = document.getElementById('mv-seq');
			if (e && e.parentNode) { e.parentNode.removeChild(e); }
			window.__mseq = null;
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()
	ic.settle(wv, 8)
	if _, err := wv.EvalJS(`window.__mseq.v.playbackRate = 0.25; window.__mseq.v.play();`); err != nil {
		fmt.Printf("  ✗ 判据 A2-6：play() 调用失败: %v\n", err)
		return
	}

	const wantFrames = 5
	sampled := []int{}
	last := -1
	deadline := time.Now().Add(6 * time.Second)
	for len(sampled) < wantFrames && time.Now().Before(deadline) {
		ic.settle(wv, 1)
		t, ok := mvEvalNumber(wv, `window.__mseq.v.currentTime`)
		if !ok {
			time.Sleep(8 * time.Millisecond)
			continue
		}
		frame := int(math.Floor(t*mvGridFPS + 1e-6))
		if frame > len(mvGridColors)-1 {
			frame = len(mvGridColors) - 1 // 末尾：宿主把越界时刻收敛到最后一帧
		}
		if frame == last {
			time.Sleep(12 * time.Millisecond)
			continue
		}
		want, err := mvRefFrameRGB(clip, frame)
		if err != nil {
			fmt.Printf("  ✗ 判据 A2-6：第 %d 帧参照抽取失败: %v\n", frame, err)
			return
		}
		got, matched := mvSampleUntil(wv, ic, cx, cy, want, 250*time.Millisecond)
		if !matched {
			fmt.Printf("  ✗ 判据 A2-6：currentTime=%.3fs（第 %d 帧）画面 %v ≠ 第 %d 帧 %v（实际最像第 %d 帧）\n",
				t, frame, got, frame, want, mvNearestFrame(wv, clip, cx, cy))
			return
		}
		sampled = append(sampled, frame)
		last = frame
		time.Sleep(10 * time.Millisecond)
	}
	if len(sampled) < wantFrames {
		fmt.Printf("  ✗ 判据 A2-6：6s 内只采到 %d 帧（样本 %d 帧、0.25 倍速 ≈4s 放完）：%v\n",
			len(sampled), len(mvGridColors), sampled)
		return
	}
	for i := 1; i < len(sampled); i++ {
		if sampled[i] <= sampled[i-1] {
			fmt.Printf("  ✗ 判据 A2-6：帧号未严格递增：%v\n", sampled)
			return
		}
	}
	fmt.Printf("  ✓ 判据 A2-6：连续采样 %d 帧（帧号 %v），每帧画面都 = 该帧参照\n", len(sampled), sampled)
}

// mvFrameCallbackRecord 是一次 requestVideoFrameCallback 回调记录到的 metadata。
type mvFrameCallbackRecord struct {
	Presented float64 `json:"presented"`
	Media     float64 `json:"media"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
}

// mediaFrameCallbackSelfCheck 是判据 A2-7：**播放中 requestVideoFrameCallback 真的被
// 调用**，且 metadata 的 presentedFrames / mediaTime 随帧推进递增（主线 A2-③）。
//
// 页面里让回调**自续注册**（回调里再 request 一次）——这既是 rVFC 的常规用法，也顺带
// 验证「回调一次性 + 重新注册有效」这条契约。
//
// 已知粒度差异（记在 docs/TECH_DEBT.md）：本引擎的「新帧呈现」判定挂在播放时钟步长上
// （250ms），所以 10fps 的 1 秒播放只回调约 4 次而不是 10 次；判据按**实际语义**断言
// （次数 ≥3、presentedFrames 从 1 起严格递增、mediaTime 严格递增），不假装逐帧。
func mediaFrameCallbackSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：帧呈现回调（主线 A2-③）===")
	clip, err := ensureTwoPhaseClip()
	if err != nil {
		fmt.Printf("  ✗ 判据 A2-7：跳过——%v\n", err)
		return
	}
	script := fmt.Sprintf(`(function(){
		window.__rvf = [];
		var v = document.createElement('video');
		v.id = 'mv-rvfc';
		v.src = %s;
		v.muted = true;
		v.setAttribute('style', 'position:fixed;left:%dpx;top:%dpx;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
		var arm = function(){
			v.requestVideoFrameCallback(function(now, md){
				window.__rvf.push({presented: md.presentedFrames, media: md.mediaTime, w: md.width, h: md.height});
				if (!v.ended) { arm(); }
			});
		};
		arm();
		document.body.appendChild(v);
		window.__rvfEl = v;
	})()`, jsString(fileURLOf(clip)), mvLeft, mvFrameTop, mvW, mvH)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A2-7：插入 <video> 失败: %v\n", err)
		return
	}
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			var e = document.getElementById('mv-rvfc');
			if (e && e.parentNode) { e.parentNode.removeChild(e); }
			window.__rvf = null;
			window.__rvfEl = null;
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()
	ic.settle(wv, 8)
	if _, err := wv.EvalJS(`window.__rvfEl.play();`); err != nil {
		fmt.Printf("  ✗ 判据 A2-7：play() 调用失败: %v\n", err)
		return
	}
	// 推进真实时间到播放结束（或超时）：媒体时钟挂在事件循环的 wall-clock 上。
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		ic.settle(wv, 1)
		if ended, ok := mvEvalString(wv, `window.__rvfEl.ended`); ok && ended == "true" {
			break
		}
		time.Sleep(8 * time.Millisecond)
	}
	ic.settle(wv, 6)

	raw, ok := mvEvalString(wv, `JSON.stringify(window.__rvf)`)
	if !ok {
		fmt.Printf("  ✗ 判据 A2-7：读回回调记录失败\n")
		return
	}
	var recs []mvFrameCallbackRecord
	if err := json.Unmarshal([]byte(raw), &recs); err != nil {
		fmt.Printf("  ✗ 判据 A2-7：解析回调记录失败: %v（%s）\n", err, raw)
		return
	}
	if len(recs) < 3 {
		fmt.Printf("  ✗ 判据 A2-7：播放全程只回调 %d 次（应 ≥3）：%+v\n", len(recs), recs)
		return
	}
	for i, r := range recs {
		if r.Presented != float64(i+1) {
			fmt.Printf("  ✗ 判据 A2-7：第 %d 次回调的 presentedFrames = %v（应自 1 起逐次 +1）：%+v\n", i+1, r.Presented, recs)
			return
		}
		if i > 0 && r.Media <= recs[i-1].Media {
			fmt.Printf("  ✗ 判据 A2-7：mediaTime 未递增：%v → %v\n", recs[i-1].Media, r.Media)
			return
		}
		if r.W != mvW || r.H != mvH {
			fmt.Printf("  ✗ 判据 A2-7：metadata 尺寸 = %vx%v，want %dx%d\n", r.W, r.H, mvW, mvH)
			return
		}
	}
	fmt.Printf("  ✓ 判据 A2-7：播放中回调 %d 次，presentedFrames 1..%d 逐个递增、mediaTime %v→%v、尺寸 %vx%v\n",
		len(recs), len(recs), recs[0].Media, recs[len(recs)-1].Media, recs[0].W, recs[0].H)
}

// ─── A4：动图帧推进（GIF 连续帧差异）────────────────────

// mvGIFColors 是自检生成的三帧 GIF 的颜色（彼此远，逐通道可判定）。
var mvGIFColors = [][3]uint8{{204, 51, 51}, {51, 204, 51}, {51, 51, 204}}

// mvGIFDelayCentis 是每帧延时（GIF 的 Delay 单位是 1/100 秒）→ 300ms。
const mvGIFDelayCentis = 30

// ensureAnimatedGIF 生成/复用三帧 GIF（每帧一色、延时 300ms）。产物在 _temp 下，
// 与其它证据同规矩（不入库）。
func ensureAnimatedGIF() (string, error) {
	abs, err := filepath.Abs(filepath.Join("_temp", "mediaverify", "animated-3frames.gif"))
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs, nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	pal := make(color.Palette, len(mvGIFColors))
	for i, c := range mvGIFColors {
		pal[i] = color.RGBA{R: c[0], G: c[1], B: c[2], A: 255}
	}
	var g gif.GIF
	for i := range mvGIFColors {
		frame := image.NewPaletted(image.Rect(0, 0, mvW, mvH), pal)
		for y := 0; y < mvH; y++ {
			for x := 0; x < mvW; x++ {
				frame.SetColorIndex(x, y, uint8(i))
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, mvGIFDelayCentis)
	}
	f, err := os.Create(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := gif.EncodeAll(f, &g); err != nil {
		return "", err
	}
	return abs, nil
}

// mediaAnimatedImageSelfCheck 是判据 A4-1：GIF 动图的**连续帧差异**（主线 A4）。
//
// 与视频那条判据的差别：视频的帧由宿主按 currentTime 抽，动图的帧由**引擎按帧时长**
// 推进（宿主只提供帧序列，见 engine/rendering/imageanimation.go）。所以这条判据测的是
// 「画面自己会换帧」：连续采样若干次，必须出现多种颜色，且每种颜色都等于样本里某一帧
// 的色（画面 = 那一帧本身，不是别的图或残影）。
func mediaAnimatedImageSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：动图帧推进（主线 A4）===")
	clip, err := ensureAnimatedGIF()
	if err != nil {
		fmt.Printf("  ✗ 判据 A4-1：跳过——%v\n", err)
		return
	}
	// ★ 用 data: URI 注入（而不是 file:// 路径）：自检的探针资源不该受宿主资源策略
	// （interceptor / 模式门禁）影响——data: 不经 loader，直接进解码路径。A1-2 用
	// data: poster 是同一个理由。
	gifBytes, err := os.ReadFile(clip)
	if err != nil {
		fmt.Printf("  ✗ 判据 A4-1：跳过——读取 GIF 失败: %v\n", err)
		return
	}
	cx := mvLeft + mvW/2
	cy := mvFrameTop + mvH/2
	script := fmt.Sprintf(`(function(){
		var img = document.createElement('img');
		img.id = 'mv-gif';
		img.src = %s;
		img.setAttribute('style', 'position:fixed;left:%dpx;top:%dpx;width:%dpx;height:%dpx;margin:0;padding:0;border:0;z-index:2147483647;background:transparent');
		document.body.appendChild(img);
	})()`, jsString(gifDataURI(gifBytes)), mvLeft, mvFrameTop, mvW, mvH)
	if _, err := wv.EvalJS(script); err != nil {
		fmt.Printf("  ✗ 判据 A4-1：插入 <img> 失败: %v\n", err)
		return
	}
	defer func() {
		if _, err := wv.EvalJS(`(function(){
			var e = document.getElementById('mv-gif');
			if (e && e.parentNode) { e.parentNode.removeChild(e); }
		})()`); err != nil {
			fmt.Printf("  （清理测试元素失败：%v）\n", err)
		}
		ic.settle(wv, 4)
	}()
	ic.settle(wv, 8) // 等图片解码 + 动图登记

	if hit, ok := mvEvalString(wv, fmt.Sprintf(`(function(){var e=document.elementFromPoint(%d,%d);return e?e.id:"";})()`, cx, cy)); !ok || hit != "mv-gif" {
		fmt.Printf("  ✗ 判据 A4-1：取样点未命中探针元素（命中 %q）\n", hit)
		return
	}

	const samples = 6
	seen := map[[3]uint8]int{}
	for i := 0; i < samples; i++ {
		pixels, err := wv.Render()
		if err != nil {
			fmt.Printf("  ✗ 判据 A4-1：渲染失败: %v\n", err)
			return
		}
		got, _ := mvPixelRGBA(pixels, wv.Width(), cx, cy)
		seen[got]++
		// 等 350ms（帧时长 300ms → 必定跨帧），期间推进事件循环。
		deadline := time.Now().Add(350 * time.Millisecond)
		for time.Now().Before(deadline) {
			ic.settle(wv, 1)
			time.Sleep(8 * time.Millisecond)
		}
	}

	matched := 0
	for c := range seen {
		for _, want := range mvGIFColors {
			if mvColorNear(c, want, mvColorTolerance) {
				matched++
				break
			}
		}
	}
	if len(seen) < 2 {
		fmt.Printf("  ✗ 判据 A4-1：%d 次采样只出现 %d 种颜色（已登记动图=%v，采样 %v）——动图停在某一帧\n",
			samples, len(seen), rendering.HasAnimatedImages(), seen)
		return
	}
	if matched != len(seen) {
		fmt.Printf("  ✗ 判据 A4-1：出现样本外的颜色（采样 %d 种，只有 %d 种属于样本帧色）\n", len(seen), matched)
		return
	}
	fmt.Printf("  ✓ 判据 A4-1：%d 次采样出现 %d 种帧色（每帧 300ms），画面随帧推进变化且都等于样本帧色\n",
		samples, len(seen))
}

// gifDataURI 把 GIF 字节编成 data: URI（自检探针资源用它绕开宿主资源策略）。
func gifDataURI(data []byte) string {
	return "data:image/gif;base64," + base64.StdEncoding.EncodeToString(data)
}
