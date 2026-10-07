// 媒体资源门禁（PurposeMedia）：`<video>/<audio>` 的资源选择必须与
// `<img>`/`<script>`/`<link>` 走**同一套**资源策略判定（语义收口的修法，
// 见 docs/media-format-verification-plan.md §9.9）。
//
// 为什么媒体不能直接复用 loadExternalResource：**内容形态不同**。图片/样式/脚本
// 的字节要进引擎（字符串通道 + 资源缓存），走 mode.go 的 loadExternalResource；
// 而媒体的解码全在宿主（引擎不背解码器与许可，见 app/mediaprobe.go 的文件头）：
// 引擎只维护状态机，元数据/帧/PCM 全由宿主按 src 注入。媒体引用因此没有
// 「引擎取内容」这一步，也就没有现成的门禁可挂——修复前媒体链路的门禁**完全
// 缺失**：同一来源（file:// / 相对路径）在 Toolkit+DenyExternal 下 `<img>` 与
// background 是 L0（拒绝），`<video>/<audio>` 却照旧被宿主读盘并解码到 L4。
//
// 本文件提供判定入口 MediaResourceAllowed：判定的**基元与顺序**与
// loadExternalResource 同源（宿主 resolver → data: → 资源策略门禁），只是不取
// 内容、不写缓存。宿主侧（app 层的媒体链路）与引擎侧（bindings 的资源选择）
// 都问它，做到「一份判定、两处执行」——不允许媒体链路自己再立一套规则：
//
//  1. 宿主 ResourceResolver **命中即放行**（任何策略下）——与
//     loadExternalResource 一致：resolver 在门禁之前，宿主显式提供的资源不受
//     策略限制（AllowHostResolved 档正是靠它放行，见 resource_policy.go）；
//  2. `data:` **与策略解耦，恒放行**（自带内容、不经任何外部通道）；
//  3. `http(s)` / `blob:` **恒拒**——媒体通道没有网络栈（引擎不代宿主联网，宿主
//     也没有网络媒体解码通道）。这与 `<img>` 的差异属**能力边界**而非策略差异：
//     图片有 fetchResource 通道，媒体没有；现状即如此，本次未放宽；
//  4. `file://` / 相对路径：只有 AllowAll（外部通道全放行）才放行。

package webkit

import (
	"strings"

	"wb-ui/engine/dom"
)

// MediaResourceAllowed 报告媒体引用 ref 是否允许被宿主导入（元数据探测 /
// 视频抽帧 / 音频 PCM）。判定见文件头注释——与 loadExternalResource 同一条判定
// （resolver → data: → 策略），只是不取内容。
//
// 宿主侧的媒体链路（app 层 mediaSrcToPath）与引擎侧的资源选择
//（bindings.MediaSrcAllowed）都必须先问本方法；未获授权时不得读盘、不得注入
// 元数据/帧/PCM。
func (wv *WebView) MediaResourceAllowed(ref string) bool {
	if wv == nil {
		return false
	}
	s := strings.TrimSpace(ref)
	if s == "" {
		return false
	}
	// ① 宿主 resolver 命中（先原样、再绝对化后各问一次）：与 loadExternalResource
	//    的两轮询问一致——宿主可能只认逻辑名（app://…）或只认绝对 URL。
	if wv.resolverProvides(s) {
		return true
	}
	// 无文档 URL（LoadHTML 直出内容）时不做绝对化，与 loadExternalResource 一致。
	if base := wv.documentBaseURL(); base != "" {
		if abs := dom.ResolveURL(base, s); abs != "" && abs != s {
			if wv.resolverProvides(abs) {
				return true
			}
		}
	}
	// ② data:：自带内容，与策略解耦，恒放行。
	if IsPolicyFreeResourceRef(s) {
		return true
	}
	low := strings.ToLower(s)
	// ③ 媒体通道没有网络栈：http(s)/blob 恒拒（任何策略下都不放行）。
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") ||
		strings.HasPrefix(low, "blob:") {
		return false
	}
	// ④ file:// / 相对路径：只有「外部通道全放行」的策略才放行。
	return wv.allowsExternalURLs()
}

// IsPolicyFreeResourceRef 报告引用是否**与资源策略解耦**（自带内容、不经任何外部
// 通道）：目前只有 `data:`（RFC 2397 内联内容）。
//
// MediaResourceAllowed 的第 ② 条与本函数是**同一份实现**——宿主在「没有 WebView
// 判定可用」时（例如 app 层的门禁尚未装配）也用它保持同一条语义，避免 data: 的
// 放行规则在宿主侧出现第二份（`<img>` 的 data: 放行同样是这条规则）。
func IsPolicyFreeResourceRef(ref string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "data:")
}
