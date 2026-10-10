package main

// 应用自证报告：把 MediaStudio 这一轮跑出来的东西（帧数/像素差/音频字节/容器校验/
// 回放观测）整理成一份可复核的 Markdown，落到 dev/media/out/studio/report.md。
//
// 报告只写**实测到的东西**：值取自本轮真实产物（帧文件、WAV 字节、ffprobe 输出、
// 页面 <video> 的 IDL），拿不到的项写「未取到」并计为不通过——不编造。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// verdict 是一条判据的结论。
type verdict struct {
	ok  bool
	got string
}

func mark(ok bool) string {
	if ok {
		return "✅ 通过"
	}
	return "❌ 未通过"
}

// writeReport 生成报告并返回路径。
func (s *studio) writeReport(tk, clip *take, rec *replayRec, uiPNG string) string {
	var b strings.Builder
	now := time.Now().Format("2006-01-02 15:04:05")

	fmt.Fprintf(&b, "# MediaStudio 端到端媒体验证报告\n\n")
	fmt.Fprintf(&b, "- 生成时间：%s\n", now)
	fmt.Fprintf(&b, "- 应用：`cmd/mediastudio`（录屏剪辑台；界面由 wb-ui 引擎渲染）\n")
	fmt.Fprintf(&b, "- 视口：%d×%d · 录制参数：%.1fs @%dfps · 音轨=%v\n",
		s.cfg.w, s.cfg.h, s.cfg.sec, s.cfg.fps, !s.cfg.noAudio)
	fmt.Fprintf(&b, "- ffmpeg：%s\n", ffmpegVersion(s.ffmpeg))
	fmt.Fprintf(&b, "- 素材目录：%s\n\n", s.cfg.samples)

	// ── 一、链路与判据 ──────────────────────────────────────────────
	fmt.Fprintf(&b, "## 一、链路判据\n\n")
	fmt.Fprintf(&b, "| # | 应用功能 | 用到的媒体能力 | 实测 | 结论 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|\n")

	// 1 界面渲染
	uiV := verdict{ok: fileExists(uiPNG) && fileSize(uiPNG) > 10000,
		got: fmt.Sprintf("ui.png %d 字节", fileSize(uiPNG))}
	fmt.Fprintf(&b, "| 1 | 界面渲染 | 引擎布局/绘制/CSS 动画/图片解码 | %s | %s |\n", uiV.got, mark(uiV.ok))

	// 2 录屏抓帧
	framesV := verdict{}
	if tk != nil {
		// 期望帧数按**本次实际录制参数**（页面滑块值）算，不按命令行默认值。
		want := wantFrames(tk.seconds, tk.fps)
		minSize := minFrameBytes(tk.dir, tk.frames)
		framesV = verdict{
			ok:  tk.frames == want && minSize > 1024,
			got: fmt.Sprintf("%d 帧（期望 %d）· 最小帧 %d 字节 · 舞台区域首末帧变化像素占比 %.2f%%",
				tk.frames, want, minSize, tk.frameDiff),
		}
	} else {
		framesV = verdict{got: "未取到（录制未完成）"}
	}
	fmt.Fprintf(&b, "| 2 | 录屏抓帧 | `WebView.Render()`（预乘 RGBA → PNG） | %s | %s |\n", framesV.got, mark(framesV.ok))

	// 3 画面随时间变化（录制期间引擎动画真的在推进）
	motionV := verdict{got: "未取到"}
	if tk != nil {
		// 基线：完全静止时该指标 <0.1%（舞台里只有每秒更新的时间戳文字在变）；
		// 0.5% 以上即说明运动块（CSS 动画）确实在推进。
		motionV = verdict{ok: tk.frameDiff > 0.5,
			got: fmt.Sprintf("舞台区域首末帧变化像素占比 %.2f%%（%d 帧间隔；静止基线 <0.1%%）",
				tk.frameDiff, tk.frames)}
	}
	fmt.Fprintf(&b, "| 3 | 画面推进 | 引擎 CSS 动画 + 事件循环推进 | %s | %s |\n", motionV.got, mark(motionV.ok))

	// 4 环回录音
	audioV := verdict{}
	switch {
	case tk == nil:
		audioV = verdict{got: "未取到"}
	case s.cfg.noAudio:
		audioV = verdict{ok: true, got: "本轮按 -no-audio 跳过"}
	case tk.pcmBytes > 0:
		frames := tk.pcmBytes / maxInt(1, tk.audioFmt.BytesPerFrame())
		audioV = verdict{ok: true, got: fmt.Sprintf("%d 字节 PCM（%dHz %dch，约 %.2fs）→ rec.wav %d 字节",
			tk.pcmBytes, tk.audioFmt.SampleRate, tk.audioFmt.Channels,
			tk.audioFmt.Duration(int64(frames)), fileSize(tk.wav))}
	default:
		audioV = verdict{got: "环回采集未拿到 PCM"}
	}
	fmt.Fprintf(&b, "| 4 | 环回录音 | WASAPI loopback + 输出设备播放测试音 | %s | %s |\n", audioV.got, mark(audioV.ok))

	// 5 视频合成
	mp4V := verdict{got: "未取到"}
	if tk != nil {
		mp4V = verdict{ok: tk.mp4Bytes > 10000,
			got: fmt.Sprintf("record.mp4 %d 字节 · 舞台区域 %s", tk.mp4Bytes, tk.stage)}
	}
	fmt.Fprintf(&b, "| 5 | 视频合成 | ffmpeg（宿主编码侧） | %s | %s |\n", mp4V.got, mark(mp4V.ok))

	// 6 剪辑
	clipV := verdict{got: "未取到"}
	if clip != nil {
		clipV = verdict{ok: clip.frames > 0 && clip.mp4Bytes > 10000,
			got: fmt.Sprintf("clip.mp4 %d 字节 · %d 帧 · %.1fs", clip.mp4Bytes, clip.frames, clip.seconds)}
	}
	fmt.Fprintf(&b, "| 6 | 剪辑 | 自持帧序列区间 + 音轨输入级裁剪 | %s | %s |\n", clipV.got, mark(clipV.ok))

	// 7 帧导出
	pngShot := filepath.Join(s.cfg.out, "frame-now.png")
	jpgShot := filepath.Join(s.cfg.out, "frame-now.jpg")
	expV := verdict{ok: fileExists(pngShot) && fileExists(jpgShot),
		got: fmt.Sprintf("frame-now.png %d 字节 · frame-now.jpg %d 字节", fileSize(pngShot), fileSize(jpgShot))}
	fmt.Fprintf(&b, "| 7 | 帧导出 | PNG / JPEG 编码（标准库） | %s | %s |\n", expV.got, mark(expV.ok))

	// 8 回放元数据（元数据链路 + 抽帧通道）
	replayV := verdict{got: "未取到"}
	if rec != nil && tk != nil {
		okDur := rec.Duration > tk.seconds-0.8 && rec.Duration < tk.seconds+0.8
		okDim := rec.VideoW == tk.stage.Dx()
		replayV = verdict{ok: okDur && okDim && rec.ReadyState >= 3,
			got: fmt.Sprintf("duration=%.2fs videoWidth=%d readyState=%.0f currentTime=%.2fs", rec.Duration, rec.VideoW, rec.ReadyState, rec.CurrentTime)}
	}
	fmt.Fprintf(&b, "| 8 | 回放（引擎 <video>） | 宿主元数据 + 按时刻抽帧 | %s | %s |\n", replayV.got, mark(replayV.ok))

	// 9 回放画面与录制帧一致（网格签名）
	sigV := verdict{got: "未取到"}
	if rec != nil {
		sigV = verdict{ok: rec.SigDiff >= 0 && rec.SigDiff < 40,
			got: fmt.Sprintf("16×16 网格平均色差 %.2f（0-255 尺度，越小越像）", rec.SigDiff)}
	}
	fmt.Fprintf(&b, "| 9 | 回放一致性 | 录制中间帧 vs 回放截图 | %s | %s |\n\n", sigV.got, mark(sigV.ok))

	// ── 二、产物 ────────────────────────────────────────────────────
	fmt.Fprintf(&b, "## 二、产物\n\n")
	fmt.Fprintf(&b, "| 产物 | 路径 | 大小 | 说明 |\n|---|---|---|---|\n")
	s.mu.Lock()
	assets := append([]assetInfo(nil), s.assets...)
	s.mu.Unlock()
	for _, a := range assets {
		fmt.Fprintf(&b, "| %s | `%s` | %d 字节 | %s |\n", a.Name, relTo(s.cfg.out, a.Path), a.Bytes, a.Detail)
	}
	fmt.Fprintf(&b, "\n中间帧序列落在 `%s`（不入库，与 `_temp` 同规矩）。\n\n", filepath.ToSlash(s.cfg.tmp))

	// ── 三、容器校验（ffprobe）──────────────────────────────────────
	fmt.Fprintf(&b, "## 三、容器校验（ffprobe）\n\n")
	if tk != nil {
		fmt.Fprintf(&b, "**record.mp4**\n\n```\n%s\n```\n\n", s.probeMedia(tk.mp4))
	} else {
		fmt.Fprintf(&b, "record.mp4：未取到\n\n")
	}
	if clip != nil {
		fmt.Fprintf(&b, "**clip.mp4**\n\n```\n%s\n```\n\n", s.probeMedia(clip.mp4))
	}
	if tk != nil && tk.wav != "" {
		fmt.Fprintf(&b, "**rec.wav**\n\n```\n%s\n```\n\n", s.probeMedia(tk.wav))
	}

	// ── 四、回放观测 ────────────────────────────────────────────────
	fmt.Fprintf(&b, "## 四、回放观测\n\n")
	if rec != nil {
		fmt.Fprintf(&b, "- 事件序列：`%s`\n", rec.Events)
		fmt.Fprintf(&b, "- duration=%.2fs · videoWidth=%d · videoHeight=%d · currentTime=%.2fs · readyState=%.0f\n",
			rec.Duration, rec.VideoW, rec.VideoH, rec.CurrentTime, rec.ReadyState)
		fmt.Fprintf(&b, "- 预览区矩形：%s\n", rec.PreviewRect)
		fmt.Fprintf(&b, "- 回放截图：`%s`；对比用录制中间帧：`%s`\n\n",
			relTo(s.cfg.out, rec.Shot), filepath.ToSlash(rec.MidFrame))
	} else {
		fmt.Fprintf(&b, "未取到回放观测数据。\n\n")
	}

	// ── 五、结论 ────────────────────────────────────────────────────
	all := []verdict{uiV, framesV, motionV, audioV, mp4V, clipV, expV, replayV, sigV}
	pass, total := 0, len(all)
	for _, v := range all {
		if v.ok {
			pass++
		}
	}
	fmt.Fprintf(&b, "## 五、结论\n\n")
	if pass == total {
		msg := "**%d/%d 项判据通过**：应用完成了「录屏 + 录音 → 合成 mp4 → 剪辑 → 导出帧 → 回放比对」的完整闭环，" +
			"这一闭环的每一步都压在本仓媒体链路上，因此可判定媒体能力端到端可用。\n\n"
		fmt.Fprintf(&b, msg, pass, total)
	} else {
		fmt.Fprintf(&b, "**%d/%d 项判据通过**，未通过项见上表（每项的实测值都在表里，可直接复核）。\n\n", pass, total)
	}

	// ── 六、应用日志 ────────────────────────────────────────────────
	fmt.Fprintf(&b, "## 六、应用日志\n\n```\n")
	s.mu.Lock()
	for _, l := range s.logs {
		fmt.Fprintf(&b, "%s\n", l)
	}
	s.mu.Unlock()
	fmt.Fprintf(&b, "```\n")

	path := filepath.Join(s.cfg.out, "report.md")
	if err := os.MkdirAll(s.cfg.out, 0o755); err != nil {
		fmt.Printf("[mediastudio] 建报告目录失败：%v\n", err)
		return path
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fmt.Printf("[mediastudio] 写报告失败：%v\n", err)
	}
	return path
}

// wantFrames 是「时长 × 帧率」的期望帧数（与 record 的取整口径一致）。
func wantFrames(seconds float64, fps int) int {
	n := int(seconds*float64(fps) + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// minFrameBytes 返回帧序列里最小帧的字节数（探测「有帧写坏了」）。
func minFrameBytes(dir string, frames int) int64 {
	min := int64(-1)
	for i := 1; i <= frames; i++ {
		size, err := os.Stat(filepath.Join(dir, fmt.Sprintf(framePattern, i)))
		if err != nil {
			return -1
		}
		if min < 0 || size.Size() < min {
			min = size.Size()
		}
	}
	if min < 0 {
		return 0
	}
	return min
}

// relTo 把绝对路径压成相对输出目录的展示形式（报告可读性）。
func relTo(base, p string) string {
	if rel, err := filepath.Rel(base, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}
