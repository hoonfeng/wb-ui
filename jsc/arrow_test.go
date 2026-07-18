package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestArrowThisBinding2(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 更简单的测试
	vm.Run(`
// 测试1: 全局作用域中的箭头函数
var arrow1 = () => typeof this;
console.log('TOP_arrow: ' + arrow1());

// 测试2: 严格模式 IIFE 中的箭头函数
(function() {
  "use strict";
  var arrow2 = () => typeof this;
  console.log('IIFE_arrow: ' + arrow2());
  
  // 测试3: IIFE 中的对象字面量箭头函数
  var obj = {
    method: () => typeof this
  };
  console.log('IIFE_obj_arrow: ' + obj.method());
  
  // 测试4: 通过.call调用箭头函数
  var fake = {};
  console.log('IIFE_call_arrow: ' + arrow2.call(fake));
})();
`)

	out := logger.String()
	fmt.Fprintf(os.Stderr, "Results:\n%s\n", out)
}
