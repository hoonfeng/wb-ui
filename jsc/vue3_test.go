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

// Method 1: Direct push
console.log("=== Method 1: p.push(4) ===");
p.push(4);
console.log("len=" + p.length + " p3=" + p[3]);

// Method 2: Re-get push each time
var t2 = [1, 2, 3];
var p2 = new Proxy(t2, h);
console.log("=== Method 2: get push each time ===");
p2["push"](4);
console.log("len=" + p2.length + " p3=" + p2[3]);

// Method 3: target approach
var t3 = [1, 2, 3];
var p3 = new Proxy(t3, h);
console.log("=== Method 3: check target ===");
var pushFn = Array.prototype.push;
pushFn.call(p3, 4);
console.log("len=" + p3.length + " p3=" + p3[3]);
console.log("target_len=" + t3.length + " target_3=" + t3[3]);
`
	
	_, err := vm.Run(code)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	
	out := logger.String()
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", out)
}
