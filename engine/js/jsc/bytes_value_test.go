package jsc

import "testing"

// TestValueOfBytesShape 钉住 `[]byte` → JS 的形态：**Uint8Array**（有 .buffer）。
//
// 为什么值得钉：TC-M-603 的探针自检实测（diag: global=object 且无 .buffer）暴露出
// ValueOf 承诺的「[]byte → Uint8Array」**并未兑现**——它走的是 `vm.Call(Uint8Array,
// undefined, ab)` 失败后的兜底分支，实际拿到的是 ArrayBuffer（typed array 构造器
// 要求 new 调用，Call 会被拒）。这类「注释说 A、实际是 B」的偏差会让调用方按
// Uint8Array 读 `.buffer` 时踩空，因此把形态固定成测试：**必须**是 Uint8Array。
func TestValueOfBytesShape(t *testing.T) {
	rt := NewInterpreter()
	rt.SetupGlobal(&BufferLogger{})
	rt.GlobalObject().Set("__bytes", rt.ValueOf([]byte{1, 2, 3, 4}))
	v, err := rt.RunJS(`(function(){
		var b = __bytes;
		return (typeof b) + "|ctor=" + (b && b.constructor && b.constructor.name) +
			"|" + (b && ("buffer" in b) ? "has-buffer" : "no-buffer") +
			"|byteLength=" + (b ? b.byteLength : "n/a");
	})()`)
	if err != nil {
		t.Fatalf("RunJS: %v", err)
	}
	const want = "object|ctor=Uint8Array|has-buffer|byteLength=4"
	if got := v.ToString(); got != want {
		t.Fatalf("[]byte 注入形态 = %q，期望 %q", got, want)
	}
}
