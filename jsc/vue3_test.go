package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestVue3PushDebug(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	code := `
var t = [1, 2, 3];
var h = {
    set: function(t, k, v, r) {
        console.log("SET:"+k+"="+v);
        return Reflect.set(t, k, v, r);
    },
    get: function(t, k, r) {
        return Reflect.get(t, k, r);
    }
};
var p = new Proxy(t, h);

// Test direct set: should trigger SET trap
console.log("=== p[0] = 99 ===");
p[0] = 99;
console.log("p[0]=" + p[0] + " t[0]=" + t[0]);

// Test push through proxy: should trigger SET trap
console.log("=== p.push(4) ===");
p.push(4);
console.log("len=" + p.length + " p[3]=" + p[3] + " t[3]=" + t[3]);

// Test push on newly-created proxy
var t2 = [1, 2, 3];
var p2 = new Proxy(t2, h);
console.log("=== p2.push(5) ===");
p2.push(5);
console.log("len=" + p2.length + " p2[3]=" + p2[3] + " t2[3]=" + t2[3]);
`
	
	_, err := vm.Run(code)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	
	out := logger.String()
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", out)
}
