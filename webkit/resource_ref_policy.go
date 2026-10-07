// 外部资源引用的**公共判定内核**（一份判定、两处执行）。
//
// 背景：`loadExternalResource`（mode.go，图片/样式/脚本的字节路径）与
// `MediaResourceAllowed`（media_resource_policy.go，媒体路径）此前各写一份判定
// ——判定基元与顺序同源，但**代码是两份**，各自演化有漂移风险。先拍板「挂账」，
// 后要求实施（docs/media-format-verification-plan.md §9.9「挂账登记」、§10 Q5
// 的选项 B「抽公共判定内核」）。
//
// 本文件把两处共用的规则收成唯一实现：
//
//  1. `resolverRefForms`：宿主 ResourceResolver 的**询问序列**（原样引用，可绝对化
//     时再问绝对 URL）——「问哪些形态」只有一份口径；
//  2. `classifyResourceRef`：引用类别（data: / 网络 / 外部）——「怎么分类」只有一份；
//  3. `WebView.resourceRefAllowed`：类别 + 策略 → 放行与否——「放行规则」只有一份。
//
// 两处执行各自保留**与自身职责相关**的部分，这不算「第二份判定」：
//   - loadExternalResource 还要取内容（resolver 的返回值、缓存、读盘 / fetch）；
//   - MediaResourceAllowed 只回答布尔，且媒体通道**没有网络通道**
//     （hasNetworkChannel=false）——这一差异是**能力边界**而非策略差异，因此作为
//     显式参数进入判定函数，而不是在两处各写一遍规则。
//
// ★ 验收口径（挂账登记规定）：本重构必须做到「探针逐格零变化」。

package webkit

import (
	"strings"

	"wb-ui/engine/dom"
)

// resourceRefKind 是引用在资源策略判定中的类别。
type resourceRefKind int

const (
	// refKindInline：`data:`（RFC 2397 内联内容）——自带内容、不经任何外部通道，
	// 与策略**解耦**，恒放行。
	refKindInline resourceRefKind = iota
	// refKindNetwork：`http(s)` / `blob:`——要取内容就得有网络通道。
	refKindNetwork
	// refKindExternal：`file://` / 相对路径——走文件系统通道。
	refKindExternal
)

// classifyResourceRef 判定引用的类别（唯一实现）。
func classifyResourceRef(ref string) resourceRefKind {
	low := strings.ToLower(strings.TrimSpace(ref))
	switch {
	case strings.HasPrefix(low, "data:"):
		return refKindInline
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"),
		strings.HasPrefix(low, "blob:"):
		return refKindNetwork
	}
	return refKindExternal
}

// IsPolicyFreeResourceRef 报告引用是否**与资源策略解耦**（自带内容、不经任何外部
// 通道）：目前只有 `data:`（RFC 2397 内联内容）。
//
// 本函数与 classifyResourceRef 是同一份分类实现（这里只把「是不是 inline」问出来），
// 宿主侧（app/mediaaccess.go 的 allowMediaRef）也用它——于是 data: 的放行规则不会
// 在宿主层出现第二份（`<img>` 的 data: 放行同样是这条规则）。
func IsPolicyFreeResourceRef(ref string) bool {
	return classifyResourceRef(ref) == refKindInline
}

// resolverRefForms 返回宿主 ResourceResolver 的**询问序列**（唯一实现）：先页面
// 写下的**原样**引用（宿主常用逻辑名 `app://theme.css`），可被文档 URL 绝对化且形态
// 不同时再问一次**绝对 URL** 形式（宿主也可能只认其中一种）。
//
// ★ 无文档 URL（LoadHTML 直出内容）时**不做**绝对化：dom.ResolveURL 在 base 为空时
// 原样返回 ref，因此这里天然只剩一种形态——与两侧既有行为一致（此前
// loadExternalResource 走 `abs != ref` 判断、MediaResourceAllowed 明写 `base != ""`，
// 两者等价）。
func (wv *WebView) resolverRefForms(ref string) []string {
	if ref == "" {
		return nil
	}
	forms := make([]string, 0, 2)
	forms = append(forms, ref)
	if wv != nil {
		if abs := dom.ResolveURL(wv.documentBaseURL(), ref); abs != ref && abs != "" {
			forms = append(forms, abs)
		}
	}
	return forms
}

// resourceRefAllowed 是**唯一的策略判定**（一份判定）：报告该引用在当前资源策略下
// 是否放行。hasNetworkChannel 表示调用方**是否有网络通道**——图片/样式/脚本有
// （fetchResource），媒体通道没有（引擎不代宿主联网，宿主也没有网络媒体解码通道）。
//
// 语义（与两侧既有行为逐条一致）：
//   - `data:`：与策略解耦，恒放行（resource_policy.go 的三条兼容原则之一）；
//   - `http(s)` / `blob:`：有网络通道时按策略（只有 AllowAll 放行）；**无**网络通道时
//     恒拒——这是**能力边界**，不是策略差异；
//   - `file://` / 相对路径：只有 AllowAll（allowsExternalURLs）放行。
//
// ★ 宿主 resolver 命中**不在**本函数内：resolver 通道先于策略（命中即放行），且取
// 内容方需要 resolver 的返回值——两处都先按 resolverRefForms 询问，未命中才落到这里。
func (wv *WebView) resourceRefAllowed(ref string, hasNetworkChannel bool) bool {
	if wv == nil {
		return false
	}
	switch classifyResourceRef(ref) {
	case refKindInline:
		return true
	case refKindNetwork:
		return hasNetworkChannel && wv.allowsExternalURLs()
	}
	return wv.allowsExternalURLs()
}
