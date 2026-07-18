package jsc

import (
	"fmt"
	"os"
	"testing"
)

func TestIIFEActual(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)
	
	// Exact replacement pattern
	code := `
		var UV={use:function(x){return this},mount:function(el){console.log("MOUNT_CALLED:"+el)}};
		var SHe=function(){return "router"};
		var $Xe=function(){return "pinia"};
		UV.use(SHe()),UV.use($Xe),(function(){console.log("M_MOUNT");try{UV.mount("#app");console.log("M_MOUNT_OK")}catch(_m){console.log("M_MOUNT_ERR:"+_m)}})();
		console.log("AFTER_MOUNT");
	`
	_, err := vm.Run(code)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Output:\n%s\n", logger.String())
}
