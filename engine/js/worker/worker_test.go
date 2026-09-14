// Tests for the worker package：脚本来源（内联 / data: / Fetcher）、错误上报、
// 消息往返与 worker 内定时器（事件循环由 run 的 ticker 驱动）。

package worker

import (
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

func TestDecodeDataURL(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte(`self.x = 1;`))
	if got, err := decodeDataURL("data:text/javascript;base64," + b64); err != nil || got != `self.x = 1;` {
		t.Fatalf("base64 data URL 解码错误: %q %v", got, err)
	}
	plain := "data:text/javascript," + url.PathEscape(`self.y = "2";`)
	if got, err := decodeDataURL(plain); err != nil || got != `self.y = "2";` {
		t.Fatalf("百分号编码 data URL 解码错误: %q %v", got, err)
	}
	if _, err := decodeDataURL("data:text/javascript;base64,!!!not-base64!!!"); err == nil {
		t.Fatal("无效 base64 应报错")
	}
	if _, err := decodeDataURL("data:text/javascript"); err == nil {
		t.Fatal("缺少逗号的 data URL 应报错")
	}
}

// TestWorkerInlineScriptRoundTrip 覆盖 worker 的 Go 侧 API（不经 bindings）：
// 内联脚本执行 + 主线程 → worker → 主线程 的往返。
func TestWorkerInlineScriptRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var got []string
	w := New(Options{
		Script:    `self.onmessage = function (e) { self.postMessage({ echo: e.data, ok: true }); };`,
		OnMessage: func(d string) { mu.Lock(); got = append(got, d); mu.Unlock() },
	})
	w.Start()
	defer w.Terminate()

	w.PostMessage(`{"n":1}`)
	if !waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) > 0 }, 3*time.Second) {
		t.Fatal("worker 未回消息")
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if !strings.Contains(first, `"n":1`) || !strings.Contains(first, `"ok":true`) {
		t.Fatalf("回信内容错误: %s", first)
	}

	w.Terminate()
	select {
	case <-w.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Terminate 后 worker goroutine 未退出")
	}
}

// TestWorkerFetcherLoadsScript 覆盖宿主注入的 Fetcher 路径。
func TestWorkerFetcherLoadsScript(t *testing.T) {
	var mu sync.Mutex
	var got []string
	var asked []string
	w := New(Options{
		URL: "app://worker.js",
		Fetcher: func(u string) (string, error) {
			asked = append(asked, u)
			return `importScripts("data:text/javascript," + encodeURIComponent("self.__v = 7;")); self.onmessage = function () { self.postMessage(self.__v); };`, nil
		},
		OnMessage: func(d string) { mu.Lock(); got = append(got, d); mu.Unlock() },
	})
	w.Start()
	defer w.Terminate()

	w.PostMessage("go")
	if !waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) > 0 }, 3*time.Second) {
		t.Fatal("注入 Fetcher 后 worker 未回消息")
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if first != "7" {
		t.Fatalf("importScripts 的结果应为 7，实际 %s", first)
	}
	if len(asked) != 1 || asked[0] != "app://worker.js" {
		t.Fatalf("Fetcher 收到的 URL 不对: %v", asked)
	}
}

// TestWorkerErrorReporting 覆盖两条错误路径：脚本来源不可用、脚本语法错误。
func TestWorkerErrorReporting(t *testing.T) {
	var mu sync.Mutex
	var errs []string
	onErr := func(name, msg string) {
		mu.Lock()
		errs = append(errs, name+"|"+msg)
		mu.Unlock()
	}

	// 1) 非 data: URL 且宿主未注入 Fetcher。
	w1 := New(Options{URL: "https://example.test/w.js", OnError: onErr})
	w1.Start()
	defer w1.Terminate()
	if !waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(errs) == 1 }, 3*time.Second) {
		t.Fatal("缺少 Fetcher 时未上报错误")
	}
	mu.Lock()
	first := errs[0]
	mu.Unlock()
	if !strings.HasPrefix(first, "NetworkError|") || !strings.Contains(first, "no fetcher") {
		t.Fatalf("错误内容错误: %s", first)
	}

	// 2) 语法错误脚本。
	w2 := New(Options{Script: "this is (not valid", OnError: onErr})
	w2.Start()
	defer w2.Terminate()
	if !waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(errs) == 2 }, 3*time.Second) {
		t.Fatal("语法错误未上报")
	}
	mu.Lock()
	second := errs[1]
	mu.Unlock()
	if !strings.HasPrefix(second, "Error|") {
		t.Fatalf("语法错误应以 Error 上报: %s", second)
	}
}

// TestWorkerTimerInsideWorker 覆盖 worker 内 setTimeout：依赖 run 的 ticker 驱动
// worker 自己的事件循环（不驱动则永远不触发）。
func TestWorkerTimerInsideWorker(t *testing.T) {
	var mu sync.Mutex
	var got []string
	w := New(Options{
		Script:    `setTimeout(function () { self.postMessage("tick"); }, 10);`,
		OnMessage: func(d string) { mu.Lock(); got = append(got, d); mu.Unlock() },
	})
	w.Start()
	defer w.Terminate()

	if !waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) > 0 }, 3*time.Second) {
		t.Fatal("worker 内 setTimeout 未触发（事件循环未被驱动）")
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if first != `"tick"` {
		t.Fatalf("回信内容错误: %s", first)
	}
}

// TestWorkerCloseStopsRunLoop 覆盖 worker 内 close()：run 循环退出，Done 关闭。
func TestWorkerCloseStopsRunLoop(t *testing.T) {
	w := New(Options{Script: `close();`})
	w.Start()
	select {
	case <-w.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("close() 后 worker 未退出")
	}
}
