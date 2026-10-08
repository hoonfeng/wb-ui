package app

// 宿主侧图像转码器（AVIF 支持，2026-10-08）：把「引擎解码器不认、但**浏览器
// 支持**」的格式交给 ffmpeg 转成 PNG，让引擎照常走既有的解码/绘制/契约路径。
//
// 为什么需要（实测结论，不是推断）：
//   - goskia 的 libSkiaSharp **没有**编入 AVIF 解码 —— `skia.NewCodec(quad.avif)`
//     返回 `unsupported or corrupt image data`（对照：同一份 PNG 正常解出 120×80）；
//   - 浏览器普遍支持 AVIF（Edge/Chrome 自 85 起）⇒ 若不处理，等级表会把 AVIF
//     记为 L0，而 Edge 能画 ⇒ 「引擎低于浏览器」，这正是本项目要消除的差异；
//   - 本机 ffmpeg 能解 AVIF（libdav1d/libaom，实测 `quad.avif` → 354 字节 PNG，
//     120×80），因此转码路线可行。
//
// ★ 为什么**只放行 AVIF**（不是「ffmpeg 能解的全都放行」）：本项验收口径是
// **对齐浏览器**，不是「比浏览器更强」。ffmpeg 同样能解 TIFF，而浏览器不支持
// TIFF（Edge 里 `<img src=x.tiff>` 一样是失败路径）——若把 TIFF 也放行，等级表
// 会出现「引擎 L3 / 浏览器 L0」的非对齐结果。因此放行清单是**白名单**，只列
// 浏览器支持的格式；TIFF 保持 L0（预期不支持，与文档 §3.4 基线一致）。
//
// 分工与 A3/A3-3 完全同构（路线 (a)：引擎不背解码器）：内存字节 → 临时文件
// （ffmpeg 认路径）→ `ffmpeg -f image2pipe -vcodec png -` → PNG 字节 → 引擎解码。
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/platform/graphics"
)

// imageTranscodeTimeout 是单次转码的超时。转码只解一帧图，正常在几十毫秒内完成；
// 超时即按失败处理（引擎按「不支持」处置，与未装配转码器逐字一致），绝不把绘制
// 路径挂在坏输入上。
const imageTranscodeTimeout = 20 * time.Second

// ImageTranscodeStats 是转码器的计数（诊断与自检用）。
type ImageTranscodeStats struct {
	// Calls 是被询问的次数（引擎在 Skia 解码失败后询问）。
	Calls int64
	// Hits 是内存缓存命中次数（同一份 AVIF 第二次起走这里，不再起 ffmpeg）。
	Hits int64
	// Transcodes 是实际起 ffmpeg 的次数。
	Transcodes int64
	// Failures 是转码失败的次数（含非白名单格式、ffmpeg 不可用、坏数据）。
	Failures int64
	// Rejected 是因**不在放行白名单**（如 TIFF）而直接拒绝的次数——它证明
	// 「对齐浏览器」的白名单真的在起作用，而不是把 ffmpeg 的能力全放出去。
	Rejected int64
}

// ImageTranscoder 是本机 ffmpeg 支撑的图像转码器。
type ImageTranscoder struct {
	bin string

	mu       sync.Mutex
	cache    map[[32]byte][]byte   // 内容摘要 → 转码后的 PNG 字节
	negative map[[32]byte]struct{} // 内容摘要 → 已知转不出来（负缓存，避免反复起进程）
	temps    map[string]struct{}   // 本进程写下的输入临时文件（供收尾清理）
	stats    ImageTranscodeStats
}

// NewImageTranscoder 建立转码器。ffmpegPath 为空时按 PATH 查找 ffmpeg；显式给了路径
// （或命令名）时先解析一次：**解析不出来就把 bin 置空**，于是 Available 的语义是
// 「手上有可用的 ffmpeg」，而不是「调用方传了个非空字符串」——没有解码器时转码
// 如实失败并记账，绝不假装成功（与 TC-M-603「未装配解码器就 reject」同一条纪律）。
func NewImageTranscoder(ffmpegPath string) *ImageTranscoder {
	bin := strings.TrimSpace(ffmpegPath)
	if bin == "" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			bin = p
		}
	} else if p, err := exec.LookPath(bin); err == nil {
		bin = p
	} else if _, err := os.Stat(bin); err != nil {
		bin = ""
	}
	return &ImageTranscoder{
		bin:      bin,
		cache:    map[[32]byte][]byte{},
		negative: map[[32]byte]struct{}{},
		temps:    map[string]struct{}{},
	}
}

// InstallImageTranscoder 建立转码器并注册到引擎（graphics.SetImageTranscoder），
// 返回它以便宿主查看计数/清理。ffmpeg 不可用时**仍然装配**：每次转码返回 false ⇒
// 引擎按「不支持」处置（AVIF 停在 L0）——如实失败，不假装成功（与媒体链路
// 「没有 ffmpeg 就保持时长未知」同一纪律）。
func InstallImageTranscoder(ffmpegPath string) *ImageTranscoder {
	t := NewImageTranscoder(ffmpegPath)
	graphics.SetImageTranscoder(t.Transcode)
	return t
}

