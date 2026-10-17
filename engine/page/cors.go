package page

// CORS（跨源资源共享）对齐。
//
// 浏览器的 fetch/XHR 对**跨源**请求强制 CORS：请求带 Origin，响应必须带
// 匹配的 Access-Control-Allow-Origin（非简单请求还要先 OPTIONS 预检），
// 否则调用方拿到的是一个网络错误（fetch reject / XHR error），内容永远
// 不可读。引擎此前直接发请求、把跨源响应原样交给页面——同源限制缺失，
// 页面能读到本不该读的跨源响应（与浏览器行为不一致，也让依赖 CORS 的
// 前端库的失败分支永远不触发）。
//
// 本文件的边界（与浏览器一致的部分 + 明确的已知差异）：
//   - 同源（scheme+host+port 全等）→ 不校验；
//   - 跨源 + 简单请求 → 带 Origin 直接发，校验响应 ACAO/ACAC；
//   - 跨源 + 非简单请求 → 先 OPTIONS 预检（ACAM/ACAH/ACAO/ACAC），通过后
//     才发实际请求；
//   - credentials: "include"（XHR withCredentials）→ ACAO 不能是 `*`，且
//     必须 ACAC: true；
//   - mode: "no-cors" → 只允许简单请求，返回 opaque 响应（status 0、body
//     空、headers 空、type "opaque"），不校验 ACAO；
//   - mode: "same-origin" → 跨源请求直接失败。
//
// 已知差异（记录，不阻塞）：
//   - 文档源为 file:/data:/about:blank（不透明源）时**不做** CORS 校验：
//     这类文档是宿主提供的本地上下文（引擎 LoadHTML/本地 UI 页面的常规
//     用法），严格按浏览器语义会让「file 页面 + 本地 HTTP 服务」全部失效。
//     http(s) 文档之间的跨源请求则完全按浏览器语义校验。
//   - 引擎不维护 cookie jar：credentials 只影响 CORS 头的校验强度，不带
//     真实 cookie（宿主可经 bridge 自行注入鉴权头）。

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// corsMode 是 fetch 的 Request.mode 子集。
type corsMode string

const (
	corsModeCORS       corsMode = "cors"
	corsModeNoCORS     corsMode = "no-cors"
	corsModeSameOrigin corsMode = "same-origin"
)

// corsErr 是 CORS 失败（调用方转成 fetch reject / XHR error）。
type corsErr struct{ msg string }

func (e *corsErr) Error() string { return "CORS: " + e.msg }

func corsFail(format string, args ...any) error {
	return &corsErr{msg: fmt.Sprintf(format, args...)}
}

// corsRequest 描述一次请求的 CORS 上下文。
type corsRequest struct {
	docOrigin    string // 文档源（规范化）；空 = 不透明源/无基准，不校验
	targetURL    string
	targetOrigin string
	method       string
	headers      http.Header
	credentials  bool
	mode         corsMode
}

// canonicalOrigin 规范化 http(s) URL 的源：小写 scheme/host，去掉默认端口。
// 非 http(s)（file:/data:/blob:/空）返回 ok=false（浏览器里是不透明源）。
func canonicalOrigin(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, true
}

// newCORSRequest 构造 CORS 上下文。docURL 为空或非 http(s) 时 docOrigin
// 为空——调用方据此跳过校验（见文件头「已知差异」）。
func newCORSRequest(docURL, targetURL, method string, headers http.Header, credentials bool, mode corsMode) *corsRequest {
	docOrigin, _ := canonicalOrigin(docURL)
	targetOrigin, _ := canonicalOrigin(targetURL)
	if method == "" {
		method = "GET"
	}
	return &corsRequest{
		docOrigin:    docOrigin,
		targetURL:    targetURL,
		targetOrigin: targetOrigin,
		method:       strings.ToUpper(method),
		headers:      headers,
		credentials:  credentials,
		mode:         mode,
	}
}

// needsCheck 报告这次请求是否要按 CORS 校验。
func (c *corsRequest) needsCheck() bool {
	if c == nil || c.docOrigin == "" || c.targetOrigin == "" {
		return false
	}
	return c.docOrigin != c.targetOrigin
}

// originHeader 是跨源请求要带的 Origin 值。
func (c *corsRequest) originHeader() string {
	if c == nil || c.docOrigin == "" {
		return ""
	}
	return c.docOrigin
}

// corsSafelistedHeaders 是 CORS 简单请求允许的请求头（Fetch 标准子集）。
var corsSafelistedHeaders = map[string]bool{
	"accept":           true,
	"accept-language":  true,
	"content-language": true,
	"content-type":     true,
	"range":            true,
}

// corsSafelistedContentType 判断 Content-Type 是否为安全值。
func corsSafelistedContentType(v string) bool {
	if i := strings.IndexByte(v, ';'); i >= 0 {
		v = v[:i]
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "application/x-www-form-urlencoded", "multipart/form-data", "text/plain":
		return true
	}
	return false
}

// nonSafelistedHeaderNames 返回需要预检的请求头名（小写、排序、逗号分隔）。
func (c *corsRequest) nonSafelistedHeaderNames() string {
	var names []string
	seen := map[string]bool{}
	for name, values := range c.headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" || lower == "origin" {
			continue
		}
		safe := corsSafelistedHeaders[lower]
		if safe && lower == "content-type" {
			safe = len(values) > 0 && corsSafelistedContentType(values[0])
		}
		if safe || seen[lower] {
			continue
		}
		seen[lower] = true
		names = append(names, lower)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// isSimple 报告该请求是否属于 CORS 简单请求（无需预检）。
