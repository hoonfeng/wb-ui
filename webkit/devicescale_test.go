package webkit

import "testing"

// TestDeviceScaleFactor 钉死 S3（CDP Emulation.setDeviceMetricsOverride 的
// deviceScaleFactor）在**引擎侧**的行为：
//  ① 默认 window.devicePixelRatio === 1（不改既有行为）
//  ② SetDeviceScaleFactor(2) → devicePixelRatio === 2，且 (min-resolution: 2dppx)
//     命中的样式**立即重算**（媒体查询随设备像素比变化）
//  ③ 回 1 → 媒体查询不再命中、样式回到基础规则
//
// 反向验证：把 SetDeviceScaleFactor 里的 InvalidateSubtree + MarkRenderTreeDirty
// 去掉，②的样式断言会失败（getComputedStyle 命中旧缓存 → 画面停在旧样式）。
func TestDeviceScaleFactor(t *testing.T) {
	wv := loadAndSettle(t, `<!DOCTYPE html><html><head><style>
#a { color: rgb(1, 2, 3); }
@media (min-resolution: 2dppx) { #a { color: rgb(4, 5, 6); } }
</style></head><body style="margin:0"><div id="a">x</div></body></html>`)

	read := func() string {
		t.Helper()
		return dynEval(t, wv, `(function(){
  var cs = getComputedStyle(document.getElementById("a"));
  return window.devicePixelRatio + "|" + cs.color + "|" +
         matchMedia("(min-resolution: 2dppx)").matches + "|" +
         matchMedia("(min-resolution: 192dpi)").matches;
})()`)
	}

	if got := read(); got != "1|rgb(1, 2, 3)|false|false" {
		t.Fatalf("DSF=1 初始状态 = %q，want %q", got, "1|rgb(1, 2, 3)|false|false")
	}
	wv.SetDeviceScaleFactor(2)
	if got := read(); got != "2|rgb(4, 5, 6)|true|true" {
		t.Fatalf("DSF=2 = %q，want %q（2dppx 与 192dpi 等价）", got, "2|rgb(4, 5, 6)|true|true")
	}
	wv.SetDeviceScaleFactor(1)
	if got := read(); got != "1|rgb(1, 2, 3)|false|false" {
		t.Fatalf("回到 DSF=1 = %q，want %q", got, "1|rgb(1, 2, 3)|false|false")
	}
	// 非正数按 1 处理（CDP 缺省 deviceScaleFactor=0 表示「跟随设备」）。
	wv.SetDeviceScaleFactor(0)
	if got := read(); got != "1|rgb(1, 2, 3)|false|false" {
		t.Fatalf("DSF=0 应回落到 1：%q", got)
	}
}
