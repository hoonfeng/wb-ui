// 资源策略（ResourcePolicy）：把「UI 库模式一律拒外部资源」的二元门禁
// 细化为宿主可声明的三档策略（决策 1 修法 (c)，见
// docs/media-format-verification-plan.md §8.2 阶段 1 / §5 G9）。
//
// 三条兼容原则（文档 §8.2 阶段 1）：
//  1. data: 与开关**解耦**——data: 自带内容、不经任何外部通道，两种模式
//     都无条件放行（属 bug 修复，不属行为放宽）；
//  2. 默认值保持历史行为——未显式调用 SetResourcePolicy 时按模式推导：
//     ModeBrowser → AllowAll、ModeToolkit → DenyExternal（与改动前完全
//     一致，唯一差异是 data: 由 ❌ 转 ✅）；
//  3. 需要读盘的宿主必须**显式**声明 AllowHostResolved，安全默认不被静默
//     放宽。
//
// 与 SetMode 的差别：本开关**不受装配锁定**——策略在每次资源引用时求值，
// WebView 构造后随时可切换（TC-M-907）；切换时清空资源缓存，避免旧策略下
// 取回的内容穿透新策略。

package webkit

import "fmt"

// ResourcePolicy 是宿主对「外部资源引用」（http(s)/file/相对路径）的放行策略。
type ResourcePolicy int

const (
	// DenyExternal（默认，ModeToolkit）：拒 http(s)/file/相对路径；data:
	// 无条件放行。安全默认：不读盘、不联网。
	DenyExternal ResourcePolicy = iota

	// AllowHostResolved：只放行宿主 ResourceResolver **明确解析出**的资源；
	// 网络与文件系统通道本身仍关闭（引擎不代宿主联网）。需要读本地图片的
	// UI 库宿主用这一档（改动一行）。
	AllowHostResolved

	// AllowAll：等价 ModeBrowser（默认，ModeBrowser）——http(s)/file/
	// 相对路径/data: 全放行。
	AllowAll
)

// String 返回策略的稳定名称（用于日志、报告与自检输出）。
func (p ResourcePolicy) String() string {
	switch p {
	case DenyExternal:
		return "deny-external"
	case AllowHostResolved:
		return "allow-host-resolved"
	case AllowAll:
		return "allow-all"
	}
	return fmt.Sprintf("ResourcePolicy(%d)", int(p))
}

// SetResourcePolicy 设置资源策略（见包注释；不受装配锁定，切换即生效）。
func (wv *WebView) SetResourcePolicy(p ResourcePolicy) {
	if wv == nil {
		return
	}
	wv.resourcePolicy = p
	wv.resourcePolicySet = true
	// 策略变化 = 「哪些引用能取到」的语义变了：清空资源缓存，否则旧策略下
	// 取回的内容会继续被使用（与 SetResourceResolver 同一条规矩）。
	wv.ClearResourceCache()
}

// ResourcePolicy 返回当前**生效**的策略（未显式设置时按运行模式推导）。
func (wv *WebView) ResourcePolicy() ResourcePolicy {
	if wv == nil {
		return AllowAll
	}
	return wv.effectiveResourcePolicy()
}

// effectiveResourcePolicy 求当前生效策略：显式设置优先，否则按模式推导
//（ModeBrowser → AllowAll；ModeToolkit → DenyExternal，即历史行为）。
func (wv *WebView) effectiveResourcePolicy() ResourcePolicy {
	if wv == nil {
		return AllowAll
	}
	if wv.resourcePolicySet {
		return wv.resourcePolicy
	}
	if wv.mode == ModeToolkit {
		return DenyExternal
	}
	return AllowAll
}

// allowsExternalURLs 报告**外部通道**（http(s)/file/相对路径）是否放行。
// 只有 AllowAll 在这一层放行；DenyExternal 与 AllowHostResolved 都拒绝
// ——AllowHostResolved 的放行发生在**宿主 resolver 通道**（见
// loadExternalResource：resolver 在门禁之前，命中即返回内容）。
func (wv *WebView) allowsExternalURLs() bool {
	return wv.effectiveResourcePolicy() == AllowAll
}

// resolverProvides 报告宿主 ResourceResolver 是否能提供该引用（只探测：
// 不取内容、不写缓存）。AllowHostResolved 策略据此逐 URL 判定放行
//（TC-M-903：resolver 能解析的 file:// 放行；resolver 返回空仍拒）。
func (wv *WebView) resolverProvides(ref string) bool {
	if wv == nil || ref == "" || wv.resourceResolver == nil {
		return false
	}
	_, ok := wv.resourceResolver(ref)
	return ok
}
