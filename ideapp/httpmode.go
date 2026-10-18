// HTTP 装配（hybrid 模式）：页面与全部子资源走真实 HTTP，WebSocket 由宿主桥接。
//
// 为什么默认改成这条路线（2026-10 排查结论）：
//
//  1. 本地加载（app://ide/ + 拦截）虽然能装配页面，但文档 origin 为空串、
//     document.cookie 不可写、资源型 URL（<img src="/api/...">）不走 fetch 拦截
//     —— 这些差异今天恰好没被前端用到，但属于「随时会被踩中」的结构性缺口；
//  2. HTTP 装配下 origin/host/protocol 与浏览器完全一致，脚本/样式/图片/fetch
//     全部走引擎网络栈，后端新增任何静态/动态路由都不需要桌面壳再补拦截；
//  3. ★ 但 wb-ui 引擎的 WebSocket 是 stub（不拨号、send 空操作、异步假 open），
//     纯 -http 模式下终端与全局事件流都不可用 —— 所以本模式仍保留 wsBridge：
//     注入 shim 覆盖 window.WebSocket，把长连接交给宿主转发。
//
// 装配方式：从后端 GET / 取 index.html（与后端实际提供的页面完全一致，避免
// 磁盘产物与后端版本漂移），在 <head> 最前注入 WS shim，再以 base+"/" 作为
// 文档 URL 调用 LoadHTMLWithBaseURL —— 页面里所有相对/绝对引用都按
// http://127.0.0.1:<port>/ 解析，走真实网络。
package ideapp

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"wb-ui/webkit"
)

// indexFetchTimeout 拉取后端首页的上限。后端刚拉起时可能还在初始化，给足
// 余量；超时由调用方决定回退策略（LoadURL）。
const indexFetchTimeout = 15 * time.Second

// loadHTTPPage 以 HTTP 为文档来源装配页面，并注入 WebSocket shim。
// 调用方必须在此之前完成 wsb.Install(wv)（Go 侧入口要在页面脚本执行前可用）。
func loadHTTPPage(wv *webkit.WebView, base string) error {
	client := &http.Client{Timeout: indexFetchTimeout}
	resp, err := client.Get(base + "/")
	if err != nil {
		return fmt.Errorf("拉取后端首页失败（%s/）：%w", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("后端首页返回状态码 %d（%s/）", resp.StatusCode, base)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取后端首页失败：%w", err)
	}
	html := injectHeadScript(string(b), wsShimJS)
	if err := wv.LoadHTMLWithBaseURL(html, base+"/"); err != nil {
		return fmt.Errorf("HTTP 装配页面失败：%w", err)
	}
	return nil
}
