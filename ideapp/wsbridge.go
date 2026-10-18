// WebSocket 桥：把页面里的 window.WebSocket 换成「宿主转发」实现。
//
// 为什么需要：wb-ui 引擎不内置真实 WebSocket 传输（bindings/dom.go 里是一个
// 语法完整但 send 空操作的 stub），而 IDE 的两条长连接都在 WebSocket 上：
//
//	/ws                全局事件流（agent 事件 / 运行状态 / 心跳）
//	/api/terminal/ws   PTY 终端（二进制帧 = 原始 PTY I/O）
//
// 桥的分工：
//   - JS 侧（wsshim.go 注入的 shim）：实现浏览器 API 面（构造器、事件、send/close），
//     对页面完全透明——前端代码零改动；
//   - Go 侧（本文件）：RFC6455 客户端（wsclient.go）连后端，把消息排队，
//     由主线程（Host.OnFrame / headless pump）派发回 JS。
//
// ★ 线程模型：goja 非线程安全，任何跨 goroutine 的 RunJS 都必须经主线程。
// 所以读循环只往队列里塞「待执行的 JS 脚本字符串」，Flush 在主线程消费。
package ideapp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// wsDialTimeout 单条连接（含握手）的上限。
const wsDialTimeout = 10 * time.Second

// wsBridge 管理全部桥接连接。
type wsBridge struct {
	backendBase string // http://127.0.0.1:<port>
	wv          *webkit.WebView

	mu     sync.Mutex
	nextID int
	conns  map[int]*wsClient
	closed map[int]bool
	queue  []string // 待主线程执行的派发脚本
}

func newWSBridge(backendBase string) *wsBridge {
	return &wsBridge{
		backendBase: strings.TrimSuffix(backendBase, "/"),
		conns:       map[int]*wsClient{},
		closed:      map[int]bool{},
	}
}

// Install 把 Go 侧入口注册到 JS 全局，并记下 WebView（派发用）。
// 必须在 LoadHTML* 之前调用：页面脚本一执行就可能 new WebSocket。
func (b *wsBridge) Install(wv *webkit.WebView) {
	b.wv = wv
	rt := wv.JSInterpreter()
	bindings.RegisterGoFunction(rt, "__wbwsOpen", func(args []jsc.JSValue) (jsc.JSValue, error) {
		if len(args) < 2 {
			return jsc.Undefined(), nil
		}
		id := int(args[0].ToNumber())
		raw := args[1].ToString()
		b.open(id, raw)
		return jsc.Undefined(), nil
	})
	bindings.RegisterGoFunction(rt, "__wbwsSend", func(args []jsc.JSValue) (jsc.JSValue, error) {
		if len(args) < 2 {
			return jsc.Undefined(), nil
		}
		b.send(int(args[0].ToNumber()), args[1].ToString())
		return jsc.Undefined(), nil
	})
	bindings.RegisterGoFunction(rt, "__wbwsClose", func(args []jsc.JSValue) (jsc.JSValue, error) {
		if len(args) < 1 {
			return jsc.Undefined(), nil
		}
		b.close(int(args[0].ToNumber()))
		return jsc.Undefined(), nil
	})
}

// open 异步拨号；成功后派发 open，失败派发 error + close。
func (b *wsBridge) open(id int, rawURL string) {
	target := b.targetURL(rawURL)
	go func() {
		cli, err := dialWS(target, wsDialTimeout)
		if err != nil {
			log.Printf("[wsbridge] 连接失败 id=%d %s：%v", id, target, err)
			b.enqueue(jsDispatch(id, "error", err.Error()))
			b.enqueue(jsDispatch(id, "close", ""))
			return
		}
		b.mu.Lock()
		if b.closed[id] {
			b.mu.Unlock()
			_ = cli.Close()
			return
		}
		b.conns[id] = cli
		b.mu.Unlock()
		log.Printf("[wsbridge] 已连接 id=%d %s", id, target)
		b.enqueue(jsSetOpen(id))
		b.readLoop(id, cli)
	}()
}

