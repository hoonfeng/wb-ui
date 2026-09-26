package webkit

import "testing"

// evtDedupPage 探针页：运行时对元素做「重复注册 / 精确移除」，把计数写回
// window，用于断言引擎的监听器语义与浏览器（DOM 规范）一致。
//
// 背景（2026-09-26 实测）：引擎此前 addEventListener 去重失效——bindings 层
// 每次调用都新建 Go listener，而 dom.EventTarget 按 listener 对象身份判重
// （listenerEquals），两者永不相等，于是：
//   - 同一 JS 回调注册两次 → 事件被处理两次；
//   - removeEventListener 只能移掉其中一个（且按注册顺序错配）。
// 桌面端表现：Vue 的 @change 在 RightPanel 上累积到 2 个 → 选择工具集时
// 「本对话已切换工具集为 X」toast 弹两条；浏览器（Blink 自带去重）只有一条。
const evtDedupPage = `<!DOCTYPE html><html><head><title>evt-dedup</title></head><body>
<div id="host"></div>
<script>
  // 1) 同一回调注册两次（第二次应 no-op），再移除一次 → 不应触发。
  (function () {
    var t = document.getElementById('host');
    var c = 0;
    var f = function () { c++; };
    t.addEventListener('change', f);
    t.addEventListener('change', f);
    t.removeEventListener('change', f);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__dupThenRemove = c;
  })();

  // 2) 同一回调注册两次且不移除 → 规范要求仍只触发一次。
  (function () {
    var t = document.createElement('div');
    document.getElementById('host').appendChild(t);
    var c = 0;
    var f = function () { c++; };
    t.addEventListener('change', f);
    t.addEventListener('change', f);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__dupNoRemove = c;
  })();

  // 3) 单次注册（正向对照）→ 触发一次。
  (function () {
    var t = document.createElement('div');
    document.getElementById('host').appendChild(t);
    var c = 0;
    var f = function () { c++; };
    t.addEventListener('change', f);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__single = c;
  })();

  // 4) 两个不同回调 → 各触发一次（去重不得误伤不同回调）。
  (function () {
    var t = document.createElement('div');
    document.getElementById('host').appendChild(t);
    var c = 0;
    var f1 = function () { c++; };
    var f2 = function () { c += 10; };
    t.addEventListener('change', f1);
    t.addEventListener('change', f2);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__twoCallbacks = c;
  })();

  // 5) capture 与 bubble 是两次独立注册（同一回调）：只移除 capture 后，
  //    bubble 那次仍须生效 —— 去重键必须带 capture 标志，否则会把 bubble
  //    注册当成重复而丢弃、或在移除时误删 bubble。
  (function () {
    var t = document.createElement('div');
    document.getElementById('host').appendChild(t);
    var c = 0;
    var f = function () { c++; };
    t.addEventListener('change', f);
    t.addEventListener('change', f, true);
    t.removeEventListener('change', f, true);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__captureRemoveKeepsBubble = c;
  })();

  // 6) 移除一个回调不得影响另一个（同名不同函数的注册彼此独立）。
  (function () {
    var t = document.createElement('div');
    document.getElementById('host').appendChild(t);
    var c = 0;
    var f = function () { c++; };
    var g = function () { c += 100; };
    t.addEventListener('change', f);
    t.addEventListener('change', g);
    t.removeEventListener('change', f);
    t.dispatchEvent(new Event('change', { bubbles: true }));
    window.__removeOne = c;
  })();

  // 7) 同一回调挂在两个不同元素上 → 各自独立触发（去重键必须带 target）。
  (function () {
    var host = document.getElementById('host');
    var a = document.createElement('div');
    var b = document.createElement('div');
    host.appendChild(a);
    host.appendChild(b);
    var ca = 0, cb = 0;
    var f = function () { ca++; };
    a.addEventListener('change', f);
    b.addEventListener('change', function () { cb++; });
    a.dispatchEvent(new Event('change', { bubbles: true }));
    b.dispatchEvent(new Event('change', { bubbles: true }));
    window.__twoTargets = ca * 10 + cb;
  })();
</script>
</body></html>`

// TestEventListenerDedupAndRemoval 锁定 addEventListener/removeEventListener
// 的浏览器语义：同 (type, callback, capture) 重复注册为 no-op（只保留一次），
// 移除精确匹配，capture 与 target 参与身份区分。
func TestEventListenerDedupAndRemoval(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	if err := wv.LoadHTML(evtDedupPage); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	read := func(name string) string {
		t.Helper()
		v, err := wv.EvalJS("String(window." + name + ")")
		if err != nil {
			t.Fatalf("EvalJS(%s): %v", name, err)
		}
		return v.ToString()
	}
	cases := []struct{ name, want, why string }{
		{"__dupThenRemove", "0", "同回调注册两次再移除一次后仍触发（重复注册未 no-op / 移除未精确）"},
		{"__dupNoRemove", "1", "同回调注册两次被触发多次（重复注册未按规范 no-op）"},
		{"__single", "1", "单次注册未触发"},
		{"__twoCallbacks", "11", "两个不同回调未各触发一次"},
		{"__captureRemoveKeepsBubble", "1", "移除 capture 注册误伤 bubble 注册（去重键缺 capture）"},
		{"__removeOne", "100", "移除一个回调后另一个回调被误删"},
		{"__twoTargets", "11", "同一回调挂两个 target 时串台（去重键缺 target）"},
	}
	for _, c := range cases {
		if got := read(c.name); got != c.want {
			t.Errorf("%s = %s，want %s：%s", c.name, got, c.want, c.why)
		}
	}
}
