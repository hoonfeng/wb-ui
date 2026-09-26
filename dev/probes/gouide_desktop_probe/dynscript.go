package main

import (
	"fmt"

	"wb-ui/webkit"
)

// 探测「运行时动态插入的 <script src> 是否被引擎加载并执行」。
//
// 这是 gou-ide 区域包 client 半的装载方式：client.js 里先
// document.createElement('script') + s.src = 编译 bundle + appendChild，
// 由 bundle 暴露 window.<Global>.mount 后再 registerSlot 注册槽位
// （见 /plugins-assets/ui-titlebar/client.js）。若动态脚本不执行，
// 槽位表就是空的 → 各区域槽位全部「未装配」空态。
func runDynScriptProbe(wv *webkit.WebView) {
	const inject = `(function () {
  window.__dyn = { inserted: false, onload: '', onerror: '', globalBefore: typeof window.UiTitlebar };
  try {
    var s = document.createElement('script');
    s.src = '/plugins-assets/ui-titlebar/assets/ui-titlebar.js?dynprobe=1';
    s.onload = function () { window.__dyn.onload = 'yes'; };
    s.onerror = function () { window.__dyn.onerror = 'yes'; };
    document.head.appendChild(s);
    window.__dyn.inserted = true;
  } catch (e) { window.__dyn.err = String(e && e.message || e); }
  return 'ok';
})()`

	fmt.Println("=== 动态 <script src> 注入实验 ===")
	fmt.Println("inject   :", evalStr(wv, inject))
	pump(wv, 30)
	fmt.Println("script 数:", evalStr(wv, `String(document.querySelectorAll('script').length)`))
	fmt.Println("全局     :", evalStr(wv, `typeof window.UiTitlebar`))
	fmt.Println("__dyn    :", evalStr(wv, `JSON.stringify(window.__dyn)`))
}
