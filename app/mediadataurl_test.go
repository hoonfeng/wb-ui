package app

import (
	"encoding/base64"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMediaDataURLToFileDecodes 覆盖解码与落盘：字节保真、扩展名按 MIME 推断、
// 同一 URI 复用同一路径（不重复写盘）。
func TestMediaDataURLToFileDecodes(t *testing.T) {
	t.Cleanup(func() { CleanupMediaDataURLFiles() })

	raw := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	uri := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(raw)

	path, ok := mediaDataURLToFile(uri)
	if !ok {
		t.Fatalf("应落盘成功，got ok=false")
	}
	if !strings.HasSuffix(path, ".wav") {
		t.Errorf("扩展名应由 MIME 推断（.wav），got %q", filepath.Base(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("落盘文件读不回：%v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("字节不保真：got %q，want %q", got, raw)
	}
	// 再次请求同一 URI：同一路径（缓存命中），且不重复写盘。
	again, ok := mediaDataURLToFile(uri)
	if !ok || again != path {
		t.Errorf("同一 URI 应复用同一路径：got %q ok=%v，want %q", again, ok, path)
	}
}

// TestMediaDataURLToFileMimeAndEncoding 覆盖 MIME 参数剥离、非 base64（百分号编码）、
// 带换行/空白的 base64，以及各类非法输入。
func TestMediaDataURLToFileMimeAndEncoding(t *testing.T) {
	t.Cleanup(func() { CleanupMediaDataURLFiles() })

	t.Run("MIME 带参数只取类型部分定扩展名", func(t *testing.T) {
		uri := "data:audio/ogg;codecs=opus;base64," + base64.StdEncoding.EncodeToString([]byte("OggS"))
		path, ok := mediaDataURLToFile(uri)
		if !ok {
			t.Fatalf("应落盘成功")
		}
		if !strings.HasSuffix(path, ".ogg") {
			t.Errorf("want .ogg，got %q", filepath.Base(path))
		}
	})

	t.Run("未知 MIME 不留扩展名", func(t *testing.T) {
		uri := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString([]byte("xyz"))
		path, ok := mediaDataURLToFile(uri)
		if !ok {
			t.Fatalf("应落盘成功")
		}
		if strings.Contains(filepath.Base(path)[len("wbui-dataurl-"):], ".") {
			t.Errorf("未知 MIME 不应带扩展名：%q", filepath.Base(path))
		}
	})

	t.Run("非 base64 走百分号解码", func(t *testing.T) {
		path, ok := mediaDataURLToFile("data:text/plain,hi%20there")
		if !ok {
			t.Fatalf("应落盘成功")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "hi there" {
			t.Errorf("got %q err=%v，want %q", got, err, "hi there")
		}
	})

	t.Run("base64 里的换行与空格被忽略", func(t *testing.T) {
		b64 := base64.StdEncoding.EncodeToString([]byte("ABCDEFGH"))
		spaced := b64[:4] + "\n " + b64[4:]
		path, ok := mediaDataURLToFile("data:audio/wav;base64," + spaced)
		if !ok {
			t.Fatalf("应落盘成功")
		}
		got, _ := os.ReadFile(path)
		if string(got) != "ABCDEFGH" {
			t.Errorf("got %q，want %q", got, "ABCDEFGH")
		}
	})

	t.Run("漏补位的 base64 也能解", func(t *testing.T) {
		b64 := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte("hello!")), "=")
		path, ok := mediaDataURLToFile("data:audio/wav;base64," + b64)
		if !ok {
			t.Fatalf("应落盘成功")
		}
		got, _ := os.ReadFile(path)
		if string(got) != "hello!" {
			t.Errorf("got %q，want %q", got, "hello!")
		}
	})

	for _, tc := range []struct{ name, uri string }{
		{"不是 data: 方案", "file:///F:/a.wav"},
		{"没有逗号", "data:audio/wav;base64"},
		{"非法 base64", "data:audio/wav;base64,!!!!"},
		{"空内容（base64）", "data:audio/wav;base64,"},
		{"空内容（plain）", "data:text/plain,"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if p, ok := mediaDataURLToFile(tc.uri); ok {
				t.Fatalf("应失败，got path=%q", p)
			}
		})
	}
}

// TestCleanupMediaDataURLFiles 收尾清理：本进程写下的文件被删除，缓存随之失效
// （再请求会重新落盘——所以清理只该在「跑完不留垃圾」时显式调用）。
func TestCleanupMediaDataURLFiles(t *testing.T) {
	uri := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString([]byte("cleanup-me"))
	path, ok := mediaDataURLToFile(uri)
	if !ok {
		t.Fatalf("应落盘成功")
	}
	if n := CleanupMediaDataURLFiles(); n == 0 {
		t.Fatalf("应至少删除 1 个文件")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("清理后文件应不存在：err=%v", err)
	}
	if p, ok := mediaDataURLToFile(uri); !ok || p != path {
		t.Errorf("清理后重新落盘应回到同一路径：got %q ok=%v，want %q", p, ok, path)
	}
	t.Cleanup(func() { CleanupMediaDataURLFiles() })
}

// TestMediaDataURLFeedsFFmpeg 是这条遗留项的**端到端判据**：`data:audio/wav` 经
// `mediaSrcToPath` 落盘后，宿主真的能用 ffmpeg 探测到音轨与时长——A3 之前 data:
// 来源正是卡在「没有路径可交给 ffmpeg」，因此 16 格只能停在 L1。
// 本机没有 ffmpeg 时跳过（与 TestMediaProbeEndToEnd 同口径）。
func TestMediaDataURLFeedsFFmpeg(t *testing.T) {
	testGateAllowAll(t) // 端到端：走 mediaSrcToPath（引用需授权），data: 恒放行
	t.Cleanup(func() { CleanupMediaDataURLFiles() })
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机没有 ffmpeg：跳过 data: → ffmpeg 的端到端探测")
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "sine.wav")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", wav)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("生成样本失败：%v\n%s", err, out)
	}
	raw, err := os.ReadFile(wav)
	if err != nil {
		t.Fatalf("读样本失败：%v", err)
	}

	path, ok := mediaSrcToPath("data:audio/wav;base64,"+base64.StdEncoding.EncodeToString(raw), "")
	if !ok {
		t.Fatalf("data: 应能落盘并给出本地路径")
	}
	meta, ok := probeWithFFmpeg(ffmpeg, path)
	if !ok {
		t.Fatalf("落盘后的 data: 应能被 ffmpeg 探测")
	}
	if !meta.HasAudio {
		t.Fatalf("应探测到音轨，got HasAudio=false（meta=%+v）", meta)
	}
	if math.Abs(meta.Duration-1) > 0.05 {
		t.Errorf("duration = %v，want ≈1s", meta.Duration)
	}
}
