// Command browser_api_probe 是本轮「引擎侧浏览器 API 补齐」的端到端探针。
//
// 覆盖的判据（全部在 ModeBrowser 的真实 HTTP 栈里跑）：
//   - 全局对象能力面：Headers / Request / Response / fetch / XHR / Image / File /
//     FormData / EventSource / window.open 是否存在且类型正确；
//   - Headers：构造 + append + 大小写不敏感 get + has + 实例原型方法齐全；
//   - Response：new Response(body, {status, headers}) 可用、instanceof 成立、
//     headers.get 可取到自定义头；
//   - fetch('/api/health')：返回值是**真 Response 实例**（instanceof Response），
//     headers.get('content-type') 可用，正文能读出；
//   - XHR：getResponseHeader / getAllResponseHeaders 返回真实响应头；
//   - Image：data: URL 与**相对 URL 的 HTTP 图**都能 load（后者是本轮要查的
//     已知缺陷：相对 URL 的 http 图曾 load=false / error=true）；
//   - window.open：不抛异常，返回 Window 或 null。
//
//	go run ./dev/probes/browser_api_probe
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"wb-ui/webkit"
)

const pageHTML = `<!DOCTYPE html>
<html><head><title>Browser API Probe</title></head>
<body><div id="host" style="width:64px;height:64px"></div></body></html>`

const pngB64 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// probeScript 在页面里安装探针，全部异步项结算时累加 window.__p.done。
// expect 是异步项总数（fetch / xhr / 3 张图）。
const probeScript = `(function () {
  var P = { done: 0, expect: 5, timeout: false, miss: [], typeofs: {}, headers: {},
            respCtor: {}, resp: {}, xhr: {}, open: null, img: [] };
  window.__p = P;
  function fin() { P.done++; }
  var G = ['fetch', 'XMLHttpRequest', 'Headers', 'Request', 'Response', 'Image', 'Blob',
           'File', 'FileReader', 'FormData', 'EventSource', 'URL', 'localStorage',
           'sessionStorage', 'navigator', 'open'];
  for (var i = 0; i < G.length; i++) {
    var n = G[i], t = typeof window[n];
    if (t === 'undefined') P.miss.push(n); else P.typeofs[n] = t;
  }
  // ── Headers 构造 + 方法 ──
  try {
    var h = new Headers({ 'Content-Type': 'application/json', 'X-Multi': 'a' });
    h.append('X-Multi', 'b');
    var missing = [];
    ['append', 'delete', 'get', 'has', 'set', 'forEach', 'entries', 'keys', 'values']
      .forEach(function (m) { if (typeof h[m] !== 'function') missing.push(m); });
    P.headers = {
      isInstance: (typeof Headers === 'function') ? (h instanceof Headers) : 'no-ctor',
      get: typeof h.get === 'function' ? h.get('content-type') : 'no-get',
      getCI: typeof h.get === 'function' ? h.get('Content-Type') : 'no-get',
      has: typeof h.has === 'function' ? h.has('x-multi') : 'no-has',
      multi: typeof h.get === 'function' ? String(h.get('x-multi')) : 'no-get',
      missing: missing
    };
  } catch (e) { P.headers.err = String(e && e.message || e); }
  // ── Response 构造器 ──
  try {
    var rc = new Response('hello', { status: 201, headers: { 'X-A': '1' } });
    P.respCtor = {
      inst: rc instanceof Response,
      status: rc.status,
      hget: (rc.headers && typeof rc.headers.get === 'function') ? rc.headers.get('X-A') : 'no-get',
      textFn: typeof rc.text
    };
  } catch (e) { P.respCtor.err = String(e && e.message || e); }
  // ── window.open ──
  try {
    var ow = window.open('/api/health', '_blank');
    P.open = { ret: ow === null ? 'null' : (ow === undefined ? 'undefined' : typeof ow) };
    if (ow) { P.open.closed = !!ow.closed; }
  } catch (e) { P.open = { err: String(e && e.message || e) }; }
  // ── fetch → Response 实例 ──
  try {
    fetch('/api/health').then(function (r) {
      P.resp.isResponse = (typeof Response === 'function') ? (r instanceof Response) : 'no-ctor';
      P.resp.instanceofHeaders = (typeof Headers === 'function' && r.headers) ? (r.headers instanceof Headers) : 'no-headers';
      P.resp.status = r.status;
      P.resp.ct = (r.headers && typeof r.headers.get === 'function') ? r.headers.get('content-type') : 'no-get';
      P.resp.hasCT = (r.headers && typeof r.headers.has === 'function') ? r.headers.has('Content-Type') : 'no-has';
      return r.text();
    }).then(function (t) { P.resp.body = String(t).slice(0, 40); fin(); })
      .catch(function (e) { P.resp.err = String(e && e.message || e); fin(); });
  } catch (e) { P.resp.err0 = String(e && e.message || e); fin(); }
  // ── XHR 响应头 ──
  try {
    var x = new XMLHttpRequest();
    x.open('GET', '/api/health', true);
    x.onload = function () {
      P.xhr.status = x.status;
      try { P.xhr.ct = x.getResponseHeader('content-type'); } catch (e) { P.xhr.ctErr = String(e && e.message || e); }
      try { P.xhr.all = String(x.getAllResponseHeaders()).slice(0, 60); } catch (e) { P.xhr.allErr = String(e && e.message || e); }
      fin();
    };
    x.onerror = function () { P.xhr.error = true; fin(); };
    x.send();
  } catch (e) { P.xhr.err0 = String(e && e.message || e); fin(); }
  // ── Image：data / 相对 URL 的 200 图 / 404 ──
  function probeImg(label, src) {
    var rec = { label: label, load: false, error: false, complete: null, nw: null, url: String(src).slice(0, 48) };
    var im = document.createElement('img');
    im.onload = function () { rec.load = true; rec.complete = im.complete; rec.nw = im.naturalWidth; fin(); };
    im.onerror = function () { rec.error = true; fin(); };
    im.src = src;
    document.body.appendChild(im);
    P.img.push(rec);
  }
  probeImg('data', '` + pngB64 + `');
  probeImg('ok-http', '/api/fs/image?path=probe');
  probeImg('404', '/api/definitely-missing.png');
  setTimeout(function () { P.timeout = true; P.done = 99; }, 5000);
  return 'installed';
})()`

type reqLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *reqLog) add(p string) {
	l.mu.Lock()
	l.seen = append(l.seen, p)
	l.mu.Unlock()
}

func (l *reqLog) dump() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.seen, " ")
}

var failures int

func check(ok bool, format string, args ...any) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
		failures++
	}
	fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(format, args...))
}

func evalStr(wv *webkit.WebView, script string) string {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

// mkPNG 生成 4x4 纯绿 PNG：探针只验证「加载成功」，颜色留给别的探针。
func mkPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{G: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func main() {
	log := &reqLog{}
	pngBytes := mkPNG()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/api/fs/image", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pageURL := srv.URL + "/index.html"
	fmt.Printf("本地 HTTP 服务器：%s\n", srv.URL)

	wv := webkit.NewWebViewWithMode(webkit.ModeBrowser)
	defer wv.Destroy()
	wv.Resize(800, 600)
	if err := wv.LoadURL(pageURL); err != nil {
		fmt.Printf("LoadURL(%s) 失败: %v\n", pageURL, err)
		os.Exit(1)
	}
	fmt.Printf("document.URL = %s\n", evalStr(wv, "document.URL"))
	if got := evalStr(wv, `document.title`); got != "Browser API Probe" {
		fmt.Printf("标题不符：%q\n", got)
	}
	if got := evalStr(wv, probeScript); got != "installed" {
		fmt.Printf("探针安装失败：%q\n", got)
		os.Exit(1)
	}

	// 驱动渲染 + 事件循环，直到所有异步项结算（或超时兜底）。
	var raw string
	for i := 0; i < 400; i++ {
		_, _ = wv.Render()
		if el := wv.JSInterpreter().EnsureEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		raw = evalStr(wv, "JSON.stringify(window.__p)")
		if strings.Contains(raw, `"done":`) {
			var done int
			fmt.Sscanf(raw[strings.Index(raw, `"done":`)+len(`"done":`):], "%d", &done)
			if done >= 5 || strings.Contains(raw, `"timeout":true`) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	fmt.Printf("\n探针输出：\n%s\n\n", raw)
	fmt.Println("── 断言 ──")

	var p map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		fmt.Printf("探针输出不是合法 JSON：%v\n", err)
		os.Exit(1)
	}
	str := func(m map[string]interface{}, k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return fmt.Sprintf("%v", m[k])
	}
	obj := func(m map[string]interface{}, k string) map[string]interface{} {
		if v, ok := m[k].(map[string]interface{}); ok {
			return v
		}
		return map[string]interface{}{}
	}
	arr := func(m map[string]interface{}, k string) []interface{} {
		if v, ok := m[k].([]interface{}); ok {
			return v
		}
		return nil
	}
	pTypeofs := obj(p, "typeofs")
	pHeaders := obj(p, "headers")
	pRespCtor := obj(p, "respCtor")
	pResp := obj(p, "resp")
	pXHR := obj(p, "xhr")
	pOpen := p["open"]

	// 能力面
	miss := arr(p, "miss")
	check(len(miss) == 0, "全局对象齐全（缺失：%v）", miss)
	for _, n := range []string{"Headers", "Request", "Response", "fetch", "XMLHttpRequest", "Image", "File", "FormData", "EventSource"} {
		check(str(pTypeofs, n) == "function", "typeof %s === function（实际 %q）", n, str(pTypeofs, n))
	}

	// Headers
	check(pHeaders["isInstance"] == true, "new Headers(...) instanceof Headers（实际 %v）", pHeaders["isInstance"])
	check(str(pHeaders, "get") == "application/json", "Headers.get('content-type') 大小写不敏感（实际 %q）", str(pHeaders, "get"))
	check(len(arr(pHeaders, "missing")) == 0, "Headers 原型方法齐全（缺失 %v）", arr(pHeaders, "missing"))
	check(pHeaders["has"] == true, "Headers.has('x-multi') 为 true（实际 %v）", pHeaders["has"])
	check(str(pHeaders, "multi") == "a, b", "Headers.get 返回多值拼接 \"a, b\"（实际 %q）", str(pHeaders, "multi"))

	// Response 构造器
	check(pRespCtor["inst"] == true, "new Response(...) instanceof Response（实际 %v）", pRespCtor["inst"])
	check(str(pRespCtor, "status") == "201", "new Response(body,{status:201}).status === 201（实际 %v）", pRespCtor["status"])
	check(str(pRespCtor, "hget") == "1", "new Response 的自定义响应头可通过 headers.get 取到（实际 %q）", str(pRespCtor, "hget"))

	// window.open
	if om, ok := pOpen.(map[string]interface{}); ok {
		_, hasErr := om["err"]
		check(!hasErr, "window.open 不抛异常（返回 %v）", om)
	} else {
		check(false, "window.open 探针缺失（%v）", pOpen)
	}

	// fetch
	check(pResp["isResponse"] == true, "fetch() 返回 Response 实例（实际 %v）", pResp["isResponse"])
	check(pResp["instanceofHeaders"] == true, "fetch().headers instanceof Headers（实际 %v）", pResp["instanceofHeaders"])
	check(str(pResp, "ct") == "application/json; charset=utf-8", "fetch().headers.get('content-type') 正确（实际 %q）", str(pResp, "ct"))
	check(pResp["hasCT"] == true, "fetch().headers.has('Content-Type') 大小写不敏感（实际 %v）", pResp["hasCT"])
	check(str(pResp, "body") == `{"ok":true}`, "fetch().text() 读到正文（实际 %q）", str(pResp, "body"))
	if e, ok := pResp["err"]; ok {
		check(false, "fetch() 链上抛错：%v", e)
	}

	// XHR
	check(str(pXHR, "ct") == "application/json; charset=utf-8", "XHR getResponseHeader('content-type') 正确（实际 %q）", str(pXHR, "ct"))
	_, ctErr := pXHR["ctErr"]
	_, allErr := pXHR["allErr"]
	check(!ctErr, "XHR getResponseHeader 不抛异常")
	check(!allErr, "XHR getAllResponseHeaders 不抛异常")

	// Image
	imgByLabel := map[string]map[string]interface{}{}
	for _, it := range arr(p, "img") {
		if m, ok := it.(map[string]interface{}); ok {
			imgByLabel[str(m, "label")] = m
		}
	}
	if im, ok := imgByLabel["data"]; ok {
		check(im["load"] == true, "Image data: URL 触发 load（实际 %v）", im)
	} else {
		check(false, "Image data: URL 探针缺失")
	}
	if im, ok := imgByLabel["ok-http"]; ok {
		check(im["load"] == true, "Image 相对 URL 的 200 图触发 load（实际 %v）", im)
		check(im["error"] != true, "Image 相对 URL 的 200 图没有落到 error")
	} else {
		check(false, "Image 相对 URL 探针缺失")
	}
	if im, ok := imgByLabel["404"]; ok {
		check(im["error"] == true, "Image 404 触发 error（实际 %v）", im)
	} else {
		check(false, "Image 404 探针缺失")
	}

	fmt.Printf("\n服务器收到请求：%s\n", log.dump())
	fmt.Printf("\n结论：%d 项失败\n", failures)
	if failures > 0 {
		os.Exit(1)
	}
}

// extractArr 从 JSON 文本里取数组字段（只用于诊断输出）。
func extractArr(s, key string) []string {
	i := strings.Index(s, key)
	if i < 0 {
		return nil
	}
	rest := s[i+len(key):]
	j := strings.Index(rest, "]")
	if j < 0 {
		return nil
	}
	inner := strings.TrimSpace(rest[:j])
	if inner == "" {
		return nil
	}
	parts := strings.Split(inner, `","`)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.Trim(p, `"`))
	}
	return out
}
