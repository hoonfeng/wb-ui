package jsc

import (
	"fmt"
	"testing"
)

func TestObjectGetProto(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 检查 Object 上的方法
	vm.Run(`
console.log('O_assign: '+(typeof Object.assign));
console.log('O_create: '+(typeof Object.create));
console.log('O_keys: '+(typeof Object.keys));
console.log('O_defineProperty: '+(typeof Object.defineProperty));
console.log('O_getPrototypeOf: '+(typeof Object.getPrototypeOf));
console.log('O_setPrototypeOf: '+(typeof Object.setPrototypeOf));
console.log('O_getOwnPropertyNames: '+(typeof Object.getOwnPropertyNames));
console.log('O_hasOwn: '+(typeof Object.hasOwn));
console.log('O_fromEntries: '+(typeof Object.fromEntries));
console.log('O_is: '+(typeof Object.is));
console.log('O_preventExtensions: '+(typeof Object.preventExtensions));
console.log('O_seal: '+(typeof Object.seal));
console.log('O_freeze: '+(typeof Object.freeze));
console.log('O_isExtensible: '+(typeof Object.isExtensible));
console.log('O_isSealed: '+(typeof Object.isSealed));
console.log('O_isFrozen: '+(typeof Object.isFrozen));
console.log('O_entries: '+(typeof Object.entries));
console.log('O_values: '+(typeof Object.values));
console.log('O_getOwnPropertyDescriptor: '+(typeof Object.getOwnPropertyDescriptor));
`)
	out := logger.String()
	fmt.Printf("=== Object API ===\n%s\n", out)
}
