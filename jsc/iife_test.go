package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestProxyPushDebug4(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	code := `
var t = [1, 2, 3];
var h = { set: function(t,k,v,r) { console.log("SET:"+k); return Reflect.set(t,k,v,r); }, get: function(t,k,r) { console.log("GET:"+k); return Reflect.get(t,k,r); } };
var p = new Proxy(t, h);

// Simple test: just typeof p.push
console.log("TYPEOF_PUSH: " + (typeof p.push));

// Direct method call
console.log("BEFORE_PUSH");
var r = p.push(4);
console.log("PUSH_RETURNED: " + r);
console.log("AFTER: len=" + p.length);
`
	
	_, err := vm.Run(code)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	
	out := logger.String()
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", out)
}
