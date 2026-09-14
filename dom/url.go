package dom

import "net/url"

// ResolveURL 把相对引用 ref 以 base（通常是文档 URL）为基准解析为绝对
// URL——对应浏览器里 document.baseURI / node.baseURI 的解析语义
// （WebKit 的 completeURL）。
//
// 原样返回 ref 的情形（无基准可解析时由调用方决定后续处理：忽略、
// 报错或按既有规则读取）：
//   - ref 为空
//   - ref 已带 scheme（http:/https:/data:/file:/app: …）——绝对引用与
//     宿主自定义 scheme（逻辑资源名）不参与解析
//   - base 为空或不是绝对 URL
//   - 任一侧 URL 解析失败
//
// 协议相对引用（"//host/x"）按浏览器语义补上 base 的 scheme。
func ResolveURL(base, ref string) string {
	if ref == "" || base == "" {
		return ref
	}
	if u, err := url.Parse(ref); err == nil && u.IsAbs() {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil || !b.IsAbs() {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