// readLoop 把后端消息转成派发脚本；连接断开时派发 close 并清理。
func (b *wsBridge) readLoop(id int, cli *wsClient) {
	for {
		op, data, err := cli.ReadMessage()
		if err != nil {
			b.mu.Lock()
			delete(b.conns, id)
			b.mu.Unlock()
			_ = cli.Close()
			b.enqueue(jsDispatch(id, "close", ""))
			return
		}
		// 终端二进制帧（PTY 输出）与文本帧都按字符串交给 xterm / JSON 解析：
		// wb-ui 引擎没有 Blob，前端 TerminalPanel 已按「字符串即输出」处理。
		if op == wsOpText || op == wsOpBinary {
			b.enqueue(jsDispatch(id, "message", string(data)))
		}
	}
}

// send 发送一条文本帧（前端 send 的都是 JSON 控制消息或终端输入）。
func (b *wsBridge) send(id int, data string) {
	b.mu.Lock()
	cli := b.conns[id]
	b.mu.Unlock()
	if cli == nil {
		return
	}
	if err := cli.WriteText([]byte(data)); err != nil {
		log.Printf("[wsbridge] 发送失败 id=%d：%v", id, err)
	}
}

// close 主动关闭连接。
func (b *wsBridge) close(id int) {
	b.mu.Lock()
	b.closed[id] = true
	cli := b.conns[id]
	delete(b.conns, id)
	b.mu.Unlock()
	if cli != nil {
		_ = cli.Close()
	}
}

// Shutdown 关闭全部连接（窗口关闭 / 自检结束时调用）。
func (b *wsBridge) Shutdown() {
	b.mu.Lock()
	conns := b.conns
	b.conns = map[int]*wsClient{}
	b.mu.Unlock()
	for _, cli := range conns {
		_ = cli.Close()
	}
}

// Flush 在主线程消费待派发脚本。每帧调用一次。
// 单帧内限流，避免终端大段输出时一帧执行过多 EvalJS 卡住渲染。
func (b *wsBridge) Flush() {
	if b == nil || b.wv == nil {
		return
	}
	const maxPerFrame = 64
	b.mu.Lock()
	n := len(b.queue)
	if n > maxPerFrame {
		n = maxPerFrame
	}
	batch := append([]string(nil), b.queue[:n]...)
	b.queue = b.queue[n:]
	b.mu.Unlock()
	for _, script := range batch {
		if _, err := b.wv.EvalJS(script); err != nil {
			log.Printf("[wsbridge] 派发失败：%v", err)
		}
	}
}

// enqueue 追加一条待派发脚本（可从任意 goroutine 调用）。
func (b *wsBridge) enqueue(script string) {
	b.mu.Lock()
	b.queue = append(b.queue, script)
	b.mu.Unlock()
}

// targetURL 把页面里的 ws URL 映射到后端：
//
//	ws://ide/ws            → ws://127.0.0.1:<port>/ws
//	ws://ide/api/terminal/ws → ws://127.0.0.1:<port>/api/terminal/ws
func (b *wsBridge) targetURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return raw
	}
	base, err := url.Parse(b.backendBase)
	if err != nil {
		return raw
	}
	base.Scheme = "ws"
	base.Path = u.Path
	base.RawQuery = u.RawQuery
	return base.String()
}

// ─── 派发脚本构造（JSON 编码，避免引号/换行注入）───────────────

func jsSetOpen(id int) string {
	return fmt.Sprintf("window.__wbwsDispatch && window.__wbwsDispatch(%d, 'open', '')", id)
}

func jsDispatch(id int, kind, payload string) string {
	b, _ := json.Marshal(payload)
	return fmt.Sprintf("window.__wbwsDispatch && window.__wbwsDispatch(%d, %q, %s)", id, kind, string(b))
}
