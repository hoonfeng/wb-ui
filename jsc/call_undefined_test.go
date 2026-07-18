package jsc

import (
	"fmt"
	"os"
	"testing"
)

// TestCallUndefined 测试 JSC 调用 undefined 时的行为和错误消息
func TestCallUndefined(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)
	vm.SetupGlobal(logger)

	// 直接测试调用 undefined
	vm.Run(`
try {
  var x = undefined;
  x();
} catch(e) {
  console.log('ERR1: ' + e);
}

try {
  var obj = {};
  obj.nonexistent();
} catch(e) {
  console.log('ERR2: ' + e);
}

try {
  (undefined)();
} catch(e) {
  console.log('ERR3: ' + e);
}
`)

	out := logger.String()
	fmt.Fprintf(os.Stderr, "Results:\n%s\n", out)
}
