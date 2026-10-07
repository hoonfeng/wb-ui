package app

// 宿主侧 `data:` URL 落盘（主线 A3 遗留：data: 来源音频停在 L1）。
//
// 为什么需要它：宿主把解码/抽帧交给 ffmpeg（路线 a 的分工：引擎不背解码器），
// 而 ffmpeg 只认「文件路径 / 管道」；`data:` 是**内联字节**（RFC 2397），天生
// 没有路径可给。于是 data: 来源的音频在 A3 交付时只能止步于元数据层（16 格 L1
// 进了基线缺口），视频的 data: 同理连抽帧都进不去（`mediaSrcToPath` 一律 false）。
//
// 修法：把 data: 解成字节 → 落到系统临时目录 → 后面的链路（元数据探测 `probeWithFFmpeg`
// / 视频抽帧 `FrameAt` / 音频 PCM 解码）把它当普通本地文件用。一处修好，三条链路
// 同时受益，不必在每一条上各写一套「先解码再喂管道」的分支。
//
// 命名 = 内容摘要（sha256 前 8 字节）+ 按 MIME 推断的扩展名，因此：
//   - 同一 data URI **只落盘一次**（一页里同一份内联样本被多处引用时很关键）；
//   - **跨进程**也命中（连续两次全量跑不会各写一份，第二次直接复用已有文件）；
//   - 探测缓存与帧缓存用的键稳定——渲染层/绑定层按 `(路径, 时刻)` 建键，路径随
//     内容而定，同一 URI 的多次请求才会对上同一个缓存项（否则每帧都要重解一次
//     base64 再落一次盘）。
//
// 扩展名只是给 ffmpeg 一个倾向性提示（它主要靠内容嗅探），未知 MIME 时留空。
import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mediaDataURLMu    sync.Mutex
	mediaDataURLPaths = map[string]string{}   // data URL 原文 → 临时文件路径
	mediaDataURLFiles = map[string]struct{}{} // 本进程写下的文件（供收尾清理）
)

// mediaDataURLToFile 把 `data:` URL 解码并落盘，返回本地路径。
// ok=false 表示不是 data: URL、编码非法或内容为空（调用方按「拿不到路径」处理，
// 与 http(s)/blob 引用同一条路径——引擎没有网络栈，那两类是另一条主线）。
func mediaDataURLToFile(uri string) (string, bool) {
	s := strings.TrimSpace(uri)
	const scheme = "data:"
	if len(s) <= len(scheme) || !strings.EqualFold(s[:len(scheme)], scheme) {
		return "", false
	}

	mediaDataURLMu.Lock()
	hit, cached := mediaDataURLPaths[s]
	mediaDataURLMu.Unlock()
	if cached {
		return hit, true
	}

	rest := s[len(scheme):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", false
	}
	meta, payload := rest[:comma], rest[comma+1:]

	encoded := strings.TrimSpace(meta)
	isBase64 := false
	if n := len(encoded); n >= len(";base64") && strings.EqualFold(encoded[n-len(";base64"):], ";base64") {
		isBase64 = true
		encoded = encoded[:n-len(";base64")]
	}
	// mediatype 可以带参数（如 `audio/ogg;codecs=opus`）：只取类型部分定扩展名。
	mime := encoded
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	mime = strings.ToLower(strings.TrimSpace(mime))

	var data []byte
	if isBase64 {
		// 去空白：页面里换行的长 base64 很常见（Go 的解码器只忽略 \r\n）。
		cleaned := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\r', '\n':
				return -1
			}
			return r
		}, payload)
		decoded, err := base64.StdEncoding.DecodeString(cleaned)
		if err != nil {
			// 手工拼的样例常漏补位：再按无补位解一次。
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(cleaned, "="))
		}
		if err != nil {
			return "", false
		}
		data = decoded
	} else {
		// 非 base64 的 data URI 按 RFC 2397 是 URL 编码的字节。
		decoded, err := url.PathUnescape(payload)
		if err != nil {
			return "", false
		}
		data = []byte(decoded)
	}
	if len(data) == 0 {
		return "", false
	}

	sum := sha256.Sum256(data)
	name := "wbui-dataurl-" + hex.EncodeToString(sum[:8]) + dataURIMimeExtension(mime)
	path := filepath.Join(os.TempDir(), name)
	if _, err := os.Stat(path); err != nil {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", false
		}
		mediaDataURLMu.Lock()
		mediaDataURLFiles[path] = struct{}{}
		mediaDataURLMu.Unlock()
	}

	mediaDataURLMu.Lock()
	mediaDataURLPaths[s] = path
	mediaDataURLMu.Unlock()
	return path, true
}

// dataURIMimeExtension 按 MIME 给出落盘文件的后缀（未知 MIME 返回空串，由 ffmpeg
// 靠内容嗅探）。
func dataURIMimeExtension(mime string) string {
	switch mime {
	case "audio/wav", "audio/wave", "audio/x-wav", "audio/vnd.wave":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg", "application/ogg", "audio/vorbis":
		return ".ogg"
	case "audio/mp4", "audio/x-m4a", "audio/m4a", "audio/aac", "audio/x-aac":
		return ".m4a"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/webm":
		return ".webm"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/ogg":
		return ".ogv"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/svg+xml":
		return ".svg"
	}
	return ""
}

// CleanupMediaDataURLFiles 删除**本进程**写下的 data: 临时文件，返回删除个数。
//
// 刻意不自动调用：文件按内容命名、跨进程复用，进程一启动就清只会白白重写一遍。
// 需要「跑完不留垃圾」时（探针收尾）显式调用即可。
func CleanupMediaDataURLFiles() int {
	mediaDataURLMu.Lock()
	paths := make([]string, 0, len(mediaDataURLFiles))
	for p := range mediaDataURLFiles {
		paths = append(paths, p)
	}
	mediaDataURLFiles = map[string]struct{}{}
	mediaDataURLPaths = map[string]string{}
	mediaDataURLMu.Unlock()

	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed
}
