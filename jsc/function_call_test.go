package jsc

import (
	"testing"
	"fmt"
)

func TestFunctionCall(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	code := `
		"use strict";
		
		// 测试 Function.prototype.call
		function greet(greeting) {
			return greeting + ' ' + this.name;
		}
		
		var obj = {name: 'World'};
		
		// 直接调用
		var r1 = greet.call(obj, 'Hello');
		console.log('CALL_RESULT: ' + r1);
		
		// 用变量保存再 call
		var fn = greet;
		var r2 = fn.call(obj, 'Hi');
		console.log('CALL_VAR_RESULT: ' + r2);
		
		// 箭头函数 call
		var arrow = (a, b) => { return a + ' ' + b; };
		var r3 = arrow.call(null, 'hello', 'world');
		console.log('ARROW_CALL: ' + r3);
		
		// call 传入 undefined this
		function strictFn() { return typeof this; }
		var r4 = strictFn.call(undefined);
		console.log('STRICT_CALL: ' + r4);
		
		// 使用 Reflect.apply
		if(typeof Reflect !== 'undefined' && Reflect.apply) {
			var r5 = Reflect.apply(greet, obj, ['Hey']);
			console.log('REFLECT_APPLY: ' + r5);
		} else {
			console.log('NO_REFLECT_APPLY');
		}
		
		console.log('ALL_DONE');
	`
	
	if _, err := vm.Run(code); err != nil {
		t.Fatalf("run err: %v", err)
	}
	
	out := log.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
