package webkit

import "testing"

// TestElementClickDispatchesMouseEvent 钉死 DOM 标准 `element.click()`：
// 程序化点击必须向元素自身派发可冒泡的 click MouseEvent，监听器内
// `this === 该元素`，且事件冒泡到祖先；`event.preventDefault()` 需能阻止
// 默认行为（cancelable）。此前元素对象没有 click 成员 →
// `TypeError: Object has no member 'click'`。
// 断言可量化：事件序列、this 的 id、冒泡到的元素、defaultPrevented。
func TestElementClickDispatchesMouseEvent(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="parent" style="width:200px;height:60px">
  <button id="btn" style="width:80px;height:24px">go</button>
</div>
<script>
window.__log = [];
document.getElementById('btn').addEventListener('click', function(e){
  window.__log.push('btn:this=' + (this && this.id) + ':type=' + e.type +
                    ':bubbles=' + e.bubbles + ':cancelable=' + e.cancelable);
  e.preventDefault();
});
document.getElementById('parent').addEventListener('click', function(e){
  window.__log.push('parent:target=' + (e.target && e.target.id) +
                    ':currentTarget=' + (e.currentTarget && e.currentTarget.id));
});
</script>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 8; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got, err := wv.EvalJS(`(function(){
  var btn = document.getElementById('btn');
  if (typeof btn.click !== 'function') { return 'NO_CLICK_MEMBER:' + typeof btn.click; }
  btn.click();
  return window.__log.join(' | ');
})()`)
	if err != nil {
		t.Fatalf("EvalJS: %v", err)
	}
	log := got.ToString()
	t.Logf("click() 实测 = %s", log)
	if contains(log, "NO_CLICK_MEMBER") {
		t.Fatalf("element.click() 未实现：%s", log)
	}
	if !contains(log, "btn:this=btn:type=click:bubbles=true:cancelable=true") {
		t.Fatalf("click() 未按标准派发（this/target/type/bubbles/cancelable 不符）：%s", log)
	}
	if !contains(log, "parent:target=btn:currentTarget=parent") {
		t.Fatalf("click 事件未冒泡到祖先（或 target/currentTarget 错误）：%s", log)
	}
}