func (c *corsRequest) isSimple() bool {
	switch c.method {
	case "GET", "HEAD", "POST":
	default:
		return false
	}
	return c.nonSafelistedHeaderNames() == ""
}

// headerTokenSet 把响应头按逗号拆成 token 集合（小写、trim、去空）。
//
// CORS 的 Access-Control-Allow-Methods / Allow-Headers 都是 **token 列表**，
// 必须逐 token 精确比较：子串匹配会把 `POSTX` 当作允许 `POST`、把
// `content-type-evil` 当作允许 `content-type`——安全边界直接失效。
func headerTokenSet(h http.Header, name string) map[string]bool {
	tokens := map[string]bool{}
	for _, v := range h.Values(name) {
		for _, tok := range strings.Split(v, ",") {
			tok = strings.ToLower(strings.TrimSpace(tok))
			if tok != "" {
				tokens[tok] = true
			}
		}
	}
	return tokens
}

// validatePreflightResponse 校验 OPTIONS 预检响应。
func (c *corsRequest) validatePreflightResponse(resp *http.Response) error {
	if err := c.validateResponseHeaders(resp.Header); err != nil {
		return err
	}
	// ★ 逐 token 精确匹配（不区分大小写），禁止子串匹配。
	allowMethods := headerTokenSet(resp.Header, "Access-Control-Allow-Methods")
	if !allowMethods[strings.ToLower(c.method)] {
		return corsFail("preflight: Access-Control-Allow-Methods 不含 %s（收到 %q）",
			c.method, strings.Join(resp.Header.Values("Access-Control-Allow-Methods"), ","))
	}
	allowHeaders := headerTokenSet(resp.Header, "Access-Control-Allow-Headers")
	// `*` 作为 ACAH 的通配（Fetch 标准：非 credentialed 预检可用 * 表示任意头）。
	wildcard := allowHeaders["*"]
	for _, name := range strings.Split(c.nonSafelistedHeaderNames(), ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if wildcard || allowHeaders[name] {
			continue
		}
		return corsFail("preflight: Access-Control-Allow-Headers 不含 %q（收到 %q）",
			name, strings.Join(resp.Header.Values("Access-Control-Allow-Headers"), ","))
	}
	return nil
}

// validateResponseHeaders 校验实际请求/预检响应共有的 CORS 头。
func (c *corsRequest) validateResponseHeaders(h http.Header) error {
	acao := strings.TrimSpace(strings.Join(h.Values("Access-Control-Allow-Origin"), ","))
	if acao == "" {
		return corsFail("响应缺少 Access-Control-Allow-Origin（跨源请求 %s）", c.targetURL)
	}
	if c.credentials {
		if acao == "*" {
			return corsFail("credentials 模式下 Access-Control-Allow-Origin 不能是 *")
		}
		// ★ credentials 模式必须**精确**等于文档源：既不能是 *，也不能是任何
		//   其他源（浏览器语义）。只查 ACAC 而不查源等于放行任意第三方。
		if !strings.EqualFold(acao, c.docOrigin) {
			return corsFail("credentials 模式下 Access-Control-Allow-Origin=%q 必须精确等于文档源 %q",
				acao, c.docOrigin)
		}
		if !strings.EqualFold(strings.TrimSpace(h.Get("Access-Control-Allow-Credentials")), "true") {
			return corsFail("credentials 模式缺少 Access-Control-Allow-Credentials: true")
		}
	} else if acao != "*" && !strings.EqualFold(acao, c.docOrigin) {
		return corsFail("Access-Control-Allow-Origin=%q 与文档源 %q 不匹配", acao, c.docOrigin)
	}
	return nil
}

// prepare 在发送实际请求前调用：处理 same-origin/no-cors 模式并返回
// opaque 标记（true = 调用方应构造 opaque 响应，不读取真实响应体）。
func (c *corsRequest) prepare(client *http.Client) (opaque bool, err error) {
	if !c.needsCheck() {
		return false, nil
	}
	switch c.mode {
	case corsModeSameOrigin:
		return false, corsFail("mode=same-origin 不允许跨源请求 %s", c.targetURL)
	case corsModeNoCORS:
		if !c.isSimple() {
			return false, corsFail("mode=no-cors 只允许简单请求（%s %s）", c.method, c.targetURL)
		}
		return true, nil
	}
	if c.isSimple() {
		return false, nil
	}
	// 非简单请求：OPTIONS 预检。
	req, err := http.NewRequest(http.MethodOptions, c.targetURL, nil)
	if err != nil {
		return false, corsFail("preflight 请求构造失败: %v", err)
	}
	req.Header.Set("Origin", c.originHeader())
	req.Header.Set("Access-Control-Request-Method", c.method)
	if names := c.nonSafelistedHeaderNames(); names != "" {
		req.Header.Set("Access-Control-Request-Headers", names)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, corsFail("preflight 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, corsFail("preflight 返回 %d", resp.StatusCode)
	}
	if err := c.validatePreflightResponse(resp); err != nil {
		return false, err
	}
	return false, nil
}

// validateFinal 校验实际请求的响应头。
func (c *corsRequest) validateFinal(resp *http.Response) error {
	if !c.needsCheck() {
		return nil
	}
	return c.validateResponseHeaders(resp.Header)
}

// applyOriginHeader 给跨源请求设置 Origin（脚本不能自己设置该头，浏览器
// 总是覆盖）。同源请求不设置。
func (c *corsRequest) applyOriginHeader(req *http.Request) {
	if c.needsCheck() {
		req.Header.Set("Origin", c.originHeader())
	}
}