// Stats 返回计数快照。
func (t *ImageTranscoder) Stats() ImageTranscodeStats {
	if t == nil {
		return ImageTranscodeStats{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats
}

// Available 报告 ffmpeg 是否可用（不可用时透传的转码一律失败）。
func (t *ImageTranscoder) Available() bool {
	return t != nil && strings.TrimSpace(t.bin) != ""
}

// Transcode 是注册给引擎的转码函数：字节 → PNG 字节，ok=false 表示不转（引擎按
// 不支持处理）。
//
// 调用时机：**只在 Skia 解码失败之后**（常见格式零开销）。
func (t *ImageTranscoder) Transcode(data []byte) ([]byte, bool) {
	if t == nil || len(data) == 0 {
		return nil, false
	}
	t.mu.Lock()
	t.stats.Calls++
	t.mu.Unlock()

	// 白名单：只放行浏览器支持的格式（当前仅 AVIF）。TIFF 等一律拒绝，
	// 保持「引擎不支持的格式 = 浏览器也不支持的格式」这一个口径。
	if !isAVIFImage(data) {
		t.mu.Lock()
		t.stats.Rejected++
		t.mu.Unlock()
		return nil, false
	}

	sum := sha256.Sum256(data)
	t.mu.Lock()
	if png, ok := t.cache[sum]; ok {
		t.stats.Hits++
		t.mu.Unlock()
		return png, true
	}
	_, bad := t.negative[sum]
	t.mu.Unlock()
	if bad {
		return nil, false
	}

	if !t.Available() {
		t.recordFailure(sum)
		return nil, false
	}

	path, ok := t.tempInputFile(sum, data)
	if !ok {
		t.recordFailure(sum)
		return nil, false
	}
	png, ok := t.runFFmpeg(path)
	if !ok {
		t.recordFailure(sum)
		return nil, false
	}

	t.mu.Lock()
	t.cache[sum] = png
	t.stats.Transcodes++
	t.mu.Unlock()
	return png, true
}

func (t *ImageTranscoder) recordFailure(sum [32]byte) {
	t.mu.Lock()
	t.negative[sum] = struct{}{}
	t.stats.Failures++
	t.mu.Unlock()
}

// tempInputFile 把待转码字节落盘并返回路径。命名 = 内容摘要（sha256 前 8 字节）：
// 同一份数据只落盘一次、**跨进程**也命中，与 data: 链路（mediadataurl.go）与
// WebAudio 解码（webaudio.go）同一套取舍。刻意不带扩展名——ffmpeg 靠内容嗅探
// 识别容器，扩展名反而可能误导（与 audioDecodeTempFile 同一条纪律）。
func (t *ImageTranscoder) tempInputFile(sum [32]byte, data []byte) (string, bool) {
	path := filepath.Join(os.TempDir(), "wbui-imgtranscode-"+hex.EncodeToString(sum[:8]))
	if _, err := os.Stat(path); err != nil {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", false
		}
		t.mu.Lock()
		t.temps[path] = struct{}{}
		t.mu.Unlock()
	}
	return path, true
}

// runFFmpeg 把输入转成 PNG 写到 stdout 收回来。`-frames:v 1`：只要第一帧（AVIF
// 是静态图；动画 AVIF 取首帧，与浏览器对 `<img>` 的静态呈现一致）。
func (t *ImageTranscoder) runFFmpeg(path string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), imageTranscodeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.bin, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-i", path, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return nil, false
	}
	return out.Bytes(), true
}

// CleanupImageTranscodeTempFiles 删除**本进程**写下的输入临时文件，返回删除个数。
// 刻意不自动调用（与 CleanupMediaDataURLFiles / CleanupWebAudioTempFiles 同一理由：
// 文件按内容命名、跨进程复用，进程一启动就清只会白白重写一遍）。
func (t *ImageTranscoder) CleanupImageTranscodeTempFiles() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	paths := make([]string, 0, len(t.temps))
	for p := range t.temps {
		paths = append(paths, p)
	}
	t.temps = map[string]struct{}{}
	t.cache = map[[32]byte][]byte{}
	t.negative = map[[32]byte]struct{}{}
	t.mu.Unlock()

	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed
}

// ─── 格式判定（按内容，不看扩展名）────────────────────────

// isAVIFImage 按 ISO-BMFF 的 `ftyp` 盒判定 AVIF：major_brand 或 compatible_brands
// 里出现 `avif`（静态）/ `avis`（序列）。
//
// 为什么按内容而非扩展名：引擎对图片一律「按内容解码、不看扩展名」（探针用例
// TC-M-404 就是 `.png` 实为 JPEG 也要能画），转码白名单必须与这条口径一致——
// 一个改名的 `x.png`（实为 AVIF）在浏览器里能画，这里也必须能画。
func isAVIFImage(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	switch string(data[8:12]) {
	case "avif", "avis":
		return true
	}
	// compatible_brands：紧跟 major_brand(4) + minor_version(4) 之后的品牌列表，
	// 每 4 字节一个，直到 ftyp 盒结束（盒长在头 4 字节；异常长度按数据末尾兜底）。
	end := len(data)
	if boxLen := int(binary.BigEndian.Uint32(data[0:4])); boxLen >= 16 && boxLen <= len(data) {
		end = boxLen
	}
	for i := 16; i+4 <= end; i += 4 {
		switch string(data[i : i+4]) {
		case "avif", "avis":
			return true
		}
	}
	return false
}
