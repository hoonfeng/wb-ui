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
	"wb-ui/engine/js/jsc"
)

// resourceRefKind 是引用在资源策略判定中的类别。
type resourceRefKind int

const (
	// refKindInline：`data:`（RFC 2397 内联内容）——自带内容、不经任何外部通道，
	// 与策略**解耦**，恒放行。
	refKindInline resourceRefKind = iota
	// refKindBlob：`blob:`——页面用 URL.createObjectURL 生成的内存对象 URL，
	// 内容在本进程 blob 存储里（**不是**网络资源）。是否放行取决于登记状态，
	// 与资源策略解耦；调用方还需要有内容注入通道（见 resourceRefVerdict）。
	refKindBlob
	// refKindNetwork：`http(s)`——要取内容就得有网络通道。
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
	case strings.HasPrefix(low, "blob:"):
		return refKindBlob
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
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

// resourceRefVerdict 是判定内核的输出：引用类别 + 是否放行 + blob 登记状态。
//
// BlobKnown 只在 Kind==refKindBlob 时有意义——取内容方据此区分「未登记/已
// revoke」与「策略拒绝」，给出与浏览器一致的行为（unknown blob URL）。
type resourceRefVerdict struct {
	Kind      resourceRefKind
	Allowed   bool
	BlobKnown bool
}

// blobRegistered 报告 blob URL 是否仍登记在本进程 blob 存储里（唯一实现）。
func blobRegistered(ref string) bool {
	_, ok := jsc.LookupBlobURL(ref)
	return ok
}

// resourceRefVerdict 是**唯一的策略判定**（一份判定）：报告该引用在当前资源
// 策略下是否放行。hasContentChannel 表示调用方**是否有内容注入通道**——
// 图片/样式/脚本有（fetchResource 取回字节进引擎），媒体通道没有（引擎不代
// 宿主联网，宿主也没有网络/blob 媒体解码通道）。
//
// 语义（与两侧既有行为逐条一致）：
//   - `data:`：与策略解耦，恒放行（resource_policy.go 的三条兼容原则之一）；
//   - `blob:`：页面自产的内存内容。**已登记**（本进程 blob 存储命中）且调用方
//     有内容注入通道 → 放行，与资源策略解耦（DenyExternal 下同样放行）；
//     未登记/已 revoke → 恒拒；没有内容注入通道（媒体通道）→ 恒拒，这是
//     **能力边界**而非策略差异（见 media_resource_policy.go）；
//   - `http(s)`：有内容通道时按策略（只有 AllowAll 放行）；**无**通道时恒拒；
//   - `file://` / 相对路径：只有 AllowAll（allowsExternalURLs）放行。
//
// ★ 宿主 resolver 命中**不在**本函数内：resolver 通道先于策略（命中即放行），且取
// 内容方需要 resolver 的返回值——两处都先按 resolverRefForms 询问，未命中才落到这里。
func (wv *WebView) resourceRefVerdict(ref string, hasContentChannel bool) resourceRefVerdict {
	kind := classifyResourceRef(ref)
	v := resourceRefVerdict{Kind: kind}
	if wv == nil {
		return v
	}
	switch kind {
	case refKindInline:
		v.Allowed = true
	case refKindBlob:
		v.BlobKnown = blobRegistered(ref)
		// ★ 与策略解耦：blob 是页面自产内容，DenyExternal 下也不该拦；
		//   唯一前提是「调用方有内容注入通道」+「blob 仍登记」。
		v.Allowed = v.BlobKnown && hasContentChannel
	case refKindNetwork:
		v.Allowed = hasContentChannel && wv.allowsExternalURLs()
	default:
		v.Allowed = wv.allowsExternalURLs()
	}
	return v
}

// resourceRefAllowed 是 resourceRefVerdict 的布尔投影（调用方只关心放行与否）。
func (wv *WebView) resourceRefAllowed(ref string, hasContentChannel bool) bool {
	return wv.resourceRefVerdict(ref, hasContentChannel).Allowed
}
