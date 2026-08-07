package webkit

import (
	"testing"
)

// TestCallFunctionThisPersist: CallFunction 两次调用共享全局 this 时，
// 属性应持久（浏览器语义：window.count 连续累加）。
func TestCallFunctionThisPersist(t *testing.T) {
	wv := NewWebView()
	src := `<html><body><script>
		window.bump = function(n) { this.count = (this.count || 0) + n; return this.count; };
	</script></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	v, err := wv.CallFunction("bump", 2)
	if err != nil {
		t.Fatalf("CallFunction(bump,2): %v", err)
	}
	t.Logf("bump(2) = %v", v.ToNumber())
	// 引擎内读 window.count（绕过 CallFunction）
	rv, err := wv.EvalJS("window.count")
	if err != nil {
		t.Fatalf("EvalJS(window.count): %v", err)
	}
	t.Logf("window.count after bump(2) = %v", rv.ToNumber())
	v, err = wv.CallFunction("bump", 3)
	if err != nil {
		t.Fatalf("CallFunction(bump,3): %v", err)
	}
	t.Logf("bump(3) = %v", v.ToNumber())
	if got := v.ToNumber(); got != 5 {
		t.Errorf("bump(2)+bump(3) = %v, want 5", got)
	}
}
