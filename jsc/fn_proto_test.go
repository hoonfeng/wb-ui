package jsc

import (
	"fmt"
	"testing"
)

func TestFunctionProtoToStringFix(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	code := `
console.log('FP_TYPE: '+(typeof Function.prototype.toString));
console.log('FP_VAL: '+Function.prototype.toString.call(function(){}));
console.log('FP_NAMED: '+Function.prototype.toString.call(function foo(){}));
console.log('FP_OBJ: '+Object.prototype.toString.call(function(){}));
console.log('F_GLOBAL: '+(typeof Function));
console.log('F_PROTO: '+(Function.prototype === (function(){}).__proto__));
`
	if _, err := vm.Run(code); err != nil {
		t.Fatalf("error: %v", err)
	}
	fmt.Printf("--- OUTPUT ---\n%s\n", log.String())
}
