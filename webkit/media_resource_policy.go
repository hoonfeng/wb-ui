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
// 本文件提供媒体路径的判定入口 MediaResourceAllowed：判定的**规则与顺序**不在这里
// 实现——已收进公共判定内核 `resource_ref_policy.go`（一份判定：resolverRefForms /
// classifyResourceRef / resourceRefAllowed），媒体路径与 loadExternalResource
// （图片/样式/脚本的字节路径）**两处执行、同一判定**，任何一侧都不得再立第二份规则。
// 本文件只标出**媒体侧的两个特征**：
//
//  1. 宿主 ResourceResolver **命中即放行**（任何策略下）：resolver 通道先于策略，
//     宿主显式提供的资源不受策略限制（AllowHostResolved 档正是靠它放行，见
//     resource_policy.go）；
//  2. 媒体通道**没有网络通道**：`http(s)` / `blob:` 因此**恒拒**——这是**能力边界**
//     而非策略差异（图片有 fetchResource 通道、媒体没有；现状即如此，未放宽）。该
//     差异以 `hasNetworkChannel=false` 显式传给判定内核，而不是在这里写一遍规则。

package webkit

import "strings"

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
	// ① 宿主 resolver 命中即放行：询问序列（原样 → 绝对化）与 loadExternalResource
	//    共用同一份实现——「问哪些形态」因此不会在两条链路之间漂移。
	for _, form := range wv.resolverRefForms(s) {
		if wv.resolverProvides(form) {
			return true
		}
	}
	// ② 策略判定（唯一实现）：data: 恒放行、file/相对路径仅 AllowAll 放行、
	//    http(s)/blob 在**没有网络通道**的媒体路径上恒拒。
	return wv.resourceRefAllowed(s, false)
}
